import hashlib
import os
from pathlib import Path
import tempfile
import unittest
import build_ai_runtime as runtime


class CompilerConfigurationTests(unittest.TestCase):
    def cache(self):
        return "# CMake cache fixture\n" + "\n".join(
            key + (":STRING=" if key == "CMAKE_BUILD_TYPE" else ":BOOL=") + value
            for key, value in runtime.OPTIONS.items()) + "\nUNRELATED:STRING=value\n"

    def test_admitted_configuration_digest(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "CMakeCache.txt"
            content = self.cache().encode()
            path.write_bytes(content)
            self.assertEqual(runtime.check_cache(path), hashlib.sha256(content).hexdigest())

    def test_ignored_reenabled_or_ambiguous_options_refused(self):
        content = self.cache()
        mutations = [
            content.replace("GGML_NATIVE:BOOL=OFF", "GGML_NATIVE:UNINITIALIZED=OFF"),
            content.replace("GGML_NATIVE:BOOL=OFF", "GGML_NATIVE:BOOL=ON"),
            content.replace("GGML_NATIVE:BOOL=OFF\n", ""),
            content + "GGML_NATIVE:BOOL=OFF\n",
            content.replace("LLAMA_USE_PREBUILT_UI:BOOL=OFF", "LLAMA_USE_PREBUILT_UI:BOOL=ON"),
            content + "\0",
            "x" * ((1 << 20) + 1),
        ]
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "CMakeCache.txt"
            for changed in mutations:
                with self.subTest(prefix=changed[:40]), self.assertRaises(ValueError):
                    path.write_text(changed)
                    runtime.check_cache(path)

    def test_symlink_and_multiple_link_inputs_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            target, path = Path(directory) / "target", Path(directory) / "CMakeCache.txt"
            target.write_text(self.cache())
            path.symlink_to(target)
            with self.assertRaises(OSError):
                runtime.check_cache(path)
            path.unlink()
            os.link(target, path)
            with self.assertRaises(ValueError):
                runtime.check_cache(path)
            path.unlink()
            path.write_text(self.cache())
            path.chmod(0o666)
            with self.assertRaises(ValueError):
                runtime.check_cache(path)


if __name__ == "__main__":
    unittest.main()
