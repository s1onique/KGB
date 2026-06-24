#!/usr/bin/env python3
# lab_memory_attribution_matrix.py — Long-window memory attribution matrix runner
#
# Runs multiple idle memory lab variants to compare native runtime toggles and
# determine whether idle RSS/PSS growth is caused by a specific subsystem or is
# only bounded allocator/warmup behavior.
#
# Variants run:
#   - all_enabled:     All subsystems (heartbeat, WG checks, BGP, BFD) enabled
#   - heartbeat_disabled:  Heartbeat disabled
#   - wg_disabled:        WG checks disabled
#   - bgp_disabled:       BGP disabled
#   - bfd_disabled:       BFD disabled
#   - bgp_bfd_disabled:   BGP+BFD disabled
#   - no_periodic:        All optional periodic subsystems disabled
#
# Usage:
#   python3 scripts/lab_memory_attribution_matrix.py [options]

import sys
import os
import subprocess
import shutil
from pathlib import Path
from datetime import datetime, timezone
from typing import Optional

# Add scripts dir to path for lab_runner
SCRIPT_DIR = Path(__file__).parent
sys.path.insert(0, str(SCRIPT_DIR))


# Matrix variants configuration
MATRIX_VARIANTS = {
    "all_enabled": {
        "description": "All subsystems enabled (baseline)",
        "disable_heartbeat": False,
        "disable_wg_checks": False,
        "disable_bgp": False,
        "disable_bfd": False,
    },
    "heartbeat_disabled": {
        "description": "Heartbeat disabled",
        "disable_heartbeat": True,
        "disable_wg_checks": False,
        "disable_bgp": False,
        "disable_bfd": False,
    },
    "wg_disabled": {
        "description": "WireGuard checks disabled",
        "disable_heartbeat": False,
        "disable_wg_checks": True,
        "disable_bgp": False,
        "disable_bfd": False,
    },
    "bgp_disabled": {
        "description": "BGP disabled",
        "disable_heartbeat": False,
        "disable_wg_checks": False,
        "disable_bgp": True,
        "disable_bfd": False,
    },
    "bfd_disabled": {
        "description": "BFD disabled",
        "disable_heartbeat": False,
        "disable_wg_checks": False,
        "disable_bgp": False,
        "disable_bfd": True,
    },
    "bgp_bfd_disabled": {
        "description": "BGP+BFD disabled",
        "disable_heartbeat": False,
        "disable_wg_checks": False,
        "disable_bgp": True,
        "disable_bfd": True,
    },
    "no_periodic": {
        "description": "All periodic subsystems disabled (baseline for allocator/warmup)",
        "disable_heartbeat": True,
        "disable_wg_checks": True,
        "disable_bgp": True,
        "disable_bfd": True,
    },
}


def run_lab_variant(
    variant_name: str,
    variant_config: dict,
    base_duration: int,
    base_interval: int,
    matrix_run_id: str,
    matrix_root: Path,
    tovarisch_binary: Path,
) -> tuple[bool, Optional[Path], str]:
    """
    Run a single lab variant and return (success, artifact_path, error_msg).
    
    The lab runner writes to: artifacts/memory-labs/tovarisch/idle-staircase/<run-id>/
    We copy it to: <matrix_root>/<variant_name>/
    """
    # Expected output location for this variant
    artifact_path = matrix_root / variant_name
    
    # The lab runner uses a fixed artifact root under idle-staircase
    lab_run_id = f"{matrix_run_id}-{variant_name}"
    lab_artifact_path = SCRIPT_DIR.parent / "artifacts" / "memory-labs" / "tovarisch" / "idle-staircase" / lab_run_id
    
    # Build command-line args for lab_tovarisch_idle_memory.py
    cmd = [
        sys.executable,
        str(SCRIPT_DIR / "lab_tovarisch_idle_memory.py"),
        "--native-events",
        "--duration", str(base_duration),
        "--interval", str(base_interval),
        "--run-id", lab_run_id,
    ]
    
    if variant_config.get("disable_heartbeat"):
        cmd.append("--disable-heartbeat")
    if variant_config.get("disable_wg_checks"):
        cmd.append("--disable-wg-checks")
    if variant_config.get("disable_bgp"):
        cmd.append("--disable-bgp")
    if variant_config.get("disable_bfd"):
        cmd.append("--disable-bfd")
    
    print(f"\n{'='*60}")
    print(f"Running variant: {variant_name}")
    print(f"Description: {variant_config['description']}")
    print(f"Command: {' '.join(cmd)}")
    print(f"Expected lab artifact: {lab_artifact_path}")
    print(f"Target variant path: {artifact_path}")
    print(f"{'='*60}")
    
    try:
        result = subprocess.run(
            cmd,
            cwd=SCRIPT_DIR.parent,
            capture_output=True,
            text=True,
            timeout=base_duration + 120,  # Allow extra time for startup/shutdown
        )
        
        if result.returncode != 0:
            return False, None, f"Lab failed with code {result.returncode}: {result.stderr}"
        
        # Check lab artifact was created
        if not lab_artifact_path.exists():
            return False, None, f"Lab artifact path not created: {lab_artifact_path}"
        
        # Copy lab artifact to matrix variant path
        import shutil
        if artifact_path.exists():
            shutil.rmtree(artifact_path)
        shutil.copytree(lab_artifact_path, artifact_path)
        
        # Verify required files exist
        required_files = ["manifest.yaml", "memory_samples.tsv", "verdict.txt", "native_event_timeline.tsv"]
        missing = [f for f in required_files if not (artifact_path / f).exists()]
        if missing:
            return False, None, f"Missing required files: {missing}"
        
        return True, artifact_path, ""
        
    except subprocess.TimeoutExpired:
        return False, None, f"Lab timed out after {base_duration + 120}s"
    except Exception as e:
        return False, None, f"Unexpected error: {e}"


def parse_verdict(artifact_path: Path) -> dict:
    """Parse verdict.txt into structured data."""
    verdict_path = artifact_path / "verdict.txt"
    if not verdict_path.exists():
        return {"verdict": "unknown", "error": "verdict.txt missing"}
    
    content = verdict_path.read_text()
    result = {"verdict": "unknown", "raw": content}
    
    import re
    
    # Extract key fields
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
    
    # Native event counts
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
    
    import re
    
    # Extract key fields
    for field in ["run_id", "platform", "commit_sha", "duration_seconds", "sample_interval_seconds"]:
        match = re.search(rf'{field}:\s*(.+)', content)
        if match:
            result[field] = match.group(1).strip()
    
    # Native event config
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
            
            counts["total"] += 1
    
    return counts


def check_disabled_subsystem_leak(
    variant_name: str, 
    variant_config: dict,
    native_counts: dict
) -> tuple[bool, str]:
    """
    Check if a disabled subsystem still emits native events (FAIL condition).
    Returns (is_leak, description).
    """
    if variant_config.get("disable_heartbeat") and native_counts.get("heartbeat", 0) > 0:
        return True, f"Heartbeat disabled but {native_counts['heartbeat']} heartbeat events emitted"
    
    if variant_config.get("disable_wg_checks") and native_counts.get("wireguard", 0) > 0:
        return True, f"WG checks disabled but {native_counts['wireguard']} WG events emitted"
    
    if variant_config.get("disable_bgp") and native_counts.get("bgp", 0) > 0:
        return True, f"BGP disabled but {native_counts['bgp']} BGP events emitted"
    
    if variant_config.get("disable_bfd") and native_counts.get("bfd", 0) > 0:
        return True, f"BFD disabled but {native_counts['bfd']} BFD events emitted"
    
    return False, ""


def determine_matrix_verdict(results: dict) -> tuple[str, str, dict]:
    """
    Determine the overall matrix verdict based on all variant results.
    
    Returns: (verdict, reason, details)
    
    Verdicts:
      - no_growth: No significant growth in any variant
      - bounded_warmup_or_allocator_highwater: Growth present but bounded across all variants
      - subsystem_correlated_growth: Specific subsystem correlates with growth
      - inconclusive: Cannot determine
    """
    import re
    
    # Collect metrics
    variant_metrics = []
    
    for name, data in results.items():
        if not data.get("success"):
            continue
        
        verdict_data = data.get("verdict", {})
        native_counts = data.get("native_counts", {})
        
        growth = verdict_data.get("total_growth_kib", 0)
        steps = verdict_data.get("steps_detected", 0)
        rate = verdict_data.get("growth_rate_kib_per_min", 0)
        samples = verdict_data.get("samples_count", 0)
        
        variant_metrics.append({
            "name": name,
            "growth": growth,
            "steps": steps,
            "rate": rate,
            "samples": samples,
            "verdict": verdict_data.get("verdict", "unknown"),
            "owner": verdict_data.get("owner", ""),
            "native_counts": native_counts,
        })
    
    if not variant_metrics:
        return "inconclusive", "No successful variant runs", {}
    
    # Check for growth patterns
    all_growths = [m["growth"] for m in variant_metrics]
    max_growth = max(all_growths) if all_growths else 0
    
    # Check for steps
    all_steps = [m["steps"] for m in variant_metrics]
    max_steps = max(all_steps) if all_steps else 0
    
    # Analyze by subsystem presence
    subsystems_present = {
        "heartbeat": [],
        "wireguard": [],
        "bgp": [],
        "bfd": [],
    }
    
    for m in variant_metrics:
        name = m["name"]
        if name == "all_enabled":
            subsystems_present["heartbeat"].append(m)
            subsystems_present["wireguard"].append(m)
            subsystems_present["bgp"].append(m)
            subsystems_present["bfd"].append(m)
        elif name == "heartbeat_disabled":
            # Heartbeat absent
            pass
        elif name == "wg_disabled":
            # WG absent
            pass
        elif name == "bgp_disabled":
            subsystems_present["bgp"].remove if m in subsystems_present["bgp"] else None
        elif name == "bfd_disabled":
            subsystems_present["bfd"].remove if m in subsystems_present["bfd"] else None
        elif name == "bgp_bfd_disabled":
            pass
        elif name == "no_periodic":
            # Baseline
            pass
    
    # Key analysis: compare all_enabled vs no_periodic
    all_enabled_metric = next((m for m in variant_metrics if m["name"] == "all_enabled"), None)
    no_periodic_metric = next((m for m in variant_metrics if m["name"] == "no_periodic"), None)
    
    details = {
        "variants": variant_metrics,
        "max_growth_kib": max_growth,
        "max_steps": max_steps,
        "all_enabled_growth": all_enabled_metric["growth"] if all_enabled_metric else None,
        "no_periodic_growth": no_periodic_metric["growth"] if no_periodic_metric else None,
    }
    
    # Decision logic
    
    # Case 1: No growth anywhere
    if max_growth < 100 and max_steps < 3:
        return "no_growth", "No significant memory growth detected in any variant", details
    
    # Case 2: Growth in all_enabled but NOT in no_periodic
    if all_enabled_metric and no_periodic_metric:
        all_enabled_growth = all_enabled_metric["growth"]
        no_periodic_growth = no_periodic_metric["growth"]
        
        # Significant growth only when subsystems are enabled
        if all_enabled_growth > 200 and no_periodic_growth < 100:
            # Check which subsystem is the likely owner
            # Compare single-subsystem disabled variants
            suspects = []
            
            if all_enabled_metric["growth"] - (next((m for m in variant_metrics if m["name"] == "heartbeat_disabled"), all_enabled_metric) or all_enabled_metric)["growth"] > 200:
                suspects.append("heartbeat")
            if all_enabled_metric["growth"] - (next((m for m in variant_metrics if m["name"] == "wg_disabled"), all_enabled_metric) or all_enabled_metric)["growth"] > 200:
                suspects.append("wireguard")
            if all_enabled_metric["growth"] - (next((m for m in variant_metrics if m["name"] == "bgp_disabled"), all_enabled_metric) or all_enabled_metric)["growth"] > 200:
                suspects.append("bgp")
            if all_enabled_metric["growth"] - (next((m for m in variant_metrics if m["name"] == "bfd_disabled"), all_enabled_metric) or all_enabled_metric)["growth"] > 200:
                suspects.append("bfd")
            
            if suspects:
                return "subsystem_correlated_growth", f"Growth correlates with subsystem(s): {', '.join(suspects)}", details
            else:
                return "subsystem_correlated_growth", "Growth correlates with periodic subsystems but specific owner unclear", details
        
        # Growth present even in no_periodic - likely allocator/warmup
        if no_periodic_growth > 200:
            return "bounded_warmup_or_allocator_highwater", f"Growth persists with all periodic paths disabled ({no_periodic_growth} KiB). Likely allocator warmup or other source.", details
    
    # Case 3: Growth in no_periodic but bounded
    if no_periodic_metric and no_periodic_metric["growth"] < 500 and no_periodic_metric["steps"] < 5:
        return "bounded_warmup_or_allocator_highwater", f"Growth is bounded even with all periodic paths disabled ({no_periodic_metric['growth']} KiB). Consistent with allocator warmup settling.", details
    
    # Case 4: Inconclusive
    return "inconclusive", "Cannot determine growth attribution from available variants", details


def write_matrix_summary(
    matrix_root: Path,
    matrix_run_id: str,
    results: dict,
    overall_verdict: str,
    verdict_reason: str,
    verdict_details: dict,
    duration: int,
    interval: int,
) -> Path:
    """Write the matrix summary document."""
    summary_path = matrix_root / "matrix-summary.md"
    
    # Build comparison table
    table_rows = []
    for name, data in sorted(results.items()):
        if not data.get("success"):
            status = "❌ FAILED"
            growth = "N/A"
            rate = "N/A"
            steps = "N/A"
            verdict_cell = "N/A"
            owner = "N/A"
            hb_count = "N/A"
            wg_count = "N/A"
            bgp_count = "N/A"
            bfd_count = "N/A"
        else:
            status = "✓ OK"
            vd = data.get("verdict", {})
            nc = data.get("native_counts", {})
            growth = vd.get("total_growth_kib", 0)
            rate = vd.get("growth_rate_kib_per_min", 0)
            steps = vd.get("steps_detected", 0)
            verdict_cell = vd.get("verdict", "unknown")
            owner = vd.get("owner", "-")
            hb_count = nc.get("heartbeat", 0)
            wg_count = nc.get("wireguard", 0)
            bgp_count = nc.get("bgp", 0)
            bfd_count = nc.get("bfd", 0)
        
        table_rows.append(f"| {name} | {status} | {growth} | {rate} | {steps} | {verdict_cell} | {owner} | {hb_count} | {wg_count} | {bgp_count} | {bfd_count} |")
    
    table_header = "| Variant | Status | Growth (KiB) | Rate (KiB/min) | Steps | Verdict | Owner | HB events | WG events | BGP events | BFD events |"
    table_sep = "|---|---|---|---|---|---|---|---|---|---|---|"
    
    # Build native event summary
    native_event_table = []
    for name, data in sorted(results.items()):
        if not data.get("success"):
            continue
        nc = data.get("native_counts", {})
        native_event_table.append(f"**{name}**: HB={nc.get('heartbeat', 0)}, WG={nc.get('wireguard', 0)}, BGP={nc.get('bgp', 0)}, BFD={nc.get('bfd', 0)}, Total={nc.get('total', 0)}")
    
    content = f"""# Memory Attribution Matrix Summary

**Matrix Run ID**: `{matrix_run_id}`
**Generated**: {datetime.now(timezone.utc).isoformat()}
**Duration**: {duration}s per variant
**Sample Interval**: {interval}s

## Overall Verdict

**{overall_verdict.upper()}**

{verdict_reason}

## Comparison Table

{table_header}
{table_sep}
{"".join(table_rows)}

## Native Event Counts

{" | ".join(native_event_table)}

## Variant Details

"""
    
    # Add per-variant details
    for name, data in sorted(results.items()):
        content += f"\n### {name}\n\n"
        
        if not data.get("success"):
            content += f"**Status**: FAILED\n\n"
            content += f"**Error**: {data.get('error', 'Unknown error')}\n\n"
            continue
        
        content += f"**Status**: OK\n\n"
        
        vd = data.get("verdict", {})
        manifest = data.get("manifest", {})
        
        content += f"- Growth: {vd.get('total_growth_kib', 'N/A')} KiB\n"
        content += f"- Rate: {vd.get('growth_rate_kib_per_min', 'N/A')} KiB/min\n"
        content += f"- Steps: {vd.get('steps_detected', 'N/A')}\n"
        content += f"- Samples: {vd.get('samples_count', 'N/A')}\n"
        content += f"- Verdict: {vd.get('verdict', 'unknown')}\n"
        content += f"- Owner: {vd.get('owner', '-')}\n"
        
        if vd.get("reason"):
            content += f"\n**Reason**: {vd['reason']}\n"
        
        content += f"\n**Manifest Config**:\n"
        content += f"- native_events_enabled: {manifest.get('native_events_enabled', 'N/A')}\n"
        content += f"- disable_heartbeat: {manifest.get('native_disable_heartbeat', 'N/A')}\n"
        content += f"- disable_wg_checks: {manifest.get('native_disable_wg_checks', 'N/A')}\n"
        content += f"- disable_bgp: {manifest.get('native_disable_bgp', 'N/A')}\n"
        content += f"- disable_bfd: {manifest.get('native_disable_bfd', 'N/A')}\n"
        
        nc = data.get("native_counts", {})
        content += f"\n**Native Events**: HB={nc.get('heartbeat', 0)}, WG={nc.get('wireguard', 0)}, BGP={nc.get('bgp', 0)}, BFD={nc.get('bfd', 0)}\n"
        
        # Check for disabled subsystem leak
        leak = data.get("disabled_leak", "")
        if leak:
            content += f"\n**⚠️ FAIL**: {leak}\n"
    
    content += f"""

## Interpretation Guide

| Verdict | Meaning |
|---------|---------|
| `no_growth` | No significant memory growth detected in any variant |
| `bounded_warmup_or_allocator_highwater` | Growth present but bounded. Consistent with allocator settling behavior. |
| `subsystem_correlated_growth` | Growth correlates with specific subsystem(s). Evidence points to periodic background paths. |
| `inconclusive` | Cannot determine attribution. More data needed. |

## Evidence Contract

This matrix proves or disproves:

1. **Bounded allocator/warmup**: If `no_periodic` variant shows bounded growth (~<500 KiB, ~<5 steps) while `all_enabled` shows none, the growth is likely allocator settling.

2. **Subsystem attribution**: If disabling a specific subsystem eliminates growth while other variants show growth, that subsystem is the likely owner.

3. **Global leak**: This matrix does NOT claim "no leak". It only classifies what the evidence shows.

## Artifact Locations

Matrix root: `{matrix_root}`

Per-variant artifacts:
"""
    
    for name in results:
        content += f"- `{matrix_root / name}/`\n"
    
    summary_path.write_text(content)
    return summary_path


def run_matrix(
    duration: int = 600,
    interval: int = 5,
    run_id: Optional[str] = None,
    skip_variants: Optional[list] = None,
) -> tuple[int, Path]:
    """
    Run the complete memory attribution matrix.
    
    Returns: (exit_code, matrix_root_path)
    """
    import platform
    
    # Platform check
    if platform.system() != "Linux":
        print("ERROR: Memory attribution matrix requires Linux with /proc")
        return 1, None
    
    if not Path("/proc").is_dir():
        print("ERROR: /proc not found. Linux required.")
        return 1, None
    
    # Generate matrix run ID
    if not run_id:
        run_id = f"matrix-{datetime.now(timezone.utc).strftime('%Y%m%d-%H%M%S')}"
    
    # Matrix root directory
    repo_root = SCRIPT_DIR.parent
    matrix_root = repo_root / "artifacts" / "memory-labs" / "tovarisch" / "idle-matrix" / run_id
    matrix_root.mkdir(parents=True, exist_ok=True)
    
    print(f"=== Memory Attribution Matrix ===")
    print(f"Run ID: {run_id}")
    print(f"Matrix Root: {matrix_root}")
    print(f"Duration: {duration}s per variant")
    print(f"Interval: {interval}s")
    print(f"Variants: {len(MATRIX_VARIANTS)}")
    
    # Check tovarisch binary
    tovarisch_binary = repo_root / "tovarisch" / "zig-out" / "bin" / "tovarisch"
    if not tovarisch_binary.exists():
        print(f"ERROR: Tovarisch binary not found: {tovarisch_binary}")
        print("Run 'make tovarisch-build' first.")
        return 1, None
    
    # Run variants
    results = {}
    failed_variants = []  # Variants that failed to run
    disabled_event_violations = []  # Variants that leaked events when disabled
    
    for variant_name, variant_config in MATRIX_VARIANTS.items():
        if skip_variants and variant_name in skip_variants:
            print(f"\nSkipping variant: {variant_name}")
            continue
        
        success, artifact_path, error = run_lab_variant(
            variant_name=variant_name,
            variant_config=variant_config,
            base_duration=duration,
            base_interval=interval,
            matrix_run_id=run_id,
            matrix_root=matrix_root,
            tovarisch_binary=tovarisch_binary,
        )
        
        variant_result = {
            "success": success,
            "error": error,
            "artifact_path": artifact_path,
            "variant_config": variant_config,
        }
        
        if not success:
            # Lab variant failed to run - fail the matrix
            failed_variants.append(variant_name)
            results[variant_name] = variant_result
            print(f"\n✗ Variant {variant_name}: FAILED")
            print(f"  Error: {error}")
            continue
        
        # Lab ran successfully - parse results
        if artifact_path:
            # Parse verdict
            variant_result["verdict"] = parse_verdict(artifact_path)
            variant_result["manifest"] = parse_manifest(artifact_path)
            variant_result["native_counts"] = count_native_events(artifact_path)
            
            # Check for disabled subsystem leak
            is_leak, leak_desc = check_disabled_subsystem_leak(
                variant_name, variant_config, variant_result["native_counts"]
            )
            variant_result["disabled_leak"] = leak_desc if is_leak else ""
            
            if is_leak:
                print(f"\n⚠️ VIOLATION: {variant_name}: {leak_desc}")
                disabled_event_violations.append(variant_name)
        
        results[variant_name] = variant_result
        
        status = "✓"
        print(f"\n{status} Variant {variant_name}: OK")
    
    # Determine overall verdict
    print(f"\n{'='*60}")
    print("Determining matrix verdict...")
    
    overall_verdict, verdict_reason, verdict_details = determine_matrix_verdict(results)
    
    print(f"\nOverall Verdict: {overall_verdict}")
    print(f"Reason: {verdict_reason}")
    
    # Write matrix summary
    summary_path = write_matrix_summary(
        matrix_root=matrix_root,
        matrix_run_id=run_id,
        results=results,
        overall_verdict=overall_verdict,
        verdict_reason=verdict_reason,
        verdict_details=verdict_details,
        duration=duration,
        interval=interval,
    )
    
    print(f"\nMatrix summary written to: {summary_path}")
    
    # Write matrix manifest (records which variants were actually run)
    manifest_path = matrix_root / "matrix-manifest.yaml"
    manifest_lines = [
        f"run_id: {run_id}",
        f"duration_seconds: {duration}",
        f"sample_interval_seconds: {interval}",
        f"variants:",
    ]
    for variant_name in results:
        manifest_lines.append(f"  - {variant_name}")
    manifest_path.write_text("\n".join(manifest_lines) + "\n")
    print(f"Matrix manifest written to: {manifest_path}")
    
    # Copy summary to artifact root
    shutil.copy2(summary_path, matrix_root.parent / f"matrix-summary-{run_id}.md")
    
    # Report failures
    if failed_variants:
        print(f"\n{'='*60}")
        print("FAILED: Required variants failed to run:")
        for v in failed_variants:
            error = results[v].get("error", "Unknown error")
            print(f"  - {v}: {error}")
        print(f"{'='*60}")
        return 1, matrix_root
    
    if disabled_event_violations:
        print(f"\n{'='*60}")
        print("FAILED: Disabled subsystem event violations:")
        for v in disabled_event_violations:
            leak = results[v].get("disabled_leak", "Unknown violation")
            print(f"  - {v}: {leak}")
        print(f"{'='*60}")
        return 1, matrix_root
    
    print(f"\n{'='*60}")
    print(f"Matrix complete: {matrix_root}")
    print(f"Overall verdict: {overall_verdict}")
    
    return 0, matrix_root


def main(argv: list[str]) -> int:
    """Main entry point."""
    import argparse
    
    parser = argparse.ArgumentParser(
        description="Memory attribution matrix runner",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Run full matrix with 10-minute variants
  python3 scripts/lab_memory_attribution_matrix.py

  # Run with 30-minute variants for longer observation
  python3 scripts/lab_memory_attribution_matrix.py --duration 1800

  # Run specific variants only
  python3 scripts/lab_memory_attribution_matrix.py --variants all_enabled no_periodic
  
  # Custom run ID
  python3 scripts/lab_memory_attribution_matrix.py --run-id my-test
        """
    )
    parser.add_argument(
        "--duration",
        type=int,
        default=600,
        help="Duration per variant in seconds (default: 600)"
    )
    parser.add_argument(
        "--interval",
        type=int,
        default=5,
        help="Memory sample interval in seconds (default: 5)"
    )
    parser.add_argument(
        "--run-id",
        default=None,
        help="Custom matrix run ID"
    )
    parser.add_argument(
        "--variants",
        nargs="+",
        default=None,
        help="Specific variants to run (default: all)"
    )
    
    args = parser.parse_args(argv)
    
    # Validate variants
    skip_variants = None
    if args.variants:
        valid_variants = set(MATRIX_VARIANTS.keys())
        requested = set(args.variants)
        invalid = requested - valid_variants
        if invalid:
            print(f"ERROR: Unknown variants: {', '.join(invalid)}")
            print(f"Valid variants: {', '.join(sorted(valid_variants))}")
            return 1
        skip_variants = valid_variants - requested
    
    exit_code, matrix_root = run_matrix(
        duration=args.duration,
        interval=args.interval,
        run_id=args.run_id,
        skip_variants=skip_variants,
    )
    
    return exit_code


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
