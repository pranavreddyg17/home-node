import json
import hashlib
import os
from pathlib import Path
import shutil
import sys
import tempfile
import unittest
from unittest.mock import patch
sys.dont_write_bytecode = True
import finalize_root
import overlay
import stage_ai_payload
from identity_test import PASSWD, GROUP, SHADOW, NSS


def fixture(base, profile):
    source = base / "overlay"
    source.mkdir(mode=0o755)
    for name in overlay.MOUNTPOINTS:
        (source / name).mkdir(mode=0o755)
    for name, mode in {**overlay.COMMON, f"etc/homenode/guest/{profile}.env": 0o644}.items():
        path = source / name
        path.parent.mkdir(parents=True, exist_ok=True)
        content = b"fixture"
        if name in (overlay.BINARY, "usr/lib/homenode/guest/homenode-guest-init"):
            content = b"\x7fELF\x02\x01" + bytes(12) + b"\x3e\x00"
        if name.endswith(".env"):
            content = f"QUOTA_BYTES={overlay.PROFILES[profile]}\n".encode()
        path.write_bytes(content)
        path.chmod(mode)
    record = {"schema": 1, "profile": profile, "sourceRevision": "a" * 40,
              "goToolchain": "go1.26.8", "guestUID": 900, "guestGID": 900,
              "bootable": False, "files": overlay.inventory(source, profile)}
    (source / "overlay.json").write_text(json.dumps(record))
    (source / "overlay.json").chmod(0o644)
    root = base / "root"
    shutil.copytree(source, root)
    for name, content in [("passwd", PASSWD), ("group", GROUP), ("shadow", SHADOW), ("nsswitch.conf", NSS)]:
        path = root / "etc" / name
        path.write_text(content)
        path.chmod(0o600 if name == "shadow" else 0o644)
    return root, source


class FinalizeTests(unittest.TestCase):
    def test_ai_adapter_alone_is_not_enabled(self):
        with tempfile.TemporaryDirectory() as directory:
            root, source = fixture(Path(directory), "ai")
            with self.assertRaisesRegex(ValueError, "AI model payload is required"):
                finalize_root.finalize(root, source)
            self.assertFalse((root / "etc/systemd").exists())

    def test_merged_ai_payload_enablement_replay_and_tamper(self):
        content = b"test-only model and runtime bytes"
        digest = hashlib.sha256(content).hexdigest()
        with tempfile.TemporaryDirectory() as directory, \
                patch.object(stage_ai_payload, "BINARY_SIZE", len(content)), patch.object(stage_ai_payload, "BINARY_SHA256", digest), \
                patch.object(stage_ai_payload.model, "SIZE", len(content)), patch.object(stage_ai_payload.model, "SHA256", digest):
            root, source = fixture(Path(directory), "ai")
            ai = Path(directory) / "ai-payload"
            ai.mkdir(mode=0o755)
            files = []
            for name, incoming, size, checksum, mode in stage_ai_payload.inputs(None, None):
                path = ai / name
                path.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
                path.write_bytes(incoming.read_bytes() if incoming is not None else content)
                path.chmod(mode)
                files.append({"path": name, "bytes": size, "sha256": checksum, "mode": mode})
            record = {"schema": 1, "runtimeRevision": stage_ai_payload.runtime.REVISION,
                      "modelRevision": stage_ai_payload.model.REVISION, "files": files,
                      "developmentOnly": True, "releaseQualified": False}
            (ai / "ai-payload.json").write_text(json.dumps(record))
            (ai / "ai-payload.json").chmod(0o644)
            shutil.copytree(ai, root, dirs_exist_ok=True)
            for _ in range(2):
                finalize_root.finalize(root, source, ai)
            wants = root / "etc/systemd/system/multi-user.target.wants"
            self.assertEqual(os.readlink(wants / "systemd-logind.service"), "/usr/lib/systemd/system/systemd-logind.service")
            self.assertEqual({p.name for p in wants.iterdir()}, {"homenode-model.service", "homenode-guest@ai.service", "systemd-logind.service"})
            self.assertEqual(os.readlink(wants / "homenode-model.service"), "/usr/lib/systemd/system/homenode-model.service")
            weights = root / "usr/lib/homenode/ai/model.gguf"
            weights.write_bytes(b"x" * len(content))
            with self.assertRaisesRegex(ValueError, "merged payload digest mismatch"):
                finalize_root.finalize(root, source, ai)
            self.assertEqual(weights.read_bytes(), b"x" * len(content))

    def test_other_profile_refuses_model_inputs(self):
        with tempfile.TemporaryDirectory() as directory:
            root, source = fixture(Path(directory), "files")
            with self.assertRaisesRegex(ValueError, "another profile"):
                finalize_root.finalize(root, source, Path(directory) / "unexpected")
            self.assertFalse((root / "etc/systemd").exists())

    def test_profile_enablement_and_replay(self):
        for profile in ("files", "video"):
            with self.subTest(profile=profile), tempfile.TemporaryDirectory() as directory:
                root, source = fixture(Path(directory), profile)
                finalize_root.finalize(root, source)
                (root / "tmp").chmod(0o1777)  # distribution tmpfiles may prepare it
                finalize_root.finalize(root, source)
                wants = root / "etc/systemd/system/multi-user.target.wants"
                self.assertEqual({p.name for p in wants.iterdir()}, {f"homenode-guest@{profile}.service", "systemd-logind.service"})
                self.assertEqual(os.readlink(wants / "systemd-logind.service"), "/usr/lib/systemd/system/systemd-logind.service")
                self.assertEqual(os.readlink(wants / f"homenode-guest@{profile}.service"),
                                 "/usr/lib/systemd/system/homenode-guest@.service")
                for name in ("tmp", "var"):
                    self.assertEqual(os.readlink(root / f"etc/systemd/system/local-fs.target.wants/{name}.mount"),
                                     f"/usr/lib/systemd/system/{name}.mount")

    def test_admission_preserves_conflicting_inputs(self):
        for variant in ("payload", "identity", "foreign", "other-profile", "parent-link"):
            with self.subTest(variant=variant), tempfile.TemporaryDirectory() as directory:
                root, source = fixture(Path(directory), "files")
                if variant == "payload":
                    (root / overlay.BINARY).write_bytes(b"changed")
                elif variant == "identity":
                    (root / "etc/passwd").write_text(PASSWD + "alias:x:900:0:alias:/:/bin/sh\n")
                elif variant in ("foreign", "other-profile"):
                    wants = root / "etc/systemd/system/multi-user.target.wants"
                    wants.mkdir(parents=True)
                    leaf = "files" if variant == "foreign" else "ai"
                    (wants / f"homenode-guest@{leaf}.service").symlink_to("/foreign/keep")
                else:
                    (root / "etc/systemd").symlink_to(source / "usr/lib/systemd", target_is_directory=True)
                original = (root / overlay.BINARY).read_bytes()
                with self.assertRaises((ValueError, OSError)):
                    finalize_root.finalize(root, source)
                self.assertEqual((root / overlay.BINARY).read_bytes(), original)
                if variant in ("foreign", "other-profile"):
                    self.assertEqual(os.readlink(wants / f"homenode-guest@{leaf}.service"), "/foreign/keep")
                self.assertFalse(os.path.lexists(root / "etc/systemd/system/local-fs.target.wants/tmp.mount"))


if __name__ == "__main__":
    unittest.main()
