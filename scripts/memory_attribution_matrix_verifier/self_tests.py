# self_tests.py — Self-test suite

import tempfile
from pathlib import Path

from .verify import verify_matrix
from .fixtures import create_matrix_fixture


def run_self_tests():
    print("=== Memory Attribution Matrix Verifier Self-Tests ===\n")
    
    tests_passed = 0
    tests_failed = 0
    
    with tempfile.TemporaryDirectory() as tmpdir:
        tmppath = Path(tmpdir)
        
        print("Test 1: Valid full matrix passes")
        fixture = create_matrix_fixture(tmppath, "valid_matrix")
        valid, error, _ = verify_matrix(fixture)
        if valid:
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: {error}")
            tests_failed += 1
        
        print("Test 2: Valid partial matrix (2 variants) passes")
        fixture = create_matrix_fixture(tmppath, "partial", variants=["all_enabled", "no_periodic"])
        valid, error, _ = verify_matrix(fixture)
        if valid:
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: {error}")
            tests_failed += 1
        
        print("Test 3: Missing matrix summary fails")
        fixture = create_matrix_fixture(tmppath, "no_summary")
        (fixture / "matrix-summary.md").unlink()
        valid, error, _ = verify_matrix(fixture)
        if not valid and "matrix-summary.md" in error:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected rejection, got valid={valid}, error={error}")
            tests_failed += 1
        
        print("Test 4: Missing declared variant fails")
        fixture = create_matrix_fixture(tmppath, "missing_var", variants=["all_enabled", "no_periodic"])
        import shutil
        shutil.rmtree(fixture / "no_periodic")
        valid, error, _ = verify_matrix(fixture)
        if not valid and "Missing" in error:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected rejection, got valid={valid}, error={error}")
            tests_failed += 1
        
        print("Test 5: Missing native events fails")
        fixture = create_matrix_fixture(tmppath, "no_native", variants=["all_enabled"])
        (fixture / "all_enabled" / "native_event_timeline.tsv").unlink()
        valid, error, results = verify_matrix(fixture)
        failed_variants = [v for v, r in results.items() if not r["valid"] and "native_event_timeline" in r.get("error", "")]
        if not valid and failed_variants:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected rejection, got valid={valid}, failed_variants={failed_variants}")
            tests_failed += 1
        
        print("Test 6: Heartbeat leak when disabled fails")
        fixture = create_matrix_fixture(tmppath, "hb_leak", variants=["heartbeat_disabled"])
        native_path = fixture / "heartbeat_disabled" / "native_event_timeline.tsv"
        content = native_path.read_text()
        native_path.write_text(content.rstrip("\n") + "\n" + "2024-01-01T00:00:30\t30000\theartbeat_tick_start\theartbeat\t\t1234\n")
        valid, error, results = verify_matrix(fixture)
        failed_variants = [v for v, r in results.items() if not r["valid"] and "Heartbeat disabled" in r.get("error", "")]
        if not valid and failed_variants:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected rejection, got valid={valid}, error={error}")
            tests_failed += 1
        
        print("Test 7: Blank owner does not capture 'reason:' from next line")
        fixture = create_matrix_fixture(tmppath, "blank_owner", variants=["all_enabled"])
        verdict_path = fixture / "all_enabled" / "verdict.txt"
        # Write verdict with blank owner field followed by reason field
        verdict_content = """verdict: bounded_warmup_or_allocator_highwater
owner:
reason: Growth is bounded, consistent with allocator warmup settling.
steps_detected: 1
total_growth_kib: 50
growth_rate_kib_per_min: 5
samples_count: 20
"""
        verdict_path.write_text(verdict_content)
        from .checks import parse_verdict
        parsed = parse_verdict(verdict_content)
        owner_val = parsed.get("owner", "NOT_FOUND")
        reason_val = parsed.get("reason", "NOT_FOUND")
        # Owner should be blank/empty, NOT "reason:"
        if owner_val == "" or owner_val == "NOT_FOUND":
            # Owner is blank or not set - this is correct
            if reason_val and "allocator" in reason_val:
                print("  PASS (owner is blank, reason is correct)")
                tests_passed += 1
            else:
                print(f"  FAIL: owner={owner_val!r}, reason={reason_val!r}")
                tests_failed += 1
        elif owner_val == "reason:":
            print(f"  FAIL: owner incorrectly captured 'reason:' from next line")
            tests_failed += 1
        else:
            print(f"  FAIL: unexpected owner={owner_val!r}")
            tests_failed += 1
        
        print("Test 8: Disabled variant native counts are verified correctly")
        from .checks import count_native_events, get_variant_config
        # Create a temp file with zero heartbeat events
        with tempfile.NamedTemporaryFile(mode='w', suffix='.tsv', delete=False) as f:
            f.write("timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid\n")
            f.write("2024-01-01T00:00:30\t30000\twg_check_start\twireguard\t\t1234\n")
            f.write("2024-01-01T00:00:30\t30000\tbgp_maintenance_start\tbgp\t\t1234\n")
            native_file = Path(f.name)
        try:
            counts = count_native_events(native_file)
            # For heartbeat_disabled, heartbeat count should be 0
            if counts.get("heartbeat", -1) == 0:
                print("  PASS (heartbeat_disabled shows 0 heartbeat events)")
                tests_passed += 1
            else:
                print(f"  FAIL: expected heartbeat=0, got heartbeat={counts.get('heartbeat')}")
                tests_failed += 1
        finally:
            native_file.unlink()
        
        print("Test 9: Artifact writer strips 'Event counts:' from rendered summary")
        # Test that the artifact writer actually strips synthetic Event counts
        # Import the artifacts module from the memory_attribution_matrix package
        import sys
        sys.path.insert(0, str(Path(__file__).parent.parent))
        from memory_attribution_matrix.artifacts import write_matrix_summary
        
        # Create a test matrix root with all_enabled variant containing synthetic Event counts
        test_root = tmppath / "test_synthetic"
        test_root.mkdir()
        variant_dir = test_root / "all_enabled"
        variant_dir.mkdir()
        
        # Create a minimal verdict.txt with synthetic Event counts in reason
        verdict_content = """verdict: bounded_warmup_or_allocator_highwater
owner:
reason: Growth is bounded. Event counts: heartbeat=20, wg=10, bgp=60, bfd=60.
steps_detected: 1
total_growth_kib: 50
growth_rate_kib_per_min: 5
samples_count: 20
"""
        (variant_dir / "verdict.txt").write_text(verdict_content)
        
        # Create minimal native_event_timeline.tsv
        (variant_dir / "native_event_timeline.tsv").write_text(
            "timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid\n"
        )
        
        # Create minimal memory_samples.tsv
        (variant_dir / "memory_samples.tsv").write_text(
            "timestamp\telapsed_sec\trss_kib\tvmdata_kib\n"
        )
        
        # Create minimal manifest.yaml
        (variant_dir / "manifest.yaml").write_text(
            "run_id: test\nplatform: Linux\ncommit_sha: abc\n"
            "native_events_enabled: true\nnative_disable_heartbeat: false\n"
            "native_disable_wg_checks: false\nnative_disable_bgp: false\n"
            "native_disable_bfd: false\nduration_seconds: 600\n"
        )
        
        # Write the summary using the artifact writer
        results = {
            "all_enabled": {
                "success": True,
                "verdict": {"verdict": "bounded_warmup_or_allocator_highwater", "owner": "",
                           "reason": "Growth is bounded. Event counts: heartbeat=20, wg=10, bgp=60, bfd=60.",
                           "total_growth_kib": 50, "growth_rate_kib_per_min": 5,
                           "steps_detected": 1, "samples_count": 20},
                "native_counts": {"heartbeat": 0, "wireguard": 0, "bgp": 0, "bfd": 0, "total": 0},
                "manifest": {"native_events_enabled": True, "native_disable_heartbeat": False,
                            "native_disable_wg_checks": False, "native_disable_bgp": False,
                            "native_disable_bfd": False},
            }
        }
        write_matrix_summary(
            matrix_root=test_root,
            matrix_run_id="test-synthetic",
            results=results,
            overall_verdict="bounded_warmup_or_allocator_highwater",
            verdict_reason="Growth is bounded.",
            verdict_details={},
            duration=600,
            interval=5,
        )
        
        # Read the rendered summary
        summary_path = test_root / "matrix-summary.md"
        rendered = summary_path.read_text()
        
        # Verify: "Event counts:" should NOT appear in rendered output
        # The artifact writer strips synthetic event counts
        if "Event counts:" not in rendered:
            # Verify: "Growth is bounded." SHOULD appear
            if "Growth is bounded." in rendered:
                # Verify: Native events section should appear
                if "Native Events" in rendered:
                    print("  PASS (Event counts stripped, Growth is bounded, Native Events present)")
                    tests_passed += 1
                else:
                    print(f"  FAIL: Native Events not in rendered output")
                    tests_failed += 1
            else:
                print(f"  FAIL: 'Growth is bounded.' not in rendered output")
                tests_failed += 1
        else:
            print(f"  FAIL: 'Event counts:' still in rendered output")
            tests_failed += 1
    
    print(f"\nResults: {tests_passed} passed, {tests_failed} failed")
    return tests_failed == 0
