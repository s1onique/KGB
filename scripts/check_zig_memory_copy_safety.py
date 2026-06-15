#!/usr/bin/env python3
# check_zig_memory_copy_safety.py — Memory copy safety hygiene gate for Zig
#
# Scans Zig source files for @memcpy usage that can cause aliasing panics.
#
# RULES:
#   - Raw @memcpy requires nearby MemoryCopySafety: annotation
#   - Same-buffer @memcpy is always forbidden (even with annotation)
#   - Use std.mem.copyForwards/copyBackwards for overlapping copies
#   - Prefer copyForwards for recv-buffer compaction (dst before src)
#
# ALLOWED PATTERNS:
#   - std.mem.copyForwards or std.mem.copyBackwards (safe for overlap)
#   - @memcpy with MemoryCopySafety: annotation (independent buffers only)
#   - for-loops for small fixed-size structs
#
# FORBIDDEN PATTERNS:
#   - Any raw @memcpy without MemoryCopySafety annotation
#   - Same-buffer @memcpy (dst and src share the same backing array)
#
# ANNOTATION RULES:
#   - Must contain "MemoryCopySafety:" prefix
#   - Must explain WHY source and destination cannot overlap
#   - Same-buffer @memcpy always fails, annotation or not

import sys
import os
import re
import argparse
from pathlib import Path
from typing import Optional

# Color codes
RED = '\033[0;31m'
GREEN = '\033[0;32m'
YELLOW = '\033[0;33m'
NC = '\033[0m'

# Annotation that allows intentional @memcpy usage
ALLOWED_ANNOTATION = "MemoryCopySafety:"

# Scan paths - focus on protocol/runtime paths where @memcpy hygiene matters most
# Exclude test files and fixtures by default (they have their own rules)
# This gate targets new code additions to prevent aliasing bugs in production paths
SCAN_PATHS = [
    "tovarisch/src/bgp",
    "tovarisch/src/bfd",
    "tovarisch/src/runtime",
    "tovarisch/src/http",
]


def find_matching_paren(content: str, start: int) -> int:
    """Find the matching closing parenthesis for an opening paren at start."""
    depth = 1
    i = start + 1
    while i < len(content) and depth > 0:
        if content[i] == '(':
            depth += 1
        elif content[i] == ')':
            depth -= 1
        i += 1
    return i - 1


def extract_base_identifier(expr: str) -> Optional[str]:
    """
    Extract the base identifier from a slice/array expression.
    E.g., "buf[0..10]" -> "buf"
         "sess.recv_buf[0..n]" -> "sess.recv_buf"
         "self.buf[self.len..]" -> "self.buf"
    """
    expr = expr.strip()
    
    # Remove subscript patterns like [x..y] or [x]
    # We want the identifier before any [ bracket
    bracket_pos = expr.find('[')
    if bracket_pos > 0:
        expr = expr[:bracket_pos]
    
    return expr if expr else None


def normalize_base(base: Optional[str]) -> Optional[str]:
    """Normalize base identifier for comparison."""
    if not base:
        return None
    base = base.strip('.')
    return base


def parse_memcpy_args(call: str) -> tuple[Optional[str], Optional[str]]:
    """Parse @memcpy(dst, src) call and extract base identifiers."""
    paren_start = call.find('(')
    if paren_start == -1:
        return None, None
    
    paren_end = find_matching_paren(call, paren_start)
    args_str = call[paren_start+1:paren_end]
    
    # Split on top-level comma only
    args = []
    depth = 0
    current = []
    for char in args_str:
        if char == '(':
            depth += 1
            current.append(char)
        elif char == ')':
            depth -= 1
            current.append(char)
        elif char == ',' and depth == 0:
            args.append(''.join(current).strip())
            current = []
        else:
            current.append(char)
    if current:
        args.append(''.join(current).strip())
    
    if len(args) < 2:
        return None, None
    
    dst = args[0]
    src = args[1]
    
    dst_base = extract_base_identifier(dst)
    src_base = extract_base_identifier(src)
    
    return dst_base, src_base


def check_same_buffer(dst_base: Optional[str], src_base: Optional[str]) -> bool:
    """Check if dst and src share the same base identifier."""
    if not dst_base or not src_base:
        return False
    
    dst_norm = normalize_base(dst_base)
    src_norm = normalize_base(src_base)
    
    if not dst_norm or not src_norm:
        return False
    
    return dst_norm == src_norm


def has_annotation(content: str, pos: int, annotation: str) -> bool:
    """Check if annotation exists within 5 lines before the given position."""
    lines = content.split('\n')
    
    # Find line number
    line_num = content[:pos].count('\n')
    
    # Check 5 lines before
    start = max(0, line_num - 5)
    for i in range(start, line_num + 1):
        if annotation in lines[i]:
            return True
    
    return False


def scan_file(filepath: str, self_test_mode: bool = False) -> tuple[int, int, list]:
    """Scan a Zig file for @memcpy usage."""
    try:
        with open(filepath, 'r') as f:
            content = f.read()
    except Exception as e:
        print(f"  {RED}[ERROR]{NC} Could not read {filepath}: {e}")
        return 1, 0, []
    
    # In normal mode: skip fixtures and test files
    # In self-test mode: only scan fixtures
    if not self_test_mode:
        if '/fixtures/' in filepath:
            print(f"  [SKIP] {filepath} (fixture)")
            return 0, 0, []
        if filepath.endswith('_tests.zig'):
            print(f"  [SKIP] {filepath} (test file)")
            return 0, 0, []
    else:
        if '/fixtures/' not in filepath:
            print(f"  [SKIP] {filepath} (not a fixture)")
            return 0, 0, []
    
    # Find all @memcpy calls (scan original content directly)
    memcpy_pattern = re.compile(r'@memcpy\s*\(')
    findings = []
    failures = 0
    total_memcpy = 0
    
    for match in memcpy_pattern.finditer(content):
        total_memcpy += 1
        
        # Find the full call (parse balanced parens)
        call_start = match.start()
        paren_start = content.find('(', call_start)
        call_end = find_matching_paren(content, paren_start)
        full_call = content[call_start:call_end+1]
        
        # Get line number
        line_num = content[:match.start()].count('\n') + 1
        
        # Parse arguments to check for same-buffer
        dst_base, src_base = parse_memcpy_args(full_call)
        same_buffer = check_same_buffer(dst_base, src_base)
        
        # Check for annotation
        annotated = has_annotation(content, match.start(), ALLOWED_ANNOTATION)
        
        if same_buffer:
            findings.append({
                'file': filepath,
                'line': line_num,
                'type': 'SAME_BUFFER',
                'dst': dst_base,
                'src': src_base,
                'annotated': annotated,
                'call': full_call[:80] + '...' if len(full_call) > 80 else full_call
            })
            failures += 1
        elif not annotated:
            findings.append({
                'file': filepath,
                'line': line_num,
                'type': 'NO_ANNOTATION',
                'dst': dst_base,
                'src': src_base,
                'annotated': False,
                'call': full_call[:80] + '...' if len(full_call) > 80 else full_call
            })
            failures += 1
    
    return failures, total_memcpy, findings


def print_finding(finding: dict, verbose: bool = False):
    """Print a single finding with appropriate styling."""
    if finding['type'] == 'SAME_BUFFER':
        print(f"  {RED}[FAIL]{NC} {finding['file']}:{finding['line']} — same-buffer @memcpy")
        print(f"         dst={finding['dst']}, src={finding['src']}")
        print(f"         Same-buffer copy is always forbidden.")
    elif finding['type'] == 'NO_ANNOTATION':
        print(f"  {RED}[FAIL]{NC} {finding['file']}:{finding['line']} — @memcpy without annotation")
        print(f"         dst={finding['dst']}, src={finding['src']}")
        print(f"         Add MemoryCopySafety: annotation or use copyForwards/copyBackwards.")
    
    if verbose:
        print(f"         Call: {finding['call']}")


def find_zig_files(paths: list) -> list:
    """Find all .zig files in given paths."""
    zig_files = []
    for path in paths:
        if os.path.isfile(path):
            if path.endswith('.zig'):
                zig_files.append(path)
        elif os.path.isdir(path):
            for root, dirs, files in os.walk(path):
                for f in files:
                    if f.endswith('.zig'):
                        zig_files.append(os.path.join(root, f))
    return sorted(zig_files)


def run_self_test() -> int:
    """Run self-test on fixture files."""
    print("=== Memory Copy Safety Self-Test ===")
    print("Testing sentinel fixtures...")
    print("")
    
    fixtures_dir = "tovarisch/fixtures"
    results = {'passed': 0, 'failed': 0}
    
    # Test bad fixture - should FAIL
    bad_fixture = os.path.join(fixtures_dir, "bad-memory-copy-pattern.zig")
    if os.path.exists(bad_fixture):
        print("Testing bad fixture (should FAIL):")
        failures, total, findings = scan_file(bad_fixture, self_test_mode=True)
        if failures > 0:
            print(f"  {GREEN}[PASS]{NC} bad-memory-copy-pattern.zig correctly failed ({failures} issues)")
            results['passed'] += 1
        else:
            print(f"  {RED}[EXPECTED FAIL]{NC} bad-memory-copy-pattern.zig passed but should fail")
            results['failed'] += 1
        print("")
    
    # Test good fixture - should PASS
    good_fixture = os.path.join(fixtures_dir, "good-memory-copy-pattern.zig")
    if os.path.exists(good_fixture):
        print("Testing good fixture (should PASS):")
        failures, total, findings = scan_file(good_fixture, self_test_mode=True)
        if failures == 0:
            print(f"  {GREEN}[PASS]{NC} good-memory-copy-pattern.zig correctly passed")
            results['passed'] += 1
        else:
            print(f"  {RED}[FAIL]{NC} good-memory-copy-pattern.zig failed but should pass")
            for f in findings:
                print_finding(f)
            results['failed'] += 1
        print("")
    
    print(f"Self-test results: {results['passed']} passed, {results['failed']} failed")
    
    if results['failed'] > 0:
        print(f"{RED}[SELF-TEST FAIL]{NC} Sentinel fixture self-test failed")
        return 1
    
    print(f"{GREEN}[SELF-TEST PASS]{NC} All sentinel fixtures tested correctly")
    return 0


def main():
    parser = argparse.ArgumentParser(
        description='Memory copy safety hygiene gate for Zig @memcpy usage'
    )
    parser.add_argument('--self-test', action='store_true',
                        help='Run self-test on fixtures instead of scanning')
    parser.add_argument('--verbose', '-v', action='store_true',
                        help='Show full call context for failures')
    args = parser.parse_args()
    
    if args.self_test:
        sys.exit(run_self_test())
    
    print("=== Memory Copy Safety Hygiene Gate ===")
    print("")
    print("Scanning targeted protocol/runtime paths: bgp, bfd, runtime, http...")
    print("")
    
    zig_files = find_zig_files(SCAN_PATHS)
    total_files = 0
    total_memcpy = 0
    total_failures = 0
    
    for filepath in zig_files:
        failures, memcpy_count, findings = scan_file(filepath)
        total_files += 1
        total_memcpy += memcpy_count
        total_failures += failures
        
        if failures > 0:
            for f in findings:
                print_finding(f, verbose=args.verbose)
        else:
            print(f"  {GREEN}[OK]{NC} {filepath}")
    
    print("")
    print(f"Scanned {total_files} files, found {total_memcpy} @memcpy calls.")
    
    if total_failures > 0:
        print("")
        print(f"{RED}[FAIL]{NC} Memory copy safety hygiene gate failed.")
        print("")
        print("To fix:")
        print("  1. For recv-buffer compaction: use std.mem.copyForwards")
        print("  2. For same-buffer shifts: use copyForwards (src >= dst) or copyBackwards")
        print("  3. For independent buffers: add MemoryCopySafety: annotation")
        print("")
        print("See: docs/tooling/zig-memory-copy-safety.md")
        sys.exit(1)
    
    print("")
    print(f"{GREEN}[PASS]{NC} Memory copy safety hygiene gate passed.")
    sys.exit(0)


if __name__ == '__main__':
    main()
