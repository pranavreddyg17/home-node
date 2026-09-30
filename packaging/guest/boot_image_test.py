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
    def exchange(self, transform, allowed_error=None):
        client, server = socket.socketpair()
        client.settimeout(2)
        server.settimeout(2)
        errors = []
        def reply():
            try:
                size = struct.unpack(">I", boot_image.read_exact(server, 4))[0]
                request = json.loads(boot_image.read_exact(server, size))
                self.assertNotIn("allowed_error", request)
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
            return boot_image.request(client, "health", allowed_error=allowed_error)
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

    def test_only_explicit_guest_error_is_allowed(self):
        response = self.exchange(lambda r: {"version": 1, "requestId": r["requestId"], "error": "OBJECT_BUSY"}, "OBJECT_BUSY")
        self.assertEqual(response["error"], "OBJECT_BUSY")
        with self.assertRaises(ValueError):
            self.exchange(lambda r: {"version": 1, "requestId": r["requestId"], "error": "OPERATION_FAILED"}, "OBJECT_BUSY")


class VideoCancellationTests(unittest.TestCase):
    def test_cancel_waits_for_deletion_then_health(self):
        with patch.object(boot_image, "request", side_effect=[
                {"state": "running"}, {"state": "cancelled"}, {"state": "cancelled"},
                {"error": "OBJECT_BUSY"}, {"state": "cancelled"}, {}, {"error": "NOT_FOUND"}, {"state": "ready"}]) as calls, \
                patch.object(boot_image.time, "sleep"):
            self.assertEqual(boot_image.cancel_video(None, "input"),
                             {"cancelled": True, "taskDeletionAcknowledged": True})
        self.assertEqual([c.args[1] for c in calls.call_args_list], ["run", "cancel", "result", "delete", "result", "delete", "result", "health"])
        self.assertEqual(calls.call_args_list[3].kwargs["allowed_error"], "OBJECT_BUSY")
        self.assertEqual(calls.call_args_list[0].kwargs["inputId"], "input")

    def test_cancel_state_and_teardown_deadline_are_required(self):
        sequences = [
            [{"state": "succeeded"}],
            [{"state": "running"}, {"state": "succeeded"}],
            [{"state": "running"}, {"state": "cancelled"}, {"state": "succeeded"}],
            [{"state": "running"}, {"state": "cancelled"}, {"state": "cancelled"}, {"error": "OBJECT_BUSY"}],
            [{"state": "running"}, {"state": "cancelled"}, {"state": "cancelled"}, {}, {"state": "cancelled"}],
            [{"state": "running"}, {"state": "cancelled"}, {"state": "cancelled"}, {}, {"error": "NOT_FOUND"}, {"state": "starting"}],
        ]
        for responses in sequences:
            with self.subTest(responses=responses), patch.object(boot_image, "request", side_effect=responses), \
                    patch.object(boot_image.time, "monotonic", side_effect=[0, 31]), self.assertRaises(ValueError):
                boot_image.cancel_video(None, "input")


class AIInferenceTests(unittest.TestCase):
    def test_actual_adapter_sequence_requires_terminal_text_and_delete(self):
        with patch.object(boot_image, "request", side_effect=[{"state": "running"}, {"state": "running"},
                {"state": "succeeded", "text": "héllo"}, {}]) as calls, patch.object(boot_image.time, "sleep"), patch.object(boot_image, "cancel_ai", return_value={"cancelled": True, "taskDeletionAcknowledged": True}):
            self.assertEqual(boot_image.ai_roundtrip(None), {"generationSucceeded": True, "textBytes": 6, "taskDeletionAcknowledged": True, "cancellation": {"cancelled": True, "taskDeletionAcknowledged": True}})
        self.assertEqual([call.args[1] for call in calls.call_args_list], ["generate", "result", "result", "delete"])

    def test_failed_empty_oversized_or_timed_out_inference_refused(self):
        cases = [[{"state": "failed"}], [{"state": "running"}, {"state": "failed"}],
                 [{"state": "running"}, {"state": "succeeded", "text": ""}],
                 [{"state": "running"}, {"state": "succeeded", "text": "x" * 32769}],
                 [{"state": "running"}, {"state": "running"}]]
        for responses in cases:
            with self.subTest(responses=responses[:1]), patch.object(boot_image, "request", side_effect=responses), \
                    patch.object(boot_image.time, "monotonic", side_effect=[0, 121]), patch.object(boot_image, "cancel_ai"), self.assertRaises(ValueError):
                boot_image.ai_roundtrip(None)


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


class AICancellationTests(unittest.TestCase):
    def test_ai_cancel_waits_for_worker_and_deletion(self):
        with patch.object(boot_image, "request", side_effect=[
                {"state": "running"}, {"state": "cancelled"}, {"state": "cancelled"},
                {"error": "OBJECT_BUSY"}, {"state": "cancelled"}, {}, {"error": "NOT_FOUND"}, {"state": "ready"}]) as calls, \
                patch.object(boot_image.time, "sleep"):
            self.assertEqual(boot_image.cancel_ai(None), {"cancelled": True, "taskDeletionAcknowledged": True})
        self.assertEqual([c.args[1] for c in calls.call_args_list], ["generate", "cancel", "result", "delete", "result", "delete", "result", "health"])
        ids = [c.kwargs["objectId"] for c in calls.call_args_list[:-1]]
        self.assertEqual(len(set(ids)), 1)

    def test_ai_completion_is_not_counted_as_cancellation(self):
        with patch.object(boot_image, "request", side_effect=[{"state": "running"}, {"state": "succeeded"}]), self.assertRaises(ValueError):
            boot_image.cancel_ai(None)

    def test_cancellation_failure_prevents_recovery_generation(self):
        with patch.object(boot_image, "cancel_ai", side_effect=ValueError("cancellation failed")), \
                patch.object(boot_image, "request") as calls, self.assertRaises(ValueError):
            boot_image.ai_roundtrip(None)
        calls.assert_not_called()


if __name__ == "__main__":
    unittest.main()
