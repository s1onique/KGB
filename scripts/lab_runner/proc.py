# lab_runner/proc.py - /proc filesystem operations
from pathlib import Path


def require_linux_proc() -> None:
    """Fail closed if not Linux with /proc."""
    import platform
    if platform.system() != "Linux":
        raise LabError("idle staircase lab requires Linux")
    proc_path = Path("/proc")
    if not proc_path.is_dir():
        raise LabError("idle staircase lab requires /proc")


def read_proc_status(pid: int) -> dict[str, int]:
    """Read /proc/<pid>/status and extract memory fields."""
    status_path = Path("/proc") / str(pid) / "status"

    if not status_path.exists():
        return {}

    text = status_path.read_text()
    result = {}

    for line in text.splitlines():
        if not line.startswith(("VmRSS:", "VmHWM:", "VmSize:", "VmData:", "VmSwap:", "VmPeak:")):
            continue
        key, raw = line.split(":", 1)
        parts = raw.strip().split()
        result[key] = int(parts[0]) if parts else 0

    return result


# Import LabError for require_linux_proc
from lab_runner.config import LabError
