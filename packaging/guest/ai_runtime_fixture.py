#!/usr/bin/env python3
"""Disposable network-isolated CPU inference fixture, not guest qualification."""
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import time
import urllib.error
import urllib.request
import build_ai_runtime as runtime
import download_ai_model as model
import overlay


def streaming_result(stream):
    total, text_bytes, finished, done, role_started = 0, 0, False, False, False
    while line := stream.readline(65537):
        total += len(line)
        if len(line) > 65536 or total > 256 << 10:
            raise ValueError("inference stream exceeds fixture bound")
        if not line.startswith(b"data: "):
            continue
        data = line[6:].strip()
        if data == b"[DONE]":
            done = True
            break
        item = json.loads(data, object_pairs_hook=overlay.unique_object)
        choices = item.get("choices") if isinstance(item, dict) else None
        if not isinstance(choices, list) or len(choices) != 1:
            raise ValueError("unexpected inference choices")
        choice = choices[0]
        if not isinstance(choice, dict) or not isinstance(choice.get("delta"), dict):
            raise ValueError("invalid inference delta")
        text = choice["delta"].get("content", "")
        if text is None and choice["delta"] == {"role": "assistant", "content": None} and choice.get("finish_reason") is None and not finished and text_bytes == 0 and not role_started:
            role_started = True
            continue
        if not isinstance(text, str) or finished:
            raise ValueError("invalid inference continuation")
        text_bytes += len(text.encode("utf-8"))
        if text_bytes > 32768:
            raise ValueError("inference text exceeds fixture bound")
        reason = choice.get("finish_reason")
        if reason is not None:
            if reason not in ("stop", "length"):
                raise ValueError("unexpected inference finish reason")
            finished = True
    if not finished or not done or text_bytes == 0:
        raise ValueError("inference stream did not complete")
    return text_bytes


def pinned_file(path, size, digest):
    runtime.private_directory(path.parent)
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_nlink != 1 or info.st_mode & 0o022 or info.st_size != size:
            raise ValueError("unexpected fixture input")
        with os.fdopen(os.dup(fd), "rb") as file:
            if hashlib.file_digest(file, "sha256").hexdigest() != digest:
                raise ValueError("fixture input digest mismatch")
        os.lseek(fd, 0, os.SEEK_SET)
        return fd
    except BaseException:
        os.close(fd)
        raise


def fixture(artifacts, models, output):
    if sys.platform != "linux" or os.geteuid() == 0 or os.getenv("HOMENODE_AI_RUNTIME_INTEGRATION") != "1":
        raise ValueError("requires explicit unprivileged disposable Linux fixture")
    parent_net = os.getenv("HOMENODE_FIXTURE_PARENT_NET")
    if not parent_net or os.readlink("/proc/self/ns/net") == parent_net:
        raise ValueError("requires a separate network namespace")
    interfaces = {line.split(":", 1)[0].strip() for line in Path("/proc/net/dev").read_text().splitlines() if ":" in line}
    capabilities = [line.split()[1] for line in Path("/proc/self/status").read_text().splitlines() if line.startswith("CapEff:")]
    if interfaces != {"lo"} or capabilities != ["0000000000000000"]:
        raise ValueError("fixture network or privilege isolation mismatch")
    runtime.private_directory(artifacts)
    manifest_fd = os.open(artifacts / "development-runtime.json", os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(manifest_fd, "rb") as file:
        info = os.fstat(file.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_nlink != 1 or info.st_mode & 0o022 or not 0 < info.st_size <= 16384:
            raise ValueError("runtime manifest exceeds bound")
        record = json.loads(file.read(16385), object_pairs_hook=overlay.unique_object)
    if not isinstance(record, dict) or record.get("sourceRevision") != runtime.REVISION or record.get("cmakeOptions") != runtime.OPTIONS or record.get("releaseQualified") is not False or type(record.get("binaryBytes")) is not int or not 0 < record["binaryBytes"] <= 256 << 20:
        raise ValueError("unexpected runtime manifest")
    runtime.private_directory(output.parent)
    os.mkdir(output, 0o700)
    binary_fd = pinned_file(artifacts / "llama-server", record["binaryBytes"], record["binarySHA256"])
    try:
        model_fd = pinned_file(models / "model.gguf", model.SIZE, model.SHA256)
        try:
            run(binary_fd, model_fd, output, record)
        finally:
            os.close(model_fd)
    finally:
        os.close(binary_fd)


def run(binary_fd, model_fd, output, record):
    command = ["/usr/bin/prlimit", "--as=1073741824", "--cpu=90", "--nproc=128", "--nofile=128", "--core=0", "--fsize=65536",
               "/proc/self/fd/" + str(binary_fd), "--model", "/proc/self/fd/" + str(model_fd),
               "--offline", "--host", "127.0.0.1", "--port", "8080", "--parallel", "1",
               "--ctx-size", "512", "--predict", "16", "--threads", "2", "--threads-batch", "2",
               "--batch-size", "64", "--ubatch-size", "64", "--cache-ram", "0", "--no-cache-prompt",
               "--no-cache-idle-slots", "--no-jinja", "--chat-template", "chatml", "--no-webui", "--no-slots",
               "--no-webui-mcp-proxy", "--log-disable", "--device", "none", "--gpu-layers", "0",
               "--threads-http", "2", "--timeout", "30"]
    # Only generated test input is used. Kernel file-size limits bound retained
    # startup diagnostics even if this admitted development server is noisy.
    fd = os.open(output / "runtime-diagnostic.log", os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "wb") as diagnostic:
        process = subprocess.Popen(command, pass_fds=(binary_fd, model_fd), stdin=subprocess.DEVNULL,
                                   stdout=subprocess.DEVNULL, stderr=diagnostic,
                                   env={"PATH": "/usr/bin:/bin", "LANG": "C.UTF-8"})
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        deadline = time.monotonic() + 60
        while True:
            if process.poll() is not None or time.monotonic() >= deadline:
                raise ValueError("inference server did not become ready")
            try:
                with opener.open("http://127.0.0.1:8080/health", timeout=2) as response:
                    if json.loads(response.read(4097), object_pairs_hook=overlay.unique_object) == {"status": "ok"}:
                        break
            except (urllib.error.URLError, TimeoutError):
                pass
            time.sleep(0.1)
        request = urllib.request.Request("http://127.0.0.1:8080/v1/chat/completions",
            data=json.dumps({"messages": [{"role": "user", "content": "Say hello."}], "max_tokens": 16,
                             "stream": True, "cache_prompt": False}).encode(), headers={"Content-Type": "application/json"})
        with opener.open(request, timeout=30) as response:
            if response.headers.get_content_type() != "text/event-stream":
                raise ValueError("inference response is not a stream")
            count = streaming_result(response)
        evidence = {"schema": 1, "runtimeSHA256": record["binarySHA256"], "modelSHA256": model.SHA256,
                    "isolatedLoopbackStreamingInference": True, "textBytes": count, "contextTokens": 512,
                    "requestedOutputTokens": 16, "releaseQualified": False}
        with (output / "inference-evidence.json").open("x") as file:
            json.dump(evidence, file, indent=2)
            file.write("\n")
    finally:
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=10)


if __name__ == "__main__":
    try:
        fixture(Path(sys.argv[1]), Path(sys.argv[2]), Path(sys.argv[3]))
    except (OSError, ValueError, KeyError, TypeError, IndexError, subprocess.SubprocessError) as error:
        reason = str(error)[:240] if type(error) is ValueError else type(error).__name__
        sys.exit("Development inference fixture failed: " + reason)
