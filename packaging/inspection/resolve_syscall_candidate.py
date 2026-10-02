#!/usr/bin/env python3
"""Observe native libseccomp resolution of the reviewed source candidate.

Output is qualification evidence, never a production activation policy.
This does not query systemd or learn a baseline from an observed unit.
"""
import ctypes
import hashlib
import json
from pathlib import Path
import platform
import subprocess
import sys

if sys.platform != "linux" or platform.machine() != "x86_64":
    sys.exit("Requires the supported Linux amd64 qualification host")
path = Path(__file__).with_name("syscall-source-candidate.json")
with path.open("rb") as source_file:
    raw = source_file.read(16385)
if len(raw) > 16384:
    sys.exit("Oversized source candidate")
candidate_digest = hashlib.sha256(raw).hexdigest()
if candidate_digest != "8fd83013bc7302b5b2043ddc44daf6acfcb5882848b6e51aed6bc7a16098b61b":
    sys.exit("Source candidate differs from reviewed exact artifact")

# Retain installed distribution package identities for backport qualification.
# This inventory is evidence, not approval of every package version.
inventory = subprocess.run(
    ["/usr/bin/dpkg-query", "-W", "-f=${binary:Package}\t${Version}\t${Architecture}\n", "systemd", "libseccomp2"],
    timeout=5, check=True, capture_output=True, text=True,
    env={"PATH": "/usr/bin:/bin", "LC_ALL": "C"})
if len(inventory.stdout) > 1024:
    sys.exit("Oversized qualification package inventory")
packages = {}
for line in inventory.stdout.splitlines():
    fields = line.split("\t")
    if len(fields) != 3 or fields[2] != "amd64" or not fields[1] or len(fields[1]) > 128:
        sys.exit("Unexpected qualification package identity")
    name = fields[0].removesuffix(":amd64")
    if name not in {"systemd", "libseccomp2"} or name in packages:
        sys.exit("Ambiguous qualification package inventory")
    packages[name] = fields[1]
if set(packages) != {"systemd", "libseccomp2"}:
    sys.exit("Incomplete qualification package inventory")

def unique(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("Duplicate candidate field")
        result[key] = value
    return result

candidate = json.loads(raw, object_pairs_hook=unique)
if candidate["schema"] != 1 or candidate["status"] != "source-candidate-not-qualified" or candidate["sourceSha256"] != "b97066aee652ea9ff26c9f4c331de0f149686911d11f813fc553e87bbec6bfa8" or candidate["groups"] != ["@mount", "@privileged", "@raw-io", "@reboot", "@swap"]:
    sys.exit("Unexpected source candidate identity")
names = candidate["candidateSyscalls"]
if not isinstance(names, list) or len(names) != 63 or names != sorted(set(names)) or any(not isinstance(name, str) or not name or len(name) > 64 or any(c not in "abcdefghijklmnopqrstuvwxyz0123456789_" for c in name) for name in names):
    sys.exit("Malformed source syscall union")
lib = ctypes.CDLL("libseccomp.so.2")
class SeccompVersion(ctypes.Structure):
    _fields_ = [("major", ctypes.c_uint), ("minor", ctypes.c_uint), ("micro", ctypes.c_uint)]

lib.seccomp_version.restype = ctypes.POINTER(SeccompVersion)
lib.seccomp_version.argtypes = []
version = lib.seccomp_version()
if not version:
    sys.exit("Missing native libseccomp version")
version_text = ".".join(str(getattr(version.contents, name)) for name in ("major", "minor", "micro"))
lib.seccomp_arch_native.restype = ctypes.c_uint32
lib.seccomp_arch_native.argtypes = []
if lib.seccomp_arch_native() != 0xC000003E:
    sys.exit("Unexpected native seccomp ABI")
lib.seccomp_syscall_resolve_name.argtypes = [ctypes.c_char_p]
lib.seccomp_syscall_resolve_name.restype = ctypes.c_int
lib.seccomp_syscall_resolve_num_arch.argtypes = [ctypes.c_uint32, ctypes.c_int]
lib.seccomp_syscall_resolve_num_arch.restype = ctypes.c_void_p
libc = ctypes.CDLL(None)
libc.free.argtypes = [ctypes.c_void_p]
libc.free.restype = None
resolved, unknown = {}, []
for name in names:
    number = lib.seccomp_syscall_resolve_name(name.encode("ascii"))
    # systemd's parser ignores only __NR_SCMP_ERROR (-1); negative pseudo
    # syscall numbers are retained and must not silently be omitted here.
    if number == -1:
        unknown.append(name)
        continue
    pointer = lib.seccomp_syscall_resolve_num_arch(0, number)
    if not pointer:
        sys.exit("Native syscall reverse resolution failed")
    try:
        canonical = ctypes.string_at(pointer).decode("ascii")
    finally:
        libc.free(pointer)
    resolved[name] = {"number": number, "canonicalName": canonical}
print(json.dumps({"schema": 1, "status": "native-resolution-observation-not-qualified", "sourceSha256": candidate["sourceSha256"], "candidateSha256": candidate_digest, "nativeABI": "x86_64", "distributionPackages": packages, "libseccompVersion": version_text, "resolved": resolved, "unknown": unknown, "requiredSyscalls": sorted({entry["canonicalName"] for entry in resolved.values()})}, sort_keys=True))
