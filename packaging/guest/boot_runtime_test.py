"""Runtime lifecycle fixture must validate the exact controller result."""
import contextlib
import io
import json
import os
import unittest
from unittest import mock

import manager_runtime


class RuntimeFixtureChecks(unittest.TestCase):
    def run_client(self, action, result, status=200):
        response = mock.Mock(status=status)
        response.read.return_value = json.dumps(result).encode()
        connection = mock.MagicMock()
        client = connection.__enter__.return_value
        client.getresponse.return_value = response
        with mock.patch.object(manager_runtime.sys, "platform", "linux"), mock.patch.object(os, "geteuid", return_value=1), mock.patch.dict(os.environ, {"HOMENODE_FILES_MANAGER_INTEGRATION": "1"}), mock.patch.object(manager_runtime.sys, "argv", ["fixture", "/tmp/supervisor.sock", "a" * 32, action, "65536"]), mock.patch.object(manager_runtime.manager_transfer, "UnixHTTP", return_value=connection), contextlib.redirect_stdout(io.StringIO()) as output:
            manager_runtime.main()
        self.assertEqual(client.request.call_count, 1)
        method, path, body, _ = client.request.call_args.args
        self.assertEqual((method, path), ("POST", "/v1/runtime"))
        request = json.loads(body)
        self.assertEqual(request["action"], action)
        self.assertEqual(request["instanceId"], "a" * 32)
        self.assertEqual(request["revision"], 1 if action == "start" else 2)
        self.assertEqual(len(request["operationId"]), 32)
        self.assertEqual(output.getvalue(), "development runtime " + action + " passed\n")

    def test_exact_start_and_shutdown(self):
        for action, revision, state in [("start", 1, "running"), ("shutdown", 2, "stopped")]:
            with self.subTest(action=action):
                self.run_client(action, {"id": "a" * 32, "workload": "files", "state": state, "revision": revision, "guestUid": 65536})

    def test_wrong_identity_revision_state_and_uid_are_refused(self):
        good = {"id": "a" * 32, "workload": "files", "state": "running", "revision": 1, "guestUid": 65536}
        for field, value in [("id", "b" * 32), ("revision", True), ("revision", 2), ("state", "stopped"), ("guestUid", 65537), ("guestUid", True), ("workload", "ai")]:
            with self.subTest(field=field, value=value), self.assertRaisesRegex(RuntimeError, "mismatch"):
                self.run_client("start", {**good, field: value})

    def test_http_refusal_is_not_success(self):
        with self.assertRaisesRegex(RuntimeError, "refused"):
            self.run_client("start", {}, 409)


if __name__ == "__main__":
    unittest.main()
