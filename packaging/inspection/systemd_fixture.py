#!/usr/bin/env python3
"""Opt-in disposable Linux service fixture for package inspection."""
import os
import ipaddress
import hashlib
import json
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
resolution = subprocess.run(
    [sys.executable, str(Path(__file__).with_name("resolve_syscall_candidate.py"))],
    timeout=10, check=True, capture_output=True, text=True,
    env={"PATH": "/usr/bin:/bin", "LC_ALL": "C"})
if len(resolution.stdout) > 16384:
    sys.exit("Oversized independent syscall observation")
syscall_observation = json.loads(resolution.stdout)
required_syscalls = syscall_observation["requiredSyscalls"]
if syscall_observation["status"] != "native-resolution-observation-not-qualified" or not required_syscalls or len(required_syscalls) != len(set(required_syscalls)):
    sys.exit("Missing independent syscall observation")
manager_identity_query = subprocess.run(
    ["/usr/bin/systemctl", "--system", "--no-pager", "show", "--property=Version,Architecture"],
    timeout=5, check=True, capture_output=True, text=True,
    env={"PATH": "/usr/bin:/bin", "LC_ALL": "C", "SYSTEMD_COLORS": "0", "SYSTEMD_PAGER": "cat"})
if len(manager_identity_query.stdout) > 512:
    sys.exit("Oversized running manager identity")
manager_identity = {}
for line in manager_identity_query.stdout.splitlines():
    key, separator, value = line.partition("=")
    if not separator or key not in {"Version", "Architecture"} or key in manager_identity or not value or len(value) > 128 or any(ord(c) < 32 or ord(c) > 126 for c in value):
        sys.exit("Ambiguous running manager identity")
    manager_identity[key] = value
if manager_identity.keys() != {"Version", "Architecture"} or manager_identity["Architecture"] != "x86-64":
    sys.exit("Unsupported running manager architecture")
syscall_observation["runningManager"] = manager_identity
allowed = {"StandardOutput", "StandardError", "Type", "RemainAfterExit", "DynamicUser", "SupplementaryGroups", "UMask", "Restart", "TimeoutStartSec", "TimeoutStopSec", "KillMode", "NoNewPrivileges", "CapabilityBoundingSet", "AmbientCapabilities", "ProtectSystem", "ProtectHome", "PrivateTmp", "PrivateDevices", "PrivateNetwork", "ProtectKernelTunables", "ProtectKernelModules", "ProtectKernelLogs", "ProtectControlGroups", "ProtectProc", "ProcSubset", "RestrictNamespaces", "RestrictSUIDSGID", "RestrictRealtime", "LockPersonality", "RestrictAddressFamilies", "IPAddressDeny", "SystemCallArchitectures", "SystemCallFilter", "InaccessiblePaths", "MemoryMax", "MemorySwapMax", "CPUQuota", "TasksMax", "OOMPolicy"}
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
    for mode in ("probe", "production"):
        unit = "homenode-inspect-fixture-" + uuid.uuid4().hex + ".service"
        operation = uuid.uuid4().hex
        boundary = time.monotonic_ns() // 1000
        command = ["/usr/bin/systemd-run", "--quiet", "--no-block", "--collect",
                   "--unit=" + unit, *properties,
                   "--property=OpenFile=" + str(private) + ":verified-package:read-only",
                   "--setenv=HOMENODE_INSPECT_ENTRY_CHILD=1",
                   "--setenv=HOMENODE_INSPECT_OPERATION=" + operation,
                   "--setenv=HOMENODE_INSPECT_SERVICE_LIMITS=1",
                   "--setenv=HOMENODE_INSPECT_HIDDEN_PATH=" + str(marker),
                   "--setenv=HOMENODE_INSPECT_DENIED_PORT=" + str(listener.getsockname()[1]), str(binary),
                   "-test.run=^TestNativeInspectionDescriptor$", "-test.count=1", "-test.v"]
        if mode == "production":
            worker = Path("/usr/lib/homenode/homenode-inspect")
            if not worker.is_file():
                sys.exit("Installed production inspector missing")
            command = ["/usr/bin/systemd-run", "--quiet", "--no-block", "--collect",
                       "--unit=" + unit, *properties,
                       "--property=OpenFile=" + str(private) + ":verified-package:read-only",
                       str(worker), "--release", "0.1.0~ci", "--operation", operation]
        keys = {"InvocationID", "Result", "ExecMainCode", "ExecMainStatus", "ActiveState", "SubState", "ExecMainStartTimestampMonotonic", "ExecMainExitTimestampMonotonic"}
        resource_values = {"MemoryMax": "268435456", "MemorySwapMax": "0", "CPUQuotaPerSecUSec": "500ms", "TasksMax": "32", "OOMPolicy": "kill", "KillMode": "control-group", "Restart": "no", "TimeoutStartUSec": "2min 30s", "TimeoutStopUSec": "5s"}
        isolation_values = {"NoNewPrivileges": "yes", "CapabilityBoundingSet": "", "AmbientCapabilities": "", "ProtectSystem": "strict", "ProtectHome": "yes", "PrivateTmp": "yes", "PrivateDevices": "yes", "PrivateNetwork": "yes", "ProtectKernelTunables": "yes", "ProtectKernelModules": "yes", "ProtectKernelLogs": "yes", "ProtectControlGroups": "yes", "ProtectProc": "invisible", "ProcSubset": "pid", "RestrictSUIDSGID": "yes", "RestrictRealtime": "yes", "LockPersonality": "yes", "UMask": "0077", "SupplementaryGroups": ""}
        process_values = {"StandardOutput": "journal", "StandardError": "journal", "RestrictNamespaces": "yes", "RestrictAddressFamilies": "AF_UNIX", "SystemCallArchitectures": "native"}
        keys |= process_values.keys()
        keys |= {"IPAddressDeny", "IPAddressAllow", "InaccessiblePaths"}
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
                # Query separately to retain the existing manager snapshot bound.
                # Expected names come only from reviewed source plus libseccomp,
                # never this unit's reported configuration.
                syscall_query = subprocess.run(
                    ["/usr/bin/systemctl", "--system", "--no-pager", "show",
                     "--property=SystemCallFilter", unit],
                    timeout=5, check=True, capture_output=True, text=True,
                    env={"PATH": "/usr/bin:/bin", "LC_ALL": "C", "SYSTEMD_COLORS": "0", "SYSTEMD_PAGER": "cat"})
                syscall_line = syscall_query.stdout.removesuffix("\n")
                if len(syscall_query.stdout) > 2048 or not syscall_line.startswith("SystemCallFilter=~") or any(c in syscall_line for c in "\r\n\x00"):
                    sys.exit("Ambiguous inspection syscall denyset")
                observed_syscalls = syscall_line.removeprefix("SystemCallFilter=~").split()
                if len(observed_syscalls) != len(set(observed_syscalls)) or set(observed_syscalls) != set(required_syscalls):
                    sys.exit("Inspection syscall denyset differs from independent source/ABI observation")
                if any(values[key] != value for key, value in resource_values.items()):
                    sys.exit("Inspection manager resource limits differ from source contract")
                if any(values[key] != value for key, value in isolation_values.items()):
                    sys.exit("Inspection manager confinement differs from source contract")
                if any(values[key] != value for key, value in process_values.items()):
                    sys.exit("Inspection manager process policy differs from source contract")
                deny_tokens = values["IPAddressDeny"].split()
                try:
                    deny = [ipaddress.ip_network(value, strict=True) for value in deny_tokens]
                except ValueError:
                    sys.exit("Noncanonical inspection IP deny policy")
                if values["IPAddressAllow"] or len(deny) != 2 or set(deny) != {ipaddress.ip_network("0.0.0.0/0"), ipaddress.ip_network("::/0")}:
                    sys.exit("Inspection IP policy permits host network access")
                protected = {"/etc/homenode", "/var/lib/homenode", "/var/lib/homenode-update", "/var/lib/homenode-backup", "/run/homenode", "/run/homenode-transfer"}
                paths = [value.removeprefix("-") for value in values["InaccessiblePaths"].split()]
                if len(paths) != len(protected) or set(paths) != protected:
                    sys.exit("Inspection protected path policy differs from source contract")
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
                    if mode == "production":
                        subprocess.run(["/usr/bin/journalctl", "--sync"], timeout=5, check=True)
                        journal = subprocess.run(
                            ["/usr/bin/journalctl", "--system", "--no-pager", "--quiet", "--all",
                             "--output=json", "--lines=2",
                             "--output-fields=MESSAGE,_SYSTEMD_UNIT,_SYSTEMD_INVOCATION_ID,_BOOT_ID,_TRANSPORT,_LINE_BREAK",
                             "--", "_SYSTEMD_UNIT=" + unit, "_SYSTEMD_INVOCATION_ID=" + invocation],
                            timeout=5, check=True, capture_output=True, text=True,
                            env={"PATH": "/usr/bin:/bin", "LC_ALL": "C", "SYSTEMD_COLORS": "0", "SYSTEMD_PAGER": "cat"})
                        if len(journal.stdout) > 8192 or len(journal.stdout.splitlines()) != 1:
                            sys.exit("Missing, oversized or multiple production result journal records")
                        def unique_fields(pairs):
                            result = {}
                            for name, value in pairs:
                                if name in result:
                                    raise ValueError("Duplicate production journal/result field")
                                result[name] = value
                            return result
                        entry = json.loads(journal.stdout, object_pairs_hook=unique_fields)
                        boot = uuid.UUID(Path("/proc/sys/kernel/random/boot_id").read_text().strip()).hex
                        if entry.get("_SYSTEMD_UNIT") != unit or entry.get("_SYSTEMD_INVOCATION_ID") != invocation or entry.get("_BOOT_ID") != boot or entry.get("_TRANSPORT") != "stdout" or entry.get("_LINE_BREAK", ""):
                            sys.exit("Production worker journal context refused")
                        payload = entry.get("MESSAGE")
                        if not isinstance(payload, str) or len(payload) > 2048:
                            sys.exit("Invalid production worker result payload")
                        result = json.loads(payload, object_pairs_hook=unique_fields)
                        with private.open("rb") as verified:
                            digest = hashlib.file_digest(verified, "sha256").hexdigest()
                        expected = {"schema": 1, "operationId": operation, "release": "0.1.0~ci", "packageSha256": digest, "packageLength": private.stat().st_size, "contentValid": True, "installAuthorized": False}
                        if result != expected or type(result.get("schema")) is not int or type(result.get("packageLength")) is not int or type(result.get("contentValid")) is not bool or type(result.get("installAuthorized")) is not bool:
                            sys.exit("Production worker result differs from admitted package")
                    print("Source-isolated inspection completed with retained manager identity: " + mode)
                    print("Independent syscall observation: " + json.dumps(syscall_observation, sort_keys=True))
                    break
                time.sleep(0.1)
            else:
                sys.exit("Inspection fixture completion timed out")
        finally:
            subprocess.run(["/usr/bin/systemctl", "stop", unit], timeout=10, check=False)
            subprocess.run(["/usr/bin/systemctl", "reset-failed", unit], timeout=5, check=False)
