#!/usr/bin/env python3
"""Admit merged adapter inputs and enable a profile in protected assembly staging."""
import hashlib
import os
from pathlib import Path
import stat
import sys
sys.dont_write_bytecode = True
import identity
import overlay


def directory(path):
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) & 0o022:
        raise ValueError("unprotected assembly directory")


def parents(root, relative, create=False):
    current = root
    directory(current)
    for component in Path(relative).parts[:-1]:
        current = current / component
        if create:
            try:
                current.mkdir(mode=0o755)
                current.chmod(0o755)
            except FileExistsError:
                pass
        if not create and not os.path.lexists(current):
            return False
        directory(current)
    return True


def admit_payload(root, source):
    record = overlay.verify(source)
    if not root.is_absolute() or root.resolve() == Path("/") or root != Path(os.path.abspath(root)) or root == source:
        raise ValueError("expected separate assembly root")
    directory(root)
    for item in record["files"]:
        if not parents(root, item["path"]):
            raise ValueError("missing payload parent")
        fd = os.open(root / item["path"], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        with os.fdopen(fd, "rb") as file:
            info = os.fstat(file.fileno())
            if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_nlink != 1 or stat.S_IMODE(info.st_mode) != item["mode"] or info.st_size != item["bytes"]:
                raise ValueError("merged payload metadata mismatch")
            digest = hashlib.sha256()
            remaining = item["bytes"]
            while remaining:
                chunk = file.read(min(remaining, 1 << 20))
                if not chunk:
                    raise ValueError("short merged payload")
                remaining -= len(chunk)
                digest.update(chunk)
            if file.read(1) or digest.hexdigest() != item["sha256"]:
                raise ValueError("merged payload digest mismatch")
    for name in overlay.MOUNTPOINTS:
        info = (root / name).lstat()
        allowed = (0o755, 0o1777) if name == "tmp" else (0o755,)
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid() or info.st_gid != os.getegid() or stat.S_IMODE(info.st_mode) not in allowed:
            raise ValueError("unexpected underlying mountpoint metadata")
    identity.check(root)
    return record


def finalize(root, source):
    # Inputs must be exclusively controlled assembly staging throughout this call.
    record = admit_payload(root, source)
    profile = record["profile"]
    links = {
        "etc/systemd/system/local-fs.target.wants/tmp.mount": "/usr/lib/systemd/system/tmp.mount",
        "etc/systemd/system/local-fs.target.wants/var.mount": "/usr/lib/systemd/system/var.mount",
        f"etc/systemd/system/multi-user.target.wants/homenode-guest@{profile}.service": "/usr/lib/systemd/system/homenode-guest@.service",
    }
    # Never replace an existing enablement or silently combine workload profiles.
    for prefix in ("etc", "usr/lib"):
        for other in overlay.PROFILES:
            if other == profile:
                continue
            name = f"{prefix}/systemd/system/multi-user.target.wants/homenode-guest@{other}.service"
            if parents(root, name) and os.path.lexists(root / name):
                raise ValueError("another workload profile is enabled")
    for name, target in links.items():
        parents(root, name, create=True)
        path = root / name
        if os.path.lexists(path) and (not path.is_symlink() or os.readlink(path) != target):
            raise ValueError("foreign unit enablement")
    for name, target in links.items():
        path = root / name
        try:
            path.symlink_to(target)
        except FileExistsError:
            if not path.is_symlink() or os.readlink(path) != target:
                raise ValueError("unit enablement changed")
        fd = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)
    return record


if __name__ == "__main__":
    try:
        if sys.platform != "linux" or os.geteuid() != 0 or os.getenv("HOMENODE_GUEST_IMAGE_BUILD") != "1":
            raise ValueError("requires explicit Linux image assembly")
        finalize(Path(sys.argv[1]), Path(sys.argv[2]))
    except (OSError, ValueError, IndexError, KeyError, TypeError, UnicodeError) as error:
        # Policy errors contain fixed messages; OS paths, process arguments and
        # account contents are deliberately absent from this summary.
        reason = str(error)[:240] if type(error) is ValueError else type(error).__name__
        sys.exit("Guest root finalization failed: " + reason + "; retain assembly staging")
