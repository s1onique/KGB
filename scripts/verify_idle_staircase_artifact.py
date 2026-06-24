#!/usr/bin/env python3
# verify_idle_staircase_artifact.py — Verifier for idle staircase memory lab artifacts
#
# ACT: Attribute and fix tovarisch idle/background staircase memory growth
#
# Verifies that a memory lab artifact is complete and well-formed.
# Rejects artifacts that are missing required files, have malformed data,
# or claim confirmed_leak without proper attribution.
#
# Usage:
#   python scripts/verify_idle_staircase_artifact.py <artifact_dir>
#   python scripts/verify_idle_staircase_artifact.py --self-test
#
# Exit codes:
#   0 - Artifact is valid
#   1 - Artifact validation failed (with reason)

import argparse
import os
import sys
import re
from pathlib import Path
from typing import Optional

REQUIRED_FILES = [
    "manifest.yaml",
    "memory_samples.tsv",
    "event_timeline.tsv",
    "verdict.txt",
]

VALID_VERDICTS = [
    "confirmed_leak",
    "bounded_warmup_or_allocator_highwater",
    "inconclusive",
]


def verify_manifest(artifact_path: Path) -> tuple[bool, Optional[str]]:
    """Verify manifest.yaml is present and non-empty."""
    manifest_path = artifact_path / "manifest.yaml"
    
    if not manifest_path.exists():
        return False, f"manifest.yaml missing in {artifact_path}"
    
    content = manifest_path.read_text()
    if len(content.strip()) < 10:
        return False, "manifest.yaml is empty or too small"
    
    # Check for required fields
    required_fields = ["run_id:", "platform:", "commit_sha:"]
    for field in required_fields:
        if field not in content:
            return False, f"manifest.yaml missing required field: {field}"
    
    return True, None


def verify_memory_samples(artifact_path: Path) -> tuple[bool, Optional[str]]:
    """Verify memory_samples.tsv has required columns and data."""
    tsv_path = artifact_path / "memory_samples.tsv"
    
    if not tsv_path.exists():
        return False, "memory_samples.tsv missing"
    
    content = tsv_path.read_text()
    lines = content.strip().split('\n')
    
    if len(lines) < 2:
        return False, "memory_samples.tsv has no data rows (only header)"
    
    # Check header columns
    header = lines[0]
    required_cols = ["timestamp", "elapsed_sec", "rss_kib", "vmdata_kib"]
    for col in required_cols:
        if col not in header:
            return False, f"memory_samples.tsv missing required column: {col}"
    
    # Verify data rows have correct number of columns
    num_cols = len(header.split('\t'))
    for i, line in enumerate(lines[1:], start=2):
        cols = line.split('\t')
        if len(cols) != num_cols:
            return False, f"memory_samples.tsv line {i} has wrong column count: {len(cols)} vs {num_cols}"
        
        # Check RSS is numeric
        try:
            rss_idx = header.split('\t').index('rss_kib')
            rss = int(cols[rss_idx])
            if rss < 0:
                return False, f"memory_samples.tsv line {i} has negative RSS: {rss}"
        except (ValueError, IndexError):
            pass
    
    return True, None


def verify_event_timeline(artifact_path: Path) -> tuple[bool, Optional[str]]:
    """Verify event_timeline.tsv has required columns."""
    tsv_path = artifact_path / "event_timeline.tsv"
    
    if not tsv_path.exists():
        return False, "event_timeline.tsv missing"
    
    content = tsv_path.read_text()
    lines = content.strip().split('\n')
    
    if len(lines) < 1:
        return False, "event_timeline.tsv is empty"
    
    # Check header columns
    header = lines[0]
    required_cols = ["timestamp", "elapsed_sec", "event", "subsystem"]
    for col in required_cols:
        if col not in header:
            return False, f"event_timeline.tsv missing required column: {col}"
    
    return True, None


def verify_verdict(artifact_path: Path) -> tuple[bool, Optional[str]]:
    """Verify verdict.txt has valid verdict and proper attribution."""
    verdict_path = artifact_path / "verdict.txt"
    
    if not verdict_path.exists():
        return False, "verdict.txt missing"
    
    content = verdict_path.read_text()
    
    # Check for verdict line
    verdict_match = re.search(r'verdict:\s*(\S+)', content)
    if not verdict_match:
        return False, "verdict.txt missing 'verdict:' field"
    
    verdict = verdict_match.group(1)
    if verdict not in VALID_VERDICTS:
        return False, f"verdict.txt has invalid verdict: {verdict}"
    
    # For confirmed_leak, require owner attribution
    if verdict == "confirmed_leak":
        if not re.search(r'owner:\s*\S+', content):
            return False, "verdict=confirmed_leak requires owner attribution"
        
        owner_match = re.search(r'owner:\s*(\S+)', content)
        owner = owner_match.group(1) if owner_match else ""
        
        if owner in ["unknown", ""]:
            return False, "verdict=confirmed_leak requires non-empty owner"
    
    # For bounded verdict, require evidence of plateau or bounded growth
    if verdict == "bounded_warmup_or_allocator_highwater":
        if not re.search(r'(total_growth_kib:|steps_detected:)', content):
            return False, "verdict=bounded requires growth evidence (total_growth_kib or steps_detected)"
        
        # If claiming bounded, should have low growth or low steps
        growth_match = re.search(r'total_growth_kib:\s*(\d+)', content)
        steps_match = re.search(r'steps_detected:\s*(\d+)', content)
        
        if growth_match:
            growth = int(growth_match.group(1))
            if growth > 2000:
                return False, "verdict=bounded but total_growth_kib > 2000, seems like continued growth"
        
        if steps_match:
            steps = int(steps_match.group(1))
            if steps > 10:
                return False, "verdict=bounded but steps_detected > 10, seems like continued staircase"
    
    return True, None


def verify_artifact(artifact_path: Path) -> tuple[bool, Optional[str]]:
    """Verify a single artifact directory."""
    if not artifact_path.exists():
        return False, f"Artifact directory does not exist: {artifact_path}"
    
    if not artifact_path.is_dir():
        return False, f"Artifact path is not a directory: {artifact_path}"
    
    # Check required files
    for filename in REQUIRED_FILES:
        filepath = artifact_path / filename
        if not filepath.exists():
            return False, f"Required file missing: {filename}"
    
    # Verify each component
    checks = [
        ("manifest", verify_manifest),
        ("memory_samples", verify_memory_samples),
        ("event_timeline", verify_event_timeline),
        ("verdict", verify_verdict),
    ]
    
    for name, check_fn in checks:
        valid, error = check_fn(artifact_path)
        if not valid:
            return False, f"[{name}] {error}"
    
    return True, None


def self_test() -> bool:
    """Run self-tests on the verifier."""
    import tempfile
    import shutil
    
    print("Running self-tests...")
    
    tests_passed = 0
    tests_failed = 0
    
    def test(name: str, should_pass: bool, setup_fn=None) -> bool:
        nonlocal tests_passed, tests_failed
        
        with tempfile.TemporaryDirectory() as tmpdir:
            artifact_path = Path(tmpdir) / "test_artifact"
            artifact_path.mkdir()
            
            if setup_fn:
                setup_fn(artifact_path)
            
            valid, error = verify_artifact(artifact_path)
            
            if should_pass and valid:
                print(f"  PASS: {name}")
                tests_passed += 1
                return True
            elif not should_pass and not valid:
                print(f"  PASS: {name} (correctly rejected)")
                tests_passed += 1
                return True
            else:
                print(f"  FAIL: {name}")
                if error:
                    print(f"        Error: {error}")
                tests_failed += 1
                return False
    
    # Test 1: Missing files (no files needed - artifact dir is empty)
    def setup_missing(p):
        pass
    test("missing all files", False, setup_missing)
    
    # Test 2: Missing memory samples
    def setup_missing_samples(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\n")
    test("missing memory_samples.tsv", False, setup_missing_samples)
    
    # Test 3: Missing events
    def setup_missing_events(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\n")
    test("missing event_timeline.tsv", False, setup_missing_events)
    
    # Test 4: Invalid verdict
    def setup_invalid_verdict(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01\t0\tlab_started\tlab\n")
        (p / "verdict.txt").write_text("verdict: invalid_verdict\n")
    test("invalid verdict", False, setup_invalid_verdict)
    
    # Test 5: confirmed_leak without owner
    def setup_leak_no_owner(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01\t0\tlab_started\tlab\n")
        (p / "verdict.txt").write_text("verdict: confirmed_leak\nreason: test\n")
    test("confirmed_leak without owner", False, setup_leak_no_owner)
    
    # Test 6: bounded verdict without evidence
    def setup_bounded_no_evidence(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01\t0\tlab_started\tlab\n")
        (p / "verdict.txt").write_text("verdict: bounded_warmup_or_allocator_highwater\n")
    test("bounded verdict without evidence", False, setup_bounded_no_evidence)
    
    # Test 7: Valid artifact
    def setup_valid(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\tvmhwm_kib\tvmswap_kib\tvmpeak_kib\tvmrss_peak_kib\n2024-01-01\t0\t1000\t2000\t1000\t0\t3000\t1000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\tdetail\n2024-01-01\t0\tlab_started\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("valid artifact", True, setup_valid)
    
    # Test 8: Valid confirmed_leak with owner
    def setup_valid_leak(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\tvmhwm_kib\tvmswap_kib\tvmpeak_kib\tvmrss_peak_kib\n2024-01-01\t0\t1000\t2000\t1000\t0\t3000\t1000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\tdetail\n2024-01-01\t0\tlab_started\tlab\n")
        (p / "verdict.txt").write_text("verdict: confirmed_leak\nowner: heartbeat\nreason: test\nsteps_detected: 5\ntotal_growth_kib: 1000\n")
    test("valid confirmed_leak", True, setup_valid_leak)
    
    # Test 9: Valid bounded verdict
    def setup_valid_bounded(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\tvmhwm_kib\tvmswap_kib\tvmpeak_kib\tvmrss_peak_kib\n2024-01-01\t0\t1000\t2000\t1000\t0\t3000\t1000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\tdetail\n2024-01-01\t0\tlab_started\tlab\n")
        (p / "verdict.txt").write_text("verdict: bounded_warmup_or_allocator_highwater\nreason: test\nsteps_detected: 2\ntotal_growth_kib: 100\n")
    test("valid bounded verdict", True, setup_valid_bounded)
    
    # Summary
    print(f"\nSelf-test results: {tests_passed} passed, {tests_failed} failed")
    return tests_failed == 0


def main():
    parser = argparse.ArgumentParser(
        description="Verify idle staircase memory lab artifacts"
    )
    parser.add_argument(
        "artifact_dir",
        nargs="?",
        help="Path to artifact directory to verify"
    )
    parser.add_argument(
        "--self-test",
        action="store_true",
        help="Run self-tests on the verifier"
    )
    
    args = parser.parse_args()
    
    if args.self_test:
        success = self_test()
        sys.exit(0 if success else 1)
    
    if not args.artifact_dir:
        parser.print_help()
        sys.exit(1)
    
    artifact_path = Path(args.artifact_dir)
    valid, error = verify_artifact(artifact_path)
    
    if valid:
        print(f"OK: Artifact is valid: {artifact_path}")
        sys.exit(0)
    else:
        print(f"ERROR: {error}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
