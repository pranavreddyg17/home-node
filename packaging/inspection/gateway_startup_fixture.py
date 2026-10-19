#!/usr/bin/env python3
"""Actual gateway/controller startup in disposable Linux; daemon is a fixture."""
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
    parser.add_argument("--controller-binary", required=True)
    parser.add_argument("--source-service", required=True)
    args = parser.parse_args()
    if sys.platform != "linux" or os.geteuid() != 0 or os.environ.get("HOMENODE_GATEWAY_STARTUP_FIXTURE") != "1":
        raise SystemExit("requires explicitly opted-in root on disposable Linux")
    names = ("homenode", "homenode-gateway", "homenode-proxy", "homenode-runtime")
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
    peer_namespace = f"hn-gp-{os.getpid()}"
    namespace_created = False
    peer_namespace_created = False
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
            shutil.copyfile(args.controller_binary, directory / "controller")
            (directory / "controller").chmod(0o755)
            for name in ("state", "data"):
                private = directory / name
                private.mkdir(mode=0o700)
                os.chown(private, controller_uid, controller_gid)
            (directory / "web").mkdir(mode=0o755)
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
            peer = subprocess.Popen(["setpriv", f"--reuid={controller_uid}", f"--regid={controller_gid}", f"--groups={bridge_gid}", str(directory / "controller"), "serve", "--controller-socket", str(control / "control.sock"), "--gateway-uid", str(pwd.getpwnam("homenode-gateway").pw_uid), "--access-gid", str(bridge_gid), "--bind", "100.100.1.2", "--origin", "https://home.example.ts.net:8787", "--state-dir", str(directory / "state"), "--data-root", str(directory / "data"), "--web-dir", str(directory / "web")], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            children.append(peer)
            ready(peer, "HomeNode listening")
            os.chown(control / "control.sock", controller_uid, bridge_gid)
            run("ip", "netns", "add", namespace)
            namespace_created = True
            run("ip", "-n", namespace, "link", "set", "lo", "up")
            run("ip", "netns", "exec", namespace, "ip", "tuntap", "add", "dev", "tailscale0", "mode", "tun")
            run("ip", "-n", namespace, "address", "add", "100.100.1.2/32", "dev", "tailscale0")
            run("ip", "-n", namespace, "link", "set", "tailscale0", "up")
            run("ip", "netns", "add", peer_namespace)
            peer_namespace_created = True
            run("ip", "-n", peer_namespace, "link", "set", "lo", "up")
            run("ip", "netns", "exec", peer_namespace, "ip", "tuntap", "add", "dev", "tailscale0", "mode", "tun")
            run("ip", "-n", peer_namespace, "address", "add", "100.100.1.3/32", "dev", "tailscale0")
            run("ip", "-n", peer_namespace, "link", "set", "tailscale0", "up")
            run("ip", "-n", namespace, "link", "add", "name", "wire0", "type", "veth", "peer", "name", "wire1", "netns", peer_namespace)
            for ns, interface, last, destination in ((namespace, "wire0", "1", "100.100.1.3"), (peer_namespace, "wire1", "2", "100.100.1.2")):
                run("ip", "-n", ns, "link", "set", "tailscale0", "mtu", "1280")
                run("ip", "-n", ns, "address", "add", f"10.212.0.{last}/30", "dev", interface)
                run("ip", "-n", ns, "link", "set", interface, "up")
                run("ip", "-n", ns, "route", "add", f"{destination}/32", "dev", "tailscale0")
            # Test transport only: copy IPv4 frames between two kernel TUNs
            # over an isolated veth carrier. No production VPN implementation
            # or real tailnet enrollment is supplied by this helper.
            relay_script = """import fcntl,os,select,socket,struct,sys
tun=os.open('/dev/net/tun',os.O_RDWR)
fcntl.ioctl(tun,0x400454ca,struct.pack('16sH22x',b'tailscale0',0x1001))
udp=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
udp.bind((sys.argv[1],28991))
udp.connect((sys.argv[2],28991))
print('RELAY-READY',flush=True)
while True:
 readable,_,_=select.select([tun,udp],[],[],1)
 if tun in readable:
  packet=os.read(tun,65535)
  if packet and packet[0]>>4==4: udp.send(packet)
 if udp in readable:
  packet=udp.recv(65535)
  if packet and packet[0]>>4==4: os.write(tun,packet)
"""
            for ns, local, remote in ((namespace, "10.212.0.1", "10.212.0.2"), (peer_namespace, "10.212.0.2", "10.212.0.1")):
                relay = subprocess.Popen(["ip", "netns", "exec", ns, sys.executable, "-u", "-c", relay_script, local, remote], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
                children.append(relay)
                ready(relay, "RELAY-READY")

            def command(case):
                unit = f"homenode-gateway-startup-{os.getpid()}-{case}.service"
                units.append(unit)
                base = "/run/homenode-startup"
                return ["systemd-run", "--quiet", "--pipe", "--wait", "--collect", f"--unit={unit}", "--property=Type=exec", "--property=User=homenode-gateway", "--property=Group=homenode-gateway", "--property=SupplementaryGroups=homenode-proxy", f"--property=NetworkNamespacePath=/run/netns/{namespace}", f"--property=BindReadOnlyPaths={directory}:{base} {daemon}:/usr/bin/tailscale", f"--setenv=SSL_CERT_FILE={base}/ca.crt", *properties, "/usr/bin/env", f"{base}/gateway", "--bind", "100.100.1.2", "--port", "8787", "--origin", "https://home.example.ts.net:8787", "--controller-socket", f"{base}/control/control.sock", "--controller-uid", str(controller_uid), "--access-gid", str(bridge_gid), "--tls-cert", f"{base}/tls/server.crt", "--tls-key", f"{base}/tls/server.key"]

            gateway = subprocess.Popen(command("valid"), stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            children.append(gateway)
            ready(gateway, "HomeNode private gateway listening")
            listeners = run("ip", "netns", "exec", namespace, "ss", "-ltn")
            if "100.100.1.2%tailscale0:8787" not in listeners and "100.100.1.2:8787" not in listeners:
                raise RuntimeError("gateway announced readiness without configured TCP listener")
            client_script = """import http.client,socket,ssl,sys
context=ssl.create_default_context(cafile=sys.argv[1])
context.minimum_version=context.maximum_version=ssl.TLSVersion.TLSv1_3
for path,host,expected in [('/api/v1/healthz','home.example.ts.net:8787',200),('/api/v1/files','home.example.ts.net:8787',401),('/api/v1/healthz','foreign.example.ts.net:8787',403)]:
 with socket.create_connection(('100.100.1.2',8787),timeout=5) as raw:
  with context.wrap_socket(raw,server_hostname='home.example.ts.net') as connection:
   connection.sendall(('GET '+path+' HTTP/1.1\\r\\nHost: '+host+'\\r\\nConnection: close\\r\\n\\r\\n').encode())
   response=http.client.HTTPResponse(connection)
   response.begin()
   assert response.status==expected,(path,response.status,expected)
   response.read(65536)
   print('HTTPS-PASS',path,expected,flush=True)
for name,hostname,client_context in [('wrong-hostname','wrong.example.ts.net',context),('untrusted-root','home.example.ts.net',ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT))]:
 try:
  with socket.create_connection(('100.100.1.2',8787),timeout=5) as raw:
   with client_context.wrap_socket(raw,server_hostname=hostname):
    raise AssertionError('invalid certificate accepted: '+name)
 except ssl.SSLCertVerificationError:
  print('TLS-REFUSAL-PASS',name,flush=True)
old=ssl.create_default_context(cafile=sys.argv[1])
old.minimum_version=old.maximum_version=ssl.TLSVersion.TLSv1_2
try:
 with socket.create_connection(('100.100.1.2',8787),timeout=5) as raw:
  with old.wrap_socket(raw,server_hostname='home.example.ts.net'):
   raise AssertionError('TLS 1.2 accepted')
except ssl.SSLError as error:
 assert error.reason=='TLSV1_ALERT_PROTOCOL_VERSION',error
 print('TLS-REFUSAL-PASS','TLS-1.2',flush=True)
"""
            results = run("ip", "netns", "exec", peer_namespace, sys.executable, "-c", client_script, str(directory / "ca.crt"))
            if results.count("HTTPS-PASS") != 3:
                raise RuntimeError(f"actual HTTPS controller checks incomplete: {results}")
            if results.count("TLS-REFUSAL-PASS") != 3:
                raise RuntimeError(f"actual TLS refusal checks incomplete: {results}")
            print("PASS actual TLS 1.3 over kernel TUN through gateway/controller executables: health, unauthenticated Files refusal and foreign Host refusal", flush=True)
            print("PASS actual TLS refuses untrusted root, wrong certificate hostname and TLS 1.2 over kernel TUN", flush=True)
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
            (tls_directory / "server.key").chmod(0o640)
            peer.terminate()
            peer.communicate(timeout=5)
            (control / "control.sock").unlink(missing_ok=True)
            # Root serves a replacement endpoint but labels it with the
            # expected controller ownership. Filesystem labels cannot replace
            # the serving process identity returned by SO_PEERCRED.
            foreign_peer = subprocess.Popen([sys.executable, "-u", "-c", peer_script, str(control / "control.sock")], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            children.append(foreign_peer)
            ready(foreign_peer, "CONTROLLER-READY")
            os.chown(control / "control.sock", controller_uid, bridge_gid)
            denied = subprocess.run(command("foreign-peer"), capture_output=True, text=True, timeout=15)
            if denied.returncode == 0 or "HomeNode private gateway listening" in denied.stdout + denied.stderr:
                raise RuntimeError("actual gateway exposed listener for foreign serving peer")
            if "private controller endpoint is not ready or its identity differs" not in denied.stdout + denied.stderr:
                raise RuntimeError(f"foreign kernel peer refusal was not reached: {denied.stdout}{denied.stderr}")
            print("PASS actual gateway refuses foreign controller kernel peer despite expected socket ownership", flush=True)
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
            if peer_namespace_created:
                run("ip", "netns", "delete", peer_namespace)
            for name in reversed(users):
                run("userdel", name)
            for name in reversed(groups):
                # userdel may remove an empty same-named private group.
                try:
                    grp.getgrnam(name)
                except KeyError:
                    continue
                run("groupdel", name)


if __name__ == "__main__":
    main()
