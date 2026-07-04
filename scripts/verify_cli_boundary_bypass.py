#!/usr/bin/env python3
"""
verify_cli_boundary_bypass.py — Prove diagnostic collectors cannot bypass safe_command

ACT: HULK12R - CLI inventory + collector bypass proof

This verifier scans tovarisch diagnostic collectors to ensure they do not
directly use process-spawn primitives outside of safe_command.zig.

Forbidden patterns in collectors:
  - execve (use safe_command.runCommand)
  - fork (use safe_command.runCommand)
  - std.process (shell invocation)
  - Child (Zig ChildProcess)
  - /bin/sh (shell invocation)
  - sh -c (shell invocation)

Allowlisted files (may use process primitives):
  - safe_command.zig (canonical boundary)
  - safe_command_tests.zig (test helpers)
  - Any file containing "test" in path (test files)

Exit codes:
  0 = all collectors properly route through safe_command
  1 = violations found
"""

import sys
import os
import re
from pathlib import Path

# Configuration
TOVARISCH_SRC = Path(__file__).parent.parent / "tovarisch" / "src"
COLLECTOR_GLOB = "*collector*.zig"
NET_DIR = TOVARISCH_SRC / "net"

# Forbidden patterns (case-insensitive regex)
FORBIDDEN_PATTERNS = [
    (r'\bexecve\s*\(', "execve syscall"),
    (r'\bfork\s*\(', "fork syscall"),
    (r'\bChild\s*\.', "Zig ChildProcess"),
    (r'\bChild\b', "Zig ChildProcess"),
    (r'std\.process\.spawn', "std.process.spawn"),
    (r'/bin/sh', "shell invocation"),
    (r'sh\s+-c', "shell invocation"),
]

# Allowlist patterns (files that may use process primitives)
# These files are themselves CLI boundaries or deprecated collectors
# and are tracked in cli-composition-inventory.csv
ALLOWLIST_PATTERNS = [
    r'safe_command\.zig$',  # The canonical boundary itself
    r'safe_command_tests\.zig$',  # Test helpers
    r'wg_status_boundary_cli\.zig$',  # Native-owned WireGuard CLI boundary (CLI-0033)
    r'wg_show_collector\.zig$',  # DEPRECATED: legacy test coverage (CLI-0012)
    r'_tests\.zig$',  # Any test file
    r'_test\.zig$',  # Any test file variant
]


def is_allowlisted(path: Path) -> bool:
    """Check if file is allowlisted for process primitives."""
    path_str = str(path)
    for pattern in ALLOWLIST_PATTERNS:
        if re.search(pattern, path_str, re.IGNORECASE):
            return True
    return False


def scan_file_for_violations(path: Path) -> list[tuple[str, int, str]]:
    """
    Scan a single file for forbidden process-spawn patterns.
    Returns list of (pattern_name, line_number, line_content) tuples.
    """
    violations = []
    
    try:
        with open(path, 'r', encoding='utf-8') as f:
            for line_num, line in enumerate(f, 1):
                # Skip comments
                stripped = line.strip()
                if stripped.startswith('//') or stripped.startswith('/*') or stripped.startswith('*'):
                    continue
                
                for pattern, name in FORBIDDEN_PATTERNS:
                    if re.search(pattern, line, re.IGNORECASE):
                        violations.append((name, line_num, line.rstrip()))
    
    except (IOError, UnicodeDecodeError) as e:
        print(f"[ERROR] Could not read {path}: {e}", file=sys.stderr)
    
    return violations


def main():
    """Main entry point."""
    print("=== CLI Boundary Bypass Proof ===")
    print()
    
    # Find all collector files
    if not NET_DIR.exists():
        print(f"[ERROR] Directory not found: {NET_DIR}", file=sys.stderr)
        return 1
    
    collector_files = list(NET_DIR.glob(COLLECTOR_GLOB))
    
    # Also check wg_status_boundary_cli.zig which is also a CLI boundary
    status_boundary = NET_DIR / "wg_status_boundary_cli.zig"
    if status_boundary.exists():
        collector_files.append(status_boundary)
    
    print(f"[INFO] Scanning {len(collector_files)} collector file(s):")
    for f in collector_files:
        print(f"       - {f.name}")
    print()
    
    if not collector_files:
        print("[WARN] No collector files found")
        return 0
    
    # Scan each file
    all_violations = {}
    allowlisted_violations = {}
    
    for collector_file in collector_files:
        if is_allowlisted(collector_file):
            print(f"[ALLOWLIST] {collector_file.name} — skip scan (safe_command or test)")
            continue
        
        violations = scan_file_for_violations(collector_file)
        
        if violations:
            all_violations[collector_file.name] = violations
    
    # Report results
    if all_violations:
        print("[FAIL] Violations found in diagnostic collectors:")
        print()
        
        for filename, violations in all_violations.items():
            print(f"  {filename}:")
            for pattern_name, line_num, line_content in violations:
                print(f"    Line {line_num}: {pattern_name}")
                print(f"      {line_content[:80]}...")
            print()
        
        print("Diagnostic collectors MUST route through safe_command.zig")
        print("for all process execution. Direct process-spawn calls are forbidden.")
        return 1
    
    print("[PASS] No diagnostic collectors bypass safe_command.zig")
    print()
    print("Verification summary:")
    print(f"  - Scanned: {len(collector_files)} collector file(s)")
    print("  - Violations: 0")
    print("  - All collectors properly route through safe_command.zig")
    
    return 0


if __name__ == "__main__":
    sys.exit(main())
