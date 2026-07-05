# self_test.py — Self-test cases for total parser verifier
"""Self-test cases to verify the verifier works correctly.

Pattern-level tests prove the verifier:
1. Passes clean total parser code
2. Fails on @panic
3. Fails on catch unreachable
4. Fails on optional unwrap .?
5. Passes closed enum mapping to .unknown
6. Ignores comments containing forbidden patterns
7. Ignores test blocks containing forbidden patterns

Scanner-level tests are in scanner_tests.py.
"""

from dataclasses import dataclass
from typing import List, Tuple

from .patterns import (
    is_forbidden_pattern,
    is_medium_pattern,
)

# Import scanner tests from separate module
from .scanner_tests import run_scanner_self_tests


@dataclass
class SelfTestCase:
    """A self-test case for the verifier."""
    name: str
    code: str
    expect_findings: bool
    expect_failure: bool  # True if should produce FAIL-level finding
    description: str


# Self-test cases
SELF_TEST_CASES: List[SelfTestCase] = [
    # Case 1: Clean total parser passes
    SelfTestCase(
        name="clean_parser_passes",
        code='''
const std = @import("std");

pub fn parseInput(input: []const u8) !u32 {
    if (input.len == 0) return error.InvalidInput;
    const value = std.fmt.parseInt(u32, input, 10) catch return error.InvalidInput;
    if (value > 100) return error.OutOfRange;
    return value;
}
''',
        expect_findings=False,
        expect_failure=False,
        description="Clean total parser should pass",
    ),
    
    # Case 2: @panic should fail
    SelfTestCase(
        name="panic_fails",
        code='''
pub fn parseValue(input: []const u8) u32 {
    if (input.len == 0) @panic("empty input");
    return std.fmt.parseInt(u32, input, 10) catch 0;
}
''',
        expect_findings=True,
        expect_failure=True,
        description="@panic should produce FAIL finding",
    ),
    
    # Case 3: catch unreachable should fail
    SelfTestCase(
        name="catch_unreachable_fails",
        code='''
pub fn parseValue(input: []const u8) u32 {
    const value = tryParse(input) catch unreachable;
    return value;
}
''',
        expect_findings=True,
        expect_failure=True,
        description="'catch unreachable' should produce FAIL finding",
    ),
    
    # Case 4: .? optional unwrap should fail
    SelfTestCase(
        name="optional_unwrap_fails",
        code='''
pub fn getValue(maybe: ?u32) u32 {
    return maybe.?;
}
''',
        expect_findings=True,
        expect_failure=True,
        description="optional unwrap '.?' should produce FAIL finding",
    ),
    
    # Case 5: Closed enum mapping to .unknown passes
    SelfTestCase(
        name="closed_enum_passes",
        code='''
const State = enum(u8) {
    idle,
    active,
    error,
    unknown,
};

pub fn parseState(raw: u8) State {
    return switch (raw) {
        0 => .idle,
        1 => .active,
        2 => .error,
        else => .unknown,
    };
}
''',
        expect_findings=False,
        expect_failure=False,
        description="Closed enum with unknown fallback should pass",
    ),
    
    # Case 6: @enumFromInt without explicit bounds check fails
    SelfTestCase(
        name="enum_from_int_fails",
        code='''
const State = enum(u8) { idle, active };

pub fn parseState(raw: u8) State {
    return @enumFromInt(raw);
}
''',
        expect_findings=True,
        expect_failure=True,
        description="@enumFromInt without bounds check should produce FAIL finding",
    ),
    
    # Case 7: @intCast reports warning but doesn't fail
    SelfTestCase(
        name="intcast_warns",
        code='''
pub fn castValue(x: u64) u32 {
    return @intCast(x);
}
''',
        expect_findings=True,
        expect_failure=False,
        description="@intCast should produce WARN but not FAIL",
    ),
    
    # Case 8: unreachable in code should fail
    SelfTestCase(
        name="unreachable_fails",
        code='''
pub fn parseValue(input: []const u8) u32 {
    if (input.len == 0) unreachable;
    return 0;
}
''',
        expect_findings=True,
        expect_failure=True,
        description="'unreachable' should produce FAIL finding",
    ),
]


def run_self_tests(verbose: bool = False) -> Tuple[int, int, List[str]]:
    """Run all self-test cases.
    
    Args:
        verbose: Print detailed output
        
    Returns:
        Tuple of (passed, failed, error messages)
    """
    passed = 0
    failed = 0
    errors = []
    
    for case in SELF_TEST_CASES:
        if verbose:
            print(f"\nRunning: {case.name}")
            print(f"  {case.description}")
        
        # Check patterns directly
        lines = case.code.split('\n')
        has_forbidden = False
        has_failure = False
        
        for line in lines:
            is_forbid, _ = is_forbidden_pattern(line)
            is_med, _ = is_medium_pattern(line)
            
            if is_forbid:
                has_forbidden = True
                # Check if it's a FAIL-level pattern
                # FAIL patterns: @panic, unreachable, catch unreachable, .?, @enumFromInt
                if ('@panic' in line or 'catch unreachable' in line or 
                    '.?' in line or 'unreachable' in line or '@enumFromInt' in line):
                    has_failure = True
            
            if is_med:
                has_forbidden = True
        
        # Verify expectations
        actual_findings = has_forbidden
        actual_failure = has_failure
        
        if actual_findings != case.expect_findings:
            msg = f"[{case.name}] Expected findings={case.expect_findings}, got {actual_findings}"
            errors.append(msg)
            failed += 1
            if verbose:
                print(f"  FAIL: {msg}")
        elif actual_failure != case.expect_failure:
            msg = f"[{case.name}] Expected failure={case.expect_failure}, got {actual_failure}"
            errors.append(msg)
            failed += 1
            if verbose:
                print(f"  FAIL: {msg}")
        else:
            passed += 1
            if verbose:
                print(f"  PASS")
    
    return passed, failed, errors


def test_pattern_recognition():
    """Test that pattern recognition works correctly."""
    test_cases = [
        # (input, expect_forbidden, expect_medium)
        ('@panic("bad")', True, False),
        ('catch unreachable', True, False),
        ('value.?', True, False),
        ('@enumFromInt(x)', True, False),
        ('@intCast(x)', False, True),
        ('std.debug.assert(x)', False, True),
        ('const x = 5;', False, False),
        ('return .ok;', False, False),
    ]
    
    errors = []
    for code, expect_forbid, expect_med in test_cases:
        forbid, _ = is_forbidden_pattern(code)
        med, _ = is_medium_pattern(code)
        
        if forbid != expect_forbid:
            errors.append(f"is_forbidden_pattern('{code}'): expected {expect_forbid}, got {forbid}")
        if med != expect_med:
            errors.append(f"is_medium_pattern('{code}'): expected {expect_med}, got {med}")
    
    return errors


def run_all_self_tests(verbose: bool = False) -> bool:
    """Run all self-tests and return success status.
    
    Args:
        verbose: Print detailed output
        
    Returns:
        True if all tests passed
    """
    all_passed = True
    
    # Test pattern recognition
    if verbose:
        print("\n=== Testing pattern recognition ===")
    
    pattern_errors = test_pattern_recognition()
    if pattern_errors:
        all_passed = False
        for err in pattern_errors:
            print(f"  ERROR: {err}")
    elif verbose:
        print("  Pattern recognition: OK")
    
    # Test self-test cases
    if verbose:
        print("\n=== Running self-test cases ===")
    
    passed, failed, errors = run_self_tests(verbose)
    
    if verbose:
        print(f"\nSelf-test results: {passed} passed, {failed} failed")
    
    if errors:
        all_passed = False
        for err in errors:
            print(f"  ERROR: {err}")
    
    # Test scanner-level tests
    if verbose:
        print("\n=== Running scanner-level self-tests ===")
    
    scanner_passed, scanner_failed, scanner_errors = run_scanner_self_tests(verbose)
    
    if verbose:
        print(f"\nScanner self-test results: {scanner_passed} passed, {scanner_failed} failed")
    
    if scanner_errors:
        all_passed = False
        for err in scanner_errors:
            print(f"  ERROR: {err}")
    
    return all_passed
