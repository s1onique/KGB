# lab_runner/validation.py - Output verification and summary
from lab_runner.config import LabRunConfig, LabError


def verify_required_outputs(config: LabRunConfig) -> None:
    """Verify required outputs exist."""
    errors = []

    verdict_path = config.artifact_path / "verdict.txt"
    if not verdict_path.exists():
        errors.append("verdict.txt not found")
    elif verdict_path.stat().st_size == 0:
        errors.append("verdict.txt is empty")

    manifest_path = config.artifact_path / "manifest.yaml"
    if not manifest_path.exists():
        errors.append("manifest.yaml not found")

    samples_path = config.artifact_path / "memory_samples.tsv"
    if not samples_path.exists():
        errors.append("memory_samples.tsv not found")

    events_path = config.artifact_path / "event_timeline.tsv"
    if not events_path.exists():
        errors.append("event_timeline.tsv not found")

    # Native event validation
    native_timeline = config.artifact_path / "native_event_timeline.tsv"
    if config.native_events and not config.disable_heartbeat:
        if not native_timeline.exists():
            errors.append(
                f"native events were enabled for heartbeat-native-enabled-smoke, "
                f"but native_event_timeline.tsv was not created.\n"
                f"Expected: {native_timeline}\n"
                f"Generated config:\n{config.artifact_path}/tovarisch_lab.conf"
            )
        elif native_timeline.stat().st_size == 0:
            errors.append(
                f"native_event_timeline.tsv exists but is empty.\n"
                f"Expected non-empty heartbeat events."
            )

    if errors:
        raise LabError("\n".join(errors))


def print_final_summary(config: LabRunConfig) -> None:
    """Print final lab summary."""
    verdict_path = config.artifact_path / "verdict.txt"

    print()
    print("=== Lab Complete ===")
    print(f"Artifact: {config.artifact_path}")

    if verdict_path.exists():
        verdict = verdict_path.read_text()
        print(f"Verdict:\n{verdict}")
    else:
        print("Verdict: (not available)")

    print()
