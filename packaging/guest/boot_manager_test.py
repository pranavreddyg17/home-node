"""Development manager input/client checks without native host effects."""
import importlib.util
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
        with mock.patch.object(manager_channel.sys, "platform", "linux"), mock.patch.object(os, "geteuid", return_value=2), mock.patch.dict(os.environ, {"HOMENODE_FILES_MANAGER_INTEGRATION": "1"}), mock.patch.object(manager_channel.sys, "argv", ["fixture", "/tmp/admitted.sock"]), mock.patch.object(manager_channel.socket, "socket"), mock.patch.object(manager_channel.boot_image, "request", return_value={"state": "foreign"}), mock.patch.object(manager_channel.boot_image, "object_roundtrip", return_value=result) as transfer:
            with self.assertRaises(ValueError):
                manager_channel.main()
            transfer.assert_not_called()


if __name__ == "__main__":
    unittest.main()
