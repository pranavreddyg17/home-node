import hashlib
import os
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import stage_ai_payload as payload


class PayloadCopyTests(unittest.TestCase):
    def test_inventory_receipt_and_payload_tampering_refused(self):
        content = b"tiny test-only payload"
        digest = hashlib.sha256(content).hexdigest()
        for fault in (None, "extra", "digest", "mode", "directory", "receipt", "boolean", "float", "symlink"):
            with self.subTest(fault=fault), tempfile.TemporaryDirectory() as directory, \
                    patch.object(payload, "BINARY_SIZE", len(content)), patch.object(payload, "BINARY_SHA256", digest), \
                    patch.object(payload.model, "SIZE", len(content)), patch.object(payload.model, "SHA256", digest):
                root = Path(directory)
                records = []
                for name, incoming, size, checksum, mode in payload.inputs(None, None):
                    path = root / name
                    path.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
                    path.write_bytes(incoming.read_bytes() if incoming is not None else content)
                    path.chmod(mode)
                    records.append({"path": name, "bytes": size, "sha256": checksum, "mode": mode})
                record = {"schema": 1, "runtimeRevision": payload.runtime.REVISION, "modelRevision": payload.model.REVISION,
                          "files": records, "developmentOnly": True, "releaseQualified": False}
                receipt = root / "ai-payload.json"
                receipt.write_text(json.dumps(record))
                receipt.chmod(0o644)
                binary = root / records[0]["path"]
                if fault == "extra": (root / "unexpected").write_text("foreign")
                if fault == "digest": binary.write_bytes(b"x" * len(content))
                if fault == "mode": binary.chmod(0o777)
                if fault == "directory": binary.parent.chmod(0o777)
                if fault == "receipt":
                    record["files"] = []
                    receipt.write_text(json.dumps(record))
                if fault == "boolean":
                    record["releaseQualified"] = 0
                    receipt.write_text(json.dumps(record))
                if fault == "float":
                    record["files"][0]["mode"] = float(record["files"][0]["mode"])
                    receipt.write_text(json.dumps(record))
                if fault == "symlink":
                    binary.unlink()
                    binary.symlink_to(receipt)
                if fault is None:
                    self.assertEqual(payload.verify(root), record)
                else:
                    with self.assertRaises(ValueError): payload.verify(root)

    def test_pinned_copy_and_no_overwrite(self):
        with tempfile.TemporaryDirectory() as directory:
            source, target = Path(directory) / "input", Path(directory) / "output"
            content = b"pinned fixture bytes" * 100000
            source.write_bytes(content)
            digest = hashlib.sha256(content).hexdigest()
            payload.copy_verified(source, target, len(content), digest, 0o644)
            self.assertEqual(target.read_bytes(), content)
            self.assertEqual(target.stat().st_mode & 0o777, 0o644)
            with self.assertRaises(FileExistsError):
                payload.copy_verified(source, target, len(content), digest, 0o644)

    def test_invalid_inputs_refused(self):
        for fault in ("digest", "size", "symlink", "hardlink", "writable"):
            with self.subTest(fault=fault), tempfile.TemporaryDirectory() as directory:
                source, target = Path(directory) / "input", Path(directory) / "output"
                content = b"fixture"
                source.write_bytes(content)
                if fault == "symlink":
                    source.unlink()
                    source.symlink_to("/dev/null")
                if fault == "hardlink":
                    os.link(source, Path(directory) / "alias")
                if fault == "writable":
                    source.chmod(0o666)
                with self.assertRaises((OSError, ValueError)):
                    payload.copy_verified(source, target, len(content) + (1 if fault == "size" else 0),
                                          "0" * 64 if fault == "digest" else hashlib.sha256(content).hexdigest(), 0o644)


if __name__ == "__main__":
    unittest.main()
