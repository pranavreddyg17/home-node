#!/usr/bin/env python3
"""Destructive networking is confined to fresh disposable Linux namespaces.

Run only in disposable CI with an explicitly opted-in, compiled Go test binary.
The veth overlay surrogate proves socket ingress enforcement, not Tailscale.
"""
import argparse
import json
import os
import pathlib
import platform
import select
import shutil
import subprocess
import sys
import tempfile


def run(*args):
    return subprocess.run(args, check=True, capture_output=True, text=True, timeout=15).stdout


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--test-binary", required=True)
    args = parser.parse_args()
    if sys.platform != "linux" or os.geteuid() != 0 or os.environ.get("HOMENODE_PRIVATE_INGRESS_FIXTURE") != "1":
        raise SystemExit("requires explicitly opted-in root on disposable Linux")
    release = platform.freedesktop_os_release()
    print("ENV " + json.dumps({"kernel": platform.release(), "architecture": platform.machine(), "distribution": release.get("ID"), "distributionVersion": release.get("VERSION_ID"), "python": platform.python_version(), "iproute": run("ip", "-Version").strip()}, sort_keys=True), flush=True)
    server, peer = f"hn-is-{os.getpid()}", f"hn-ip-{os.getpid()}"
    namespaces, children, retired = [], [], set()
    with tempfile.TemporaryDirectory(prefix="homenode-ingress-") as temporary:
        directory = pathlib.Path(temporary)
        directory.chmod(0o755)
        binary = directory / "networkcheck.test"
        shutil.copyfile(args.test_binary, binary)
        binary.chmod(0o755)
        try:
            for namespace in (server, peer):
                run("ip", "netns", "add", namespace)
                namespaces.append(namespace)
                run("ip", "-n", namespace, "link", "set", "lo", "up")
                # Disable reverse-path filtering only in these fresh namespaces
                # so the LAN source test reaches the socket's own enforcement.
                run("ip", "netns", "exec", namespace, "sysctl", "-qw", "net.ipv4.conf.all.rp_filter=0", "net.ipv4.conf.default.rp_filter=0")
            for interface, peer_interface, subnet in (("tailscale0", "overlay", "10.210.1"), ("lan", "lan", "10.210.2")):
                run("ip", "-n", server, "link", "add", interface, "type", "veth", "peer", "name", peer_interface, "netns", peer)
                for namespace, name, host in ((server, interface, "1"), (peer, peer_interface, "2")):
                    run("ip", "-n", namespace, "address", "add", f"{subnet}.{host}/30", "dev", name)
                    run("ip", "-n", namespace, "link", "set", name, "up")
            run("ip", "-n", server, "address", "add", "100.100.1.2/32", "dev", "tailscale0")
            run("ip", "-n", peer, "address", "add", "100.100.2.2/32", "dev", "lo")
            run("ip", "-n", server, "route", "add", "100.100.2.2/32", "via", "10.210.2.2", "dev", "lan")

            ports = {}
            def start_listener(mode):
                command = ["ip", "netns", "exec", server, "setpriv", "--reuid=65534", "--regid=65534", "--clear-groups", "--bounding-set=-all", "--inh-caps=-all", "--ambient-caps=-all", "env", f"HOMENODE_INGRESS_FIXTURE_CHILD={mode}", str(binary), "-test.run=^TestIngressNamespaceListenerChild$", "-test.timeout=60s"]
                child = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
                children.append(child)
                if not select.select([child.stdout], [], [], 10)[0]:
                    raise RuntimeError(f"{mode} listener readiness timed out")
                line = child.stdout.readline().strip()
                if not line.startswith("READY 100.100.1.2:"):
                    raise RuntimeError(f"{mode} listener did not start: {line}")
                return int(line.rsplit(":", 1)[1])

            for mode in ("bound", "control"):
                ports[mode] = start_listener(mode)
            ports["watched"] = start_listener("watched")
            watched_child = children[-1]

            client = """import socket,sys
s=socket.socket()
s.settimeout(1)
s.bind((sys.argv[1],0))
try:
 s.connect(('100.100.1.2',int(sys.argv[2])))
 assert s.makefile('rb').readline(64)==b'homenode-ingress\\n'
 print('accepted')
except (TimeoutError,ConnectionRefusedError,OSError):
 print('blocked')
finally:
 s.close()
"""
            cases = (("overlay", "10.210.1.1", "10.210.1.2", "accepted"), ("lan", "10.210.2.1", "10.210.2.2", "blocked"), ("lan", "10.210.2.1", "100.100.2.2", "blocked"), ("overlay", "10.210.1.1", "10.210.1.2", "accepted"))
            def check_cases(phase, bound_available=True):
                for interface, gateway, source, expected in cases:
                    run("ip", "-n", peer, "route", "replace", "100.100.1.2/32", "via", gateway, "dev", interface)
                    # Both listeners must remain alive: a dead bound listener is
                    # never acceptable evidence for a negative connection attempt.
                    if any(child.poll() is not None for child in children if child not in retired):
                        raise RuntimeError("listener exited during ingress checks")
                    for mode in ("control", "bound"):
                        result = run("ip", "netns", "exec", peer, sys.executable, "-c", client, source, str(ports[mode])).strip()
                        want = "accepted" if mode == "control" else (expected if bound_available else "blocked")
                        if result != want:
                            raise RuntimeError(f"{mode} {interface} source={source}: {result}, expected {want}")
                        print(f"PASS {phase} {mode} ingress={interface} source={source}: {result}", flush=True)

            check_cases("initial")
            result = run("ip", "netns", "exec", peer, sys.executable, "-c", client, "10.210.1.2", str(ports["watched"])).strip()
            if result != "accepted":
                raise RuntimeError("watched listener did not accept permitted ingress before replacement")
            old_index = json.loads(run("ip", "-j", "-n", server, "link", "show", "tailscale0"))[0]["ifindex"]
            run("ip", "-n", server, "link", "delete", "tailscale0")
            run("ip", "-n", server, "link", "add", "tailscale0", "type", "veth", "peer", "name", "overlay", "netns", peer)
            for namespace, name, host in ((server, "tailscale0", "1"), (peer, "overlay", "2")):
                run("ip", "-n", namespace, "address", "add", f"10.210.1.{host}/30", "dev", name)
                run("ip", "-n", namespace, "link", "set", name, "up")
            run("ip", "-n", server, "address", "add", "100.100.1.2/32", "dev", "tailscale0")
            new_index = json.loads(run("ip", "-j", "-n", server, "link", "show", "tailscale0"))[0]["ifindex"]
            if old_index == new_index:
                raise RuntimeError("fixture did not replace the kernel interface index")
            watched_output, _ = watched_child.communicate(timeout=5)
            if watched_child.returncode != 0 or "WATCH-CLOSED" not in watched_output:
                raise RuntimeError(f"interface watcher did not stop Accept: {watched_output}")
            retired.add(watched_child)
            print("PASS interface watcher closed Accept after real interface replacement", flush=True)
            check_cases("recreated-old-listener", bound_available=False)
            ports["bound"] = start_listener("bound")
            check_cases("restarted-listener")
            # Exercise the complete production interface/address admission and
            # one-second health observation against a real kernel TUN. Packet
            # routing assertions above remain the separate veth qualification.
            run("ip", "-n", server, "link", "delete", "tailscale0")
            run("ip", "netns", "exec", server, "ip", "tuntap", "add", "dev", "tailscale0", "mode", "tun")
            run("ip", "-n", server, "address", "add", "100.100.1.2/32", "dev", "tailscale0")
            run("ip", "-n", server, "link", "set", "tailscale0", "up")
            start_listener("production")
            production_child = children[-1]
            run("ip", "-n", server, "address", "delete", "100.100.1.2/32", "dev", "tailscale0")
            production_output, _ = production_child.communicate(timeout=5)
            if production_child.returncode != 0 or "WATCH-CLOSED" not in production_output:
                raise RuntimeError(f"production TUN address observation did not close Accept: {production_output}")
            retired.add(production_child)
            # Admission must now refuse rather than exposing a fallback socket.
            refusal = run("ip", "netns", "exec", server, "setpriv", "--reuid=65534", "--regid=65534", "--clear-groups", "--bounding-set=-all", "--inh-caps=-all", "--ambient-caps=-all", "env", "HOMENODE_INGRESS_FIXTURE_CHILD=production-refusal", str(binary), "-test.run=^TestIngressNamespaceListenerChild$", "-test.timeout=10s")
            if "ADMISSION-REFUSED" not in refusal:
                raise RuntimeError("production listener did not refuse missing TUN address")
            print("PASS real TUN production admission, address-loss shutdown and restart refusal", flush=True)
            print("PASS unprivileged device-bound ingress without host firewall rules", flush=True)
        finally:
            for child in children:
                child.terminate()
                try:
                    child.communicate(timeout=5)
                except subprocess.TimeoutExpired:
                    child.kill()
                    child.communicate(timeout=5)
            for namespace in reversed(namespaces):
                run("ip", "netns", "delete", namespace)


if __name__ == "__main__":
    main()
