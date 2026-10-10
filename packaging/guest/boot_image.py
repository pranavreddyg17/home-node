#!/usr/bin/env python3
"""Disposable unprivileged TCG/KVM boot/adapter fixture; not production qualification."""
import base64
import fcntl
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
import boot_evidence


def development_boot_accelerator():
    accelerator = os.getenv("HOMENODE_GUEST_BOOT_ACCELERATOR", "tcg")
    if accelerator not in {"tcg", "kvm"}:
        raise ValueError("unsupported development boot accelerator")
    if accelerator == "kvm":
        descriptor = os.open("/dev/kvm", os.O_RDWR | os.O_NOFOLLOW | os.O_CLOEXEC)
        try:
            metadata = os.fstat(descriptor)
            if not stat.S_ISCHR(metadata.st_mode) or os.major(metadata.st_rdev) != 10 or os.minor(metadata.st_rdev) != 232:
                raise ValueError("development KVM device identity refused")
            if fcntl.ioctl(descriptor, 0xAE00, 0) != 12:  # KVM_GET_API_VERSION
                raise ValueError("development KVM API refused")
        finally:
            os.close(descriptor)
    return accelerator


def read_exact(channel, size):
    data = bytearray()
    while len(data) < size:
        chunk = channel.recv(size - len(data))
        if not chunk:
            raise ValueError("guest channel closed")
        data.extend(chunk)
    return bytes(data)


def request(channel, operation, allowed_error=None, **fields):
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
    if not isinstance(response, dict) or set(response) - allowed or type(response.get("version")) is not int or response.get("version") != 1 or response.get("requestId") != identifier:
        raise ValueError("guest request failed")
    for name in ("offset", "size"):
        if name in response and (type(response[name]) is not int or not 0 <= response[name] <= 512 << 30):
            raise ValueError("invalid numeric guest response")
    for name in ("error", "state", "sha256", "data", "text"):
        if name in response and not isinstance(response[name], str):
            raise ValueError("invalid text guest response")
    if response.get("error") and response["error"] != allowed_error:
        raise ValueError("guest request failed")
    return response


def object_roundtrip(channel):
    object_id = uuid.uuid4().hex
    content = bytes(range(256)) * 4096 + b"end"
    checksum = hashlib.sha256(content).hexdigest()
    for offset in range(0, len(content), 256 << 10):
        chunk = content[offset:offset + (256 << 10)]
        fields = {"objectId": object_id, "offset": offset, "size": len(content),
                  "sha256": hashlib.sha256(chunk).hexdigest(), "data": base64.b64encode(chunk).decode()}
        if request(channel, "upload", **fields).get("offset") != offset + len(chunk):
            raise ValueError("guest chunk upload mismatch")
        if offset == 256 << 10:
            # Lost acknowledgement recovery must preserve bytes and progress.
            if request(channel, "upload", **fields).get("offset") != offset + len(chunk):
                raise ValueError("guest acknowledged chunk replay mismatch")
    finalized = request(channel, "finalize", objectId=object_id, size=len(content), sha256=checksum)
    if finalized.get("size") != len(content) or finalized.get("sha256") != checksum:
        raise ValueError("guest finalization mismatch")
    downloaded = bytearray()
    while len(downloaded) < len(content):
        part = request(channel, "download", objectId=object_id, offset=len(downloaded))
        chunk = base64.b64decode(part.get("data", ""), validate=True)
        if not 0 < len(chunk) <= 256 << 10 or len(downloaded) + len(chunk) > len(content) or part.get("offset") != len(downloaded) + len(chunk) or part.get("sha256") != hashlib.sha256(chunk).hexdigest():
            raise ValueError("guest object chunk mismatch")
        downloaded.extend(chunk)
    if downloaded != content or hashlib.sha256(downloaded).hexdigest() != checksum:
        raise ValueError("guest download mismatch")
    request(channel, "delete", objectId=object_id)
    return {"bytes": len(content), "chunkBytes": 256 << 10, "acknowledgedChunkReplay": True}


def cancel_video(channel, input_id):
    job_id = uuid.uuid4().hex
    if request(channel, "run", objectId=job_id, inputId=input_id, preset="mp4-1080p").get("state") != "running":
        raise ValueError("cancellation fixture was not started")
    return cancel_task(channel, job_id)


def cancel_ai(channel):
    job_id = uuid.uuid4().hex
    if request(channel, "generate", objectId=job_id, prompt="Count slowly from one to one hundred.").get("state") != "running":
        raise ValueError("AI cancellation fixture was not started")
    return cancel_task(channel, job_id)


def cancel_task(channel, job_id):
    if request(channel, "cancel", objectId=job_id).get("state") != "cancelled":
        raise ValueError("task cancellation did not take effect")
    deadline = time.monotonic() + 30
    while True:
        if request(channel, "result", objectId=job_id).get("state") != "cancelled":
            raise ValueError("cancelled task state changed")
        response = request(channel, "delete", allowed_error="OBJECT_BUSY", objectId=job_id)
        if not response.get("error"):
            break
        if time.monotonic() >= deadline:
            raise ValueError("cancelled worker did not release deletion")
        time.sleep(0.1)
    if request(channel, "result", allowed_error="NOT_FOUND", objectId=job_id).get("error") != "NOT_FOUND":
        raise ValueError("deleted task remains visible")
    if request(channel, "health").get("state") != "ready":
        raise ValueError("adapter unavailable after cancellation")
    return {"cancelled": True, "taskDeletionAcknowledged": True}


def ai_roundtrip(channel):
    cancellation = cancel_ai(channel)
    generation = uuid.uuid4().hex
    if request(channel, "generate", objectId=generation, prompt="Say hello briefly.").get("state") != "running":
        raise ValueError("guest inference was not started")
    deadline = time.monotonic() + 120
    while True:
        result = request(channel, "result", objectId=generation)
        if result.get("state") == "succeeded":
            break
        if result.get("state") != "running" or time.monotonic() >= deadline:
            raise ValueError("guest inference did not succeed")
        time.sleep(0.2)
    text = result.get("text", "")
    if not 0 < len(text.encode("utf-8")) <= 32768:
        raise ValueError("guest inference text exceeds bound")
    request(channel, "delete", objectId=generation)
    return {"generationSucceeded": True, "textBytes": len(text.encode("utf-8")), "taskDeletionAcknowledged": True, "cancellation": cancellation}


def video_roundtrip(channel, output):
    # Source material is generated locally; no owner files are used. All probing
    # happens only in this disposable unprivileged CI fixture, never the server.
    source = output / "fixture-source.mp4"
    fd = os.open(source, os.O_RDWR | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    try:
        subprocess.run(["/usr/bin/ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
                        "-f", "lavfi", "-i", "color=c=black:s=1920x1080:r=30", "-frames:v", "30",
                        "-threads", "1", "-c:v", "libx264", "-preset", "ultrafast", "-movflags", "+faststart",
                        "-f", "mp4", "/proc/self/fd/" + str(fd)], pass_fds=(fd,), check=True, timeout=30)
        if not 0 < os.fstat(fd).st_size <= 256 << 10:
            raise ValueError("fixture source exceeds transfer bound")
        os.lseek(fd, 0, os.SEEK_SET)
        content = os.read(fd, 256 << 10)
    finally:
        os.close(fd)
    input_id = uuid.uuid4().hex
    checksum = hashlib.sha256(content).hexdigest()
    uploaded = request(channel, "upload", objectId=input_id, size=len(content), sha256=checksum,
                       data=base64.b64encode(content).decode())
    if uploaded.get("offset") != len(content):
        raise ValueError("video input upload mismatch")
    finalized = request(channel, "finalize", objectId=input_id, size=len(content), sha256=checksum)
    if finalized.get("size") != len(content) or finalized.get("sha256") != checksum:
        raise ValueError("video input finalization mismatch")
    cancellation = cancel_video(channel, input_id)
    evidence = []
    for preset, width, height in [("mp4-720p", 1280, 720), ("mp4-1080p", 1920, 1080)]:
        job_id = uuid.uuid4().hex
        if request(channel, "run", objectId=job_id, inputId=input_id, preset=preset).get("state") != "running":
            raise ValueError("video job was not started")
        deadline = time.monotonic() + 120
        while True:
            status = request(channel, "result", objectId=job_id)
            if status.get("state") == "succeeded":
                break
            if status.get("state") != "running" or time.monotonic() >= deadline:
                raise ValueError("booted video conversion did not succeed")
            time.sleep(0.2)
        size = status.get("size")
        if type(size) is not int or not 0 < size <= 2 << 20:
            raise ValueError("video fixture output exceeds bound")
        encoded = bytearray()
        while len(encoded) < size:
            part = request(channel, "download", objectId=job_id, offset=len(encoded))
            chunk = base64.b64decode(part.get("data", ""), validate=True)
            if not 0 < len(chunk) <= 256 << 10 or len(encoded) + len(chunk) > size or part.get("offset") != len(encoded) + len(chunk) or part.get("sha256") != hashlib.sha256(chunk).hexdigest():
                raise ValueError("video output chunk mismatch")
            encoded.extend(chunk)
        if hashlib.sha256(encoded).hexdigest() != status.get("sha256"):
            raise ValueError("video output digest mismatch")
        result = output / ("fixture-" + preset + ".mp4")
        fd = os.open(result, os.O_RDWR | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        try:
            with os.fdopen(os.dup(fd), "wb") as file:
                file.write(encoded)
                file.flush()
                os.fsync(file.fileno())
            probe = subprocess.run(["/usr/bin/prlimit", "--as=536870912", "--cpu=15", "--core=0",
                                    "/usr/bin/ffprobe", "-v", "error", "-protocol_whitelist", "file,pipe",
                                    "-select_streams", "v:0", "-show_entries", "stream=codec_name,width,height",
                                    "-of", "json", "/proc/self/fd/" + str(fd)],
                                   pass_fds=(fd,), capture_output=True, check=True, timeout=20)
        finally:
            os.close(fd)
        if len(probe.stdout) > 16384:
            raise ValueError("video probe exceeds bound")
        metadata = json.loads(probe.stdout, object_pairs_hook=overlay.unique_object)
        if metadata.get("streams") != [{"codec_name": "h264", "width": width, "height": height}]:
            raise ValueError("video preset codec or dimensions mismatch")
        request(channel, "delete", objectId=job_id)
        evidence.append({"preset": preset, "width": width, "height": height, "codec": "h264", "bytes": size})
    request(channel, "delete", objectId=input_id)
    return evidence, cancellation


def boot(image, manifest, output):
    if sys.platform != "linux" or os.geteuid() == 0 or os.getenv("HOMENODE_GUEST_BOOT_INTEGRATION") != "1":
        raise ValueError("requires explicit unprivileged disposable Linux fixture")
    accelerator = development_boot_accelerator()
    metadata_fd = os.open(manifest, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(metadata_fd, "rb") as file:
        metadata_info = os.fstat(file.fileno())
        if not stat.S_ISREG(metadata_info.st_mode) or not 0 < metadata_info.st_size <= 16384:
            raise ValueError("development manifest exceeds bound")
        metadata = file.read(16385)
        if len(metadata) != metadata_info.st_size:
            raise ValueError("development manifest changed")
    record = json.loads(metadata, object_pairs_hook=overlay.unique_object)
    if not isinstance(record, dict) or record.get("profile") not in ("files", "video", "ai") or record.get("releaseQualified") is not False or record.get("bootValidated") is not False:
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
            result = boot_vm(fd, data_fd, output, record, accelerator)
        finally:
            os.close(data_fd)
        return result
    finally:
        os.close(fd)


def qmp_message(channel, deadline):
    channel.settimeout(max(0.001, deadline - time.monotonic()))
    line = bytearray()
    while len(line) < 8192:
        if time.monotonic() >= deadline:
            raise ValueError("QMP shutdown evidence timed out")
        byte = channel.recv(1)
        if not byte:
            raise ValueError("QMP closed before shutdown evidence")
        line.extend(byte)
        if byte == b"\n":
            message = json.loads(line)
            if not isinstance(message, dict):
                raise ValueError("invalid QMP message")
            return message
    raise ValueError("QMP message exceeds bound")


def qmp_shutdown(channel):
    deadline = time.monotonic() + 90
    if not isinstance(qmp_message(channel, deadline).get("QMP"), dict):
        raise ValueError("QMP greeting missing")
    for command, identifier in (("qmp_capabilities", "capabilities"), ("system_powerdown", "powerdown")):
        channel.sendall(json.dumps({"execute": command, "id": identifier}).encode() + b"\n")
        acknowledged, shutdown = False, False
        for _ in range(256):
            message = qmp_message(channel, deadline)
            if "error" in message:
                raise ValueError("QMP shutdown command refused")
            if message.get("event") == "SHUTDOWN":
                data = message.get("data", {})
                if command != "system_powerdown" or not isinstance(data, dict) or data.get("guest") is not True or data.get("reason") != "guest-shutdown":
                    raise ValueError("shutdown was not guest initiated")
                shutdown = True
            if "return" in message:
                if message.get("id") != identifier or message["return"] != {} or acknowledged:
                    raise ValueError("unexpected QMP acknowledgement")
                acknowledged = True
            if acknowledged and (command == "qmp_capabilities" or shutdown):
                break
        else:
            raise ValueError("QMP shutdown evidence exceeds event bound")
    return {"guestInitiated": True, "reason": "guest-shutdown"}


def boot_vm(system_fd, data_fd, output, record, accelerator="tcg"):
    if accelerator not in {"tcg", "kvm"}:
        raise ValueError("unsupported development boot accelerator")
    channel_path = output / "adapter.sock"
    qmp_path = output / "qmp.sock"
    if len(os.fsencode(channel_path)) > 100 or "," in str(channel_path) or "\n" in str(channel_path):
        raise ValueError("fixture socket path exceeds bound")
    command = ["/usr/bin/qemu-system-x86_64", "-machine", "q35", "-accel", accelerator, "-m", "1536" if record["profile"] == "ai" else "512", "-smp", "2" if record["profile"] == "ai" else "1",
               "-nodefaults", "-nic", "none", "-display", "none", "-monitor", "none", "-qmp", f"unix:{qmp_path},server=on,wait=off", "-serial", "stdio", "-no-reboot",
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
            while True:
                state = request(channel, "health").get("state")
                if state == "ready":
                    break
                if state != "starting" or time.monotonic() >= deadline:
                    raise ValueError("guest did not report ready")
                time.sleep(0.2)
            channel.settimeout(30)
            object_evidence = object_roundtrip(channel)
            video_evidence, cancellation = video_roundtrip(channel, output) if record["profile"] == "video" else ([], None)
            ai_evidence = ai_roundtrip(channel) if record["profile"] == "ai" else None
        with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as monitor:
            monitor.settimeout(90)
            monitor.connect(str(qmp_path))
            shutdown_evidence = qmp_shutdown(monitor)
        if process.wait(timeout=30) != 0:
            raise ValueError("guest poweroff process exit failed")
        subprocess.run(["/usr/sbin/e2fsck", "-f", "-n", "/proc/self/fd/" + str(data_fd)],
                       pass_fds=(data_fd,), stdin=subprocess.DEVNULL,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True, timeout=30)
        shutdown_evidence["qemuExitCode"] = 0
        shutdown_evidence["readOnlyFilesystemCheck"] = True
        result = {"schema": 1, "profile": record["profile"], "imageSHA256": record["sha256"],
                  "accelerator": accelerator, "tcgBootAndObjectRoundTrip": accelerator == "tcg",
                  "kvmBootAndObjectRoundTrip": accelerator == "kvm", "objectTransfer": object_evidence,
                  "videoPresets": video_evidence, "videoCancellation": cancellation, "aiInference": ai_evidence, "shutdown": shutdown_evidence, "releaseQualified": False}
        boot_evidence.validate(result, record, accelerator)
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
        # Disposable disks are retained. Failure teardown is not shutdown evidence.


if __name__ == "__main__":
    try:
        print(json.dumps(boot(Path(sys.argv[1]), Path(sys.argv[2]), Path(sys.argv[3]))))
    except (OSError, ValueError, KeyError, IndexError, TypeError, subprocess.SubprocessError):
        sys.exit("Development guest boot failed; retain fixture diagnostics")
