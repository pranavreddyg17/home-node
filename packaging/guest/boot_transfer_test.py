"""Transfer fixture must not mistake a foreign peer grant for a passing denial."""
import contextlib
import io
import os
import unittest
from unittest import mock

import manager_transfer


class TransferFixtureChecks(unittest.TestCase):
    def test_foreign_uid_grant_fails_before_object_work(self):
        response = mock.Mock(status=200)
        response.read.return_value = b"{}"
        connection = mock.MagicMock()
        connection.__enter__.return_value.getresponse.return_value = response
        with mock.patch.object(manager_transfer.sys, "platform", "linux"), mock.patch.object(os, "geteuid", return_value=3), mock.patch.dict(os.environ, {"HOMENODE_FILES_MANAGER_INTEGRATION": "1"}), mock.patch.object(manager_transfer.sys, "argv", ["fixture", "/tmp/transfer.sock", "a" * 32, "deny"]), mock.patch.object(manager_transfer, "UnixHTTP", return_value=connection), mock.patch.object(manager_transfer.boot_image, "object_roundtrip") as transfer, mock.patch.object(manager_transfer.time, "sleep") as sleep:
            with self.assertRaisesRegex(RuntimeError, "not denied"):
                manager_transfer.main()
            transfer.assert_not_called()
            sleep.assert_not_called()

    def test_foreign_uid_denial_does_not_attempt_guest_object_work(self):
        response = mock.Mock(status=403)
        response.read.return_value = b"peer denied"
        connection = mock.MagicMock()
        connection.__enter__.return_value.getresponse.return_value = response
        with mock.patch.object(manager_transfer.sys, "platform", "linux"), mock.patch.object(os, "geteuid", return_value=3), mock.patch.dict(os.environ, {"HOMENODE_FILES_MANAGER_INTEGRATION": "1"}), mock.patch.object(manager_transfer.sys, "argv", ["fixture", "/tmp/transfer.sock", "a" * 32, "deny"]), mock.patch.object(manager_transfer, "UnixHTTP", return_value=connection), mock.patch.object(manager_transfer.boot_image, "object_roundtrip") as transfer, contextlib.redirect_stdout(io.StringIO()) as output:
            manager_transfer.main()
            transfer.assert_not_called()
        self.assertEqual(output.getvalue(), "development transfer deny passed\n")


if __name__ == "__main__":
    unittest.main()
