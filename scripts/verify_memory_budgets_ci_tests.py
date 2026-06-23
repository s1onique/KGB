#!/usr/bin/env python3
"""
Self-tests for CI baseline validation.

This module contains only the self-test logic for CI baseline validation.
The core validation logic lives in verify_memory_budgets_ci.py.
"""

import os
import json
import tempfile
import sys


def run_ci_baseline_self_tests() -> bool:
    """Run self-tests for CI baseline validation. Returns True if all pass."""
    # Import here to avoid circular dependency and ensure we're testing the actual module
    from verify_memory_budgets_ci import check_ci_baseline_evidence_exists
    
    print("\n=== CI Baseline Self-Tests ===\n")
    
    # Create valid artifact data for testing
    valid_artifact = {
        "schema_version": "1.0",
        "evidence_kind": "real_evidence",
        "service": {
            "name": "tovarisch",
            "version": "0.1.0",
            "commit": "abc123"
        },
        "environment": {
            "arch": "linux/amd64",
            "environment_label": "github_hosted_ubuntu",
            "_github_workflow_run": 28013478727,
            "_github_artifact_id": 7815736588,
            "_github_artifact_name": "tovarisch-memory-lab"
        },
        "workload": {
            "type": "idle-warmup",
            "operations": 0,
            "errors": 0,
            "duration_ms": 60000
        },
        "memory": {
            "first": {"rss_kib": 8000},
            "max": {"rss_kib": 8200},
            "last": {"rss_kib": 8100},
            "growth": {"rss_kib": 100}
        },
        "decision": {"pass": True, "reason": "test"}
    }
    
    tests_passed = 0
    tests_failed = 0
    
    with tempfile.TemporaryDirectory() as tmpdir:
        # Create proper artifact directory structure matching real layout
        # The code looks for: repo_root/artifacts/memory-labs/{service}/
        artifact_dir = os.path.join(tmpdir, "artifacts", "memory-labs", "tovarisch")
        os.makedirs(artifact_dir)
        
        # Create valid budget data
        valid_budget = {
            "version": "1.0",
            "service": "tovarisch",
            "ci_idle_baselines": {
                "github_hosted_ubuntu": {
                    "idle": {"rss_kib": 8200, "pss_kib": 6200},
                    "evidence_sources": [
                        {
                            "workflow_run": 28013478727,
                            "artifact_id": 7815736588,
                            "artifact_name": "tovarisch-memory-lab",
                            "service_version": "0.1.0",
                            "service_commit": "abc123",
                            "workload": "tovarisch-idle-warmup",
                            "duration_seconds": 60,
                            "environment_label": "github_hosted_ubuntu"
                        }
                    ]
                }
            },
            "arch_budgets": {},
            "enforcement": {"gate_level": "fast_local", "fail_on_budget_exceeded": True}
        }
        
        # Test 1: Valid CI baseline with matching real_evidence passes
        artifact_path = os.path.join(artifact_dir, "github-hosted-ubuntu-idle-28013478727.json")
        with open(artifact_path, "w") as f:
            json.dump(valid_artifact, f)
        
        errors = check_ci_baseline_evidence_exists(valid_budget, "test.yaml", tmpdir)
        if len(errors) == 0:
            print("  PASS: Valid CI baseline with matching real_evidence passes")
            tests_passed += 1
        else:
            print(f"  FAIL: Valid CI baseline failed: {errors}")
            tests_failed += 1
        
        # Test 2: Missing artifact fails
        os.remove(artifact_path)
        errors = check_ci_baseline_evidence_exists(valid_budget, "test.yaml", tmpdir)
        if len(errors) > 0 and any("No artifact found" in e for e in errors):
            print("  PASS: Missing artifact correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: Missing artifact should have failed: {errors}")
            tests_failed += 1
        
        # Test 3: Schema fixture referenced as baseline evidence fails
        fixture_artifact = dict(valid_artifact)
        fixture_artifact["evidence_kind"] = "schema_fixture"
        with open(artifact_path, "w") as f:
            json.dump(fixture_artifact, f)
        
        errors = check_ci_baseline_evidence_exists(valid_budget, "test.yaml", tmpdir)
        if len(errors) > 0 and any("real_evidence" in e for e in errors):
            print("  PASS: Schema fixture correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: Schema fixture should have failed: {errors}")
            tests_failed += 1
        
        # Test 4: Wrong environment label fails
        wrong_env_artifact = dict(valid_artifact)
        wrong_env_artifact["environment"] = dict(valid_artifact["environment"])
        wrong_env_artifact["environment"]["environment_label"] = "router_armv7"
        with open(artifact_path, "w") as f:
            json.dump(wrong_env_artifact, f)
        
        errors = check_ci_baseline_evidence_exists(valid_budget, "test.yaml", tmpdir)
        if len(errors) > 0 and any("environment_label" in e.lower() for e in errors):
            print("  PASS: Wrong environment label correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: Wrong environment label should have failed: {errors}")
            tests_failed += 1
        
        # Test 5: Wrong service fails
        wrong_service_artifact = dict(valid_artifact)
        wrong_service_artifact["service"] = {"name": "uvb76", "version": "1.0.0", "commit": "xyz789"}
        with open(artifact_path, "w") as f:
            json.dump(wrong_service_artifact, f)
        
        errors = check_ci_baseline_evidence_exists(valid_budget, "test.yaml", tmpdir)
        if len(errors) > 0 and any("service.name" in e for e in errors):
            print("  PASS: Wrong service correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: Wrong service should have failed: {errors}")
            tests_failed += 1
        
        # Test 6: Invalid JSON fails
        with open(artifact_path, "w") as f:
            f.write("not valid json{{{")
        
        errors = check_ci_baseline_evidence_exists(valid_budget, "test.yaml", tmpdir)
        if len(errors) > 0 and any("not valid JSON" in e for e in errors):
            print("  PASS: Invalid JSON correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: Invalid JSON should have failed: {errors}")
            tests_failed += 1
        
        # Test 7: Mismatched workflow metadata fails
        mismatch_artifact = dict(valid_artifact)
        mismatch_artifact["environment"] = dict(valid_artifact["environment"])
        mismatch_artifact["environment"]["_github_artifact_id"] = 9999999999
        with open(artifact_path, "w") as f:
            json.dump(mismatch_artifact, f)
        
        errors = check_ci_baseline_evidence_exists(valid_budget, "test.yaml", tmpdir)
        if len(errors) > 0 and any("artifact_id" in e for e in errors):
            print("  PASS: Mismatched artifact_id correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: Mismatched artifact_id should have failed: {errors}")
            tests_failed += 1
    
    print(f"\n=== CI Baseline Self-Test Results ===")
    print(f"  Passed: {tests_passed}, Failed: {tests_failed}")
    return tests_failed == 0


if __name__ == "__main__":
    sys.exit(0 if run_ci_baseline_self_tests() else 1)
