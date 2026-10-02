#!/usr/bin/env python3
"""Compare development file SBOM against independently extracted package bytes."""
import hashlib
import json
import pathlib
import sys


def unique(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON key")
        result[key] = value
    return result


def sha256(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def verify(root, archive, version, evidence):
    root, archive, evidence = map(pathlib.Path, (root, archive, evidence))
    if evidence.is_symlink() or not evidence.is_file() or evidence.stat().st_size > 8 * 1024 * 1024:
        raise ValueError("invalid evidence file")
    bom = json.loads(evidence.read_bytes(), object_pairs_hook=unique)
    if not isinstance(bom, dict) or not isinstance(bom.get("metadata"), dict):
        raise ValueError("invalid BOM object")
    if bom.get("compositions") != [{"aggregate": "incomplete", "assemblies": ["homenode-package"]}]:
        raise ValueError("development inventory must declare incomplete composition")
    expected = {"type": "application", "bom-ref": "homenode-package", "name": "homenode",
                "version": version, "hashes": [{"alg": "SHA-256", "content": sha256(archive)}]}
    if bom.get("bomFormat") != "CycloneDX" or bom.get("specVersion") != "1.6" or bom.get("metadata", {}).get("component") != expected:
        raise ValueError("package binding mismatch")
    claims = bom.get("components")
    if not isinstance(claims, list) or not 1 <= len(claims) <= 4096:
        raise ValueError("invalid file inventory")
    if (root / "usr").is_symlink() or not (root / "usr").is_dir():
        raise ValueError("payload root must be a real directory")
    actual = {}
    for path in (root / "usr").rglob("*"):
        if path.is_symlink():
            raise ValueError("payload link")
        if path.is_dir():
            continue
        if not path.is_file():
            raise ValueError("payload special file")
        relative = path.relative_to(root).as_posix()
        if len(relative.encode("utf-8")) > 240 or len(actual) >= 4096:
            raise ValueError("payload inventory limit exceeded")
        actual[relative] = sha256(path)
    retained = set()
    for claim in claims:
        if not isinstance(claim, dict) or not isinstance(claim.get("name"), str):
            raise ValueError("invalid file claim")
        name = claim["name"]
        if name in retained or name not in actual:
            raise ValueError("duplicate or foreign file claim")
        retained.add(name)
        if claim != {"type": "file", "bom-ref": "file:" + name, "name": name,
                     "hashes": [{"alg": "SHA-256", "content": actual[name]}]}:
            raise ValueError("file binding mismatch")
    if retained != set(actual):
        raise ValueError("missing file claim")


if __name__ == "__main__":
    if len(sys.argv) != 5:
        raise SystemExit("usage: verify_sbom.py EXTRACTED_ROOT PACKAGE VERSION SBOM")
    verify(*sys.argv[1:])
