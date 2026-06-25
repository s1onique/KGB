# artifacts.py — Matrix artifact writing

import shutil
from pathlib import Path
from datetime import datetime, timezone
from typing import Dict, Any


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


def write_matrix_manifest(matrix_root: Path, run_id: str, duration: int, interval: int, results: dict) -> Path:
    """Write the matrix manifest file."""
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
    return manifest_path
