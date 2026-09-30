#!/usr/bin/env python3
"""Opt-in disposable Linux CI fixture; never validates guest boot prerequisites."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import uuid

if sys.platform != "linux" or os.geteuid() != 0 or os.getenv("HOMENODE_GUEST_SYSTEMD_INTEGRATION") != "1":
    sys.exit("Requires explicit disposable Linux root fixture")

binary = Path("/usr/lib/homenode-fixtures/guest-init.test")
if not binary.is_file():
    sys.exit("Missing fixture binary")
source = Path(__file__).with_name("homenode-data-init.service").read_text()
allowed = {"Type", "User", "Group", "UMask", "TimeoutStartSec", "NoNewPrivileges", "CapabilityBoundingSet", "AmbientCapabilities", "ProtectSystem", "ProtectHome", "PrivateTmp", "ProtectKernelTunables", "ProtectKernelModules", "ProtectControlGroups", "RestrictSUIDSGID", "RestrictRealtime", "RestrictNamespaces", "RestrictAddressFamilies", "DevicePolicy", "ReadWritePaths", "TasksMax"}
properties, seen, section = [], set(), ""
for line in source.splitlines():
    line = line.strip()
    if not line or line.startswith("#"):
        continue
    if line.startswith("["):
        section = line
        continue
    if section != "[Service]":
        continue
    key, separator, value = line.partition("=")
    if not separator or key in seen:
        sys.exit("Invalid fixture source")
    seen.add(key)
    if key in ("ExecStart", "RemainAfterExit"):
        continue
    if key not in allowed:
        sys.exit("Unreviewed fixture property")
    properties.append("--property=" + key + "=" + value)
if set(seen) != allowed | {"ExecStart", "RemainAfterExit"}:
    sys.exit("Incomplete fixture source")
# Only an exclusively created ordinary directory is removed. Existing /data is refused.
os.mkdir("/data", 0o755)
try:
    with tempfile.TemporaryDirectory(prefix="homenode-private-tmp-probe-") as private:
        marker = Path(private) / "marker"
        marker.write_text("fixture")
        command = ["/usr/bin/systemd-run", "--quiet", "--wait", "--pipe", "--collect", "--unit=homenode-init-fixture-" + uuid.uuid4().hex,
                   "--property=RemainAfterExit=no", *properties,
                   "--setenv=HOMENODE_GUEST_INIT_INTEGRATION=1", "--setenv=HOMENODE_GUEST_INIT_CAPABILITIES=1",
                   "--setenv=HOMENODE_GUEST_INIT_PARENT=/data", "--setenv=HOMENODE_GUEST_INIT_SERVICE=1", "--setenv=HOMENODE_GUEST_INIT_HIDDEN_PATH=" + str(marker),
                   str(binary), "-test.run=^TestNativeGuestObjectInitialization$", "-test.count=1"]
        result = subprocess.run(command, timeout=45, check=False)
        if result.returncode:
            sys.exit("Guest unit protection fixture failed")
finally:
    os.rmdir("/data")
