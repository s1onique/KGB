# lab_runner/tovarisch.py - Tovarisch process lifecycle
import subprocess
import sys
import time
from pathlib import Path

from lab_runner.config import LabRunConfig, LabError


def start_tovarisch(config: LabRunConfig) -> subprocess.Popen:
    """Start tovarisch process with subprocess.Popen."""
    if not config.tovarisch_binary.exists():
        raise LabError(f"tovarisch binary not found: {config.tovarisch_binary}")

    config_path = config.artifact_path / "tovarisch_lab.conf"

    process = subprocess.Popen(
        [
            str(config.tovarisch_binary),
            "serve",
            "--config",
            str(config_path),
            "--listen",
            f"{config.lab_bind}:{config.lab_port}",
        ],
        stdout=sys.stdout,
        stderr=sys.stderr,
        text=True,
        cwd=str(config.repo_root),
    )

    return process


def wait_for_tovarisch_ready(config: LabRunConfig, process: subprocess.Popen) -> None:
    """Wait for tovarisch to be ready and verify it's running."""
    time.sleep(2)

    if process.poll() is not None:
        raise LabError("tovarisch process exited during startup")

    pid = process.pid
    proc_path = Path("/proc") / str(pid)
    if not proc_path.is_dir():
        raise LabError(f"/proc/{pid} disappeared while waiting for tovarisch to be ready")


def terminate_process(process: subprocess.Popen) -> None:
    """Terminate process gracefully, then kill if still alive."""
    if process is None:
        return

    pid = process.pid

    try:
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)
    except Exception:
        pass

    # Verify /proc is gone
    try:
        proc_path = Path("/proc") / str(pid)
        if proc_path.is_dir():
            try:
                process.kill()
                process.wait(timeout=5)
            except Exception:
                pass
    except Exception:
        pass
