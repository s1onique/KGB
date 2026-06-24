# lab_runner/artifacts.py - Artifact file generation
import subprocess
from datetime import datetime, timezone
from pathlib import Path

from lab_runner.config import LabRunConfig, LabError


def write_tovarisch_config(config: LabRunConfig) -> Path:
    """Generate tovarisch config file with [lab] section."""
    config_path = config.artifact_path / "tovarisch_lab.conf"

    lines = ["[lab]"]
    lines.append(f"native_events_enabled = {'true' if config.native_events else 'false'}")

    if config.native_events and config.native_events_path:
        lines.append(f"native_events_path = \"{config.native_events_path}\"")

    lines.append(f"disable_heartbeat = {'true' if config.disable_heartbeat else 'false'}")
    lines.append(f"disable_wg_checks = {'true' if config.disable_wg_checks else 'false'}")
    lines.append(f"disable_bgp = {'true' if config.disable_bgp else 'false'}")
    lines.append(f"disable_bfd = {'true' if config.disable_bfd else 'false'}")
    lines.append("")

    config_path.write_text("\n".join(lines))
    return config_path


def print_tovarisch_config(config: LabRunConfig) -> None:
    """Print the generated tovarisch config for diagnostics."""
    print("=== Tovarisch Config ===")
    config_path = config.artifact_path / "tovarisch_lab.conf"
    if config_path.exists():
        print(config_path.read_text())
    print("========================")


def yaml_bool(value: bool) -> str:
    """Convert Python bool to YAML-like lowercase string."""
    return "true" if value else "false"


def write_manifest(config: LabRunConfig) -> Path:
    """Write lab manifest YAML."""
    manifest_path = config.artifact_path / "manifest.yaml"

    # Get git info
    try:
        git_sha = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            cwd=config.repo_root,
            capture_output=True,
            text=True,
            timeout=10
        ).stdout.strip()
    except Exception:
        git_sha = "unknown"

    try:
        git_dirty = subprocess.run(
            ["git", "status", "--porcelain"],
            cwd=config.repo_root,
            capture_output=True,
            text=True,
            timeout=10
        ).stdout.strip()
        git_dirty_count = len(git_dirty.splitlines()) if git_dirty else 0
    except Exception:
        git_dirty_count = -1

    import platform
    kernel = platform.release()
    arch = platform.machine()
    native_path_str = str(config.native_events_path) if config.native_events_path else ""

    manifest_lines = [
        "# Idle Staircase Memory Lab Manifest",
        f"# Generated: {datetime.now(timezone.utc).isoformat()}",
        f"run_id: \"{config.run_id}\"",
        f"platform: {platform.system()}",
        f"kernel: {kernel}",
        f"architecture: {arch}",
        f"duration_seconds: {config.duration}",
        f"sample_interval_seconds: {config.interval}",
        f"status_burst: {yaml_bool(config.status_burst)}",
        f"strace_enabled: {yaml_bool(config.strace)}",
        f"lab_port: {config.lab_port}",
        f"lab_bind: {config.lab_bind}",
        "# CRITICAL: Shell-side synthetic events CANNOT produce confirmed_leak verdicts",
        "event_source: shell_synthetic",
        f"heartbeat_enabled: {yaml_bool(config.heartbeat_enabled)}",
        f"wg_check_enabled: {yaml_bool(config.wg_check_enabled)}",
        f"bgp_bfd_enabled: {yaml_bool(config.bgp_bfd_enabled)}",
        "# Native event source (tovarisch-native, can be used for confirmed_leak)",
        f"native_events_enabled: {yaml_bool(config.native_events)}",
        f"native_events_path: \"{native_path_str}\"",
        f"native_disable_heartbeat: {yaml_bool(config.disable_heartbeat)}",
        f"native_disable_wg_checks: {yaml_bool(config.disable_wg_checks)}",
        f"native_disable_bgp: {yaml_bool(config.disable_bgp)}",
        f"native_disable_bfd: {yaml_bool(config.disable_bfd)}",
        f"tovarisch_binary: \"{config.tovarisch_binary}\"",
        f"binary_exists: {yaml_bool(config.tovarisch_binary.exists())}",
        f"commit_sha: {git_sha}",
        f"git_dirty: {git_dirty_count}",
        f"lab_start_iso: {datetime.now(timezone.utc).isoformat()}",
        "",
    ]

    manifest_path.write_text("\n".join(manifest_lines))
    return manifest_path


def write_memory_samples_header(config: LabRunConfig) -> Path:
    """Write memory_samples.tsv header."""
    path = config.artifact_path / "memory_samples.tsv"
    path.write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\tvmhwm_kib\tvmswap_kib\tvmpeak_kib\tvmrss_peak_kib\n")
    return path


def write_event_timeline_header(config: LabRunConfig) -> Path:
    """Write event_timeline.tsv header."""
    path = config.artifact_path / "event_timeline.tsv"
    path.write_text("timestamp\telapsed_sec\tevent\tsubsystem\tdetail\n")
    return path


def append_event(config: LabRunConfig, elapsed_seconds: int, event: str,
                 subsystem: str, detail: str = "") -> None:
    """Append an event to the synthetic event timeline."""
    ts = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3]
    path = config.artifact_path / "event_timeline.tsv"
    with open(path, "a") as f:
        f.write(f"{ts}\t{elapsed_seconds}\t{event}\t{subsystem}\t{detail}\n")


def append_memory_sample(config: LabRunConfig, elapsed_seconds: int, pid: int) -> None:
    """Read /proc status and append to memory_samples.tsv."""
    from lab_runner.proc import read_proc_status

    mem = read_proc_status(pid)
    rss = mem.get("VmRSS", 0)
    vmdata = mem.get("VmData", 0)
    vmhwm = mem.get("VmHWM", 0)
    vmswap = mem.get("VmSwap", 0)
    vmpeak = mem.get("VmPeak", 0)

    ts = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3]
    path = config.artifact_path / "memory_samples.tsv"
    with open(path, "a") as f:
        f.write(f"{ts}\t{elapsed_seconds}\t{rss}\t{vmdata}\t{vmhwm}\t{vmswap}\t{vmpeak}\t{vmhwm}\n")
