#!/usr/bin/env python3
"""Build an unsigned development guest image on an explicit disposable Linux runner."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import sys
sys.dont_write_bytecode = True
import import_overlay

MKOSI_REVISION = "52448a27f6f869c108352ed82fcfb9be633703fb"


def build(archive, digest, profile, revision, output, tool, ai_payload=None):
    if sys.platform != "linux" or os.geteuid() != 0 or os.getenv("HOMENODE_GUEST_IMAGE_BUILD") != "1":
        raise ValueError("requires explicit disposable Linux image builder")
    if profile not in ("files", "video", "ai") or (profile == "ai") != (ai_payload is not None):
        raise ValueError("expected matching profile and AI payload")
    parent = output.parent.lstat()
    if not output.is_absolute() or output != Path(os.path.abspath(output)) or not stat.S_ISDIR(parent.st_mode) or parent.st_uid != 0 or stat.S_IMODE(parent.st_mode) & 0o077:
        raise ValueError("expected private root-owned output parent")
    if not tool.is_absolute() or tool.is_symlink() or not tool.is_dir() or tool.stat().st_uid != 0 or stat.S_IMODE(tool.stat().st_mode) & 0o022:
        raise ValueError("expected root-owned pinned image tool checkout")
    command = ["/usr/bin/git", "-c", "core.fsmonitor=false", "-C", str(tool)]
    observed = subprocess.run(command + ["rev-parse", "HEAD"], check=True, capture_output=True, text=True, timeout=15).stdout.strip()
    changed = subprocess.run(command + ["status", "--porcelain", "--untracked-files=all"], check=True, capture_output=True, text=True, timeout=15).stdout
    if observed != MKOSI_REVISION or changed:
        raise ValueError("image tool source identity mismatch")
    os.mkdir(output, 0o700)  # exclusive; retain failed assembly for diagnosis
    import_overlay.import_overlay(archive, output / "overlay", digest, profile, revision)
    source = Path(__file__).resolve().parent
    shutil.copytree(source / "image", output / "recipe")
    # Only these reviewed helper sources and the verified overlay enter scripts.
    guest = output / "recipe/guest"
    guest.mkdir(mode=0o755)
    for name in ("finalize_root.py", "identity.py", "overlay.py"):
        shutil.copyfile(source / name, guest / name)
        (guest / name).chmod(0o644)
    if profile == "ai":
        import stage_ai_payload
        stage_ai_payload.verify(ai_payload)
        shutil.copytree(ai_payload, output / "recipe/ai-payload")
        stage_ai_payload.verify(output / "recipe/ai-payload")
        for name in ("stage_ai_payload.py", "build_ai_runtime.py", "download_ai_model.py", "homenode-model.service", "homenode-ai-model.conf"):
            shutil.copyfile(source / name, guest / name)
            (guest / name).chmod(0o644)
        configuration = output / "recipe/mkosi.conf"
        text = configuration.read_text()
        if text.count("ExtraTrees=overlay\n") != 1 or text.count("BuildSources=guest:guest,overlay:overlay\n") != 1:
            raise ValueError("unexpected AI assembly recipe")
        text = text.replace("ExtraTrees=overlay\n", "ExtraTrees=overlay,ai-payload\n")
        text = text.replace("BuildSources=guest:guest,overlay:overlay\n", "BuildSources=guest:guest,overlay:overlay,ai-payload:ai-payload\n")
        configuration.write_text(text)
    # Move the new owned overlay into the recipe, without touching base/host data.
    os.rename(output / "overlay", output / "recipe/overlay")
    image_output = output / "artifacts"
    image_output.mkdir(mode=0o700)
    arguments = [str(tool / "bin/mkosi"), "--directory=" + str(output / "recipe"),
                 "--output-directory=" + str(image_output), "--output=homenode-development-" + profile]
    if profile == "video":
        arguments.append("--package=ffmpeg")
    if profile == "ai":
        arguments.append("--package=libstdc++6,libgomp1,libgcc-s1,libc6")
    subprocess.run(arguments + ["summary"], check=True, timeout=30)
    subprocess.run(arguments + ["build"], check=True, timeout=1200)
    image = image_output / ("homenode-development-" + profile + ".raw")
    fd = os.open(image, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as file:
        info = os.fstat(file.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_nlink != 1 or not 0 < info.st_size <= 8 << 30:
            raise ValueError("unexpected development image output")
        checksum = hashlib.sha256()
        while chunk := file.read(1 << 20):
            checksum.update(chunk)
    record = {"schema": 1, "profile": profile, "sourceRevision": revision,
              "overlaySHA256": digest, "mkosiRevision": MKOSI_REVISION,
              "bytes": info.st_size, "sha256": checksum.hexdigest(),
              "releaseQualified": False, "bootValidated": False}
    with (image_output / "development-build.json").open("x") as file:
        json.dump(record, file, indent=2)
        file.write("\n")
        file.flush()
        os.fsync(file.fileno())
    return record


if __name__ == "__main__":
    try:
        result = build(Path(sys.argv[1]), sys.argv[2], sys.argv[3], sys.argv[4], Path(sys.argv[5]), Path(sys.argv[6]), Path(sys.argv[7]) if len(sys.argv) == 8 else None)
        print(json.dumps(result))
    except (OSError, ValueError, IndexError, subprocess.SubprocessError) as error:
        # Policy errors contain fixed messages; OS paths, process arguments and
        # account contents are deliberately absent from this summary.
        reason = str(error)[:240] if type(error) is ValueError else type(error).__name__
        sys.exit("Development image build failed: " + reason + "; retain assembly staging")
