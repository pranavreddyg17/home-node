#!/usr/bin/env python3
"""Opt-in disposable Linux service fixture for package inspection."""
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile
import uuid

if sys.platform != "linux" or os.geteuid() != 0 or os.getenv("HOMENODE_INSPECT_SYSTEMD_INTEGRATION") != "1":
    sys.exit("Requires explicit disposable Linux root fixture")
binary = Path("/usr/lib/homenode-fixtures/inspect.test")
package = Path(os.environ["HOMENODE_PACKAGE_CONTENT_FIXTURE"])
if not binary.is_file() or not package.is_file():
    sys.exit("Missing fixture binary or development package")
source = Path(__file__).resolve().parents[1] / "systemd/homenode-inspect.service"
allowed = {"Type", "RemainAfterExit", "DynamicUser", "SupplementaryGroups", "UMask", "Restart", "TimeoutStartSec", "TimeoutStopSec", "KillMode", "NoNewPrivileges", "CapabilityBoundingSet", "AmbientCapabilities", "ProtectSystem", "ProtectHome", "PrivateTmp", "PrivateDevices", "PrivateNetwork", "ProtectKernelTunables", "ProtectKernelModules", "ProtectKernelLogs", "ProtectControlGroups", "ProtectProc", "ProcSubset", "RestrictNamespaces", "RestrictSUIDSGID", "RestrictRealtime", "LockPersonality", "RestrictAddressFamilies", "IPAddressDeny", "SystemCallArchitectures", "SystemCallFilter", "InaccessiblePaths", "MemoryMax", "MemorySwapMax", "CPUQuota", "TasksMax", "OOMPolicy"}
properties, seen, section = [], set(), ""
for raw in source.read_text().splitlines():
    line = raw.strip()
    if not line or line.startswith("#"):
        continue
    if line.startswith("["):
        section = line
        continue
    if section != "[Service]":
        continue
    key, separator, value = line.partition("=")
    if not separator or key in seen:
        sys.exit("Ambiguous service fixture source")
    seen.add(key)
    if key in {"ExecStart", "EnvironmentFile", "OpenFile"}:
        continue
    if key not in allowed:
        sys.exit("Unreviewed service fixture property: " + key)
    # This security fixture waits for unit deactivation; completion identity
    # retention is exercised separately by the real-manager Go fixture.
    if key == "RemainAfterExit":
        value = "no"
    properties.append("--property=" + key + "=" + value)
if seen != allowed | {"ExecStart", "EnvironmentFile", "OpenFile"}:
    sys.exit("Incomplete service fixture source")
with tempfile.TemporaryDirectory(prefix="hn-inspect-systemd-") as directory, tempfile.TemporaryDirectory(prefix="hn-inspect-hidden-") as hidden, socket.socket() as listener:
    Path(hidden).chmod(0o755)
    marker = Path(hidden) / "marker"
    marker.write_text("world-readable host fixture")
    marker.chmod(0o644)
    listener.bind(("127.0.0.1", 0))
    listener.listen(1)
    private = Path(directory) / "package.deb"
    shutil.copyfile(package, private)
    private.chmod(0o400)
    command = ["/usr/bin/systemd-run", "--quiet", "--wait", "--pipe", "--collect",
               "--unit=homenode-inspect-fixture-" + uuid.uuid4().hex, *properties,
               "--property=OpenFile=" + str(private) + ":verified-package:read-only",
               "--setenv=HOMENODE_INSPECT_ENTRY_CHILD=1",
               "--setenv=HOMENODE_INSPECT_OPERATION=" + uuid.uuid4().hex,
               "--setenv=HOMENODE_INSPECT_SERVICE_LIMITS=1",
               "--setenv=HOMENODE_INSPECT_HIDDEN_PATH=" + str(marker),
               "--setenv=HOMENODE_INSPECT_DENIED_PORT=" + str(listener.getsockname()[1]), str(binary),
               "-test.run=^TestNativeInspectionDescriptor$", "-test.count=1", "-test.v"]
    subprocess.run(command, timeout=180, check=True)
