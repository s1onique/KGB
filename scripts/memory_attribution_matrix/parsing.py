# parsing.py — Verdict and manifest parsing

import re
from pathlib import Path
from typing import Optional


def parse_verdict(artifact_path: Path) -> dict:
    """Parse verdict.txt into structured data."""
    verdict_path = artifact_path / "verdict.txt"
    if not verdict_path.exists():
        return {"verdict": "unknown", "error": "verdict.txt missing"}
    
    content = verdict_path.read_text()
    result = {"verdict": "unknown", "raw": content}
    
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
    
    reason_match = re.search(r'reason:\s*(.+?)(?:\n|$)', content, re.DOTALL)
    if reason_match:
        result["reason"] = reason_match.group(1).strip()
    
    native_counts_match = re.search(r'native_event_counts:\s*(.+?)(?:\n|$)', content)
    if native_counts_match:
        result["native_event_counts_raw"] = native_counts_match.group(1).strip()
    
    native_correlated_match = re.search(r'native_correlated:\s*(.+?)(?:\n|$)', content)
    if native_correlated_match:
        result["native_correlated_raw"] = native_correlated_match.group(1).strip()
    
    return result


def parse_manifest(artifact_path: Path) -> dict:
    """Parse manifest.yaml into structured data."""
    manifest_path = artifact_path / "manifest.yaml"
    if not manifest_path.exists():
        return {}
    
    content = manifest_path.read_text()
    result = {}
    
    for field in ["run_id", "platform", "commit_sha", "duration_seconds", "sample_interval_seconds"]:
        match = re.search(rf'{field}:\s*(.+)', content)
        if match:
            result[field] = match.group(1).strip()
    
    for field in ["native_events_enabled", "native_disable_heartbeat", "native_disable_wg_checks", 
                  "native_disable_bgp", "native_disable_bfd"]:
        match = re.search(rf'{field}:\s*(.+)', content)
        if match:
            val = match.group(1).strip().lower()
            result[field] = val == "true"
    
    return result


def count_native_events(artifact_path: Path) -> dict:
    """Count native events by subsystem from native_event_timeline.tsv."""
    native_path = artifact_path / "native_event_timeline.tsv"
    counts = {
        "heartbeat": 0,
        "wireguard": 0,
        "bgp": 0,
        "bfd": 0,
        "health": 0,
        "status": 0,
        "total": 0,
    }
    
    if not native_path.exists():
        return counts
    
    lines = native_path.read_text().strip().split('\n')
    for line in lines[1:]:
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
            
            counts["total"] += 1
    
    return counts
