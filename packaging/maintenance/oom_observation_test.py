import unittest

from oom_observation import wait_for_oom


class OOMObservationTest(unittest.TestCase):
    def observe(self, expected):
        responses = iter(expected)

        def query(property_name):
            name, value = next(responses)
            self.assertEqual(name, property_name)
            return value

        return query

    def test_oom_between_result_and_terminal_state_queries(self):
        wait_for_oom(self.observe([
            ("Result", "success"), ("ActiveState", "failed"),
            ("Result", "oom-kill"),
        ]))

    def test_terminal_non_oom_exit_is_still_refused(self):
        times = iter([0, 0, 61])
        with self.assertRaisesRegex(RuntimeError, "without an OOM result"):
            wait_for_oom(self.observe([
                ("Result", "exit-code"), ("ActiveState", "failed"),
                ("Result", "exit-code"),
            ]), monotonic=lambda: next(times), sleep=lambda _: None)

    def test_oom_notification_after_terminal_state_is_still_observed(self):
        wait_for_oom(self.observe([
            ("Result", "signal"), ("ActiveState", "failed"),
            ("Result", "signal"), ("Result", "oom-kill"),
        ]), sleep=lambda _: None)

    def test_active_unit_does_not_imply_oom(self):
        times = iter([0, 0, 61])
        with self.assertRaisesRegex(RuntimeError, "did not report OOM"):
            wait_for_oom(self.observe([
                ("Result", "success"), ("ActiveState", "active"),
            ]), monotonic=lambda: next(times), sleep=lambda _: None)


if __name__ == "__main__":
    unittest.main()
