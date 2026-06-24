# lab_runner/self_tests.py - Self-tests for the lab runner
import tempfile
from pathlib import Path

from lab_runner.config import LabRunConfig, parse_args, LabError
from lab_runner.artifacts import write_tovarisch_config, write_manifest


def run_self_tests() -> bool:
    """Run self-tests on the lab runner. Returns True if all pass."""
    print("=== Idle Staircase Lab Runner Self-Tests ===")
    print()

    tests_passed = 0
    tests_failed = 0

    # Test 1: Config with native events writes native_events_path
    print("Test 1: Config with native events writes native_events_path")
    with tempfile.TemporaryDirectory() as tmpdir:
        tmppath = Path(tmpdir)
        artifact_path = tmppath / "test-run"
        artifact_path.mkdir()

        config = LabRunConfig(
            duration=60, interval=5, run_id="test-run",
            native_events=True,
            native_events_path=artifact_path / "native_event_timeline.tsv",
            disable_heartbeat=False, disable_wg_checks=True,
            disable_bgp=True, disable_bfd=True,
            strace=False, status_burst=False,
            repo_root=tmppath, script_dir=tmppath,
            artifact_root=tmppath, artifact_path=artifact_path,
            tovarisch_binary=tmppath / "tovarisch",
            lab_bind="127.0.0.1", lab_port=8317,
            heartbeat_enabled=True, wg_check_enabled=False, bgp_bfd_enabled=False,
        )

        config_path = write_tovarisch_config(config)
        content = config_path.read_text()

        if "native_events_path" in content and "native_event_timeline.tsv" in content:
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: native_events_path not in config:\n{content}")
            tests_failed += 1

    # Test 2: Config without native path omits native_events_path
    print("Test 2: Config without native path omits native_events_path")
    with tempfile.TemporaryDirectory() as tmpdir:
        tmppath = Path(tmpdir)
        artifact_path = tmppath / "test-run"
        artifact_path.mkdir()

        config = LabRunConfig(
            duration=60, interval=5, run_id="test-run",
            native_events=True, native_events_path=None,
            disable_heartbeat=False, disable_wg_checks=True,
            disable_bgp=True, disable_bfd=True,
            strace=False, status_burst=False,
            repo_root=tmppath, script_dir=tmppath,
            artifact_root=tmppath, artifact_path=artifact_path,
            tovarisch_binary=tmppath / "tovarisch",
            lab_bind="127.0.0.1", lab_port=8317,
            heartbeat_enabled=True, wg_check_enabled=False, bgp_bfd_enabled=False,
        )

        config_path = write_tovarisch_config(config)
        content = config_path.read_text()

        if "native_events_path" not in content:
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: native_events_path should not be in config:\n{content}")
            tests_failed += 1

    # Test 3: Config with disable_heartbeat writes correct flag
    print("Test 3: Config with disable_heartbeat writes correct flag")
    with tempfile.TemporaryDirectory() as tmpdir:
        tmppath = Path(tmpdir)
        artifact_path = tmppath / "test-run"
        artifact_path.mkdir()

        config = LabRunConfig(
            duration=60, interval=5, run_id="test-run",
            native_events=True, native_events_path=None,
            disable_heartbeat=True, disable_wg_checks=True,
            disable_bgp=True, disable_bfd=True,
            strace=False, status_burst=False,
            repo_root=tmppath, script_dir=tmppath,
            artifact_root=tmppath, artifact_path=artifact_path,
            tovarisch_binary=tmppath / "tovarisch",
            lab_bind="127.0.0.1", lab_port=8317,
            heartbeat_enabled=True, wg_check_enabled=False, bgp_bfd_enabled=False,
        )

        config_path = write_tovarisch_config(config)
        content = config_path.read_text()

        if "disable_heartbeat = true" in content:
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: disable_heartbeat = true not in config:\n{content}")
            tests_failed += 1

    # Test 4: Manifest records native flags
    print("Test 4: Manifest records native flags")
    with tempfile.TemporaryDirectory() as tmpdir:
        tmppath = Path(tmpdir)
        artifact_path = tmppath / "test-run"
        artifact_path.mkdir()

        config = LabRunConfig(
            duration=60, interval=5, run_id="test-run",
            native_events=True,
            native_events_path=artifact_path / "native_event_timeline.tsv",
            disable_heartbeat=True, disable_wg_checks=False,
            disable_bgp=True, disable_bfd=False,
            strace=False, status_burst=False,
            repo_root=tmppath, script_dir=tmppath,
            artifact_root=tmppath, artifact_path=artifact_path,
            tovarisch_binary=tmppath / "tovarisch",
            lab_bind="127.0.0.1", lab_port=8317,
            heartbeat_enabled=True, wg_check_enabled=True, bgp_bfd_enabled=True,
        )

        manifest_path = write_manifest(config)
        content = manifest_path.read_text()

        checks = [
            "native_events_enabled: true" in content,
            "native_disable_heartbeat: true" in content,
            "native_disable_wg_checks: false" in content,
            "native_disable_bgp: true" in content,
            "native_disable_bfd: false" in content,
        ]

        if all(checks):
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: manifest missing native flags")
            tests_failed += 1

    # Test 5: /proc status parser parses fixture
    print("Test 5: /proc status parser parses fixture")
    fixture = """
VmPeak:    10240 kB
VmSize:    20480 kB
VmHWM:     5120 kB
VmRSS:     4096 kB
VmData:    3072 kB
VmSwap:       0 kB
"""

    result = {}
    for line in fixture.splitlines():
        if not line.startswith(("VmRSS:", "VmHWM:", "VmSize:", "VmData:", "VmSwap:", "VmPeak:")):
            continue
        key, raw = line.split(":", 1)
        parts = raw.strip().split()
        result[key] = int(parts[0]) if parts else 0

    if (result.get("VmRSS") == 4096 and result.get("VmHWM") == 5120 and
        result.get("VmSize") == 20480 and result.get("VmData") == 3072 and
        result.get("VmSwap") == 0 and result.get("VmPeak") == 10240):
        print("  PASS")
        tests_passed += 1
    else:
        print(f"  FAIL: parsed values incorrect: {result}")
        tests_failed += 1

    # Test 6: Invalid run_id is rejected
    print("Test 6: Invalid run_id is rejected")
    try:
        parse_args(["--run-id", "../escape"])
        print("  FAIL: should have rejected ../escape")
        tests_failed += 1
    except LabError as e:
        print(f"  PASS (correctly rejected: {e})")
        tests_passed += 1

    # Test 7: Empty run_id auto-generates
    print("Test 7: Empty run_id auto-generates")
    args = parse_args(["--run-id", ""])
    if args.run_id and args.run_id.startswith("idle-"):
        print("  PASS (auto-generated run_id)")
        tests_passed += 1
    else:
        print(f"  FAIL: expected auto-generated run_id, got: {args.run_id}")
        tests_failed += 1

    # Test 8: Duration accepts integer values
    print("Test 8: Duration accepts integer values")
    args = parse_args(["--duration", "10"])
    if args.duration == 10:
        print("  PASS")
        tests_passed += 1
    else:
        print(f"  FAIL: duration not 10")
        tests_failed += 1

    # Test 9: interval validation
    print("Test 9: interval validation")
    args = parse_args(["--duration", "10", "--interval", "5"])
    if args.interval <= args.duration:
        print("  PASS")
        tests_passed += 1
    else:
        print(f"  FAIL: interval > duration")
        tests_failed += 1

    # Test 10: Artifact path resolution
    print("Test 10: Artifact path resolution preserves layout")
    args = parse_args(["--run-id", "test-artifact-layout"])
    if args.run_id == "test-artifact-layout":
        print("  PASS")
        tests_passed += 1
    else:
        print(f"  FAIL: run_id incorrect: {args.run_id}")
        tests_failed += 1

    # Test 11: repo_root resolves to repo root, not scripts/
    print("Test 11: repo_root resolves to repo root")
    args = parse_args(["--duration", "10"])
    # repo_root should be the parent of scripts/
    # Check that tovarisch_binary path makes sense
    expected_tovarisch = args.repo_root / "tovarisch" / "zig-out" / "bin" / "tovarisch"
    if args.tovarisch_binary == expected_tovarisch:
        print("  PASS")
        tests_passed += 1
    else:
        print(f"  FAIL: tovarisch_binary wrong: {args.tovarisch_binary}")
        tests_failed += 1

    # Test 12: script_dir resolves to scripts/, not scripts/lab_runner/
    print("Test 12: script_dir resolves to scripts/")
    args = parse_args(["--duration", "10"])
    # script_dir should end with "scripts"
    if str(args.script_dir).endswith("scripts"):
        print("  PASS")
        tests_passed += 1
    else:
        print(f"  FAIL: script_dir wrong: {args.script_dir}")
        tests_failed += 1

    # Test 13: analyzer_script path is at scripts/idle_staircase_analyzer_cli.py
    print("Test 13: analyzer_script path is correct")
    args = parse_args(["--duration", "10"])
    expected_analyzer = args.script_dir / "idle_staircase_analyzer_cli.py"
    if expected_analyzer.name == "idle_staircase_analyzer_cli.py":
        print("  PASS")
        tests_passed += 1
    else:
        print(f"  FAIL: analyzer path wrong: {expected_analyzer}")
        tests_failed += 1

    # Test 14: artifact_root is repo/artifacts/..., not repo/scripts/artifacts/...
    print("Test 14: artifact_root path is correct")
    args = parse_args(["--duration", "10"])
    expected_artifact = args.repo_root / "artifacts" / "memory-labs" / "tovarisch" / "idle-staircase"
    if args.artifact_root == expected_artifact:
        print("  PASS")
        tests_passed += 1
    else:
        print(f"  FAIL: artifact_root wrong: {args.artifact_root}")
        tests_failed += 1

    # Test 15: manifest booleans are lowercase
    print("Test 15: manifest booleans are lowercase")
    with tempfile.TemporaryDirectory() as tmpdir:
        tmppath = Path(tmpdir)
        artifact_path = tmppath / "test-run"
        artifact_path.mkdir()

        config = LabRunConfig(
            duration=60, interval=5, run_id="test-run",
            native_events=True,
            native_events_path=artifact_path / "native_event_timeline.tsv",
            disable_heartbeat=True, disable_wg_checks=False,
            disable_bgp=True, disable_bfd=False,
            strace=False, status_burst=True,
            repo_root=tmppath, script_dir=tmppath,
            artifact_root=tmppath, artifact_path=artifact_path,
            tovarisch_binary=tmppath / "tovarisch",
            lab_bind="127.0.0.1", lab_port=8317,
            heartbeat_enabled=True, wg_check_enabled=True, bgp_bfd_enabled=True,
        )

        manifest_path = write_manifest(config)
        content = manifest_path.read_text()

        checks = [
            "native_events_enabled: true" in content,
            "native_events_enabled: True" not in content,
            "native_disable_heartbeat: true" in content,
            "native_disable_heartbeat: True" not in content,
            "native_disable_wg_checks: false" in content,
            "native_disable_wg_checks: False" not in content,
            "status_burst: true" in content,
            "status_burst: True" not in content,
            "strace_enabled: false" in content,
            "strace_enabled: False" not in content,
        ]

        if all(checks):
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: manifest booleans not lowercase")
            print(content)
            tests_failed += 1

    print()
    print(f"Results: {tests_passed} passed, {tests_failed} failed")
    return tests_failed == 0
