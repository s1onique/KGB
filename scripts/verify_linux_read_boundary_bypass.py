#!/usr/bin/env python3
"""
verify_linux_read_boundary_bypass.py — Verify no direct sysfs/procfs reads outside linux_read.zig

ACT-TOVARISCH-ZIG-HULK16: Migrate legacy Linux readers to canonical linux_read boundary

This script verifies that all Linux sysfs/procfs-style diagnostic reads
go through the canonical linux_read.zig boundary helper.

Acceptable direct file access patterns:
1. linux_read.zig itself (the boundary helper)
2. Test files that create fixtures in /tmp
3. WireGuard key files in /etc/wireguard/ (explicit allowlist)
4. Config files (explicit allowlist: /etc/kgb/, /var/lib/kgb/)
5. Lab events files (explicit allowlist: /tmp/kgb_lab_*)

Forbidden patterns (potential bypasses):
- std.c.open/read on /sys/* without going through linux_read
- std.c.open/read on /proc/* without going through linux_read
- Direct openForRead calls bypassing the boundary
- Raw sysfs paths in diagnostic collectors

Usage:
    python scripts/verify_linux_read_boundary_bypass.py [--fix]
"""

import argparse
import re
import sys
from pathlib import Path
from typing import NamedTuple, List, Set


class Violation(NamedTuple):
    file: str
    line: int
    pattern: str
    context: str


# Patterns that indicate direct sysfs/procfs access without the boundary
# These patterns match IN THE BODY OF DIAGNOSTIC CODE, not in:
# - linux_read.zig (the boundary helper)
# - _tests.zig files (test fixtures)
# - Files with /tmp/kgb_ (test fixtures)
DIRECT_ACCESS_PATTERNS = [
    # Direct std.c.open on sysfs/procfs paths in non-test code
    (r'std\.c\.open\s*\(\s*["\']/sys/', 'direct std.c.open on /sys'),
    (r'std\.c\.open\s*\(\s*["\']/proc/', 'direct std.c.open on /proc'),
    (r'std\.c\.open\s*\(\s*["\']/run/', 'direct std.c.open on /run'),
    # Direct read on sysfs/procfs file descriptors
    (r'std\.c\.read\s*\([^)]*fd[^)]*\).*[/"\'](sys|proc)', 'direct std.c.read with sysfs/procfs'),
    # Direct openForRead calls in diagnostic code (non-test)
    (r'openForRead\s*\([^)]*["\'](/sys|/proc)', 'openForRead with sysfs/procfs path'),
    # Pattern that looks like inline sysfs path construction
    # BUT NOT in linux_read.zig or test files
    (r'(sysfs_root|sysfs).*["\']/sys/class/net', 'sysfs path construction'),
]

# Path-level allowlist patterns (files that are fully exempt)
ALLOWLIST_PATH_PATTERNS = [
    # linux_read.zig itself (the boundary helper defines the paths)
    r'linux_read\.zig',
    # WireGuard keys - explicit allowlist
    r'/etc/wireguard/',
    # KGB config files
    r'/etc/kgb/',
    r'/var/lib/kgb/',
    # Lab event files
    r'/tmp/kgb_lab_',
    # Test fixtures
    r'/tmp/kgb_',
    # WireGuard key generation/reading
    r'wg/generate',
    r'wg/peer',
    r'cli/wg_args',
    # Test files creating fixtures
    r'_tests\.zig',
    r'fixtures/',
    # Linux stats tests
    r'linux_stats_tests\.zig',
    r'linux_interface_stats_tests\.zig',
    # metrics.zig defines constants, not doing direct reads
    r'metrics\.zig',
    # linux_interfaces.zig uses opendir/readdir for enumeration (allowed)
    r'linux_interfaces\.zig',
]

# File+line allowlist patterns (specific exceptions for known harmless content)
# Tuples of (path_pattern, line_pattern)
ALLOWLIST_FILE_LINE_PATTERNS = [
    # tunnel_check.zig: DEFAULT_SYSFS_NET_PATH constant is harmless
    # This allows the constant while still catching any direct /sys or /proc reads
    (
        r'^tovarisch/src/tunnel_check\.zig$',
        r'DEFAULT_SYSFS_NET_PATH\s*=\s*"/sys/class/net"',
    ),
]

# Files that are known to do direct sysfs/procfs access (legacy, should be empty after HULK16)
# All these files have been migrated to use linux_read.zig boundary
LEGACY_FILES: dict = {
    # 'tovarisch/src/runtime/telemetry.zig': 'MIGRATED in HULK16 - now uses linux_read.zig',
    # 'tovarisch/src/net/linux_stats.zig': 'MIGRATED in HULK16 - now uses linux_read.zig',
    # 'tovarisch/src/net/extended_interface_stats.zig': 'MIGRATED in HULK16 - now uses linux_read.zig',
    # 'tovarisch/src/tunnel_check.zig': 'MIGRATED in HULK16 - now uses caller-provided allocator',
}


def is_path_allowlisted(file_path: str) -> bool:
    """Check if a file path matches a path-level allowlist pattern."""
    for pattern in ALLOWLIST_PATH_PATTERNS:
        if re.search(pattern, file_path):
            return True
    return False


def is_file_line_allowlisted(file_path: str, line: str) -> bool:
    """Check if a file+line matches a file+line allowlist pattern."""
    for path_re, line_re in ALLOWLIST_FILE_LINE_PATTERNS:
        if re.search(path_re, file_path) and re.search(line_re, line):
            return True
    return False


def is_allowlisted(file_path: str, line: str) -> bool:
    """Check if a line matches any allowlist pattern."""
    # Check path-level allowlist
    if is_path_allowlisted(file_path):
        return True
    # Check file+line allowlist
    if is_file_line_allowlisted(file_path, line):
        return True
    return False


def is_legacy_file(file_path: str) -> bool:
    """Check if file is a known legacy file."""
    return file_path in LEGACY_FILES


def check_file(file_path: str, content: str) -> List[Violation]:
    """Check a single file for boundary violations."""
    violations = []
    lines = content.split('\n')
    
    for i, line in enumerate(lines, 1):
        # Skip comments
        if re.match(r'^\s*//', line):
            continue
        
        # Skip allowlisted files
        if is_allowlisted(file_path, line):
            continue
        
        # Check for direct access patterns
        for pattern, description in DIRECT_ACCESS_PATTERNS:
            if re.search(pattern, line, re.IGNORECASE):
                violations.append(Violation(
                    file=file_path,
                    line=i,
                    pattern=description,
                    context=line.strip()
                ))
                break
    
    return violations


def main():
    parser = argparse.ArgumentParser(
        description='Verify no direct sysfs/procfs reads outside linux_read boundary'
    )
    parser.add_argument('--fix', action='store_true', 
                        help='Attempt to fix violations (not implemented)')
    parser.add_argument('--verbose', '-v', action='store_true',
                        help='Show detailed output')
    args = parser.parse_args()
    
    tovarisch_src = Path('tovarisch/src')
    violations: List[Violation] = []
    legacy_files_checked = 0
    files_checked = 0
    
    # Check all .zig files
    for zig_file in tovarisch_src.rglob('*.zig'):
        rel_path = str(zig_file.relative_to('.'))
        files_checked += 1
        
        # Track legacy files
        if is_legacy_file(rel_path):
            legacy_files_checked += 1
            if args.verbose:
                print(f"  [LEGACY] {rel_path}")
            continue
        
        try:
            content = zig_file.read_text()
        except Exception as e:
            print(f"Error reading {rel_path}: {e}", file=sys.stderr)
            continue
        
        file_violations = check_file(rel_path, content)
        violations.extend(file_violations)
    
    # Report results
    print("=" * 60)
    print("Linux sysfs/procfs boundary verification")
    print("=" * 60)
    print(f"Checked: {files_checked} .zig files in tovarisch/src")
    print(f"Legacy files documented: {legacy_files_checked}")
    print(f"Violations found: {len(violations)}")
    print()
    
    if violations:
        print("VIOLATIONS:")
        for v in violations:
            print(f"  {v.file}:{v.line}")
            print(f"    Pattern: {v.pattern}")
            print(f"    Code: {v.context[:80]}...")
            print()
        
        print("To fix violations:")
        print("  1. Migrate direct file reads to use linux_read.zig boundary")
        print("  2. Add the file to ALLOWLIST_PATTERNS if it's a legitimate exception")
        print("  3. Add the file to LEGACY_FILES if it's documented legacy code")
        print()
        return 1
    
    # Report on legacy files (should be empty after HULK16)
    if legacy_files_checked > 0:
        print("LEGACY FILES (documented, should migrate to linux_read):")
        for file_path, reason in LEGACY_FILES.items():
            print(f"  - {file_path}")
            print(f"    Reason: {reason}")
        print()
    
    if legacy_files_checked == 0 and len(violations) == 0:
        print("RESULT: PASS - No unauthorized sysfs/procfs reads found")
        print("All legacy files have been migrated to linux_read.zig boundary")
    else:
        print("RESULT: PASS - No unauthorized sysfs/procfs reads found")
    
    print()
    print("Boundary helper: tovarisch/src/net/linux_read.zig")
    print("Canonical API: linuxRead(allocator, path, root, config) -> LinuxReadResult")
    return 0


if __name__ == '__main__':
    sys.exit(main())
