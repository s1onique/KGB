#!/usr/bin/env python3
"""
Verifier for UVB-76 memory attribution lab artifacts.

Validates that attribution lab artifacts conform to the expected contract:
- manifest.yaml exists and has required fields
- memstats-{start,midpoint,end}.json exist with valid fields
- heap-{start,midpoint,end}.pprof files exist and are non-empty
- goroutine-{start,midpoint,end}.txt files exist and are non-empty
- rss-pss.tsv exists with proper coverage
- lab-result.json is valid JSON

Supports self-test mode with fixture validation.
"""

import json
import os
import sys

# Import the validation library
import attribution_contract as ac

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def run_verifier(repo_root):
    """Run the attribution artifact verifier."""
    all_errors = []
    print("=== UVB-76 Memory Attribution Artifact Verifier ===\n")
    
    fixture_dirs = [os.path.join(repo_root, "docs", "memory", "fixtures")]
    real_evidence_dirs = [os.path.join(repo_root, "artifacts", "memory-labs", "uvb76", "attribution")]
    
    print("A. Checking attribution fixtures (docs/memory/fixtures/)...")
    validated_fixtures = []
    for fixture_dir in fixture_dirs:
        if not os.path.isdir(fixture_dir):
            continue
        for entry in os.listdir(fixture_dir):
            if "attribution" in entry and entry.endswith(".json"):
                path = os.path.join(fixture_dir, entry)
                print(f"  Checking fixture: {entry}")
                errors = ac.validate_fixture(path)
                if errors:
                    for e in errors:
                        print(f"    ERROR: {e}")
                    all_errors.extend(errors)
                else:
                    validated_fixtures.append(path)
                    print(f"    OK: Valid schema_fixture")
    
    print(f"\nB. Checking real evidence artifacts (artifacts/memory-labs/uvb76/attribution/)...")
    validated_artifacts = []
    for evidence_dir in real_evidence_dirs:
        if not os.path.isdir(evidence_dir):
            print(f"  Directory does not exist yet: {evidence_dir}")
            print(f"  (Run make lab-uvb76-memory-attribution to generate artifacts)")
            continue
        
        for entry in os.listdir(evidence_dir):
            entry_path = os.path.join(evidence_dir, entry)
            if not os.path.isdir(entry_path):
                continue
            if not ac.is_attribution_dir(entry_path):
                continue
            
            print(f"  Validating: {entry}/")
            errors = ac.validate_attribution_dir(entry_path)
            if errors:
                for e in errors:
                    print(f"    ERROR: {e}")
                all_errors.extend(errors)
            else:
                validated_artifacts.append(entry_path)
                print(f"    OK: Valid attribution artifact")
    
    print("\n" + "=" * 50)
    print("SUMMARY:")
    print(f"  Attribution fixtures: {len(validated_fixtures)} valid")
    print(f"  Attribution artifacts: {len(validated_artifacts)} valid")
    if all_errors:
        print(f"  Errors: {len(all_errors)}")
    
    return all_errors


def main():
    if "--self-test" in sys.argv:
        # Import self-test module
        from importlib import import_module
        selftest = import_module("verify_uvb76_memory_attribution_artifacts_selftest")
        errors, results = selftest.run_self_tests()
        print("\n" + "=" * 50)
        print("SELF-TEST SUMMARY:")
        for name, passed in results.items():
            status = "PASS" if passed else "FAIL"
            print(f"  {name}: {status}")
        
        if errors:
            print("\nSELF-TEST ERRORS:")
            for e in errors:
                print(f"  - {e}")
            sys.exit(1)
        else:
            print("\nAll self-tests passed!")
            sys.exit(0)
    
    errors = run_verifier(REPO_ROOT)
    
    print("\n" + "=" * 50)
    if errors:
        print("\nVERIFICATION FAILED:")
        for e in errors:
            print(f"  - {e}")
        sys.exit(1)
    else:
        print("\nVERIFICATION PASSED")
        sys.exit(0)


if __name__ == "__main__":
    main()
