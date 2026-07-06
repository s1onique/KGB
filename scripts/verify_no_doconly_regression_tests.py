#!/usr/bin/env python3
# verify_no_doconly_regression_tests.py — Reject documentation-only regression/ACT/memory tests
#
# ACT-HULK29R-ZIG016-MEMOWN03-NO-DOCONLY-REGRESSION-TESTS
#
# This verifier scans Zig test files and fails when a test appears to be a
# regression/ACT/memory/leak contract but its body is documentation-only.
#
# A test is "contract-like" if its name or nearby comments contain any of these
# markers (case-insensitive):
#   ACT-, regression, leak, memory leak, memory ownership, RSS, ownership contract
#
# A suspicious test fails if its body is trivial, e.g.:
#   try std.testing.expect(true);
#   try std.testing.expectEqual(true, true);
#   return;
#
# Exit codes:
#   0 — all checks pass
#   1 — verification failed
#   2 — internal error

import re
import sys
import os
from pathlib import Path
from typing import List, Tuple, Optional

# Repository root relative paths
REPO_ROOT = Path(__file__).parent.parent

# Markers that indicate a test is contract-like (ACT, regression, memory, etc.)
CONTRACT_MARKERS = [
    "ACT-",
    "regression",
    "leak",
    "memory leak",
    "memory ownership",
    "RSS",
    "ownership contract",
]

# Patterns for trivial body detection (fail if only these are present)
TRIVIAL_PATTERNS = [
    re.compile(r'^\s*try\s+std\.testing\.expect\s*\(\s*true\s*\)\s*;?\s*$'),
    re.compile(r'^\s*try\s+std\.testing\.expectEqual\s*\(\s*true\s*,\s*true\s*\)\s*;?\s*$'),
    re.compile(r'^\s*try\s+std\.testing\.expectEqual\s*\(\s*@as\s*\(\s*bool\s*,\s*true\s*\)\s*,\s*true\s*\)\s*;?\s*$'),
    re.compile(r'^\s*return\s*;?\s*$'),
]

# Patterns that indicate meaningful test content (evidence of executable contract)
MEANINGFUL_MARKERS = [
    "std.testing.allocator",
    ".deinit(",
    "allocator.alloc",
    "allocator.dupe",
    "allocator.free",
    "Fake",
    "asRunner(",
    "cliWireguardStatusWithRunner",
    "parseWgDumpOutput",
    "while (",
    "for (",
    "expectEqual(@as(",
    "expect(result ==",
    "try wg_cli",
    "FakeWgCommandRunner",
    "FakeBackend",
]


def _relative_path(path: Path) -> str:
    """Return repo-relative path for cleaner diagnostics."""
    try:
        return str(path.relative_to(REPO_ROOT))
    except ValueError:
        return str(path)


def _find_test_blocks(content: str) -> List[Tuple[str, str, str, int]]:
    """
    Find all test blocks in Zig source code.
    
    Returns list of (test_name, test_body, preceding_comments, line_number)
    where:
      test_name: the quoted test name
      test_body: the content between braces
      preceding_comments: ~10 lines of comments before the test
      line_number: 1-based line where test starts
    """
    tests = []
    lines = content.split('\n')
    
    i = 0
    while i < len(lines):
        line = lines[i]
        
        # Look for test declaration: test "name" {
        match = re.match(r'^\s*test\s+"([^"]+)"\s*\{', line)
        if match:
            test_name = match.group(1)
            start_line = i + 1  # 1-based
            
            # Collect preceding comments (up to 10 lines before)
            comment_start = max(0, i - 10)
            preceding_comments = '\n'.join(lines[comment_start:i])
            
            # Parse the test body with brace counting
            body_lines = []
            brace_count = 1
            j = i + 1
            
            while j < len(lines) and brace_count > 0:
                body_line = lines[j]
                for ch in body_line:
                    if ch == '{':
                        brace_count += 1
                    elif ch == '}':
                        brace_count -= 1
                body_lines.append(body_line)
                j += 1
            
            test_body = '\n'.join(body_lines[:-1])  # Exclude closing brace
            tests.append((test_name, test_body, preceding_comments, start_line))
            
            i = j - 1  # Continue after the closing brace
        
        i += 1
    
    return tests


def _is_contract_like(test_name: str, preceding_comments: str) -> bool:
    """Check if test is marked as a contract-like test (ACT, regression, etc.)."""
    combined = (test_name + '\n' + preceding_comments).lower()
    return any(marker.lower() in combined for marker in CONTRACT_MARKERS)


def _is_trivial_body(test_body: str) -> bool:
    """Check if test body contains only trivial assertions or comments."""
    # Normalize: remove comments, collapse whitespace
    normalized_lines = []
    for line in test_body.split('\n'):
        stripped = line.strip()
        # Remove line comments
        if '//' in stripped:
            idx = stripped.index('//')
            stripped = stripped[:idx].strip()
        if stripped:
            normalized_lines.append(stripped)
    
    if not normalized_lines:
        return True
    
    # Check if all remaining lines are trivial patterns
    for line in normalized_lines:
        is_trivial = any(pattern.match(line) for pattern in TRIVIAL_PATTERNS)
        if not is_trivial:
            return False
    
    return True


def _has_meaningful_content(test_body: str) -> bool:
    """Check if test body contains meaningful evidence of executable behavior."""
    for marker in MEANINGFUL_MARKERS:
        if marker in test_body:
            return True
    return False


def _strip_comments(content: str) -> str:
    """Remove // comments from content."""
    lines = []
    for line in content.split('\n'):
        if '//' in line:
            idx = line.index('//')
            line = line[:idx]
        lines.append(line)
    return '\n'.join(lines)


def check_file(path: Path) -> List[Tuple[str, str, int]]:
    """
    Check a single Zig test file for documentation-only regression tests.
    
    Returns list of (file_path, error_message, line_number) tuples.
    """
    errors = []
    
    if not path.exists():
        return errors
    
    try:
        content = path.read_text()
    except Exception as e:
        errors.append((_relative_path(path), f"failed to read file: {e}", 0))
        return errors
    
    test_blocks = _find_test_blocks(content)
    
    for test_name, test_body, preceding_comments, start_line in test_blocks:
        if _is_contract_like(test_name, preceding_comments):
            # This is a contract-like test - check if body is trivial
            stripped_body = _strip_comments(test_body)
            
            if _is_trivial_body(stripped_body):
                errors.append((
                    _relative_path(path),
                    f"documentation-only regression test:\n"
                    f"  test \"{test_name}\"\n"
                    f"  reason: contract-like marker found near test, but body only asserts true\n"
                    f"  fix: remove the test, rename it as documentation, or replace it with an executable seam/allocator test",
                    start_line
                ))
    
    return errors


def find_zig_test_files() -> List[Path]:
    """Find all Zig test files in the repository."""
    test_files = []
    
    # Scan tovarisch/src for Zig files
    tovarisch_src = REPO_ROOT / "tovarisch/src"
    if tovarisch_src.exists():
        for zig_file in tovarisch_src.rglob("*.zig"):
            # Check if file contains test blocks
            try:
                content = zig_file.read_text()
                if 'test "' in content:
                    test_files.append(zig_file)
            except Exception:
                pass
    
    return sorted(test_files)


def main() -> int:
    """Run all doc-only regression test checks."""
    all_errors = []
    
    # Find all Zig test files
    test_files = find_zig_test_files()
    
    if not test_files:
        print("DOC-ONLY REGRESSION TESTS VERIFIER: PASS")
        print("note: no Zig test files found")
        return 0
    
    # Check each test file
    for test_file in test_files:
        file_errors = check_file(test_file)
        for file_path, message, line in file_errors:
            if line > 0:
                all_errors.append(f"{file_path}:{line}: {message}")
            else:
                all_errors.append(f"{file_path}: {message}")
    
    # Handle --self-test flag
    if "--self-test" in sys.argv:
        _run_self_test(all_errors)
        return 0
    
    # Output results
    if all_errors:
        print("DOC-ONLY REGRESSION TESTS VERIFIER: FAIL")
        for error in sorted(all_errors):
            print(error)
        return 1
    
    print("DOC-ONLY REGRESSION TESTS VERIFIER: PASS")
    print(f"checked_files={len(test_files)}")
    
    return 0


def _run_self_test(errors: List[str]) -> None:
    """Run self-test verification."""
    import tempfile
    import os
    
    print("Running self-test...")
    
    test_cases = [
        # Case 1: ACT-marked test with only expect(true) - should fail
        {
            "name": "ACT-marked expect(true) should fail",
            "content": '''
// ACT-HULK29R-ZIG016-MEMOWN03
test "ACT regression test" {
    try std.testing.expect(true);
}
''',
            "should_fail": True,
        },
        # Case 2: regression test with comments plus trivial assertion - should fail
        {
            "name": "regression with trivial assertion should fail",
            "content": '''
// Regression test for memory leak
test "memory leak regression" {
    // TODO: add real test
    try std.testing.expectEqual(true, true);
}
''',
            "should_fail": True,
        },
        # Case 3: MEMOWN02-style test with allocator - should pass
        {
            "name": "MEMOWN02-style with std.testing.allocator should pass",
            "content": '''
// MEMOWN02 command runner seam test
const std = @import("std");

test "CliBackend runner seam allocates with allocator" {
    const allocator = std.testing.allocator;
    var fake_runner = FakeWgCommandRunner.init(.{...});
    _ = try cliWireguardStatusWithRunner(allocator, "/fake/wg", fake_runner.asRunner());
}
''',
            "should_fail": False,
        },
        # Case 4: ordinary smoke test with expect(true) - should pass (not contract-like)
        {
            "name": "ordinary smoke test should pass",
            "content": '''
// Ordinary smoke test
test "basic sanity check" {
    try std.testing.expect(true);
}
''',
            "should_fail": False,
        },
        # Case 5: parser test that calls parseWgDumpOutput - should pass
        {
            "name": "parser test should pass",
            "content": '''
// Parser regression test
test "parseWgDumpOutput: interface name from parameter" {
    const result = try parseWgDumpOutput(dump, "wg0");
    try std.testing.expectEqualStrings("wg0", result.interface);
}
''',
            "should_fail": False,
        },
        # Case 6: test with loops - should pass
        {
            "name": "test with while loop should pass",
            "content": '''
// Memory leak regression
test "repeated calls do not leak" {
    var i: usize = 0;
    while (i < 100) : (i += 1) {
        const result = try getStatus();
        try std.testing.expect(result.peer_count == 1);
    }
}
''',
            "should_fail": False,
        },
        # Case 7: multiple test blocks in one file
        {
            "name": "multiple test blocks should be detected correctly",
            "content": '''
// First test is contract-like with trivial body
test "RSS leak regression" {
    try std.testing.expect(true);
}

// Second test is contract-like but has real content
test "memory ownership contract" {
    const allocator = std.testing.allocator;
    _ = try allocator.alloc(u8, 1024);
    try std.testing.expect(true);
}
''',
            "should_fail": True,  # First test should fail
        },
        # Case 8: return; only - should fail if contract-like
        {
            "name": "return-only body should fail if contract-like",
            "content": '''
// ownership contract test
test "ownership contract" {
    return;
}
''',
            "should_fail": True,
        },
    ]
    
    passed = 0
    failed = 0
    
    for tc in test_cases:
        with tempfile.NamedTemporaryFile(mode='w', suffix='.zig', delete=False) as f:
            f.write(tc["content"])
            f.flush()
            
            test_path = Path(f.name)
            errors = check_file(test_path)
            
            has_errors = len(errors) > 0
            should_fail = tc["should_fail"]
            
            if has_errors == should_fail:
                print(f"  PASS: {tc['name']}")
                passed += 1
            else:
                print(f"  FAIL: {tc['name']}")
                print(f"    expected: {'fail' if should_fail else 'pass'}")
                print(f"    actual: {'fail' if has_errors else 'pass'}")
                if errors:
                    print(f"    errors: {errors}")
                failed += 1
            
            os.unlink(f.name)
    
    print(f"\nSelf-test results: {passed} passed, {failed} failed")
    
    if failed > 0:
        print("SELF-TEST: FAIL")
        sys.exit(1)
    else:
        print("SELF-TEST: PASS")


if __name__ == "__main__":
    sys.exit(main())
