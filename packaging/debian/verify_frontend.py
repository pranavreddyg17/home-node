#!/usr/bin/env python3
"""Check frontend evidence against extracted assets and reviewed npm lock entries."""
import hashlib
import json
import pathlib
import os
import stat
import sys

from verify_sbom import unique


def manifest_bytes(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or not 1 <= before.st_size <= 1024 * 1024:
            raise ValueError('invalid installed package manifest')
        data = bytearray()
        while len(data) <= 1024 * 1024:
            chunk = os.read(fd, min(65536, 1024 * 1024 + 1 - len(data)))
            if not chunk:
                break
            data.extend(chunk)
        after = os.fstat(fd)
        if len(data) != before.st_size or after.st_size != before.st_size or after.st_mtime_ns != before.st_mtime_ns:
            raise ValueError('changed installed package manifest')
        return bytes(data)
    finally:
        os.close(fd)


def verify(web_root, evidence, lockfile):
    root, evidence, lockfile = map(pathlib.Path, (web_root, evidence, lockfile))
    if root.is_symlink() or not root.is_dir():
        raise ValueError('invalid web payload root')
    if evidence.is_symlink() or not evidence.is_file() or evidence.stat().st_size > 8 * 1024 * 1024:
        raise ValueError('invalid frontend evidence')
    record = json.loads(evidence.read_bytes(), object_pairs_hook=unique)
    if not isinstance(record, dict) or set(record) != {'schema', 'completeness', 'assets', 'dependencies'} or type(record.get('schema')) is not int or record.get('schema') != 1 or record.get('completeness') != 'incomplete':
        raise ValueError('invalid frontend evidence schema')
    assets = record.get('assets')
    if not isinstance(assets, list) or not 1 <= len(assets) <= 4096:
        raise ValueError('invalid assets')
    actual = {}
    for path in root.rglob('*'):
        if path.is_symlink():
            raise ValueError('web payload link')
        if path.is_dir():
            continue
        if not path.is_file():
            raise ValueError('web payload special file')
        name = path.relative_to(root).as_posix()
        if len(name.encode('utf-8')) > 240 or len(actual) >= 4096:
            raise ValueError('web payload inventory limit')
        with path.open('rb') as stream:
            actual[name] = hashlib.file_digest(stream, 'sha256').hexdigest()
    names = set()
    for asset in assets:
        if not isinstance(asset, dict) or not isinstance(asset.get('path'), str):
            raise ValueError('invalid asset claim')
        name = asset['path']
        if name in names or name not in actual or asset != {'path': name, 'sha256': actual[name]}:
            raise ValueError('asset identity mismatch')
        names.add(name)
    if names != set(actual):
        raise ValueError('unclaimed web asset')
    locked = json.loads(lockfile.read_bytes(), object_pairs_hook=unique)['packages']
    dependencies = record.get('dependencies')
    if not isinstance(dependencies, list) or not 1 <= len(dependencies) <= 4096:
        raise ValueError('invalid dependency inventory')
    seen = set()
    for dependency in dependencies:
        if not isinstance(dependency, dict) or set(dependency) != {'path', 'name', 'version', 'integrity', 'resolved', 'license', 'manifestSHA256'} or not isinstance(dependency.get('path'), str):
            raise ValueError('invalid dependency claim')
        key = dependency['path']
        entry = locked.get(key)
        if key in seen or not isinstance(entry, dict) or '/node_modules/' not in '/' + key:
            raise ValueError('foreign or duplicate dependency')
        seen.add(key)
        if not key.startswith('node_modules/') or pathlib.PurePosixPath(key).as_posix() != key or '..' in pathlib.PurePosixPath(key).parts:
            raise ValueError('invalid installed package path')
        manifest = lockfile.parent / key / 'package.json'
        if manifest.is_symlink() or not manifest.is_file() or manifest.stat().st_size > 1024 * 1024:
            raise ValueError('invalid installed package manifest')
        data = manifest_bytes(manifest)
        if dependency.get('manifestSHA256') != hashlib.sha256(data).hexdigest():
            raise ValueError('installed package manifest mismatch')
        name = key.rsplit('node_modules/', 1)[1]
        installed = json.loads(data, object_pairs_hook=unique)
        if not isinstance(installed, dict) or installed.get('name') != name or installed.get('version') != dependency['version'] or installed.get('license') != dependency['license']:
            raise ValueError('installed manifest identity/license mismatch')
        if dependency.get('name') != name or not entry.get('version') or dependency.get('version') != entry['version'] or dependency.get('integrity') != entry.get('integrity') or dependency.get('resolved') != entry.get('resolved'):
            raise ValueError('dependency lock identity mismatch')


if __name__ == '__main__':
    if len(sys.argv) != 4:
        raise SystemExit('usage: verify_frontend.py EXTRACTED_WEB EVIDENCE LOCKFILE')
    verify(*sys.argv[1:])
