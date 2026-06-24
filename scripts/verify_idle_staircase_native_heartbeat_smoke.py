#!/usr/bin/env python3
# verify_idle_staircase_native_heartbeat_smoke.py
# Verifies native heartbeat smoke test artifacts for heartbeat-native artifact capture proof.
#
# This verifier checks that:
# 1. Enabled run: native_event_timeline.tsv exists with heartbeat events
# 2. Disabled run: no heartbeat native events, heartbeat is disabled via lab config
#
# Usage:
#   python3 scripts/verify_idle_staircase_native_heartbeat_smoke.py \
#     --enabled <path_to_enabled_artifact> \
#     --disabled <path_to_disabled_artifact>
#   python3 scripts/verify_idle_staircase_native_heartbeat_smoke.py --self-test
#
# Exit codes:
#   0 - Both artifacts pass verification (or self-tests pass)
#   1 - Verification failed (with reason)

import argparse
import sys
import tempfile
from pathlib import Path
from typing import Optional


def read_file(path: Path) -> Optional[str]:
    """Read file content or return None if not found."""
    try:
        return path.read_text()
    except FileNotFoundError:
        return None
    except Exception as e:
        print(f"Warning: Could not read {path}: {e}", file=sys.stderr)
        return None


def parse_tsv_lines(content: str) -> tuple[list[str], list[list[str]]]:
    """Parse TSV content into header and data rows."""
    lines = content.strip().split("\n")
    if not lines:
        return [], []
    header = lines[0].split("\t")
    data_rows = [line.split("\t") for line in lines[1:]]
    return header, data_rows


def verify_enabled_artifact(artifact_path: Path) -> tuple[bool, str]:
    """Verify the enabled heartbeat artifact has native heartbeat events."""
    errors = []
    
    # Check required files exist
    manifest = read_file(artifact_path / "manifest.yaml")
    if manifest is None:
        errors.append("manifest.yaml not found")
    elif "native_events_enabled: true" not in manifest:
        errors.append("manifest.yaml: native_events_enabled is not true")
    elif "native_disable_heartbeat: false" not in manifest:
        errors.append("manifest.yaml: native_disable_heartbeat is not false")
    
    config = read_file(artifact_path / "tovarisch_lab.conf")
    if config is None:
        errors.append("tovarisch_lab.conf not found")
    elif "disable_heartbeat = false" not in config:
        errors.append("tovarisch_lab.conf: disable_heartbeat is not false")
    
    # Check native event timeline exists and has heartbeat events
    native_timeline = read_file(artifact_path / "native_event_timeline.tsv")
    if native_timeline is None:
        errors.append("native_event_timeline.tsv not found")
    else:
        header, data_rows = parse_tsv_lines(native_timeline)
        if len(data_rows) == 0:
            errors.append("native_event_timeline.tsv: only header, no data rows")
        
        # Expected columns: timestamp, elapsed_millis, event, subsystem, detail, pid
        # Column indices: 0, 1, 2, 3, 4, 5
        
        # Check for heartbeat events using proper TSV column parsing
        has_start = False
        has_end = False
        has_valid_subsystem = False
        heartbeat_millis = []
        valid_pids = True
        
        for row in data_rows:
            if len(row) < 6:
                continue
            event_col = row[2]
            subsystem_col = row[3]
            elapsed_col = row[1]
            pid_col = row[5]
            
            if event_col == "heartbeat_tick_start":
                has_start = True
            if event_col == "heartbeat_tick_end":
                has_end = True
            
            # Check subsystem column is "heartbeat"
            if subsystem_col == "heartbeat":
                has_valid_subsystem = True
            
            # Collect heartbeat elapsed millis
            if "heartbeat" in event_col:
                try:
                    millis = int(elapsed_col)
                    heartbeat_millis.append(millis)
                except ValueError:
                    pass
            
            # Check pid is numeric
            if pid_col:
                try:
                    int(pid_col)
                except ValueError:
                    valid_pids = False
        
        if not has_start:
            errors.append("native_event_timeline.tsv: no heartbeat_tick_start events")
        if not has_end:
            errors.append("native_event_timeline.tsv: no heartbeat_tick_end events")
        if not has_valid_subsystem:
            errors.append("native_event_timeline.tsv: no events with subsystem='heartbeat'")
        if not valid_pids:
            errors.append("native_event_timeline.tsv: pid column contains non-numeric values")
        
        # Check for heartbeat-aligned elapsed millis (>= 30000)
        if heartbeat_millis:
            max_millis = max(heartbeat_millis)
            if max_millis < 30000:
                errors.append(f"native_event_timeline.tsv: no heartbeat events >= 30000ms (max={max_millis})")
        else:
            errors.append("native_event_timeline.tsv: no heartbeat elapsed_millis values found")
    
    if errors:
        return False, "; ".join(errors)
    return True, "OK"


def verify_disabled_artifact(artifact_path: Path) -> tuple[bool, str]:
    """Verify the disabled heartbeat artifact has no native heartbeat events."""
    errors = []
    
    # Check manifest records heartbeat disabled
    manifest = read_file(artifact_path / "manifest.yaml")
    if manifest is None:
        errors.append("manifest.yaml not found")
    elif "native_events_enabled: true" not in manifest:
        errors.append("manifest.yaml: native_events_enabled is not true")
    elif "native_disable_heartbeat: true" not in manifest:
        errors.append("manifest.yaml: native_disable_heartbeat is not true")
    
    config = read_file(artifact_path / "tovarisch_lab.conf")
    if config is None:
        errors.append("tovarisch_lab.conf not found")
    elif "disable_heartbeat = true" not in config:
        errors.append("tovarisch_lab.conf: disable_heartbeat is not true")
    
    # Check native event timeline has no heartbeat events
    native_timeline = read_file(artifact_path / "native_event_timeline.tsv")
    if native_timeline is not None:
        header, data_rows = parse_tsv_lines(native_timeline)
        
        # Check for heartbeat events - should be absent or header-only
        has_heartbeat = False
        has_heartbeat_subsystem = False
        
        for row in data_rows:
            if len(row) < 4:
                continue
            event_col = row[2]
            subsystem_col = row[3]
            
            if event_col in ("heartbeat_tick_start", "heartbeat_tick_end", "heartbeat_tick_failed"):
                has_heartbeat = True
            if subsystem_col == "heartbeat":
                has_heartbeat_subsystem = True
        
        if has_heartbeat:
            errors.append("native_event_timeline.tsv: contains heartbeat events when disabled")
        if has_heartbeat_subsystem:
            errors.append("native_event_timeline.tsv: contains heartbeat subsystem events when disabled")
    
    # If native timeline doesn't exist, that's also acceptable for disabled
    # (tovarisch might not create the file when no events are emitted)
    
    if errors:
        return False, "; ".join(errors)
    return True, "OK"


def create_enabled_fixture(tmpdir: Path, include_valid_events: bool = True, 
                          wrong_subsystem: bool = False, missing_pids: bool = False,
                          name: str = "enabled_fixture") -> Path:
    """Create a test fixture for enabled artifact."""
    artifact = tmpdir / name
    artifact.mkdir(exist_ok=True)
    
    # manifest.yaml
    (artifact / "manifest.yaml").write_text(
        'run_id: test\nplatform: Linux\ncommit_sha: abc123\n'
        'native_events_enabled: true\n'
        'native_disable_heartbeat: false\n'
    )
    
    # tovarisch_lab.conf
    (artifact / "tovarisch_lab.conf").write_text(
        "[lab]\n"
        "native_events_enabled = true\n"
        "native_events_path = \"\"\n"
        "disable_heartbeat = false\n"
        "disable_wg_checks = true\n"
        "disable_bgp = true\n"
        "disable_bfd = true\n"
    )
    
    # memory_samples.tsv (minimal)
    (artifact / "memory_samples.tsv").write_text(
        "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
        "2026-01-01T00:00:00\t0\t1000\t2000\n"
    )
    
    # event_timeline.tsv (minimal)
    (artifact / "event_timeline.tsv").write_text(
        "timestamp\telapsed_sec\tevent\tsubsystem\n"
        "2026-01-01T00:00:00\t0\tlab_started\tlab\n"
    )
    
    # verdict.txt
    (artifact / "verdict.txt").write_text(
        "verdict: inconclusive\n"
    )
    
    # native_event_timeline.tsv
    if include_valid_events:
        pid = "1234" if not missing_pids else "not_numeric"
        subsystem = "heartbeat" if not wrong_subsystem else "wireguard"
        (artifact / "native_event_timeline.tsv").write_text(
            f"timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid\n"
            f"2026-01-01T00:00:30.000\t30000\theartbeat_tick_start\t{subsystem}\t\t{pid}\n"
            f"2026-01-01T00:00:30.000\t30000\theartbeat_tick_end\t{subsystem}\t\t{pid}\n"
            f"2026-01-01T00:01:00.000\t60000\theartbeat_tick_start\t{subsystem}\t\t{pid}\n"
            f"2026-01-01T00:01:00.000\t60000\theartbeat_tick_end\t{subsystem}\t\t{pid}\n"
        )
    else:
        # Header only
        (artifact / "native_event_timeline.tsv").write_text(
            "timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid\n"
        )
    
    return artifact


def create_disabled_fixture(tmpdir: Path, include_heartbeat: bool = False,
                           missing_timeline: bool = False,
                           name: str = "disabled_fixture") -> Path:
    """Create a test fixture for disabled artifact."""
    artifact = tmpdir / name
    artifact.mkdir(exist_ok=True)
    
    # manifest.yaml
    (artifact / "manifest.yaml").write_text(
        'run_id: test\nplatform: Linux\ncommit_sha: abc123\n'
        'native_events_enabled: true\n'
        'native_disable_heartbeat: true\n'
    )
    
    # tovarisch_lab.conf
    (artifact / "tovarisch_lab.conf").write_text(
        "[lab]\n"
        "native_events_enabled = true\n"
        "native_events_path = \"\"\n"
        "disable_heartbeat = true\n"
        "disable_wg_checks = true\n"
        "disable_bgp = true\n"
        "disable_bfd = true\n"
    )
    
    # memory_samples.tsv (minimal)
    (artifact / "memory_samples.tsv").write_text(
        "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
        "2026-01-01T00:00:00\t0\t1000\t2000\n"
    )
    
    # event_timeline.tsv (minimal)
    (artifact / "event_timeline.tsv").write_text(
        "timestamp\telapsed_sec\tevent\tsubsystem\n"
        "2026-01-01T00:00:00\t0\tlab_started\tlab\n"
    )
    
    # verdict.txt
    (artifact / "verdict.txt").write_text(
        "verdict: inconclusive\n"
    )
    
    # native_event_timeline.tsv
    if not missing_timeline:
        if include_heartbeat:
            (artifact / "native_event_timeline.tsv").write_text(
                "timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid\n"
                "2026-01-01T00:00:30.000\t30000\theartbeat_tick_start\theartbeat\t\t1234\n"
            )
        else:
            # Header only or empty
            (artifact / "native_event_timeline.tsv").write_text(
                "timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid\n"
            )
    
    return artifact


def run_self_tests() -> bool:
    """Run self-tests on the smoke verifier. Returns True if all pass."""
    print("=== Native Heartbeat Smoke Verifier Self-Tests ===")
    print()
    
    tests_passed = 0
    tests_failed = 0
    
    with tempfile.TemporaryDirectory() as tmpdir:
        tmppath = Path(tmpdir)
        
        # Test 1: Valid enabled fixture passes
        print("Test 1: Valid enabled fixture passes")
        fixture = create_enabled_fixture(tmppath, include_valid_events=True)
        valid, error = verify_enabled_artifact(fixture)
        if valid:
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: {error}")
            tests_failed += 1
        
        # Test 2: Enabled fixture with wrong subsystem fails
        print("Test 2: Enabled fixture with wrong subsystem fails")
        fixture = create_enabled_fixture(tmppath, include_valid_events=True, wrong_subsystem=True, name="enabled_fixture2")
        valid, error = verify_enabled_artifact(fixture)
        if not valid and "subsystem='heartbeat'" in error:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected subsystem rejection, got valid={valid}, error={error}")
            tests_failed += 1
        
        # Test 3: Enabled fixture with non-numeric pid fails
        print("Test 3: Enabled fixture with non-numeric pid fails")
        fixture = create_enabled_fixture(tmppath, include_valid_events=True, missing_pids=True, name="enabled_fixture3")
        valid, error = verify_enabled_artifact(fixture)
        if not valid and "non-numeric" in error:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected pid rejection, got valid={valid}, error={error}")
            tests_failed += 1
        
        # Test 4: Enabled fixture with header-only fails
        print("Test 4: Enabled fixture with header-only fails")
        fixture = create_enabled_fixture(tmppath, include_valid_events=False, name="enabled_fixture4")
        valid, error = verify_enabled_artifact(fixture)
        if not valid and "no data rows" in error:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected no-data rejection, got valid={valid}, error={error}")
            tests_failed += 1
        
        # Test 5: Valid disabled fixture passes
        print("Test 5: Valid disabled fixture passes")
        fixture = create_disabled_fixture(tmppath, include_heartbeat=False, name="disabled_fixture5")
        valid, error = verify_disabled_artifact(fixture)
        if valid:
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: {error}")
            tests_failed += 1
        
        # Test 6: Disabled fixture with heartbeat events fails
        print("Test 6: Disabled fixture with heartbeat events fails")
        fixture = create_disabled_fixture(tmppath, include_heartbeat=True, name="disabled_fixture6")
        valid, error = verify_disabled_artifact(fixture)
        if not valid and "contains heartbeat" in error:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected heartbeat rejection, got valid={valid}, error={error}")
            tests_failed += 1
        
        # Test 7: Disabled fixture without timeline passes
        print("Test 7: Disabled fixture without timeline passes")
        fixture = create_disabled_fixture(tmppath, missing_timeline=True)
        valid, error = verify_disabled_artifact(fixture)
        if valid:
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: {error}")
            tests_failed += 1
    
    print()
    print(f"Results: {tests_passed} passed, {tests_failed} failed")
    return tests_failed == 0


def main():
    parser = argparse.ArgumentParser(
        description="Verify native heartbeat smoke test artifacts"
    )
    parser.add_argument(
        "--enabled",
        help="Path to enabled heartbeat artifact directory"
    )
    parser.add_argument(
        "--disabled",
        help="Path to disabled heartbeat artifact directory"
    )
    parser.add_argument(
        "--self-test",
        action="store_true",
        help="Run self-tests on the verifier"
    )
    
    args = parser.parse_args()
    
    if args.self_test:
        success = run_self_tests()
        sys.exit(0 if success else 1)
    
    if not args.enabled or not args.disabled:
        parser.print_help()
        sys.exit(1)
    
    enabled_path = Path(args.enabled)
    disabled_path = Path(args.disabled)
    
    print("=== Native Heartbeat Smoke Verification ===")
    print()
    
    # Verify enabled artifact
    print(f"Checking enabled artifact: {enabled_path}")
    enabled_valid, enabled_error = verify_enabled_artifact(enabled_path)
    if enabled_valid:
        print(f"  Enabled: {enabled_error}")
    else:
        print(f"  Enabled FAILED: {enabled_error}")
    print()
    
    # Verify disabled artifact
    print(f"Checking disabled artifact: {disabled_path}")
    disabled_valid, disabled_error = verify_disabled_artifact(disabled_path)
    if disabled_valid:
        print(f"  Disabled: {disabled_error}")
    else:
        print(f"  Disabled FAILED: {disabled_error}")
    print()
    
    # Summary
    if enabled_valid and disabled_valid:
        print("VERIFICATION PASSED: Both artifacts verified successfully")
        print()
        print("Evidence:")
        print("- Enabled: native_event_timeline.tsv contains heartbeat_tick_start/end events")
        print("- Enabled: subsystem='heartbeat' and numeric pid columns")
        print("- Enabled: elapsed_millis values are heartbeat-aligned (>= 30000ms)")
        print("- Disabled: manifest/config show disable_heartbeat=true")
        print("- Disabled: no heartbeat events in native_event_timeline.tsv")
        sys.exit(0)
    else:
        print("VERIFICATION FAILED")
        sys.exit(1)


if __name__ == "__main__":
    main()
