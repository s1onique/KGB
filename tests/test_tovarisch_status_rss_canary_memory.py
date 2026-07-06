#!/usr/bin/env python3
"""Unit tests for tovarisch_status_rss_canary memory parsing."""

import os, sys, tempfile, unittest
from pathlib import Path
from unittest.mock import patch

# Import support helper to add scripts to path
from tovarisch_status_rss_canary_test_support import SCRIPTS_DIR
sys.path.insert(0, str(SCRIPTS_DIR))

import tovarisch_status_rss_canary_lib as canary


class TestParseMemorySize(unittest.TestCase):
    """Test parse_memory_size_kib function."""

    def test_kb_suffix(self):
        self.assertEqual(canary.parse_memory_size_kib("100 kB"), 100)
        self.assertEqual(canary.parse_memory_size_kib("100 KB"), 100)
        self.assertEqual(canary.parse_memory_size_kib("  50 kB  "), 50)

    def test_mb_suffix(self):
        self.assertEqual(canary.parse_memory_size_kib("10 MB"), 10 * 1024)
        self.assertEqual(canary.parse_memory_size_kib("1.5 mB"), int(1.5 * 1024))

    def test_gb_suffix(self):
        self.assertEqual(canary.parse_memory_size_kib("1 GB"), 1024 * 1024)
        self.assertEqual(canary.parse_memory_size_kib("2 gB"), 2 * 1024 * 1024)

    def test_plain_integer(self):
        self.assertEqual(canary.parse_memory_size_kib("12345"), 12345)


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
            result = canary.parse_smaps_rollup(path)
            self.assertIsNotNone(result)
            self.assertEqual(result["Rss"], 6820)
            self.assertEqual(result["Pss"], 4500)
            self.assertEqual(result["Private_Clean"], 1500)
            self.assertEqual(result["Private_Dirty"], 1316)
            self.assertEqual(result["Shared_Clean"], 3500)
            self.assertEqual(result["Shared_Dirty"], 504)
            self.assertEqual(result["private_kib"], 1500 + 1316)
        finally:
            os.unlink(path)

    def test_missing_file_returns_none(self):
        """Test that missing file returns None."""
        result = canary.parse_smaps_rollup("/nonexistent/path/smaps_rollup")
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
            result = canary.parse_smaps_rollup(path)
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
            result = canary.parse_proc_status(path)
            self.assertIsNotNone(result)
            self.assertEqual(result["VmRSS"], 6820)
            self.assertEqual(result["RssAnon"], 1316)
            self.assertEqual(result["RssFile"], 3500)
            self.assertEqual(result["RssShmem"], 504)
            self.assertEqual(result["VmData"], 8192)
            self.assertEqual(result["private_kib"], 1316)
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
            result = canary.parse_proc_status(path)
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
            result = canary.parse_proc_status(path)
            self.assertIsNotNone(result)
            self.assertEqual(result["private_kib"], 6820)
        finally:
            os.unlink(path)


class TestMemorySourceSelection(unittest.TestCase):
    """Test memory source selection logic."""

    def test_prefers_smaps_rollup_when_available(self):
        """Test that smaps_rollup is preferred when available."""
        fake_pid = 99999

        with patch.object(canary, "platform") as mock_platform, \
             patch("os.path.isdir") as mock_isdir, \
             patch("os.path.isfile") as mock_isfile:

            mock_platform.system.return_value = "Linux"
            mock_isdir.return_value = True
            mock_isfile.side_effect = lambda p: "smaps_rollup" in p

            smaps_content = """Rss:                 6820 kB
Pss:                 4500 kB
Private_Clean:       1500 kB
Private_Dirty:       1316 kB
Shared_Clean:        3500 kB
Shared_Dirty:         504 kB
"""
            with patch("builtins.open", unittest.mock.mock_open(read_data=smaps_content)):
                source, metrics, err = canary.get_memory_source(
                    fake_pid, allow_missing_smaps_rollup=False
                )

            self.assertEqual(source, "smaps_rollup")
            self.assertIsNotNone(metrics)
            self.assertEqual(metrics["Rss"], 6820)

    def test_falls_back_to_status_when_smaps_missing(self):
        """Test fallback to /proc/PID/status when smaps_rollup is missing."""
        fake_pid = 99999

        def isfile_side_effect(path):
            return "status" in path

        with patch.object(canary, "platform") as mock_platform, \
             patch("os.path.isdir") as mock_isdir, \
             patch("os.path.isfile") as mock_isfile:

            mock_platform.system.return_value = "Linux"
            mock_isdir.return_value = True
            mock_isfile.side_effect = isfile_side_effect

            status_content = """VmRSS:               6820 kB
RssAnon:             1316 kB
"""
            with patch("builtins.open", unittest.mock.mock_open(read_data=status_content)):
                source, metrics, err = canary.get_memory_source(
                    fake_pid, allow_missing_smaps_rollup=True
                )

            self.assertEqual(source, "status")
            self.assertIsNotNone(metrics)
            self.assertEqual(metrics["VmRSS"], 6820)

    def test_returns_none_when_proc_files_missing(self):
        """Test that None is returned when proc files are missing."""
        fake_pid = 99999

        with patch.object(canary, "platform") as mock_platform, \
             patch("os.path.isdir") as mock_isdir, \
             patch("os.path.isfile") as mock_isfile:

            mock_platform.system.return_value = "Linux"
            mock_isdir.return_value = True
            mock_isfile.return_value = False

            source, metrics, err = canary.get_memory_source(
                fake_pid, allow_missing_smaps_rollup=True
            )

            self.assertIsNone(source)
            self.assertIsNone(metrics)


if __name__ == "__main__":
    unittest.main(verbosity=2)
