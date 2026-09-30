"""Opt-in tmpfiles integration targets only a newly created fixture root."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


@unittest.skipUnless(sys.platform == "linux" and os.geteuid() == 0 and
                     os.getenv("HOMENODE_GUEST_VOLATILE_INTEGRATION") == "1",
                     "requires explicit disposable Linux root fixture")
class VolatileTests(unittest.TestCase):
    def test_native_ephemeral_directory_creation_and_replay(self):
        with tempfile.TemporaryDirectory(prefix="homenode-tmpfiles-") as directory:
            root = Path(directory)
            config = root / "usr/lib/tmpfiles.d/homenode-volatile.conf"
            config.parent.mkdir(parents=True)
            config.write_bytes(Path(__file__).with_name("homenode-volatile.conf").read_bytes())
            (root / "var").mkdir(mode=0o755)
            data = root / "data"
            data.mkdir(mode=0o755)
            marker = data / "preserve"
            marker.write_bytes(b"workload fixture")
            command = ["/usr/bin/systemd-tmpfiles", "--root=" + str(root),
                       "--create", "homenode-volatile.conf"]
            subprocess.run(command, check=True, timeout=15)
            for name, mode in [("tmp", 0o1777), ("log", 0o755), ("lib", 0o755)]:
                info = (root / "var" / name).stat()
                self.assertEqual((info.st_uid, info.st_gid, info.st_mode & 0o7777),
                                 (0, 0, mode))
            temporary = root / "var/tmp/retain"
            temporary.write_bytes(b"temporary fixture")
            subprocess.run(command, check=True, timeout=15)
            self.assertEqual(temporary.read_bytes(), b"temporary fixture")
            self.assertEqual(marker.read_bytes(), b"workload fixture")
            self.assertFalse((root / "var/log/journal").exists())


if __name__ == "__main__":
    unittest.main()
