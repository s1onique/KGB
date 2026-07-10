"""
Main entry point for UVB-76 Artifact Secret Hygiene verifier.

Provides the main verification scan that:
- Validates artifact inventory
- Scans tracked files for prohibited secret patterns
- Reports findings without exposing secret values
"""

import glob
import os
import subprocess
import sys

from .inventory import ARTIFACT_INVENTORY, validate_inventory
from .scanner import (
    SecretFinding,
    get_candidate_files,
    scan_file_for_secrets,
    format_finding,
    MAX_DIAGNOSTICS,
    MAX_FILES_SCANNED,
)
from .tests.self_test import run_self_tests

# Paths - use git to discover repository root
def discover_repo_root() -> str:
    """Discover repository root using git."""
    try:
        result = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            capture_output=True,
            text=True,
            timeout=10,
        )
        if result.returncode == 0:
            root = result.stdout.strip()
            # Verify it contains expected repo anchors
            if os.path.exists(os.path.join(root, '.git')) or os.path.exists(os.path.join(root, 'tovarisch')):
                return root
    except (subprocess.TimeoutExpired, FileNotFoundError, Exception):
        pass
    # Fallback: traverse up until we find .git or known repo marker
    current = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    while current != '/':
        if os.path.exists(os.path.join(current, '.git')):
            return current
        parent = os.path.dirname(current)
        if parent == current:
            break
        current = parent
    return current

REPO_ROOT = discover_repo_root()


def run_verifier() -> list[str]:
    """Run the main verification scan."""
    all_errors = []

    print("=== UVB-76 Artifact Secret Hygiene Verifier (HULK05) ===\n")

    # A. Validate inventory
    print("A. Validating artifact inventory...")
    inv_errors = validate_inventory()
    if inv_errors:
        for e in inv_errors:
            print(f"  ERROR: {e}")
            all_errors.append(f"Inventory validation failed: {e}")
    else:
        print(f"  OK: Inventory valid ({len(ARTIFACT_INVENTORY)} surfaces)")

    # B. Get candidate files (tracked + untracked)
    print("\nB. Getting candidate files...")
    candidate_files = get_candidate_files(REPO_ROOT)

    if not candidate_files:
        all_errors.append("FATAL: No files found in repository - git may be unavailable")
        return all_errors

    print(f"  Found {len(candidate_files)} candidate files")

    if len(candidate_files) > MAX_FILES_SCANNED:
        all_errors.append(f"Too many files to scan: {len(candidate_files)} > {MAX_FILES_SCANNED}")
        return all_errors

    # C. Scan files
    print("\nC. Scanning for prohibited secrets...")
    all_findings: list[SecretFinding] = []
    scanned = 0
    artifact_matches = 0

    for path in candidate_files:
        # Skip common non-artifact directories (but NOT the hygiene package itself)
        # The hygiene package files must be scanned to prove self-consistency
        skip_dirs = {'.git', 'zig-cache', 'zig-out', '.zig-cache', 'coverage', 'kcov-output', 'node_modules'}
        parts = path.split(os.sep)
        if any(d in skip_dirs for d in parts):
            continue

        # Determine if this is an artifact surface
        rel_path = os.path.relpath(path, REPO_ROOT)
        matched_surface = None
        for surf in ARTIFACT_INVENTORY:
            if surf.path in rel_path or glob.fnmatch.fnmatch(rel_path, surf.path.replace('**/*', '*')):
                matched_surface = surf
                break

        is_artifact_surface = matched_surface is not None

        # Note: RuleSet.NONE surfaces are no longer in inventory.
        # Universal rules now cover all surfaces (patterns built from fragments).

        if is_artifact_surface:
            artifact_matches += 1

        findings = scan_file_for_secrets(path, artifact_surface=is_artifact_surface)
        all_findings.extend(findings)
        scanned += 1

        if len(all_findings) >= MAX_DIAGNOSTICS:
            print(f"  Reached maximum diagnostics limit ({MAX_DIAGNOSTICS})")
            break

    print(f"  Scanned {scanned} files")
    print(f"  Matched {artifact_matches} artifact surface files")

    # D. Report findings
    if all_findings:
        print(f"\n  Found {len(all_findings)} prohibited secret(s):")
        for finding in all_findings[:MAX_DIAGNOSTICS]:
            print(f"    {format_finding(finding, REPO_ROOT)}")
            print(f"      Remediation: {finding.remediation}")
        all_errors.extend([format_finding(f, REPO_ROOT) for f in all_findings])
    else:
        print("  OK: No prohibited secrets detected")

    return all_errors


def main():
    """Main entry point."""
    if "--self-test" in sys.argv:
        print("=== Self-Test Mode ===\n")
        errors, results, total, passed = run_self_tests()

        print(f"\nSELF-TEST SUMMARY: {passed}/{total} passed\n")
        for name, test_passed in results.items():
            status = "PASS" if test_passed else "FAIL"
            print(f"  {name}: {status}")

        if errors:
            print("\nSELF-TEST ERRORS:")
            for e in errors:
                print(f"  - {e}")
            sys.exit(1)
        else:
            print("\nAll self-tests passed!")
            sys.exit(0)

    errors = run_verifier()

    print("\n" + "=" * 60)
    if errors:
        print("\nVERIFICATION FAILED:")
        print(f"  {len(errors)} error(s) found")
        sys.exit(1)
    else:
        print("\nVERIFICATION PASSED")
        sys.exit(0)


if __name__ == "__main__":
    main()
