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


def parse_native_events(native_timeline_path: Path) -> list[dict]:
    """Parse native event timeline into structured records.
    
    Returns list of dicts with: elapsed_millis, event, subsystem, detail
    """
    events = []
    if not native_timeline_path.exists():
        return events
    
    lines = native_timeline_path.read_text().strip().split('\n')
    for line in lines[1:]:  # Skip header
        cols = line.split('\t')
        if len(cols) >= 4:
            try:
                events.append({
                    'elapsed_millis': int(cols[1]) if len(cols) > 1 else 0,
                    'event': cols[2] if len(cols) > 2 else '',
                    'subsystem': cols[3] if len(cols) > 3 else '',
                    'detail': cols[4] if len(cols) > 4 else '',
                })
            except (ValueError, IndexError):
                pass
    
    return events


def count_native_events_by_subsystem(native_events: list[dict]) -> dict[str, int]:
    """Count native events by subsystem."""
    counts = {
        "heartbeat": 0,
        "wireguard": 0,
        "bgp": 0,
        "bfd": 0,
        "health": 0,
        "status": 0,
    }
    
    subsystem_map = {
        "heartbeat": "heartbeat",
        "wireguard": "wireguard",
        "bgp": "bgp",
        "bfd": "bfd",
        "health": "health",
        "status": "status",
    }
    
    for event in native_events:
        subsystem = event.get('subsystem', '').lower()
        if subsystem in subsystem_map:
            counts[subsystem_map[subsystem]] += 1
    
    return counts


def correlate_native_events_with_steps(
    native_events: list[dict],
    step_timestamps: list[int],
    window_sec: int = 30
) -> dict[str, int]:
    """Correlate native events with memory steps within time window.
    
    Returns dict of subsystem -> correlated event count
    """
    correlated = {
        "heartbeat": 0,
        "wireguard": 0,
        "bgp": 0,
        "bfd": 0,
        "health": 0,
        "status": 0,
    }
    
    if not step_timestamps:
        return correlated
    
    for event in native_events:
        elapsed_millis = event.get('elapsed_millis', 0)
        elapsed_sec = elapsed_millis // 1000
        subsystem = event.get('subsystem', '').lower()
        
        # Check if event is near any memory step
        for step_ts in step_timestamps:
            if abs(elapsed_sec - step_ts) <= window_sec:
                if subsystem in correlated:
                    correlated[subsystem] += 1
                break
    
    return correlated


def analyze(
    memory_samples_path: Path,
    artifact_path: Path,
    event_timeline_path: Path,
    duration_sec: int,
    heartbeat_enabled: bool = True,
    wg_check_enabled: bool = True,
    bgp_bfd_enabled: bool = True,
    native_events_enabled: bool = False,
    native_event_timeline_path: Optional[Path] = None,
) -> str:
    """
    Analyze memory samples and event timeline to determine verdict.
    
    Shell-side synthetic events CANNOT produce confirmed_leak verdicts.
    Real attribution requires tovarisch-native event emission.
    
    Returns the verdict string.
    """
    # Extract RSS values and detect staircase pattern
    rss_values = extract_rss_values(memory_samples_path)
    steps_detected, step_timestamps = detect_memory_steps(rss_values)
    
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
    
    # Parse native events if available
    native_events = []
    native_event_counts = {}
    native_correlated = {}
    
    if native_events_enabled and native_event_timeline_path:
        native_events = parse_native_events(native_event_timeline_path)
        native_event_counts = count_native_events_by_subsystem(native_events)
        native_correlated = correlate_native_events_with_steps(native_events, step_timestamps)
    
    # Determine verdict based on evidence
    verdict = "inconclusive"
    reason = ""
    owner = ""
    owner_evidence = ""
    
    # Check if this is shell-side synthetic artifact
    is_shell_synthetic = is_shell_synthetic_artifact(event_timeline_path)
    
    # Key check: shell-only artifacts cannot produce confirmed_leak
    has_native_events = native_events_enabled and len(native_events) > 0
    
    # Determine if we have enough evidence for confirmed_leak
    if steps_detected >= CONFIRMED_LEAK_MIN_STEPS and total_growth > CONFIRMED_LEAK_MIN_GROWTH_KIB:
        if not has_native_events:
            # Shell-only artifacts cannot produce confirmed_leak
            verdict = "inconclusive"
            reason = (
                f"Staircase growth detected ({steps_detected} steps, {total_growth} KiB total, "
                f"{growth_rate_per_min} KiB/min) but owner is unattributed. "
                "Events are shell-side synthetic and cannot be used for attribution. "
                "Need tovarisch-native event emission to identify the periodic background owner."
            )
        else:
            # Native events available - check for correlation
            # Find dominant subsystem from native events
            dominant_native = max(native_correlated.items(), key=lambda x: x[1])
            dominant_name, dominant_count = dominant_native
            
            if dominant_count > 0:
                # We have native events correlated with memory steps
                # Additional check: total native event count for this subsystem
                total_native_for_subsystem = native_event_counts.get(dominant_name, 0)
                
                if total_native_for_subsystem >= CONFIRMED_LEAK_MIN_STEPS:
                    # Sufficient native evidence
                    verdict = "confirmed_leak"
                    owner = dominant_name
                    owner_evidence = (
                        f"Native events from {owner} subsystem correlate with {dominant_count} memory steps. "
                        f"Total {owner} native events: {total_native_for_subsystem}. "
                        f"Growth pattern: {steps_detected} steps, {total_growth} KiB total."
                    )
                    reason = owner_evidence
                else:
                    verdict = "inconclusive"
                    reason = (
                        f"Native events present but insufficient for attribution. "
                        f"Correlated {dominant_name} events: {dominant_count}, "
                        f"total {dominant_name} events: {total_native_for_subsystem} "
                        f"(need >= {CONFIRMED_LEAK_MIN_STEPS}). "
                        f"Growth: {steps_detected} steps, {total_growth} KiB."
                    )
            else:
                verdict = "inconclusive"
                reason = (
                    f"Native events present but none correlate with memory steps. "
                    f"Growth: {steps_detected} steps, {total_growth} KiB. "
                    f"Need native events near memory step timestamps for attribution."
                )
    elif steps_detected >= 5 and total_growth > 200:
        if has_native_events:
            verdict = "inconclusive"
            reason = (
                f"Possible staircase pattern: {steps_detected} steps, {total_growth} KiB. "
                f"Native events: heartbeat={native_event_counts.get('heartbeat', 0)}, "
                f"wg={native_event_counts.get('wireguard', 0)}, "
                f"bgp={native_event_counts.get('bgp', 0)}, bfd={native_event_counts.get('bfd', 0)}. "
                "Need longer observation or targeted testing."
            )
        else:
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
    
    # Format native event info
    native_info = ""
    if has_native_events:
        native_correlated_str = (
            f"heartbeat={native_correlated.get('heartbeat', 0)},"
            f"wg={native_correlated.get('wireguard', 0)},"
            f"bgp={native_correlated.get('bgp', 0)},"
            f"bfd={native_correlated.get('bfd', 0)}"
        )
        native_counts_str = (
            f"heartbeat={native_event_counts.get('heartbeat', 0)},"
            f"wg={native_event_counts.get('wireguard', 0)},"
            f"bgp={native_event_counts.get('bgp', 0)},"
            f"bfd={native_event_counts.get('bfd', 0)}"
        )
        native_info = (
            f"\nnative_events_enabled: true\n"
            f"native_event_count: {len(native_events)}\n"
            f"native_event_counts: {native_counts_str}\n"
            f"native_correlated: {native_correlated_str}"
        )
    else:
        native_info = "\nnative_events_enabled: false\nnative_event_count: 0"
    
    # Write verdict
    verdict_content = f"""verdict: {verdict}
owner: {owner}
reason: {reason}
steps_detected: {steps_detected}
total_growth_kib: {total_growth}
growth_rate_kib_per_min: {growth_rate_per_min}
samples_count: {len(rss_values)}
suspected_owner: {owner}
owner_evidence: {owner_evidence}
correlated_events: {correlated_events}
enabled_subsystems: {enabled_str}
disabled_subsystems: {disabled_str}{native_info}
"""
    
    verdict_path = artifact_path / "verdict.txt"
    verdict_path.write_text(verdict_content)
    
    return verdict
