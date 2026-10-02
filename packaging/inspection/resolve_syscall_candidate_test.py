#!/usr/bin/env python3
"""Portable refusal tests; no native library or package-manager effects."""
import ctypes
import json
from pathlib import Path
import platform
import runpy
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

HERE = Path(__file__).resolve().parent

class CandidateRefusalTests(unittest.TestCase):
    def refuse(self, data, message):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            script = root / "resolve_syscall_candidate.py"
            script.write_bytes((HERE / script.name).read_bytes())
            (root / "syscall-source-candidate.json").write_bytes(data)
            with patch.object(sys, "platform", "linux"), patch.object(platform, "machine", return_value="x86_64"), patch.object(subprocess, "run", side_effect=AssertionError("package manager reached")) as process, patch.object(ctypes, "CDLL", side_effect=AssertionError("native library reached")) as library:
                with self.assertRaisesRegex(SystemExit, message):
                    runpy.run_path(str(script), run_name="__main__")
                process.assert_not_called()
                library.assert_not_called()

    def test_changed_source_hash_claim_cannot_admit_changed_names(self):
        candidate = json.loads((HERE / "syscall-source-candidate.json").read_bytes())
        candidate["candidateSyscalls"][0] = "write"
        self.refuse(json.dumps(candidate).encode(), "differs from reviewed exact artifact")

    def test_semantically_equal_reencoding_still_requires_exact_reviewed_artifact(self):
        candidate = json.loads((HERE / "syscall-source-candidate.json").read_bytes())
        self.refuse(json.dumps(candidate, sort_keys=True).encode(), "differs from reviewed exact artifact")

    def test_oversized_candidate_refuses_before_decode_and_effects(self):
        self.refuse(b"x" * 16385, "Oversized source candidate")

    def test_injected_duplicate_claim_refuses_before_effects(self):
        original = (HERE / "syscall-source-candidate.json").read_bytes()
        changed = original.replace(b'"schema": 1,', b'"schema": 1, "schema": 1,', 1)
        self.refuse(changed, "differs from reviewed exact artifact")

if __name__ == "__main__":
    unittest.main()
