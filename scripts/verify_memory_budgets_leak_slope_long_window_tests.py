#!/usr/bin/env python3
"""
Self-tests for long-window leak-slope CI baseline validation.

This module contains only the self-test logic for long-window validation.
The core validation logic lives in verify_memory_budgets_leak_slope_long_window.py.
"""

import copy
import os
import json
import tempfile
import sys


def run_long_window_self_tests() -> bool:
    """Run self-tests for long-window leak-slope validation. Returns True if all pass."""
    from verify_memory_budgets_leak_slope_long_window import (
        validate_ci_leak_slope_long_window,
        check_long_window_evidence_traceability,
    )
    
    print("\n=== Long-Window Leak-Slope Self-Tests ===\n")
    
    tests_passed = 0
    tests_failed = 0
    
    valid_long_window = {
        "version": "1.0", "service": "uvb76",
        "ci_leak_slope_long_window": {
            "github_hosted_ubuntu": {
                "evidence_status": "long_window_evidence",
                "signal_quality": "long_window",
                "long_window_seconds": 900,
                "uvb76_leak_slope": {"rss_slope_kib_per_min": 360.72, "operations": 9000},
                "evidence_sources": [{
                    "workflow_run": 28024460972, "artifact_id": 7820989961,
                    "artifact_name": "uvb76-memory-lab", "service_version": "unknown",
                    "service_commit": "189937815e2493d4179e6bc9729cde74785699ef",
                    "workload": "uvb76-leak-slope", "duration_seconds": 906.198,
                    "operations": 9000, "request_errors": 0,
                    "environment_label": "github_hosted_ubuntu",
                    "workload_type": "uvb76_leak_slope",
                    "long_window_seconds": 900, "signal_quality": "long_window"
                }]
            }
        }, "arch_budgets": {},
        "enforcement": {"gate_level": "fast_local", "fail_on_budget_exceeded": True}
    }
    
    # Test 1: Valid long-window baseline schema passes
    errors = validate_ci_leak_slope_long_window(valid_long_window, "test.yaml")
    if len(errors) == 0:
        print("  PASS: Valid long-window baseline schema passes")
        tests_passed += 1
    else:
        print(f"  FAIL: Valid long-window baseline failed: {errors}")
        tests_failed += 1
    
    # Test 2: Wrong signal_quality fails
    wrong_signal = copy.deepcopy(valid_long_window)
    wrong_signal["ci_leak_slope_long_window"]["github_hosted_ubuntu"]["signal_quality"] = "warmup_sensitive"
    errors = validate_ci_leak_slope_long_window(wrong_signal, "test.yaml")
    if len(errors) > 0 and any("signal_quality" in e for e in errors):
        print("  PASS: Wrong signal_quality correctly fails")
        tests_passed += 1
    else:
        print(f"  FAIL: Wrong signal_quality should have failed: {errors}")
        tests_failed += 1
    
    # Test 3: Short duration_seconds fails
    short_duration = copy.deepcopy(valid_long_window)
    short_duration["ci_leak_slope_long_window"]["github_hosted_ubuntu"]["long_window_seconds"] = 100
    errors = validate_ci_leak_slope_long_window(short_duration, "test.yaml")
    if len(errors) > 0 and any("long_window_seconds" in e for e in errors):
        print("  PASS: Short duration_seconds correctly fails")
        tests_passed += 1
    else:
        print(f"  FAIL: Short duration_seconds should have failed: {errors}")
        tests_failed += 1
    
    # Test 4: Short operations fails
    short_ops = copy.deepcopy(valid_long_window)
    short_ops["ci_leak_slope_long_window"]["github_hosted_ubuntu"]["evidence_sources"][0]["operations"] = 100
    errors = validate_ci_leak_slope_long_window(short_ops, "test.yaml")
    if len(errors) > 0 and any("operations" in e for e in errors):
        print("  PASS: Short operations correctly fails")
        tests_passed += 1
    else:
        print(f"  FAIL: Short operations should have failed: {errors}")
        tests_failed += 1
    
    # Test 5: Non-zero request_errors fails
    with_errors = copy.deepcopy(valid_long_window)
    with_errors["ci_leak_slope_long_window"]["github_hosted_ubuntu"]["evidence_sources"][0]["request_errors"] = 5
    errors = validate_ci_leak_slope_long_window(with_errors, "test.yaml")
    if len(errors) > 0 and any("request_errors" in e for e in errors):
        print("  PASS: Non-zero request_errors correctly fails")
        tests_passed += 1
    else:
        print(f"  FAIL: Non-zero request_errors should have failed: {errors}")
        tests_failed += 1
    
    # Test 6: Missing evidence_status fails
    missing_status = copy.deepcopy(valid_long_window)
    del missing_status["ci_leak_slope_long_window"]["github_hosted_ubuntu"]["evidence_status"]
    errors = validate_ci_leak_slope_long_window(missing_status, "test.yaml")
    if len(errors) > 0 and any("evidence_status" in e for e in errors):
        print("  PASS: Missing evidence_status correctly fails")
        tests_passed += 1
    else:
        print(f"  FAIL: Missing evidence_status should have failed: {errors}")
        tests_failed += 1
    
    # Test 7: Missing required field fails
    missing_field = copy.deepcopy(valid_long_window)
    del missing_field["ci_leak_slope_long_window"]["github_hosted_ubuntu"]["evidence_sources"][0]["operations"]
    errors = validate_ci_leak_slope_long_window(missing_field, "test.yaml")
    if len(errors) > 0 and any("operations" in e for e in errors):
        print("  PASS: Missing operations field correctly fails")
        tests_passed += 1
    else:
        print(f"  FAIL: Missing operations field should have failed: {errors}")
        tests_failed += 1
    
    # Test 8: Non-numeric operations fails
    non_numeric_ops = copy.deepcopy(valid_long_window)
    non_numeric_ops["ci_leak_slope_long_window"]["github_hosted_ubuntu"]["evidence_sources"][0]["operations"] = "9000"
    errors = validate_ci_leak_slope_long_window(non_numeric_ops, "test.yaml")
    if len(errors) > 0 and any("must be int" in e for e in errors):
        print("  PASS: Non-numeric operations correctly fails")
        tests_passed += 1
    else:
        print(f"  FAIL: Non-numeric operations should have failed: {errors}")
        tests_failed += 1
    
    # Test 9: Non-numeric duration_seconds fails
    non_numeric_dur = copy.deepcopy(valid_long_window)
    non_numeric_dur["ci_leak_slope_long_window"]["github_hosted_ubuntu"]["evidence_sources"][0]["duration_seconds"] = "906.198"
    errors = validate_ci_leak_slope_long_window(non_numeric_dur, "test.yaml")
    if len(errors) > 0 and any("duration_seconds" in e.lower() and ("numeric" in e.lower() or "str" in e.lower()) for e in errors):
        print("  PASS: Non-numeric duration_seconds correctly fails")
        tests_passed += 1
    else:
        print(f"  FAIL: Non-numeric duration_seconds should have failed: {errors}")
        tests_failed += 1
    
    # Test 10-20: Traceability tests
    with tempfile.TemporaryDirectory() as tmpdir:
        evidence_dir = os.path.join(tmpdir, "docs", "evidence", "memory-lab", "run-28024460972")
        os.makedirs(evidence_dir)
        
        manifest_content = """# Test manifest
evidence_class: memory_leak_slope_long_window
signal_quality: long_window
workflow_run_id: 28024460972
source_commit_sha: "189937815e2493d4179e6bc9729cde74785699ef"

artifacts:
  uvb76:
    artifact_id: 7820989961
    artifact_name: uvb76-memory-lab
    expired: false
"""
        with open(os.path.join(evidence_dir, "manifest.yaml"), "w") as f:
            f.write(manifest_content)
        
        tsv_content = "# Artifact ID to Name mapping\nartifact_id\tname\tsize_in_bytes\texpired\n7820989961\tuvb76-memory-lab\t4381\tfalse\n"
        with open(os.path.join(evidence_dir, "artifacts.tsv"), "w") as f:
            f.write(tsv_content)
        
        valid_artifact = {
            "schema_version": "1.0", "evidence_kind": "real_evidence",
            "service": {"name": "uvb76", "version": "unknown", "commit": "1899378"},
            "environment": {"arch": "linux/amd64"},
            "workload": {"type": "uvb76-leak-slope", "operations": 9000, "errors": 0, "duration_ms": 906198},
            "memory": {"first": {"rss_kib": 11768, "pss_kib": 11760}, "max": {"rss_kib": 17344, "pss_kib": 17336},
                "last": {"rss_kib": 17216, "pss_kib": 17208}, "growth": {"rss_kib": 5448, "pss_kib": 5448}},
            "leak_slope": {"sampled_points": 543, "duration_seconds": 906.198, "rss_first_kib": 11768,
                "rss_max_kib": 17344, "rss_last_kib": 17216, "pss_first_kib": 11760, "pss_max_kib": 17336,
                "pss_last_kib": 17208, "rss_growth_kib": 5448, "pss_growth_kib": 5448,
                "rss_slope_kib_per_min": 360.72, "pss_slope_kib_per_min": 360.72,
                "request_count": 9000, "request_errors": 0},
            "decision": {"pass": True, "reason": "test"}
        }
        
        artifact_path = os.path.join(evidence_dir, "uvb76-leak-slope-28024460972.json")
        with open(artifact_path, "w") as f:
            json.dump(valid_artifact, f)
        
        # Test 10: Valid artifact passes
        errors = check_long_window_evidence_traceability(valid_long_window, "test.yaml", tmpdir)
        if len(errors) == 0:
            print("  PASS: Valid long-window artifact traceability passes")
            tests_passed += 1
        else:
            print(f"  FAIL: Valid long-window artifact traceability failed: {errors}")
            tests_failed += 1
        
        # Test 11: Missing manifest fails
        os.remove(os.path.join(evidence_dir, "manifest.yaml"))
        errors = check_long_window_evidence_traceability(valid_long_window, "test.yaml", tmpdir)
        if len(errors) > 0 and any("Manifest not found" in e for e in errors):
            print("  PASS: Missing manifest correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: Missing manifest should have failed: {errors}")
            tests_failed += 1
        
        with open(os.path.join(evidence_dir, "manifest.yaml"), "w") as f:
            f.write(manifest_content)
        
        # Test 12: Missing artifacts.tsv fails
        os.remove(os.path.join(evidence_dir, "artifacts.tsv"))
        errors = check_long_window_evidence_traceability(valid_long_window, "test.yaml", tmpdir)
        if len(errors) > 0 and any("Artifacts TSV not found" in e for e in errors):
            print("  PASS: Missing artifacts.tsv correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: Missing artifacts.tsv should have failed: {errors}")
            tests_failed += 1
        
        with open(os.path.join(evidence_dir, "artifacts.tsv"), "w") as f:
            f.write(tsv_content)
        
        # Test 13: Wrong artifact_name fails
        wrong_name = copy.deepcopy(valid_long_window)
        wrong_name["ci_leak_slope_long_window"]["github_hosted_ubuntu"]["evidence_sources"][0]["artifact_name"] = "wrong-name"
        errors = check_long_window_evidence_traceability(wrong_name, "test.yaml", tmpdir)
        if len(errors) > 0 and any("artifact_name" in x.lower() for x in errors):
            print("  PASS: Wrong artifact_name correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: Wrong artifact_name should have failed: {errors}")
            tests_failed += 1
        
        # Test 14: Wrong service_commit fails
        wrong_commit = copy.deepcopy(valid_long_window)
        wrong_commit["ci_leak_slope_long_window"]["github_hosted_ubuntu"]["evidence_sources"][0]["service_commit"] = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
        errors = check_long_window_evidence_traceability(wrong_commit, "test.yaml", tmpdir)
        if len(errors) > 0 and any("commit" in x.lower() for x in errors):
            print("  PASS: Wrong service_commit correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: Wrong service_commit should have failed: {errors}")
            tests_failed += 1
        
        # Test 15: Declared operations mismatch fails
        wrong_ops = copy.deepcopy(valid_long_window)
        wrong_ops["ci_leak_slope_long_window"]["github_hosted_ubuntu"]["evidence_sources"][0]["operations"] = 8888
        errors = check_long_window_evidence_traceability(wrong_ops, "test.yaml", tmpdir)
        if len(errors) > 0 and any("operations" in x.lower() for x in errors):
            print("  PASS: Declared operations mismatch correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: Declared operations mismatch should have failed: {errors}")
            tests_failed += 1
        
        # Test 16: Declared duration_seconds mismatch fails
        wrong_dur = copy.deepcopy(valid_long_window)
        wrong_dur["ci_leak_slope_long_window"]["github_hosted_ubuntu"]["evidence_sources"][0]["duration_seconds"] = 100.0
        errors = check_long_window_evidence_traceability(wrong_dur, "test.yaml", tmpdir)
        if len(errors) > 0 and any("duration" in x.lower() for x in errors):
            print("  PASS: Declared duration_seconds mismatch correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: Declared duration_seconds mismatch should have failed: {errors}")
            tests_failed += 1
        
        # Test 17: Declared request_errors mismatch fails
        wrong_errs = copy.deepcopy(valid_long_window)
        wrong_errs["ci_leak_slope_long_window"]["github_hosted_ubuntu"]["evidence_sources"][0]["request_errors"] = 1
        errors = check_long_window_evidence_traceability(wrong_errs, "test.yaml", tmpdir)
        if len(errors) > 0 and any("request_errors" in x.lower() for x in errors):
            print("  PASS: Declared request_errors mismatch correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: Declared request_errors mismatch should have failed: {errors}")
            tests_failed += 1
        
        # Test 18: Declared workload mismatch fails
        wrong_wl = copy.deepcopy(valid_long_window)
        wrong_wl["ci_leak_slope_long_window"]["github_hosted_ubuntu"]["evidence_sources"][0]["workload"] = "wrong-workload"
        errors = check_long_window_evidence_traceability(wrong_wl, "test.yaml", tmpdir)
        if len(errors) > 0 and any("workload" in x.lower() for x in errors):
            print("  PASS: Declared workload mismatch correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: Declared workload mismatch should have failed: {errors}")
            tests_failed += 1
        
        # Test 19: Near-miss duration (differs by <1s but >0.001s) fails
        near_miss_dur = copy.deepcopy(valid_long_window)
        near_miss_dur["ci_leak_slope_long_window"]["github_hosted_ubuntu"]["evidence_sources"][0]["duration_seconds"] = 906.9
        errors = check_long_window_evidence_traceability(near_miss_dur, "test.yaml", tmpdir)
        if len(errors) > 0 and any("duration" in x.lower() for x in errors):
            print("  PASS: Near-miss duration (0.7s diff) correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: Near-miss duration should have failed: {errors}")
            tests_failed += 1
    
    print(f"\n=== Long-Window Self-Test Results ===")
    print(f"  Passed: {tests_passed}, Failed: {tests_failed}")
    return tests_failed == 0


if __name__ == "__main__":
    success = run_long_window_self_tests()
    sys.exit(0 if success else 1)
