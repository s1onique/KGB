#!/usr/bin/env python3
"""Phase contract tests for tovarisch_status_rss_canary run_canary().

Verifies exact runtime sequence: preflight, warmup, baseline memory, sample,
final memory, delta calculation, threshold evaluation.

MEMOWN-0011 = thin CLI wrapper
MEMOWN-0026 = implementation library
"""

import sys, unittest
from unittest.mock import patch, Mock, call

from tovarisch_status_rss_canary_test_support import SCRIPTS_DIR
sys.path.insert(0, str(SCRIPTS_DIR))

import tovarisch_status_rss_canary_lib as canary


class TestRunCanaryPhaseContract(unittest.TestCase):
    """Test run_canary() phase execution contract."""

    def _run_with_mocks(
        self,
        *,
        warmup_requests=1,
        sample_requests=1,
        interval_seconds=0.0,
        http_side_effect=None,
        memory_side_effect=None,
        max_rss_kib_growth=4096,
        max_private_kib_growth=1024,
    ):
        """Helper: run run_canary with mocked dependencies."""
        http_default = lambda url, timeout: (True, '{"status":"ok"}')
        memory_default = lambda pid, allow: (
            "smaps_rollup",
            {"Rss": 1000, "private_kib": 500},
            ""
        )

        mock_http = Mock(side_effect=http_side_effect or http_default)
        mock_memory = Mock(side_effect=memory_side_effect or memory_default)

        with patch.object(canary, "platform") as mock_platform, \
             patch("os.path.isdir") as mock_isdir, \
             patch.object(canary, "http_get", mock_http), \
             patch.object(canary, "get_memory_source", mock_memory), \
             patch.object(canary.time, "sleep"):

            mock_platform.system.return_value = "Linux"
            mock_isdir.return_value = True

            result = canary.run_canary(
                url="http://example.com/status",
                pid=12345,
                warmup_requests=warmup_requests,
                sample_requests=sample_requests,
                interval_seconds=interval_seconds,
                timeout_seconds=2.0,
                max_rss_kib_growth=max_rss_kib_growth,
                max_private_kib_growth=max_private_kib_growth,
                allow_missing_smaps_rollup=False,
                verbose=False,
            )

        return result, mock_http, mock_memory

    def test_run_canary_success_uses_expected_http_request_count(self):
        """Verify exact HTTP request count: preflight + warmup + sample."""
        warmup_requests = 3
        sample_requests = 5
        expected_total = 1 + warmup_requests + sample_requests  # 9

        result, mock_http, _ = self._run_with_mocks(
            warmup_requests=warmup_requests,
            sample_requests=sample_requests,
        )

        self.assertEqual(mock_http.call_count, expected_total)
        self.assertEqual(result["status"], "pass")

        # Verify all calls used configured URL and timeout
        for call_args in mock_http.call_args_list:
            url, timeout = call_args[0]
            self.assertEqual(url, "http://example.com/status")
            self.assertEqual(timeout, 2.0)

    def test_run_canary_success_samples_memory_before_and_after_sample_phase(self):
        """Verify exactly two memory samples: baseline then final."""
        memory_samples = [
            ("smaps_rollup", {"Rss": 1000, "private_kib": 500}, ""),
            ("smaps_rollup", {"Rss": 1100, "private_kib": 550}, ""),
        ]

        result, _, mock_memory = self._run_with_mocks(
            memory_side_effect=lambda pid, allow: memory_samples.pop(0)
        )

        self.assertEqual(mock_memory.call_count, 2)
        self.assertEqual(result["status"], "pass")
        self.assertEqual(result["rss_kib_before"], 1000)
        self.assertEqual(result["rss_kib_after"], 1100)
        self.assertEqual(result["rss_kib_delta"], 100)
        self.assertEqual(result["private_kib_before"], 500)
        self.assertEqual(result["private_kib_after"], 550)
        self.assertEqual(result["private_kib_delta"], 50)

    def test_run_canary_success_preserves_phase_order(self):
        """Verify exact phase order: preflight, warmup, sleep, baseline, sample, sleep, final."""
        events = []

        def fake_http_get(url, timeout):
            events.append("http")
            return True, '{"status":"ok"}'

        def fake_get_memory_source(pid, allow_missing):
            events.append("memory")
            if events.count("memory") == 1:
                return "smaps_rollup", {"Rss": 1000, "private_kib": 500}, ""
            return "smaps_rollup", {"Rss": 1001, "private_kib": 501}, ""

        def fake_sleep(seconds):
            events.append(f"sleep:{seconds}")

        with patch.object(canary, "platform") as mock_platform, \
             patch("os.path.isdir") as mock_isdir, \
             patch.object(canary, "http_get", fake_http_get), \
             patch.object(canary, "get_memory_source", fake_get_memory_source), \
             patch.object(canary.time, "sleep", fake_sleep):

            mock_platform.system.return_value = "Linux"
            mock_isdir.return_value = True

            result = canary.run_canary(
                url="http://example.com/status",
                pid=12345,
                warmup_requests=2,
                sample_requests=3,
                interval_seconds=0.0,
                timeout_seconds=2.0,
                max_rss_kib_growth=4096,
                max_private_kib_growth=1024,
                allow_missing_smaps_rollup=False,
                verbose=False,
            )

        # Expected order: preflight http, warmup x2, sleep:0.1, memory(baseline),
        #                 sample x3, sleep:0.1, memory(final)
        expected = [
            "http",       # preflight
            "http",       # warmup #1
            "http",       # warmup #2
            "sleep:0.1",  # before baseline
            "memory",     # baseline
            "http",       # sample #1
            "http",       # sample #2
            "http",       # sample #3
            "sleep:0.1",  # before final
            "memory",     # final
        ]
        self.assertEqual(events, expected)
        self.assertEqual(result["status"], "pass")

    def test_run_canary_interval_sleep_count_matches_request_phases(self):
        """Verify interval sleeps between warmup requests and sample requests."""
        warmup_requests = 2
        sample_requests = 3
        interval_seconds = 0.25

        with patch.object(canary, "platform") as mock_platform, \
             patch("os.path.isdir") as mock_isdir, \
             patch.object(canary, "http_get", lambda url, t: (True, '{}')), \
             patch.object(canary, "get_memory_source", lambda pid, allow: (
                 "smaps_rollup", {"Rss": 1000, "private_kib": 500}, ""
             )), \
             patch.object(canary.time, "sleep") as mock_sleep:

            mock_platform.system.return_value = "Linux"
            mock_isdir.return_value = True

            canary.run_canary(
                url="http://example.com/status",
                pid=12345,
                warmup_requests=warmup_requests,
                sample_requests=sample_requests,
                interval_seconds=interval_seconds,
                timeout_seconds=2.0,
                max_rss_kib_growth=4096,
                max_private_kib_growth=1024,
                allow_missing_smaps_rollup=False,
                verbose=False,
            )

        # Expected sleeps:
        # - 0.25 after each warmup request (2 times)
        # - 0.1 before baseline memory
        # - 0.25 after each sample request (3 times)
        # - 0.1 before final memory
        expected_calls = [
            call(0.25),  # after warmup #1
            call(0.25),  # after warmup #2
            call(0.1),   # before baseline
            call(0.25),  # after sample #1
            call(0.25),  # after sample #2
            call(0.25),  # after sample #3
            call(0.1),   # before final
        ]
        mock_sleep.assert_has_calls(expected_calls)

    def test_run_canary_preflight_failure_short_circuits(self):
        """Preflight failure skips warmup and memory sampling."""
        def fail_preflight(url, timeout):
            return False, "url_error_Connection refused"

        result, mock_http, mock_memory = self._run_with_mocks(
            http_side_effect=fail_preflight
        )

        self.assertEqual(result["status"], "fail")
        self.assertTrue(result["reason"].startswith("endpoint_unreachable"))
        self.assertEqual(mock_http.call_count, 1)
        mock_memory.assert_not_called()

    def test_run_canary_warmup_failure_short_circuits_before_baseline(self):
        """Warmup failure skips baseline memory sampling."""
        call_count = [0]

        def warmup_then_fail(url, timeout):
            call_count[0] += 1
            # preflight + warmup #1 + warmup #2 = 3 successes, fail on warmup #3
            if call_count[0] <= 3:
                return True, '{"status":"ok"}'
            return False, "url_error_Connection refused"

        result, mock_http, mock_memory = self._run_with_mocks(
            warmup_requests=3,
            http_side_effect=warmup_then_fail,
        )

        self.assertEqual(result["status"], "fail")
        self.assertTrue(result["reason"].startswith("warmup_request_failed"))
        self.assertEqual(mock_http.call_count, 4)  # preflight + 3 warmup
        mock_memory.assert_not_called()
        self.assertIsNone(result["rss_kib_before"])
        self.assertIsNone(result["rss_kib_after"])

    def test_run_canary_baseline_memory_failure_skips_before_sample_phase(self):
        """Baseline memory failure skips sample phase."""
        memory_call_count = [0]

        def baseline_then_fail(pid, allow):
            memory_call_count[0] += 1
            if memory_call_count[0] == 1:
                return None, None, "proc_files_missing"
            return "smaps_rollup", {"Rss": 1000, "private_kib": 500}, ""

        result, mock_http, mock_memory = self._run_with_mocks(
            memory_side_effect=baseline_then_fail,
        )

        self.assertEqual(result["status"], "skip")
        self.assertEqual(result["reason"], "proc_files_missing")
        # HTTP: preflight + warmup = 2 total
        self.assertEqual(mock_http.call_count, 2)
        # Memory: only baseline attempted
        self.assertEqual(mock_memory.call_count, 1)
        # No sample requests sent
        self.assertIsNone(result["rss_kib_before"])
        self.assertIsNone(result["rss_kib_after"])

    def test_run_canary_sample_failure_short_circuits_before_final_memory(self):
        """Sample failure skips final memory sampling."""
        call_count = [0]

        def succeed_then_fail(url, timeout):
            call_count[0] += 1
            # preflight + 1 warmup + 1 sample = success
            if call_count[0] <= 3:
                return True, '{"status":"ok"}'
            return False, "url_error_Connection refused"

        result, mock_http, mock_memory = self._run_with_mocks(
            warmup_requests=1,
            sample_requests=3,
            http_side_effect=succeed_then_fail,
        )

        self.assertEqual(result["status"], "fail")
        self.assertTrue(result["reason"].startswith("sample_request_failed"))
        # HTTP: preflight + warmup + sample #1 + sample #2 (fails)
        self.assertEqual(mock_http.call_count, 4)
        # Memory: only baseline called (final skipped)
        self.assertEqual(mock_memory.call_count, 1)
        # Final memory fields not populated
        self.assertIsNotNone(result["rss_kib_before"])
        self.assertIsNone(result["rss_kib_after"])

    def test_run_canary_final_memory_failure_skips_after_sample_phase(self):
        """Final memory failure skips threshold evaluation."""
        memory_call_count = [0]

        def baseline_ok_final_fail(pid, allow):
            memory_call_count[0] += 1
            if memory_call_count[0] == 1:
                return "smaps_rollup", {"Rss": 1000, "private_kib": 500}, ""
            return None, None, "proc_files_missing"

        result, _, mock_memory = self._run_with_mocks(
            memory_side_effect=baseline_ok_final_fail,
        )

        self.assertEqual(result["status"], "skip")
        self.assertEqual(result["reason"], "proc_files_missing")
        # Memory: both baseline and final called
        self.assertEqual(mock_memory.call_count, 2)
        # Baseline populated
        self.assertEqual(result["rss_kib_before"], 1000)
        # Final not populated
        self.assertIsNone(result["rss_kib_after"])
        self.assertIsNone(result["rss_kib_delta"])
        self.assertIsNone(result["private_kib_delta"])

    def test_run_canary_rss_threshold_reason_wins_when_both_exceed(self):
        """RSS threshold takes precedence when both RSS and private exceed."""
        memory_samples = [
            ("smaps_rollup", {"Rss": 1000, "private_kib": 100}, ""),
            ("smaps_rollup", {"Rss": 6000, "private_kib": 2000}, ""),  # both exceed
        ]

        result, _, mock_memory = self._run_with_mocks(
            memory_side_effect=lambda pid, allow: memory_samples.pop(0),
            max_rss_kib_growth=4096,
            max_private_kib_growth=1024,
        )

        self.assertEqual(result["status"], "fail")
        # RSS exceeds first, so RSS reason wins
        self.assertEqual(result["reason"], "rss_kib_delta_exceeded")
        # Both values populated correctly
        self.assertEqual(result["rss_kib_delta"], 5000)
        self.assertEqual(result["private_kib_delta"], 1900)


if __name__ == "__main__":
    unittest.main(verbosity=2)
