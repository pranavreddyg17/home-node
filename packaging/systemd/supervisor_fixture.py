#!/usr/bin/env python3
"""Opt-in disposable CI runtime checks for supervisor service protections."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import uuid

if sys.platform != "linux" or os.geteuid() != 0 or os.getenv("HOMENODE_SUPERVISOR_SYSTEMD_INTEGRATION") != "1":
    sys.exit("Requires explicit disposable Linux root fixture")
allowed = {"Type", "User", "Group", "UMask", "TimeoutStopSec", "NoNewPrivileges", "CapabilityBoundingSet", "AmbientCapabilities", "ProtectSystem", "ProtectHome", "PrivateTmp", "ProtectKernelTunables", "ProtectKernelModules", "ProtectControlGroups", "RestrictSUIDSGID", "LockPersonality", "RestrictRealtime", "RestrictNamespaces", "RestrictAddressFamilies", "ReadWritePaths", "InaccessiblePaths", "MemoryMax", "TasksMax"}
adapted = {"EnvironmentFile", "ExecStart", "RuntimeDirectory", "RuntimeDirectoryMode", "StateDirectory", "StateDirectoryMode", "Restart", "RestartSec"}
properties, seen, section = [], set(), ""
for line in Path(__file__).with_name("homenode-supervisor.service").read_text().splitlines():
    line = line.strip()
    if not line or line.startswith("#"):
        continue
    if line.startswith("["):
        section = line
        continue
    if section != "[Service]":
        continue
    key, separator, value = line.partition("=")
    if not separator or key in seen or key not in allowed | adapted:
        sys.exit("Unreviewed supervisor fixture source")
    seen.add(key)
    if key in allowed:
        properties.append("--property=" + key + "=" + value)
if seen != allowed | adapted:
    sys.exit("Incomplete supervisor fixture source")
memory = os.getenv("HOMENODE_SUPERVISOR_MEMORY_PID")
memory_environment = []
if memory is not None:
    for key in ("HOMENODE_SUPERVISOR_MEMORY_PID", "HOMENODE_SUPERVISOR_MEMORY_ID", "HOMENODE_SUPERVISOR_MEMORY_MAX"):
        value = os.environ.get(key, "")
        if not value or len(value) > 64 or not value.isalnum():
            sys.exit("Invalid memory observation fixture input")
        memory_environment.append("--setenv=" + key + "=" + value)
created, markers = [], []

def create(path, exclusive=False):
    path = Path(path)
    if path.exists():
        if exclusive or path.is_symlink() or not path.is_dir() or path.stat().st_uid != 0 or path.stat().st_mode & 0o022:
            raise RuntimeError("Existing fixture path refused")
        return
    create(path.parent)
    path.mkdir(mode=0o755)
    created.append(path)

try:
    for directory in ("/var/lib/homenode/supervisor", "/var/lib/homenode/images", "/var/lib/homenode/volumes", "/run/homenode", "/var/lib/homenode/control", "/etc/homenode/tls"):
        create(directory, exclusive=True)
    for directory in ("/var/lib/homenode/control", "/etc/homenode/tls"):
        marker = Path(directory) / "fixture-secret"
        with marker.open("x") as file:
            file.write("not a real secret")
        marker.chmod(0o600)
        markers.append(marker)
    with tempfile.TemporaryDirectory(prefix="homenode-supervisor-private-tmp-") as temporary:
        hidden = Path(temporary) / "marker"
        hidden.write_text("fixture")
        command = ["/usr/bin/systemd-run", "--quiet", "--wait", "--pipe", "--collect", "--unit=homenode-supervisor-fixture-" + uuid.uuid4().hex,
                   "--property=Restart=no", *properties, *memory_environment,
                   "--setenv=HOMENODE_VOLUME_INTEGRATION=1", "--setenv=HOMENODE_GUEST_UID_ACCOUNTS_INTEGRATION=1", "--setenv=HOMENODE_SUPERVISOR_SOURCE_FIXTURE=1",
                   "--setenv=HOMENODE_SUPERVISOR_VOLUME_PARENT=/var/lib/homenode/volumes",
                   "--setenv=HOMENODE_SUPERVISOR_HIDDEN_PATH=" + str(hidden),
                   "/usr/lib/homenode-fixtures/supervisor.test",
                   "-test.run=^TestNativeSupervisorMemoryObservation$" if memory is not None else "-test.run=^(TestNative(FreshVolumeFormattingPreservesExistingData|PreparedVolumeCleanup|VolumePublicationIdentity|GuestUIDVolumeAdmission|GuestChannelDirectoryOwnership|GuestNSSNameServiceEligibility|GuestAutomaticUIDAllocationEligibility|SupervisorServiceIsolation)|TestReadOnlyComponentOpenRetainsParentAndRefusesLinks|TestGuestMemoryDomainRefusesSyntheticFilesystem)$", "-test.count=1"]
        if subprocess.run(command, timeout=60, check=False).returncode:
            sys.exit("Supervisor source protection fixture failed")
finally:
    for marker in reversed(markers):
        marker.unlink()
    for directory in reversed(created):
        directory.rmdir()
