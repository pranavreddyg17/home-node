#!/usr/bin/env python3
"""Independently compare compiled Go notice claims with source bytes."""
import hashlib
import json
import pathlib
import re
import sys

from go_notices import read_sources
from verify_frontend import manifest_bytes
from verify_sbom import unique


def verify(compiled_path, sources_path, notices_path, readable_path=None):
    compiled = json.loads(manifest_bytes(compiled_path, 8 * 1024 * 1024), object_pairs_hook=unique)
    claims = json.loads(manifest_bytes(notices_path, 8 * 1024 * 1024), object_pairs_hook=unique)
    if (not isinstance(compiled, dict) or type(compiled.get('schema')) is not int
            or compiled['schema'] != 1 or compiled.get('sourceSumsVerified') is not True):
        raise ValueError('unqualified compiled evidence')
    if (not isinstance(claims, dict) or set(claims) != {'schema', 'completeness', 'modules'}
            or type(claims['schema']) is not int or claims['schema'] != 1
            or claims['completeness'] != 'incomplete' or not isinstance(claims['modules'], list)
            or len(claims['modules']) > 4096):
        raise ValueError('invalid notice inventory')
    expected = {}
    for binary in compiled['binaries']:
        for original in binary['dependencies'] or []:
            dependency = original.get('Replace') or original
            identity = (dependency['Path'], dependency['Version'])
            if identity in expected and expected[identity] != dependency['Sum']:
                raise ValueError('conflicting compiled dependency')
            expected[identity] = dependency['Sum']
    sources = {}
    for entry in read_sources(sources_path):
        source = entry.get('Replace') or entry
        identity = (source.get('Path'), source.get('Version'))
        if identity in sources:
            raise ValueError('duplicate source identity')
        sources[identity] = source
    seen = set()
    for module in claims['modules']:
        if not isinstance(module, dict) or set(module) != {'module', 'version', 'sum', 'licenseFiles'}:
            raise ValueError('invalid module claim')
        identity = (module['module'], module['version'])
        if identity in seen or identity not in expected or module['sum'] != expected[identity]:
            raise ValueError('foreign or duplicate compiled module claim')
        seen.add(identity)
        source = sources.get(identity)
        if not source or not module['sum'] or source.get('Sum') != module['sum']:
            raise ValueError('source identity mismatch')
        directory = pathlib.Path(source['Dir'])
        if directory.is_symlink() or not directory.is_dir():
            raise ValueError('unsafe module source directory')
        files = sorted(path for path in directory.iterdir()
                       if re.fullmatch(r'(LICENSE|COPYING|NOTICE)(\..*)?', path.name, re.IGNORECASE))
        if len(files) > 16:
            raise ValueError('excessive module notices')
        actual = []
        for path in files:
            data = manifest_bytes(path)
            actual.append({'path': path.name, 'sha256': hashlib.sha256(data).hexdigest(),
                           'text': data.decode('utf-8')})
        if module['licenseFiles'] != actual:
            raise ValueError('module notice source mismatch')
    if seen != set(expected):
        raise ValueError('missing compiled module notices')
    if readable_path is not None:
        text = 'HomeNode development Go notices\nIncomplete source collection; license review remains required.\n'
        for module in sorted(claims['modules'], key=lambda value: (value['module'], value['version'])):
            text += '\n%s@%s\nSource sum: %s\n' % (module['module'], module['version'], module['sum'])
            if not module['licenseFiles']:
                text += 'No matched module-root notice files collected.\n'
            for notice in module['licenseFiles']:
                text += 'Source: %s\nSHA256: %s\n%s\n' % (notice['path'], notice['sha256'], notice['text'])
        if manifest_bytes(readable_path, 8 * 1024 * 1024) != text.encode('utf-8'):
            raise ValueError('readable Go notices differ from verified sources')


if __name__ == '__main__':
    if len(sys.argv) not in (4, 5):
        raise ValueError('usage: verify_go_notices.py COMPILED_EVIDENCE SOURCE_STREAM NOTICES [READABLE_NOTICES]')
    verify(*sys.argv[1:])
