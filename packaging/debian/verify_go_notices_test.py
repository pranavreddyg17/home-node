#!/usr/bin/env python3
import copy
import json
import pathlib
import tempfile
import unittest

from go_notices import collect
from verify_go_notices import verify


class GoNoticeVerificationTests(unittest.TestCase):
    def test_source_and_inventory_substitutions(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            (root / 'LICENSE').write_text('reviewed fixture notice')
            dependency = {'Path': 'fixture.example/dep', 'Version': 'v1.0.0', 'Sum': 'fixture-sum'}
            compiled = {'schema': 1, 'sourceSumsVerified': True,
                        'binaries': [{'dependencies': [dependency]}]}
            sources = [dict(dependency, Dir=str(root))]
            paths = [root / name for name in ['compiled.json', 'sources.json', 'notices.json']]
            paths[0].write_text(json.dumps(compiled))
            paths[1].write_text(json.dumps(sources[0]))
            notices = collect(compiled, sources)
            paths[2].write_text(json.dumps(notices))
            verify(*paths)
            mutations = []
            for field in ['module', 'version', 'sum']:
                changed = copy.deepcopy(notices)
                changed['modules'][0][field] = 'foreign'
                mutations.append(changed)
            changed = copy.deepcopy(notices)
            changed['modules'][0]['licenseFiles'][0]['text'] = 'foreign'
            mutations.extend([changed, dict(notices, modules=[]),
                              dict(notices, modules=notices['modules'] * 2),
                              dict(notices, schema=True), dict(notices, extra=True)])
            for mutation in mutations:
                paths[2].write_text(json.dumps(mutation))
                with self.assertRaises(ValueError):
                    verify(*paths)
            paths[2].write_text(json.dumps(notices))
            (root / 'LICENSE').write_text('changed source bytes')
            with self.assertRaises(ValueError):
                verify(*paths)
            (root / 'LICENSE').unlink()
            (root / 'LICENSE').symlink_to(paths[0])
            with self.assertRaises(OSError):
                verify(*paths)


if __name__ == '__main__':
    unittest.main()
