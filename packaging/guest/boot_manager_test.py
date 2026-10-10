"""Development manager input/client checks without native host effects."""
import importlib.util
import base64
import hashlib
import json
import os
import struct
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import manager_channel

spec = importlib.util.spec_from_file_location("manager_fixture", Path(__file__).resolve().parents[1] / "systemd" / "reserved_manager_fixture.py")
fixture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixture)


class ManagerInputs(unittest.TestCase):
    def record(self):
        return {"schema": 1, "profile": "files", "sourceRevision": "a" * 40,
                "overlaySHA256": "b" * 64, "mkosiRevision": "c" * 40,
                "bytes": 1024, "sha256": "d" * 64,
                "releaseQualified": False, "bootValidated": False}

    def test_bounded_manifest_sets_only_development_identity(self):
        with tempfile.TemporaryDirectory() as directory, mock.patch.dict(os.environ, {"HOMENODE_FILES_MANAGER_INTEGRATION": "1"}, clear=True):
            manifest = Path(directory) / "manifest.json"
            manifest.write_text(json.dumps(self.record()))
            fixture.prepare_files_input(str(Path(directory) / "disk.raw"), str(manifest))
            self.assertEqual(os.environ["HOMENODE_FILES_IMAGE_SHA256"], "d" * 64)
            self.assertEqual(os.environ["HOMENODE_FILES_IMAGE_BYTES"], "1024")

    def test_malformed_inputs_never_publish_identity(self):
        cases = [{"schema": True}, {"profile": "video"}, {"releaseQualified": True},
                 {"bootValidated": True}, {"bytes": True}, {"bytes": 1.5},
                 {"bytes": 0}, {"bytes": (8 << 30) + 1}, {"sha256": "D" * 64},
                 {"sha256": 2}, {"foreign": "field"}]
        with tempfile.TemporaryDirectory() as directory:
            manifest = Path(directory) / "manifest.json"
            for change in cases:
                with self.subTest(change=change), mock.patch.dict(os.environ, {"HOMENODE_FILES_MANAGER_INTEGRATION": "1"}, clear=True):
                    manifest.write_text(json.dumps(self.record() | change))
                    with self.assertRaises((ValueError, RuntimeError)):
                        fixture.prepare_files_input(str(Path(directory) / "disk.raw"), str(manifest))
                    self.assertNotIn("HOMENODE_FILES_IMAGE", os.environ)
            for text in ('{"schema":1,"schema":1}', "x" * 16385):
                with mock.patch.dict(os.environ, {"HOMENODE_FILES_MANAGER_INTEGRATION": "1"}, clear=True):
                    manifest.write_text(text)
                    with self.assertRaises((ValueError, RuntimeError)):
                        fixture.prepare_files_input(str(Path(directory) / "disk.raw"), str(manifest))
                    self.assertNotIn("HOMENODE_FILES_IMAGE", os.environ)

    def test_client_requires_ready_state_before_object_work(self):
        result = {"bytes": 1048579, "chunkBytes": 262144, "acknowledgedChunkReplay": True}
        with mock.patch.object(manager_channel.sys, "platform", "linux"), mock.patch.object(os, "geteuid", return_value=2), mock.patch.dict(os.environ, {"HOMENODE_FILES_MANAGER_INTEGRATION": "1"}), mock.patch.object(manager_channel.sys, "argv", ["fixture", "/tmp/admitted.sock", "Generated_Token_12345678901", "0"]), mock.patch.object(manager_channel.socket, "socket"), mock.patch.object(manager_channel.boot_image, "request", return_value={"state": "foreign"}) as request, mock.patch.object(manager_channel.boot_image, "object_roundtrip", return_value=result) as transfer:
            with self.assertRaises(ValueError):
                manager_channel.main()
            transfer.assert_not_called()
            request.assert_called_once()
            self.assertEqual(request.call_args.args[1], "health")

    def test_restart_object_is_retained_then_verified_before_deletion(self):
        content = b"HomeNode development restart persistence\n" * 128
        digest = hashlib.sha256(content).hexdigest()
        identifier = "a" * 32
        with mock.patch.object(manager_channel.boot_image, "request", side_effect=[{"offset": len(content)}, {"size": len(content), "sha256": digest}]) as request:
            manager_channel.persistent_object(object(), identifier, "0")
            self.assertEqual([call.args[1] for call in request.call_args_list], ["upload", "finalize"])
            self.assertEqual(request.call_args_list[0].kwargs["objectId"], identifier)
        with mock.patch.object(manager_channel.boot_image, "request", side_effect=[{"offset": len(content), "sha256": digest, "data": base64.b64encode(content).decode()}, {}]) as request:
            manager_channel.persistent_object(object(), identifier, "1")
            self.assertEqual([call.args[1] for call in request.call_args_list], ["download", "delete"])

    def test_restart_corruption_never_acknowledges_deletion(self):
        content = b"HomeNode development restart persistence\n" * 128
        record = {"offset": len(content), "sha256": hashlib.sha256(content).hexdigest(), "data": base64.b64encode(content).decode()}
        for change in ({"offset": 0}, {"sha256": "a" * 64}, {"data": base64.b64encode(b"corrupt").decode()}, {"data": "!"}):
            with self.subTest(change=change), mock.patch.object(manager_channel.boot_image, "request", return_value=record | change) as request:
                with self.assertRaises(ValueError):
                    manager_channel.persistent_object(object(), "a" * 32, "1")
                self.assertEqual([call.args[1] for call in request.call_args_list], ["download"])

    def test_large_transfer_closes_old_channel_and_refuses_changed_peer(self):
        old, new = mock.Mock(), mock.Mock()
        old.getsockopt.return_value = b"originalpeer"
        new.getsockopt.return_value = b"foreign-peer"
        def reconnect():
            old.close.assert_called_once()
            return new
        def response(_channel, operation, **fields):
            self.assertEqual(operation, "upload")
            return {"offset": fields["offset"] + 262144}
        with mock.patch.object(manager_channel.socket, "SO_PEERCRED", 17, create=True), mock.patch.object(manager_channel.boot_image, "request", side_effect=response) as request:
            with self.assertRaisesRegex(ValueError, "peer changed"):
                manager_channel.large_object_roundtrip(old, reconnect)
            self.assertEqual(request.call_count, 2)

    def test_unacknowledged_disconnect_refuses_foreign_peer_before_replay(self):
        first, second, foreign = mock.Mock(), mock.Mock(), mock.Mock()
        first.getsockopt.return_value = second.getsockopt.return_value = b"originalpeer"
        foreign.getsockopt.return_value = b"foreign-peer"
        connections = iter((second, foreign))
        def reconnect():
            if second.close.call_count:
                second.close.assert_called_once()
            else:
                first.close.assert_called_once()
            return next(connections)
        def response(_channel, operation, **fields):
            self.assertEqual(operation, "upload")
            return {"offset": fields["offset"] + 262144}
        with mock.patch.object(manager_channel.socket, "SO_PEERCRED", 17, create=True), mock.patch.object(manager_channel.boot_image, "request", side_effect=response) as request:
            with self.assertRaisesRegex(ValueError, "unacknowledged reconnect"):
                manager_channel.large_object_roundtrip(first, reconnect)
        frame = second.sendall.call_args.args[0]
        self.assertEqual(struct.unpack(">I", frame[:4])[0], len(frame) - 4)
        message = json.loads(frame[4:])
        self.assertEqual(message["operation"], "upload")
        self.assertEqual(message["offset"], 524288)
        self.assertEqual(len(base64.b64decode(message["data"], validate=True)), 262144)
        second.recv.assert_not_called()
        foreign.sendall.assert_not_called()
        self.assertEqual(request.call_count, 3)

    def test_capacity_refusal_cleans_only_probe_object(self):
        chunk = bytes(range(256)) * 1024
        with mock.patch.object(manager_channel.boot_image, "request", side_effect=[
                {"offset": 262144}, {"error": "CAPACITY_UNAVAILABLE"}, {}]) as request:
            manager_channel.capacity_roundtrip(object(), chunk)
        calls = request.call_args_list
        self.assertEqual([call.args[1] for call in calls], ["upload", "upload", "delete"])
        self.assertEqual(calls[1].kwargs["offset"], 262144)
        self.assertEqual(len({call.kwargs["objectId"] for call in calls}), 1)
        self.assertEqual(calls[0].kwargs["allowed_error"], "CAPACITY_UNAVAILABLE")

    def test_capacity_probe_refuses_invalid_acknowledgment(self):
        with mock.patch.object(manager_channel.boot_image, "request", return_value={"offset": 0}) as request:
            with self.assertRaisesRegex(ValueError, "response mismatch"):
                manager_channel.capacity_roundtrip(object(), b"chunk")
        request.assert_called_once()

    def test_uncertain_upload_retries_identical_bytes_after_timeout(self):
        first, second = mock.Mock(), mock.Mock()
        second.getsockopt.return_value = b"originalpeer"
        fields = {"objectId": "a" * 32, "offset": 524288, "size": 1 << 30,
                  "sha256": "b" * 64, "data": "fixture"}
        def reconnect():
            first.close.assert_called_once()
            return second
        with mock.patch.object(manager_channel.socket, "SO_PEERCRED", 17, create=True), mock.patch.object(manager_channel.boot_image, "request", side_effect=[TimeoutError("lost response"), {"offset": 786432}]) as request:
            channel, response = manager_channel.replay_uncertain_upload(first, reconnect, b"originalpeer", fields)
        self.assertIs(channel, second)
        self.assertEqual(response["offset"], 786432)
        self.assertEqual([call.kwargs for call in request.call_args_list], [fields, fields])

    def test_uncertain_upload_never_retries_guest_refusal(self):
        reconnect = mock.Mock()
        with mock.patch.object(manager_channel.boot_image, "request", side_effect=ValueError("guest request failed: CAPACITY_UNAVAILABLE")) as request:
            with self.assertRaisesRegex(ValueError, "CAPACITY_UNAVAILABLE"):
                manager_channel.replay_uncertain_upload(mock.Mock(), reconnect, b"originalpeer", {})
        reconnect.assert_not_called()
        request.assert_called_once()

    def test_uncertain_upload_refuses_changed_peer_before_replaying(self):
        reconnect = mock.Mock()
        reconnect.return_value.getsockopt.return_value = b"foreign-peer"
        with mock.patch.object(manager_channel.socket, "SO_PEERCRED", 17, create=True), mock.patch.object(manager_channel.boot_image, "request", side_effect=TimeoutError("lost response")) as request:
            with self.assertRaisesRegex(ValueError, "peer changed"):
                manager_channel.replay_uncertain_upload(mock.Mock(), reconnect, b"originalpeer", {})
        request.assert_called_once()

    def test_uncertain_upload_stops_after_three_transport_failures(self):
        reconnect = mock.Mock()
        reconnect.return_value.getsockopt.return_value = b"originalpeer"
        with mock.patch.object(manager_channel.socket, "SO_PEERCRED", 17, create=True), mock.patch.object(manager_channel.boot_image, "request", side_effect=TimeoutError("lost response")) as request:
            with self.assertRaises(TimeoutError):
                manager_channel.replay_uncertain_upload(mock.Mock(), reconnect, b"originalpeer", {})
        self.assertEqual(request.call_count, 3)
        self.assertEqual(reconnect.call_count, 2)


if __name__ == "__main__":
    unittest.main()
