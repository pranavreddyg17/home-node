"""Root-only disposable browser staging. Importing this module has no effects."""
import grp
import os
import pathlib
import pwd
import shutil
import stat
import subprocess
import sys


def protected_program(value):
    path = pathlib.Path(value)
    if not path.is_absolute() or str(path.resolve()) != str(path):
        raise RuntimeError("browser runtime program must be canonical")
    metadata = path.lstat()
    if not stat.S_ISREG(metadata.st_mode) or metadata.st_uid != 0 or metadata.st_mode & 0o022:
        raise RuntimeError(f"browser runtime program is unprotected: uid={metadata.st_uid} gid={metadata.st_gid} mode={stat.S_IMODE(metadata.st_mode):o} regular={stat.S_ISREG(metadata.st_mode)}")
    for parent in path.parents:
        metadata = parent.lstat()
        if not stat.S_ISDIR(metadata.st_mode) or metadata.st_uid != 0 or metadata.st_mode & 0o022:
            raise RuntimeError(f"browser runtime ancestor is unprotected: path={parent} uid={metadata.st_uid} gid={metadata.st_gid} mode={stat.S_IMODE(metadata.st_mode):o}")
    return str(path)


class NativeBrowser:
    def __init__(self, directory, node, executable, client, modules, users, groups, payload_bytes="32768"):
        if sys.platform != "linux" or os.geteuid() != 0 or os.environ.get("HOMENODE_FILES_BROWSER_INTEGRATION") != "1":
            raise RuntimeError("explicit disposable Linux browser staging required")
        if payload_bytes not in ("32768", "1073741824"):
            raise RuntimeError("unsupported native browser payload size")
        self.payload_bytes = payload_bytes
        self.runtime_seconds = 630 if payload_bytes == "1073741824" else 330
        self.node, self.executable = protected_program(node), protected_program(executable)
        self.name = "homenode-browser-fixture"
        for lookup in (pwd.getpwnam, grp.getgrnam):
            try:
                lookup(self.name)
            except KeyError:
                continue
            raise RuntimeError("browser fixture requires a fresh account and group")
        self.home = directory / "browser-home"
        subprocess.run(["groupadd", "--system", self.name], check=True, capture_output=True, timeout=10)
        groups.append(self.name)
        subprocess.run(["useradd", "--system", "--no-create-home", "--home-dir", str(self.home), "--shell", "/usr/sbin/nologin", "--gid", self.name, "--groups", "", self.name], check=True, capture_output=True, timeout=10)
        users.append(self.name)
        account = pwd.getpwnam(self.name)
        if account.pw_uid <= 0 or account.pw_gid <= 0 or set(os.getgrouplist(self.name, account.pw_gid)) != {account.pw_gid}:
            raise RuntimeError("browser account has unexpected authority")
        for path in (self.home, self.home / ".pki", self.home / ".pki/nssdb", self.home / "tmp"):
            path.mkdir(mode=0o700)
            os.chown(path, account.pw_uid, account.pw_gid)
        # Chromium honors an existing legacy shared NSS directory, including
        # current versions whose new default lives under .local/share/pki.
        initialize = ["setpriv", f"--reuid={account.pw_uid}", f"--regid={account.pw_gid}", "--clear-groups", "--no-new-privs", "/usr/bin/env", f"HOME={self.home}", "/usr/bin/certutil", "-d", f"sql:{self.home}/.pki/nssdb"]
        subprocess.run([*initialize, "-N", "--empty-password"], check=True, capture_output=True, timeout=10)
        # Trust this self-signed peer only in the disposable account. The host
        # trust store, owner profiles and TLS private key are never modified.
        subprocess.run([*initialize, "-A", "-t", "P,,", "-n", "homenode-fixture-peer", "-i", str(directory / "ca.crt")], check=True, capture_output=True, timeout=10)
        runtime = directory / "browser-runtime"
        runtime.mkdir(mode=0o755)
        self.client = runtime / "files-https.mjs"
        shutil.copyfile(client, self.client)
        self.client.chmod(0o644)
        for package in ("@playwright/test", "playwright", "playwright-core"):
            target = runtime / "node_modules" / package
            target.parent.mkdir(parents=True, exist_ok=True, mode=0o755)
            shutil.copytree(pathlib.Path(modules) / package, target)

    def run(self, invitation, namespace, units):
        if not invitation or len(invitation) > 4096:
            raise RuntimeError("private browser invitation handoff missing or oversized")
        unit = f"homenode-browser-fixture-{os.getpid()}.service"
        units.append(unit)
        command = ["systemd-run", "--quiet", "--pipe", "--wait", "--collect", f"--unit={unit}", "--property=Type=exec", f"--property=User={self.name}", f"--property=Group={self.name}", f"--property=NetworkNamespacePath=/run/netns/{namespace}", "--property=UMask=0077", "--property=KillMode=control-group", "--property=TimeoutStopSec=10s", f"--property=RuntimeMaxSec={self.runtime_seconds}s", f"--setenv=HOME={self.home}", f"--setenv=TMPDIR={self.home}/tmp", "--setenv=HOMENODE_NATIVE_HTTPS_BROWSER=1", f"--setenv=HOMENODE_NATIVE_BROWSER_BYTES={self.payload_bytes}", self.node, str(self.client), self.executable]
        result = subprocess.run(command, input=invitation, capture_output=True, timeout=self.runtime_seconds + 5)
        marker = f"PASS native browser HTTPS Files: TLS 1.3, virtual passkey, session inventory, signed start/stop, {self.payload_bytes}-byte upload/download, Trash/restore and narrow viewport; browser=".encode()
        hostname_refusal = b"PASS native browser refuses certificate hostname mismatch"
        filename_safety = b"PASS native browser filename text rendering and rename identity"
        resume = b"PASS native browser one GiB pause, reload and resume retains transfer identity and acknowledged offset"
        if self.payload_bytes == "1073741824" and resume not in result.stdout:
            raise RuntimeError("native large browser workflow omitted acknowledged resume evidence: " + (result.stdout + result.stderr)[-4096:].decode(errors="replace"))
        if result.returncode != 0 or len(result.stdout) + len(result.stderr) > 65536 or marker not in result.stdout or hostname_refusal not in result.stdout or filename_safety not in result.stdout:
            raise RuntimeError("native browser workflow failed: " + (result.stdout + result.stderr)[-4096:].decode(errors="replace"))
        for line in result.stdout.splitlines():
            if line.startswith(marker) or line in (hostname_refusal, filename_safety, resume):
                print(line[:512].decode(), flush=True)
