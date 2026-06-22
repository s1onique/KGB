#!/usr/bin/env python3
"""
Frontend Test Hygiene Verifier

Scans scripts, package.json, and CI configs to ensure:
- Quality gate routes through the safe wrapper
- No raw unbounded Vitest usage in gate/CI paths
- Safe commands are documented

Exit codes:
  0 - All checks passed
  1 - Hygiene violations found
  2 - Self-test failed
"""

import argparse
import re
import subprocess
import sys
from pathlib import Path

# Known safe entrypoints
SAFE_WRAPPER = "scripts/run_frontend_tests.sh"
SAFE_SCRIPTS = {"test:run", "test:ci"}  # Safe npm scripts

# Forbidden patterns for gate/CI paths
FORBIDDEN_PATTERNS = [
    (r'npm\s+test\b', "raw 'npm test' in gate path"),
    (r'npm\s+run\s+test\b(?!.*:watch)', "raw 'npm run test' in gate path"),
    (r'npx\s+vitest\b(?!\s+run)', "raw 'vitest' without 'run' mode"),
    (r'vitest\s+--watch', "vitest --watch in gate path"),
    (r'vitest\s+(?!--)', "raw vitest invocation without explicit 'run'"),
]

# Files to scan for hygiene violations (supports glob patterns)
GATE_FILES = [
    "scripts/quality_gate.sh",
]

# Package.json files to scan
PACKAGE_JSON_FILES = [
    "uvb76/web/package.json",
]


def expand_paths(repo_root: Path, patterns: list) -> list:
    """Expand glob patterns and return sorted unique list of existing files."""
    paths: list = []
    for pattern in patterns:
        # Try glob expansion first
        matches = list(repo_root.glob(pattern))
        if matches:
            paths.extend(matches)
        else:
            # Fall back to literal path
            literal = repo_root / pattern
            if literal.exists():
                paths.append(literal)
    return sorted(set(paths))


def scan_file_for_patterns(filepath: Path, patterns: list, context_lines: int = 2) -> list:
    """Scan a file for forbidden patterns, returning matches with context."""
    if not filepath.exists():
        return []

    matches = []
    try:
        content = filepath.read_text()
        lines = content.split('\n')

        for line_num, line in enumerate(lines, 1):
            for pattern, description in patterns:
                if re.search(pattern, line):
                    matches.append({
                        'file': str(filepath),
                        'line': line_num,
                        'content': line.strip(),
                        'description': description,
                        'context': lines[max(0, line_num - context_lines - 1):line_num + context_lines]
                    })
    except Exception as e:
        print(f"[warn] Could not read {filepath}: {e}", file=sys.stderr)

    return matches


def check_gate_file_hygiene(filepath: Path) -> list:
    """Check a gate script for unsafe test invocations."""
    patterns = [
        (r'npm\s+test\b', "raw 'npm test' in gate path"),
        (r'npm\s+run\s+test\b', "raw 'npm run test' in gate path"),
        (r'npx\s+vitest\b(?!\s+run)', "raw 'npx vitest' without 'run' mode"),
        (r'\bvitest\b(?!\s+(run|--))', "raw 'vitest' invocation"),
        (r'(?<!scripts/)run_frontend_tests\.sh', "indirect wrapper reference"),  # warn only
    ]

    # Build pattern that matches safe patterns to exclude
    safe_patterns = [
        r'run_frontend_tests\.sh',
        r'test:run',
        r'test:ci',
        r'npm run test:run',
        r'npm run test:ci',
    ]

    matches = []
    if not filepath.exists():
        return matches

    content = filepath.read_text()
    lines = content.split('\n')

    for line_num, line in enumerate(lines, 1):
        # Skip comments
        stripped = line.strip()
        if stripped.startswith('#'):
            continue

        # Check for forbidden patterns
        for pattern, description in patterns:
            if re.search(pattern, line):
                # Check if line contains safe wrapper usage
                if SAFE_WRAPPER in line:
                    continue

                # Check for safe npm scripts
                is_safe = False
                for safe in safe_patterns:
                    if re.search(safe, line):
                        is_safe = True
                        break

                if not is_safe:
                    matches.append({
                        'file': str(filepath),
                        'line': line_num,
                        'content': line.strip(),
                        'description': description,
                    })

    return matches


def is_safe_package_test_command(cmd: str) -> bool:
    """Check if a package test command is safe (uses wrapper or has bounded settings)."""
    # Only wrapper is considered safe - it enforces bounded workers and timeouts
    return "run_frontend_tests.sh" in cmd


def check_package_json_hygiene(filepath: Path) -> list:
    """Check package.json test scripts for unsafe configurations."""
    matches = []
    if not filepath.exists():
        return matches

    import json
    try:
        pkg = json.loads(filepath.read_text())
    except json.JSONDecodeError as e:
        matches.append({
            'file': str(filepath),
            'line': 1,
            'content': '',
            'description': f"Invalid JSON: {e}",
        })
        return matches

    scripts = pkg.get('scripts', {})

    # Check for unsafe test scripts
    for name, cmd in scripts.items():
        if name.startswith('test'):
            # Watch mode scripts are allowed as explicit developer escape hatch
            if 'watch' in name.lower():
                continue

            # All test scripts must use the wrapper for safety
            if not is_safe_package_test_command(cmd):
                matches.append({
                    'file': str(filepath),
                    'line': 1,
                    'content': f'  "{name}": "{cmd}"',
                    'description': f"Unsafe test script '{name}' - must use run_frontend_tests.sh",
                })

    return matches


def check_workflow_hygiene(workflow_dir: Path) -> list:
    """Check GitHub workflows for unsafe test patterns."""
    matches = []
    if not workflow_dir.exists():
        return matches

    for workflow in list(workflow_dir.glob('**/*.yml')) + list(workflow_dir.glob('**/*.yaml')):
        content = workflow.read_text()

        # Look for test steps
        in_test_step = False
        lines = content.split('\n')

        for line_num, line in enumerate(lines, 1):
            if 'test' in line.lower() and ('run' in line or 'npm' in line):
                stripped = line.strip()

                # Skip if using safe wrapper
                if SAFE_WRAPPER in stripped:
                    continue

                # Skip comments
                if stripped.startswith('#'):
                    continue

                # Check for unsafe patterns
                if re.search(r'npm\s+test\b', stripped):
                    matches.append({
                        'file': str(workflow),
                        'line': line_num,
                        'content': stripped,
                        'description': "raw 'npm test' in workflow",
                    })

                if re.search(r'npx\s+vitest\b(?!\s+run)', stripped):
                    matches.append({
                        'file': str(workflow),
                        'line': line_num,
                        'content': stripped,
                        'description': "raw 'npx vitest' without 'run' in workflow",
                    })

    return matches


def run_self_tests() -> bool:
    """Run self-test suite to verify the verifier works correctly."""
    print("[self-test] Running hygiene verifier self-tests...")

    failures = []

    # Test 1: Safe wrapper in gate file should pass
    safe_gate_content = """#!/bin/bash
./scripts/run_frontend_tests.sh
"""
    safe_gate = Path("/tmp/test_safe_gate.sh")
    safe_gate.write_text(safe_gate_content)
    matches = check_gate_file_hygiene(safe_gate)
    if matches:
        failures.append("Test 1 failed: Safe gate file incorrectly flagged")

    # Test 2: Raw npm test should be flagged
    unsafe_gate_content = """#!/bin/bash
npm test
"""
    unsafe_gate = Path("/tmp/test_unsafe_gate.sh")
    unsafe_gate.write_text(unsafe_gate_content)
    matches = check_gate_file_hygiene(unsafe_gate)
    if not matches:
        failures.append("Test 2 failed: Raw 'npm test' not detected")
    elif 'npm test' not in matches[0]['description']:
        failures.append("Test 2 failed: Wrong pattern detected")

    # Test 3: Raw vitest should be flagged
    unsafe_vitest_content = """#!/bin/bash
npx vitest
"""
    unsafe_vitest = Path("/tmp/test_unsafe_vitest.sh")
    unsafe_vitest.write_text(unsafe_vitest_content)
    matches = check_gate_file_hygiene(unsafe_vitest)
    if not matches:
        failures.append("Test 3 failed: Raw 'npx vitest' not detected")

    # Test 4: vitest run should pass
    safe_vitest_content = """#!/bin/bash
npx vitest run
"""
    safe_vitest = Path("/tmp/test_safe_vitest.sh")
    safe_vitest.write_text(safe_vitest_content)
    matches = check_gate_file_hygiene(safe_vitest)
    if matches:
        failures.append("Test 4 failed: 'vitest run' incorrectly flagged")

    # Test 5: Package.json check
    import json
    safe_pkg = {
        "scripts": {
            "test:run": "vitest run",
            "test": "vitest",  # Unsafe default
        }
    }
    safe_pkg_file = Path("/tmp/test_package.json")
    safe_pkg_file.write_text(json.dumps(safe_pkg))
    matches = check_package_json_hygiene(safe_pkg_file)
    if not matches:
        failures.append("Test 5 failed: Unsafe 'test' script not detected in package.json")

    # Cleanup
    for f in [safe_gate, unsafe_gate, unsafe_vitest, safe_vitest, safe_pkg_file]:
        f.unlink(missing_ok=True)

    if failures:
        print("[self-test] FAILURES:")
        for f in failures:
            print(f"  - {f}")
        return False

    print("[self-test] All self-tests passed!")
    return True


def main():
    parser = argparse.ArgumentParser(
        description="Verify frontend test hygiene - no unbounded Vitest in gate paths"
    )
    parser.add_argument(
        '--self-test',
        action='store_true',
        help='Run self-tests and exit'
    )
    parser.add_argument(
        '--verbose', '-v',
        action='store_true',
        help='Show detailed output'
    )
    args = parser.parse_args()

    if args.self_test:
        success = run_self_tests()
        sys.exit(0 if success else 2)

    repo_root = Path(__file__).parent.parent
    all_matches = []

    print("[hygiene] Scanning for unsafe frontend test entrypoints...")

    # Check gate files (expand glob patterns)
    for filepath in expand_paths(repo_root, GATE_FILES):
        matches = check_gate_file_hygiene(filepath)
        all_matches.extend(matches)

    # Check package.json files
    for pkg_file in PACKAGE_JSON_FILES:
        filepath = repo_root / pkg_file
        matches = check_package_json_hygiene(filepath)
        all_matches.extend(matches)

    # Check workflows
    workflow_dir = repo_root / ".github" / "workflows"
    matches = check_workflow_hygiene(workflow_dir)
    all_matches.extend(matches)

    if all_matches:
        print(f"[hygiene] Found {len(all_matches)} hygiene violation(s):")
        for m in all_matches:
            print(f"  {m['file']}:{m['line']}: {m['description']}")
            if args.verbose and 'content' in m:
                print(f"    {m['content']}")
        print("\n[hygiene] FIX: Use scripts/run_frontend_tests.sh for gate/CI paths")
        print("[hygiene] Allowlist: npm scripts 'test:run', 'test:ci' are safe")
        sys.exit(1)

    print("[hygiene] All checks passed - no unbounded Vitest in gate paths")
    sys.exit(0)


if __name__ == "__main__":
    main()
