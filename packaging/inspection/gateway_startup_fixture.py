#!/usr/bin/env python3
"""Actual gateway startup in disposable Linux; daemon/controller are fixtures."""
import argparse
import grp
import os
import pathlib
import pwd
import select
import shutil
import subprocess
import sys
import tempfile
import time

from private_ingress_fixture import run, source_hardening


def ready(process, marker, timeout=12):
    deadline, lines = time.monotonic() + timeout, []
    while time.monotonic() < deadline:
        if select.select([process.stdout], [], [], min(1, max(0, deadline-time.monotonic())))[0]:
            line = process.stdout.readline()
            if not line:
                break
            lines.append(line)
            if marker in line:
                return
    raise RuntimeError(f"process did not reach {marker}: {''.join(lines)}")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--gateway-binary", required=True)
    parser.add_argument("--source-service", required=True)
    args = parser.parse_args()
    if sys.platform != "linux" or os.geteuid() != 0 or os.environ.get("HOMENODE_GATEWAY_STARTUP_FIXTURE") != "1":
        raise SystemExit("requires explicitly opted-in root on disposable Linux")
    names = ("homenode", "homenode-gateway", "homenode-proxy")
    for name in names:
        try:
            grp.getgrnam(name)
        except KeyError:
            pass
        else:
            raise RuntimeError(f"refusing preexisting group {name}")
    for name in names[:2]:
        try:
            pwd.getpwnam(name)
        except KeyError:
            pass
        else:
            raise RuntimeError(f"refusing preexisting account {name}")
    properties = source_hardening(args.source_service)
    users, groups, units, children = [], [], [], []
    namespace = f"hn-gs-{os.getpid()}"
    namespace_created = False
    with tempfile.TemporaryDirectory(prefix="homenode-gateway-startup-") as temporary:
        directory = pathlib.Path(temporary)
        directory.chmod(0o755)
        try:
            for name in names:
                run("groupadd", "--system", name)
                groups.append(name)
            for name in names[:2]:
                run("useradd", "--system", "--no-create-home", "--shell", "/usr/sbin/nologin", "--gid", name, "--groups", "homenode-proxy", name)
                users.append(name)
            controller_uid = pwd.getpwnam("homenode").pw_uid
            controller_gid = grp.getgrnam("homenode").gr_gid
            gateway_gid = grp.getgrnam("homenode-gateway").gr_gid
            bridge_gid = grp.getgrnam("homenode-proxy").gr_gid
            shutil.copyfile(args.gateway_binary, directory / "gateway")
            (directory / "gateway").chmod(0o755)
            tls_directory = directory / "tls"
            tls_directory.mkdir(mode=0o750)
            os.chown(tls_directory, 0, gateway_gid)
            run("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "30", "-subj", "/CN=home.example.ts.net", "-addext", "subjectAltName=DNS:home.example.ts.net", "-addext", "extendedKeyUsage=serverAuth", "-keyout", str(tls_directory / "server.key"), "-out", str(tls_directory / "server.crt"))
            os.chown(tls_directory / "server.key", 0, gateway_gid)
            (tls_directory / "server.key").chmod(0o640)
            (tls_directory / "server.crt").chmod(0o644)
            shutil.copyfile(tls_directory / "server.crt", directory / "ca.crt")
            (directory / "ca.crt").chmod(0o644)
            # Controlled daemon identity only. Never read or replace host
            # enrollment state; this executable is overmounted in the unit.
            daemon = directory / "tailscale"
            daemon.write_text("#!/usr/bin/python3\nimport sys\nif sys.argv[1:] != ['status','--json','--peers=false']: sys.exit(1)\nprint('{\"BackendState\":\"Running\",\"TailscaleIPs\":[\"100.100.1.2\"],\"Self\":{\"DNSName\":\"home.example.ts.net.\"}}')\n")
            daemon.chmod(0o755)
            control = directory / "control"
            control.mkdir(mode=0o750)
            os.chown(control, controller_uid, bridge_gid)
            peer_script = """import socket,os,sys
p=sys.argv[1]
s=socket.socket(socket.AF_UNIX)
s.bind(p)
os.chmod(p,0o660)
s.listen()
print('CONTROLLER-READY',flush=True)
while True:
 c,_=s.accept()
 c.close()
"""
            peer = subprocess.Popen(["setpriv", f"--reuid={controller_uid}", f"--regid={controller_gid}", f"--groups={bridge_gid}", sys.executable, "-u", "-c", peer_script, str(control / "control.sock")], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            children.append(peer)
            ready(peer, "CONTROLLER-READY")
            os.chown(control / "control.sock", controller_uid, bridge_gid)
            run("ip", "netns", "add", namespace)
            namespace_created = True
            run("ip", "-n", namespace, "link", "set", "lo", "up")
            run("ip", "netns", "exec", namespace, "ip", "tuntap", "add", "dev", "tailscale0", "mode", "tun")
            run("ip", "-n", namespace, "address", "add", "100.100.1.2/32", "dev", "tailscale0")
            run("ip", "-n", namespace, "link", "set", "tailscale0", "up")

            def command(case):
                unit = f"homenode-gateway-startup-{os.getpid()}-{case}.service"
                units.append(unit)
                base = "/run/homenode-startup"
                return ["systemd-run", "--quiet", "--pipe", "--wait", "--collect", f"--unit={unit}", "--property=Type=exec", "--property=User=homenode-gateway", "--property=Group=homenode-gateway", "--property=SupplementaryGroups=homenode-proxy", f"--property=NetworkNamespacePath=/run/netns/{namespace}", f"--property=BindReadOnlyPaths={directory}:{base} {daemon}:/usr/bin/tailscale", f"--setenv=SSL_CERT_FILE={base}/ca.crt", *properties, f"{base}/gateway", "--bind", "100.100.1.2", "--port", "8787", "--origin", "https://home.example.ts.net:8787", "--controller-socket", f"{base}/control/control.sock", "--controller-uid", str(controller_uid), "--access-gid", str(bridge_gid), "--tls-cert", f"{base}/tls/server.crt", "--tls-key", f"{base}/tls/server.key"]

            gateway = subprocess.Popen(command("valid"), stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            children.append(gateway)
            ready(gateway, "HomeNode private gateway listening")
            listeners = run("ip", "netns", "exec", namespace, "ss", "-ltn")
            if "100.100.1.2%tailscale0:8787" not in listeners and "100.100.1.2:8787" not in listeners:
                raise RuntimeError("gateway announced readiness without configured TCP listener")
            run("ip", "-n", namespace, "address", "delete", "100.100.1.2/32", "dev", "tailscale0")
            output, _ = gateway.communicate(timeout=15)
            if gateway.returncode == 0 or "private ingress interface changed" not in output:
                raise RuntimeError(f"actual gateway did not exit for readmission after address loss: {output}")
            print("PASS actual gateway named identities, protected TLS load, controller peer admission and TUN-loss exit", flush=True)
            run("ip", "-n", namespace, "address", "add", "100.100.1.2/32", "dev", "tailscale0")
            (tls_directory / "server.key").chmod(0o644)
            denied = subprocess.run(command("bad-key"), capture_output=True, text=True, timeout=15)
            if denied.returncode == 0 or "HomeNode private gateway listening" in denied.stdout + denied.stderr:
                raise RuntimeError("actual gateway exposed listener with world-readable key")
            if "TLS key" not in denied.stdout + denied.stderr:
                raise RuntimeError(f"unsafe key refusal was not reached: {denied.stdout}{denied.stderr}")
            print("PASS actual gateway refuses unsafe protected key before listener startup", flush=True)
        finally:
            for unit in units:
                subprocess.run(["systemctl", "stop", unit], capture_output=True, timeout=15)
            for child in children:
                child.terminate()
                try:
                    child.communicate(timeout=5)
                except subprocess.TimeoutExpired:
                    child.kill()
                    child.communicate(timeout=5)
            if namespace_created:
                run("ip", "netns", "delete", namespace)
            for name in reversed(users):
                run("userdel", name)
            for name in reversed(groups):
                run("groupdel", name)


if __name__ == "__main__":
    main()
