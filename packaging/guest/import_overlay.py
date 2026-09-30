#!/usr/bin/env python3
"""Import a digest-pinned adapter overlay into new private assembly staging."""
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import sys
import tarfile
import tempfile
sys.dont_write_bytecode = True
import overlay

MAX_ARCHIVE = 64 << 20


def import_overlay(archive_path, output, expected_digest, profile, revision):
    if profile not in overlay.PROFILES or not re.fullmatch(r"[a-f0-9]{64}", expected_digest) or not re.fullmatch(r"[a-f0-9]{40}", revision):
        raise ValueError("invalid independent artifact identity")
    parent = output.parent.lstat()
    if not output.is_absolute() or not stat.S_ISDIR(parent.st_mode) or parent.st_uid != os.geteuid() or stat.S_IMODE(parent.st_mode) & 0o022:
        raise ValueError("assembly parent must be protected and owned")
    files = {**overlay.COMMON, f"etc/homenode/guest/{profile}.env": 0o644, "overlay.json": 0o644}
    directories = set()
    for name in files:
        parent = PurePosixPath(name).parent
        while str(parent) != ".":
            directories.add(str(parent))
            parent = parent.parent
    fd = os.open(archive_path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as source, tempfile.TemporaryFile() as snapshot:
        info = os.fstat(source.fileno())
        if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= MAX_ARCHIVE:
            raise ValueError("invalid archive")
        digest, size = hashlib.sha256(), 0
        while chunk := source.read(1 << 20):
            size += len(chunk)
            if size > MAX_ARCHIVE:
                raise ValueError("archive exceeds bound")
            digest.update(chunk)
            snapshot.write(chunk)
        if size != info.st_size or digest.hexdigest() != expected_digest:
            raise ValueError("archive digest mismatch")
        snapshot.seek(0)
        with tarfile.open(fileobj=snapshot, mode="r:") as archive:
            members, names = [], set()
            for member in archive:
                if len(members) >= 32 or member.name in names or member.pax_headers or member.uid != 0 or member.gid != 0:
                    raise ValueError("invalid archive header")
                names.add(member.name)
                if member.name in directories:
                    if not member.isdir() or member.mode != 0o755 or member.size != 0:
                        raise ValueError("invalid directory header")
                elif member.name in files:
                    if member.type not in (tarfile.REGTYPE, tarfile.AREGTYPE) or member.mode != files[member.name] or not 0 < member.size <= MAX_ARCHIVE:
                        raise ValueError("invalid file header")
                    if member.name == "overlay.json" and member.size > 16384:
                        raise ValueError("metadata exceeds bound")
                else:
                    raise ValueError("unexpected archive member")
                members.append(member)
            if names != directories | set(files):
                raise ValueError("incomplete archive")
            # No extract/extractall or archive-selected destination paths.
            os.mkdir(output, 0o700)
            for name in sorted(directories, key=lambda value: (value.count("/"), value)):
                (output / name).mkdir(mode=0o755)
                (output / name).chmod(0o755)
            for member in members:
                if member.name not in files:
                    continue
                fd = os.open(output / member.name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
                with os.fdopen(fd, "wb") as destination:
                    content = archive.extractfile(member)
                    if content is None:
                        raise ValueError("missing member data")
                    with content:
                        copied = 0
                        while chunk := content.read(1 << 20):
                            copied += len(chunk)
                            if copied > member.size:
                                raise ValueError("member exceeds header")
                            destination.write(chunk)
                    if copied != member.size:
                        raise ValueError("short member")
                    destination.flush()
                    os.fchmod(destination.fileno(), files[member.name])
                    os.fsync(destination.fileno())
            record = overlay.verify(output)
            if record["profile"] != profile or record["sourceRevision"] != revision:
                raise ValueError("source identity mismatch")
            for name in sorted(directories, reverse=True) + ["."]:
                fd = os.open(output / name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
                try:
                    os.fsync(fd)
                finally:
                    os.close(fd)
            output.chmod(0o755)
            for path in (output, output.parent):
                fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
                try:
                    os.fsync(fd)
                finally:
                    os.close(fd)


if __name__ == "__main__":
    try:
        import_overlay(Path(sys.argv[1]), Path(sys.argv[2]), *sys.argv[3:])
    except (ValueError, OSError, KeyError, IndexError, TypeError, tarfile.TarError):
        sys.exit("Guest overlay import failed; retain private staging for diagnosis")
