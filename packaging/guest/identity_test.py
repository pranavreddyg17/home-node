import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
sys.dont_write_bytecode = True
import identity

PASSWD = "root:x:0:0:root:/root:/bin/sh\nhomenode-guest:x:900:900:HomeNode guest adapter:/nonexistent:/usr/sbin/nologin\n"
GROUP = "root:x:0:\nhomenode-guest:x:900:\n"
SHADOW = "root:!*:::::::\nhomenode-guest:!*:::::::\n"
NSS = "passwd: files systemd\ngroup: files systemd\nshadow: files\n"


class IdentityTests(unittest.TestCase):
    def test_profile_identity(self):
        identity.validate(PASSWD, GROUP, SHADOW, NSS)
        for passwd, group, shadow, nss in [
            (PASSWD.replace("900:900", "899:900"), GROUP, SHADOW, NSS),
            (PASSWD + "alias:x:900:0:alias:/:/bin/sh\n", GROUP, SHADOW, NSS),
            (PASSWD + "other:x:901:900:other:/:/bin/sh\n", GROUP, SHADOW, NSS),
            (PASSWD, GROUP + "sudo:x:27:homenode-guest\n", SHADOW, NSS),
            (PASSWD, GROUP + "alias:x:900:\n", SHADOW, NSS),
            (PASSWD, GROUP, SHADOW.replace("homenode-guest:!*", "homenode-guest:hash"), NSS),
            (PASSWD, GROUP, SHADOW, NSS.replace("files systemd", "files ldap")),
        ]:
            with self.assertRaises(ValueError):
                identity.validate(passwd, group, shadow, nss)

    def test_protected_identity_inputs(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "etc").mkdir()
            path = root / "etc/shadow"
            path.write_text(SHADOW)
            path.chmod(0o600)
            self.assertEqual(identity.read_file(root, "etc/shadow"), SHADOW)
            path.chmod(0o644)
            with self.assertRaises(ValueError):
                identity.read_file(root, "etc/shadow")
            path.unlink()
            path.symlink_to("/etc/shadow")
            with self.assertRaises(OSError):
                identity.read_file(root, "etc/shadow")

    def test_replaceable_directories_and_unexpected_paths(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            etc = root / "etc"
            etc.mkdir()
            shadow = etc / "shadow"
            shadow.write_text(SHADOW)
            shadow.chmod(0o600)
            for target in (root, etc):
                for mode in (0o777, 0o775):
                    target.chmod(mode)
                    with self.assertRaises(ValueError):
                        identity.read_file(root, "etc/shadow")
                target.chmod(0o755)
            self.assertEqual(identity.read_file(root, "etc/shadow"), SHADOW)
            for name in ("../etc/shadow", "/etc/shadow", "etc/../etc/shadow", "etc/gshadow"):
                with self.assertRaises(ValueError):
                    identity.read_file(root, name)
            shadow.unlink()
            etc.rmdir()
            etc.symlink_to("/etc", target_is_directory=True)
            with self.assertRaises(OSError):
                identity.read_file(root, "etc/shadow")

    @unittest.skipUnless(os.getenv("HOMENODE_GUEST_ACCOUNT_INTEGRATION") == "1" and os.geteuid() == 0 and sys.platform == "linux", "disposable Linux root fixture only")
    def test_native_sysusers_exact_identity_and_collision(self):
        for collision in (False, True):
            with tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                (root / "etc").mkdir()
                config = root / "usr/lib/sysusers.d/homenode-guest.conf"
                config.parent.mkdir(parents=True)
                shutil.copyfile(Path(__file__).with_name("homenode-guest.conf"), config)
                passwd = "root:x:0:0:root:/root:/bin/sh\n"
                groups = "root:x:0:\n"
                if collision:
                    passwd += "foreign:x:900:900:foreign:/nonexistent:/usr/sbin/nologin\n"
                    groups += "foreign:x:900:\n"
                for name, content in {"passwd": passwd, "group": groups, "shadow": "root:!*:::::::\n", "nsswitch.conf": NSS}.items():
                    (root / "etc" / name).write_text(content)
                    (root / "etc" / name).chmod(0o600 if name == "shadow" else 0o644)
                result = subprocess.run(["/usr/bin/systemd-sysusers", "--root", str(root)], env={"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LANG": "C", "LC_ALL": "C"}, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10)
                if collision:
                    with self.assertRaises(ValueError):
                        identity.check(root)
                else:
                    self.assertEqual(result.returncode, 0)
                    identity.check(root)


if __name__ == "__main__":
    unittest.main()
