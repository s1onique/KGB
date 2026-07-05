"""
Self-test module for effect boundary verifier.

Contains self-test logic to verify the verifier works correctly.
"""

import os
import shutil
import tempfile
from pathlib import Path
from typing import List, Set

from .scanner import Violation, scan_directory


def run_self_test() -> bool:
    """
    Run self-test to verify the verifier works correctly.
    
    Tests:
    1. Clean tree should pass
    2. PURE module using std.fs.cwd() should fail
    3. PURE module using std.process should fail
    4. BOUNDARY module using std.fs.cwd() should pass
    5. Production import of test file should fail
    6. Comments containing forbidden patterns should not trigger
    
    Returns:
        True if all tests pass, False otherwise
    """
    print("[self-test] Running effect boundary verifier self-test...")
    
    all_passed = True
    test_dir = None
    
    # Import classification sets for self-test
    # These are imported here to avoid circular imports
    from .classifications import (
        PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES
    )
    
    try:
        # Create temporary directory for test files
        test_dir = tempfile.mkdtemp(prefix="effect_boundary_test_")
        test_path = Path(test_dir)
        
        # =====================================================================
        # Test 1: Clean tree should pass
        # =====================================================================
        print("[self-test] Test 1: Clean PURE module should pass...")
        
        clean_module = test_path / "clean_module.zig"
        clean_module.write_text('''
const std = @import("std");

pub const MyEnum = enum { a, b, c };

pub fn parse(input: []const u8) MyEnum {
    if (std.mem.eql(u8, input, "a")) return .a;
    return .b;
}
''')
        
        violations, test_imports, _ = scan_directory(
            test_path, PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES
        )
        if violations or test_imports:
            print(f"[FAIL] Test 1: Clean module should not have violations")
            all_passed = False
        else:
            print("[PASS] Test 1: Clean module passed")
        
        # =====================================================================
        # Test 2: PURE module using std.fs.cwd() should fail
        # =====================================================================
        print("[self-test] Test 2: PURE module with std.fs.cwd() should fail...")
        
        cwd_module = test_path / "cwd_violation.zig"
        cwd_module.write_text('''
const std = @import("std");

pub fn readCurrentDir() !void {
    const cwd = std.fs.cwd();
    _ = cwd;
}
''')
        
        # Force this file to be treated as PURE
        violations, _, _ = scan_directory(
            test_path, PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES,
            force_modules={"cwd_violation.zig"}
        )
        
        cwd_violations = [v for v in violations if "cwd_violation.zig" in v.file and "std.fs.cwd" in v.description]
        if not cwd_violations:
            print(f"[FAIL] Test 2: std.fs.cwd() violation not detected")
            all_passed = False
        else:
            print(f"[PASS] Test 2: std.fs.cwd() violation detected at line {cwd_violations[0].line}")
        
        # =====================================================================
        # Test 3: PURE module using std.process should fail
        # =====================================================================
        print("[self-test] Test 3: PURE module with std.process should fail...")
        
        process_module = test_path / "process_violation.zig"
        process_module.write_text('''
const std = @import("std");

pub fn spawnProcess() !void {
    try std.process.spawn();
}
''')
        
        violations, _, _ = scan_directory(
            test_path, PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES,
            force_modules={"process_violation.zig"}
        )
        
        process_violations = [v for v in violations if "process_violation.zig" in v.file and "std.process" in v.description]
        if not process_violations:
            print(f"[FAIL] Test 3: std.process violation not detected")
            all_passed = False
        else:
            print(f"[PASS] Test 3: std.process violation detected at line {process_violations[0].line}")
        
        # =====================================================================
        # Test 4: BOUNDARY module using std.fs.cwd() should pass
        # =====================================================================
        print("[self-test] Test 4: BOUNDARY module with std.fs.cwd() should pass...")
        
        # First, add the test file to BOUNDARY modules
        BOUNDARY_MODULES.add("boundary_allowed.zig")
        
        boundary_module = test_path / "boundary_allowed.zig"
        boundary_module.write_text('''
const std = @import("std");

// This is a BOUNDARY module - effects are allowed
pub fn readCurrentDir() !void {
    const cwd = std.fs.cwd();
    _ = cwd;
}
''')
        
        violations, _, _ = scan_directory(
            test_path, PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES,
            force_modules=set()  # Empty - don't force to PURE
        )
        
        # Filter to only the BOUNDARY module's violations
        boundary_violations = [v for v in violations if "boundary_allowed.zig" in v.file]
        if boundary_violations:
            print(f"[FAIL] Test 4: BOUNDARY module should not have violations")
            all_passed = False
        else:
            print("[PASS] Test 4: BOUNDARY module passed")
        
        # Clean up
        BOUNDARY_MODULES.discard("boundary_allowed.zig")
        
        # =====================================================================
        # Test 5: Production import of test file should fail
        # =====================================================================
        print("[self-test] Test 5: Production import of *_tests.zig should fail...")
        
        test_file = test_path / "my_tests.zig"
        test_file.write_text('''
test "sample" {
    // This is a test file
}
''')
        
        prod_file = test_path / "producer.zig"
        prod_file.write_text('''
const std = @import("std");
const my_tests = @import("my_tests.zig");
''')
        
        violations, test_imports, _ = scan_directory(
            test_path, PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES,
            force_modules={"producer.zig"}
        )
        
        prod_imports = [(f, imp, line) for f, imp, line in test_imports if "producer.zig" in f]
        if not prod_imports:
            print(f"[FAIL] Test 5: Production import of test file not detected")
            all_passed = False
        else:
            print(f"[PASS] Test 5: Production import of test file detected at line {prod_imports[0][2]}")
        
        # =====================================================================
        # Test 6: Comment containing forbidden pattern should not trigger
        # =====================================================================
        print("[self-test] Test 6: Comments with forbidden pattern should not trigger...")
        
        comment_module = test_path / "comment_module.zig"
        comment_module.write_text('''
const std = @import("std");

// NOTE: std.fs.cwd() should not be used in pure functions.
// std.process is forbidden for PURE modules.
// @panic is not allowed on external input.
pub fn pureFunction() []const u8 {
    return "hello";
}
''')
        
        violations, _, _ = scan_directory(
            test_path, PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES,
            force_modules={"comment_module.zig"}
        )
        
        comment_violations = [v for v in violations if "comment_module.zig" in v.file]
        if comment_violations:
            print(f"[FAIL] Test 6: Comment exclusion failed - found violations")
            for v in comment_violations:
                print(f"      Line {v.line}: {v.description}")
            all_passed = False
        else:
            print("[PASS] Test 6: Comments correctly excluded")
        
    finally:
        # Cleanup
        if test_dir and os.path.exists(test_dir):
            shutil.rmtree(test_dir)
    
    return all_passed
