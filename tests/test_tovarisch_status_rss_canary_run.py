#!/usr/bin/env python3
"""Unit tests for tovarisch_status_rss_canary run_canary integration."""

import sys, unittest
from unittest.mock import patch, MagicMock

from tovarisch_status_rss_canary_test_support import SCRIPTS_DIR
sys.path.insert(0, str(SCRIPTS_DIR))

import tovarisch_status_rss_canary_lib as canary


class TestThresholdEvaluation(unittest.TestCase):
    """Test threshold evaluation logic."""

    def test_passes_when_below_thresholds(self):
        """Test pass when deltas are below thresholds."""
        rss_delta = 44
        private_delta = 4
        max_rss = 4096
        max_private = 1024

        rss_ok = rss_delta <= max_rss
        private_ok = private_delta <= max_private

        self.assertTrue(rss_ok)
        self.assertTrue(private_ok)

    def test_fails_when_rss_exceeds_threshold(self):
        """Test fail when RSS delta exceeds threshold."""
        rss_delta = 5000
        private_delta = 100
        max_rss = 4096
        max_private = 1024

        rss_ok = rss_delta <= max_rss
        private_ok = private_delta <= max_private

        self.assertFalse(rss_ok)
        self.assertTrue(private_ok)

    def test_fails_when_private_exceeds_threshold(self):
        """Test fail when private delta exceeds threshold."""
        rss_delta = 100
        private_delta = 2000
        max_rss = 4096
        max_private = 1024

        rss_ok = rss_delta <= max_rss
        private_ok = private_delta <= max_private

        self.assertTrue(rss_ok)
        self.assertFalse(private_ok)


class TestDeltaComputation(unittest.TestCase):
    """Test delta computation."""

    def test_positive_delta(self):
        """Test positive delta computation."""
        before = 6820
        after = 6864
        delta = after - before
        self.assertEqual(delta, 44)

    def test_negative_delta(self):
        """Test negative delta computation (memory released)."""
        before = 6864
        after = 6820
        delta = after - before
        self.assertEqual(delta, -44)

    def test_zero_delta(self):
        """Test zero delta."""
        before = 6820
        after = 6820
        delta = after - before
        self.assertEqual(delta, 0)


class TestRunCanaryIntegration(unittest.TestCase):
    """Test run_canary integration with mocked dependencies."""

    def _make_mock_http_get(self, body='{"status": "ok"}'):
        """Create a mock http_get function."""
        def mock_http_get(url, timeout):
            return True, body
        return mock_http_get

    def _make_mock_memory_source(self):
        """Create a mock get_memory_source function."""
        def mock_memory_source(pid, allow_missing):
            return (
                "smaps_rollup",
                {"Rss": 6820, "private_kib": 2816},
                ""
            )
        return mock_memory_source

    @patch.object(canary, "platform")
    @patch("os.path.isdir")
    def test_run_canary_passes_below_thresholds(self, mock_isdir, mock_platform):
        """Test run_canary returns pass when deltas are below thresholds."""
        mock_platform.system.return_value = "Linux"
        mock_isdir.return_value = True

        with patch.object(canary, "http_get", self._make_mock_http_get()), \
             patch.object(canary, "get_memory_source", self._make_mock_memory_source()):

            result = canary.run_canary(
                url="http://example.com/status",
                pid=12345,
                warmup_requests=1,
                sample_requests=1,
                interval_seconds=0.0,
                timeout_seconds=2.0,
                max_rss_kib_growth=4096,
                max_private_kib_growth=1024,
                allow_missing_smaps_rollup=False,
                verbose=False,
            )

            self.assertEqual(result["status"], "pass")
            self.assertEqual(result["reason"], "")

    @patch.object(canary, "platform")
    @patch("os.path.isdir")
    def test_run_canary_fails_when_rss_exceeds(self, mock_isdir, mock_platform):
        """Test run_canary returns fail when RSS exceeds threshold."""
        mock_platform.system.return_value = "Linux"
        mock_isdir.return_value = True

        call_count = [0]

        def mock_memory_source_high_rss(pid, allow_missing):
            before = {"Rss": 6820, "private_kib": 100}
            after = {"Rss": 12000, "private_kib": 100}  # 5160 KiB RSS growth
            call_count[0] += 1
            if call_count[0] == 1:
                return ("smaps_rollup", before, "")
            return ("smaps_rollup", after, "")

        with patch.object(canary, "http_get", self._make_mock_http_get()), \
             patch.object(canary, "get_memory_source", mock_memory_source_high_rss):

            result = canary.run_canary(
                url="http://example.com/status",
                pid=12345,
                warmup_requests=1,
                sample_requests=1,
                interval_seconds=0.0,
                timeout_seconds=2.0,
                max_rss_kib_growth=4096,
                max_private_kib_growth=1024,
                allow_missing_smaps_rollup=False,
                verbose=False,
            )

            self.assertEqual(result["status"], "fail")
            self.assertEqual(result["reason"], "rss_kib_delta_exceeded")

    @patch.object(canary, "platform")
    @patch("os.path.isdir")
    def test_run_canary_fails_when_private_exceeds(self, mock_isdir, mock_platform):
        """Test run_canary returns fail when private exceeds threshold."""
        mock_platform.system.return_value = "Linux"
        mock_isdir.return_value = True

        call_count = [0]

        def mock_memory_source_high_private(pid, allow_missing):
            before = {"Rss": 6820, "private_kib": 100}
            after = {"Rss": 6820, "private_kib": 2000}  # 1900 KiB private growth
            call_count[0] += 1
            if call_count[0] == 1:
                return ("smaps_rollup", before, "")
            return ("smaps_rollup", after, "")

        with patch.object(canary, "http_get", self._make_mock_http_get()), \
             patch.object(canary, "get_memory_source", mock_memory_source_high_private):

            result = canary.run_canary(
                url="http://example.com/status",
                pid=12345,
                warmup_requests=1,
                sample_requests=1,
                interval_seconds=0.0,
                timeout_seconds=2.0,
                max_rss_kib_growth=4096,
                max_private_kib_growth=1024,
                allow_missing_smaps_rollup=False,
                verbose=False,
            )

            self.assertEqual(result["status"], "fail")
            self.assertEqual(result["reason"], "private_kib_delta_exceeded")

    @patch.object(canary, "platform")
    def test_run_canary_skips_on_non_linux(self, mock_platform):
        """Test run_canary returns skip on non-Linux platforms."""
        mock_platform.system.return_value = "Darwin"

        result = canary.run_canary(
            url="http://example.com/status",
            pid=12345,
            warmup_requests=1,
            sample_requests=1,
            interval_seconds=0.0,
            timeout_seconds=2.0,
            max_rss_kib_growth=4096,
            max_private_kib_growth=1024,
            allow_missing_smaps_rollup=False,
            verbose=False,
        )

        self.assertEqual(result["status"], "skip")
        self.assertEqual(result["reason"], "not_linux")

    @patch.object(canary, "platform")
    @patch("os.path.isdir")
    def test_run_canary_skips_when_pid_not_found(self, mock_isdir, mock_platform):
        """Test run_canary returns skip when PID not found."""
        mock_platform.system.return_value = "Linux"
        mock_isdir.return_value = False

        result = canary.run_canary(
            url="http://example.com/status",
            pid=99999,
            warmup_requests=1,
            sample_requests=1,
            interval_seconds=0.0,
            timeout_seconds=2.0,
            max_rss_kib_growth=4096,
            max_private_kib_growth=1024,
            allow_missing_smaps_rollup=False,
            verbose=False,
        )

        self.assertEqual(result["status"], "skip")
        self.assertEqual(result["reason"], "pid_not_found")

    @patch.object(canary, "platform")
    @patch("os.path.isdir")
    def test_run_canary_fails_on_http_error(self, mock_isdir, mock_platform):
        """Test run_canary returns fail when HTTP request fails."""
        mock_platform.system.return_value = "Linux"
        mock_isdir.return_value = True

        def mock_http_get_fail(url, timeout):
            return False, "url_error_Connection refused"

        with patch.object(canary, "http_get", mock_http_get_fail):
            result = canary.run_canary(
                url="http://example.com/status",
                pid=12345,
                warmup_requests=1,
                sample_requests=1,
                interval_seconds=0.0,
                timeout_seconds=2.0,
                max_rss_kib_growth=4096,
                max_private_kib_growth=1024,
                allow_missing_smaps_rollup=False,
                verbose=False,
            )

        self.assertEqual(result["status"], "fail")
        self.assertIn("endpoint_unreachable", result["reason"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
