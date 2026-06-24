# test_fixtures.py — Reusable test fixtures for idle staircase verifier
"""Reusable test fixtures for verifier self-tests."""

from pathlib import Path


def minimal_manifest(p: Path) -> None:
    """Minimal valid manifest."""
    (p / "manifest.yaml").write_text(
        "run_id: test\nplatform: Linux\ncommit_sha: abc123\n"
    )


def minimal_samples(p: Path) -> None:
    """Minimal valid memory samples."""
    (p / "memory_samples.tsv").write_text(
        "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n2024-01-01\t0\t1000\t2000\n"
    )


def minimal_events(p: Path) -> None:
    """Minimal valid event timeline."""
    (p / "event_timeline.tsv").write_text(
        "timestamp\telapsed_sec\tevent\tsubsystem\n"
        "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
        "2024-01-01T00:01:40\t100\tidle_complete\tlab\n"
    )


def minimal_verdict(p: Path, verdict: str = "inconclusive") -> None:
    """Minimal valid verdict."""
    (p / "verdict.txt").write_text(f"verdict: {verdict}\nreason: test\n")


def complete_artifact(p: Path) -> None:
    """Complete valid artifact with all files."""
    minimal_manifest(p)
    minimal_samples(p)
    minimal_events(p)
    minimal_verdict(p)


def native_events_artifact(p: Path) -> None:
    """Artifact with native events enabled."""
    complete_artifact(p)
    (p / "native_event_timeline.tsv").write_text(
        "timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid\n"
        "2024-01-01T00:00:00\t30000\theartbeat_tick_start\theartbeat\t\t1234\n"
        "2024-01-01T00:00:30\t60000\theartbeat_tick_end\theartbeat\t\t1234\n"
    )


def confirmed_leak_artifact(p: Path) -> None:
    """Artifact that should produce confirmed_leak with native event attribution.
    
    CRITICAL: confirmed_leak requires native events from native_event_timeline.tsv
    that correlate with memory steps. Shell-side event_timeline.tsv is NOT sufficient.
    """
    minimal_manifest(p)
    (p / "memory_samples.tsv").write_text(
        "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
        "2024-01-01T00:00:00\t0\t1000\t2000\n"
        "2024-01-01T00:00:30\t30\t1200\t2200\n"
        "2024-01-01T00:01:00\t60\t1400\t2400\n"
        "2024-01-01T00:01:30\t90\t1600\t2600\n"
        "2024-01-01T00:02:00\t120\t1800\t2800\n"
        "2024-01-01T00:02:30\t150\t2000\t3000\n"
    )
    # Shell-side event timeline (NOT used for confirmed_leak attribution)
    (p / "event_timeline.tsv").write_text(
        "timestamp\telapsed_sec\tevent\tsubsystem\n"
        "2024-01-01T00:00:00\t0\tlab_started\tlab\n"
        "2024-01-01T00:02:30\t150\tidle_complete\tlab\n"
    )
    # NATIVE events that correlate with memory steps (30s, 60s, 90s, 120s, 150s)
    # These are in milliseconds: 30000, 60000, 90000, 120000, 150000
    (p / "native_event_timeline.tsv").write_text(
        "timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid\n"
        "2024-01-01T00:00:30\t30000\theartbeat_tick_start\theartbeat\t\t1234\n"
        "2024-01-01T00:00:30\t30000\theartbeat_tick_end\theartbeat\t\t1234\n"
        "2024-01-01T00:01:00\t60000\theartbeat_tick_start\theartbeat\t\t1234\n"
        "2024-01-01T00:01:00\t60000\theartbeat_tick_end\theartbeat\t\t1234\n"
        "2024-01-01T00:01:30\t90000\theartbeat_tick_start\theartbeat\t\t1234\n"
        "2024-01-01T00:01:30\t90000\theartbeat_tick_end\theartbeat\t\t1234\n"
        "2024-01-01T00:02:00\t120000\theartbeat_tick_start\theartbeat\t\t1234\n"
        "2024-01-01T00:02:00\t120000\theartbeat_tick_end\theartbeat\t\t1234\n"
        "2024-01-01T00:02:30\t150000\theartbeat_tick_start\theartbeat\t\t1234\n"
        "2024-01-01T00:02:30\t150000\theartbeat_tick_end\theartbeat\t\t1234\n"
    )
    (p / "verdict.txt").write_text(
        "verdict: confirmed_leak\n"
        "owner: heartbeat\n"
        "reason: Native events from native_event_timeline.tsv correlate with memory steps\n"
        "steps_detected: 5\n"
        "total_growth_kib: 1000\n"
        "growth_rate_kib_per_min: 40\n"
        "owner_evidence: Native heartbeat events correlate with 5 memory steps at t=30,60,90,120,150s\n"
        "correlated_events: heartbeat=10,wg=0,bgp=0,bfd=0,health=0,status=0\n"
        "native_events_enabled: true\n"
        "native_event_count: 10\n"
    )


def shell_only_artifact(p: Path) -> None:
    """Artifact with shell-side synthetic events only."""
    minimal_manifest(p)
    (p / "memory_samples.tsv").write_text(
        "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
        "2024-01-01\t0\t1000\t2000\n"
        "2024-01-01\t30\t1100\t2100\n"
        "2024-01-01\t60\t1200\t2200\n"
        "2024-01-01\t90\t1300\t2300\n"
        "2024-01-01\t120\t1400\t2400\n"
    )
    (p / "event_timeline.tsv").write_text(
        "timestamp\telapsed_sec\tevent\tsubsystem\n"
        "2024-01-01T00:00:00\t0\tsubsystem_config\tlab\n"
        "2024-01-01T00:00:30\t30\theartbeat_tick_start\tlab\n"
        "2024-01-01T00:01:00\t60\theartbeat_tick_end\tlab\n"
        "2024-01-01T00:01:30\t90\theartbeat_tick_start\tlab\n"
        "2024-01-01T00:02:00\t120\theartbeat_tick_end\tlab\n"
    )
    (p / "verdict.txt").write_text(
        "verdict: inconclusive\n"
        "reason: Shell-side synthetic events cannot produce confirmed_leak\n"
    )
