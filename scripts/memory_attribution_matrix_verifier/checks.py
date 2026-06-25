# checks.py — Verification checks

import re
from pathlib import Path
from typing import Tuple

CANONICAL_VARIANTS = [
    "all_enabled",
    "heartbeat_disabled",
    "wg_disabled",
    "bgp_disabled",
    "bfd_disabled",
    "bgp_bfd_disabled",
    "no_periodic",
]


def read_file(path):
    try:
        return path.read_text()
    except FileNotFoundError:
        return None
    except Exception as e:
        import sys
        print(f"Warning: Could not read {path}: {e}", file=sys.stderr)
        return None


def parse_verdict(content):
    result = {"verdict": "unknown"}
    
    # Fix: Use line-bound parsing to avoid capturing across newlines
    # owner: blank should not capture "reason:" from the next line
    
    # Parse line by line to ensure proper line-bound behavior
    lines = content.split('\n')
    for line in lines:
        stripped = line.strip()
        
        if stripped.startswith("verdict:"):
            # Extract verdict value (everything after "verdict:")
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
            # Capture reason - everything on this line after "reason:"
            val = stripped[len("reason:"):].strip()
            if val:
                result["reason"] = val
    
    return result


def parse_manifest(content):
    result = {}
    for line in content.split('\n'):
        stripped = line.strip()
        
        if stripped.startswith("run_id:"):
            result["run_id"] = stripped[len("run_id:"):].strip()
        elif stripped.startswith("platform:"):
            result["platform"] = stripped[len("platform:"):].strip()
        elif stripped.startswith("commit_sha:"):
            result["commit_sha"] = stripped[len("commit_sha:"):].strip()
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
        elif stripped.startswith("duration_seconds:"):
            val = stripped[len("duration_seconds:"):].strip()
            if val.isdigit():
                result["duration_seconds"] = int(val)
    
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
