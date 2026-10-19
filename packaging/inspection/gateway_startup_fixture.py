#!/usr/bin/env python3
"""Actual gateway/controller startup in disposable Linux; daemon is a fixture."""
import argparse
import hashlib
import grp
import json
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


def observe_service(unit, binary, uid, gid):
    pid = int(run("systemctl", "show", unit, "--property=MainPID", "--value").strip())
    if pid <= 1:
        raise RuntimeError("native service has no running main process")
    expected, actual = os.stat(binary), os.stat(f"/proc/{pid}/exe")
    if (expected.st_dev, expected.st_ino) != (actual.st_dev, actual.st_ino):
        raise RuntimeError("native service is not executing the copied application binary")
    fields = dict(line.split(":", 1) for line in pathlib.Path(f"/proc/{pid}/status").read_text().splitlines() if ":" in line)
    if int(fields["Uid"].split()[1]) != uid or int(fields["Gid"].split()[1]) != gid:
        raise RuntimeError("native service kernel identity differs from named account")
    for name in ("CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb"):
        if fields[name].strip() != "0000000000000000":
            raise RuntimeError(f"native service retained {name}")
    if fields["NoNewPrivs"].strip() != "1" or fields["Seccomp"].strip() != "2":
        raise RuntimeError("native service lacks kernel no-new-privileges or seccomp filtering")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--gateway-binary", required=True)
    parser.add_argument("--controller-binary", required=True)
    parser.add_argument("--identity-client-binary", required=True)
    parser.add_argument("--web-dist", required=True)
    parser.add_argument("--source-service", required=True)
    parser.add_argument("--controller-source-service", required=True)
    args = parser.parse_args()
    if sys.platform != "linux" or os.geteuid() != 0 or os.environ.get("HOMENODE_GATEWAY_STARTUP_FIXTURE") != "1":
        raise SystemExit("requires explicitly opted-in root on disposable Linux")
    web_dist = pathlib.Path(args.web_dist).resolve(strict=True)
    web_manifest = {}
    for asset in web_dist.rglob("*"):
        if asset.is_symlink():
            raise RuntimeError("fixture frontend must not contain symlinks")
        if asset.is_file():
            if asset.stat().st_size > 8 << 20 or len(web_manifest) >= 512:
                raise RuntimeError("fixture frontend exceeds serving bounds")
            web_manifest["/" + asset.relative_to(web_dist).as_posix()] = hashlib.sha256(asset.read_bytes()).hexdigest()
    if "/index.html" not in web_manifest:
        raise RuntimeError("built frontend index missing")
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
    controller_properties = source_hardening(args.controller_source_service)
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
            runtime_gid = grp.getgrnam("homenode-runtime").gr_gid
            shutil.copyfile(args.gateway_binary, directory / "gateway")
            (directory / "gateway").chmod(0o755)
            shutil.copyfile(args.controller_binary, directory / "controller")
            (directory / "controller").chmod(0o755)
            shutil.copyfile(args.identity_client_binary, directory / "identity-client.test")
            (directory / "identity-client.test").chmod(0o755)
            for name in ("state", "data"):
                private = directory / name
                private.mkdir(mode=0o700)
                os.chown(private, controller_uid, controller_gid)
            shutil.copytree(web_dist, directory / "web")
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
            controller_unit = f"homenode-controller-startup-{os.getpid()}.service"
            units.append(controller_unit)
            controller_base = "/run/homenode-startup"
            controller_command = ["systemd-run", "--quiet", "--pipe", "--wait", "--collect", f"--unit={controller_unit}", "--property=Type=exec", "--property=User=homenode", "--property=Group=homenode", "--property=SupplementaryGroups=homenode-proxy homenode-runtime", f"--property=BindPaths={directory}:{controller_base}", f"--property=ReadWritePaths={controller_base}/state {controller_base}/data {controller_base}/control", *controller_properties, "/usr/bin/env", f"{controller_base}/controller", "serve", "--controller-socket", f"{controller_base}/control/control.sock", "--gateway-uid", str(pwd.getpwnam("homenode-gateway").pw_uid), "--access-gid", str(bridge_gid), "--bind", "100.100.1.2", "--origin", "https://home.example.ts.net:8787", "--state-dir", f"{controller_base}/state", "--data-root", f"{controller_base}/data", "--web-dir", f"{controller_base}/web"]
            peer = subprocess.Popen(controller_command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            children.append(peer)
            ready(peer, "HomeNode listening")
            observe_service(controller_unit, directory / "controller", controller_uid, controller_gid)
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
            observe_service(units[-1], directory / "gateway", pwd.getpwnam("homenode-gateway").pw_uid, gateway_gid)
            print("PASS actual gateway/controller kernel identities, zero capabilities, no-new-privileges and seccomp observed", flush=True)
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
            frontend_client = """import hashlib,html.parser,http.client,json,socket,ssl,sys
expected=json.load(sys.stdin)
context=ssl.create_default_context(cafile=sys.argv[1])
context.minimum_version=ssl.TLSVersion.TLSv1_3
def fetch(path):
 with socket.create_connection(('100.100.1.2',8787),timeout=5) as raw:
  with context.wrap_socket(raw,server_hostname='home.example.ts.net') as connection:
   connection.sendall(('GET '+path+' HTTP/1.1\\r\\nHost: home.example.ts.net:8787\\r\\nConnection: close\\r\\n\\r\\n').encode())
   response=http.client.HTTPResponse(connection)
   response.begin()
   assert response.status==200,(path,response.status)
   data=response.read((8<<20)+1)
   assert len(data)<=8<<20
   assert hashlib.sha256(data).hexdigest()==expected[path if path!='/' else '/index.html']
   assert response.getheader('X-Content-Type-Options')=='nosniff'
   assert response.getheader('Strict-Transport-Security')
   assert response.getheader('X-Frame-Options')=='DENY'
   csp=response.getheader('Content-Security-Policy','')
   for directive in [\"default-src 'none'\",\"script-src 'self'\",\"connect-src 'self'\",\"frame-ancestors 'none'\",\"base-uri 'none'\"]: assert directive in csp
   assert \"'unsafe-inline'\" not in csp and \"'unsafe-eval'\" not in csp
   cache=response.getheader('Cache-Control')
   assert cache==('no-store' if path=='/' else 'public, max-age=31536000, immutable')
   return data,response.getheader('Content-Type','').split(';')[0]
class Assets(html.parser.HTMLParser):
 def __init__(self): super().__init__(); self.scripts=[]; self.styles=[]
 def handle_starttag(self,tag,attributes):
  a=dict(attributes)
  if tag=='script' and a.get('src'): self.scripts.append(a['src'])
  if tag=='link' and a.get('rel')=='stylesheet': self.styles.append(a['href'])
index,content_type=fetch('/')
assert content_type=='text/html'
assets=Assets(); assets.feed(index.decode())
assert assets.scripts and assets.styles
for path in assets.scripts+assets.styles:
 assert path.startswith('/assets/') and path in expected
 _,kind=fetch(path)
 assert kind in (('text/javascript','application/javascript') if path in assets.scripts else ('text/css',))
print('FRONTEND-HTTPS-PASS',flush=True)
"""
            frontend_result = subprocess.run(["ip", "netns", "exec", peer_namespace, sys.executable, "-c", frontend_client, str(directory / "ca.crt")], input=json.dumps(web_manifest), capture_output=True, text=True, timeout=30)
            if frontend_result.returncode != 0 or "FRONTEND-HTTPS-PASS" not in frontend_result.stdout:
                raise RuntimeError(f"built frontend HTTPS delivery failed: {frontend_result.stdout}{frontend_result.stderr}")
            print("PASS built frontend HTML/JS/CSS hashes, CSP, HTTPS and cache policy through actual gateway/controller", flush=True)
            setup_output = run("setpriv", f"--reuid={controller_uid}", f"--regid={controller_gid}", f"--groups={bridge_gid}", str(directory / "controller"), "setup-code", "--state-dir", str(directory / "state"))
            setup_code = setup_output.strip().splitlines()[-1]
            identity_input = json.dumps({"RootPEM": str(directory / "ca.crt"), "SetupCode": setup_code})
            identity_client = subprocess.run(["ip", "netns", "exec", peer_namespace, "env", "HOMENODE_NATIVE_HTTPS_IDENTITY=1", str(directory / "identity-client.test"), "-test.run=^TestNativeHTTPSIdentityClient$", "-test.timeout=40s", "-test.v"], input=identity_input, capture_output=True, text=True, timeout=45)
            if identity_client.returncode != 0 or "--- PASS: TestNativeHTTPSIdentityClient" not in identity_client.stdout:
                raise RuntimeError(f"native HTTPS identity protocol failed: {identity_client.stdout}{identity_client.stderr}")
            print("PASS actual HTTPS identity: registration, signed approval, limited pairing, reused-TLS revocation and replacement recovery", flush=True)
            control_tls_script = """import socket,ssl,sys
context=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.minimum_version=ssl.TLSVersion.TLSv1_3
context.load_cert_chain(sys.argv[1],sys.argv[2])
server=socket.socket()
server.bind(('100.100.1.2',8788))
server.listen()
print('CONTROL-TLS-READY',flush=True)
while True:
 raw,_=server.accept()
 with context.wrap_socket(raw,server_side=True) as connection:
  connection.settimeout(3)
  connection.recv(4096)
  connection.sendall(b'HTTP/1.1 200 OK\\r\\nContent-Length: 7\\r\\nConnection: close\\r\\n\\r\\ncontrol')
"""
            control_tls = subprocess.Popen(["ip", "netns", "exec", namespace, sys.executable, "-u", "-c", control_tls_script, str(tls_directory / "server.crt"), str(tls_directory / "server.key")], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            children.append(control_tls)
            ready(control_tls, "CONTROL-TLS-READY")
            for ns, interface in ((namespace, "wire0"), (peer_namespace, "wire1")):
                run("ip", "netns", "exec", ns, "sysctl", "-qw", "net.ipv4.conf.all.rp_filter=0", f"net.ipv4.conf.{interface}.rp_filter=0", "net.ipv4.conf.tailscale0.rp_filter=0")
            run("ip", "-n", peer_namespace, "route", "replace", "100.100.1.2/32", "via", "10.212.0.1", "dev", "wire1", "src", "100.100.1.3")
            carrier_client = """import http.client,socket,ssl,sys
context=ssl.create_default_context(cafile=sys.argv[1])
context.minimum_version=ssl.TLSVersion.TLSv1_3
def control():
 with socket.create_connection(('100.100.1.2',8788),timeout=3,source_address=('100.100.1.3',0)) as raw:
  with context.wrap_socket(raw,server_hostname='home.example.ts.net') as connection:
   connection.sendall(b'GET / HTTP/1.1\\r\\nHost: home.example.ts.net:8788\\r\\nConnection: close\\r\\n\\r\\n')
   response=http.client.HTTPResponse(connection)
   response.begin()
   assert response.status==200 and response.read(64)==b'control'
control()
try:
 with socket.create_connection(('100.100.1.2',8787),timeout=2,source_address=('100.100.1.3',0)) as raw:
  with context.wrap_socket(raw,server_hostname='home.example.ts.net'):
   raise AssertionError('carrier CGNAT path reached actual gateway TLS')
except (TimeoutError,ConnectionRefusedError,ConnectionResetError):
 pass
control()
print('CARRIER-REFUSED-WITH-CONTROLS',flush=True)
"""
            carrier_result = run("ip", "netns", "exec", peer_namespace, sys.executable, "-c", carrier_client, str(directory / "ca.crt"))
            if "CARRIER-REFUSED-WITH-CONTROLS" not in carrier_result or gateway.poll() is not None or control_tls.poll() is not None:
                raise RuntimeError("carrier refusal did not preserve live gateway and positive TLS controls")
            run("ip", "-n", peer_namespace, "route", "replace", "100.100.1.2/32", "dev", "tailscale0")
            print("PASS actual gateway refuses carrier CGNAT ingress while unbound TLS control works before and after", flush=True)
            run("ip", "-n", namespace, "address", "delete", "100.100.1.2/32", "dev", "tailscale0")
            output, _ = gateway.communicate(timeout=15)
            if gateway.returncode == 0 or "private ingress interface changed" not in output:
                raise RuntimeError(f"actual gateway did not exit for readmission after address loss: {output}")
            print("PASS actual gateway named identities, protected TLS load, controller peer admission and TUN-loss exit", flush=True)
            run("ip", "-n", namespace, "address", "add", "100.100.1.2/32", "dev", "tailscale0")
            run("groupmod", "--non-unique", "--gid", str(gateway_gid), "homenode-runtime")
            try:
                alias_gateway = subprocess.run(command("aliased-groups"), capture_output=True, text=True, timeout=15)
                if alias_gateway.returncode == 0 or "gateway requires its dedicated Linux identity" not in alias_gateway.stdout + alias_gateway.stderr:
                    raise RuntimeError("gateway did not refuse aliased runtime/private key group at identity admission")
                alias_unit = f"homenode-controller-alias-{os.getpid()}.service"
                units.append(alias_unit)
                alias_command = [f"--unit={alias_unit}" if argument.startswith("--unit=") else argument for argument in controller_command]
                alias_controller = subprocess.run(alias_command, capture_output=True, text=True, timeout=15)
                if alias_controller.returncode == 0 or "invalid dedicated controller proxy configuration" not in alias_controller.stdout + alias_controller.stderr:
                    raise RuntimeError("controller did not refuse aliased runtime/private key group at identity admission")
            finally:
                run("groupmod", "--gid", str(runtime_gid), "homenode-runtime")
            print("PASS actual gateway/controller refuse aliased NSS runtime and private TLS-key groups", flush=True)
            (tls_directory / "server.key").chmod(0o644)
            denied = subprocess.run(command("bad-key"), capture_output=True, text=True, timeout=15)
            if denied.returncode == 0 or "HomeNode private gateway listening" in denied.stdout + denied.stderr:
                raise RuntimeError("actual gateway exposed listener with world-readable key")
            if "TLS key" not in denied.stdout + denied.stderr:
                raise RuntimeError(f"unsafe key refusal was not reached: {denied.stdout}{denied.stderr}")
            print("PASS actual gateway refuses unsafe protected key before listener startup", flush=True)
            (tls_directory / "server.key").chmod(0o640)
            run("systemctl", "stop", controller_unit)
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
