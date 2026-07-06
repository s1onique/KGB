#!/usr/bin/env python3
"""Unit tests for tovarisch_status_rss_canary CLI and argument handling."""

import argparse, sys, unittest
from unittest.mock import patch

from tovarisch_status_rss_canary_test_support import SCRIPTS_DIR
sys.path.insert(0, str(SCRIPTS_DIR))

import tovarisch_status_rss_canary_lib as canary


class TestCLIDefaults(unittest.TestCase):
    """Test CLI argument defaults."""

    def test_default_values(self):
        """Test that default values are correct."""
        parser = canary.build_parser()
        defaults = {
            "warmup_requests": 25,
            "sample_requests": 200,
            "interval_seconds": 0.0,
            "timeout_seconds": 2.0,
            "max_rss_kib_growth": 4096,
            "max_private_kib_growth": 1024,
            "output": "text",
        }

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
        parser = canary.build_parser()
        args = parser.parse_args([
            "--url", "http://example.com/status",
            "--pid", "12345"
        ])
        self.assertEqual(args.url, "http://example.com/status")
        self.assertEqual(args.pid, 12345)

    def test_parser_requires_url_and_pid(self):
        """Test that parser requires --url and --pid."""
        parser = canary.build_parser()
        with self.assertRaises(SystemExit):
            parser.parse_args([])


class TestArgValidation(unittest.TestCase):
    """Test argument validation."""

    def test_validate_args_returns_none_for_valid_args(self):
        """Test validate_args returns None for valid args."""
        parser = canary.build_parser()
        args = parser.parse_args([
            "--url", "http://example.com/status",
            "--pid", "12345"
        ])
        result = canary.validate_args(args)
        self.assertIsNone(result)

    def test_validate_args_rejects_negative_warmup(self):
        """Test validate_args rejects negative warmup_requests."""
        parser = canary.build_parser()
        args = parser.parse_args([
            "--url", "http://example.com/status",
            "--pid", "12345",
            "--warmup-requests", "-1"
        ])
        result = canary.validate_args(args)
        self.assertIsNotNone(result)
        self.assertIn("warmup", result.lower())

    def test_validate_args_rejects_negative_sample(self):
        """Test validate_args rejects negative sample_requests."""
        parser = canary.build_parser()
        args = parser.parse_args([
            "--url", "http://example.com/status",
            "--pid", "12345",
            "--sample-requests", "-1"
        ])
        result = canary.validate_args(args)
        self.assertIsNotNone(result)
        self.assertIn("sample", result.lower())

    def test_validate_args_rejects_zero_timeout(self):
        """Test validate_args rejects zero timeout_seconds."""
        parser = canary.build_parser()
        args = parser.parse_args([
            "--url", "http://example.com/status",
            "--pid", "12345",
            "--timeout-seconds", "0"
        ])
        result = canary.validate_args(args)
        self.assertIsNotNone(result)
        self.assertIn("timeout", result.lower())


class TestMainExitCodes(unittest.TestCase):
    """Test main() exit code mapping."""

    def test_exit_code_pass(self):
        """Test exit code 0 for pass status."""
        from tovarisch_status_rss_canary import EXIT_CODES
        self.assertEqual(EXIT_CODES["pass"], 0)

    def test_exit_code_fail(self):
        """Test exit code 1 for fail status."""
        from tovarisch_status_rss_canary import EXIT_CODES
        self.assertEqual(EXIT_CODES["fail"], 1)

    def test_exit_code_skip(self):
        """Test exit code 2 for skip status."""
        from tovarisch_status_rss_canary import EXIT_CODES
        self.assertEqual(EXIT_CODES["skip"], 2)

    def test_exit_code_error(self):
        """Test exit code 3 for error status."""
        from tovarisch_status_rss_canary import EXIT_CODES
        self.assertEqual(EXIT_CODES["error"], 3)


if __name__ == "__main__":
    unittest.main(verbosity=2)
