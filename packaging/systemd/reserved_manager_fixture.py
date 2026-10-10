"""Temporarily qualify identity configuration on an explicitly disposable Linux KVM runner."""
import fcntl
import json
import os
import stat
import subprocess
import sys


def main():
    if sys.platform != "linux" or os.geteuid() != 0 or os.environ.get("HOMENODE_KVM_DOMAIN_INTEGRATION") != "1":
        raise RuntimeError("explicit disposable Linux root fixture required")
    if len(sys.argv) not in (2, 4) or not os.path.isabs(sys.argv[1]):
        raise RuntimeError("absolute Go executable required")
    test = "TestNativeReservedDACManagerLaunch"
    if len(sys.argv) == 4:
        prepare_files_input(sys.argv[2], sys.argv[3])
        test = "TestNativeReservedFilesManagerLaunch"
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
        subprocess.run([sys.argv[1], "test", "./internal/supervisor", "-run", "^" + test + "$", "-count=1", "-v"], check=True, timeout=600)
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


def prepare_files_input(image, manifest):
    if os.environ.get("HOMENODE_FILES_MANAGER_INTEGRATION") != "1" or not os.path.isabs(image) or not os.path.isabs(manifest):
        raise RuntimeError("explicit absolute development Files inputs required")
    fd = os.open(manifest, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as file:
        info = os.fstat(file.fileno())
        if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= 16384:
            raise RuntimeError("invalid development manifest")
        data = file.read(16385)
        if len(data) != info.st_size:
            raise RuntimeError("development manifest changed")
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError("duplicate manifest field")
            result[key] = value
        return result
    record = json.loads(data, object_pairs_hook=unique)
    expected = {"schema", "profile", "sourceRevision", "overlaySHA256", "mkosiRevision", "bytes", "sha256", "releaseQualified", "bootValidated"}
    if not isinstance(record, dict) or set(record) != expected or type(record["schema"]) is not int or record["schema"] != 1 or record["profile"] != "files" or record["releaseQualified"] is not False or record["bootValidated"] is not False:
        raise RuntimeError("expected unqualified development Files manifest")
    digest, size = record["sha256"], record["bytes"]
    if not isinstance(digest, str) or len(digest) != 64 or any(char not in "0123456789abcdef" for char in digest) or type(size) is not int or not 0 < size <= 8 << 30:
        raise RuntimeError("invalid development image identity")
    os.environ.update(HOMENODE_FILES_IMAGE=image, HOMENODE_FILES_IMAGE_SHA256=digest, HOMENODE_FILES_IMAGE_BYTES=str(size))


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
