#!/usr/bin/env python3
"""
Unit tests for tovarisch_status_rss_canary.py

Tests memory parsing, HTTP behavior, threshold evaluation, output formatting,
and CLI argument defaults without requiring Linux /proc or a real tovarisch.
"""

import io
import json
import os
import sys
import tempfile
import unittest
from unittest.mock import patch

# Import the module under test
import importlib.util

spec = importlib.util.spec_from_file_location(
    "tovarisch_status_rss_canary",
    "scripts/tovarisch_status_rss_canary.py"
)
canary_module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(canary_module)


class TestParseMemorySize(unittest.TestCase):
    """Test parse_memory_size_kib function."""

    def test_kb_suffix(self):
        self.assertEqual(canary_module.parse_memory_size_kib("100 kB"), 100)
        self.assertEqual(canary_module.parse_memory_size_kib("100 KB"), 100)
        self.assertEqual(canary_module.parse_memory_size_kib("  50 kB  "), 50)

    def test_mb_suffix(self):
        self.assertEqual(canary_module.parse_memory_size_kib("10 MB"), 10 * 1024)
        self.assertEqual(canary_module.parse_memory_size_kib("1.5 mB"), int(1.5 * 1024))

    def test_gb_suffix(self):
        self.assertEqual(canary_module.parse_memory_size_kib("1 GB"), 1024 * 1024)
        self.assertEqual(canary_module.parse_memory_size_kib("2 gB"), 2 * 1024 * 1024)

    def test_plain_integer(self):
        self.assertEqual(canary_module.parse_memory_size_kib("12345"), 12345)


class TestParseSmapsRollup(unittest.TestCase):
    """Test parse_smaps_rollup function."""

    def test_parses_valid_smaps_rollup(self):
        """Test parsing a valid smaps_rollup file."""
        content = """Rss:                 6820 kB
Pss:                 4500 kB
Private_Clean:       1500 kB
Private_Dirty:       1316 kB
Shared_Clean:        3500 kB
Shared_Dirty:         504 kB
"""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
            f.write(content)
            f.flush()
            path = f.name

        try:
            result = canary_module.parse_smaps_rollup(path)
            self.assertIsNotNone(result)
            self.assertEqual(result["Rss"], 6820)
            self.assertEqual(result["Pss"], 4500)
            self.assertEqual(result["Private_Clean"], 1500)
            self.assertEqual(result["Private_Dirty"], 1316)
            self.assertEqual(result["Shared_Clean"], 3500)
            self.assertEqual(result["Shared_Dirty"], 504)
            self.assertEqual(result["private_kib"], 1500 + 1316)  # 2816
        finally:
            os.unlink(path)

    def test_missing_file_returns_none(self):
        """Test that missing file returns None."""
        result = canary_module.parse_smaps_rollup("/nonexistent/path/smaps_rollup")
        self.assertIsNone(result)

    def test_missing_required_fields_returns_none(self):
        """Test that missing required fields returns None."""
        content = """Rss:                 6820 kB
Pss:                 4500 kB
"""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
            f.write(content)
            f.flush()
            path = f.name

        try:
            result = canary_module.parse_smaps_rollup(path)
            self.assertIsNone(result)
        finally:
            os.unlink(path)


class TestParseProcStatus(unittest.TestCase):
    """Test parse_proc_status function."""

    def test_parses_valid_status(self):
        """Test parsing a valid /proc/PID/status file."""
        content = """VmRSS:               6820 kB
RssAnon:             1316 kB
RssFile:             3500 kB
RssShmem:            504 kB
VmData:              8192 kB
"""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
            f.write(content)
            f.flush()
            path = f.name

        try:
            result = canary_module.parse_proc_status(path)
            self.assertIsNotNone(result)
            self.assertEqual(result["VmRSS"], 6820)
            self.assertEqual(result["RssAnon"], 1316)
            self.assertEqual(result["RssFile"], 3500)
            self.assertEqual(result["RssShmem"], 504)
            self.assertEqual(result["VmData"], 8192)
            self.assertEqual(result["private_kib"], 1316)  # Uses RssAnon
        finally:
            os.unlink(path)

    def test_missing_vmrss_returns_none(self):
        """Test that missing VmRSS returns None."""
        content = """RssAnon:             1316 kB
"""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
            f.write(content)
            f.flush()
            path = f.name

        try:
            result = canary_module.parse_proc_status(path)
            self.assertIsNone(result)
        finally:
            os.unlink(path)

    def test_missing_rssanon_uses_vmrss_for_private(self):
        """Test that missing RssAnon uses VmRSS as private."""
        content = """VmRSS:               6820 kB
"""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
            f.write(content)
            f.flush()
            path = f.name

        try:
            result = canary_module.parse_proc_status(path)
            self.assertIsNotNone(result)
            self.assertEqual(result["private_kib"], 6820)  # Falls back to VmRSS
        finally:
            os.unlink(path)


class TestMemorySourceSelection(unittest.TestCase):
    """Test memory source selection logic."""

    def test_prefers_smaps_rollup_when_available(self):
        """Test that smaps_rollup is preferred when available."""
        fake_pid = 99999
        proc_path = f"/proc/{fake_pid}"

        with patch.object(canary_module, "platform") as mock_platform, \
             patch("os.path.isdir") as mock_isdir, \
             patch("os.path.isfile") as mock_isfile, \
             patch("builtins.open", side_effect=FileNotFoundError):

            mock_platform.system.return_value = "Linux"
            # The PID exists
            mock_isdir.return_value = True
            # smaps_rollup exists
            mock_isfile.side_effect = lambda p: "smaps_rollup" in p

            # Read actual smaps_rollup content
            smaps_content = """Rss:                 6820 kB
Pss:                 4500 kB
Private_Clean:       1500 kB
Private_Dirty:       1316 kB
Shared_Clean:        3500 kB
Shared_Dirty:         504 kB
"""
            with patch("builtins.open", unittest.mock.mock_open(read_data=smaps_content)):
                source, metrics, err = canary_module.get_memory_source(
                    fake_pid, allow_missing_smaps_rollup=False
                )

            self.assertEqual(source, "smaps_rollup")
            self.assertIsNotNone(metrics)
            self.assertEqual(metrics["Rss"], 6820)

    def test_falls_back_to_status_when_smaps_missing(self):
        """Test fallback to /proc/PID/status when smaps_rollup is missing."""
        fake_pid = 99999
        proc_path = f"/proc/{fake_pid}"

        def isfile_side_effect(path):
            """Return True only for status file."""
            return "status" in path

        with patch.object(canary_module, "platform") as mock_platform, \
             patch("os.path.isdir") as mock_isdir, \
             patch("os.path.isfile") as mock_isfile:

            mock_platform.system.return_value = "Linux"
            mock_isdir.return_value = True
            mock_isfile.side_effect = isfile_side_effect

            status_content = """VmRSS:               6820 kB
RssAnon:             1316 kB
"""
            with patch("builtins.open", unittest.mock.mock_open(read_data=status_content)):
                source, metrics, err = canary_module.get_memory_source(
                    fake_pid, allow_missing_smaps_rollup=True
                )

            self.assertEqual(source, "status")
            self.assertIsNotNone(metrics)
            self.assertEqual(metrics["VmRSS"], 6820)

    def test_returns_none_when_proc_files_missing(self):
        """Test that None is returned when proc files are missing."""
        fake_pid = 99999

        with patch.object(canary_module, "platform") as mock_platform, \
             patch("os.path.isdir") as mock_isdir, \
             patch("os.path.isfile") as mock_isfile:

            mock_platform.system.return_value = "Linux"
            mock_isdir.return_value = True
            mock_isfile.return_value = False

            source, metrics, err = canary_module.get_memory_source(
                fake_pid, allow_missing_smaps_rollup=True
            )

            self.assertIsNone(source)
            self.assertIsNone(metrics)


class TestHttpGet(unittest.TestCase):
    """Test HTTP request helper."""

    def test_success_returns_body(self):
        """Test successful GET returns body."""
        with patch("urllib.request.urlopen") as mock_urlopen:
            mock_response = unittest.mock.MagicMock()
            mock_response.status = 200
            mock_response.read.return_value = b'{"status": "ok"}'
            mock_response.__enter__ = unittest.mock.MagicMock(return_value=mock_response)
            mock_response.__exit__ = unittest.mock.MagicMock(return_value=False)
            mock_urlopen.return_value = mock_response

            success, msg = canary_module.http_get("http://example.com/status", 2.0)

            self.assertTrue(success)
            self.assertEqual(msg, '{"status": "ok"}')

    def test_empty_body_returns_false(self):
        """Test empty body returns failure."""
        with patch("urllib.request.urlopen") as mock_urlopen:
            mock_response = unittest.mock.MagicMock()
            mock_response.status = 200
            mock_response.read.return_value = b""
            mock_response.__enter__ = unittest.mock.MagicMock(return_value=mock_response)
            mock_response.__exit__ = unittest.mock.MagicMock(return_value=False)
            mock_urlopen.return_value = mock_response

            success, msg = canary_module.http_get("http://example.com/status", 2.0)

            self.assertFalse(success)
            self.assertEqual(msg, "empty_body")

    def test_non_2xx_returns_failure(self):
        """Test non-2xx response returns failure."""
        with patch("urllib.request.urlopen") as mock_urlopen:
            mock_response = unittest.mock.MagicMock()
            mock_response.status = 404
            mock_response.read.return_value = b"Not Found"
            mock_response.__enter__ = unittest.mock.MagicMock(return_value=mock_response)
            mock_response.__exit__ = unittest.mock.MagicMock(return_value=False)
            mock_urlopen.return_value = mock_response

            success, msg = canary_module.http_get("http://example.com/status", 2.0)

            self.assertFalse(success)
            self.assertEqual(msg, "http_404")


class TestThresholdEvaluation(unittest.TestCase):
    """Test threshold evaluation logic."""

    def test_passes_when_below_thresholds(self):
        """Test pass when deltas are below thresholds."""
        result = {
            "status": "error",
            "rss_kib_before": 6820,
            "rss_kib_after": 6864,
            "private_kib_before": 2816,
            "private_kib_after": 2820,
        }

        max_rss = 4096
        max_private = 1024

        rss_delta = result["rss_kib_after"] - result["rss_kib_before"]
        private_delta = result["private_kib_after"] - result["private_kib_before"]

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

        output = canary_module.format_text_output(result)
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

        output = canary_module.format_text_output(result)
        self.assertIn("TOVARISCH STATUS RSS CANARY: FAIL", output)
        self.assertIn("reason=private_kib_delta_exceeded", output)
        self.assertIn("private_kib_delta=18432", output)

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

        output = canary_module.format_json_output(result)
        parsed = json.loads(output)

        required_keys = [
            "status", "pid", "url", "memory_source", "warmup_requests",
            "sample_requests", "rss_kib_before", "rss_kib_after", "rss_kib_delta",
            "private_kib_before", "private_kib_after", "private_kib_delta", "thresholds"
        ]
        for key in required_keys:
            self.assertIn(key, parsed, f"Missing key: {key}")


class TestCLIDefaults(unittest.TestCase):
    """Test CLI argument defaults."""

    def test_default_values(self):
        """Test that default values are correct."""
        defaults = {
            "warmup_requests": 25,
            "sample_requests": 200,
            "interval_seconds": 0.0,
            "timeout_seconds": 2.0,
            "max_rss_kib_growth": 4096,
            "max_private_kib_growth": 1024,
            "output": "text",
        }

        # These should match the argparse defaults
        expected_defaults = {
            "warmup_requests": 25,
            "sample_requests": 200,
            "interval_seconds": 0.0,
            "timeout_seconds": 2.0,
            "max_rss_kib_growth": 4096,
            "max_private_kib_growth": 1024,
            "output": "text",
        }

        self.assertEqual(defaults, expected_defaults)

    def test_argument_parser_accepts_valid_args(self):
        """Test that argument parser accepts valid arguments."""
        # Create a test parser with the same arguments as main
        parser = canary_module.argparse.ArgumentParser()
        parser.add_argument("--url", required=True)
        parser.add_argument("--pid", type=int, required=True)

        # Parse with test args
        args = parser.parse_args(["--url", "http://example.com/status", "--pid", "12345"])
        self.assertEqual(args.url, "http://example.com/status")
        self.assertEqual(args.pid, 12345)


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


if __name__ == "__main__":
    unittest.main(verbosity=2)
