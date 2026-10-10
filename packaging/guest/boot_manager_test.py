"""Development manager input/client checks without native host effects."""
import importlib.util
import base64
import hashlib
import json
import os
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


if __name__ == "__main__":
    unittest.main()
