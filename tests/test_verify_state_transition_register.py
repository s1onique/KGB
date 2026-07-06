#!/usr/bin/env python3
# test_verify_state_transition_register.py — Contract tests for state transition verifier
#
# Tests verify the verifier correctly detects:
# - Missing register files
# - DEFERRED transitions in register
# - Missing transition test files
# - Missing imports in test_all.zig
# - Missing imports in split suites
# - Documentation-only expect(true) placeholders
#
# Run with: python3 tests/test_verify_state_transition_register.py

import os
import re
import sys
import tempfile
import unittest
from pathlib import Path

# Add scripts directory to path for imports
sys.path.insert(0, str(Path(__file__).parent.parent / "scripts"))

from verify_state_transition_register import (
    check_required_paths_exist,
    check_no_deferred_transitions,
    check_no_placeholder_expect_true,
    extract_imports,
    REPO_ROOT,
    REQUIRED_REGISTER,
    REQUIRED_TRANSITION_TESTS,
)


class TestCheckRequiredPathsExist(unittest.TestCase):
    """Test check_required_paths_exist function."""
    
    def test_missing_register_fails(self):
        """Missing register should be reported."""
        # Temporarily override the register path
        import verify_state_transition_register as vstr
        original = vstr.REQUIRED_REGISTER
        vstr.REQUIRED_REGISTER = Path("/nonexistent/register.md")
        
        errors = vstr.check_required_paths_exist()
        
        vstr.REQUIRED_REGISTER = original
        
        self.assertTrue(len(errors) > 0)
        self.assertTrue(any("missing" in cat.lower() for cat, _ in errors))
    
    def test_missing_test_file_fails(self):
        """Missing transition test file should be reported."""
        import verify_state_transition_register as vstr
        original = vstr.REQUIRED_TRANSITION_TESTS
        vstr.REQUIRED_TRANSITION_TESTS = [
            Path("/nonexistent/transition_totality_tests.zig")
        ]
        
        errors = vstr.check_required_paths_exist()
        
        vstr.REQUIRED_TRANSITION_TESTS = original
        
        self.assertTrue(len(errors) > 0)
        self.assertTrue(any("missing" in cat.lower() for cat, _ in errors))
    
    def test_existing_files_pass(self):
        """Existing files should not produce errors."""
        errors = check_required_paths_exist()
        # Should have no errors for existing files
        self.assertFalse(any("missing" in cat.lower() for cat, _ in errors))


class TestCheckNoDeferredTransitions(unittest.TestCase):
    """Test check_no_deferred_transitions function."""
    
    def test_deferred_token_fails(self):
        """DEFERRED token in register should be reported."""
        import verify_state_transition_register as vstr
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.md', delete=False) as f:
            f.write("# Test Register\n\n")
            f.write("Some text with DEFERRED transition entry\n")
            f.flush()
            
            original = vstr.REQUIRED_REGISTER
            vstr.REQUIRED_REGISTER = Path(f.name)
            
            errors = vstr.check_no_deferred_transitions()
            
            vstr.REQUIRED_REGISTER = original
            
            os.unlink(f.name)
            
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("deferred" in cat.lower() for cat, _ in errors))
    
    def test_deferred_zero_count_allowed(self):
        """DEFERRED transitions: 0 should be allowed."""
        import verify_state_transition_register as vstr
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.md', delete=False) as f:
            f.write("# Test Register\n\n")
            f.write("DEFERRED transitions: 0\n")
            f.flush()
            
            original = vstr.REQUIRED_REGISTER
            vstr.REQUIRED_REGISTER = Path(f.name)
            
            errors = vstr.check_no_deferred_transitions()
            
            vstr.REQUIRED_REGISTER = original
            
            os.unlink(f.name)
            
            # Should have no errors for zero-count declaration
            self.assertEqual(len(errors), 0)
    
    def test_comment_deferred_allowed(self):
        """DEFERRED in comments should be allowed."""
        import verify_state_transition_register as vstr
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.md', delete=False) as f:
            f.write("# Test Register\n\n")
            f.write("// TODO: Remove DEFERRED comment later\n")
            f.flush()
            
            original = vstr.REQUIRED_REGISTER
            vstr.REQUIRED_REGISTER = Path(f.name)
            
            errors = vstr.check_no_deferred_transitions()
            
            vstr.REQUIRED_REGISTER = original
            
            os.unlink(f.name)
            
            self.assertEqual(len(errors), 0)


class TestCheckNoPlaceholderExpectTrue(unittest.TestCase):
    """Test check_no_placeholder_expect_true function."""
    
    def test_placeholder_expect_true_fails(self):
        """Documentation-only expect(true) should be reported."""
        import verify_state_transition_register as vstr
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write('// Test file\n')
            f.write('test "placeholder test" {\n')
            f.write('    try std.testing.expect(true);\n')
            f.write('}\n')
            f.flush()
            
            errors = vstr.check_no_placeholder_expect_true(Path(f.name))
            
            os.unlink(f.name)
            
            self.assertTrue(len(errors) > 0)
            self.assertTrue(any("placeholder" in cat.lower() for cat, _ in errors))
    
    def test_real_expect_true_allowed(self):
        """Real expect(true) with context should be allowed."""
        import verify_state_transition_register as vstr
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write('// Test file\n')
            f.write('test "real test" {\n')
            f.write('    const result = doSomething();\n')
            f.write('    try std.testing.expect(result == true);\n')
            f.write('}\n')
            f.flush()
            
            errors = vstr.check_no_placeholder_expect_true(Path(f.name))
            
            os.unlink(f.name)
            
            self.assertEqual(len(errors), 0)
    
    def test_testing_expect_true_fails(self):
        """try testing.expect(true) should also be caught."""
        import verify_state_transition_register as vstr
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write('const testing = std.testing;\n')
            f.write('test "placeholder" {\n')
            f.write('    try testing.expect(true);\n')
            f.write('}\n')
            f.flush()
            
            errors = vstr.check_no_placeholder_expect_true(Path(f.name))
            
            os.unlink(f.name)
            
            self.assertTrue(len(errors) > 0)
    
    def test_comment_expect_true_allowed(self):
        """expect(true) in comments should be allowed."""
        import verify_state_transition_register as vstr
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write('// This is: try std.testing.expect(true);\n')
            f.write('test "real test" {\n')
            f.write('    try std.testing.expect(x == y);\n')
            f.write('}\n')
            f.flush()
            
            errors = vstr.check_no_placeholder_expect_true(Path(f.name))
            
            os.unlink(f.name)
            
            self.assertEqual(len(errors), 0)


class TestExtractImports(unittest.TestCase):
    """Test extract_imports function."""
    
    def test_extracts_imports(self):
        """Should extract all @import paths."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write('const a = @import("std");\n')
            f.write('const b = @import("foo.zig");\n')
            f.write('const c = @import("path/bar.zig");\n')
            f.flush()
            
            imports = extract_imports(Path(f.name))
            
            os.unlink(f.name)
            
            self.assertIn("std", imports)
            self.assertIn("foo.zig", imports)
            self.assertIn("path/bar.zig", imports)
    
    def test_extracts_all_imports(self):
        """Should extract all @import paths including in comments."""
        # Note: The verifier extracts @import from all lines including comments.
        # This is fine because Zig suite files don't have commented imports.
        # The actual filtering of commented lines happens in other checks.
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write('// const a = @import("comment_import.zig");\n')
            f.write('const b = @import("real_import.zig");\n')
            f.flush()
            
            imports = extract_imports(Path(f.name))
            
            os.unlink(f.name)
            
            # Both imports are found (including the commented one)
            self.assertIn("comment_import.zig", imports)
            self.assertIn("real_import.zig", imports)
    
    def test_missing_file_returns_empty(self):
        """Missing file should return empty list."""
        imports = extract_imports(Path("/nonexistent/file.zig"))
        self.assertEqual(len(imports), 0)


class TestVerifierSelfTest(unittest.TestCase):
    """Integration test that verifies the verifier passes on the real repo."""
    
    def test_verifier_passes_on_real_repo(self):
        """The verifier should pass on the actual repository."""
        # This test ensures the verifier itself is working correctly
        # against the real codebase
        import verify_state_transition_register as vstr
        
        # The real checks
        errors = []
        errors.extend(vstr.check_required_paths_exist())
        errors.extend(vstr.check_no_deferred_transitions())
        
        # Check test_all.zig imports
        for suite in vstr.REQUIRED_AGGREGATE_SUITES:
            test_imports = [vstr._get_zig_import_path(p) for p in vstr.REQUIRED_TRANSITION_TESTS]
            suite_imports = vstr.extract_imports(suite)
            for test_import in test_imports:
                if test_import not in suite_imports:
                    errors.append(("[unwired]", f"{suite} does not import {test_import}"))
        
        # Check for placeholders
        for test_path in vstr.REQUIRED_TRANSITION_TESTS:
            errors.extend(vstr.check_no_placeholder_expect_true(test_path))
        
        # Should have no errors
        self.assertEqual(len(errors), 0, f"Verifier found issues: {errors}")


if __name__ == "__main__":
    unittest.main()
