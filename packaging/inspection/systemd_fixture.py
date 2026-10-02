#!/usr/bin/env python3
"""Opt-in disposable Linux service fixture for package inspection."""
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile
import time
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
    unit = "homenode-inspect-fixture-" + uuid.uuid4().hex + ".service"
    boundary = time.monotonic_ns() // 1000
    command = ["/usr/bin/systemd-run", "--quiet", "--no-block", "--collect",
               "--unit=" + unit, *properties,
               "--property=OpenFile=" + str(private) + ":verified-package:read-only",
               "--setenv=HOMENODE_INSPECT_ENTRY_CHILD=1",
               "--setenv=HOMENODE_INSPECT_OPERATION=" + uuid.uuid4().hex,
               "--setenv=HOMENODE_INSPECT_SERVICE_LIMITS=1",
               "--setenv=HOMENODE_INSPECT_HIDDEN_PATH=" + str(marker),
               "--setenv=HOMENODE_INSPECT_DENIED_PORT=" + str(listener.getsockname()[1]), str(binary),
               "-test.run=^TestNativeInspectionDescriptor$", "-test.count=1", "-test.v"]
    keys = {"InvocationID", "Result", "ExecMainCode", "ExecMainStatus", "ActiveState", "SubState", "ExecMainStartTimestampMonotonic", "ExecMainExitTimestampMonotonic"}
    resource_values = {"MemoryMax": "268435456", "MemorySwapMax": "0", "CPUQuotaPerSecUSec": "500ms", "TasksMax": "32", "OOMPolicy": "kill", "KillMode": "control-group", "Restart": "no", "TimeoutStartUSec": "2min 30s", "TimeoutStopUSec": "5s"}
    isolation_values = {"NoNewPrivileges": "yes", "CapabilityBoundingSet": "", "AmbientCapabilities": "", "ProtectSystem": "strict", "ProtectHome": "yes", "PrivateTmp": "yes", "PrivateDevices": "yes", "PrivateNetwork": "yes", "ProtectKernelTunables": "yes", "ProtectKernelModules": "yes", "ProtectKernelLogs": "yes", "ProtectControlGroups": "yes", "ProtectProc": "invisible", "ProcSubset": "pid", "RestrictSUIDSGID": "yes", "RestrictRealtime": "yes", "LockPersonality": "yes", "UMask": "0077", "SupplementaryGroups": ""}
    process_values = {"RestrictNamespaces": "yes", "RestrictAddressFamilies": "AF_UNIX", "SystemCallArchitectures": "native"}
    keys |= process_values.keys()
    keys |= isolation_values.keys()
    keys |= resource_values.keys()
    invocation = None
    try:
        subprocess.run(command, timeout=10, check=True)
        deadline = time.monotonic() + 180
        while time.monotonic() < deadline:
            query = subprocess.run(["/usr/bin/systemctl", "--system", "--no-pager", "show",
                                    "--property=" + ",".join(sorted(keys)), unit],
                                   timeout=5, check=True, capture_output=True, text=True)
            if len(query.stdout) > 2048:
                sys.exit("Oversized inspection manager evidence")
            values = {}
            for line in query.stdout.splitlines():
                key, separator, value = line.partition("=")
                if not separator or key not in keys or key in values:
                    sys.exit("Ambiguous inspection manager evidence")
                values[key] = value
            if values.keys() != keys:
                sys.exit("Missing inspection manager evidence")
            if any(values[key] != value for key, value in resource_values.items()):
                sys.exit("Inspection manager resource limits differ from source contract")
            if any(values[key] != value for key, value in isolation_values.items()):
                sys.exit("Inspection manager confinement differs from source contract")
            if any(values[key] != value for key, value in process_values.items()):
                sys.exit("Inspection manager process policy differs from source contract")
            observed = values["InvocationID"]
            if observed:
                if len(observed) != 32 or any(c not in "0123456789abcdef" for c in observed) or observed == "0" * 32:
                    sys.exit("Invalid inspection invocation")
                if invocation is not None and invocation != observed:
                    sys.exit("Inspection invocation changed")
                invocation = observed
            if values["ActiveState"] == "failed":
                sys.exit("Isolated inspection fixture failed: " + values["Result"])
            if values["ActiveState"] == "active" and values["SubState"] == "exited":
                start = int(values["ExecMainStartTimestampMonotonic"])
                end = int(values["ExecMainExitTimestampMonotonic"])
                if invocation is None or values["Result"] != "success" or values["ExecMainCode"] != "1" or values["ExecMainStatus"] != "0" or start < boundary or end < start:
                    sys.exit("Inspection completion evidence refused")
                print("Source-isolated inspection completed with retained manager identity")
                break
            time.sleep(0.1)
        else:
            sys.exit("Inspection fixture completion timed out")
    finally:
        subprocess.run(["/usr/bin/systemctl", "stop", unit], timeout=10, check=False)
        subprocess.run(["/usr/bin/systemctl", "reset-failed", unit], timeout=5, check=False)
