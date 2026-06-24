# artifact_checks.py — Artifact validation checks
"""Artifact component validation functions."""

import re
from pathlib import Path
from typing import Optional

from .schema import (
    REQUIRED_FILES,
    REQUIRED_EVENT_COLS,
    TERMINAL_EVENTS,
    VALID_VERDICTS,
    CONFIRMED_LEAK_MIN_STEPS,
    CONFIRMED_LEAK_MIN_GROWTH_KIB,
    BOUNDED_MAX_GROWTH_KIB,
    BOUNDED_MAX_STEPS,
)
from .correlation import is_shell_synthetic_artifact, find_correlated_events


def verify_manifest(artifact_path: Path) -> tuple[bool, Optional[str]]:
    """Verify manifest.yaml is present and non-empty."""
    manifest_path = artifact_path / "manifest.yaml"
    
    if not manifest_path.exists():
        return False, f"manifest.yaml missing in {artifact_path}"
    
    content = manifest_path.read_text()
    if len(content.strip()) < 10:
        return False, "manifest.yaml is empty or too small"
    
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
    
    header = lines[0]
    required_cols = ["timestamp", "elapsed_sec", "rss_kib", "vmdata_kib"]
    for col in required_cols:
        if col not in header:
            return False, f"memory_samples.tsv missing required column: {col}"
    
    num_cols = len(header.split('\t'))
    for i, line in enumerate(lines[1:], start=2):
        cols = line.split('\t')
        if len(cols) != num_cols:
            return False, f"memory_samples.tsv line {i} has wrong column count: {len(cols)} vs {num_cols}"
        
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
    
    header = lines[0]
    header_cols = header.split('\t')
    for col in REQUIRED_EVENT_COLS:
        if col not in header_cols:
            return False, f"event_timeline.tsv missing required column: {col}"
    
    num_cols = len(header_cols)
    
    if len(lines) < 2:
        return False, "event_timeline.tsv has no data rows (header only)"
    
    has_lab_started = False
    has_terminal = False
    
    for i, line in enumerate(lines[1:], start=2):
        cols = line.split('\t')
        
        if len(cols) != num_cols:
            return False, f"event_timeline.tsv line {i} has wrong column count: {len(cols)} vs {num_cols}"
        
        if len(cols) >= 3:
            event = cols[2]
            
            if event == "lab_started":
                has_lab_started = True
            
            if event in TERMINAL_EVENTS:
                has_terminal = True
    
    if not has_lab_started:
        return False, "event_timeline.tsv missing required 'lab_started' event"
    
    if not has_terminal:
        return False, f"event_timeline.tsv missing terminal event (one of: {', '.join(TERMINAL_EVENTS)})"
    
    return True, None


def verify_verdict(artifact_path: Path) -> tuple[bool, Optional[str]]:
    """Verify verdict.txt has valid verdict and proper attribution."""
    verdict_path = artifact_path / "verdict.txt"
    
    if not verdict_path.exists():
        return False, "verdict.txt missing"
    
    content = verdict_path.read_text()
    
    verdict_match = re.search(r'verdict:\s*(\S+)', content)
    if not verdict_match:
        return False, "verdict.txt missing 'verdict:' field"
    
    verdict = verdict_match.group(1)
    if verdict not in VALID_VERDICTS:
        return False, f"verdict.txt has invalid verdict: {verdict}"
    
    owner_match = re.search(r'owner:\s*(\S+)', content)
    owner = owner_match.group(1) if owner_match else ""
    
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
        
        if not re.search(r'steps_detected:', content):
            return False, "verdict=confirmed_leak requires steps_detected evidence"
        
        if not re.search(r'total_growth_kib:', content):
            return False, "verdict=confirmed_leak requires total_growth_kib evidence"
        
        if not re.search(r'growth_rate_kib_per_min:', content):
            return False, "verdict=confirmed_leak requires growth_rate_kib_per_min evidence"
        
        steps_match = re.search(r'steps_detected:\s*(\d+)', content)
        growth_match = re.search(r'total_growth_kib:\s*(\d+)', content)
        
        if not steps_match:
            return False, "verdict=confirmed_leak requires steps_detected field"
        
        if not growth_match:
            return False, "verdict=confirmed_leak requires total_growth_kib field"
        
        steps_detected = int(steps_match.group(1))
        total_growth_kib = int(growth_match.group(1))
        
        if steps_detected < CONFIRMED_LEAK_MIN_STEPS:
            return False, f"verdict=confirmed_leak requires steps_detected >= {CONFIRMED_LEAK_MIN_STEPS}, got {steps_detected}"
        
        if total_growth_kib <= CONFIRMED_LEAK_MIN_GROWTH_KIB:
            return False, f"verdict=confirmed_leak requires total_growth_kib > {CONFIRMED_LEAK_MIN_GROWTH_KIB}, got {total_growth_kib}"
        
        if not owner_evidence:
            return False, "verdict=confirmed_leak requires non-empty owner_evidence field"
        
        if not correlated_events:
            return False, "verdict=confirmed_leak requires non-empty correlated_events field"
        
        # CRITICAL: Reject confirmed_leak if shell-side synthetic events present
        event_timeline_path = artifact_path / "event_timeline.tsv"
        if event_timeline_path.exists() and is_shell_synthetic_artifact(event_timeline_path):
            return False, "verdict=confirmed_leak rejected: artifact contains shell-side synthetic events. Shell-side events may enrich inconclusive artifacts but cannot produce confirmed_leak. Real attribution requires tovarisch-native event emission."
        
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
        
        growth_match = re.search(r'total_growth_kib:\s*(\d+)', content)
        steps_match = re.search(r'steps_detected:\s*(\d+)', content)
        
        if growth_match:
            growth = int(growth_match.group(1))
            if growth > BOUNDED_MAX_GROWTH_KIB:
                return False, f"verdict=bounded but total_growth_kib > {BOUNDED_MAX_GROWTH_KIB}, seems like continued growth"
        
        if steps_match:
            steps = int(steps_match.group(1))
            if steps > BOUNDED_MAX_STEPS:
                return False, f"verdict=bounded but steps_detected > {BOUNDED_MAX_STEPS}, seems like continued staircase"
    
    # For inconclusive verdict with staircase growth, require reason explaining unattribution
    if verdict == "inconclusive":
        steps_match = re.search(r'steps_detected:\s*(\d+)', content)
        growth_match = re.search(r'total_growth_kib:\s*(\d+)', content)
        
        if steps_match and growth_match:
            steps = int(steps_match.group(1))
            growth = int(growth_match.group(1))
            
            if steps >= CONFIRMED_LEAK_MIN_STEPS and growth > CONFIRMED_LEAK_MIN_GROWTH_KIB and owner in ["", "unknown"]:
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
    
    for filename in REQUIRED_FILES:
        filepath = artifact_path / filename
        if not filepath.exists():
            return False, f"Required file missing: {filename}"
    
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
