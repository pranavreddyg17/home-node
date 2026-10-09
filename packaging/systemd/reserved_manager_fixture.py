"""Temporarily qualify NSS on an explicitly disposable Linux KVM runner."""
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
    config_fd = None
    original = installed = metadata = None
    changed = False
    try:
        lock_metadata = os.fstat(account_fd)
        if not stat.S_ISREG(lock_metadata.st_mode) or stat.S_IMODE(lock_metadata.st_mode) != 0o600 or lock_metadata.st_uid != 0 or lock_metadata.st_gid != 0 or lock_metadata.st_nlink != 1 or lock_metadata.st_size != 0:
            raise RuntimeError("unqualified account lock")
        verify_account_lock(account_fd, lock_metadata)
        fcntl.lockf(account_fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        config_fd = os.open("/etc/nsswitch.conf", os.O_RDWR | os.O_NOFOLLOW | os.O_NONBLOCK)
        metadata = os.fstat(config_fd)
        if not stat.S_ISREG(metadata.st_mode) or metadata.st_uid != 0 or metadata.st_gid != 0 or metadata.st_nlink != 1 or metadata.st_mode & 0o22 or metadata.st_size > 65536:
            raise RuntimeError("unqualified NSS configuration")
        original = os.pread(config_fd, 65537, 0)
        lines = original.decode("utf-8").splitlines()
        identity = {"passwd", "group", "shadow", "subid"}
        retained = [line for line in lines if line.split(":", 1)[0].strip() not in identity]
        installed = ("\n".join(retained + [name + ": files" for name in sorted(identity)]) + "\n").encode()
        verify(config_fd, metadata, original)
        changed = True
        replace(config_fd, installed)
        fcntl.lockf(account_fd, fcntl.LOCK_UN)
        subprocess.run([sys.argv[1], "test", "./internal/supervisor", "-run", "^TestNativeReservedDACManagerLaunch$", "-count=1", "-v"], check=True)
    finally:
        try:
            if changed:
                verify_account_lock(account_fd, lock_metadata)
                fcntl.lockf(account_fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
                verify(config_fd, metadata, installed)
                replace(config_fd, original)
        finally:
            if config_fd is not None:
                os.close(config_fd)
            os.close(account_fd)


def verify(fd, original_metadata, expected):
    current = os.fstat(fd)
    named = os.stat("/etc/nsswitch.conf", follow_symlinks=False)
    keys = ("st_dev", "st_ino", "st_mode", "st_uid", "st_gid", "st_nlink")
    if any(getattr(current, key) != getattr(original_metadata, key) or getattr(named, key) != getattr(current, key) for key in keys) or os.pread(fd, 65537, 0) != expected:
        raise RuntimeError("NSS fixture authority changed; preserving evidence")


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
            raise RuntimeError("NSS fixture publication incomplete")
        offset += written
    os.ftruncate(fd, len(data))
    os.fsync(fd)


if __name__ == "__main__":
    main()
