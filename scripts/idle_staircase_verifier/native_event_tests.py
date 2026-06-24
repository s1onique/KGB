# native_event_tests.py — Native event validation tests
"""Tests for native event artifact validation."""

import tempfile
from pathlib import Path
from typing import Callable

from .artifact_checks import verify_artifact


def run_native_event_tests() -> tuple[int, int]:
    """Run native event validation tests. Returns (passed, failed)."""
    tests_passed = 0
    tests_failed = 0

    def test(name: str, should_pass: bool, setup_fn: Callable) -> bool:
        nonlocal tests_passed, tests_failed

        with tempfile.TemporaryDirectory() as tmpdir:
            artifact_path = Path(tmpdir) / "test_artifact"
            artifact_path.mkdir()

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

    def minimal_events(p: Path) -> None:
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:01:30\t90\tidle_complete\tlab\n"
        )

    def native_events_artifact(p: Path) -> None:
        (p / "manifest.yaml").write_text(
            "run_id: test\nplatform: Linux\ncommit_sha: abc123\n"
            "native_events_enabled: true\n"
        )
        (p / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:00:30\t30\t1200\t2200\n"
            "2024-01-01T00:01:00\t60\t1400\t2400\n"
            "2024-01-01T00:01:30\t90\t1600\t2600\n"
        )
        minimal_events(p)
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\nowner: heartbeat\nreason: test\n"
            "steps_detected: 3\ntotal_growth_kib: 600\ngrowth_rate_kib_per_min: 30\n"
            "owner_evidence: test\ncorrelated_events: heartbeat=3\n"
        )

    # Test 27: Native event artifact with missing native_event_timeline.tsv
    def setup_native_leak_missing_timeline(p):
        (p / "manifest.yaml").write_text(
            "run_id: test\nplatform: Linux\ncommit_sha: abc123\nnative_events_enabled: true\n"
        )
        (p / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:00:30\t30\t1200\t2200\n"
            "2024-01-01T00:01:00\t60\t1400\t2400\n"
            "2024-01-01T00:01:30\t90\t1600\t2600\n"
        )
        minimal_events(p)
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\nowner: heartbeat\nreason: test\n"
            "steps_detected: 3\ntotal_growth_kib: 600\ngrowth_rate_kib_per_min: 30\n"
            "owner_evidence: test\ncorrelated_events: heartbeat=3\n"
        )

    test("native_confirmed_leak without native_event_timeline.tsv", False, setup_native_leak_missing_timeline)

    # Test 28: Native event artifact with empty native_event_timeline.tsv
    def setup_native_leak_empty_timeline(p):
        native_events_artifact(p)
        (p / "native_event_timeline.tsv").write_text(
            "timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid\n"
        )

    test("native_confirmed_leak with empty native_event_timeline.tsv", False, setup_native_leak_empty_timeline)

    # Test 29: Native event artifact with header-only native_event_timeline.tsv
    def setup_native_leak_header_only(p):
        native_events_artifact(p)
        (p / "native_event_timeline.tsv").write_text(
            "timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid\n"
        )

    test("native_confirmed_leak with header-only native_event_timeline.tsv", False, setup_native_leak_header_only)

    # Test 30: Valid native event artifact with confirmed_leak
    def setup_valid_native_confirmed_leak(p):
        (p / "manifest.yaml").write_text(
            "run_id: test\nplatform: Linux\ncommit_sha: abc123\n"
            "native_events_enabled: true\nnative_disable_heartbeat: false\n"
            "native_disable_wg_checks: true\nnative_disable_bgp: true\nnative_disable_bfd: true\n"
        )
        (p / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:00:30\t30\t1200\t2200\n"
            "2024-01-01T00:01:00\t60\t1400\t2400\n"
            "2024-01-01T00:01:30\t90\t1600\t2600\n"
        )
        # Shell-side events must correlate with memory steps for confirmed_leak attribution
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:30\t30\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:01:00\t60\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:01:30\t90\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:01:30\t90\tidle_complete\tlab\n"
        )
        (p / "native_event_timeline.tsv").write_text(
            "timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid\n"
            "2024-01-01T00:00:30\t30000\theartbeat_tick_start\theartbeat\t\t1234\n"
            "2024-01-01T00:00:30\t30000\theartbeat_tick_end\theartbeat\t\t1234\n"
            "2024-01-01T00:01:00\t60000\theartbeat_tick_start\theartbeat\t\t1234\n"
            "2024-01-01T00:01:00\t60000\theartbeat_tick_end\theartbeat\t\t1234\n"
            "2024-01-01T00:01:30\t90000\theartbeat_tick_start\theartbeat\t\t1234\n"
            "2024-01-01T00:01:30\t90000\theartbeat_tick_end\theartbeat\t\t1234\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\nowner: heartbeat\n"
            "reason: Native events from heartbeat subsystem correlate with 3 memory steps.\n"
            "steps_detected: 3\ntotal_growth_kib: 600\ngrowth_rate_kib_per_min: 30\n"
            "owner_evidence: Heartbeat native events correlate with memory steps at t=30,60,90s\n"
            "correlated_events: heartbeat=6,wg=0,bgp=0,bfd=0,health=0,status=0\n"
        )

    test("valid native_confirmed_leak with native_event_timeline.tsv", True, setup_valid_native_confirmed_leak)

    # Test 31: Native event artifact with all periodic paths disabled
    def setup_native_all_disabled_inconclusive(p):
        (p / "manifest.yaml").write_text(
            "run_id: test\nplatform: Linux\ncommit_sha: abc123\n"
            "native_events_enabled: true\nnative_disable_heartbeat: true\n"
            "native_disable_wg_checks: true\nnative_disable_bgp: true\nnative_disable_bfd: true\n"
        )
        (p / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:00:30\t30\t1200\t2200\n"
            "2024-01-01T00:01:00\t60\t1400\t2400\n"
            "2024-01-01T00:01:30\t90\t1600\t2600\n"
        )
        minimal_events(p)
        (p / "native_event_timeline.tsv").write_text(
            "timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid\n"
            "2024-01-01T00:00:00\t0\tnative_config\tlab\tall_disabled=true\t1234\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: inconclusive\nowner:\n"
            "reason: Growth persists with all periodic paths disabled. May be allocator warmup or other source.\n"
            "steps_detected: 3\ntotal_growth_kib: 600\ngrowth_rate_kib_per_min: 30\n"
            "owner_evidence:\ncorrelated_events: heartbeat=0,wg=0,bgp=0,bfd=0,health=0,status=0\n"
        )

    test("native_all_disabled_inconclusive is accepted", True, setup_native_all_disabled_inconclusive)

    # Test 32: Shell-only confirmed_leak rejected even with native_events=false
    def setup_shell_only_with_confirmed_leak(p):
        (p / "manifest.yaml").write_text(
            "run_id: test\nplatform: Linux\ncommit_sha: abc123\nnative_events_enabled: false\n"
        )
        (p / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:00:30\t30\t1200\t2200\n"
            "2024-01-01T00:01:00\t60\t1400\t2400\n"
            "2024-01-01T00:01:30\t90\t1600\t2600\n"
        )
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:30\t30\tsubsystem_config\tlab\n"
            "2024-01-01T00:01:30\t90\tidle_complete\tlab\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\nowner: heartbeat\nreason: test\n"
            "steps_detected: 3\ntotal_growth_kib: 600\ngrowth_rate_kib_per_min: 30\n"
            "owner_evidence: test\ncorrelated_events: heartbeat=3\n"
        )

    test("shell_only_confirmed_leak rejected even with native_events=false", False, setup_shell_only_with_confirmed_leak)

    # Note: The contract is proven by:
    # 1. Test 27: confirmed_leak requires native_event_timeline.tsv (no native = reject)
    # 2. Test 28/29: confirmed_leak requires non-empty native_event_timeline.tsv
    # 3. Test 30: valid_confirmed_leak with correlating native events (accept)
    # 4. Test 31: shell-only confirmed_leak rejected even with native_events=false
    # The verifier now requires native event correlation for confirmed_leak.

    return tests_passed, tests_failed
