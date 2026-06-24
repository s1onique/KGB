# self_tests.py — Self-test fixtures and runner
"""Self-tests for verifier functionality."""

import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Callable, Optional

from .artifact_checks import verify_artifact
from .test_fixtures import (
    complete_artifact,
    confirmed_leak_artifact,
    minimal_events,
    minimal_manifest,
    minimal_samples,
    minimal_verdict,
    shell_only_artifact,
)
from .native_event_tests import run_native_event_tests


def run_cli_wrapper_smoke_test() -> bool:
    """Smoke test that exercises the public CLI wrapper via subprocess."""
    import os

    wrapper = Path(__file__).resolve().parents[1] / "verify_idle_staircase_artifact.py"

    with tempfile.TemporaryDirectory() as tmpdir:
        artifact_path = Path(tmpdir) / "smoke_artifact"
        artifact_path.mkdir()
        complete_artifact(artifact_path)

        result = subprocess.run(
            [sys.executable, str(wrapper), str(artifact_path)],
            text=True,
            capture_output=True,
            cwd=os.getcwd(),
        )

        return result.returncode == 0 and "OK" in result.stdout


def run_self_tests() -> bool:
    """Run self-tests on the verifier."""
    print("Running self-tests...")

    tests_passed = 0
    tests_failed = 0

    def test(name: str, should_pass: bool, setup_fn: Optional[Callable] = None) -> bool:
        nonlocal tests_passed, tests_failed

        with tempfile.TemporaryDirectory() as tmpdir:
            artifact_path = Path(tmpdir) / "test_artifact"
            artifact_path.mkdir()

            if setup_fn:
                setup_fn(artifact_path)

            valid, error = verify_artifact(artifact_path)

            if should_pass and valid:
                print(f"  PASS: {name}")
                tests_passed += 1
                return True
            elif not should_pass and not valid:
                print(f"  PASS: {name} (correctly rejected)")
                tests_passed += 1
                return True
            else:
                print(f"  FAIL: {name}")
                if error:
                    print(f"        Error: {error}")
                tests_failed += 1
                return False

    # Test 1: Missing all files
    test("missing all files", False, None)

    # Test 2: Missing memory samples
    def setup_missing_samples(p):
        minimal_manifest(p)
        minimal_events(p)
        minimal_verdict(p)

    test("missing memory_samples.tsv", False, setup_missing_samples)

    # Test 3: Missing events
    def setup_missing_events(p):
        minimal_manifest(p)
        minimal_samples(p)
        minimal_verdict(p)

    test("missing event_timeline.tsv", False, setup_missing_events)

    # Test 4: Invalid verdict
    def setup_invalid_verdict(p):
        minimal_manifest(p)
        minimal_samples(p)
        minimal_events(p)
        (p / "verdict.txt").write_text("verdict: invalid_verdict\n")

    test("invalid verdict", False, setup_invalid_verdict)

    # Test 5: confirmed_leak without owner
    def setup_leak_no_owner(p):
        complete_artifact(p)
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\nreason: test\n"
            "steps_detected: 5\ntotal_growth_kib: 1000\n"
        )

    test("confirmed_leak without owner", False, setup_leak_no_owner)

    # Test 6: confirmed_leak owner=unknown
    def setup_leak_owner_unknown(p):
        complete_artifact(p)
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\nowner: unknown\nreason: test\n"
            "steps_detected: 5\ntotal_growth_kib: 1000\n"
        )

    test("confirmed_leak owner=unknown", False, setup_leak_owner_unknown)

    # Test 7: confirmed_leak with owner but no event evidence
    def setup_leak_no_event_evidence(p):
        complete_artifact(p)
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\nowner: heartbeat\nreason: test\n"
            "steps_detected: 5\ntotal_growth_kib: 1000\n"
            "owner_evidence: test evidence\ncorrelated_events: heartbeat=0\n"
        )

    test("confirmed_leak with owner but no event evidence", False, setup_leak_no_event_evidence)

    # Test 8: confirmed_leak without correlated events
    def setup_leak_no_correlated(p):
        complete_artifact(p)
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\nowner: heartbeat\nreason: test\n"
            "steps_detected: 5\ntotal_growth_kib: 1000\n"
            "owner_evidence: test evidence\ncorrelated_events: heartbeat=0\n"
        )

    test("confirmed_leak without correlated events", False, setup_leak_no_correlated)

    # Test 9: Valid confirmed_leak with native events
    test("valid confirmed_leak with native events", True, confirmed_leak_artifact)

    # Test 9a: steps_detected < 3 threshold
    def setup_leak_steps_below_threshold(p):
        complete_artifact(p)
        (p / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:00:30\t30\t1200\t2200\n"
            "2024-01-01T00:02:00\t120\t1200\t2200\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\nowner: heartbeat\nreason: test\n"
            "steps_detected: 2\ntotal_growth_kib: 600\n"
            "owner_evidence: test\ncorrelated_events: heartbeat=1\n"
        )

    test("confirmed_leak rejected for steps_detected < 3", False, setup_leak_steps_below_threshold)

    # Test 9b: total_growth_kib <= 500 threshold
    def setup_leak_growth_below_threshold(p):
        complete_artifact(p)
        (p / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:00:30\t30\t1100\t2100\n"
            "2024-01-01T00:01:00\t60\t1200\t2200\n"
            "2024-01-01T00:01:30\t90\t1300\t2300\n"
            "2024-01-01T00:02:00\t120\t1300\t2300\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\nowner: heartbeat\nreason: test\n"
            "steps_detected: 3\ntotal_growth_kib: 300\n"
            "owner_evidence: test\ncorrelated_events: heartbeat=3\n"
        )

    test("confirmed_leak rejected for total_growth_kib <= 500", False, setup_leak_growth_below_threshold)

    # Test 9c: Shell-side synthetic events MUST be rejected
    def setup_shell_synthetic_rejected(p):
        shell_only_artifact(p)
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\nowner: heartbeat\nreason: test\n"
            "steps_detected: 1\ntotal_growth_kib: 100\n"
            "owner_evidence: test\ncorrelated_events: heartbeat=1\n"
        )

    test("shell_side synthetic rejected for confirmed_leak", False, setup_shell_synthetic_rejected)

    # Test 10: bounded verdict without evidence
    def setup_bounded_no_evidence(p):
        complete_artifact(p)
        (p / "verdict.txt").write_text("verdict: bounded_warmup_or_allocator_highwater\n")

    test("bounded verdict without evidence", False, setup_bounded_no_evidence)

    # Test 11: bounded verdict with excessive growth
    def setup_bounded_excessive_growth(p):
        complete_artifact(p)
        (p / "verdict.txt").write_text(
            "verdict: bounded_warmup_or_allocator_highwater\n"
            "steps_detected: 2\ntotal_growth_kib: 3000\n"
        )

    test("bounded verdict with excessive growth", False, setup_bounded_excessive_growth)

    # Test 12: bounded verdict with excessive steps
    def setup_bounded_excessive_steps(p):
        complete_artifact(p)
        (p / "verdict.txt").write_text(
            "verdict: bounded_warmup_or_allocator_highwater\n"
            "steps_detected: 15\ntotal_growth_kib: 500\n"
        )

    test("bounded verdict with excessive steps", False, setup_bounded_excessive_steps)

    # Test 13: valid bounded verdict
    def setup_valid_bounded(p):
        complete_artifact(p)
        (p / "verdict.txt").write_text(
            "verdict: bounded_warmup_or_allocator_highwater\n"
            "reason: test\nsteps_detected: 2\ntotal_growth_kib: 100\n"
        )

    test("valid bounded verdict", True, setup_valid_bounded)

    # Test 14: inconclusive staircase with owner-unattributed reason
    def setup_inconclusive_staircase_unknown(p):
        complete_artifact(p)
        (p / "verdict.txt").write_text(
            "verdict: inconclusive\n"
            "reason: Staircase growth detected but owner is unattributed. Event correlation required.\n"
            "steps_detected: 5\ntotal_growth_kib: 600\ngrowth_rate_kib_per_min: 10\nowner:\n"
        )

    test("inconclusive staircase with owner-unattributed reason", True, setup_inconclusive_staircase_unknown)

    # Test 15: inconclusive staircase without owner explanation
    def setup_inconclusive_staircase_no_reason(p):
        complete_artifact(p)
        (p / "verdict.txt").write_text(
            "verdict: inconclusive\nreason: Growth pattern unclear\n"
            "steps_detected: 5\ntotal_growth_kib: 600\ngrowth_rate_kib_per_min: 10\n"
        )

    test("inconclusive staircase without owner explanation", False, setup_inconclusive_staircase_no_reason)

    # Test 16: valid basic inconclusive
    test("valid basic inconclusive artifact", True, complete_artifact)

    # Test 17: event timeline header only
    def setup_events_header_only(p):
        complete_artifact(p)
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n")

    test("event timeline header only", False, setup_events_header_only)

    # Test 18: event timeline missing lab_started
    def setup_events_no_lab_started(p):
        complete_artifact(p)
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:30\t30\theartbeat_tick\theartbeat\n"
            "2024-01-01T00:01:40\t100\tidle_complete\tlab\n"
        )

    test("event timeline missing lab_started", False, setup_events_no_lab_started)

    # Test 19: event timeline missing terminal event
    def setup_events_no_terminal(p):
        complete_artifact(p)
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:30\t30\theartbeat_tick\theartbeat\n"
        )

    test("event timeline missing terminal event", False, setup_events_no_terminal)

    # Test 20: event timeline with wrong column count
    def setup_events_wrong_cols(p):
        complete_artifact(p)
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\n"
            "2024-01-01T00:01:40\t100\tidle_complete\tlab\n"
        )

    test("event timeline with wrong column count", False, setup_events_wrong_cols)

    # Test 21: event timeline with shutdown as terminal
    def setup_events_shutdown_terminal(p):
        complete_artifact(p)
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:01:40\t100\tshutdown\tlab\n"
        )

    test("event timeline with shutdown as terminal", True, setup_events_shutdown_terminal)

    # Test 22: event timeline with lab_failed as terminal
    def setup_events_lab_failed_terminal(p):
        complete_artifact(p)
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:50\t50\tlab_failed\terror\n"
        )

    test("event timeline with lab_failed as terminal", True, setup_events_lab_failed_terminal)

    # Test 23: subsystem_config marker rejects confirmed_leak
    def setup_subsystem_config(p):
        complete_artifact(p)
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:30\t30\tsubsystem_config\tlab\n"
            "2024-01-01T00:01:40\t100\tidle_complete\tlab\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\nowner: heartbeat\nreason: test\n"
            "steps_detected: 3\ntotal_growth_kib: 600\ngrowth_rate_kib_per_min: 10\n"
            "owner_evidence: test\ncorrelated_events: heartbeat=1\n"
        )

    test("confirmed_leak rejected for subsystem_config marker", False, setup_subsystem_config)

    # Test 24: empty owner_evidence rejected
    def setup_leak_empty_evidence(p):
        complete_artifact(p)
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:30\t30\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:01:40\t100\tidle_complete\tlab\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\nowner: heartbeat\nreason: test\n"
            "steps_detected: 3\ntotal_growth_kib: 600\ngrowth_rate_kib_per_min: 10\n"
            "owner_evidence:\ncorrelated_events: heartbeat=1\n"
        )

    test("confirmed_leak rejected for empty owner_evidence", False, setup_leak_empty_evidence)

    # Test 25: CLI wrapper smoke test via subprocess
    if run_cli_wrapper_smoke_test():
        print("  PASS: CLI wrapper smoke test via subprocess")
        tests_passed += 1
    else:
        print("  FAIL: CLI wrapper smoke test via subprocess")
        tests_failed += 1

    # Tests 26-31: Native event validation tests (delegated to external module)
    print("\nRunning native event tests...")
    native_passed, native_failed = run_native_event_tests()
    tests_passed += native_passed
    tests_failed += native_failed

    print(f"\nSelf-test results: {tests_passed} passed, {tests_failed} failed")
    return tests_failed == 0
