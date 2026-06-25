# fixtures.py — Test fixtures for self-tests

import shutil
from pathlib import Path


def get_variant_config(variant_name):
    from .checks import get_variant_config as _get_config
    return _get_config(variant_name)


def create_matrix_fixture(tmpdir, name, variants=None):
    from .checks import CANONICAL_VARIANTS
    
    matrix = tmpdir / name
    matrix.mkdir(exist_ok=True)
    
    if variants is None:
        variants = CANONICAL_VARIANTS
    
    for variant in variants:
        variant_path = matrix / variant
        variant_path.mkdir(exist_ok=True)
        
        variant_config = get_variant_config(variant)
        disable_hb = variant_config.get("disable_heartbeat", False) if variant_config else False
        disable_wg = variant_config.get("disable_wg_checks", False) if variant_config else False
        disable_bgp = variant_config.get("disable_bgp", False) if variant_config else False
        disable_bfd = variant_config.get("disable_bfd", False) if variant_config else False
        
        (variant_path / "manifest.yaml").write_text(
            f"run_id: test-{variant}\nplatform: Linux\ncommit_sha: abc123\n"
            f"native_events_enabled: true\nnative_disable_heartbeat: {str(disable_hb).lower()}\n"
            f"native_disable_wg_checks: {str(disable_wg).lower()}\n"
            f"native_disable_bgp: {str(disable_bgp).lower()}\n"
            f"native_disable_bfd: {str(disable_bfd).lower()}\n"
            f"duration_seconds: 600\n"
        )
        
        growth = 200 if variant == "all_enabled" else 50
        (variant_path / "verdict.txt").write_text(
            f"verdict: inconclusive\nsteps_detected: 2\ntotal_growth_kib: {growth}\n"
            f"growth_rate_kib_per_min: 5\nsamples_count: 20\n"
        )
        
        (variant_path / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:05:00\t300\t1050\t2050\n"
        )
        
        hb_count = 0 if disable_hb else 6
        wg_count = 0 if disable_wg else 2
        bgp_count = 0 if disable_bgp else 10
        bfd_count = 0 if disable_bfd else 10
        
        lines = ["timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid"]
        for i in range(hb_count):
            lines.append(f"2024-01-01T00:00:{30+i*30}\t{30000+i*30000}\theartbeat_tick_start\theartbeat\t\t1234")
        for i in range(wg_count):
            lines.append(f"2024-01-01T00:01:{i*60}\t{60000+i*60000}\twg_check_start\twireguard\t\t1234")
        for i in range(bgp_count):
            lines.append(f"2024-01-01T00:00:{i*10}\t{i*10000}\tbgp_maintenance_start\tbgp\t\t1234")
        for i in range(bfd_count):
            lines.append(f"2024-01-01T00:00:{i*10}\t{i*10000}\tbfd_tick_start\tbfd\t\t1234")
        
        (variant_path / "native_event_timeline.tsv").write_text("\n".join(lines) + "\n")
    
    summary = "# Memory Attribution Matrix Summary\n\n**Matrix Run ID**: `test-matrix`\n**Overall Verdict**: `INCONCLUSIVE`\n\n| Variant | Status | Growth (KiB) | Rate (KiB/min) | Steps | Verdict |\n|---------|--------|--------------|----------------|-------|---------|\n"
    for variant in variants:
        summary += f"| {variant} | OK | 200 | 5 | 2 | inconclusive |\n"
    
    (matrix / "matrix-summary.md").write_text(summary)
    
    manifest = f"run_id: test-matrix\nduration_seconds: 600\nsample_interval_seconds: 5\nvariants:\n"
    for variant in variants:
        manifest += f"  - {variant}\n"
    (matrix / "matrix-manifest.yaml").write_text(manifest)
    
    return matrix
