#!/usr/bin/env python3
"""Unit tests for tovarisch_status_rss_canary output formatting."""

import json, sys, unittest

from tovarisch_status_rss_canary_test_support import SCRIPTS_DIR
sys.path.insert(0, str(SCRIPTS_DIR))

import tovarisch_status_rss_canary_lib as canary


class TestOutputFormatting(unittest.TestCase):
    """Test output formatting functions."""

    def test_text_pass_output(self):
        """Test text output for pass case."""
        result = {
            "status": "pass",
            "pid": 2174927,
            "url": "http://10.149.149.1:8317/status",
            "memory_source": "smaps_rollup",
            "warmup_requests": 25,
            "sample_requests": 200,
            "rss_kib_before": 6820,
            "rss_kib_after": 6864,
            "rss_kib_delta": 44,
            "private_kib_before": 2816,
            "private_kib_after": 2820,
            "private_kib_delta": 4,
            "thresholds": {
                "max_rss_kib_growth": 4096,
                "max_private_kib_growth": 1024,
            },
        }

        output = canary.format_text_output(result)
        self.assertIn("TOVARISCH STATUS RSS CANARY: PASS", output)
        self.assertIn("pid=2174927", output)
        self.assertIn("memory_source=smaps_rollup", output)
        self.assertIn("rss_kib_delta=44", output)
        self.assertIn("private_kib_delta=4", output)

    def test_text_fail_output(self):
        """Test text output for fail case."""
        result = {
            "status": "fail",
            "reason": "private_kib_delta_exceeded",
            "private_kib_delta": 18432,
            "rss_kib_delta": 19000,
            "sample_requests": 200,
            "thresholds": {
                "max_private_kib_growth": 1024,
            },
        }

        output = canary.format_text_output(result)
        self.assertIn("TOVARISCH STATUS RSS CANARY: FAIL", output)
        self.assertIn("reason=private_kib_delta_exceeded", output)
        self.assertIn("private_kib_delta=18432", output)

    def test_text_skip_output(self):
        """Test text output for skip case."""
        result = {
            "status": "skip",
            "reason": "not_linux",
        }

        output = canary.format_text_output(result)
        self.assertIn("TOVARISCH STATUS RSS CANARY: SKIP", output)
        self.assertIn("reason=not_linux", output)

    def test_text_error_output(self):
        """Test text output for error case."""
        result = {
            "status": "error",
            "reason": "unknown",
        }

        output = canary.format_text_output(result)
        self.assertIn("TOVARISCH STATUS RSS CANARY: ERROR", output)
        self.assertIn("reason=unknown", output)

    def test_json_output_contains_required_keys(self):
        """Test JSON output contains all required keys."""
        result = {
            "status": "pass",
            "pid": 2174927,
            "url": "http://10.149.149.1:8317/status",
            "memory_source": "smaps_rollup",
            "warmup_requests": 25,
            "sample_requests": 200,
            "rss_kib_before": 6820,
            "rss_kib_after": 6864,
            "rss_kib_delta": 44,
            "private_kib_before": 2816,
            "private_kib_after": 2820,
            "private_kib_delta": 4,
            "thresholds": {
                "max_rss_kib_growth": 4096,
                "max_private_kib_growth": 1024,
            },
        }

        output = canary.format_json_output(result)
        parsed = json.loads(output)

        required_keys = [
            "status", "pid", "url", "memory_source", "warmup_requests",
            "sample_requests", "rss_kib_before", "rss_kib_after", "rss_kib_delta",
            "private_kib_before", "private_kib_after", "private_kib_delta", "thresholds"
        ]
        for key in required_keys:
            self.assertIn(key, parsed, f"Missing key: {key}")

    def test_json_output_ends_with_newline(self):
        """Test JSON output ends with newline."""
        result = {"status": "pass", "pid": 123}
        output = canary.format_json_output(result)
        self.assertTrue(output.endswith("\n"))


if __name__ == "__main__":
    unittest.main(verbosity=2)
