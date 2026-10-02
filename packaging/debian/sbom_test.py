#!/usr/bin/env python3
import copy
import json
import pathlib
import tempfile
import unittest

from sbom import document
from verify_sbom import verify


class PackageEvidenceTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = pathlib.Path(self.temporary.name)
        (self.root / 'usr').mkdir()
        self.payload = self.root / 'usr/app'
        self.payload.write_bytes(b'payload')
        self.archive = self.root / 'package.deb'
        self.archive.write_bytes(b'archive')
        self.evidence = self.root / 'sbom.json'
        self.bom = document(self.root, self.archive, '1')
        self.write(self.bom)

    def write(self, bom):
        self.evidence.write_text(json.dumps(bom))

    def check(self):
        verify(self.root, self.archive, '1', self.evidence)

    def test_matching_archive_and_payload(self):
        self.check()

    def test_changed_package_or_payload(self):
        for path in (self.archive, self.payload):
            with self.subTest(path=path.name):
                original = path.read_bytes()
                path.write_bytes(b'changed')
                with self.assertRaises(ValueError):
                    self.check()
                path.write_bytes(original)

    def test_incomplete_foreign_duplicate_and_wrong_claims(self):
        for mutation in ('missing', 'foreign', 'duplicate', 'wrong-hash', 'wrong-version'):
            with self.subTest(mutation=mutation):
                bom = copy.deepcopy(self.bom)
                if mutation == 'missing':
                    bom['components'] = []
                elif mutation == 'foreign':
                    bom['components'][0]['name'] = 'usr/foreign'
                elif mutation == 'duplicate':
                    bom['components'].append(copy.deepcopy(bom['components'][0]))
                elif mutation == 'wrong-hash':
                    bom['components'][0]['hashes'][0]['content'] = '0' * 64
                else:
                    bom['metadata']['component']['version'] = '2'
                self.write(bom)
                with self.assertRaises(ValueError):
                    self.check()

    def test_extra_extracted_file_and_link(self):
        extra = self.root / 'usr/extra'
        extra.write_bytes(b'extra')
        with self.assertRaises(ValueError):
            self.check()
        extra.unlink()
        extra.symlink_to(self.payload)
        with self.assertRaises(ValueError):
            self.check()
        with self.assertRaises(ValueError):
            document(self.root, self.archive, '1')

    def test_payload_root_link(self):
        (self.root / 'usr').rename(self.root / 'foreign')
        (self.root / 'usr').symlink_to(self.root / 'foreign', target_is_directory=True)
        with self.assertRaises(ValueError):
            document(self.root, self.archive, '1')
        with self.assertRaises(ValueError):
            self.check()

    def test_inventory_count_limit(self):
        for number in range(4096):
            (self.root / 'usr' / str(number)).write_bytes(b'x')
        with self.assertRaises(ValueError):
            document(self.root, self.archive, '1')
        with self.assertRaises(ValueError):
            self.check()

    def test_payload_path_limit(self):
        directory = self.root / 'usr' / ('a' * 120)
        directory.mkdir()
        (directory / ('b' * 120)).write_bytes(b'x')
        with self.assertRaises(ValueError):
            document(self.root, self.archive, '1')
        with self.assertRaises(ValueError):
            self.check()

    def test_duplicate_json_key(self):
        self.evidence.write_text('{"bomFormat":"CycloneDX",' + self.evidence.read_text()[1:])
        with self.assertRaises(ValueError):
            self.check()


if __name__ == '__main__':
    unittest.main()
