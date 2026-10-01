"""Disposable Linux CI fixture; never run on an owner's installation."""
import os
import pathlib
import socket
import struct
import subprocess
import time

if os.geteuid() != 0 or os.environ.get("HOMENODE_ACTIVATION_SYSTEMD_INTEGRATION") != "1":
    raise SystemExit("requires explicit disposable root fixture")

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


try:
    paths[0].write_text(f"""[Unit]
Description=Disposable HomeNode inherited listener test
Requires={socket_unit}
After={socket_unit}
[Service]
Type=oneshot
User=1001
Group=1001
Sockets={socket_unit}
ExecStart={binary} -test.run=^TestNativeInheritedPrivateListener$ -test.v
Environment=HOMENODE_ACTIVATION_CHILD=1 HOMENODE_ACTIVATION_SYSTEMD=1 HOMENODE_ACTIVATION_PATH={socket_path}
NoNewPrivileges=yes
CapabilityBoundingSet=
AmbientCapabilities=
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
RestrictAddressFamilies=AF_UNIX
TimeoutStartSec=20
""")
    paths[1].write_text(f"""[Unit]
Description=Disposable HomeNode root-created listener
[Socket]
ListenStream={socket_path}
FileDescriptorName=homenode-app-maintenance
SocketUser=root
SocketGroup=1003
SocketMode=0660
DirectoryMode=0755
Service={service}
RemoveOnStop=yes
""")
    for path in paths:
        path.chmod(0o644)
    command("systemctl", "daemon-reload")
    command("systemctl", "start", socket_unit)
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as connection:
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
    print("Root-created named socket activated UID 1001; filesystem access denied; inherited accept succeeded.")
finally:
    subprocess.run(["systemctl", "stop", service, socket_unit], check=False, capture_output=True, timeout=30)
    for path in paths:
        path.unlink(missing_ok=True)
    subprocess.run(["systemctl", "daemon-reload"], check=False, capture_output=True, timeout=30)
    subprocess.run(["systemctl", "reset-failed", service, socket_unit], check=False, capture_output=True, timeout=30)
    if runtime_root.exists():
        runtime_root.rmdir()
