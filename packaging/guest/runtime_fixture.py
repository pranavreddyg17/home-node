#!/usr/bin/env python3
"""Disposable Linux CI: mount guest tmpfs sources only in a new private namespace."""
import os
from pathlib import Path
import subprocess
import sys

if sys.platform != "linux" or os.geteuid() != 0 or os.getenv("HOMENODE_GUEST_RUNTIME_INTEGRATION") != "1":
    sys.exit("Requires explicit disposable Linux root fixture")

namespace = os.readlink("/proc/self/ns/mnt")
if sys.argv[1:] != ["--child"]:
    if len(sys.argv) != 1:
        sys.exit("Unexpected fixture arguments")
    environment = {**os.environ, "HOMENODE_GUEST_RUNTIME_PARENT_NAMESPACE": namespace}
    subprocess.run(["/usr/bin/unshare", "--mount", "--propagation", "private", "--fork",
                    sys.executable, str(Path(__file__).resolve()), "--child"],
                   env=environment, check=True, timeout=90)
    sys.exit(0)

if namespace == os.getenv("HOMENODE_GUEST_RUNTIME_PARENT_NAMESPACE") or not os.getenv("HOMENODE_GUEST_RUNTIME_PARENT_NAMESPACE"):
    sys.exit("Private mount namespace was not created")
binary = Path("/usr/lib/homenode-fixtures/guestmount.test")
if not binary.is_file():
    sys.exit("Missing fixture binary")


def check(expectation):
    subprocess.run([str(binary), "-test.run=^TestNativeRuntimeMountAdmission$", "-test.count=1"],
                   env={**os.environ, "HOMENODE_GUEST_RUNTIME_EXPECT": expectation},
                   check=True, timeout=15)


for leaf in ("tmp", "var"):
    source = Path(__file__).with_name(leaf + ".mount").read_text()
    values, section = {}, ""
    for line in source.splitlines():
        if line.startswith("["):
            section = line
        elif section == "[Mount]" and line and not line.startswith("#"):
            key, separator, value = line.partition("=")
            if not separator or key in values:
                sys.exit("Invalid mount source")
            values[key] = value
    if set(values) != {"What", "Where", "Type", "Options", "TimeoutSec"} or values["What"] != "tmpfs" or values["Type"] != "tmpfs" or values["Where"] != "/" + leaf:
        sys.exit("Unexpected mount source")
    subprocess.run(["/usr/bin/mount", "-t", "tmpfs", "-o", values["Options"], "tmpfs", "/" + leaf], check=True, timeout=15)
check("accept")

# Only this child namespace sees these mounts and their metadata changes.
for mode, path, safe in [(0o755, "/tmp", 0o1777), (0o777, "/var", 0o755)]:
    os.chmod(path, mode)
    check("reject")
    os.chmod(path, safe)

safe = "rw,nodev,nosuid,noexec,size=64M,nr_inodes=8192"
for options in ["rw,nodev,nosuid,noexec,size=65M,nr_inodes=8192",
                "rw,nodev,nosuid,noexec,size=64M,nr_inodes=8193",
                "rw,dev,suid,exec,size=64M,nr_inodes=8192",
                "ro,nodev,nosuid,noexec,size=64M,nr_inodes=8192"]:
    subprocess.run(["/usr/bin/mount", "-o", "remount," + options, "/tmp"], check=True, timeout=15)
    check("reject")
    subprocess.run(["/usr/bin/mount", "-o", "remount," + safe, "/tmp"], check=True, timeout=15)
check("accept")
# Exercise the filesystem views expected from PrivateTmp/ProtectSystem without
# weakening the production adapter unit. This does not execute that source unit.
private = Path("/tmp/private/tmp")
private.mkdir(parents=True)
private.chmod(0o1777)
subprocess.run(["/usr/bin/mount", "--bind", str(private), "/tmp"], check=True, timeout=15)
subprocess.run(["/usr/bin/mount", "-o", "remount,ro,nodev,nosuid,noexec,size=64M,nr_inodes=8192", "/var"], check=True, timeout=15)
check("accept")
# Exiting the exclusively created namespace releases its mounts; no host unmount.
