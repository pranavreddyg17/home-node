#!/usr/bin/env python3
import hashlib
import io
import json
import pathlib
import tempfile
import unittest

from go_notices import collect, read_sources, run


class CompiledGoNoticeTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = pathlib.Path(temporary.name)
        (self.root / 'LICENSE').write_bytes(b'fixture notice')
        self.dependency = {'Path': 'fixture.example/dep', 'Version': 'v1.0.0', 'Sum': 'fixture-sum'}
        self.evidence = {'schema': 1, 'sourceSumsVerified': True,
                         'binaries': [{'dependencies': [self.dependency]}]}
        self.sources = [dict(self.dependency, Dir=str(self.root))]

    def test_exact_compiled_source_and_notice_identity(self):
        result = collect(self.evidence, self.sources)
        self.assertEqual(result['completeness'], 'incomplete')
        self.assertEqual(result['modules'][0]['licenseFiles'], [{'path': 'LICENSE',
            'sha256': hashlib.sha256(b'fixture notice').hexdigest(), 'text': 'fixture notice'}])
        self.assertNotIn(str(self.root), str(result))

    def test_unqualified_or_wrong_source_refusal(self):
        for field, value in [('Version', 'v2.0.0'), ('Sum', 'foreign'), ('Path', 'foreign')]:
            with self.subTest(field=field):
                source = dict(self.sources[0], **{field: value})
                with self.assertRaises(ValueError):
                    collect(self.evidence, [source])
        with self.assertRaises(ValueError):
            collect(dict(self.evidence, sourceSumsVerified=False), self.sources)
        with self.assertRaises(ValueError):
            collect(self.evidence, self.sources * 2)

    def test_notice_link_refusal(self):
        notice = self.root / 'LICENSE'
        notice.rename(self.root / 'original')
        notice.symlink_to(self.root / 'original')
        with self.assertRaises(OSError):
            collect(self.evidence, self.sources)

    def test_cli_complete_stream_and_atomic_validation(self):
        evidence = self.root / 'compiled.json'
        sources = self.root / 'sources.json'
        evidence.write_text(json.dumps(self.evidence))
        sources.write_text(json.dumps({'Main': True, 'Path': 'fixture.example/main'})
                           + '\n' + json.dumps(self.sources[0]) + '\n')
        output = io.BytesIO()
        run([evidence, sources], output)
        self.assertEqual(json.loads(output.getvalue()), collect(self.evidence, self.sources))
        for content in ['{}\n{"Path":"a","Path":"b"}', '{} trailing', '[]', '   ']:
            sources.write_text(content)
            output = io.BytesIO()
            with self.assertRaises(ValueError):
                run([evidence, sources], output)
            self.assertEqual(output.getvalue(), b'')

    def test_source_stream_bounds_and_links(self):
        sources = self.root / 'sources.json'
        sources.write_text('{}\n' * 4097)
        with self.assertRaises(ValueError):
            read_sources(sources)
        sources.unlink()
        sources.symlink_to(self.root / 'LICENSE')
        with self.assertRaises(OSError):
            read_sources(sources)
        with self.assertRaises(ValueError):
            collect(dict(self.evidence, schema=True), self.sources)


if __name__ == '__main__':
    unittest.main()
