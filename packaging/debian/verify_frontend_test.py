#!/usr/bin/env python3
import copy
import hashlib
import json
import pathlib
import tempfile
import unittest

from verify_frontend import verify


class FrontendEvidenceTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.directory = pathlib.Path(temporary.name)
        self.root = self.directory / 'web'
        self.root.mkdir()
        self.asset = self.root / 'index.html'
        self.asset.write_bytes(b'frontend')
        self.evidence = self.directory / 'evidence.json'
        self.lock = self.directory / 'lock.json'
        self.lock.write_text(json.dumps({'packages': {'node_modules/react': {
            'version': '1.0.0', 'integrity': 'fixture', 'resolved': 'fixture-source'}}}))
        self.manifest = self.directory / 'node_modules/react/package.json'
        self.manifest.parent.mkdir(parents=True)
        self.manifest.write_bytes(b'{"name":"react","version":"1.0.0"}')
        self.record = {'schema': 1, 'completeness': 'incomplete',
            'assets': [{'path': 'index.html', 'sha256': hashlib.sha256(b'frontend').hexdigest()}],
            'dependencies': [{'path': 'node_modules/react', 'name': 'react',
                'version': '1.0.0', 'integrity': 'fixture', 'resolved': 'fixture-source',
                'manifestSHA256': hashlib.sha256(self.manifest.read_bytes()).hexdigest()}]}
        self.write(self.record)

    def write(self, record):
        self.evidence.write_text(json.dumps(record))

    def check(self):
        verify(self.root, self.evidence, self.lock)

    def test_matching_assets_and_lock(self):
        self.check()

    def test_changed_asset_and_unclaimed_asset(self):
        self.asset.write_bytes(b'changed')
        with self.assertRaises(ValueError):
            self.check()
        self.asset.write_bytes(b'frontend')
        (self.root / 'extra.js').write_bytes(b'extra')
        with self.assertRaises(ValueError):
            self.check()

    def test_asset_and_dependency_claim_mutations(self):
        for mutation in ('missing', 'duplicate', 'duplicate-dependency', 'foreign', 'version', 'integrity', 'resolved', 'name', 'complete'):
            with self.subTest(mutation=mutation):
                record = copy.deepcopy(self.record)
                if mutation == 'missing':
                    record['assets'] = []
                elif mutation == 'duplicate':
                    record['assets'].append(copy.deepcopy(record['assets'][0]))
                elif mutation == 'duplicate-dependency':
                    record['dependencies'].append(copy.deepcopy(record['dependencies'][0]))
                elif mutation == 'foreign':
                    record['dependencies'][0]['path'] = 'node_modules/foreign'
                elif mutation == 'complete':
                    record['completeness'] = 'complete'
                else:
                    record['dependencies'][0][mutation] = 'changed'
                self.write(record)
                with self.assertRaises(ValueError):
                    self.check()

    def test_payload_and_evidence_links(self):
        self.asset.unlink()
        self.asset.symlink_to(self.lock)
        with self.assertRaises(ValueError):
            self.check()
        self.asset.unlink()
        self.asset.write_bytes(b'frontend')
        self.evidence.unlink()
        self.evidence.symlink_to(self.lock)
        with self.assertRaises(ValueError):
            self.check()

    def test_malformed_shapes_and_boolean_schema(self):
        variants = [None, [], dict(self.record, schema=True),
                    dict(self.record, assets=[None]),
                    dict(self.record, dependencies=[{'path': []}])]
        for record in variants:
            with self.subTest(record=record):
                self.write(record)
                with self.assertRaises(ValueError):
                    self.check()

    def test_changed_manifest_with_retained_version(self):
        self.manifest.write_bytes(b'{"name":"react","version":"1.0.0","extra":true}')
        with self.assertRaises(ValueError):
            self.check()

    def test_duplicate_json_key(self):
        self.evidence.write_text('{"schema":1,' + self.evidence.read_text()[1:])
        with self.assertRaises(ValueError):
            self.check()


if __name__ == '__main__':
    unittest.main()
