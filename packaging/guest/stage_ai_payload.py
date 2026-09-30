#!/usr/bin/env python3
"""Assemble admitted development AI inputs; not a production image installer."""
import hashlib
import json
import os
from pathlib import Path
import stat
import sys
import build_ai_runtime as runtime
import download_ai_model as model

BINARY_SIZE = 14530880
BINARY_SHA256 = "1f8b8183270b7ea94a223a7f1c8e1e1f0026e2072f843770848944e9309d1fa9"


def copy_verified(source, target, size, checksum, mode):
    fd = os.open(source, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as incoming:
        info = os.fstat(incoming.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or info.st_mode & 0o022 or info.st_size != size:
            raise ValueError("unexpected AI payload input")
        output_fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, mode)
        with os.fdopen(output_fd, "wb") as outgoing:
            digest, copied = hashlib.sha256(), 0
            while chunk := incoming.read(min(1 << 20, size - copied + 1)):
                copied += len(chunk)
                if copied > size:
                    raise ValueError("AI payload input grew")
                outgoing.write(chunk)
                digest.update(chunk)
            if copied != size or digest.hexdigest() != checksum:
                raise ValueError("AI payload integrity mismatch")
            outgoing.flush()
            os.fsync(outgoing.fileno())
            os.fchmod(outgoing.fileno(), mode)


def stage(binary, weights, output):
    if sys.platform != "linux" or os.geteuid() != 0 or os.getenv("HOMENODE_GUEST_IMAGE_BUILD") != "1":
        raise ValueError("requires explicit disposable root Linux assembly")
    runtime.private_directory(output.parent)
    if not output.is_absolute() or output != Path(os.path.abspath(output)):
        raise ValueError("expected canonical AI assembly path")
    os.mkdir(output, 0o700)
    source = Path(__file__).resolve().parent
    units = [("usr/lib/systemd/system/homenode-model.service", source / "homenode-model.service"),
             ("usr/lib/systemd/system/homenode-guest@ai.service.d/20-model.conf", source / "homenode-ai-model.conf")]
    inputs = [("usr/lib/homenode/ai/llama-server", binary, BINARY_SIZE, BINARY_SHA256, 0o755),
              ("usr/lib/homenode/ai/model.gguf", weights, model.SIZE, model.SHA256, 0o644)]
    for name, unit in units:
        data = unit.read_bytes()
        if not 0 < len(data) <= 16384:
            raise ValueError("AI unit source exceeds bound")
        inputs.append((name, unit, len(data), hashlib.sha256(data).hexdigest(), 0o644))
    files = []
    for name, incoming, size, checksum, mode in inputs:
        target = output / name
        target.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
        copy_verified(incoming, target, size, checksum, mode)
        files.append({"path": name, "bytes": size, "sha256": checksum, "mode": mode})
    record = {"schema": 1, "runtimeRevision": runtime.REVISION, "modelRevision": model.REVISION,
              "files": files, "developmentOnly": True, "releaseQualified": False}
    with (output / "ai-payload.json").open("x") as file:
        json.dump(record, file, indent=2)
        file.write("\n")
        file.flush()
        os.fsync(file.fileno())
    return record


if __name__ == "__main__":
    try:
        print(json.dumps(stage(Path(sys.argv[1]), Path(sys.argv[2]), Path(sys.argv[3]))))
    except (OSError, ValueError, IndexError) as error:
        reason = str(error)[:240] if type(error) is ValueError else type(error).__name__
        sys.exit("Development AI staging failed: " + reason + "; retain assembly diagnostics")
