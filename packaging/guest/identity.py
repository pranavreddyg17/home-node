#!/usr/bin/env python3
"""Read-only admission of the fixed guest service identity in assembly roots."""
import os
import re
from pathlib import Path
import stat
import sys

NAME = "homenode-guest"
UID = GID = 900
LIMIT = 1 << 20


def read_file(root, name):
    fd = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        for part in name.split("/")[:-1]:
            next_fd = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd)
            fd = next_fd
        file_fd = os.open(name.split("/")[-1], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=fd)
        with os.fdopen(file_fd, "rb") as file:
            info = os.fstat(file.fileno())
            if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_size > LIMIT or info.st_mode & 0o022 or (name == "etc/shadow" and info.st_mode & 0o007):
                raise ValueError("unprotected identity input")
            data = file.read(LIMIT + 1)
            if len(data) > LIMIT or b"\0" in data or b"\r" in data:
                raise ValueError("invalid identity input")
            return data.decode("utf-8")
    finally:
        os.close(fd)


def rows(data, fields):
    result, names = [], set()
    for line in data.splitlines():
        if not line:
            continue
        row = line.split(":")
        if len(row) != fields or not row[0] or row[0] in names:
            raise ValueError("invalid identity database")
        names.add(row[0])
        result.append(row)
    if not result:
        raise ValueError("empty identity database")
    return result


def number(value):
    if not re.fullmatch(r"0|[1-9][0-9]*", value) or int(value) >= (1 << 32) - 1:
        raise ValueError("invalid identity number")
    return int(value)


def validate(passwd, groups, shadow, nss):
    users, group_rows, credentials = rows(passwd, 7), rows(groups, 4), rows(shadow, 9)
    sources = {}
    for line in nss.splitlines():
        line = line.split("#", 1)[0].strip()
        if not line:
            continue
        name, separator, rest = line.partition(":")
        if name.strip() not in ("passwd", "group", "shadow"):
            continue
        name = name.strip()
        if not separator or name in sources or rest.split() not in (["files"], ["files", "systemd"]):
            raise ValueError("nonlocal identity resolution")
        sources[name] = True
    if set(sources) != {"passwd", "group", "shadow"}:
        raise ValueError("missing local identity sources")
    expected = [NAME, "x", str(UID), str(GID), "HomeNode guest adapter", "/nonexistent", "/usr/sbin/nologin"]
    found = [row for row in users if row[0] == NAME]
    if found != [expected]:
        raise ValueError("guest identity mismatch")
    for row in users:
        uid, gid = number(row[2]), number(row[3])
        if row[0] != NAME and (uid == UID or gid == GID):
            raise ValueError("guest identity alias")
    own = [row for row in group_rows if row[0] == NAME]
    if len(own) != 1 or own[0][2] != str(GID) or own[0][1] not in ("x", "!", "*") or own[0][3] not in ("", NAME):
        raise ValueError("guest group mismatch")
    for row in group_rows:
        if row[0] != NAME and (number(row[2]) == GID or NAME in row[3].split(",")):
            raise ValueError("guest supplementary privilege")
    locked = [row for row in credentials if row[0] == NAME]
    if len(locked) != 1 or not locked[0][1].startswith(("!", "*")):
        raise ValueError("guest login is not locked")


def check(root):
    validate(*(read_file(root, name) for name in ("etc/passwd", "etc/group", "etc/shadow", "etc/nsswitch.conf")))


if __name__ == "__main__":
    try:
        check(Path(sys.argv[1]))
    except (OSError, ValueError, IndexError, UnicodeError):
        sys.exit("Guest identity admission failed")
