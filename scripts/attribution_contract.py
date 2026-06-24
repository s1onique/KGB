#!/usr/bin/env python3
"""
Attribution artifact contract validation library.

Validates attribution lab artifacts conform to the expected contract:
- manifest.yaml exists and has required fields
- memstats-{start,midpoint,end}.json with required numeric fields
- heap-{start,midpoint,end}.pprof non-empty heap profiles
- goroutine-{start,midpoint,end}.txt non-empty goroutine dumps
- rss-pss.tsv time-series with timestamp and RSS coverage
- lab-result.json valid JSON object

Reference: kgb://doctrine/embedded-memory-frugality
"""

import json
import os


def is_attribution_dir(path):
    """Check if a directory is an attribution lab artifact directory."""
    return os.path.isdir(path) and os.path.exists(os.path.join(path, "manifest.yaml"))


def validate_manifest(path):
    """Validate manifest.yaml has required fields."""
    errors = []
    
    if not os.path.exists(path):
        return [f"manifest.yaml does not exist: {path}"]
    
    try:
        with open(path, 'r') as f:
            content = f.read()
    except Exception as e:
        return [f"Failed to read manifest.yaml: {e}"]
    
    required_fields = [
        "schema_version:",
        "run_timestamp:",
        "git_commit:",
        "uvb76_version:",
        "configured_duration_seconds:",
        "sample_interval_ms:",
        "pid:",
        "pss_available:",
        "checkpoints:",
    ]
    
    for field in required_fields:
        if field not in content:
            errors.append(f"manifest.yaml missing required field: {field}")
    
    for phase in ["start", "midpoint", "end"]:
        if f'phase: "{phase}"' not in content and f"phase: '{phase}'" not in content:
            if f"phase: {phase}" not in content:
                errors.append(f"manifest.yaml missing checkpoint phase: {phase}")
    
    return errors


def validate_memstats_json(path, phase):
    """Validate memstats-{phase}.json has required numeric fields."""
    errors = []
    
    if not os.path.exists(path):
        return [f"memstats-{phase}.json does not exist: {path}"]
    
    try:
        with open(path, 'r') as f:
            data = json.load(f)
    except json.JSONDecodeError as e:
        return [f"Invalid JSON in memstats-{phase}.json: {e}"]
    
    if not isinstance(data, dict):
        return [f"memstats-{phase}.json must be a JSON object"]
    
    if data.get("phase") != phase:
        errors.append(f"memstats-{phase}.json phase mismatch: expected {phase}, got {data.get('phase')}")
    
    numeric_fields = [
        "pid", "sample_rss_kib", "sample_pss_kib",
        "goroutines", "heap_alloc_bytes", "heap_inuse_bytes",
        "heap_objects", "heap_sys_bytes", "sys_bytes"
    ]
    
    for field in numeric_fields:
        if field not in data:
            errors.append(f"memstats-{phase}.json missing required field: {field}")
        elif not isinstance(data[field], (int, float)):
            errors.append(f"memstats-{phase}.json field {field} must be numeric")
    
    if "forced_gc" not in data:
        errors.append(f"memstats-{phase}.json missing required field: forced_gc")
    elif not isinstance(data["forced_gc"], bool):
        errors.append(f"memstats-{phase}.json field forced_gc must be boolean")
    
    return errors


def validate_pprof_file(path, phase):
    """Validate heap profile file exists and is non-empty."""
    errors = []
    
    if not os.path.exists(path):
        return [f"heap-{phase}.pprof does not exist: {path}"]
    
    size = os.path.getsize(path)
    if size == 0:
        errors.append(f"heap-{phase}.pprof is empty: {path}")
    
    return errors


def validate_goroutine_dump(path, phase):
    """Validate goroutine dump file exists and is non-empty."""
    errors = []
    
    if not os.path.exists(path):
        return [f"goroutine-{phase}.txt does not exist: {path}"]
    
    size = os.path.getsize(path)
    if size == 0:
        errors.append(f"goroutine-{phase}.txt is empty: {path}")
    
    return errors


# FIX: Exact header validation, tight midpoint tolerance based on sample_interval_ms
def validate_rss_samples(path, configured_duration_seconds=600, sample_interval_ms=5000):
    """Validate rss-pss.tsv has proper coverage including midpoint validation."""
    errors = []
    
    if not os.path.exists(path):
        return ["rss-pss.tsv does not exist"]
    
    try:
        with open(path, 'r') as f:
            lines = f.readlines()
    except Exception as e:
        return [f"Failed to read rss-pss.tsv: {e}"]
    
    if len(lines) < 2:
        return ["rss-pss.tsv must have header + at least one sample"]
    
    # FIX: Exact header validation - must be exactly these 4 columns in this order
    header_parts = lines[0].strip().split('\t')
    expected_header = ["timestamp", "elapsed_ms", "rss_kib", "pss_kib"]
    if header_parts != expected_header:
        errors.append(f"rss-pss.tsv header must be exactly {expected_header}, got {header_parts}")
    
    samples = []
    for i, line in enumerate(lines[1:], start=2):
        parts = line.strip().split('\t')
        # FIX: Exact column count - must be exactly 4
        if len(parts) != 4:
            errors.append(f"rss-pss.tsv line {i} must have exactly 4 columns, got {len(parts)}")
            continue
        try:
            elapsed = int(parts[1])
            rss = int(parts[2])
            pss = int(parts[3])
            samples.append((elapsed, rss, pss))
        except ValueError:
            errors.append(f"rss-pss.tsv line {i} has non-numeric values")
    
    if len(samples) < 2:
        return ["rss-pss.tsv must have at least start and end samples"]
    
    first_elapsed = samples[0][0]
    last_elapsed = samples[-1][0]
    
    if first_elapsed > 1000:
        errors.append(f"rss-pss.tsv first sample elapsed_ms ({first_elapsed}) too large, expected near 0")
    
    if last_elapsed - first_elapsed < 10000:
        errors.append(f"rss-pss.tsv sample duration too short: {last_elapsed - first_elapsed}ms")
    
    # FIX: Tight midpoint tolerance based on sample_interval_ms
    midpoint_target = (configured_duration_seconds * 1000) // 2
    midpoint_tolerance = max(sample_interval_ms * 2, 5000)  # 2x interval or 5s minimum
    has_midpoint = False
    for elapsed, _, _ in samples:
        if abs(elapsed - midpoint_target) <= midpoint_tolerance:
            has_midpoint = True
            break
    if not has_midpoint:
        errors.append(f"rss-pss.tsv missing midpoint sample near {midpoint_target}ms (±{midpoint_tolerance}ms)")
    
    return errors


def parse_manifest_full(manifest_path):
    """Parse manifest to extract all relevant config values."""
    pss_available = True  # Default to true (stricter)
    configured_duration = 600  # Default 10 min
    sample_interval_ms = 5000  # Default 5s
    
    if not os.path.exists(manifest_path):
        return pss_available, configured_duration, sample_interval_ms
    
    try:
        with open(manifest_path, 'r') as f:
            content = f.read()
        for line in content.split('\n'):
            if line.strip().startswith('pss_available:'):
                pss_available = 'true' in line.lower()
            elif line.strip().startswith('configured_duration_seconds:'):
                val = line.split(':')[1].strip().strip(',').strip()
                try:
                    configured_duration = int(val)
                except ValueError:
                    pass
            elif line.strip().startswith('sample_interval_ms:'):
                val = line.split(':')[1].strip().strip(',').strip()
                try:
                    sample_interval_ms = int(val)
                except ValueError:
                    pass
    except Exception:
        pass
    
    return pss_available, configured_duration, sample_interval_ms


def validate_rss_samples_with_pss_check(path, pss_available=True, configured_duration_seconds=600, sample_interval_ms=5000):
    """Validate rss-pss.tsv with PSS consistency checks."""
    errors = validate_rss_samples(path, configured_duration_seconds, sample_interval_ms)
    
    # FIX: Check PSS consistency - if pss_available=true, PSS should not be all zeros
    if pss_available:
        try:
            with open(path, 'r') as f:
                lines = f.readlines()
            non_zero_pss = False
            for line in lines[1:]:  # Skip header
                parts = line.strip().split('\t')
                if len(parts) >= 4:
                    try:
                        pss = int(parts[3])
                        if pss > 0:
                            non_zero_pss = True
                            break
                    except ValueError:
                        pass
            if not non_zero_pss:
                errors.append("rss-pss.tsv has pss_available=true but all PSS values are zero")
        except Exception:
            pass
    
    return errors


def validate_lab_result_json(path):
    """Validate lab-result.json is valid JSON."""
    errors = []
    
    if not os.path.exists(path):
        return ["lab-result.json does not exist"]
    
    try:
        with open(path, 'r') as f:
            data = json.load(f)
    except json.JSONDecodeError as e:
        return [f"Invalid JSON in lab-result.json: {e}"]
    
    if not isinstance(data, dict):
        return ["lab-result.json must be a JSON object"]
    
    return errors


def validate_attribution_dir(dir_path):
    """Validate a complete attribution lab artifact directory."""
    all_errors = []
    
    manifest_path = os.path.join(dir_path, "manifest.yaml")
    all_errors.extend(validate_manifest(manifest_path))
    
    # FIX: Parse manifest for PSS and timing config
    pss_available, configured_duration, sample_interval = parse_manifest_full(manifest_path)
    
    for phase in ["start", "midpoint", "end"]:
        memstats_path = os.path.join(dir_path, f"memstats-{phase}.json")
        all_errors.extend(validate_memstats_json(memstats_path, phase))
    
    for phase in ["start", "midpoint", "end"]:
        pprof_path = os.path.join(dir_path, f"heap-{phase}.pprof")
        all_errors.extend(validate_pprof_file(pprof_path, phase))
    
    for phase in ["start", "midpoint", "end"]:
        goroutine_path = os.path.join(dir_path, f"goroutine-{phase}.txt")
        all_errors.extend(validate_goroutine_dump(goroutine_path, phase))
    
    # FIX: Use PSS-aware validator with proper timing parameters
    rss_path = os.path.join(dir_path, "rss-pss.tsv")
    all_errors.extend(validate_rss_samples_with_pss_check(
        rss_path,
        pss_available=pss_available,
        configured_duration_seconds=configured_duration,
        sample_interval_ms=sample_interval,
    ))
    
    result_path = os.path.join(dir_path, "lab-result.json")
    all_errors.extend(validate_lab_result_json(result_path))
    
    return all_errors


def validate_fixture(path):
    """Validate a fixture file is a valid schema_fixture."""
    errors = []
    try:
        with open(path, 'r') as f:
            data = json.load(f)
        if data.get("evidence_kind") != "schema_fixture":
            errors.append(f"Wrong evidence_kind: {data.get('evidence_kind')}")
    except Exception as e:
        errors.append(f"Invalid fixture: {e}")
    return errors

