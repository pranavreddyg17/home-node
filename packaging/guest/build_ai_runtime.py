#!/usr/bin/env python3
"""Build a pinned development inference server; no model or release approval."""
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import stat
import subprocess
import sys

REVISION = "7fe450e19305b828c199d602c23a8337aaa1f03b"
OPTIONS = {
    "CMAKE_BUILD_TYPE": "Release", "BUILD_SHARED_LIBS": "OFF",
    "LLAMA_BUILD_COMMON": "ON", "LLAMA_BUILD_TOOLS": "ON", "LLAMA_BUILD_SERVER": "ON",
    "LLAMA_BUILD_TESTS": "OFF", "LLAMA_BUILD_EXAMPLES": "OFF", "LLAMA_BUILD_APP": "OFF",
    "LLAMA_BUILD_UI": "OFF", "LLAMA_USE_PREBUILT_UI": "OFF", "LLAMA_OPENSSL": "OFF",
    "LLAMA_SUBPROCESS": "OFF", "LLAMA_LLGUIDANCE": "OFF",
    "GGML_NATIVE": "OFF", "GGML_CPU": "ON", "GGML_BACKEND_DL": "OFF",
    "GGML_CPU_ALL_VARIANTS": "OFF", "GGML_CUDA": "OFF", "GGML_VULKAN": "OFF",
    "GGML_METAL": "OFF", "GGML_RPC": "OFF", "GGML_BLAS": "OFF",
    "GGML_SSE42": "OFF", "GGML_AVX": "OFF", "GGML_AVX2": "OFF", "GGML_BMI2": "OFF",
    "GGML_FMA": "OFF", "GGML_F16C": "OFF",
}


def private_directory(path):
    if not path.is_absolute() or path != Path(os.path.abspath(path)):
        raise ValueError("expected absolute canonical directory")
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) & 0o022:
        raise ValueError("expected protected owned directory")


def build(source, output):
    if sys.platform != "linux" or platform.machine() != "x86_64" or os.geteuid() == 0 or os.getenv("HOMENODE_AI_RUNTIME_BUILD") != "1":
        raise ValueError("requires explicit unprivileged disposable Linux x86-64 builder")
    private_directory(source)
    private_directory(output.parent)
    if not output.is_absolute() or output != Path(os.path.abspath(output)):
        raise ValueError("expected absolute canonical output")
    environment = {"PATH": "/usr/bin:/bin", "LANG": "C.UTF-8"}
    git = ["/usr/bin/git", "-c", "core.fsmonitor=false", "-C", str(source)]
    head = subprocess.run(git + ["rev-parse", "HEAD"], env=environment, capture_output=True, text=True, check=True, timeout=15).stdout.strip()
    changed = subprocess.run(git + ["status", "--porcelain", "--untracked-files=all"], env=environment, capture_output=True, check=True, timeout=15).stdout
    if head != REVISION or changed:
        raise ValueError("inference source identity mismatch")
    os.mkdir(output, 0o700)
    build_dir = output / "build"
    subprocess.run(["/usr/bin/cmake", "-S", str(source), "-B", str(build_dir)] +
                   ["-D" + key + "=" + value for key, value in OPTIONS.items()],
                   env=environment, check=True, timeout=120)
    subprocess.run(["/usr/bin/cmake", "--build", str(build_dir), "--target", "llama-server", "--parallel", "2"],
                   env=environment, check=True, timeout=1200)
    binary = build_dir / "bin/llama-server"
    fd = os.open(binary, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as file:
        info = os.fstat(file.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_nlink != 1 or not 0 < info.st_size <= 256 << 20:
            raise ValueError("unexpected inference binary")
        header = file.read(20)
        if len(header) != 20 or header[:6] != b"\x7fELF\x02\x01" or header[18:20] != b"\x3e\x00":
            raise ValueError("expected Linux x86-64 inference binary")
        file.seek(0)
        checksum = hashlib.file_digest(file, "sha256").hexdigest()
    # This executes only a binary compiled from the admitted upstream revision,
    # in the disposable builder. No guest/owner-supplied executable is run.
    subprocess.run([str(binary), "--version"], env=environment, check=True, timeout=15)
    artifacts = output / "artifacts"
    artifacts.mkdir(mode=0o700)
    shutil.copyfile(binary, artifacts / "llama-server")
    (artifacts / "llama-server").chmod(0o755)
    shutil.copyfile(source / "LICENSE", artifacts / "llama.cpp-LICENSE")
    record = {"schema": 1, "sourceRevision": REVISION, "cmakeOptions": OPTIONS,
              "binarySHA256": checksum, "binaryBytes": info.st_size,
              "releaseQualified": False, "modelIncluded": False}
    with (artifacts / "development-runtime.json").open("x") as file:
        json.dump(record, file, indent=2)
        file.write("\n")
    return record


if __name__ == "__main__":
    try:
        print(json.dumps(build(Path(sys.argv[1]), Path(sys.argv[2]))))
    except (OSError, ValueError, IndexError, subprocess.SubprocessError) as error:
        reason = str(error)[:240] if type(error) is ValueError else type(error).__name__
        sys.exit("Development inference build failed: " + reason + "; retain build diagnostics")
