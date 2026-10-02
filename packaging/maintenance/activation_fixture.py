"""Disposable Linux CI fixture; never run on an owner's installation."""
import os
import configparser
import grp
import pathlib
import socket
import struct
import subprocess
import time

if os.geteuid() != 0 or os.environ.get("HOMENODE_ACTIVATION_SYSTEMD_INTEGRATION") != "1":
    raise SystemExit("requires explicit disposable root fixture")

packet_flag = os.environ.get("HOMENODE_ACTIVATION_PACKET", "0")
if packet_flag not in ("0", "1"):
    raise SystemExit("invalid activation transport")
packet = packet_flag == "1"
listener_directive = "ListenSequentialPacket" if packet else "ListenStream"
descriptor_name = "homenode-backup-credential" if packet else "homenode-app-maintenance"
socket_type = socket.SOCK_SEQPACKET if packet else socket.SOCK_STREAM

unit_root = pathlib.Path("/run/systemd/system")
service = "homenode-activation-fixture.service"
socket_unit = "homenode-activation-fixture.socket"
runtime_root = pathlib.Path("/run/homenode-activation-fixture")
socket_path = runtime_root / "apps.sock"
binary = pathlib.Path("/usr/lib/homenode-fixtures/activation.test")
paths = [unit_root / service, unit_root / socket_unit]
if not binary.is_file() or any(path.exists() for path in paths) or runtime_root.exists():
    raise SystemExit("fixture prerequisites or vacancy invalid")


def command(*arguments):
    return subprocess.run(arguments, check=True, capture_output=True, text=True, timeout=30)


group_name = "homenode-activation-fixture"
for lookup, value in ((grp.getgrnam, group_name), (grp.getgrgid, 1003)):
    try:
        lookup(value)
    except KeyError:
        continue
    raise SystemExit("fixture group vacancy invalid")
resource_directives = ""
resource_properties = {}
if packet:
    source = configparser.ConfigParser(interpolation=None, strict=True)
    source.optionxform = str
    source.read(pathlib.Path(__file__).resolve().parents[1] / "systemd" / "homenode-backup.service")
    service_source = source["Service"]
    required = {"MemoryMax": "1G", "MemorySwapMax": "0", "CPUQuota": "100%", "TasksMax": "64", "OOMPolicy": "kill", "KillMode": "control-group"}
    for key, value in required.items():
        if service_source.get(key) != value:
            raise SystemExit("backup resource policy differs from required fixture contract")
    resource_directives = "".join(f"{key}={service_source[key]}\n" for key in required)
    resource_properties = {"MemoryMax": "1073741824", "MemorySwapMax": "0", "CPUQuotaPerSecUSec": "1s", "TasksMax": "64", "OOMPolicy": "kill", "KillMode": "control-group"}

group_created = False
try:
    command("/usr/sbin/groupadd", "--gid", "1003", group_name)
    group_created = True
    paths[0].write_text(f"""[Unit]
Description=Disposable HomeNode inherited listener test
Requires={socket_unit}
After={socket_unit}
[Service]
Type=simple
User=1001
Group=1001
Sockets={socket_unit}
ExecStart={binary} -test.run=^TestNativeInheritedPrivateListener$ -test.v
Environment=HOMENODE_ACTIVATION_CHILD=1 HOMENODE_ACTIVATION_SYSTEMD=1 HOMENODE_ACTIVATION_PATH={socket_path} HOMENODE_ACTIVATION_PACKET={packet_flag}
NoNewPrivileges=yes
CapabilityBoundingSet=
AmbientCapabilities=
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
RestrictAddressFamilies=AF_UNIX
TimeoutStartSec=20
{resource_directives}""")
    paths[1].write_text(f"""[Unit]
Description=Disposable HomeNode root-created listener
[Socket]
{listener_directive}={socket_path}
FileDescriptorName={descriptor_name}
SocketUser=root
SocketGroup={group_name}
SocketMode=0660
DirectoryMode=0755
Service={service}
RemoveOnStop=yes
""")
    for path in paths:
        path.chmod(0o644)
    command("systemctl", "daemon-reload")
    command("systemctl", "start", socket_unit)
    for key, expected in resource_properties.items():
        effective = command("systemctl", "show", service, f"--property={key}", "--value").stdout.strip()
        if effective != expected:
            raise RuntimeError(f"backup resource property {key} was not applied")
    with socket.socket(socket.AF_UNIX, socket_type) as connection:
        connection.settimeout(15)
        connection.connect(str(socket_path))
        _, uid, _ = struct.unpack("3i", connection.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12))
        if uid != 0:
            raise RuntimeError("listener did not retain root credentials")
        output = bytearray()
        while True:
            data = connection.recv(64)
            if not data:
                break
            output.extend(data)
            if len(output) > 64:
                raise RuntimeError("unexpected fixture response size")
        if output != b"controller-1001":
            raise RuntimeError("unprivileged activated service did not respond")
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        phase = command("systemctl", "show", service, "--property=ActiveState", "--value").stdout.strip()
        if phase not in ("activating", "active", "deactivating"):
            break
        time.sleep(0.1)
    else:
        raise RuntimeError("activated service did not finish")
    result = command("systemctl", "show", service, "--property=ExecMainStatus", "--value").stdout.strip()
    if phase != "inactive" or result != "0":
        raise RuntimeError("activated service failed")
    if packet:
        print("Backup source CPU/memory/swap/tasks/OOM/group termination directives match loaded systemd properties; OOM enforcement is not exercised.")
    print(f"Root-created named {'packet' if packet else 'stream'} socket activated UID 1001; filesystem access denied; inherited accept succeeded.")
except Exception:
    for arguments in (("systemctl", "status", service, socket_unit, "--no-pager", "--full"), ("journalctl", "-u", service, "-u", socket_unit, "--no-pager", "-n", "80")):
        result = subprocess.run(arguments, check=False, capture_output=True, text=True, timeout=30)
        print(result.stdout, result.stderr, flush=True)
    raise
finally:
    subprocess.run(["systemctl", "stop", service, socket_unit], check=False, capture_output=True, timeout=30)
    for path in paths:
        path.unlink(missing_ok=True)
    subprocess.run(["systemctl", "daemon-reload"], check=False, capture_output=True, timeout=30)
    subprocess.run(["systemctl", "reset-failed", service, socket_unit], check=False, capture_output=True, timeout=30)
    if runtime_root.exists():
        runtime_root.rmdir()
    if group_created:
        command("/usr/sbin/groupdel", group_name)
