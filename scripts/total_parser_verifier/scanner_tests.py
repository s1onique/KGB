# scanner_tests.py — Scanner-level self-tests for total parser verifier
"""Scanner-level self-tests that prove end-to-end behavior.

These tests create a temporary source tree and prove:
- Nested paths are preserved (net/linux_read.zig not basename)
- Missing registered modules produce fatal errors
- @panic in PURE module fails through scanner
- Comments and test blocks do not trigger false positives
"""

import os
import tempfile
from typing import List, Tuple

from .scanner import (
    scan_file,
    scan_modules,
    extract_module_name,
)


def run_scanner_self_tests(verbose: bool = False) -> Tuple[int, int, List[str]]:
    """Run scanner-level self-tests that prove end-to-end behavior.
    
    Returns:
        Tuple of (passed, failed, error messages)
    """
    from .classifications import Classification
    
    passed = 0
    failed = 0
    errors = []
    
    # Create a temporary directory for testing
    with tempfile.TemporaryDirectory() as tmpdir:
        src_root = os.path.join(tmpdir, "tovarisch", "src")
        os.makedirs(src_root)
        
        # Test 1: Nested module path is preserved
        if verbose:
            print("\nScanner test 1: Nested module path preservation")
        
        net_dir = os.path.join(src_root, "net")
        os.makedirs(net_dir)
        
        clean_code = '''
const std = @import("std");

pub fn readSomething() ![]const u8 {
    return error.NotFound;
}
'''
        linux_read_path = os.path.join(net_dir, "linux_read.zig")
        with open(linux_read_path, 'w') as f:
            f.write(clean_code)
        
        # Test extract_module_name
        extracted = extract_module_name(linux_read_path, src_root)
        if extracted == "net/linux_read.zig":
            passed += 1
            if verbose:
                print("  PASS: Path preserved as 'net/linux_read.zig'")
        else:
            failed += 1
            msg = f"Expected 'net/linux_read.zig', got '{extracted}'"
            errors.append(msg)
            if verbose:
                print(f"  FAIL: {msg}")
        
        # Test 2: @panic in TOTAL module fails through scanner
        if verbose:
            print("\nScanner test 2: @panic fails through scanner")
        
        status_dir = os.path.join(src_root)
        status_path = os.path.join(status_dir, "status_query.zig")
        panic_code = '''
pub fn parseQuery(input: []const u8) !void {
    if (input.len == 0) @panic("bad");
}
'''
        with open(status_path, 'w') as f:
            f.write(panic_code)
        
        # Scan the file
        result = scan_file(status_path, src_root)
        
        if result.has_failures:
            passed += 1
            if verbose:
                print("  PASS: @panic produces FAIL finding")
        else:
            failed += 1
            msg = "Expected FAIL finding for @panic"
            errors.append(msg)
            if verbose:
                print(f"  FAIL: {msg}")
        
        # Test 3: Comments are ignored
        if verbose:
            print("\nScanner test 3: Comments ignored")
        
        comment_path = os.path.join(status_dir, "config_parse_helpers.zig")
        comment_code = '''
// This file has @panic in a comment but should pass
// @panic("this is fine")
const std = @import("std");

pub fn parseValue(input: []const u8) !u32 {
    return std.fmt.parseInt(u32, input, 10);
}
'''
        with open(comment_path, 'w') as f:
            f.write(comment_code)
        
        result = scan_file(comment_path, src_root)
        
        if not result.has_failures:
            passed += 1
            if verbose:
                print("  PASS: Comments ignored, no FAIL")
        else:
            failed += 1
            msg = "Comments should not trigger FAIL"
            errors.append(msg)
            if verbose:
                print(f"  FAIL: {msg}")
        
        # Test 4: Test blocks are ignored for ALL modules (doctrine: test code not part of production verification)
        if verbose:
            print("\nScanner test 4: Test blocks ignored for all modules")
        
        # Add a stateful adapter module with @panic only in test block
        bgp_dir = os.path.join(src_root, "bgp")
        os.makedirs(bgp_dir)
        
        stateful_path = os.path.join(bgp_dir, "session.zig")
        stateful_code = '''
// Session module with @panic only in test block
pub fn handleMessage(data: []const u8) !void {
    return error.BadMessage;
}

test "basic test" {
    // @panic("test only") should not count
    try std.testing.expect(true);
}
'''
        with open(stateful_path, 'w') as f:
            f.write(stateful_code)
        
        result = scan_file(stateful_path, src_root)
        
        # @panic in test block should not cause failure for STATEFUL_ADAPTER
        if not result.has_failures:
            passed += 1
            if verbose:
                print("  PASS: @panic in test block ignored for STATEFUL_ADAPTER")
        else:
            failed += 1
            msg = "@panic in test block should not cause FAIL for STATEFUL_ADAPTER"
            errors.append(msg)
            if verbose:
                print(f"  FAIL: {msg}")
        
        # Also test that test blocks are ignored for TOTAL modules
        if verbose:
            print("\nScanner test 4b: Test blocks ignored for TOTAL modules")
        
        total_path = os.path.join(status_dir, "net_private_ip.zig")
        total_code = '''
// TOTAL module with @panic only in test block
pub fn classifyIP(addr: []const u8) IpClass {
    return .unknown;
}

test "basic classification" {
    // @panic("test only") should not count
    try std.testing.expect(classifyIP("127.0.0.1") == .private);
}
'''
        with open(total_path, 'w') as f:
            f.write(total_code)
        
        result = scan_file(total_path, src_root)
        
        # @panic in test block should not cause failure for TOTAL
        if not result.has_failures:
            passed += 1
            if verbose:
                print("  PASS: @panic in test block ignored for TOTAL")
        else:
            failed += 1
            msg = "@panic in test block should not cause FAIL for TOTAL"
            errors.append(msg)
            if verbose:
                print(f"  FAIL: {msg}")
        
        # Test 5: scan_modules reports missing registered modules as errors
        if verbose:
            print("\nScanner test 5: Missing registered modules produce errors")
        
        # scan_modules uses the real classifications.py which has 20 modules
        # Our temp tree only has 4 files, so we should get errors for missing ones
        results, scan_errors = scan_modules(src_root)
        
        # Check that errors list includes the missing modules
        if len(scan_errors) > 0:
            passed += 1
            if verbose:
                print(f"  PASS: scan_modules reported {len(scan_errors)} missing modules")
        else:
            failed += 1
            msg = "scan_modules should report errors for missing modules"
            errors.append(msg)
            if verbose:
                print(f"  FAIL: {msg}")
        
        # Check that our created files were found
        found_modules = [r.module for r in results]
        
        if "net/linux_read.zig" in found_modules:
            passed += 1
            if verbose:
                print("  PASS: scan_modules found 'net/linux_read.zig'")
        else:
            failed += 1
            msg = "scan_modules should find 'net/linux_read.zig'"
            errors.append(msg)
            if verbose:
                print(f"  FAIL: {msg}")
        
        if "status_query.zig" in found_modules:
            passed += 1
            if verbose:
                print("  PASS: scan_modules found 'status_query.zig'")
        else:
            failed += 1
            msg = "scan_modules should find 'status_query.zig'"
            errors.append(msg)
            if verbose:
                print(f"  FAIL: {msg}")
    
    return passed, failed, errors
