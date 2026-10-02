#!/usr/bin/env python3
"""Emit development file evidence, not a qualified dependency/license SBOM."""
import hashlib
import json
import pathlib
import sys


def digest(path):
    value = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def document(root, archive, version):
    root = pathlib.Path(root)
    archive = pathlib.Path(archive)
    if not archive.is_file() or archive.is_symlink():
        raise ValueError("package must be a regular file")
    if (root / "usr").is_symlink() or not (root / "usr").is_dir():
        raise ValueError("payload root must be a real directory")
    files = []
    for path in sorted((root / "usr").rglob("*")):
        if path.is_symlink():
            raise ValueError("payload links are unsupported")
        if path.is_dir():
            continue
        if not path.is_file():
            raise ValueError("payload special files are unsupported")
        relative = path.relative_to(root).as_posix()
        if len(relative.encode("utf-8")) > 240 or len(files) >= 4096:
            raise ValueError("payload inventory limit exceeded")
        files.append({"type": "file", "bom-ref": "file:" + relative,
                      "name": relative,
                      "hashes": [{"alg": "SHA-256", "content": digest(path)}]})
    if not files:
        raise ValueError("empty payload inventory")
    return {"bomFormat": "CycloneDX", "specVersion": "1.6", "version": 1,
            "metadata": {"component": {"type": "application", "bom-ref": "homenode-package",
                "name": "homenode", "version": version,
                "hashes": [{"alg": "SHA-256", "content": digest(archive)}]}},
            "components": files,
            "compositions": [{"aggregate": "incomplete", "assemblies": ["homenode-package"]}]}


if __name__ == "__main__":
    if len(sys.argv) != 4:
        raise SystemExit("usage: sbom.py STAGED_ROOT PACKAGE VERSION")
    encoded = json.dumps(document(*sys.argv[1:]), sort_keys=True, separators=(",", ":")).encode("utf-8") + b"\n"
    if len(encoded) > 8 * 1024 * 1024:
        raise SystemExit("SBOM exceeds evidence limit")
    sys.stdout.buffer.write(encoded)
