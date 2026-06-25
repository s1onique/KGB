# runner.py — Lab variant execution

import sys
import subprocess
import shutil
from pathlib import Path
from typing import Optional, Tuple

SCRIPT_DIR = Path(__file__).parent.parent


def run_lab_variant(
    variant_name: str,
    variant_config: dict,
    base_duration: int,
    base_interval: int,
    matrix_run_id: str,
    matrix_root: Path,
    tovarisch_binary: Path,
) -> Tuple[bool, Optional[Path], str]:
    """
    Run a single lab variant and return (success, artifact_path, error_msg).
    
    The lab runner writes to: artifacts/memory-labs/tovarisch/idle-staircase/<run-id>/
    We copy it to: <matrix_root>/<variant_name>/
    """
    artifact_path = matrix_root / variant_name
    lab_run_id = f"{matrix_run_id}-{variant_name}"
    lab_artifact_path = SCRIPT_DIR.parent / "artifacts" / "memory-labs" / "tovarisch" / "idle-staircase" / lab_run_id
    
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
            timeout=base_duration + 120,
        )
        
        if result.returncode != 0:
            return False, None, f"Lab failed with code {result.returncode}: {result.stderr}"
        
        if not lab_artifact_path.exists():
            return False, None, f"Lab artifact path not created: {lab_artifact_path}"
        
        if artifact_path.exists():
            shutil.rmtree(artifact_path)
        shutil.copytree(lab_artifact_path, artifact_path)
        
        required_files = ["manifest.yaml", "memory_samples.tsv", "verdict.txt", "native_event_timeline.tsv"]
        missing = [f for f in required_files if not (artifact_path / f).exists()]
        if missing:
            return False, None, f"Missing required files: {missing}"
        
        return True, artifact_path, ""
        
    except subprocess.TimeoutExpired:
        return False, None, f"Lab timed out after {base_duration + 120}s"
    except Exception as e:
        return False, None, f"Unexpected error: {e}"
