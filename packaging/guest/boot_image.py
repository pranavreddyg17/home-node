#!/usr/bin/env python3
"""Disposable unprivileged TCG boot/adapter fixture; not production qualification."""
import base64
from collections import deque
import hashlib
import json
import os
from pathlib import Path
import socket
import stat
import struct
import subprocess
import sys
import threading
import time
import uuid
sys.dont_write_bytecode = True
import overlay


def read_exact(channel, size):
    data = bytearray()
    while len(data) < size:
        chunk = channel.recv(size - len(data))
        if not chunk:
            raise ValueError("guest channel closed")
        data.extend(chunk)
    return bytes(data)


def request(channel, operation, **fields):
    identifier = uuid.uuid4().hex
    payload = json.dumps({"version": 1, "requestId": identifier, "operation": operation, **fields}).encode()
    if len(payload) > 512 << 10:
        raise ValueError("fixture request exceeds frame bound")
    channel.sendall(struct.pack(">I", len(payload)) + payload)
    size = struct.unpack(">I", read_exact(channel, 4))[0]
    if not 0 < size <= 512 << 10:
        raise ValueError("guest response exceeds frame bound")
    response = json.loads(read_exact(channel, size), object_pairs_hook=overlay.unique_object)
    allowed = {"version", "requestId", "error", "state", "offset", "size", "sha256", "data", "text"}
    if not isinstance(response, dict) or set(response) - allowed or type(response.get("version")) is not int or response.get("version") != 1 or response.get("requestId") != identifier or response.get("error"):
        raise ValueError("guest request failed")
    return response


def boot(image, manifest, output):
    if sys.platform != "linux" or os.geteuid() == 0 or os.getenv("HOMENODE_GUEST_BOOT_INTEGRATION") != "1":
        raise ValueError("requires explicit unprivileged disposable Linux fixture")
    metadata_fd = os.open(manifest, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(metadata_fd, "rb") as file:
        metadata_info = os.fstat(file.fileno())
        if not stat.S_ISREG(metadata_info.st_mode) or not 0 < metadata_info.st_size <= 16384:
            raise ValueError("development manifest exceeds bound")
        metadata = file.read(16385)
        if len(metadata) != metadata_info.st_size:
            raise ValueError("development manifest changed")
    record = json.loads(metadata, object_pairs_hook=overlay.unique_object)
    if not isinstance(record, dict) or record.get("profile") not in ("files", "video") or record.get("releaseQualified") is not False or record.get("bootValidated") is not False:
        raise ValueError("expected unqualified development input")
    parent = output.parent.lstat()
    if not output.is_absolute() or not stat.S_ISDIR(parent.st_mode) or parent.st_uid != os.geteuid() or stat.S_IMODE(parent.st_mode) & 0o022:
        raise ValueError("expected protected fixture parent")
    fd = os.open(image, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= 8 << 30 or info.st_size != record.get("bytes"):
            raise ValueError("unexpected image input")
        digest = hashlib.sha256()
        while chunk := os.read(fd, 1 << 20):
            digest.update(chunk)
        if digest.hexdigest() != record.get("sha256"):
            raise ValueError("development image digest mismatch")
        os.mkdir(output, 0o700)
        data = output / "fixture-data.raw"
        data_fd = os.open(data, os.O_RDWR | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        try:
            os.ftruncate(data_fd, 128 << 20)
            subprocess.run(["/usr/sbin/mkfs.ext4", "-q", "-F", "-m", "0", "-E", "nodiscard,lazy_itable_init=0,lazy_journal_init=0", "-L", "homenode-data", "/proc/self/fd/" + str(data_fd)],
                           pass_fds=(data_fd,), check=True, timeout=30)
            os.fsync(data_fd)
            result = boot_vm(fd, data_fd, output, record)
        finally:
            os.close(data_fd)
        return result
    finally:
        os.close(fd)


def boot_vm(system_fd, data_fd, output, record):
    channel_path = output / "adapter.sock"
    if len(os.fsencode(channel_path)) > 100 or "," in str(channel_path) or "\n" in str(channel_path):
        raise ValueError("fixture socket path exceeds bound")
    command = ["/usr/bin/qemu-system-x86_64", "-machine", "q35", "-accel", "tcg", "-m", "512", "-smp", "1",
               "-nodefaults", "-nic", "none", "-display", "none", "-monitor", "none", "-serial", "stdio", "-no-reboot",
               "-drive", f"file=/proc/self/fd/{system_fd},if=none,id=system,format=raw,readonly=on",
               "-device", "virtio-blk-pci,drive=system,serial=homenode-system",
               "-drive", f"file=/proc/self/fd/{data_fd},if=none,id=data,format=raw",
               "-device", "virtio-blk-pci,drive=data,serial=homenode-data",
               "-object", "rng-random,id=rng,filename=/dev/urandom",
               "-device", "virtio-rng-pci,rng=rng,max-bytes=1024,period=1000",
               "-device", "virtio-serial-pci",
               "-chardev", f"socket,id=adapter,path={channel_path},server=on,wait=off",
               "-device", "virtserialport,chardev=adapter,name=org.homenode.adapter"]
    process = subprocess.Popen(command, pass_fds=(system_fd, data_fd), stdin=subprocess.DEVNULL,
                               stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    logs = deque(maxlen=128)  # bounded 8 MiB retained diagnostics
    def drain():
        while chunk := process.stdout.read(64 << 10):
            logs.append(chunk)
    reader = threading.Thread(target=drain)
    reader.start()
    try:
        deadline = time.monotonic() + 240
        with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as channel:
            while True:
                if process.poll() is not None or time.monotonic() >= deadline:
                    raise ValueError("guest did not expose fixture channel")
                try:
                    channel.connect(str(channel_path))
                    break
                except (FileNotFoundError, ConnectionRefusedError):
                    time.sleep(0.1)
            channel.settimeout(max(1, deadline - time.monotonic()))
            if request(channel, "health").get("state") != "ready":
                raise ValueError("guest did not report ready")
            channel.settimeout(30)
            object_id = uuid.uuid4().hex
            content = b"HomeNode booted guest object round trip\n" * 1024
            checksum = hashlib.sha256(content).hexdigest()
            uploaded = request(channel, "upload", objectId=object_id, size=len(content), sha256=checksum,
                               data=base64.b64encode(content).decode())
            if uploaded.get("offset") != len(content):
                raise ValueError("guest upload mismatch")
            finalized = request(channel, "finalize", objectId=object_id, size=len(content), sha256=checksum)
            if finalized.get("size") != len(content) or finalized.get("sha256") != checksum:
                raise ValueError("guest finalization mismatch")
            downloaded = request(channel, "download", objectId=object_id)
            if base64.b64decode(downloaded.get("data", ""), validate=True) != content:
                raise ValueError("guest download mismatch")
            request(channel, "delete", objectId=object_id)
        result = {"schema": 1, "profile": record["profile"], "imageSHA256": record["sha256"],
                  "tcgBootAndObjectRoundTrip": True, "releaseQualified": False}
        with (output / "boot-evidence.json").open("x") as file:
            json.dump(result, file, indent=2)
            file.write("\n")
        return result
    finally:
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=10)
        reader.join(timeout=10)
        process.stdout.close()
        if reader.is_alive():
            raise ValueError("fixture diagnostic reader did not stop")
        with (output / "boot-console.log").open("xb") as file:
            file.write(b"".join(logs))
        # Disposable disks are retained; termination is not a clean backup stop.


if __name__ == "__main__":
    try:
        print(json.dumps(boot(Path(sys.argv[1]), Path(sys.argv[2]), Path(sys.argv[3]))))
    except (OSError, ValueError, KeyError, IndexError, TypeError, subprocess.SubprocessError):
        sys.exit("Development guest boot failed; retain fixture diagnostics")
