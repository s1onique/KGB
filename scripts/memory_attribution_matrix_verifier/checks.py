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
    reason_match = re.search(r'reason:\s*(.+?)(?:\n\n|$)', content, re.DOTALL)
    if reason_match:
        result["reason"] = reason_match.group(1).strip()
    return result


def parse_manifest(content):
    result = {}
    for field in ["run_id", "platform", "commit_sha"]:
        match = re.search(rf'{field}:\s*(.+)', content)
        if match:
            result[field] = match.group(1).strip()
    for field in ["native_events_enabled", "native_disable_heartbeat", 
                  "native_disable_wg_checks", "native_disable_bgp", "native_disable_bfd"]:
        match = re.search(rf'{field}:\s*(.+)', content)
        if match:
            val = match.group(1).strip().lower()
            result[field] = val == "true"
    duration_match = re.search(r'duration_seconds:\s*(\d+)', content)
    if duration_match:
        result["duration_seconds"] = int(duration_match.group(1))
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
