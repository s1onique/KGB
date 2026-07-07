#!/usr/bin/env python3
"""
Self-test for tovarisch_test_base_fingerprint.py

Verifies the fingerprint artifact contains all required fields
and that the script executes successfully.

Usage:
    python3 tests/test_tovarisch_test_base_fingerprint.py -v
"""

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


class TestTovarischTestBaseFingerprint(unittest.TestCase):
    """Unit tests for tovarisch test-base fingerprint script."""
    
    @classmethod
    def setUpClass(cls):
        """Run fingerprint script once for all tests."""
        # Find the script
        script_path = Path(__file__).parent.parent / "scripts" / "tovarisch_test_base_fingerprint.py"
        cls.script_path = script_path
        
        if not script_path.exists():
            raise unittest.SkipTest(f"Script not found: {script_path}")
        
        # Create temp output directory
        cls.temp_dir = tempfile.mkdtemp(prefix="fingerprint_test_")
        cls.output_dir = os.path.join(cls.temp_dir, "external-analysis")
        os.makedirs(cls.output_dir, exist_ok=True)
        
        # Run the script with a known seed
        cls.test_seed = "0xa710199f"
        result = subprocess.run(
            [
                sys.executable,
                str(script_path),
                "--seed", cls.test_seed,
                "--output-dir", cls.output_dir,
            ],
            capture_output=True,
            text=True,
            timeout=300,
        )
        cls.result = result
        
        # Parse artifact if created
        cls.artifact_path = os.path.join(cls.output_dir, "tovarisch-test-base-fingerprint.json")
        cls.fingerprint = None
        if os.path.exists(cls.artifact_path):
            with open(cls.artifact_path) as f:
                cls.fingerprint = json.load(f)
    
    @classmethod
    def tearDownClass(cls):
        """Clean up temp directory."""
        import shutil
        if hasattr(cls, 'temp_dir') and os.path.exists(cls.temp_dir):
            shutil.rmtree(cls.temp_dir, ignore_errors=True)
    
    def test_script_executed_successfully(self):
        """Script should execute without errors (may have test failures but no script errors)."""
        # We don't require exit code 0 because tests may fail
        # Exit codes: 0=tests pass, 1=test failures, 2=parse error on successful run
        self.assertIn(
            self.result.returncode,
            [0, 1, 2],  # 0=pass, 1=test failures, 2=parse error (expected when cached)
            f"Script failed unexpectedly: {self.result.stderr}"
        )
    
    def test_artifact_created(self):
        """Fingerprint artifact file should be created."""
        self.assertTrue(
            os.path.exists(self.artifact_path),
            f"Artifact not created at: {self.artifact_path}\nStdout: {self.result.stdout}\nStderr: {self.result.stderr}"
        )
    
    def test_artifact_contains_sha(self):
        """Artifact must contain git_sha field."""
        self.assertIn("git_sha", self.fingerprint)
        self.assertIsInstance(self.fingerprint["git_sha"], str)
    
    def test_artifact_contains_zig_version(self):
        """Artifact must contain zig_version field."""
        self.assertIn("zig_version", self.fingerprint)
        self.assertIsInstance(self.fingerprint["zig_version"], str)
        self.assertTrue(len(self.fingerprint["zig_version"]) > 0)
    
    def test_artifact_contains_seed(self):
        """Artifact must contain seed field matching requested value."""
        self.assertIn("seed", self.fingerprint)
        self.assertEqual(self.fingerprint["seed"], self.test_seed)
    
    def test_artifact_contains_summary(self):
        """Artifact must contain summary with pass/skip/fail/total."""
        self.assertIn("summary", self.fingerprint)
        summary = self.fingerprint["summary"]
        
        self.assertIn("pass", summary)
        self.assertIn("skip", summary)
        self.assertIn("fail", summary)
        self.assertIn("total", summary)
        
        # All should be integers
        self.assertIsInstance(summary["pass"], int)
        self.assertIsInstance(summary["skip"], int)
        self.assertIsInstance(summary["fail"], int)
        self.assertIsInstance(summary["total"], int)
        
        # Total should equal pass + skip + fail
        expected_total = summary["pass"] + summary["skip"] + summary["fail"]
        self.assertEqual(summary["total"], expected_total)
    
    def test_artifact_contains_raw_summary_line(self):
        """Artifact must contain raw_summary_line field."""
        self.assertIn("raw_summary_line", self.fingerprint)
        self.assertIsInstance(self.fingerprint["raw_summary_line"], str)
    
    def test_successful_run_has_meaningful_summary(self):
        """If exit_code is 0, must either have meaningful summary OR parse_error.
        
        This is the critical contract: no silent success with empty summary.
        Either tests ran and were parsed (exit_code=0, total>0, no parse_error),
        OR we correctly caught the issue (exit_code=0, total=0, parse_error set).
        """
        if self.fingerprint["exit_code"] == 0:
            if self.fingerprint["summary"]["total"] == 0:
                # If tests passed but no summary parsed, parse_error MUST be set
                self.assertIn("parse_error", self.fingerprint,
                    "exit_code=0 with total=0 requires parse_error to be set (fail-closed)")
                self.assertIn("raw_output_tail", self.fingerprint,
                    "parse error case must include raw_output_tail for debugging")
            else:
                # If tests ran and were parsed, summary must be meaningful
                self.assertGreater(self.fingerprint["summary"]["total"], 0,
                    "Successful test run must have total > 0")
                self.assertTrue(self.fingerprint["raw_summary_line"],
                    "Successful test run must have non-empty raw_summary_line")
    
    def test_artifact_contains_raw_output_tail(self):
        """Artifact must contain raw_output_tail for debugging."""
        self.assertIn("raw_output_tail", self.fingerprint)
        self.assertIsInstance(self.fingerprint["raw_output_tail"], str)
    
    def test_artifact_contains_git_sha(self):
        """Git SHA should be present and non-empty when in git repo."""
        self.assertIn("git_sha", self.fingerprint)
        sha = self.fingerprint["git_sha"]
        # Should be 40-char hex or "unknown" or start with valid hex
        self.assertTrue(
            sha == "unknown" or len(sha) >= 7,
            f"Unexpected git_sha: {sha}"
        )
    
    def test_artifact_contains_timestamp(self):
        """Artifact must contain timestamp field."""
        self.assertIn("timestamp", self.fingerprint)
        self.assertIsInstance(self.fingerprint["timestamp"], str)
    
    def test_artifact_contains_command(self):
        """Artifact must contain the command that was run."""
        self.assertIn("command", self.fingerprint)
        self.assertIn("test-base", self.fingerprint["command"])
        self.assertIn(self.test_seed, self.fingerprint["command"])

    def test_artifact_ends_with_newline(self):
        """Fingerprint artifact must be newline-terminated for hygiene gates."""
        with open(self.artifact_path, "rb") as f:
            data = f.read()

        self.assertTrue(data, "Artifact should not be empty")
        self.assertTrue(
            data.endswith(b"\n"),
            "Fingerprint artifact must end with a newline",
        )


class TestFingerprintParsing(unittest.TestCase):
    """Test the parsing logic in isolation."""
    
    def test_parse_pass_skip_fail(self):
        """Test summary line parsing with various formats."""
        import sys
        from pathlib import Path
        # Add repo root to path for imports
        repo_root = Path(__file__).parent.parent
        if str(repo_root) not in sys.path:
            sys.path.insert(0, str(repo_root))
        
        from scripts.tovarisch_test_base_fingerprint import parse_test_summary
        
        # Standard format: "742 pass, 7 skip, 1 fail"
        result = parse_test_summary("742 pass, 7 skip, 1 fail\nAll tests passed")
        self.assertEqual(result["pass"], 742)
        self.assertEqual(result["skip"], 7)
        self.assertEqual(result["fail"], 1)
        self.assertEqual(result["total"], 750)
        
        # Zig summary format: "726/750 tests passed (24 skipped)"
        result = parse_test_summary("Build Summary: 4/4 steps succeeded; 726/750 tests passed (24 skipped)")
        self.assertEqual(result["pass"], 726)
        self.assertEqual(result["skip"], 24)
        self.assertEqual(result["fail"], 0)
        self.assertEqual(result["total"], 750)
        
        # Alternative format: "726 pass, 24 skip (750 total)"
        result = parse_test_summary("+- run test 726 pass, 24 skip (750 total)")
        self.assertEqual(result["pass"], 726)
        self.assertEqual(result["skip"], 24)
        self.assertEqual(result["fail"], 0)
        self.assertEqual(result["total"], 750)
        
        # All passed format
        result = parse_test_summary("All 100 tests passed")
        self.assertEqual(result["pass"], 100)
        self.assertEqual(result["skip"], 0)
        self.assertEqual(result["fail"], 0)
        self.assertEqual(result["total"], 100)
    
    def test_extract_raw_summary_line(self):
        """Test raw summary line extraction."""
        import sys
        from pathlib import Path
        # Add repo root to path for imports
        repo_root = Path(__file__).parent.parent
        if str(repo_root) not in sys.path:
            sys.path.insert(0, str(repo_root))
        
        from scripts.tovarisch_test_base_fingerprint import extract_raw_summary_line
        
        output = """
Building test-base...
Running tests...
742 pass, 7 skip, 1 fail
All 750 tests run.
"""
        line = extract_raw_summary_line(output)
        self.assertEqual(line, "742 pass, 7 skip, 1 fail")
        
        # No summary line
        output = "Just some other output"
        line = extract_raw_summary_line(output)
        self.assertEqual(line, "")


if __name__ == "__main__":
    unittest.main(verbosity=2)
