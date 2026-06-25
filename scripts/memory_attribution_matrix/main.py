# main.py — Matrix execution logic

import shutil
import platform
from pathlib import Path
from datetime import datetime, timezone
from typing import Optional, Tuple

from .variants import MATRIX_VARIANTS
from .runner import run_lab_variant
from .parsing import parse_verdict, parse_manifest, count_native_events
from .validation import check_disabled_subsystem_leak
from .verdict import determine_matrix_verdict
from .artifacts import write_matrix_summary, write_matrix_manifest

SCRIPT_DIR = Path(__file__).parent.parent


def run_matrix(
    duration: int = 600,
    interval: int = 5,
    run_id: Optional[str] = None,
    skip_variants: Optional[list] = None,
) -> Tuple[int, Optional[Path]]:
    """
    Run the complete memory attribution matrix.
    
    Returns: (exit_code, matrix_root_path)
    """
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
    failed_variants = []
    disabled_event_violations = []
    
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
            failed_variants.append(variant_name)
            results[variant_name] = variant_result
            print(f"\n✗ Variant {variant_name}: FAILED")
            print(f"  Error: {error}")
            continue
        
        if artifact_path:
            variant_result["verdict"] = parse_verdict(artifact_path)
            variant_result["manifest"] = parse_manifest(artifact_path)
            variant_result["native_counts"] = count_native_events(artifact_path)
            
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
    
    # Write artifacts
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
    
    manifest_path = write_matrix_manifest(
        matrix_root=matrix_root,
        run_id=run_id,
        duration=duration,
        interval=interval,
        results=results,
    )
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
