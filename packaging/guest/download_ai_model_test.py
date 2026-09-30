import hashlib
import io
import unittest
from unittest.mock import patch
import download_ai_model as model


class ModelTransferTests(unittest.TestCase):
    def test_exact_transfer(self):
        content = b"test model bytes" * 100000
        output = io.BytesIO()
        model.copy_verified(io.BytesIO(content), output, len(content), hashlib.sha256(content).hexdigest())
        self.assertEqual(output.getvalue(), content)

    def test_mismatched_size_digest_and_deadline_refused(self):
        for content, size, digest in [(b"abc", 2, hashlib.sha256(b"abc").hexdigest()),
                                     (b"abc", 4, hashlib.sha256(b"abc").hexdigest()),
                                     (b"abc", 3, hashlib.sha256(b"wrong").hexdigest())]:
            with self.subTest(size=size), self.assertRaises(ValueError):
                model.copy_verified(io.BytesIO(content), io.BytesIO(), size, digest)
        with patch.object(model.time, "monotonic", side_effect=[0, 181]), self.assertRaises(ValueError):
            model.copy_verified(io.BytesIO(b"abc"), io.BytesIO(), 3, hashlib.sha256(b"abc").hexdigest())

    def test_download_redirect_boundary(self):
        for url in (model.URL, "https://cdn-lfs.hf.co/model", "https://cas-bridge.xethub.hf.co/model"):
            self.assertTrue(model.allowed_url(url))
        for url in ("http://huggingface.co/model", "https://huggingface.co.evil.test/model",
                    "https://evil.test/model", "https://user:secret@huggingface.co/model",
                    "https://huggingface.co:8443/model", "https://huggingface.co:invalid/model",
                    "https://[broken/model", "file:///tmp/model", "https://127.0.0.1/model"):
            self.assertFalse(model.allowed_url(url))


if __name__ == "__main__":
    unittest.main()
