#!/usr/bin/env python3
"""Verifier for UVB-76 memory attribution analysis artifacts. Reference: kgb://doctrine/embedded-memory-frugality"""

import json
import os
import sys
import tempfile
import shutil

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ALLOWED_VERDICTS = ["confirmed_leak", "bounded_warmup_plateau", "inconclusive"]


def validate_analysis_json(path, run_dir=None):
    errors = []
    if not os.path.exists(path):
        return [f"analysis.json does not exist: {path}"]
    try:
        with open(path) as f:
            data = json.load(f)
    except json.JSONDecodeError as e:
        return [f"Invalid JSON in analysis.json: {e}"]
    if not isinstance(data, dict):
        return ["analysis.json must be a JSON object"]
    for field in ["schema_version", "analysis_kind", "run_id", "verdict", "verdict_confidence",
                  "verdict_rationale", "evidence_summary", "limitations", "recommended_next_steps"]:
        if field not in data:
            errors.append(f"analysis.json missing required field: {field}")
    if "verdict" in data and data["verdict"] not in ALLOWED_VERDICTS:
        errors.append(f"analysis.json verdict must be one of {ALLOWED_VERDICTS}, got: {data['verdict']}")
    
    verdict = data.get("verdict")
    if verdict == "confirmed_leak":
        if "heap_analysis" not in data or "top_allocators" not in data.get("heap_analysis", {}):
            errors.append("verdict=confirmed_leak requires top_allocators in heap_analysis")
        elif data.get("heap_analysis", {}).get("top_allocators") == "unknown":
            errors.append("verdict=confirmed_leak requires identified allocation owners from heap profile comparison")
    
    elif verdict == "bounded_warmup_plateau":
        # Check attribution artifacts exist - manifest.yaml alone is NOT sufficient
        src = data.get("source_artifacts", [])
        has_attr = any(a.startswith("heap-") or a.startswith("memstats-") or "attribution" in a for a in src)
        if run_dir:
            # Require actual attribution markers: heap triplet OR memstats triplet
            has_heap_triplet = all(os.path.exists(os.path.join(run_dir, name)) for name in [
                "heap-start.pprof", "heap-midpoint.pprof", "heap-end.pprof"])
            has_memstats_triplet = all(os.path.exists(os.path.join(run_dir, name)) for name in [
                "memstats-start.json", "memstats-midpoint.json", "memstats-end.json"])
            has_rss_pss = os.path.exists(os.path.join(run_dir, "rss-pss.tsv"))
            has_attr = has_rss_pss and (has_heap_triplet or has_memstats_triplet)
        if not has_attr:
            errors.append("verdict=bounded_warmup_plateau requires attribution lab artifacts (heap profiles + rss-pss.tsv, OR memstats checkpoints + rss-pss.tsv)")
        # Check plateau evidence with strict threshold
        sl = data.get("slope_analysis", {}).get("plateau_indicator", {})
        if isinstance(sl, dict):
            ratio = sl.get("slope_midpoint_to_end_vs_start_to_midpoint", 1.0)
            confirmed = sl.get("confirmed_plateau_evidence", False)
            if ratio >= 0.5 and not confirmed:
                errors.append(f"verdict=bounded_warmup_plateau requires slope ratio < 0.5 or confirmed_plateau_evidence=true (got {ratio})")
    
    elif verdict == "inconclusive":
        if not data.get("recommended_next_steps"):
            errors.append("verdict=inconclusive requires non-empty recommended_next_steps")
    
    if "privacy_check" in data and not data["privacy_check"].get("compliant", False):
        errors.append("analysis.json privacy_check.compliant must be true")
    return errors


def validate_analysis_dir(dir_path):
    errors = validate_analysis_json(os.path.join(dir_path, "analysis.json"), dir_path)
    if not os.path.exists(os.path.join(dir_path, "analysis.md")):
        errors.append("analysis.md is recommended but not present")
    return errors


def run_verifier(repo_root):
    all_errors = []
    print("=== UVB-76 Memory Attribution Analysis Verifier ===\n")
    validated = []
    for entry in os.listdir(os.path.join(repo_root, "docs", "evidence", "memory-lab")):
        entry_path = os.path.join(repo_root, "docs", "evidence", "memory-lab", entry)
        if os.path.isdir(entry_path) and os.path.exists(os.path.join(entry_path, "analysis.json")):
            print(f"  Validating: {entry}/analysis.json")
            errors = validate_analysis_dir(entry_path)
            if errors:
                for e in errors:
                    print(f"    ERROR: {e}")
                all_errors.extend(errors)
            else:
                validated.append(entry_path)
                print(f"    OK: Valid analysis artifact")
    print(f"\n{'='*50}\nSUMMARY: {len(validated)} artifacts validated, {len(all_errors)} errors")
    return all_errors


def run_self_tests():
    results, errors = {}, []
    print("=== Attribution Analysis Verifier Self-Tests ===\n")
    test_dir = tempfile.mkdtemp(prefix="attribution-analysis-test-")
    try:
        # Test 1: bounded_warmup_plateau without attribution artifacts
        d1 = os.path.join(test_dir, "t1")
        os.makedirs(d1)
        with open(os.path.join(d1, "analysis.json"), "w") as f:
            json.dump({"schema_version": "1.0", "analysis_kind": "test", "run_id": "t1",
                       "verdict": "bounded_warmup_plateau", "verdict_confidence": "m",
                       "verdict_rationale": "test", "evidence_summary": {}, "slope_analysis": {},
                       "heap_analysis": {"top_allocators": "unknown"}, "limitations": [],
                       "recommended_next_steps": [], "source_artifacts": ["leak-slope.json"],
                       "privacy_check": {"compliant": True}}, f)
        errs = validate_analysis_json(os.path.join(d1, "analysis.json"), d1)
        results["bounded_no_attr"] = "requires attribution lab artifacts" in str(errs)
        print(f"  bounded_no_attr: {'PASS' if results['bounded_no_attr'] else 'FAIL'}")
        
        # Test 2: bounded_warmup_plateau with ratio 0.954
        d2 = os.path.join(test_dir, "t2")
        os.makedirs(d2)
        for f in ["heap-start.pprof", "heap-midpoint.pprof", "heap-end.pprof", "manifest.yaml"]:
            open(os.path.join(d2, f), "w").close()
        with open(os.path.join(d2, "analysis.json"), "w") as f:
            json.dump({"schema_version": "1.0", "analysis_kind": "test", "run_id": "t2",
                       "verdict": "bounded_warmup_plateau", "verdict_confidence": "m",
                       "verdict_rationale": "test", "evidence_summary": {},
                       "slope_analysis": {"plateau_indicator": {"slope_midpoint_to_end_vs_start_to_midpoint": 0.954}},
                       "heap_analysis": {"top_allocators": "unknown"}, "limitations": [],
                       "recommended_next_steps": [], "source_artifacts": ["heap-start.pprof"],
                       "privacy_check": {"compliant": True}}, f)
        errs = validate_analysis_json(os.path.join(d2, "analysis.json"), d2)
        results["bounded_high_ratio"] = "slope ratio" in str(errs).lower() or "flattening" in str(errs).lower()
        print(f"  bounded_high_ratio: {'PASS' if results['bounded_high_ratio'] else 'FAIL'}")
        
        # Test 3: bounded_warmup_plateau with valid evidence (heap triplet + rss-pss.tsv)
        d3 = os.path.join(test_dir, "t3")
        os.makedirs(d3)
        for f in ["heap-start.pprof", "heap-midpoint.pprof", "heap-end.pprof", "rss-pss.tsv"]:
            open(os.path.join(d3, f), "w").close()
        with open(os.path.join(d3, "analysis.json"), "w") as f:
            json.dump({"schema_version": "1.0", "analysis_kind": "test", "run_id": "t3",
                       "verdict": "bounded_warmup_plateau", "verdict_confidence": "h",
                       "verdict_rationale": "test", "evidence_summary": {},
                       "slope_analysis": {"plateau_indicator": {"slope_midpoint_to_end_vs_start_to_midpoint": 0.3, "confirmed_plateau_evidence": True}},
                       "heap_analysis": {"top_allocators": "goroutine stacks"}, "limitations": [],
                       "recommended_next_steps": [], "source_artifacts": ["heap-start.pprof"],
                       "privacy_check": {"compliant": True}}, f)
        errs = validate_analysis_json(os.path.join(d3, "analysis.json"), d3)
        results["bounded_valid"] = len(errs) == 0
        print(f"  bounded_valid: {'PASS' if results['bounded_valid'] else 'FAIL'}")
        
        # Test 4: bounded_warmup_plateau with manifest.yaml only (no real attribution)
        d4 = os.path.join(test_dir, "t4")
        os.makedirs(d4)
        open(os.path.join(d4, "manifest.yaml"), "w").close()  # manifest.yaml alone NOT sufficient
        with open(os.path.join(d4, "analysis.json"), "w") as f:
            json.dump({"schema_version": "1.0", "analysis_kind": "test", "run_id": "t4",
                       "verdict": "bounded_warmup_plateau", "verdict_confidence": "m",
                       "verdict_rationale": "test", "evidence_summary": {},
                       "slope_analysis": {"plateau_indicator": {"slope_midpoint_to_end_vs_start_to_midpoint": 0.3}},
                       "heap_analysis": {"top_allocators": "unknown"}, "limitations": [],
                       "recommended_next_steps": [], "source_artifacts": ["manifest.yaml"],
                       "privacy_check": {"compliant": True}}, f)
        errs = validate_analysis_json(os.path.join(d4, "analysis.json"), d4)
        results["bounded_manifest_only"] = "requires attribution lab artifacts" in str(errs)
        print(f"  bounded_manifest_only: {'PASS' if results['bounded_manifest_only'] else 'FAIL'}")
        
        # Test 5: confirmed_leak with unknown allocators
        d5 = os.path.join(test_dir, "t5")
        os.makedirs(d5)
        with open(os.path.join(d5, "analysis.json"), "w") as f:
            json.dump({"schema_version": "1.0", "analysis_kind": "test", "run_id": "t5",
                       "verdict": "confirmed_leak", "verdict_confidence": "h",
                       "verdict_rationale": "test", "evidence_summary": {},
                       "heap_analysis": {"top_allocators": "unknown"}, "limitations": [],
                       "recommended_next_steps": [], "privacy_check": {"compliant": True}}, f)
        errs = validate_analysis_json(os.path.join(d5, "analysis.json"), d5)
        results["leak_unknown"] = "identified allocation owners" in str(errs)
        print(f"  leak_unknown: {'PASS' if results['leak_unknown'] else 'FAIL'}")
        
        # Test 6: inconclusive with empty next_steps
        d6 = os.path.join(test_dir, "t6")
        os.makedirs(d6)
        with open(os.path.join(d6, "analysis.json"), "w") as f:
            json.dump({"schema_version": "1.0", "analysis_kind": "test", "run_id": "t6",
                       "verdict": "inconclusive", "verdict_confidence": "l",
                       "verdict_rationale": "test", "evidence_summary": {},
                       "limitations": [], "recommended_next_steps": [],
                       "privacy_check": {"compliant": True}}, f)
        errs = validate_analysis_json(os.path.join(d6, "analysis.json"), d6)
        results["inconclusive_empty"] = "non-empty recommended_next_steps" in str(errs)
        print(f"  inconclusive_empty: {'PASS' if results['inconclusive_empty'] else 'FAIL'}")
    finally:
        shutil.rmtree(test_dir, ignore_errors=True)
    print(f"\n{'='*50}\nSELF-TEST SUMMARY:")
    for k, v in results.items():
        print(f"  {k}: {'PASS' if v else 'FAIL'}")
    return [f"{k}: FAIL" for k, v in results.items() if not v], results


def main():
    if "--self-test" in sys.argv:
        errs, _ = run_self_tests()
        if errs:
            print("\nSELF-TEST ERRORS:", errs)
            sys.exit(1)
        print("\nAll self-tests passed!")
        sys.exit(0)
    errs = run_verifier(REPO_ROOT)
    print(f"\n{'='*50}\n{'VERIFICATION FAILED' if errs else 'VERIFICATION PASSED'}")
    sys.exit(1 if errs else 0)


if __name__ == "__main__":
    main()
