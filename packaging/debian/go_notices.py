#!/usr/bin/env python3
"""Collect Go notice sources for reviewed compiled module identities."""
import hashlib
import pathlib
import re

from verify_frontend import manifest_bytes


def collect(evidence, sources):
    if evidence.get('schema') != 1 or evidence.get('sourceSumsVerified') is not True:
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
