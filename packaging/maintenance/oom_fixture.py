"""Disposable Linux CI only: exercise backup OOM policy with a sibling process."""
import configparser
import os
import pathlib
import subprocess
import time

if os.geteuid() != 0 or os.environ.get("HOMENODE_BACKUP_OOM_INTEGRATION") != "1":
    raise SystemExit("requires explicit disposable Linux root fixture")

unit = "homenode-backup-oom-fixture.service"
unit_path = pathlib.Path("/run/systemd/system") / unit
harness = pathlib.Path("/run/homenode-backup-oom-fixture.py")
runtime = pathlib.Path("/run/homenode-backup-oom-fixture")
if any(path.exists() for path in (unit_path, harness, runtime)):
    raise SystemExit("fixture paths are occupied")
source = configparser.ConfigParser(interpolation=None, strict=True)
source.optionxform = str
source.read(pathlib.Path(__file__).resolve().parents[1] / "systemd" / "homenode-backup.service")
required = {"MemoryMax": "1G", "MemorySwapMax": "0", "CPUQuota": "100%", "TasksMax": "64", "OOMPolicy": "kill", "KillMode": "control-group"}
if any(source["Service"].get(key) != value for key, value in required.items()):
    raise SystemExit("backup resource policy differs from fixture contract")
resources = "".join(f"{key}={source['Service'][key]}\n" for key in required)


def command(*args, check=True):
    return subprocess.run(args, check=check, capture_output=True, text=True, timeout=30)


try:
    harness.write_text("""import os, pathlib, time
child = os.fork()
if child == 0:
    while True:
        time.sleep(1)
pathlib.Path('/run/homenode-backup-oom-fixture/sibling').write_text(str(child))
deadline = time.monotonic() + 30
while not pathlib.Path('/run/homenode-backup-oom-fixture/allocate').exists():
    if time.monotonic() >= deadline:
        raise SystemExit('parent did not authorize allocation')
    time.sleep(0.05)
# Allocate and touch more than the service memory ceiling, without relying on
# virtual-address overcommit. Keep each allocation live until the OOM event.
blocks = []
for _ in range(32):
    block = bytearray(64 << 20)
    for offset in range(0, len(block), 4096):
        block[offset] = 1
    blocks.append(block)
raise SystemExit('memory ceiling was not enforced')
""")
    harness.chmod(0o644)
    unit_path.write_text(f"""[Unit]
Description=Disposable HomeNode backup OOM sibling termination test
[Service]
Type=simple
User=1001
Group=1001
ExecStart=/usr/bin/python3 {harness}
RuntimeDirectory=homenode-backup-oom-fixture
RuntimeDirectoryMode=0700
RuntimeDirectoryPreserve=yes
Restart=no
NoNewPrivileges=yes
CapabilityBoundingSet=
AmbientCapabilities=
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
TimeoutStopSec=5
{resources}""")
    unit_path.chmod(0o644)
    command("systemctl", "daemon-reload")
    command("systemctl", "start", unit)
    ready = time.monotonic() + 15
    while not (runtime / "sibling").exists() and time.monotonic() < ready:
        time.sleep(0.1)
    sibling = int((runtime / "sibling").read_text())
    if sibling <= 1 or not pathlib.Path(f"/proc/{sibling}").exists():
        raise RuntimeError("sibling was not alive before memory pressure")
    cgroup = command("systemctl", "show", unit, "--property=ControlGroup", "--value").stdout.strip()
    membership = pathlib.Path(f"/proc/{sibling}/cgroup").read_text().splitlines()
    if not cgroup.startswith("/") or f"0::{cgroup}" not in membership:
        raise RuntimeError("sibling did not inherit service cgroup")
    if command("systemctl", "show", unit, "--property=MemoryMax", "--value").stdout.strip() != "1073741824":
        raise RuntimeError("test service memory ceiling was not loaded")
    kernel_group = pathlib.Path("/sys/fs/cgroup") / cgroup.lstrip("/")
    for name, expected in {"memory.max": "1073741824", "memory.swap.max": "0", "memory.oom.group": "1", "pids.max": "64"}.items():
        if (kernel_group / name).read_text().strip() != expected:
            raise RuntimeError(f"kernel resource control {name} was not applied")
    quota, period = (kernel_group / "cpu.max").read_text().split()
    if quota == "max" or int(quota) <= 0 or int(quota) != int(period):
        raise RuntimeError("kernel CPU quota did not bound service to one CPU")
    (runtime / "allocate").write_text("go")
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        result = command("systemctl", "show", unit, "--property=Result", "--value").stdout.strip()
        if result == "oom-kill":
            break
        phase = command("systemctl", "show", unit, "--property=ActiveState", "--value").stdout.strip()
        if phase in ("failed", "inactive"):
            raise RuntimeError("fixture stopped without an OOM result")
        time.sleep(0.2)
    else:
        raise RuntimeError("bounded service did not report OOM")
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        main_pid = command("systemctl", "show", unit, "--property=MainPID", "--value").stdout.strip()
        phase = command("systemctl", "show", unit, "--property=ActiveState", "--value").stdout.strip()
        if not pathlib.Path(f"/proc/{sibling}").exists() and main_pid == "0" and phase == "failed":
            break
        time.sleep(0.1)
    else:
        raise RuntimeError("worker or sibling survived backup OOM shutdown")
    print("Backup source memory ceiling caused service OOM; sibling process was terminated.")
except Exception:
    # Retain only disposable fixture diagnostics before its unit is removed.
    for arguments in (
        ("systemctl", "show", unit, "--property=Result,ActiveState,SubState,ExecMainStatus,MemoryMax,MemorySwapMax,OOMPolicy,KillMode,ControlGroup"),
        ("journalctl", "-u", unit, "--no-pager", "-n", "50"),
    ):
        try:
            diagnostic = subprocess.run(arguments, check=False, capture_output=True, text=True, timeout=10)
            print(diagnostic.stdout, diagnostic.stderr, flush=True)
        except subprocess.SubprocessError as diagnostic_error:
            print(f"Fixture diagnostic unavailable: {type(diagnostic_error).__name__}", flush=True)
    raise
finally:
    command("systemctl", "stop", unit, check=False)
    unit_path.unlink(missing_ok=True)
    harness.unlink(missing_ok=True)
    command("systemctl", "daemon-reload", check=False)
    command("systemctl", "reset-failed", unit, check=False)
    if runtime.exists():
        (runtime / "sibling").unlink(missing_ok=True)
        (runtime / "allocate").unlink(missing_ok=True)
        runtime.rmdir()
