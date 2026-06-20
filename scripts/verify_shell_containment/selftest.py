"""
Self-test fixtures and logic for shell containment verifier.
"""

import os
import tempfile
import shutil

from .rules import check_script
from .loader import load_inventory


# Test fixtures - script content mapped to expected behavior
TEST_FIXTURES = {
    "good_thin_launcher.sh": {
        "content": """#!/bin/sh
# ShellRole: launcher
# Thin launcher that execs a typed binary
exec python3 -m mymodule "$@"
""",
        "inventory": {},
        "expect_pass": True,
        "description": "Thin launcher - no risk tokens",
    },
    "good_annotated.sh": {
        "content": """#!/bin/bash
# ShellJustification: CI glue script that orchestrates typed binaries
# ShellRole: ci-glue
# MigrationPlan: Will be replaced when Go lab harness is ready
set -e
./build.sh
./test.sh
""",
        "inventory": {},
        "expect_pass": True,
        "description": "Properly annotated CI glue script",
    },
    "bad_json_parsing.sh": {
        "content": """#!/bin/bash
# This script has risky JSON parsing without justification
result=$(curl -s http://api.example.com/data)
value=$(echo "$result" | jq '.value')
if [ "$value" = "true" ]; then
    echo "enabled"
fi
""",
        "inventory": {},
        "expect_pass": False,
        "description": "JSON parsing without justification - should fail",
    },
    "bad_polling.sh": {
        "content": """#!/bin/bash
# This script has polling without justification
while true; do
    status=$(curl -s http://api.example.com/status)
    if [ "$status" = "ready" ]; then
        break
    fi
    sleep 5
done
""",
        "inventory": {},
        "expect_pass": False,
        "description": "Polling loop without justification - should fail",
    },
    "bad_release.sh": {
        "content": """#!/bin/bash
# This script has gh release commands without justification
gh release create v1.0.0 ./dist/*.zip
gh release upload v1.0.0 ./artifacts.json
""",
        "inventory": {},
        "expect_pass": False,
        "description": "gh release without justification - should fail",
    },
    "bad_role_only.sh": {
        "content": """#!/bin/bash
# ShellRole: launcher
# This script uses jq without justification
data=$(echo '{"foo": 1}' | jq '.foo')
""",
        "inventory": {},
        "expect_pass": False,
        "description": "Has ShellRole but missing ShellJustification - should fail",
    },
    "grandfathered_example.sh": {
        "content": """#!/bin/bash
# grandfathered: Listed in shell_inventory.csv
data=$(curl -s http://api.example.com/data)
jq '.' <<< "$data"
""",
        "inventory": {
            "grandfathered_example.sh": {
                "disposition": "grandfathered_needs_owner",
                "risk_flags": "jq",
                "owner": "team-platform",
                "notes": "",
            }
        },
        "expect_pass": True,
        "description": "Properly grandfathered with named owner",
    },
}

# Test inventory for inventory-related tests
TEST_INVENTORY = {
    "good_thin_launcher.sh": {
        "disposition": "keep_wrapper",
        "risk_flags": "none",
        "owner": "none",
        "notes": "",
    },
    "good_annotated.sh": {
        "disposition": "keep_wrapper",
        "risk_flags": "none",
        "owner": "none",
        "notes": "",
    },
    "grandfathered_example.sh": {
        "disposition": "grandfathered_needs_owner",
        "risk_flags": "jq",
        "owner": "team-platform",
        "notes": "",
    },
    "test_bootstrap.sh": {
        "disposition": "grandfathered_needs_owner",
        "risk_flags": "jq",
        "owner": "TBD",
        "notes": "Bootstrap inventory",
    },
}


def run_tests() -> bool:
    """
    Run self-test with fixture scripts.
    
    Returns:
        True if all tests passed, False otherwise
    """
    test_dir = tempfile.mkdtemp()
    all_passed = True

    try:
        # Write fixture files
        for name, fixture in TEST_FIXTURES.items():
            path = os.path.join(test_dir, name)
            with open(path, "w") as f:
                f.write(fixture["content"])
            os.chmod(path, 0o755)

        # Run script tests
        for name, fixture in TEST_FIXTURES.items():
            script_path = os.path.join(test_dir, name)
            result = check_script(script_path, fixture["inventory"])
            
            print(f"[test] Testing {name} ({fixture['description']})...")
            if fixture["expect_pass"]:
                if result.passed:
                    print("  PASS")
                else:
                    print(f"  FAIL: {result.violations}")
                    all_passed = False
            else:
                if not result.passed:
                    print("  PASS (correctly rejected)")
                else:
                    print("  FAIL: Should have been rejected")
                    all_passed = False

        # Test CSV loader with comment handling
        print("[test] Testing CSV loader with comments...")
        csv_content = """path,disposition,risk_flags,owner,notes
# This is a comment
scripts/test.sh,grandfathered_needs_owner,jq,team-platform,
"""
        csv_path = os.path.join(test_dir, "test_inventory.csv")
        with open(csv_path, "w") as f:
            f.write(csv_content)
        
        try:
            loaded = load_inventory(csv_path)
            if "scripts/test.sh" in loaded:
                print("  PASS (CSV loader correctly handles comments)")
            else:
                print(f"  FAIL: CSV loader missed entry, got: {loaded}")
                all_passed = False
        except Exception as e:
            print(f"  FAIL: CSV loader raised exception: {e}")
            all_passed = False

        # Test: risky script with full annotations but NOT in inventory should FAIL
        print("[test] Testing risky script with annotations but not in inventory (should fail)...")
        risky_with_annotations = """#!/bin/bash
# ShellJustification: testing
# ShellRole: test
# MigrationPlan: never
jq '.' <<< '{}'
"""
        risky_path = os.path.join(test_dir, "risky_not_in_inventory.sh")
        with open(risky_path, "w") as f:
            f.write(risky_with_annotations)
        os.chmod(risky_path, 0o755)
        result = check_script(risky_path, {})  # empty inventory
        if not result.passed and "must be listed in" in result.violations[0]:
            print("  PASS (correctly rejected - must be in CSV)")
        else:
            print(f"  FAIL: Should reject risky script not in CSV, got: {result.violations}")
            all_passed = False

        # Test: risky script listed as keep_wrapper should FAIL
        print("[test] Testing risky script listed as keep_wrapper (should fail)...")
        bad_inventory = {
            "misclassified.sh": {
                "disposition": "keep_wrapper",
                "risk_flags": "none",
                "owner": "none",
                "notes": "",
            }
        }
        misclassified_path = os.path.join(test_dir, "misclassified.sh")
        with open(misclassified_path, "w") as f:
            f.write(risky_with_annotations)
        os.chmod(misclassified_path, 0o755)
        result = check_script(misclassified_path, bad_inventory)
        if not result.passed and "keep_wrapper" in result.violations[0]:
            print("  PASS (correctly rejected - keep_wrapper mismatch)")
        else:
            print(f"  FAIL: Should reject misclassified script, got: {result.violations}")
            all_passed = False

    finally:
        shutil.rmtree(test_dir)

    return all_passed
