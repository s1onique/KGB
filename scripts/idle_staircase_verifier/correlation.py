# correlation.py — Memory step extraction and event correlation logic
"""Memory step detection and event correlation for attribution."""

import re
from pathlib import Path
from typing import Optional

from .schema import (
    STEP_THRESHOLD_KIB,
    OWNER_TO_PREFIXES,
    SUBSYSTEM_KEYS,
)


def extract_rss_values(tsv_path: Path) -> list[tuple[int, int]]:
    """
    Extract RSS values from memory samples TSV.
    
    Returns list of (elapsed_sec, rss_kib) tuples.
    """
    results = []
    if not tsv_path.exists():
        return results
    
    lines = tsv_path.read_text().strip().split('\n')
    # Skip header
    for line in lines[1:]:
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
    """
    Detect staircase memory steps.
    
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
    counts = {k: 0 for k in SUBSYSTEM_KEYS}
    
    if not event_timeline_path.exists():
        return counts
    
    lines = event_timeline_path.read_text().strip().split('\n')
    # Skip header
    for line in lines[1:]:
        cols = line.split('\t')
        if len(cols) >= 4:
            event = cols[2]
            subsystem = cols[3]
            
            # Match event patterns to subsystem
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


def find_correlated_events(
    event_timeline_path: Path,
    memory_samples_path: Path,
    owner: str,
    correlation_window_sec: int = 30
) -> Optional[str]:
    """
    Find events from the specified owner subsystem that correlate with memory steps.
    
    Returns a description of the correlation if found, None otherwise.
    """
    # Get memory step timestamps
    rss_values = extract_rss_values(memory_samples_path)
    _, step_timestamps = detect_memory_steps(rss_values)
    
    if not step_timestamps:
        return None
    
    # Get event prefixes for owner
    prefixes = OWNER_TO_PREFIXES.get(owner.lower(), [])
    if not prefixes:
        return None
    
    # Parse event timeline
    if not event_timeline_path.exists():
        return None
    
    lines = event_timeline_path.read_text().strip().split('\n')
    correlated = []
    
    # Skip header
    for line in lines[1:]:
        cols = line.split('\t')
        if len(cols) >= 3:
            try:
                event = cols[2]
                elapsed = int(cols[1])
                
                if any(event.startswith(p) for p in prefixes):
                    # Check if near a memory step
                    for step_ts in step_timestamps:
                        if abs(elapsed - step_ts) <= correlation_window_sec:
                            correlated.append((elapsed, event))
                            break
            except (ValueError, IndexError):
                pass
    
    if correlated:
        return f"{len(correlated)} events correlated with memory steps"
    
    return None


def is_shell_synthetic_artifact(event_timeline_path: Path) -> bool:
    """
    Check if artifact contains shell-side synthetic events.
    
    Shell-side synthetic events have subsystem="lab" with heartbeat_/wg_/bgp_/bfd_ prefixes,
    or explicit subsystem_config markers.
    """
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


def correlated_events_to_string(counts: dict[str, int]) -> str:
    """Format subsystem event counts as correlation string."""
    parts = [f"{k}={counts[k]}" for k in SUBSYSTEM_KEYS if counts[k] > 0 or k in ["heartbeat", "wireguard", "bgp", "bfd"]]
    return ",".join(parts) if parts else ""
