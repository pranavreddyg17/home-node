import importlib.util
import json
import hashlib
import tarfile
from pathlib import Path
import sys
import tempfile
import unittest
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("overlay", Path(__file__).with_name("overlay.py"))
overlay = importlib.util.module_from_spec(spec)
spec.loader.exec_module(overlay)


class OverlayTests(unittest.TestCase):
    def test_inventory_and_tamper(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name, mode in {**overlay.COMMON, "etc/homenode/guest/files.env": 0o644}.items():
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                content = b"fixture"
                if name in (overlay.BINARY, "usr/lib/homenode/guest/homenode-guest-init"):
                    content = b"\x7fELF\x02\x01" + bytes(12) + b"\x3e\x00"
                if name.endswith("files.env"):
                    content = b"QUOTA_BYTES=17179869184\n"
                path.write_bytes(content)
                path.chmod(mode)
            record = {"schema": 1, "profile": "files", "sourceRevision": "a" * 40, "goToolchain": "go1.26.8",
                      "guestUID": 900, "guestGID": 900, "bootable": False, "files": overlay.inventory(root, "files")}
            metadata = root / "overlay.json"
            metadata.write_text(json.dumps(record))
            metadata.chmod(0o644)
            overlay.verify(root)
            with tempfile.TemporaryDirectory() as archives:
                first, second = Path(archives) / "first.tar", Path(archives) / "second.tar"
                overlay.package(root, first, 1800000000)
                overlay.package(root, second, 1800000000)
                self.assertEqual(hashlib.sha256(first.read_bytes()).digest(), hashlib.sha256(second.read_bytes()).digest())
                self.assertEqual(first.stat().st_mode & 0o777, 0o444)
                with tarfile.open(first) as archive:
                    for member in archive.getmembers():
                        self.assertEqual((member.uid, member.gid, member.mtime), (0, 0, 1800000000))
                        self.assertFalse(member.issym() or member.islnk())
                        self.assertFalse(member.name.startswith("/") or ".." in member.name.split("/"))
                with self.assertRaises(FileExistsError):
                    overlay.package(root, first, 1800000000)
            binary = root / overlay.BINARY
            original = binary.read_bytes()
            binary.write_bytes(original + b"changed")
            with self.assertRaises(ValueError):
                overlay.verify(root)
            binary.write_bytes(original)
            binary.chmod(0o777)
            with self.assertRaises(ValueError):
                overlay.verify(root)
            binary.chmod(0o755)
            extra = root / "foreign"
            extra.write_text("unexpected")
            with self.assertRaises(ValueError):
                overlay.verify(root)
            extra.unlink()
            extra.symlink_to(binary)
            with self.assertRaises(ValueError):
                overlay.verify(root)

    def test_duplicate_metadata(self):
        with self.assertRaises(ValueError):
            json.loads('{"schema":1,"schema":1}', object_pairs_hook=overlay.unique_object)


if __name__ == "__main__":
    unittest.main()
