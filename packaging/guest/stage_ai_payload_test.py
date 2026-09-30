import hashlib
import os
from pathlib import Path
import tempfile
import unittest
import stage_ai_payload as payload


class PayloadCopyTests(unittest.TestCase):
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
