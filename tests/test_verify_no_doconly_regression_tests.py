#!/usr/bin/env python3
# test_verify_no_doconly_regression_tests.py — Contract tests for doc-only regression tests verifier
#
# ACT-HULK29R-ZIG016-MEMOWN03-NO-DOCONLY-REGRESSION-TESTS
#
# Tests verify the verifier correctly detects:
# - ACT-marked tests with only `try std.testing.expect(true);`
# - Regression tests with comments plus trivial assertions
# - Passes MEMOWN02-style tests using std.testing.allocator and FakeWgCommandRunner
# - Passes ordinary smoke tests that are not marked as ACT/regression/leak/memory
# - Reports repo-relative paths and line numbers
# - Handles multiple test blocks in one file
# - Handles nested braces correctly
#
# Run with: python3 tests/test_verify_no_doconly_regression_tests.py
# Run with verbose: python3 tests/test_verify_no_doconly_regression_tests.py -v

import os
import sys
import tempfile
import unittest
from pathlib import Path

# Add scripts directory to path for imports
sys.path.insert(0, str(Path(__file__).parent.parent / "scripts"))

from verify_no_doconly_regression_tests import (
    _find_test_blocks,
    _is_contract_like,
    _is_trivial_body,
    _strip_comments,
    check_file,
    REPO_ROOT,
)


class TestFindTestBlocks(unittest.TestCase):
    """Test _find_test_blocks function."""
    
    def test_finds_simple_test(self):
        """Should find a simple test block."""
        content = '''const std = @import("std");

test "basic test" {
    try std.testing.expect(true);
}
'''
        tests = _find_test_blocks(content)
        self.assertEqual(len(tests), 1)
        self.assertEqual(tests[0][0], "basic test")
        self.assertIn("expect(true)", tests[0][1])
    
    def test_finds_multiple_tests(self):
        """Should find multiple test blocks."""
        content = '''test "first" {
    try std.testing.expect(true);
}

test "second" {
    try std.testing.expect(false);
}
'''
        tests = _find_test_blocks(content)
        self.assertEqual(len(tests), 2)
        self.assertEqual(tests[0][0], "first")
        self.assertEqual(tests[1][0], "second")
    
    def test_captures_preceding_comments(self):
        """Should capture preceding comments."""
        content = '''// Comment line 1
// Comment line 2
test "with comments" {
    try std.testing.expect(true);
}
'''
        tests = _find_test_blocks(content)
        self.assertEqual(len(tests), 1)
        self.assertIn("Comment line 1", tests[0][2])
        self.assertIn("Comment line 2", tests[0][2])
    
    def test_reports_correct_line_number(self):
        """Should report correct line number."""
        content = '''//
// Line 2
// Line 3
// Line 4
test "test on line 5" {
// Line 6
}
'''
        tests = _find_test_blocks(content)
        self.assertEqual(tests[0][3], 5)  # 1-based line number
    
    def test_handles_nested_braces(self):
        """Should handle nested braces in structs/lambdas."""
        content = '''test "nested" {
    const callback = struct {
        fn call() void {}
    };
    try std.testing.expect(true);
}
'''
        tests = _find_test_blocks(content)
        self.assertEqual(len(tests), 1)
        self.assertIn("struct", tests[0][1])


class TestIsContractLike(unittest.TestCase):
    """Test _is_contract_like function."""
    
    def test_act_marker_in_name(self):
        """Should detect ACT- in test name."""
        self.assertTrue(_is_contract_like("ACT-HULK29R test", ""))
    
    def test_act_marker_in_comments(self):
        """Should detect ACT- in preceding comments."""
        self.assertTrue(_is_contract_like("some test", "// ACT-HULK29R"))
    
    def test_regression_marker(self):
        """Should detect 'regression' in name."""
        self.assertTrue(_is_contract_like("memory regression test", ""))
    
    def test_leak_marker(self):
        """Should detect 'leak' in name."""
        self.assertTrue(_is_contract_like("leak test", ""))
    
    def test_memory_leak_marker(self):
        """Should detect 'memory leak' in name."""
        self.assertTrue(_is_contract_like("memory leak regression", ""))
    
    def test_rss_marker(self):
        """Should detect 'RSS' in name."""
        self.assertTrue(_is_contract_like("RSS growth test", ""))
    
    def test_ownership_contract_marker(self):
        """Should detect 'ownership contract' in name."""
        self.assertTrue(_is_contract_like("ownership contract test", ""))
    
    def test_case_insensitive(self):
        """Should be case-insensitive."""
        self.assertTrue(_is_contract_like("ACT-test", ""))
        self.assertTrue(_is_contract_like("act-test", ""))
        self.assertTrue(_is_contract_like("Regression", ""))
    
    def test_not_contract_like(self):
        """Should return False for ordinary test names."""
        self.assertFalse(_is_contract_like("basic sanity check", "// regular test"))
        self.assertFalse(_is_contract_like("parser test", ""))


class TestIsTrivialBody(unittest.TestCase):
    """Test _is_trivial_body function."""
    
    def test_expect_true_is_trivial(self):
        """try std.testing.expect(true) is trivial."""
        self.assertTrue(_is_trivial_body("try std.testing.expect(true);"))
    
    def test_expect_equal_true_true_is_trivial(self):
        """try std.testing.expectEqual(true, true) is trivial."""
        self.assertTrue(_is_trivial_body("try std.testing.expectEqual(true, true);"))
    
    def test_return_is_trivial(self):
        """return; is trivial."""
        self.assertTrue(_is_trivial_body("return;"))
    
    def test_expect_with_variable_is_not_trivial(self):
        """expect with a variable is not trivial."""
        self.assertFalse(_is_trivial_body("try std.testing.expect(result == true);"))
    
    def test_allocator_alloc_is_not_trivial(self):
        """allocator.alloc is not trivial."""
        self.assertFalse(_is_trivial_body("const buf = try allocator.alloc(u8, 1024);"))


class TestStripComments(unittest.TestCase):
    """Test _strip_comments function."""
    
    def test_removes_line_comments(self):
        """Should remove // comments."""
        content = '''const x = 1; // this is a comment
const y = 2;'''
        stripped = _strip_comments(content)
        self.assertNotIn("//", stripped)
        self.assertIn("const x = 1;", stripped)
        self.assertIn("const y = 2;", stripped)


class TestCheckFile(unittest.TestCase):
    """Test check_file function."""
    
    def test_fails_act_marked_test_with_expect_true(self):
        """ACT-marked test with only expect(true) should be flagged."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write('''// ACT-HULK29R-ZIG016-MEMOWN03
test "ACT regression test" {
    try std.testing.expect(true);
}
''')
            f.flush()
            
            test_path = Path(f.name)
            errors = check_file(test_path)
            
            os.unlink(f.name)
            
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("documentation-only regression test" in str(e) for e in errors))
    
    def test_fails_regression_with_comments_trivial_assertion(self):
        """Regression test with comments plus trivial assertion should be flagged."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write('''// Regression test for memory leak
test "memory leak regression" {
    // TODO: add real test
    try std.testing.expectEqual(true, true);
}
''')
            f.flush()
            
            test_path = Path(f.name)
            errors = check_file(test_path)
            
            os.unlink(f.name)
            
            self.assertTrue(len(errors) > 0)
    
    def test_passes_memown02_style_with_allocator(self):
        """MEMOWN02-style test with std.testing.allocator should pass."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write('''// MEMOWN02 command runner seam test
test "CliBackend runner seam allocates with allocator" {
    const allocator = std.testing.allocator;
    var fake_runner = FakeWgCommandRunner.init(.{...});
    _ = try cliWireguardStatusWithRunner(allocator, "/fake/wg", fake_runner.asRunner());
}
''')
            f.flush()
            
            test_path = Path(f.name)
            errors = check_file(test_path)
            
            os.unlink(f.name)
            
            self.assertEqual(len(errors), 0)
    
    def test_passes_ordinary_smoke_test(self):
        """Ordinary smoke test with expect(true) should pass (not contract-like)."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write('''// Ordinary smoke test
test "basic sanity check" {
    try std.testing.expect(true);
}
''')
            f.flush()
            
            test_path = Path(f.name)
            errors = check_file(test_path)
            
            os.unlink(f.name)
            
            self.assertEqual(len(errors), 0)
    
    def test_passes_parser_test_with_parsewgdumpoutput(self):
        """Parser test that calls parseWgDumpOutput should pass."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write('''// Parser regression test
test "parseWgDumpOutput: interface name from parameter" {
    const result = try parseWgDumpOutput(dump, "wg0");
    try std.testing.expectEqualStrings("wg0", result.interface);
}
''')
            f.flush()
            
            test_path = Path(f.name)
            errors = check_file(test_path)
            
            os.unlink(f.name)
            
            self.assertEqual(len(errors), 0)
    
    def test_reports_repo_relative_path(self):
        """Should report repo-relative path in errors."""
        # Create a temp file and check its relative path is computed
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False, dir=REPO_ROOT / "tovarisch/src") as f:
            f.write('''// ACT test
test "ACT regression" {
    try std.testing.expect(true);
}
''')
            f.flush()
            
            test_path = Path(f.name)
            errors = check_file(test_path)
            
            os.unlink(f.name)
            
            self.assertTrue(len(errors) > 0)
            # Error path should be relative to repo root
            error_path = errors[0][0]
            self.assertFalse(error_path.startswith("/"))
    
    def test_handles_multiple_test_blocks(self):
        """Should handle multiple test blocks in one file."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write('''// First test is contract-like with trivial body
test "RSS leak regression" {
    try std.testing.expect(true);
}

// Second test is contract-like but has real content
test "memory ownership contract" {
    const allocator = std.testing.allocator;
    _ = try allocator.alloc(u8, 1024);
    try std.testing.expect(true);
}
''')
            f.flush()
            
            test_path = Path(f.name)
            errors = check_file(test_path)
            
            os.unlink(f.name)
            
            # First test should fail, second should pass
            self.assertTrue(len(errors) > 0)
            # Should have exactly 1 error (for the first test)
            self.assertEqual(len(errors), 1)
            self.assertIn("RSS leak regression", str(errors[0]))
    
    def test_handles_return_only_body(self):
        """Test with return; only should fail if contract-like."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write('''// ownership contract test
test "ownership contract" {
    return;
}
''')
            f.flush()
            
            test_path = Path(f.name)
            errors = check_file(test_path)
            
            os.unlink(f.name)
            
            self.assertTrue(len(errors) > 0)
    
    def test_passes_test_with_while_loop(self):
        """Test with while loop should pass."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write('''// Memory leak regression
test "repeated calls do not leak" {
    var i: usize = 0;
    while (i < 100) : (i += 1) {
        const result = try getStatus();
        try std.testing.expect(result.peer_count == 1);
    }
}
''')
            f.flush()
            
            test_path = Path(f.name)
            errors = check_file(test_path)
            
            os.unlink(f.name)
            
            self.assertEqual(len(errors), 0)
    
    def test_passes_test_with_fakewgrunner(self):
        """Test using FakeWgCommandRunner.asRunner() should pass."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write('''// MEMOWN02 test
test "FakeWgCommandRunner seam test" {
    var fake_runner = FakeWgCommandRunner.init(.{...});
    const runner = fake_runner.asRunner();
    try std.testing.expect(runner != null);
}
''')
            f.flush()
            
            test_path = Path(f.name)
            errors = check_file(test_path)
            
            os.unlink(f.name)
            
            self.assertEqual(len(errors), 0)


class TestVerifierSelfTest(unittest.TestCase):
    """Integration test that verifies the verifier passes on the real repo."""
    
    def test_verifier_self_test_passes(self):
        """The verifier's internal self-test should pass."""
        import subprocess
        result = subprocess.run(
            [sys.executable, str(Path(__file__).parent.parent / "scripts/verify_no_doconly_regression_tests.py"), "--self-test"],
            capture_output=True,
            text=True,
            cwd=str(REPO_ROOT)
        )
        
        self.assertEqual(result.returncode, 0, 
            f"Self-test failed:\nstdout: {result.stdout}\nstderr: {result.stderr}")


if __name__ == "__main__":
    unittest.main()
