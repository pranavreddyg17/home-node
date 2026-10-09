"""Temporarily qualify identity configuration on an explicitly disposable Linux KVM runner."""
import fcntl
import os
import stat
import subprocess
import sys


def main():
    if sys.platform != "linux" or os.geteuid() != 0 or os.environ.get("HOMENODE_KVM_DOMAIN_INTEGRATION") != "1":
        raise RuntimeError("explicit disposable Linux root fixture required")
    if len(sys.argv) != 2 or not os.path.isabs(sys.argv[1]):
        raise RuntimeError("absolute Go executable required")
    account_fd = os.open("/etc/.pwd.lock", os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW | os.O_NONBLOCK, 0o600)
    records = []
    try:
        lock_metadata = os.fstat(account_fd)
        if not stat.S_ISREG(lock_metadata.st_mode) or stat.S_IMODE(lock_metadata.st_mode) != 0o600 or lock_metadata.st_uid != 0 or lock_metadata.st_gid != 0 or lock_metadata.st_nlink != 1 or lock_metadata.st_size != 0:
            raise RuntimeError("unqualified account lock")
        verify_account_lock(account_fd, lock_metadata)
        fcntl.lockf(account_fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        for path, transform in (("/etc/nsswitch.conf", qualify_nss), ("/etc/login.defs", qualify_allocators)):
            fd = os.open(path, os.O_RDWR | os.O_NOFOLLOW | os.O_NONBLOCK)
            record = {"path": path, "fd": fd, "changed": False}
            records.append(record)
            metadata = os.fstat(fd)
            if not stat.S_ISREG(metadata.st_mode) or metadata.st_uid != 0 or metadata.st_gid != 0 or metadata.st_nlink != 1 or metadata.st_mode & 0o22 or metadata.st_size > 65536:
                raise RuntimeError("unqualified identity configuration")
            original = os.pread(fd, 65537, 0)
            installed = transform(original)
            record.update(metadata=metadata, original=original, installed=installed)
            verify(path, fd, metadata, original)
            record["changed"] = True
            replace(fd, installed)
            verify(path, fd, metadata, installed)
        fcntl.lockf(account_fd, fcntl.LOCK_UN)
        subprocess.run([sys.argv[1], "test", "./internal/supervisor", "-run", "^TestNativeReservedDACManagerLaunch$", "-count=1", "-v"], check=True)
    finally:
        try:
            if any(record["changed"] for record in records):
                verify_account_lock(account_fd, lock_metadata)
                fcntl.lockf(account_fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
                # Preflight every changed file before restoring any file.
                for record in records:
                    if record["changed"]:
                        verify(record["path"], record["fd"], record["metadata"], record["installed"])
                for record in reversed(records):
                    if record["changed"]:
                        replace(record["fd"], record["original"])
                        verify(record["path"], record["fd"], record["metadata"], record["original"])
        finally:
            for record in records:
                os.close(record["fd"])
            os.close(account_fd)


def qualify_nss(original):
    identity = {"passwd", "group", "shadow", "subid"}
    retained = [line for line in original.decode("utf-8").splitlines() if line.split(":", 1)[0].strip() not in identity]
    return ("\n".join(retained + [name + ": files" for name in sorted(identity)]) + "\n").encode()


def qualify_allocators(original):
    bounds = {"UID_MIN": 1000, "UID_MAX": 60000, "SYS_UID_MIN": 100,
              "SYS_UID_MAX": 999, "SUB_UID_MIN": 100000, "SUB_UID_MAX": 600100000}
    retained = []
    for line in original.decode("utf-8").splitlines():
        fields = line.split("#", 1)[0].split()
        if not fields or fields[0] not in bounds:
            retained.append(line)
    return ("\n".join(retained + [f"{name} {value}" for name, value in bounds.items()]) + "\n").encode()


def verify(path, fd, original_metadata, expected):
    current = os.fstat(fd)
    named = os.stat(path, follow_symlinks=False)
    keys = ("st_dev", "st_ino", "st_mode", "st_uid", "st_gid", "st_nlink")
    if any(getattr(current, key) != getattr(original_metadata, key) or getattr(named, key) != getattr(current, key) for key in keys) or os.pread(fd, 65537, 0) != expected:
        raise RuntimeError("identity fixture authority changed; preserving evidence")


def verify_account_lock(fd, original):
    current = os.fstat(fd)
    named = os.stat("/etc/.pwd.lock", follow_symlinks=False)
    for key in ("st_dev", "st_ino", "st_mode", "st_uid", "st_gid", "st_nlink", "st_size"):
        if getattr(current, key) != getattr(original, key) or getattr(named, key) != getattr(current, key):
            raise RuntimeError("account lock replaced; preserving fixture evidence")


def replace(fd, data):
    offset = 0
    while offset < len(data):
        written = os.pwrite(fd, data[offset:], offset)
        if written <= 0:
            raise RuntimeError("identity fixture publication incomplete")
        offset += written
    os.ftruncate(fd, len(data))
    os.fsync(fd)


if __name__ == "__main__":
    main()
