#!/usr/bin/env python3
# verify_memory_attribution_matrix.py — Verifier for memory attribution matrix artifacts

import sys
import re
import shutil
from pathlib import Path
from typing import Optional

CANONICAL_VARIANTS = [
    "all_enabled",
    "heartbeat_disabled",
    "wg_disabled",
    "bgp_disabled",
    "bfd_disabled",
    "bgp_bfd_disabled",
    "no_periodic",
]


def read_matrix_manifest(matrix_root):
    manifest_path = matrix_root / "matrix-manifest.yaml"
    if not manifest_path.exists():
        return None
    content = manifest_path.read_text()
    variants = []
    for line in content.split('\n'):
        line = line.strip()
        if line.startswith('- '):
            variant = line[2:].strip()
            if variant:
                variants.append(variant)
    return variants if variants else CANONICAL_VARIANTS


def read_file(path):
    try:
        return path.read_text()
    except FileNotFoundError:
        return None
    except Exception as e:
        print(f"Warning: Could not read {path}: {e}", file=sys.stderr)
        return None


def parse_verdict(content):
    result = {"verdict": "unknown"}
    verdict_match = re.search(r'verdict:\s*(\S+)', content)
    if verdict_match:
        result["verdict"] = verdict_match.group(1)
    owner_match = re.search(r'owner:\s*(\S+)', content)
    if owner_match:
        result["owner"] = owner_match.group(1)
    steps_match = re.search(r'steps_detected:\s*(\d+)', content)
    if steps_match:
        result["steps_detected"] = int(steps_match.group(1))
    growth_match = re.search(r'total_growth_kib:\s*(\d+)', content)
    if growth_match:
        result["total_growth_kib"] = int(growth_match.group(1))
    rate_match = re.search(r'growth_rate_kib_per_min:\s*(\d+)', content)
    if rate_match:
        result["growth_rate_kib_per_min"] = int(rate_match.group(1))
    samples_match = re.search(r'samples_count:\s*(\d+)', content)
    if samples_match:
        result["samples_count"] = int(samples_match.group(1))
    reason_match = re.search(r'reason:\s*(.+?)(?:\n\n|$)', content, re.DOTALL)
    if reason_match:
        result["reason"] = reason_match.group(1).strip()
    return result


def parse_manifest(content):
    result = {}
    for field in ["run_id", "platform", "commit_sha"]:
        match = re.search(rf'{field}:\s*(.+)', content)
        if match:
            result[field] = match.group(1).strip()
    for field in ["native_events_enabled", "native_disable_heartbeat", 
                  "native_disable_wg_checks", "native_disable_bgp", "native_disable_bfd"]:
        match = re.search(rf'{field}:\s*(.+)', content)
        if match:
            val = match.group(1).strip().lower()
            result[field] = val == "true"
    duration_match = re.search(r'duration_seconds:\s*(\d+)', content)
    if duration_match:
        result["duration_seconds"] = int(duration_match.group(1))
    return result


def count_native_events(native_path):
    counts = {"heartbeat": 0, "wireguard": 0, "bgp": 0, "bfd": 0, "total": 0}
    content = read_file(native_path)
    if not content:
        return counts
    for line in content.strip().split('\n')[1:]:
        cols = line.split('\t')
        if len(cols) >= 3:
            event = cols[2]
            if event.startswith("heartbeat_") or event == "heartbeat":
                counts["heartbeat"] += 1
            elif event.startswith("wg_") or event == "wg":
                counts["wireguard"] += 1
            elif event.startswith("bgp_") or event == "bgp":
                counts["bgp"] += 1
            elif event.startswith("bfd_") or event == "bfd":
                counts["bfd"] += 1
            counts["total"] += 1
    return counts


def get_variant_config(variant_name):
    configs = {
        "all_enabled": {"disable_heartbeat": False, "disable_wg_checks": False, 
                        "disable_bgp": False, "disable_bfd": False},
        "heartbeat_disabled": {"disable_heartbeat": True, "disable_wg_checks": False,
                              "disable_bgp": False, "disable_bfd": False},
        "wg_disabled": {"disable_heartbeat": False, "disable_wg_checks": True,
                        "disable_bgp": False, "disable_bfd": False},
        "bgp_disabled": {"disable_heartbeat": False, "disable_wg_checks": False,
                         "disable_bgp": True, "disable_bfd": False},
        "bfd_disabled": {"disable_heartbeat": False, "disable_wg_checks": False,
                         "disable_bgp": False, "disable_bfd": True},
        "bgp_bfd_disabled": {"disable_heartbeat": False, "disable_wg_checks": False,
                             "disable_bgp": True, "disable_bfd": True},
        "no_periodic": {"disable_heartbeat": True, "disable_wg_checks": True,
                        "disable_bgp": True, "disable_bfd": True},
    }
    return configs.get(variant_name)


def verify_variant(variant_path, variant_name):
    errors = []
    data = {}
    manifest = read_file(variant_path / "manifest.yaml")
    verdict = read_file(variant_path / "verdict.txt")
    samples = read_file(variant_path / "memory_samples.tsv")
    native_events = read_file(variant_path / "native_event_timeline.tsv")
    
    if not manifest:
        errors.append("manifest.yaml missing")
    if not verdict:
        errors.append("verdict.txt missing")
    if not samples:
        errors.append("memory_samples.tsv missing")
    if not native_events:
        errors.append("native_event_timeline.tsv missing")
    
    if errors:
        return False, "; ".join(errors), data
    
    manifest_data = parse_manifest(manifest)
    data["manifest"] = manifest_data
    
    if not manifest_data.get("native_events_enabled"):
        errors.append("native_events_enabled is not true in manifest")
    
    verdict_data = parse_verdict(verdict)
    data["verdict"] = verdict_data
    
    valid_verdicts = ["confirmed_leak", "bounded_warmup_or_allocator_highwater", "inconclusive", "no_growth"]
    if verdict_data.get("verdict") not in valid_verdicts:
        errors.append(f"Invalid verdict: {verdict_data.get('verdict')}")
    
    native_counts = count_native_events(variant_path / "native_event_timeline.tsv")
    data["native_counts"] = native_counts
    
    variant_config = get_variant_config(variant_name)
    if variant_config:
        if variant_config.get("disable_heartbeat") and native_counts.get("heartbeat", 0) > 0:
            errors.append(f"Heartbeat disabled but {native_counts['heartbeat']} heartbeat events emitted")
        if variant_config.get("disable_wg_checks") and native_counts.get("wireguard", 0) > 0:
            errors.append(f"WG checks disabled but {native_counts['wireguard']} WG events emitted")
        if variant_config.get("disable_bgp") and native_counts.get("bgp", 0) > 0:
            errors.append(f"BGP disabled but {native_counts['bgp']} BGP events emitted")
        if variant_config.get("disable_bfd") and native_counts.get("bfd", 0) > 0:
            errors.append(f"BFD disabled but {native_counts['bfd']} BFD events emitted")
    
    sample_count = verdict_data.get("samples_count", 0)
    duration = manifest_data.get("duration_seconds", 0)
    min_samples = max(5, duration // 60)
    
    if sample_count < min_samples:
        errors.append(f"Sample count ({sample_count}) below minimum ({min_samples})")
    if duration < 300:
        errors.append(f"Duration ({duration}s) below minimum (300s)")
    
    if errors:
        return False, "; ".join(errors), data
    return True, "", data


def verify_matrix(matrix_root):
    errors = []
    results = {}
    
    summary = read_file(matrix_root / "matrix-summary.md")
    if not summary:
        errors.append("matrix-summary.md missing")
    
    declared_variants = read_matrix_manifest(matrix_root)
    
    missing_variants = []
    for variant in declared_variants:
        variant_path = matrix_root / variant
        if not variant_path.exists():
            missing_variants.append(variant)
    
    if missing_variants:
        errors.append(f"Missing declared variant directories: {', '.join(missing_variants)}")
    
    if errors:
        return False, "; ".join(errors), results
    
    all_valid = True
    for variant in declared_variants:
        variant_path = matrix_root / variant
        valid, error, data = verify_variant(variant_path, variant)
        results[variant] = {"valid": valid, "error": error, "data": data}
        if not valid:
            all_valid = False
    
    if not all_valid:
        failed = [v for v, r in results.items() if not r["valid"]]
        return False, f"Variant verification failed: {', '.join(failed)}", results
    
    if summary:
        verdict_match = re.search(r'\*\*Overall Verdict\*\*\s*\n\s*\*\*(\w+)\*\*', summary)
        if verdict_match:
            verdict = verdict_match.group(1).strip().lower()
            valid_verdicts = ["no_growth", "bounded_warmup_or_allocator_highwater", 
                             "subsystem_correlated_growth", "inconclusive"]
            if verdict not in valid_verdicts:
                return False, f"Invalid matrix verdict: {verdict}", results
    
    return True, "", results


def create_matrix_fixture(tmpdir, name, variants=None):
    matrix = tmpdir / name
    matrix.mkdir(exist_ok=True)
    
    if variants is None:
        variants = CANONICAL_VARIANTS
    
    for variant in variants:
        variant_path = matrix / variant
        variant_path.mkdir(exist_ok=True)
        
        variant_config = get_variant_config(variant)
        disable_hb = variant_config.get("disable_heartbeat", False) if variant_config else False
        disable_wg = variant_config.get("disable_wg_checks", False) if variant_config else False
        disable_bgp = variant_config.get("disable_bgp", False) if variant_config else False
        disable_bfd = variant_config.get("disable_bfd", False) if variant_config else False
        
        (variant_path / "manifest.yaml").write_text(
            f"run_id: test-{variant}\nplatform: Linux\ncommit_sha: abc123\n"
            f"native_events_enabled: true\nnative_disable_heartbeat: {str(disable_hb).lower()}\n"
            f"native_disable_wg_checks: {str(disable_wg).lower()}\n"
            f"native_disable_bgp: {str(disable_bgp).lower()}\n"
            f"native_disable_bfd: {str(disable_bfd).lower()}\n"
            f"duration_seconds: 600\n"
        )
        
        growth = 200 if variant == "all_enabled" else 50
        (variant_path / "verdict.txt").write_text(
            f"verdict: inconclusive\nsteps_detected: 2\ntotal_growth_kib: {growth}\n"
            f"growth_rate_kib_per_min: 5\nsamples_count: 20\n"
        )
        
        (variant_path / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:05:00\t300\t1050\t2050\n"
        )
        
        hb_count = 0 if disable_hb else 6
        wg_count = 0 if disable_wg else 2
        bgp_count = 0 if disable_bgp else 10
        bfd_count = 0 if disable_bfd else 10
        
        lines = ["timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid"]
        for i in range(hb_count):
            lines.append(f"2024-01-01T00:00:{30+i*30}\t{30000+i*30000}\theartbeat_tick_start\theartbeat\t\t1234")
        for i in range(wg_count):
            lines.append(f"2024-01-01T00:01:{i*60}\t{60000+i*60000}\twg_check_start\twireguard\t\t1234")
        for i in range(bgp_count):
            lines.append(f"2024-01-01T00:00:{i*10}\t{i*10000}\tbgp_maintenance_start\tbgp\t\t1234")
        for i in range(bfd_count):
            lines.append(f"2024-01-01T00:00:{i*10}\t{i*10000}\tbfd_tick_start\tbfd\t\t1234")
        
        (variant_path / "native_event_timeline.tsv").write_text("\n".join(lines) + "\n")
    
    summary = "# Memory Attribution Matrix Summary\n\n**Matrix Run ID**: `test-matrix`\n**Overall Verdict**: `INCONCLUSIVE`\n\n| Variant | Status | Growth (KiB) | Rate (KiB/min) | Steps | Verdict |\n|---------|--------|--------------|----------------|-------|---------|\n"
    for variant in variants:
        summary += f"| {variant} | OK | 200 | 5 | 2 | inconclusive |\n"
    
    (matrix / "matrix-summary.md").write_text(summary)
    
    manifest = f"run_id: test-matrix\nduration_seconds: 600\nsample_interval_seconds: 5\nvariants:\n"
    for variant in variants:
        manifest += f"  - {variant}\n"
    (matrix / "matrix-manifest.yaml").write_text(manifest)
    
    return matrix


def run_self_tests():
    import tempfile
    
    print("=== Memory Attribution Matrix Verifier Self-Tests ===\n")
    
    tests_passed = 0
    tests_failed = 0
    
    with tempfile.TemporaryDirectory() as tmpdir:
        tmppath = Path(tmpdir)
        
        print("Test 1: Valid full matrix passes")
        fixture = create_matrix_fixture(tmppath, "valid_matrix")
        valid, error, _ = verify_matrix(fixture)
        if valid:
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: {error}")
            tests_failed += 1
        
        print("Test 2: Valid partial matrix (2 variants) passes")
        fixture = create_matrix_fixture(tmppath, "partial", variants=["all_enabled", "no_periodic"])
        valid, error, _ = verify_matrix(fixture)
        if valid:
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: {error}")
            tests_failed += 1
        
        print("Test 3: Missing matrix summary fails")
        fixture = create_matrix_fixture(tmppath, "no_summary")
        (fixture / "matrix-summary.md").unlink()
        valid, error, _ = verify_matrix(fixture)
        if not valid and "matrix-summary.md" in error:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected rejection, got valid={valid}, error={error}")
            tests_failed += 1
        
        print("Test 4: Missing declared variant fails")
        fixture = create_matrix_fixture(tmppath, "missing_var", variants=["all_enabled", "no_periodic"])
        shutil.rmtree(fixture / "no_periodic")
        valid, error, _ = verify_matrix(fixture)
        if not valid and "Missing" in error:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected rejection, got valid={valid}, error={error}")
            tests_failed += 1
        
        print("Test 5: Missing native events fails")
        fixture = create_matrix_fixture(tmppath, "no_native", variants=["all_enabled"])
        (fixture / "all_enabled" / "native_event_timeline.tsv").unlink()
        valid, error, results = verify_matrix(fixture)
        failed_variants = [v for v, r in results.items() if not r["valid"] and "native_event_timeline" in r.get("error", "")]
        if not valid and failed_variants:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected rejection, got valid={valid}, failed_variants={failed_variants}")
            tests_failed += 1
        
        print("Test 6: Heartbeat leak when disabled fails")
        fixture = create_matrix_fixture(tmppath, "hb_leak", variants=["heartbeat_disabled"])
        native_path = fixture / "heartbeat_disabled" / "native_event_timeline.tsv"
        content = native_path.read_text()
        # Ensure clean append on a new TSV row
        native_path.write_text(content.rstrip("\n") + "\n" + "2024-01-01T00:00:30\t30000\theartbeat_tick_start\theartbeat\t\t1234\n")
        valid, error, results = verify_matrix(fixture)
        failed_variants = [v for v, r in results.items() if not r["valid"] and "Heartbeat disabled" in r.get("error", "")]
        if not valid and failed_variants:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected rejection, got valid={valid}, error={error}")
            tests_failed += 1
    
    print(f"\nResults: {tests_passed} passed, {tests_failed} failed")
    return tests_failed == 0


def main():
    import argparse
    
    parser = argparse.ArgumentParser(description="Verify memory attribution matrix artifacts")
    parser.add_argument("matrix_root", nargs="?", help="Path to matrix root directory")
    parser.add_argument("--self-test", action="store_true", help="Run self-tests")
    
    args = parser.parse_args()
    
    if args.self_test:
        success = run_self_tests()
        sys.exit(0 if success else 1)
    
    if not args.matrix_root:
        parser.print_help()
        sys.exit(1)
    
    matrix_root = Path(args.matrix_root)
    
    if not matrix_root.exists():
        print(f"ERROR: Matrix root does not exist: {matrix_root}", file=sys.stderr)
        sys.exit(1)
    
    print("=== Memory Attribution Matrix Verifier ===")
    print(f"Matrix Root: {matrix_root}")
    
    declared = read_matrix_manifest(matrix_root)
    if declared is None:
        print(f"Variants: {len(CANONICAL_VARIANTS)} (canonical full set)")
    else:
        print(f"Variants: {len(declared)} (from matrix-manifest.yaml)")
    print()
    
    valid, error, results = verify_matrix(matrix_root)
    
    print("Variant Results:\n")
    
    all_ok = True
    for variant in declared or CANONICAL_VARIANTS:
        if variant not in results:
            continue
        variant_data = results[variant]
        if variant_data.get("valid"):
            print(f"  OK {variant}")
            data = variant_data.get("data", {})
            vd = data.get("verdict", {})
            nc = data.get("native_counts", {})
            print(f"    Verdict: {vd.get('verdict', 'unknown')}")
            print(f"    Growth: {vd.get('total_growth_kib', 'N/A')} KiB")
            print(f"    Native events: HB={nc.get('heartbeat', 0)}, WG={nc.get('wireguard', 0)}, BGP={nc.get('bgp', 0)}, BFD={nc.get('bfd', 0)}")
        else:
            print(f"  FAIL {variant}: {variant_data.get('error', 'Unknown error')}")
            all_ok = False
        print()
    
    if valid:
        print("VERIFICATION PASSED")
        sys.exit(0)
    else:
        print(f"VERIFICATION FAILED: {error}")
        sys.exit(1)


if __name__ == "__main__":
    main()
