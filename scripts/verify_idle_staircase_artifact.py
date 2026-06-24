#!/usr/bin/env python3
# verify_idle_staircase_artifact.py — Verifier for idle staircase memory lab artifacts
#
# ACT: Attribute actual tovarisch idle staircase memory owner
#
# Verifies that a memory lab artifact is complete and well-formed.
# Rejects artifacts that are missing required files, have malformed data,
# or claim confirmed_leak without proper attribution.
#
# Enhanced verification for owner attribution:
# - confirmed_leak requires non-empty owner (not "unknown")
# - confirmed_leak requires memory step evidence
# - confirmed_leak requires event timeline evidence for that owner
# - confirmed_leak requires owner evidence text
# - confirmed_leak requires at least one correlated event near a memory step,
#   OR a targeted red/green test reference
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

# Event types for attribution
HEARTBEAT_EVENTS = ["heartbeat_tick", "heartbeat_tick_start", "heartbeat_tick_end", "heartbeat_emit"]
WG_EVENTS = ["wg_check", "wg_check_start", "wg_check_failed", "wg_check_end", "wg_show"]
BGP_EVENTS = ["bgp_maintenance", "bgp_maintenance_start", "bgp_maintenance_end", "bgp_reconnect"]
BFD_EVENTS = ["bfd_tick", "bfd_tick_start", "bfd_tick_end"]
STATUS_EVENTS = ["status_burst", "status_burst_start", "status_burst_complete"]


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


def count_events_by_subsystem(event_timeline_path: Path) -> dict[str, int]:
    """Count events by subsystem from event timeline."""
    counts = {
        "heartbeat": 0,
        "wireguard": 0,
        "bgp": 0,
        "bfd": 0,
        "health": 0,
        "status": 0,
    }
    
    if not event_timeline_path.exists():
        return counts
    
    content = event_timeline_path.read_text()
    lines = content.strip().split('\n')
    
    for line in lines[1:]:  # Skip header
        cols = line.split('\t')
        if len(cols) >= 4:
            event = cols[2]
            subsystem = cols[3]
            
            # Match event patterns
            if any(event.startswith(prefix) for prefix in ["heartbeat_", "heartbeat"]):
                counts["heartbeat"] += 1
            elif any(event.startswith(prefix) for prefix in ["wg_", "wg"]):
                counts["wireguard"] += 1
            elif any(event.startswith(prefix) for prefix in ["bgp_", "bgp"]):
                counts["bgp"] += 1
            elif any(event.startswith(prefix) for prefix in ["bfd_", "bfd"]):
                counts["bfd"] += 1
            elif any(event.startswith(prefix) for prefix in ["health_", "health"]):
                counts["health"] += 1
            elif any(event.startswith(prefix) for prefix in ["status_", "status"]):
                counts["status"] += 1
    
    return counts


def find_correlated_events(
    event_timeline_path: Path,
    memory_samples_path: Path,
    owner: str
) -> Optional[str]:
    """
    Find events from the specified owner subsystem that correlate with memory steps.
    
    Returns a description of the correlation if found, None otherwise.
    
    A correlation exists if:
    - Owner has events in the timeline
    - At least one event timestamp aligns with a memory step (within step threshold)
    """
    if not event_timeline_path.exists() or not memory_samples_path.exists():
        return None
    
    # Parse memory samples to find step timestamps
    mem_lines = memory_samples_path.read_text().strip().split('\n')
    step_timestamps = set()
    
    prev_rss = None
    for line in mem_lines[1:]:  # Skip header
        cols = line.split('\t')
        if len(cols) >= 3:
            try:
                elapsed = int(cols[1])
                rss = int(cols[2])
                
                if prev_rss is not None:
                    delta = rss - prev_rss
                    if delta > 50:  # Step threshold
                        step_timestamps.add(elapsed)
                
                prev_rss = rss
            except (ValueError, IndexError):
                pass
    
    if not step_timestamps:
        return None
    
    # Parse event timeline
    event_lines = event_timeline_path.read_text().strip().split('\n')
    
    # Map owner string to event prefixes
    owner_prefixes = {
        "heartbeat": ["heartbeat_", "heartbeat"],
        "wireguard": ["wg_", "wg"],
        "bgp": ["bgp_", "bgp"],
        "bfd": ["bfd_", "bfd"],
    }
    
    prefixes = owner_prefixes.get(owner.lower(), [])
    if not prefixes:
        return None
    
    # Find events from owner subsystem near memory steps
    correlated = []
    for line in event_lines[1:]:  # Skip header
        cols = line.split('\t')
        if len(cols) >= 3:
            event = cols[2]
            elapsed = int(cols[1])
            
            if any(event.startswith(p) for p in prefixes):
                # Check if this event is near a memory step (within 30 seconds)
                for step_ts in step_timestamps:
                    if abs(elapsed - step_ts) <= 30:
                        correlated.append((elapsed, event))
                        break
    
    if correlated:
        return f"{len(correlated)} events correlated with memory steps"
    
    return None


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
    
    # Extract additional attribution fields
    suspected_owner_match = re.search(r'suspected_owner:\s*(\S+)', content)
    suspected_owner = suspected_owner_match.group(1) if suspected_owner_match else ""
    
    owner_evidence_match = re.search(r'owner_evidence:\s*(.+?)(?:\n|$)', content, re.DOTALL)
    owner_evidence = owner_evidence_match.group(1).strip() if owner_evidence_match else ""
    
    correlated_events_match = re.search(r'correlated_events:\s*(.+?)(?:\n|$)', content, re.DOTALL)
    correlated_events = correlated_events_match.group(1).strip() if correlated_events_match else ""
    
    # For confirmed_leak, require enhanced attribution
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
        
        # CRITICAL: Enforce documented thresholds for confirmed_leak
        # confirmed_leak requires steps_detected >= 3 AND total_growth_kib > 500
        steps_match = re.search(r'steps_detected:\s*(\d+)', content)
        growth_match = re.search(r'total_growth_kib:\s*(\d+)', content)
        
        if not steps_match:
            return False, "verdict=confirmed_leak requires steps_detected field"
        
        if not growth_match:
            return False, "verdict=confirmed_leak requires total_growth_kib field"
        
        steps_detected = int(steps_match.group(1))
        total_growth_kib = int(growth_match.group(1))
        
        if steps_detected < 3:
            return False, f"verdict=confirmed_leak requires steps_detected >= 3, got {steps_detected}"
        
        if total_growth_kib <= 500:
            return False, f"verdict=confirmed_leak requires total_growth_kib > 500, got {total_growth_kib}"
        
        # Require owner evidence text
        if not owner_evidence:
            return False, "verdict=confirmed_leak requires non-empty owner_evidence field"
        
        # Require correlated events evidence
        if not correlated_events:
            return False, "verdict=confirmed_leak requires non-empty correlated_events field"
        
        # CRITICAL: Reject confirmed_leak if ANY shell-side synthetic events are present
        # Shell-side synthetic events are on fixed intervals and cannot be used for attribution
        event_timeline_path = artifact_path / "event_timeline.tsv"
        if event_timeline_path.exists():
            timeline_content = event_timeline_path.read_text()
            
            # Check for shell-side synthetic event markers
            has_subsystem_config = "subsystem_config" in timeline_content
            has_synthetic_events = False
            
            # Check if timeline has shell-side synthetic events (heartbeat_tick, wg_check, etc.)
            synthetic_event_prefixes = ["heartbeat_", "wg_", "bgp_", "bfd_"]
            lines = timeline_content.strip().split('\n')
            for line in lines[1:]:  # Skip header
                cols = line.split('\t')
                if len(cols) >= 3:
                    event = cols[2]
                    subsystem = cols[3]
                    # Shell-side synthetic events have lab subsystem or fixed-interval patterns
                    if subsystem == "lab" and any(event.startswith(p) for p in synthetic_event_prefixes):
                        has_synthetic_events = True
                        break
                    # Also check for explicit synthetic markers
                    if "synthetic" in event.lower() or "shell-side" in event.lower():
                        has_synthetic_events = True
                        break
            
            # If ANY shell-side synthetic events are present, reject confirmed_leak
            if has_subsystem_config or has_synthetic_events:
                return False, "verdict=confirmed_leak rejected: artifact contains shell-side synthetic events. Shell-side events may enrich inconclusive artifacts but cannot produce confirmed_leak. Real attribution requires tovarisch-native event emission."
            
            # Check if any event/subsystem correlates with owner
            owner_lower = owner.lower()
            if owner_lower not in timeline_content.lower():
                return False, f"verdict=confirmed_leak with owner='{owner}' requires matching event evidence in timeline"
            
            # Check for correlated events near memory steps
            correlation = find_correlated_events(
                event_timeline_path,
                artifact_path / "memory_samples.tsv",
                owner
            )
            
            if not correlation:
                return False, f"verdict=confirmed_leak with owner='{owner}' requires at least one correlated event near a memory step, or targeted red/green test reference"
    
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
    
    # === Confirmed leak tests (enhanced attribution) ===
    
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
        (p / "verdict.txt").write_text("verdict: confirmed_leak\nowner: heartbeat\nreason: test\nsteps_detected: 5\ntotal_growth_kib: 1000\ngrowth_rate_kib_per_min: 10\nowner_evidence: test evidence\ncorrelated_events: heartbeat=0\n")
    test("confirmed_leak with owner but no event evidence", False, setup_leak_no_event_evidence)
    
    # Test 8: confirmed_leak with owner and event evidence but no correlated events
    def setup_leak_no_correlated(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: confirmed_leak\nowner: heartbeat\nreason: test\nsteps_detected: 5\ntotal_growth_kib: 1000\ngrowth_rate_kib_per_min: 10\nowner_evidence: test evidence\ncorrelated_events: heartbeat=0\n")
    test("confirmed_leak without correlated events near memory steps", False, setup_leak_no_correlated)
    
    # Test 9: valid confirmed_leak with owner, event evidence, and correlated events
    # NOTE: This test uses heartbeat subsystem WITHOUT shell-side synthetic markers
    # Real tovarisch-native events would have subsystem="heartbeat" without the lab prefix
    # FIXED: Uses documented thresholds: steps_detected >= 3 AND total_growth_kib > 500
    def setup_valid_leak(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        # Memory samples with 3 actual memory steps (each >50 KiB)
        # RSS progression: 1000 -> 1200 (step) -> 1400 (step) -> 1600 (step) = 600 KiB total
        (p / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:00:30\t30\t1200\t2200\n"  # Step +200
            "2024-01-01T00:01:00\t60\t1400\t2400\n"  # Step +200
            "2024-01-01T00:01:30\t90\t1600\t2600\n"  # Step +200
            "2024-01-01T00:02:00\t120\t1600\t2600\n"
        )
        # Event timeline with tovarisch-native heartbeat tick (subsystem=heartbeat, NOT lab)
        # No subsystem_config marker - this is real tovarisch-native instrumentation
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:30\t30\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:01:00\t60\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:01:30\t90\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:02:00\t120\tidle_complete\tlab\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\n"
            "owner: heartbeat\n"
            "reason: Dominant heartbeat subsystem with 3 events correlated with memory steps\n"
            "steps_detected: 3\n"
            "total_growth_kib: 600\n"
            "growth_rate_kib_per_min: 30\n"
            "owner_evidence: Heartbeat events at t=30,60,90 correlate exactly with memory steps at same timestamps\n"
            "correlated_events: heartbeat=3,wg=0,bgp=0,bfd=0,health=0,status=0\n"
        )
    test("valid confirmed_leak with tovarisch-native events", True, setup_valid_leak)
    
    # Test 9a: confirmed_leak rejected for steps_detected < 3
    def setup_leak_steps_below_threshold(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:00:30\t30\t1200\t2200\n"
            "2024-01-01T00:02:00\t120\t1200\t2200\n"
        )
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:30\t30\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:02:00\t120\tidle_complete\tlab\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\n"
            "owner: heartbeat\n"
            "reason: test\n"
            "steps_detected: 2\n"  # Below threshold
            "total_growth_kib: 600\n"
            "growth_rate_kib_per_min: 10\n"
            "owner_evidence: test\n"
            "correlated_events: heartbeat=1\n"
        )
    test("confirmed_leak rejected for steps_detected < 3", False, setup_leak_steps_below_threshold)
    
    # Test 9b: confirmed_leak rejected for total_growth_kib <= 500
    def setup_leak_growth_below_threshold(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:00:30\t30\t1100\t2100\n"
            "2024-01-01T00:01:00\t60\t1200\t2200\n"
            "2024-01-01T00:01:30\t90\t1300\t2300\n"
            "2024-01-01T00:02:00\t120\t1300\t2300\n"
        )
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:30\t30\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:01:00\t60\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:01:30\t90\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:02:00\t120\tidle_complete\tlab\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\n"
            "owner: heartbeat\n"
            "reason: test\n"
            "steps_detected: 3\n"
            "total_growth_kib: 300\n"  # Below threshold (500)
            "growth_rate_kib_per_min: 10\n"
            "owner_evidence: test\n"
            "correlated_events: heartbeat=3\n"
        )
    test("confirmed_leak rejected for total_growth_kib <= 500", False, setup_leak_growth_below_threshold)
    
    # Test 9b: confirmed_leak with shell-side synthetic events MUST be rejected
    def setup_leak_shell_synthetic_rejected(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:00:30\t30\t1100\t2100\n"
            "2024-01-01T00:01:40\t100\t1100\t2100\n"
        )
        # Shell-side synthetic events have subsystem="lab" with heartbeat_ prefix
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:30\t30\theartbeat_tick\tlab\n"
            "2024-01-01T00:01:40\t100\tidle_complete\tlab\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\n"
            "owner: heartbeat\n"
            "reason: test\n"
            "steps_detected: 1\n"
            "total_growth_kib: 100\n"
            "growth_rate_kib_per_min: 10\n"
            "owner_evidence: test\n"
            "correlated_events: heartbeat=1\n"
        )
    test("confirmed_leak with shell-side synthetic events rejected", False, setup_leak_shell_synthetic_rejected)
    
    # === Bounded verdict tests ===
    
    # Test 10: bounded verdict without evidence
    def setup_bounded_no_evidence(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: bounded_warmup_or_allocator_highwater\n")
    test("bounded verdict without evidence", False, setup_bounded_no_evidence)
    
    # Test 11: bounded verdict with excessive growth
    def setup_bounded_excessive_growth(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: bounded_warmup_or_allocator_highwater\nsteps_detected: 2\ntotal_growth_kib: 3000\n")
    test("bounded verdict with excessive growth (>2000 KiB)", False, setup_bounded_excessive_growth)
    
    # Test 12: bounded verdict with excessive steps
    def setup_bounded_excessive_steps(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: bounded_warmup_or_allocator_highwater\nsteps_detected: 15\ntotal_growth_kib: 500\n")
    test("bounded verdict with excessive steps (>10)", False, setup_bounded_excessive_steps)
    
    # Test 13: valid bounded verdict
    def setup_valid_bounded(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: bounded_warmup_or_allocator_highwater\nreason: test\nsteps_detected: 2\ntotal_growth_kib: 100\n")
    test("valid bounded verdict", True, setup_valid_bounded)
    
    # === Inconclusive verdict tests ===
    
    # Test 14: inconclusive with owner-unknown staircase and proper reason
    def setup_inconclusive_staircase_unknown(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: Staircase growth detected but owner is unattributed. Event correlation required.\nsteps_detected: 5\ntotal_growth_kib: 600\ngrowth_rate_kib_per_min: 10\nowner:\n")
    test("inconclusive staircase with owner-unattributed reason", True, setup_inconclusive_staircase_unknown)
    
    # Test 15: inconclusive staircase without proper reason
    def setup_inconclusive_staircase_no_reason(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: Growth pattern unclear\nsteps_detected: 5\ntotal_growth_kib: 600\ngrowth_rate_kib_per_min: 10\n")
    test("inconclusive staircase without owner explanation", False, setup_inconclusive_staircase_no_reason)
    
    # Test 16: valid basic inconclusive
    def setup_valid_inconclusive(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("valid basic inconclusive artifact", True, setup_valid_inconclusive)
    
    # === Event timeline tests ===
    
    # Test 17: event timeline with header only (no data rows)
    def setup_events_header_only(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline header only", False, setup_events_header_only)
    
    # Test 18: event timeline missing lab_started
    def setup_events_no_lab_started(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:30\t30\theartbeat_tick\theartbeat\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline missing lab_started", False, setup_events_no_lab_started)
    
    # Test 19: event timeline missing terminal event
    def setup_events_no_terminal(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:00:30\t30\theartbeat_tick\theartbeat\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline missing terminal event", False, setup_events_no_terminal)
    
    # Test 20: event timeline with wrong column count
    def setup_events_wrong_cols(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline with wrong column count", False, setup_events_wrong_cols)
    
    # Test 21: event timeline with shutdown as terminal
    def setup_events_shutdown_terminal(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tshutdown\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline with shutdown as terminal", True, setup_events_shutdown_terminal)
    
    # Test 22: event timeline with lab_failed as terminal
    def setup_events_lab_failed_terminal(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:00:50\t50\tlab_failed\terror\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline with lab_failed as terminal", True, setup_events_lab_failed_terminal)
    
    # === Summary ===
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
