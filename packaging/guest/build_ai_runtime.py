#!/usr/bin/env python3
"""Build a pinned development inference server; no model or release approval."""
import hashlib
import json
import os
from pathlib import Path
import platform
import re
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
    "GGML_OPENMP": "ON", "GGML_OPENMP_FETCH": "OFF", "GGML_CPU_KLEIDIAI": "OFF",
}
SYSTEM_LIBRARIES = {"libstdc++.so.6", "libm.so.6", "libgomp.so.1", "libgcc_s.so.1", "libc.so.6", "ld-linux-x86-64.so.2"}
INTERPRETER = "/lib64/ld-linux-x86-64.so.2"
NOTICE_FILES = ("LICENSE", "licenses/LICENSE-jsonhpp", "vendor/cpp-httplib/LICENSE",
                "vendor/hash/xxhash/LICENSE", "vendor/hash/sha256/LICENSE", "vendor/hash/rotate-bits/LICENSE.md")


def stage_notices(source, artifacts):
    destination = artifacts / "notices"
    destination.mkdir(mode=0o700)
    records = []
    for name in NOTICE_FILES:
        fd = os.open(source / name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        with os.fdopen(fd, "rb") as file:
            info = os.fstat(file.fileno())
            if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_nlink != 1 or info.st_mode & 0o022 or not 0 < info.st_size <= 1 << 20:
                raise ValueError("unexpected source notice")
            data = file.read((1 << 20) + 1)
        if len(data) != info.st_size:
            raise ValueError("source notice changed")
        target = destination / name
        target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        with target.open("xb") as file:
            file.write(data)
        records.append({"sourcePath": name, "bytes": len(data), "sha256": hashlib.sha256(data).hexdigest()})
    return records


def dependencies(dynamic, program):
    if len(dynamic) > 65536 or len(program) > 65536:
        raise ValueError("ELF dependency report exceeds bound")
    needed = set()
    for line in dynamic.splitlines():
        if any("(" + tag + ")" in line for tag in ("RPATH", "RUNPATH", "AUDIT", "DEPAUDIT", "FILTER", "AUXILIARY")):
            raise ValueError("unexpected ELF loader policy")
        if "(NEEDED)" not in line:
            continue
        match = re.fullmatch(r"\s*0x[0-9a-fA-F]+\s+\(NEEDED\)\s+Shared library: \[([^\]]+)\]\s*", line)
        if not match or match[1] not in SYSTEM_LIBRARIES or match[1] in needed:
            raise ValueError("unexpected ELF dependency")
        needed.add(match[1])
    interpreters = re.findall(r"\[Requesting program interpreter: ([^\]]+)\]", program)
    if interpreters != [INTERPRETER] or "libc.so.6" not in needed or "libstdc++.so.6" not in needed:
        raise ValueError("unexpected ELF runtime contract")
    return sorted(needed)


def private_directory(path):
    if not path.is_absolute() or path != Path(os.path.abspath(path)):
        raise ValueError("expected absolute canonical directory")
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) & 0o022:
        raise ValueError("expected protected owned directory")


def check_cache(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as file:
        info = os.fstat(file.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_nlink != 1 or info.st_mode & 0o022 or not 0 < info.st_size <= 1 << 20:
            raise ValueError("unexpected compiler configuration")
        data = file.read((1 << 20) + 1)
    if len(data) != info.st_size or b"\0" in data:
        raise ValueError("compiler configuration changed")
    found = {}
    for line in data.decode("utf-8").splitlines():
        if line.startswith(("#", "//")) or not line.strip():
            continue
        entry, separator, value = line.partition("=")
        key, colon, kind = entry.partition(":")
        if key not in OPTIONS:
            continue
        expected_kind = "STRING" if key == "CMAKE_BUILD_TYPE" else "BOOL"
        if not separator or not colon or key in found or kind != expected_kind or value != OPTIONS[key]:
            raise ValueError("compiler option was not admitted")
        found[key] = value
    if found != OPTIONS:
        raise ValueError("compiler options are missing")
    return hashlib.sha256(data).hexdigest()


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
    cache_digest = check_cache(build_dir / "CMakeCache.txt")
    subprocess.run(["/usr/bin/cmake", "--build", str(build_dir), "--target", "llama-server", "--parallel", "2"],
                   env=environment, check=True, timeout=1200)
    if check_cache(build_dir / "CMakeCache.txt") != cache_digest:
        raise ValueError("compiler configuration changed during build")
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
    reports = []
    for option in ("--dynamic", "--program-headers"):
        reports.append(subprocess.run(["/usr/bin/readelf", "--wide", option, str(binary)],
                                      env=environment, capture_output=True, text=True, check=True, timeout=15).stdout)
    libraries = dependencies(*reports)
    # This executes only a binary compiled from the admitted upstream revision,
    # in the disposable builder. No guest/owner-supplied executable is run.
    subprocess.run([str(binary), "--version"], env=environment, check=True, timeout=15)
    artifacts = output / "artifacts"
    artifacts.mkdir(mode=0o700)
    shutil.copyfile(binary, artifacts / "llama-server")
    (artifacts / "llama-server").chmod(0o755)
    shutil.copyfile(source / "LICENSE", artifacts / "llama.cpp-LICENSE")
    notices = stage_notices(source, artifacts)
    record = {"schema": 1, "sourceRevision": REVISION, "cmakeOptions": OPTIONS,
              "cmakeCacheSHA256": cache_digest,
              "dynamicLibraries": libraries, "interpreter": INTERPRETER,
              "sourceNotices": notices, "licenseReviewComplete": False,
              "binarySHA256": checksum, "binaryBytes": info.st_size,
              "releaseQualified": False, "modelIncluded": False}
    with (artifacts / "development-runtime.json").open("x") as file:
        json.dump(record, file, indent=2)
        file.write("\n")
    return record


if __name__ == "__main__":
    try:
        print(json.dumps(build(Path(sys.argv[1]), Path(sys.argv[2]))))
    except (OSError, ValueError, IndexError, UnicodeError, subprocess.SubprocessError) as error:
        reason = str(error)[:240] if type(error) is ValueError else type(error).__name__
        sys.exit("Development inference build failed: " + reason + "; retain build diagnostics")
