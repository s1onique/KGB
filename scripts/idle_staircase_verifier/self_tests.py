# self_tests.py — Self-test fixtures and runner
"""Self-tests for verifier functionality."""

import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Callable, Optional

from .artifact_checks import verify_artifact


def run_cli_wrapper_smoke_test() -> bool:
    """Smoke test that exercises the public CLI wrapper via subprocess."""
    import os
    
    wrapper = Path(__file__).resolve().parents[1] / "verify_idle_staircase_artifact.py"
    
    with tempfile.TemporaryDirectory() as tmpdir:
        artifact_path = Path(tmpdir) / "smoke_artifact"
        artifact_path.mkdir()
        
        # Create a valid artifact
        (artifact_path / "manifest.yaml").write_text(
            "run_id: smoke_test\nplatform: Linux\ncommit_sha: abc\n"
        )
        (artifact_path / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n"
        )
        (artifact_path / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:01:40\t100\tidle_complete\tlab\n"
        )
        (artifact_path / "verdict.txt").write_text(
            "verdict: inconclusive\nreason: smoke test\n"
        )
        
        # Run the public CLI wrapper
        result = subprocess.run(
            [sys.executable, str(wrapper), str(artifact_path)],
            text=True,
            capture_output=True,
            cwd=os.getcwd(),
        )
        
        if result.returncode == 0 and "OK" in result.stdout:
            return True
        return False


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
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("missing memory_samples.tsv", False, setup_missing_samples)
    
    # Test 3: Missing events
    def setup_missing_events(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("missing event_timeline.tsv", False, setup_missing_events)
    
    # Test 4: Invalid verdict
    def setup_invalid_verdict(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: invalid_verdict\n")
    test("invalid verdict", False, setup_invalid_verdict)
    
    # Test 5: confirmed_leak without owner
    def setup_leak_no_owner(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: confirmed_leak\nreason: test\nsteps_detected: 5\ntotal_growth_kib: 1000\ngrowth_rate_kib_per_min: 10\n")
    test("confirmed_leak without owner", False, setup_leak_no_owner)
    
    # Test 6: confirmed_leak with owner=unknown
    def setup_leak_owner_unknown(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: confirmed_leak\nowner: unknown\nreason: test\nsteps_detected: 5\ntotal_growth_kib: 1000\ngrowth_rate_kib_per_min: 10\n")
    test("confirmed_leak owner=unknown", False, setup_leak_owner_unknown)
    
    # Test 7: confirmed_leak with owner but no matching event evidence
    def setup_leak_no_event_evidence(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: confirmed_leak\nowner: heartbeat\nreason: test\nsteps_detected: 5\ntotal_growth_kib: 1000\ngrowth_rate_kib_per_min: 10\nowner_evidence: test evidence\ncorrelated_events: heartbeat=0\n")
    test("confirmed_leak with owner but no event evidence", False, setup_leak_no_event_evidence)
    
    # Test 8: confirmed_leak with owner and event evidence but no correlated events
    def setup_leak_no_correlated(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: confirmed_leak\nowner: heartbeat\nreason: test\nsteps_detected: 5\ntotal_growth_kib: 1000\ngrowth_rate_kib_per_min: 10\nowner_evidence: test evidence\ncorrelated_events: heartbeat=0\n")
    test("confirmed_leak without correlated events near memory steps", False, setup_leak_no_correlated)
    
    # Test 9: valid confirmed_leak with tovarisch-native events
    def setup_valid_leak(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:00:30\t30\t1200\t2200\n"
            "2024-01-01T00:01:00\t60\t1400\t2400\n"
            "2024-01-01T00:01:30\t90\t1600\t2600\n"
            "2024-01-01T00:02:00\t120\t1600\t2600\n"
        )
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:30\t30\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:01:00\t60\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:01:30\t90\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:02:00\t120\tidle_complete\tlab\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\n"
            "owner: heartbeat\n"
            "reason: Dominant heartbeat subsystem with 3 events correlated with memory steps\n"
            "steps_detected: 3\n"
            "total_growth_kib: 600\n"
            "growth_rate_kib_per_min: 30\n"
            "owner_evidence: Heartbeat events at t=30,60,90 correlate exactly with memory steps at same timestamps\n"
            "correlated_events: heartbeat=3,wg=0,bgp=0,bfd=0,health=0,status=0\n"
        )
    test("valid confirmed_leak with tovarisch-native events", True, setup_valid_leak)
    
    # Test 9a: confirmed_leak rejected for steps_detected < 3
    def setup_leak_steps_below_threshold(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:00:30\t30\t1200\t2200\n"
            "2024-01-01T00:02:00\t120\t1200\t2200\n"
        )
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:30\t30\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:02:00\t120\tidle_complete\tlab\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\n"
            "owner: heartbeat\n"
            "reason: test\n"
            "steps_detected: 2\n"
            "total_growth_kib: 600\n"
            "growth_rate_kib_per_min: 10\n"
            "owner_evidence: test\n"
            "correlated_events: heartbeat=1\n"
        )
    test("confirmed_leak rejected for steps_detected < 3", False, setup_leak_steps_below_threshold)
    
    # Test 9b: confirmed_leak rejected for total_growth_kib <= 500
    def setup_leak_growth_below_threshold(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:00:30\t30\t1100\t2100\n"
            "2024-01-01T00:01:00\t60\t1200\t2200\n"
            "2024-01-01T00:01:30\t90\t1300\t2300\n"
            "2024-01-01T00:02:00\t120\t1300\t2300\n"
        )
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:30\t30\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:01:00\t60\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:01:30\t90\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:02:00\t120\tidle_complete\tlab\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\n"
            "owner: heartbeat\n"
            "reason: test\n"
            "steps_detected: 3\n"
            "total_growth_kib: 300\n"
            "growth_rate_kib_per_min: 10\n"
            "owner_evidence: test\n"
            "correlated_events: heartbeat=3\n"
        )
    test("confirmed_leak rejected for total_growth_kib <= 500", False, setup_leak_growth_below_threshold)
    
    # Test 9c: confirmed_leak with shell-side synthetic events MUST be rejected
    def setup_leak_shell_synthetic_rejected(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
            "2024-01-01T00:00:00\t0\t1000\t2000\n"
            "2024-01-01T00:00:30\t30\t1100\t2100\n"
            "2024-01-01T00:01:40\t100\t1100\t2100\n"
        )
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:30\t30\theartbeat_tick\tlab\n"
            "2024-01-01T00:01:40\t100\tidle_complete\tlab\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\n"
            "owner: heartbeat\n"
            "reason: test\n"
            "steps_detected: 1\n"
            "total_growth_kib: 100\n"
            "growth_rate_kib_per_min: 10\n"
            "owner_evidence: test\n"
            "correlated_events: heartbeat=1\n"
        )
    test("confirmed_leak with shell-side synthetic events rejected", False, setup_leak_shell_synthetic_rejected)
    
    # Test 10: bounded verdict without evidence
    def setup_bounded_no_evidence(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: bounded_warmup_or_allocator_highwater\n")
    test("bounded verdict without evidence", False, setup_bounded_no_evidence)
    
    # Test 11: bounded verdict with excessive growth
    def setup_bounded_excessive_growth(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: bounded_warmup_or_allocator_highwater\nsteps_detected: 2\ntotal_growth_kib: 3000\n")
    test("bounded verdict with excessive growth (>2000 KiB)", False, setup_bounded_excessive_growth)
    
    # Test 12: bounded verdict with excessive steps
    def setup_bounded_excessive_steps(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: bounded_warmup_or_allocator_highwater\nsteps_detected: 15\ntotal_growth_kib: 500\n")
    test("bounded verdict with excessive steps (>10)", False, setup_bounded_excessive_steps)
    
    # Test 13: valid bounded verdict
    def setup_valid_bounded(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: bounded_warmup_or_allocator_highwater\nreason: test\nsteps_detected: 2\ntotal_growth_kib: 100\n")
    test("valid bounded verdict", True, setup_valid_bounded)
    
    # Test 14: inconclusive with owner-unknown staircase and proper reason
    def setup_inconclusive_staircase_unknown(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: Staircase growth detected but owner is unattributed. Event correlation required.\nsteps_detected: 5\ntotal_growth_kib: 600\ngrowth_rate_kib_per_min: 10\nowner:\n")
    test("inconclusive staircase with owner-unattributed reason", True, setup_inconclusive_staircase_unknown)
    
    # Test 15: inconclusive staircase without proper reason
    def setup_inconclusive_staircase_no_reason(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: Growth pattern unclear\nsteps_detected: 5\ntotal_growth_kib: 600\ngrowth_rate_kib_per_min: 10\n")
    test("inconclusive staircase without owner explanation", False, setup_inconclusive_staircase_no_reason)
    
    # Test 16: valid basic inconclusive
    def setup_valid_inconclusive(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("valid basic inconclusive artifact", True, setup_valid_inconclusive)
    
    # Test 17: event timeline with header only (no data rows)
    def setup_events_header_only(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline header only", False, setup_events_header_only)
    
    # Test 18: event timeline missing lab_started
    def setup_events_no_lab_started(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:30\t30\theartbeat_tick\theartbeat\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline missing lab_started", False, setup_events_no_lab_started)
    
    # Test 19: event timeline missing terminal event
    def setup_events_no_terminal(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:00:30\t30\theartbeat_tick\theartbeat\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline missing terminal event", False, setup_events_no_terminal)
    
    # Test 20: event timeline with wrong column count
    def setup_events_wrong_cols(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline with wrong column count", False, setup_events_wrong_cols)
    
    # Test 21: event timeline with shutdown as terminal
    def setup_events_shutdown_terminal(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tshutdown\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline with shutdown as terminal", True, setup_events_shutdown_terminal)
    
    # Test 22: event timeline with lab_failed as terminal
    def setup_events_lab_failed_terminal(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:00:50\t50\tlab_failed\terror\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("event timeline with lab_failed as terminal", True, setup_events_lab_failed_terminal)
    
    # Test 23: CLI smoke test (exercise public wrapper path)
    def setup_cli_smoke(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text("timestamp\telapsed_sec\tevent\tsubsystem\n2024-01-01T00:00:00\t0\tlab_started\tlab\n2024-01-01T00:01:40\t100\tidle_complete\tlab\n")
        (p / "verdict.txt").write_text("verdict: inconclusive\nreason: test\n")
    test("valid artifact via internal verify_artifact (CLI smoke)", True, setup_cli_smoke)
    
    # Test 24: manifest with subsystem_config triggers shell-synthetic detection
    def setup_subsystem_config(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:30\t30\tsubsystem_config\tlab\n"
            "2024-01-01T00:01:40\t100\tidle_complete\tlab\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\n"
            "owner: heartbeat\n"
            "reason: test\n"
            "steps_detected: 3\n"
            "total_growth_kib: 600\n"
            "growth_rate_kib_per_min: 10\n"
            "owner_evidence: test\n"
            "correlated_events: heartbeat=1\n"
        )
    test("confirmed_leak rejected for subsystem_config marker", False, setup_subsystem_config)
    
    # Test 25: verdict with empty owner_evidence rejected for confirmed_leak
    def setup_leak_empty_evidence(p):
        (p / "manifest.yaml").write_text("run_id: test\nplatform: Linux\ncommit_sha: abc123\n")
        (p / "memory_samples.tsv").write_text("timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n")
        (p / "event_timeline.tsv").write_text(
            "timestamp\telapsed_sec\tevent\tsubsystem\n"
            "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
            "2024-01-01T00:00:30\t30\theartbeat_tick\ttovarisch.heartbeat\n"
            "2024-01-01T00:01:40\t100\tidle_complete\tlab\n"
        )
        (p / "verdict.txt").write_text(
            "verdict: confirmed_leak\n"
            "owner: heartbeat\n"
            "reason: test\n"
            "steps_detected: 3\n"
            "total_growth_kib: 600\n"
            "growth_rate_kib_per_min: 10\n"
            "owner_evidence: \n"
            "correlated_events: heartbeat=1\n"
        )
    test("confirmed_leak rejected for empty owner_evidence", False, setup_leak_empty_evidence)
    
    # Test 26: Real CLI wrapper smoke test (subprocess)
    if run_cli_wrapper_smoke_test():
        print("  PASS: CLI wrapper smoke test via subprocess")
        tests_passed += 1
    else:
        print("  FAIL: CLI wrapper smoke test via subprocess")
        tests_failed += 1
    
    print(f"\nSelf-test results: {tests_passed} passed, {tests_failed} failed")
    return tests_failed == 0

