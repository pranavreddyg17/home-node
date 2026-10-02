#!/usr/bin/env python3
import copy
import hashlib
import json
import pathlib
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

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
                'license': None, 'licenseFiles': [], 'manifestSHA256': hashlib.sha256(self.manifest.read_bytes()).hexdigest()}]}
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
        for mutation in ('missing', 'duplicate', 'duplicate-dependency', 'foreign', 'version', 'integrity', 'resolved', 'name', 'license', 'complete', 'unknown'):
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
                elif mutation == 'unknown':
                    record['unreviewed'] = True
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

    def test_manifest_link_and_oversize_refusal(self):
        self.manifest.unlink()
        self.manifest.symlink_to(self.lock)
        with self.assertRaises(ValueError):
            self.check()
        self.manifest.unlink()
        self.manifest.write_bytes(b' ' * (1024 * 1024 + 1))
        with self.assertRaises(ValueError):
            self.check()

    def test_changed_manifest_during_descriptor_read(self):
        original_read = os.read
        before = self.manifest.stat()
        changed = False

        def mutate(fd, size):
            nonlocal changed
            data = original_read(fd, size)
            if data and not changed:
                changed = True
                self.manifest.write_bytes(b'x' * before.st_size)
                os.utime(self.manifest, ns=(before.st_atime_ns, before.st_mtime_ns + 1000000))
            return data

        with patch('verify_frontend.os.read', side_effect=mutate):
            with self.assertRaisesRegex(ValueError, 'changed installed package manifest'):
                self.check()
        self.assertTrue(changed)

    def test_short_descriptor_reads_preserve_exact_identity(self):
        original_read = os.read
        with patch('verify_frontend.os.read', side_effect=lambda fd, size: original_read(fd, min(size, 3))):
            self.check()

    def test_cli_bytecode_setting_keeps_checkout_clean(self):
        for disabled in (False, True):
            with self.subTest(bytecode_disabled=disabled):
                tools = self.directory / ('disabled' if disabled else 'enabled')
                tools.mkdir()
                for name in ('verify_frontend.py', 'verify_sbom.py'):
                    shutil.copyfile(pathlib.Path(__file__).with_name(name), tools / name)
                environment = dict(os.environ)
                environment.pop('PYTHONDONTWRITEBYTECODE', None)
                if disabled:
                    environment['PYTHONDONTWRITEBYTECODE'] = '1'
                result = subprocess.run([sys.executable, str(tools / 'verify_frontend.py'),
                    str(self.root), str(self.evidence), str(self.lock)],
                    env=environment, capture_output=True, text=True, check=False)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual((tools / '__pycache__').exists(), not disabled)

    def test_notice_bytes_and_claims_are_bound(self):
        notice = self.manifest.parent / 'LICENSE'
        notice.write_bytes(b'fixture notice')
        self.record['dependencies'][0]['licenseFiles'] = [{'path': 'LICENSE',
            'sha256': hashlib.sha256(b'fixture notice').hexdigest(), 'text': 'fixture notice'}]
        self.write(self.record)
        self.check()
        self.record['dependencies'][0]['licenseFiles'][0]['text'] = 'changed claim'
        self.write(self.record)
        with self.assertRaisesRegex(ValueError, 'license/notice file mismatch'):
            self.check()

    def test_duplicate_json_key(self):
        self.evidence.write_text('{"schema":1,' + self.evidence.read_text()[1:])
        with self.assertRaises(ValueError):
            self.check()


if __name__ == '__main__':
    unittest.main()
