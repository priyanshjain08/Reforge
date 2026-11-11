import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))
from detectors import statistical as st  # noqa: E402
from diagnostics.rules import diagnose  # noqa: E402


class Detectors(unittest.TestCase):
    def test_spike_detected(self):
        self.assertTrue(st.zscore_spike([10, 11, 10, 12, 11, 10, 11, 10, 12, 11, 95], name="cpu").triggered)

    def test_flat_not_detected(self):
        self.assertFalse(st.zscore_spike([10, 11] * 10, name="cpu").triggered)

    def test_insufficient_history(self):
        self.assertFalse(st.zscore_spike([1, 2, 3]).triggered)

    def test_growth(self):
        self.assertTrue(st.gradual_growth(list(range(0, 60, 2))).triggered)

    def test_threshold_severity(self):
        self.assertEqual(st.threshold([99], 80).severity, "critical")

    def test_consecutive(self):
        self.assertTrue(st.consecutive_failures([True, False, False, False]).triggered)
        self.assertFalse(st.consecutive_failures([False, False, True]).triggered)


class Diagnosis(unittest.TestCase):
    def test_exited_container_confirmed(self):
        d = diagnose({"service": "payment", "consecutive_failures": 3, "container_state": "exited", "exit_code": 137})
        self.assertEqual(d.status, "confirmed")
        self.assertEqual(d.recommended_action, "restart_container")
        self.assertGreaterEqual(d.confidence, 0.8)

    def test_crash_loop_escalates(self):
        self.assertEqual(diagnose({"service": "payment", "container_restarts": 4}).recommended_action, "escalate")

    def test_running_but_failing_is_only_suspected(self):
        d = diagnose({"service": "payment", "consecutive_failures": 3, "container_state": "running"})
        self.assertEqual(d.status, "suspected")
        self.assertLess(d.confidence, 0.7)

    def test_no_evidence(self):
        self.assertLess(diagnose({"service": "x"}).confidence, 0.2)


if __name__ == "__main__":
    unittest.main()
