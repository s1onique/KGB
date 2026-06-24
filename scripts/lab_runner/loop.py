# lab_runner/loop.py - Idle sampling loop and status burst
import subprocess
import time
from pathlib import Path

from lab_runner.config import LabRunConfig, LabError
from lab_runner.artifacts import append_event, append_memory_sample


def run_idle_loop(config: LabRunConfig, process: subprocess.Popen) -> int:
    """Run the idle sampling loop. Returns sample count."""
    start_time = time.monotonic()
    sample_count = 0
    heartbeat_tick_count = 0
    last_heartbeat_log = 0
    last_wg_check = 0

    pid = process.pid

    if not (Path("/proc") / str(pid)).is_dir():
        raise LabError(f"/proc/{pid} disappeared before sampling started")

    print(f"Running idle for {config.duration} seconds...")

    while True:
        if process.poll() is not None:
            raise LabError(f"tovarisch exited unexpectedly during sampling (exit code: {process.returncode})")

        elapsed = int(time.monotonic() - start_time)

        proc_path = Path("/proc") / str(pid)
        if not proc_path.is_dir():
            raise LabError(f"/proc/{pid} disappeared while sampling; tovarisch may have exited")

        try:
            append_memory_sample(config, elapsed, pid)
            sample_count += 1
        except Exception as e:
            raise LabError(f"Failed to sample /proc/{pid}/status: {e}")

        # Synthetic heartbeat tick (every 30s)
        if config.heartbeat_enabled and elapsed > 0 and elapsed % 30 == 0 and elapsed != last_heartbeat_log:
            heartbeat_tick_count += 1
            append_event(config, elapsed, "heartbeat_tick", "heartbeat",
                        f"uptime={elapsed}s,tick={heartbeat_tick_count}")
            last_heartbeat_log = elapsed

        # Synthetic WG check (every 60s)
        if config.wg_check_enabled and elapsed > 0 and elapsed % 60 == 0 and elapsed != last_wg_check:
            append_event(config, elapsed, "wg_check", "wireguard", "periodic_60s_check")
            last_wg_check = elapsed

        # Synthetic BGP/BFD (every 10s)
        if config.bgp_bfd_enabled and elapsed > 0 and elapsed % 10 == 0:
            append_event(config, elapsed, "bgp_maintenance", "bgp", "periodic_maintenance")
            append_event(config, elapsed, "bfd_tick", "bfd", "periodic_tick")

        if elapsed >= config.duration:
            break

        time.sleep(config.interval)

    return sample_count


def run_status_burst(config: LabRunConfig, process: subprocess.Popen) -> None:
    """Run /status burst test."""
    print("Running /status burst test...")
    pid = process.pid

    append_event(config, int(time.monotonic()), "status_burst_start", "status", "5000 requests")
    burst_start = time.monotonic()

    for _ in range(5000):
        try:
            subprocess.run(
                ["curl", "-s", f"http://{config.lab_bind}:{config.lab_port}/status"],
                capture_output=True,
                timeout=5,
            )
        except Exception:
            pass

    final_elapsed = int(time.monotonic())
    append_memory_sample(config, final_elapsed, pid)
    burst_duration = int(time.monotonic() - burst_start)
    append_event(config, final_elapsed, "status_burst_complete", "status",
                f"duration={burst_duration}s")
