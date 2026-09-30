import hashlib
import io
import json
from pathlib import Path
import sys
import tarfile
import tempfile
import unittest
sys.dont_write_bytecode = True
import overlay
import import_overlay


class ImportTests(unittest.TestCase):
    def test_invalid_headers_before_staging(self):
        for name, kind in [("../escape", tarfile.REGTYPE), ("/absolute", tarfile.REGTYPE),
                           (overlay.BINARY, tarfile.SYMTYPE), (overlay.BINARY, tarfile.FIFOTYPE)]:
            with self.subTest(name=name, kind=kind), tempfile.TemporaryDirectory() as directory:
                base = Path(directory)
                archive = base / "bad.tar"
                with tarfile.open(archive, "w", format=tarfile.USTAR_FORMAT) as output:
                    member = tarfile.TarInfo(name)
                    member.type, member.uid, member.gid, member.mode = kind, 0, 0, 0o755
                    output.addfile(member)
                destination = base / "output"
                digest = hashlib.sha256(archive.read_bytes()).hexdigest()
                with self.assertRaises(ValueError):
                    import_overlay.import_overlay(archive, destination, digest, "files", "a" * 40)
                self.assertFalse(destination.exists())

    def test_duplicate_headers_and_archive_bound(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            archive = base / "duplicate.tar"
            with tarfile.open(archive, "w", format=tarfile.USTAR_FORMAT) as output:
                for _ in range(2):
                    member = tarfile.TarInfo("usr")
                    member.type, member.mode = tarfile.DIRTYPE, 0o755
                    output.addfile(member)
            digest = hashlib.sha256(archive.read_bytes()).hexdigest()
            with self.assertRaises(ValueError):
                import_overlay.import_overlay(archive, base / "output", digest, "files", "a" * 40)
            self.assertFalse((base / "output").exists())
            with archive.open("wb") as output:
                output.truncate(import_overlay.MAX_ARCHIVE + 1)
            with self.assertRaises(ValueError):
                import_overlay.import_overlay(archive, base / "output", "0" * 64, "files", "a" * 40)
            self.assertFalse((base / "output").exists())

    def test_valid_round_trip_and_no_overwrite(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            root = base / "source"
            root.mkdir()
            for name, mode in {**overlay.COMMON, "etc/homenode/guest/files.env": 0o644}.items():
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                data = b"fixture"
                if name in (overlay.BINARY, "usr/lib/homenode/guest/homenode-guest-init"):
                    data = b"\x7fELF\x02\x01" + bytes(12) + b"\x3e\x00"
                if name.endswith("files.env"):
                    data = b"QUOTA_BYTES=17179869184\n"
                path.write_bytes(data)
                path.chmod(mode)
            record = {"schema": 1, "profile": "files", "sourceRevision": "a" * 40, "goToolchain": "go1.26.8",
                      "guestUID": 900, "guestGID": 900, "bootable": False, "files": overlay.inventory(root, "files")}
            (root / "overlay.json").write_text(json.dumps(record))
            (root / "overlay.json").chmod(0o644)
            archive = base / "source.tar"
            overlay.package(root, archive, 1800000000)
            digest = hashlib.sha256(archive.read_bytes()).hexdigest()
            destination = base / "destination"
            with self.assertRaises(ValueError):
                import_overlay.import_overlay(archive, destination, "0" * 64, "files", "a" * 40)
            self.assertFalse(destination.exists())
            failed = base / "wrong-source"
            with self.assertRaises(ValueError):
                import_overlay.import_overlay(archive, failed, digest, "files", "b" * 40)
            self.assertEqual(failed.stat().st_mode & 0o777, 0o700)
            import_overlay.import_overlay(archive, destination, digest, "files", "a" * 40)
            self.assertEqual(overlay.verify(destination), record)
            with self.assertRaises(FileExistsError):
                import_overlay.import_overlay(archive, destination, digest, "files", "a" * 40)


if __name__ == "__main__":
    unittest.main()
