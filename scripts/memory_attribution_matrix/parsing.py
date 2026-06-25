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
    
    # Fix: Use line-bound parsing to avoid capturing across newlines
    # owner: blank should not capture "reason:" from the next line
    
    # Parse line by line to ensure proper line-bound behavior
    lines = content.split('\n')
    for line in lines:
        stripped = line.strip()
        
        if stripped.startswith("verdict:"):
            val = stripped[len("verdict:"):].strip()
            if val:
                result["verdict"] = val
        
        elif stripped.startswith("owner:"):
            # Extract owner value - everything after "owner:" on THIS line only
            val = stripped[len("owner:"):].strip()
            if val:
                result["owner"] = val
            else:
                result["owner"] = ""  # Explicitly blank
        
        elif stripped.startswith("steps_detected:"):
            val = stripped[len("steps_detected:"):].strip()
            if val.isdigit():
                result["steps_detected"] = int(val)
        
        elif stripped.startswith("total_growth_kib:"):
            val = stripped[len("total_growth_kib:"):].strip()
            if val.isdigit():
                result["total_growth_kib"] = int(val)
        
        elif stripped.startswith("growth_rate_kib_per_min:"):
            val = stripped[len("growth_rate_kib_per_min:"):].strip()
            if val.isdigit():
                result["growth_rate_kib_per_min"] = int(val)
        
        elif stripped.startswith("samples_count:"):
            val = stripped[len("samples_count:"):].strip()
            if val.isdigit():
                result["samples_count"] = int(val)
        
        elif stripped.startswith("reason:"):
            val = stripped[len("reason:"):].strip()
            if val:
                result["reason"] = val
        
        elif stripped.startswith("native_event_counts:"):
            val = stripped[len("native_event_counts:"):].strip()
            if val:
                result["native_event_counts_raw"] = val
        
        elif stripped.startswith("native_correlated:"):
            val = stripped[len("native_correlated:"):].strip()
            if val:
                result["native_correlated_raw"] = val
    
    return result


def parse_manifest(artifact_path: Path) -> dict:
    """Parse manifest.yaml into structured data."""
    manifest_path = artifact_path / "manifest.yaml"
    if not manifest_path.exists():
        return {}
    
    content = manifest_path.read_text()
    result = {}
    
    for line in content.split('\n'):
        stripped = line.strip()
        
        if stripped.startswith("run_id:"):
            result["run_id"] = stripped[len("run_id:"):].strip()
        elif stripped.startswith("platform:"):
            result["platform"] = stripped[len("platform:"):].strip()
        elif stripped.startswith("commit_sha:"):
            result["commit_sha"] = stripped[len("commit_sha:"):].strip()
        elif stripped.startswith("duration_seconds:"):
            val = stripped[len("duration_seconds:"):].strip()
            if val.isdigit():
                result["duration_seconds"] = int(val)
        elif stripped.startswith("sample_interval_seconds:"):
            val = stripped[len("sample_interval_seconds:"):].strip()
            if val.isdigit():
                result["sample_interval_seconds"] = int(val)
        elif stripped.startswith("native_events_enabled:"):
            result["native_events_enabled"] = stripped[len("native_events_enabled:"):].strip().lower() == "true"
        elif stripped.startswith("native_disable_heartbeat:"):
            result["native_disable_heartbeat"] = stripped[len("native_disable_heartbeat:"):].strip().lower() == "true"
        elif stripped.startswith("native_disable_wg_checks:"):
            result["native_disable_wg_checks"] = stripped[len("native_disable_wg_checks:"):].strip().lower() == "true"
        elif stripped.startswith("native_disable_bgp:"):
            result["native_disable_bgp"] = stripped[len("native_disable_bgp:"):].strip().lower() == "true"
        elif stripped.startswith("native_disable_bfd:"):
            result["native_disable_bfd"] = stripped[len("native_disable_bfd:"):].strip().lower() == "true"
    
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
