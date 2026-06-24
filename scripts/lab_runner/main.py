# lab_runner/main.py - Main entry point
import sys
import shutil
from pathlib import Path

from lab_runner.config import parse_args, LabError
from lab_runner.proc import require_linux_proc
from lab_runner.artifacts import (
    write_tovarisch_config, print_tovarisch_config, write_manifest,
    write_memory_samples_header, write_event_timeline_header,
    append_event,
)
from lab_runner.tovarisch import start_tovarisch, wait_for_tovarisch_ready, terminate_process
from lab_runner.loop import run_idle_loop, run_status_burst
from lab_runner.analyzer import run_analyzer
from lab_runner.validation import verify_required_outputs, print_final_summary
from lab_runner.self_tests import run_self_tests


def copy_native_timeline(config) -> None:
    """Copy native event timeline to artifact path if needed."""
    if not config.native_events or not config.native_events_path:
        return

    src = config.native_events_path
    dst = config.artifact_path / "native_event_timeline.tsv"

    if not src.exists():
        return

    if src.resolve() == dst.resolve():
        return

    shutil.copy2(src, dst)


def main(argv: list[str]) -> int:
    """Main entry point."""
    try:
        if "--self-test" in argv:
            success = run_self_tests()
            return 0 if success else 1

        config = parse_args(argv)

        print("=== Idle Staircase Memory Lab ===")
        print(f"Platform: Linux, Duration: {config.duration}s, Interval: {config.interval}s")
        print()
        print("Shell-side synthetic event toggles (NO actual runtime effect):")
        print(f"  Heartbeat: {config.heartbeat_enabled}, WG: {config.wg_check_enabled}, BGP/BFD: {config.bgp_bfd_enabled}")
        print()
        print(f"Tovarisch-native event emission: {config.native_events}")
        print("Native runtime toggles (REAL runtime effect):")
        print(f"  Disable heartbeat: {config.disable_heartbeat}")
        print(f"  Disable WG checks: {config.disable_wg_checks}")
        print(f"  Disable BGP: {config.disable_bgp}")
        print(f"  Disable BFD: {config.disable_bfd}")

        require_linux_proc()

        config.artifact_path.mkdir(parents=True, exist_ok=True)
        print(f"Artifact: {config.artifact_path}")

        write_tovarisch_config(config)
        print_tovarisch_config(config)
        write_manifest(config)
        write_memory_samples_header(config)
        write_event_timeline_header(config)

        print(f"Starting tovarisch on {config.lab_bind}:{config.lab_port}...")
        process = start_tovarisch(config)
        wait_for_tovarisch_ready(config, process)

        try:
            append_event(config, 0, "lab_started", "lab", f"PID={process.pid}")
            append_event(config, 0, "subsystem_config", "lab",
                        f"heartbeat={config.heartbeat_enabled},wg={config.wg_check_enabled},bgp_bfd={config.bgp_bfd_enabled}")

            if config.native_events:
                append_event(config, 0, "native_config", "lab",
                            f"events={config.native_events},heartbeat={config.disable_heartbeat},wg={config.disable_wg_checks},bgp={config.disable_bgp},bfd={config.disable_bfd}")

            sample_count = run_idle_loop(config, process)

            if config.status_burst:
                run_status_burst(config, process)

            append_event(config, config.duration, "idle_complete", "lab", f"sampled={sample_count} times")
            append_event(config, config.duration, "shutdown", "lab", "stopping")

        finally:
            terminate_process(process)
            copy_native_timeline(config)

        print()
        print("Analyzing memory samples...")
        run_analyzer(config)

        verify_required_outputs(config)
        print_final_summary(config)

        return 0

    except LabError as e:
        print(f"ERROR: {e}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("Interrupted", file=sys.stderr)
        return 130
