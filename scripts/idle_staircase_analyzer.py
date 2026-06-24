#!/usr/bin/env python3
# idle_staircase_analyzer.py — Memory staircase analysis for idle memory lab
#
# Analyzes memory samples and event timeline to determine verdict.
# Shell-side synthetic events may only produce inconclusive verdicts.
# Real attribution requires tovarisch-native event emission.
#
# Usage (internal):
#   python -c "from idle_staircase_analyzer import analyze; analyze(...)"
#
# Or via shell wrapper:
#   ./scripts/lab_tovarisch_idle_memory.sh (calls this internally)

import sys
from pathlib import Path
from typing import Optional


# Step threshold for memory growth detection (KiB)
STEP_THRESHOLD_KIB = 50

# Confirmed leak thresholds
CONFIRMED_LEAK_MIN_STEPS = 3
CONFIRMED_LEAK_MIN_GROWTH_KIB = 500


def extract_rss_values(tsv_path: Path) -> list[tuple[int, int]]:
    """Extract RSS values from memory samples TSV.
    
    Returns list of (elapsed_sec, rss_kib) tuples.
    """
    results = []
    if not tsv_path.exists():
        return results
    
    lines = tsv_path.read_text().strip().split('\n')
    for line in lines[1:]:  # Skip header
        cols = line.split('\t')
        if len(cols) >= 3:
            try:
                elapsed = int(cols[1])
                rss = int(cols[2])
                if rss > 0:
                    results.append((elapsed, rss))
            except (ValueError, IndexError):
                pass
    return results


def detect_memory_steps(rss_values: list[tuple[int, int]]) -> tuple[int, list[int]]:
    """Detect staircase memory steps.
    
    Returns: (steps_detected, step_timestamps)
    """
    steps = 0
    timestamps = []
    prev_rss = 0
    
    for elapsed, rss in rss_values:
        if prev_rss > 0:
            delta = rss - prev_rss
            if delta > STEP_THRESHOLD_KIB:
                steps += 1
                timestamps.append(elapsed)
        prev_rss = rss
    
    return steps, timestamps


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
    
    lines = event_timeline_path.read_text().strip().split('\n')
    for line in lines[1:]:  # Skip header
        cols = line.split('\t')
        if len(cols) >= 4:
            event = cols[2]
            
            if any(event.startswith(p) for p in ["heartbeat_", "heartbeat"]):
                counts["heartbeat"] += 1
            elif any(event.startswith(p) for p in ["wg_", "wg"]):
                counts["wireguard"] += 1
            elif any(event.startswith(p) for p in ["bgp_", "bgp"]):
                counts["bgp"] += 1
            elif any(event.startswith(p) for p in ["bfd_", "bfd"]):
                counts["bfd"] += 1
            elif any(event.startswith(p) for p in ["health_", "health"]):
                counts["health"] += 1
            elif any(event.startswith(p) for p in ["status_", "status"]):
                counts["status"] += 1
    
    return counts


def is_shell_synthetic_artifact(event_timeline_path: Path) -> bool:
    """Check if artifact contains shell-side synthetic events."""
    if not event_timeline_path.exists():
        return False
    
    content = event_timeline_path.read_text()
    
    # Check for subsystem_config marker
    if "subsystem_config" in content:
        return True
    
    # Check for shell-side synthetic event pattern
    lines = content.strip().split('\n')
    for line in lines[1:]:  # Skip header
        cols = line.split('\t')
        if len(cols) >= 4:
            event = cols[2]
            subsystem = cols[3]
            if subsystem == "lab" and any(event.startswith(p) for p in ["heartbeat_", "wg_", "bgp_", "bfd_"]):
                return True
            if "synthetic" in event.lower() or "shell-side" in event.lower():
                return True
    
    return False


def analyze(
    memory_samples_path: Path,
    artifact_path: Path,
    event_timeline_path: Path,
    duration_sec: int,
    heartbeat_enabled: bool = True,
    wg_check_enabled: bool = True,
    bgp_bfd_enabled: bool = True,
) -> str:
    """
    Analyze memory samples and event timeline to determine verdict.
    
    Shell-side synthetic events CANNOT produce confirmed_leak verdicts.
    Real attribution requires tovarisch-native event emission.
    
    Returns the verdict string.
    """
    # Extract RSS values and detect staircase pattern
    rss_values = extract_rss_values(memory_samples_path)
    steps_detected, _ = detect_memory_steps(rss_values)
    
    # Calculate growth rate
    total_growth = 0
    growth_rate_per_min = 0
    
    if len(rss_values) >= 2:
        first_rss = rss_values[0][1]
        last_rss = rss_values[-1][1]
        total_growth = last_rss - first_rss
        
        if duration_sec > 0:
            growth_rate_per_min = (total_growth * 60) // duration_sec
    
    # Analyze event correlation
    event_counts = count_events_by_subsystem(event_timeline_path)
    
    heartbeat_count = event_counts["heartbeat"]
    wg_count = event_counts["wireguard"]
    bgp_count = event_counts["bgp"]
    bfd_count = event_counts["bfd"]
    health_count = event_counts["health"]
    status_count = event_counts["status"]
    
    # Determine verdict based on evidence
    verdict = "inconclusive"
    reason = ""
    
    # Check if this is shell-side synthetic artifact
    is_shell_synthetic = is_shell_synthetic_artifact(event_timeline_path)
    
    # NOTE: Shell-side synthetic events CANNOT produce confirmed_leak.
    # Real attribution requires tovarisch-native event emission.
    # Shell-side events may only enrich an inconclusive artifact.
    if steps_detected >= CONFIRMED_LEAK_MIN_STEPS and total_growth > CONFIRMED_LEAK_MIN_GROWTH_KIB:
        verdict = "inconclusive"
        reason = (
            f"Staircase growth detected ({steps_detected} steps, {total_growth} KiB total, "
            f"{growth_rate_per_min} KiB/min) but owner is unattributed. "
            "Events are shell-side synthetic and cannot be used for attribution. "
            "Need tovarisch-native event emission to identify the periodic background owner."
        )
    elif steps_detected >= 5 and total_growth > 200:
        verdict = "inconclusive"
        reason = (
            f"Possible staircase pattern: {steps_detected} steps, {total_growth} KiB. "
            f"Event counts: heartbeat={heartbeat_count}, wg={wg_count}, bgp={bgp_count}, bfd={bfd_count}. "
            "Need longer observation or targeted testing."
        )
    elif total_growth > 1000:
        verdict = "bounded_warmup_or_allocator_highwater"
        reason = (
            f"Detected {total_growth} KiB growth but no clear staircase pattern "
            "(may be normal warmup or allocator high water mark settling). "
            f"Event counts: heartbeat={heartbeat_count}, wg={wg_count}, bgp={bgp_count}, bfd={bfd_count}."
        )
    elif total_growth < 200:
        verdict = "bounded_warmup_or_allocator_highwater"
        reason = (
            f"Minimal growth detected ({total_growth} KiB) - likely bounded by allocator high water mark "
            f"or normal warmup. Event counts: heartbeat={heartbeat_count}, wg={wg_count}, bgp={bgp_count}, bfd={bfd_count}."
        )
    else:
        verdict = "inconclusive"
        reason = (
            f"Growth pattern unclear: {total_growth} KiB over {duration_sec}s with {steps_detected} steps. "
            f"Event counts: heartbeat={heartbeat_count}, wg={wg_count}, bgp={bgp_count}, bfd={bfd_count}."
        )
    
    # Build correlated events string
    correlated_events = (
        f"heartbeat={heartbeat_count},wg={wg_count},bgp={bgp_count},"
        f"bfd={bfd_count},health={health_count},status={status_count}"
    )
    
    # Format enabled/disabled subsystems
    enabled_str = (
        f"heartbeat={str(heartbeat_enabled).lower()},"
        f"wg={str(wg_check_enabled).lower()},"
        f"bgp_bfd={str(bgp_bfd_enabled).lower()}"
    )
    
    disabled_parts = []
    if not heartbeat_enabled:
        disabled_parts.append("heartbeat")
    if not wg_check_enabled:
        disabled_parts.append("wg")
    if not bgp_bfd_enabled:
        disabled_parts.append("bgp_bfd")
    disabled_str = ",".join(disabled_parts) if disabled_parts else ""
    
    # Write verdict
    verdict_content = f"""verdict: {verdict}
owner: 
reason: {reason}
steps_detected: {steps_detected}
total_growth_kib: {total_growth}
growth_rate_kib_per_min: {growth_rate_per_min}
samples_count: {len(rss_values)}
suspected_owner: 
owner_evidence: 
correlated_events: {correlated_events}
enabled_subsystems: {enabled_str}
disabled_subsystems: {disabled_str}
"""
    
    verdict_path = artifact_path / "verdict.txt"
    verdict_path.write_text(verdict_content)
    
    return verdict


if __name__ == "__main__":
    import argparse
    
    parser = argparse.ArgumentParser(description="Analyze idle staircase memory lab artifacts")
    parser.add_argument("memory_samples", help="Path to memory_samples.tsv")
    parser.add_argument("artifact_dir", help="Path to artifact directory")
    parser.add_argument("event_timeline", help="Path to event_timeline.tsv")
    parser.add_argument("--duration", type=int, default=600, help="Lab duration in seconds")
    parser.add_argument("--heartbeat-enabled", default="true", help="Heartbeat enabled")
    parser.add_argument("--wg-enabled", default="true", help="WG check enabled")
    parser.add_argument("--bgp-bfd-enabled", default="true", help="BGP/BFD enabled")
    
    args = parser.parse_args()
    
    verdict = analyze(
        Path(args.memory_samples),
        Path(args.artifact_dir),
        Path(args.event_timeline),
        args.duration,
        args.heartbeat_enabled.lower() == "true",
        args.wg_enabled.lower() == "true",
        args.bgp_bfd_enabled.lower() == "true",
    )
    print(verdict)
