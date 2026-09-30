import base64
import hashlib
import json
import socket
import struct
import sys
import threading
import unittest
from unittest.mock import patch
sys.dont_write_bytecode = True
import boot_image


class BootProtocolTests(unittest.TestCase):
    def exchange(self, transform):
        client, server = socket.socketpair()
        client.settimeout(2)
        server.settimeout(2)
        errors = []
        def reply():
            try:
                size = struct.unpack(">I", boot_image.read_exact(server, 4))[0]
                request = json.loads(boot_image.read_exact(server, size))
                message = transform(request)
                payload = message if isinstance(message, bytes) else json.dumps(message).encode()
                frame = struct.pack(">I", len(payload)) + payload
                for part in (frame[:3], frame[3:9], frame[9:]):
                    server.sendall(part)
            except Exception as error:
                errors.append(error)
            finally:
                server.close()
        thread = threading.Thread(target=reply)
        thread.start()
        try:
            return boot_image.request(client, "health")
        finally:
            client.close()
            thread.join(timeout=3)
            self.assertFalse(thread.is_alive())
            self.assertEqual(errors, [])

    def test_fragmented_health_response(self):
        response = self.exchange(lambda r: {"version": 1, "requestId": r["requestId"], "state": "ready"})
        self.assertEqual(response["state"], "ready")

    def test_invalid_guest_response_refused(self):
        for kind in ("identity", "error", "unknown", "boolean", "duplicate", "float", "negative", "text-type"):
            with self.subTest(kind=kind):
                def transform(request):
                    response = {"version": 1, "requestId": request["requestId"], "state": "ready"}
                    if kind == "identity": response["requestId"] = "different"
                    if kind == "error": response["error"] = "WORKLOAD_UNAVAILABLE"
                    if kind == "unknown": response["unrecognized"] = True
                    if kind == "boolean": response["version"] = True
                    if kind == "float": response["offset"] = 1.0
                    if kind == "negative": response["size"] = -1
                    if kind == "text-type": response["data"] = []
                    if kind == "duplicate": return b'{"version":1,"version":1}'
                    return response
                with self.assertRaises(ValueError):
                    self.exchange(transform)

    def test_frame_bound_before_response_allocation(self):
        client, server = socket.socketpair()
        with client, server:
            client.settimeout(2)
            # Small request cannot fill the local socket buffer; peer supplies only
            # an oversized header, so refusal must happen before reading a body.
            server.sendall(struct.pack(">I", (512 << 10) + 1))
            with self.assertRaises(ValueError):
                boot_image.request(client, "health")


class ObjectTransferTests(unittest.TestCase):
    def transfer(self, fault=None):
        stored = bytearray()
        uploads = []
        deleted = []
        def adapter(channel, operation, **fields):
            if operation == "upload":
                chunk = base64.b64decode(fields["data"], validate=True)
                self.assertEqual(hashlib.sha256(chunk).hexdigest(), fields["sha256"])
                offset = fields["offset"]
                uploads.append(offset)
                if offset == len(stored):
                    stored.extend(chunk)
                else:
                    self.assertEqual(stored[offset:offset + len(chunk)], chunk)
                if fault == "replay" and len(uploads) == 3:
                    return {"offset": 0}
                return {"offset": len(stored)}
            if operation == "finalize":
                self.assertEqual(len(stored), fields["size"])
                self.assertEqual(hashlib.sha256(stored).hexdigest(), fields["sha256"])
                return {"size": len(stored), "sha256": "wrong" if fault == "finalize" else fields["sha256"]}
            if operation == "download":
                offset = fields["offset"]
                chunk = stored[offset:offset + (256 << 10)]
                if fault == "empty":
                    chunk = b""
                return {"offset": offset + len(chunk) + (1 if fault == "offset" else 0),
                        "data": base64.b64encode(chunk).decode(),
                        "sha256": "wrong" if fault == "digest" else hashlib.sha256(chunk).hexdigest()}
            self.assertEqual(operation, "delete")
            deleted.append(fields["objectId"])
            return {}
        with patch.object(boot_image, "request", side_effect=adapter):
            evidence = boot_image.object_roundtrip(None)
        self.assertEqual(uploads, [0, 262144, 262144, 524288, 786432, 1048576])
        self.assertEqual(len(deleted), 1)
        self.assertEqual(evidence, {"bytes": 1048579, "chunkBytes": 262144, "acknowledgedChunkReplay": True})

    def test_chunked_transfer_and_replay(self):
        self.transfer()

    def test_bad_transfer_evidence_refused(self):
        for fault in ("replay", "finalize", "empty", "offset", "digest"):
            with self.subTest(fault=fault), self.assertRaises(ValueError):
                self.transfer(fault)


if __name__ == "__main__":
    unittest.main()
