import json
import socket
import struct
import sys
import threading
import unittest
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
        for kind in ("identity", "error", "unknown", "boolean", "duplicate"):
            with self.subTest(kind=kind):
                def transform(request):
                    response = {"version": 1, "requestId": request["requestId"], "state": "ready"}
                    if kind == "identity": response["requestId"] = "different"
                    if kind == "error": response["error"] = "WORKLOAD_UNAVAILABLE"
                    if kind == "unknown": response["unrecognized"] = True
                    if kind == "boolean": response["version"] = True
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


if __name__ == "__main__":
    unittest.main()
