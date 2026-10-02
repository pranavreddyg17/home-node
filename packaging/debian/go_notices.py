#!/usr/bin/env python3
"""Collect Go notice sources for reviewed compiled module identities."""
import hashlib
import json
import pathlib
import re
import sys

from verify_frontend import manifest_bytes
from verify_sbom import unique

LIMIT = 8 * 1024 * 1024


def read_sources(path):
    """Read Go's concatenated module objects without accepting duplicate keys."""
    data = manifest_bytes(path, LIMIT).decode('utf-8')
    decoder = json.JSONDecoder(object_pairs_hook=unique)
    sources, offset = [], 0
    while offset < len(data):
        if data[offset].isspace():
            offset += 1
            continue
        source, offset = decoder.raw_decode(data, offset)
        if not isinstance(source, dict) or len(sources) >= 4096:
            raise ValueError('invalid or excessive module source metadata')
        sources.append(source)
    if not sources:
        raise ValueError('empty module source metadata')
    return sources


def render_notices(result):
    parts = ['HomeNode development Go notices\nIncomplete source collection; license review remains required.\n']
    for module in result['modules']:
        parts.append('\n' + module['module'] + '@' + module['version'] + '\nSource sum: ' + module['sum'] + '\n')
        if not module['licenseFiles']:
            parts.append('No matched module-root notice files collected.\n')
        for notice in module['licenseFiles']:
            parts.append('Source: ' + notice['path'] + '\nSHA256: ' + notice['sha256'] + '\n' + notice['text'] + '\n')
    data = ''.join(parts).encode('utf-8')
    if len(data) > LIMIT:
        raise ValueError('readable module notices exceed limit')
    return data


def run(arguments, output):
    if len(arguments) not in (2, 3):
        raise ValueError('usage: go_notices.py COMPILED_EVIDENCE MODULE_SOURCE_STREAM [READABLE_NOTICES]')
    evidence = json.loads(manifest_bytes(arguments[0], LIMIT), object_pairs_hook=unique)
    result = collect(evidence, read_sources(arguments[1]))
    encoded = (json.dumps(result, ensure_ascii=False, sort_keys=True) + '\n').encode('utf-8')
    if len(encoded) > LIMIT:
        raise ValueError('module notice output exceeds limit')
    readable = render_notices(result)
    if len(arguments) == 3:
        pathlib.Path(arguments[2]).write_bytes(readable)
    if output.write(encoded) != len(encoded):
        raise OSError('short module notice output write')


def collect(evidence, sources):
    if (not isinstance(evidence, dict) or type(evidence.get('schema')) is not int
            or evidence['schema'] != 1 or evidence.get('sourceSumsVerified') is not True):
        raise ValueError('compiled source sums must be qualified first')
    index = {}
    for source in sources:
        effective = source.get('Replace') or source
        key = (effective.get('Path'), effective.get('Version'))
        if key in index:
            raise ValueError('ambiguous module source metadata')
        index[key] = effective
    retained = {}
    for binary in evidence['binaries']:
        for original in binary['dependencies'] or []:
            dependency = original.get('Replace') or original
            key = (dependency['Path'], dependency['Version'])
            if key in retained:
                if retained[key]['sum'] != dependency['Sum']:
                    raise ValueError('conflicting compiled module source sum')
                continue
            source = index.get(key)
            if not source or not dependency['Sum'] or source.get('Sum') != dependency['Sum']:
                raise ValueError('source metadata differs from compiled module')
            directory = pathlib.Path(source['Dir'])
            if directory.is_symlink() or not directory.is_dir():
                raise ValueError('invalid module source directory')
            notices = []
            for path in sorted(directory.iterdir()):
                if not re.fullmatch(r'(LICENSE|COPYING|NOTICE)(\..*)?', path.name, re.IGNORECASE):
                    continue
                if len(notices) >= 16:
                    raise ValueError('too many module notice files')
                data = manifest_bytes(path)
                notices.append({'path': path.name, 'sha256': hashlib.sha256(data).hexdigest(),
                                'text': data.decode('utf-8')})
            retained[key] = {'module': key[0], 'version': key[1], 'sum': dependency['Sum'],
                             'licenseFiles': notices}
            if len(retained) > 4096:
                raise ValueError('too many compiled modules')
    return {'schema': 1, 'completeness': 'incomplete',
            'modules': [retained[key] for key in sorted(retained)]}


if __name__ == '__main__':
    run(sys.argv[1:], sys.stdout.buffer)
