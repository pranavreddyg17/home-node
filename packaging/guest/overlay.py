#!/usr/bin/env python3
"""Verify fixed adapter overlay bytes, without claiming boot/release acceptance."""
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import sys

PROFILES = {"files": 16 << 30, "video": 8 << 30, "ai": 16 << 30}
BINARY = "usr/lib/homenode/guest/homenode-guest"
COMMON = {BINARY: 0o755, "usr/lib/systemd/system/homenode-guest@.service": 0o644,
          "usr/lib/udev/rules.d/60-homenode-adapter.rules": 0o644,
          "usr/lib/sysusers.d/homenode-guest.conf": 0o644}


def inventory(root, profile):
    expected = {**COMMON, f"etc/homenode/guest/{profile}.env": 0o644}
    observed = set()
    expected_dirs = set()
    for name in expected:
        parent = PurePosixPath(name).parent
        while str(parent) != ".":
            expected_dirs.add(str(parent))
            parent = parent.parent
    observed_dirs = set()
    for base, dirs, files in os.walk(root, followlinks=False):
        for name in dirs + files:
            path = Path(base) / name
            info = path.lstat()
            if stat.S_ISLNK(info.st_mode) or not (stat.S_ISDIR(info.st_mode) or stat.S_ISREG(info.st_mode)):
                raise ValueError("unsafe entry")
            if stat.S_ISDIR(info.st_mode):
                if stat.S_IMODE(info.st_mode) != 0o755:
                    raise ValueError("unsafe directory permission")
                observed_dirs.add(path.relative_to(root).as_posix())
            if path.is_file() and path != root / "overlay.json":
                observed.add(path.relative_to(root).as_posix())
    if observed != set(expected) or observed_dirs != expected_dirs:
        raise ValueError("unexpected inventory")
    records = []
    for name, mode in sorted(expected.items()):
        path = root / name
        info = path.lstat()
        if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != mode:
            raise ValueError("invalid permission")
        digest = hashlib.sha256()
        with path.open("rb") as file:
            while chunk := file.read(1 << 20):
                digest.update(chunk)
        records.append({"path": name, "mode": mode, "bytes": info.st_size, "sha256": digest.hexdigest()})
    with (root / BINARY).open("rb") as file:
        header = file.read(20)
    if len(header) != 20 or header[:6] != b"\x7fELF\x02\x01" or header[18:20] != b"\x3e\x00":
        raise ValueError("expected ELF x86-64")
    if (root / f"etc/homenode/guest/{profile}.env").read_text() != f"QUOTA_BYTES={PROFILES[profile]}\n":
        raise ValueError("quota mismatch")
    return records


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate field")
        result[key] = value
    return result


def verify(root):
    path = root / "overlay.json"
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o644 or info.st_size > 16384:
        raise ValueError("invalid metadata")
    record = json.loads(path.read_bytes(), object_pairs_hook=unique_object)
    if set(record) != {"schema", "profile", "sourceRevision", "goToolchain", "guestUID", "guestGID", "bootable", "files"}:
        raise ValueError("invalid fields")
    if type(record["schema"]) is not int or record["schema"] != 1 or record["profile"] not in PROFILES or record["bootable"] is not False:
        raise ValueError("invalid identity")
    if not re.fullmatch(r"[a-f0-9]{40}", record["sourceRevision"]) or not re.fullmatch(r"go[0-9]+\.[0-9]+\.[0-9]+", record["goToolchain"]):
        raise ValueError("invalid source identity")
    if record["guestUID"] != 900 or record["guestGID"] != 900 or record["files"] != inventory(root, record["profile"]):
        raise ValueError("integrity mismatch")


def main():
    action, root = sys.argv[1], Path(sys.argv[2])
    if root.is_symlink() or not root.is_dir():
        raise ValueError("invalid root")
    if action == "create":
        profile, revision, toolchain = sys.argv[3:]
        if profile not in PROFILES:
            raise ValueError("invalid profile")
        record = {"schema": 1, "profile": profile, "sourceRevision": revision, "goToolchain": toolchain,
                  "guestUID": 900, "guestGID": 900, "bootable": False, "files": inventory(root, profile)}
        with (root / "overlay.json").open("x") as file:
            json.dump(record, file, indent=2)
            file.write("\n")
        (root / "overlay.json").chmod(0o644)
    elif action != "verify":
        raise ValueError("invalid action")
    verify(root)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, KeyError, IndexError, TypeError):
        sys.exit("Guest overlay verification failed")
