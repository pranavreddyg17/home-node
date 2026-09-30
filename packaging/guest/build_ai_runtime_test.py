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


class RuntimeDependencyTests(unittest.TestCase):
    def dynamic(self):
        return "\n".join(" 0x0000000000000001 (NEEDED) Shared library: [" + name + "]"
                         for name in sorted(runtime.SYSTEM_LIBRARIES))

    def program(self):
        return "      [Requesting program interpreter: " + runtime.INTERPRETER + "]\n"

    def test_system_runtime_inventory(self):
        self.assertEqual(runtime.dependencies(self.dynamic(), self.program()), sorted(runtime.SYSTEM_LIBRARIES))

    def test_foreign_loader_paths_libraries_or_ambiguous_reports_refused(self):
        cases = [
            (self.dynamic().replace("libc.so.6", "libforeign.so"), self.program()),
            (self.dynamic().replace("ld-linux-x86-64.so.2", "ld-foreign.so.2"), self.program()),
            (self.dynamic() + "\n 0x1 (NEEDED) Shared library: [libc.so.6]", self.program()),
            (self.dynamic() + "\n 0x1 (NEEDED) malformed", self.program()),
            (self.dynamic(), self.program().replace(runtime.INTERPRETER, "/tmp/loader")),
            (self.dynamic(), self.program() * 2),
            (self.dynamic(), ""),
            ("x" * 65537, self.program()),
        ]
        cases.extend((self.dynamic() + "\n 0x1 (" + tag + ") [/tmp/runtime]", self.program())
                     for tag in ("RPATH", "RUNPATH", "AUDIT", "DEPAUDIT", "FILTER", "AUXILIARY"))
        for dynamic, program in cases:
            with self.subTest(dynamic=dynamic[:50], program=program[:50]), self.assertRaises(ValueError):
                runtime.dependencies(dynamic, program)


class SourceNoticeTests(unittest.TestCase):
    def test_notices_preserve_source_bytes_and_identity(self):
        with tempfile.TemporaryDirectory() as directory:
            source, artifacts = Path(directory) / "source", Path(directory) / "artifacts"
            source.mkdir()
            artifacts.mkdir()
            expected = []
            for name in runtime.NOTICE_FILES:
                path = source / name
                path.parent.mkdir(parents=True, exist_ok=True)
                data = ("notice fixture " + name + "\n").encode()
                path.write_bytes(data)
                expected.append({"sourcePath": name, "bytes": len(data), "sha256": hashlib.sha256(data).hexdigest()})
            self.assertEqual(runtime.stage_notices(source, artifacts), expected)
            for name in runtime.NOTICE_FILES:
                self.assertEqual((artifacts / "notices" / name).read_bytes(), (source / name).read_bytes())
            with self.assertRaises(FileExistsError):
                runtime.stage_notices(source, artifacts)

    def test_missing_or_symlinked_notice_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            source, artifacts = Path(directory) / "source", Path(directory) / "artifacts"
            source.mkdir()
            artifacts.mkdir()
            with self.assertRaises(FileNotFoundError):
                runtime.stage_notices(source, artifacts)
            (artifacts / "notices").rmdir()
            (source / "LICENSE").symlink_to("/dev/null")
            with self.assertRaises(OSError):
                runtime.stage_notices(source, artifacts)


if __name__ == "__main__":
    unittest.main()
