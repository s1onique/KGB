#!/usr/bin/env python3
# verify_idle_staircase_artifact.py — Verifier for idle staircase memory lab artifacts
#
# ACT: Repair tovarisch idle-staircase lab blockers
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

# Required columns for event_timeline.tsv
REQUIRED_EVENT_COLS = ["timestamp", "elapsed_sec", "event", "subsystem"]

# Terminal events that end the lab
TERMINAL_EVENTS = ["idle_complete", "shutdown", "lab_failed"]


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
    """Verify event_timeline.tsv has required structure and events."""
    tsv_path = artifact_path / "event_timeline.tsv"
    
    if not tsv_path.exists():
        return False, "event_timeline.tsv missing"
    
    content = tsv_path.read_text()
    lines = content.strip().split('\n')
    
    if len(lines) < 1:
        return False, "event_timeline.tsv is empty"
    
    # Check header columns - must have all required columns
    header = lines[0]
    header_cols = header.split('\t')
    for col in REQUIRED_EVENT_COLS:
        if col not in header_cols:
            return False, f"event_timeline.tsv missing required column: {col}"
    
    # Verify header has consistent column count
    num_cols = len(header_cols)
    
    # Require at least one data row (after header)
    if len(lines) < 2:
        return False, "event_timeline.tsv has no data rows (header only)"
    
    # Parse event rows and verify structure
    events = []
    has_lab_started = False
    has_terminal = False
    
    for i, line in enumerate(lines[1:], start=2):
        cols = line.split('\t')
        
        # Reject malformed rows with wrong column count
        if len(cols) != num_cols:
            return False, f"event_timeline.tsv line {i} has wrong column count: {len(cols)} vs {num_cols}"
        
        # Get event name (3rd column, index 2)
        if len(cols) >= 3:
            event = cols[2]
            events.append(event)
            
            if event == "lab_started":
                has_lab_started = True
            
            if event in TERMINAL_EVENTS:
                has_terminal = True
    
    # Require lab_started event
    if not has_lab_started:
        return False, "event_timeline.tsv missing required 'lab_started' event"
    
    # Require terminal event
    if not has_terminal:
        return False, f"event_timeline.tsv missing terminal event (one of: {', '.join(TERMINAL_EVENTS)})"
    
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
    
    # Extract owner if present
    owner_match = re.search(r'owner:\s*(\S+)', content)
    owner = owner_match.group(1) if owner_match else ""
    
    # For confirmed_leak, require non-empty, non-unknown owner
    if verdict == "confirmed_leak":
        if not owner_match:
            return False, "verdict=confirmed_leak requires owner attribution"
        
        if owner in ["unknown", ""]:
            return False, "verdict=confirmed_leak requires non-empty owner (not 'unknown')"
        
        # Require evidence fields
        if not re.search(r'steps_detected:', content):
            return False, "verdict=confirmed_leak requires steps_detected evidence"
        
        if not re.search(r'total_growth_kib:', content):
            return False, "verdict=confirmed_leak requires total_growth_kib evidence"
        
        if not re.search(r'growth_rate_kib_per_min:', content):
            return False, "verdict=confirmed_leak requires growth_rate_kib_per_min evidence"
        
        # If owner is specified, require matching event evidence in timeline
        event_timeline_path = artifact_path / "event_timeline.tsv"
        if event_timeline_path.exists():
            timeline_content = event_timeline_path.read_text()
            # Check if any event/subsystem correlates with owner
            owner_lower = owner.lower()
            if owner_lower not in timeline_content.lower():
                return False, f"verdict=confirmed_leak with owner='{owner}' requires matching event evidence in timeline"
    
    # For bounded verdict, require evidence and reject obvious continued staircase
    if verdict == "bounded_warmup_or_allocator_highwater":
        if not re.search(r'(total_growth_kib:|steps_detected:)', content):
            return False, "verdict=bounded requires growth evidence (total_growth_kib or steps_detected)"
        
        # Reject bounded verdict if it shows obvious continued staircase growth
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
    
    # For inconclusive verdict with staircase growth, require reason explaining unattribution
    if verdict == "inconclusive":
        # Check if this is an owner-unknown staircase finding
        steps_match = re.search(r'steps_detected:\s*(\d+)', content)
        growth_match = re.search(r'total_growth_kib:\s*(\d+)', content)
        
        if steps_match and growth_match:
            steps = int(steps_match.group(1))
            growth = int(growth_match.group(1))
            
            # If this looks like a staircase finding with unknown owner, reason must explain it
            if steps >= 3 and growth > 500 and owner in ["", "unknown"]:
                reason_match = re.search(r'reason:\s*(.+?)(?:\n|$)', content, re.DOTALL)
                if reason_match:
                    reason = reason_match.group(1).lower()
                    if "unattributed" not in reason and "owner" not in reason:
                        return False, "verdict=inconclusive for staircase growth requires reason explaining owner is unattributed"
                else:
                    return False, "verdict=inconclusive for staircase growth requires reason field"
    
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
    
    # === Basic file/format tests ===
    
    # Test 1: Missing all files
    def setup_missing(p):
        pass
    test("missing all files", False, setup_missing)
    
    # Test 2: Missing memory samples
    def setup_missing_samples(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("missing memory_samples.tsv", False, setup_missing_samples)
    
    # Test 3: Missing events
    def setup_missing_events(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("missing event_timeline.tsv", False, setup_missing_events)
    
    # Test 4: Invalid verdict
    def setup_invalid_verdict(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: invalid_verdict\n")
    test("invalid verdict", False, setup_invalid_verdict)
    
    # === Confirmed leak tests ===
    
    # Test 5: confirmed_leak without owner
    def setup_leak_no_owner(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: confirmed_leak\nreason: test\nsteps_detected: 5\ntotal_growth_kib: 1000\ngrowth_rate_kib_per_min: 10\n")
    test("confirmed_leak without owner", False, setup_leak_no_owner)
    
    # Test 6: confirmed_leak with owner=unknown
    def setup_leak_owner_unknown(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: confirmed_leak\nowner: unknown\nreason: test\nsteps_detected: 5\ntotal_growth_kib: 1000\ngrowth_rate_kib_per_min: 10\n")
    test("confirmed_leak owner=unknown", False, setup_leak_owner_unknown)
    
    # Test 7: confirmed_leak with owner but no matching event evidence
    def setup_leak_no_event_evidence(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: confirmed_leak\nowner: heartbeat\nreason: test\nsteps_detected: 5\ntotal_growth_kib: 1000\ngrowth_rate_kib_per_min: 10\n")
    test("confirmed_leak with owner but no event evidence", False, setup_leak_no_event_evidence)
    
    # Test 8: valid confirmed_leak with owner and event evidence
    def setup_valid_leak(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:00:30\t30\theartbeat_tick\theartbeat\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: confirmed_leak\nowner: heartbeat\nreason: test\nsteps_detected: 5\ntotal_growth_kib: 1000\ngrowth_rate_kib_per_min: 10\n")
    test("valid confirmed_leak with event evidence", True, setup_valid_leak)
    
    # === Bounded verdict tests ===
    
    # Test 9: bounded verdict without evidence
    def setup_bounded_no_evidence(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: bounded_warmup_or_allocator_highwater\n")
    test("bounded verdict without evidence", False, setup_bounded_no_evidence)
    
    # Test 10: bounded verdict with excessive growth
    def setup_bounded_excessive_growth(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: bounded_warmup_or_allocator_highwater\nsteps_detected: 2\ntotal_growth_kib: 3000\n")
    test("bounded verdict with excessive growth (>2000 KiB)", False, setup_bounded_excessive_growth)
    
    # Test 11: bounded verdict with excessive steps
    def setup_bounded_excessive_steps(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: bounded_warmup_or_allocator_highwater\nsteps_detected: 15\ntotal_growth_kib: 500\n")
    test("bounded verdict with excessive steps (>10)", False, setup_bounded_excessive_steps)
    
    # Test 12: valid bounded verdict
    def setup_valid_bounded(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: bounded_warmup_or_allocator_highwater\nreason: test\nsteps_detected: 2\ntotal_growth_kib: 100\n")
    test("valid bounded verdict", True, setup_valid_bounded)
    
    # === Inconclusive verdict tests ===
    
    # Test 13: inconclusive with owner-unknown staircase and proper reason
    def setup_inconclusive_staircase_unknown(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: Staircase growth detected but owner is unattributed. Event correlation required.\nsteps_detected: 5\ntotal_growth_kib: 600\ngrowth_rate_kib_per_min: 10\nowner:\n")
    test("inconclusive staircase with owner-unattributed reason", True, setup_inconclusive_staircase_unknown)
    
    # Test 14: inconclusive staircase without proper reason
    def setup_inconclusive_staircase_no_reason(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: Growth pattern unclear\nsteps_detected: 5\ntotal_growth_kib: 600\ngrowth_rate_kib_per_min: 10\n")
    test("inconclusive staircase without owner explanation", False, setup_inconclusive_staircase_no_reason)
    
    # Test 15: valid basic inconclusive
    def setup_valid_inconclusive(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("valid basic inconclusive artifact", True, setup_valid_inconclusive)
    
    # === Event timeline tests ===
    
    # Test 16: event timeline with header only (no data rows)
    def setup_events_header_only(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline header only", False, setup_events_header_only)
    
    # Test 17: event timeline missing lab_started
    def setup_events_no_lab_started(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:30\t30\theartbeat_tick\theartbeat\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline missing lab_started", False, setup_events_no_lab_started)
    
    # Test 18: event timeline missing terminal event
    def setup_events_no_terminal(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:00:30\t30\theartbeat_tick\theartbeat\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline missing terminal event", False, setup_events_no_terminal)
    
    # Test 19: event timeline with wrong column count
    def setup_events_wrong_cols(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline with wrong column count", False, setup_events_wrong_cols)
    
    # Test 20: event timeline with shutdown as terminal
    def setup_events_shutdown_terminal(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tshutdown\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline with shutdown as terminal", True, setup_events_shutdown_terminal)
    
    # Test 21: event timeline with lab_failed as terminal
    def setup_events_lab_failed_terminal(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:00:50\t50\tlab_failed\terror\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline with lab_failed as terminal", True, setup_events_lab_failed_terminal)
    
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
