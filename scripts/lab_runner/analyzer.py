# lab_runner/analyzer.py - Analyzer invocation
import subprocess
import sys

from lab_runner.config import LabRunConfig, LabError


def run_analyzer(config: LabRunConfig) -> None:
    """Run the idle staircase analyzer CLI."""
    analyzer_script = config.script_dir / "idle_staircase_analyzer_cli.py"
    if not analyzer_script.exists():
        raise LabError(f"analyzer script not found: {analyzer_script}")

    native_timeline = config.artifact_path / "native_event_timeline.tsv"
    native_timeline_arg = str(native_timeline) if config.native_events else ""

    result = subprocess.run(
        [
            "python3",
            str(analyzer_script),
            str(config.artifact_path / "memory_samples.tsv"),
            str(config.artifact_path),
            str(config.artifact_path / "event_timeline.tsv"),
            "--duration", str(config.duration),
            "--heartbeat-enabled", str(config.heartbeat_enabled).lower(),
            "--wg-enabled", str(config.wg_check_enabled).lower(),
            "--bgp-bfd-enabled", str(config.bgp_bfd_enabled).lower(),
            "--native-events", str(config.native_events).lower(),
        ] + ([] if not native_timeline_arg else ["--native-event-timeline", native_timeline_arg]),
        text=True,
        capture_output=True,
        cwd=str(config.script_dir),
    )

    if result.returncode != 0:
        print("ERROR: analyzer exited with non-zero status")
        print(f"analyzer exit code: {result.returncode}")
        print("analyzer stdout:")
        print(result.stdout)
        print("analyzer stderr:", file=sys.stderr)
        print(result.stderr, file=sys.stderr)
        raise LabError("analyzer failed")

    print(result.stdout)
