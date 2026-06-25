#!/usr/bin/env python3
"""
verify_wg_netlink_lab_artifact.py - WireGuard netlink lab artifact verifier

Validates lab artifacts from the WireGuard generic-netlink runtime proof harness.

Usage:
    # Self-test (fixture validation)
    python3 scripts/verify_wg_netlink_lab_artifact.py --self-test

    # Validate real artifacts
    python3 scripts/verify_wg_netlink_lab_artifact.py --artifacts-dir ./artifacts/wg-netlink-lab --mode linux-proof

Exit codes:
    0 - All validations passed
    1 - Validation failed
    2 - Self-test failed

This verifier is NOT part of make gate (self-test only).
"""

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple


# =============================================================================
# Self-test case definitions
# =============================================================================

SELF_TEST_CASES = [
    # (name, preflight_data, interface_state_data, backend_status_data, expected_pass, expected_error_contains)
    ("valid_proof_passes",
     {"can_run": True, "reason": "ok", "wg_module_loaded": True, "has_ip_command": True},
     {"interface": "wg-kgb0", "pre_existing": False, "created_by_lab": True, "teardown_action": "deleted"},
     {"success": True, "interface": "wg-kgb0", "peer_count": 0, "backend_kind": "generic_netlink", "no_sensitive_data": True},
     True, None),

    ("success_false_fails",
     {"can_run": True, "reason": "ok", "wg_module_loaded": True, "has_ip_command": True},
     {"interface": "wg-kgb0", "pre_existing": False, "created_by_lab": True, "teardown_action": "deleted"},
     {"success": False, "interface": "wg-kgb0", "peer_count": 0, "backend_kind": "generic_netlink", "no_sensitive_data": True, "error": "test_error"},
     False, "success must be True"),

    ("wrong_backend_kind_fails",
     {"can_run": True, "reason": "ok", "wg_module_loaded": True, "has_ip_command": True},
     {"interface": "wg-kgb0", "pre_existing": False, "created_by_lab": True, "teardown_action": "deleted"},
     {"success": True, "interface": "wg-kgb0", "peer_count": 0, "backend_kind": "cli", "no_sensitive_data": True},
     False, "backend_kind"),

    ("wrong_interface_fails",
     {"can_run": True, "reason": "ok", "wg_module_loaded": True, "has_ip_command": True},
     {"interface": "wg-other0", "pre_existing": False, "created_by_lab": True, "teardown_action": "deleted"},
     {"success": True, "interface": "wg-other0", "peer_count": 0, "backend_kind": "generic_netlink", "no_sensitive_data": True},
     False, "interface"),

    ("peer_count_nonzero_fails",
     {"can_run": True, "reason": "ok", "wg_module_loaded": True, "has_ip_command": True},
     {"interface": "wg-kgb0", "pre_existing": False, "created_by_lab": True, "teardown_action": "deleted"},
     {"success": True, "interface": "wg-kgb0", "peer_count": 1, "backend_kind": "generic_netlink", "no_sensitive_data": True},
     False, "peer_count"),

    ("no_sensitive_data_false_fails",
     {"can_run": True, "reason": "ok", "wg_module_loaded": True, "has_ip_command": True},
     {"interface": "wg-kgb0", "pre_existing": False, "created_by_lab": True, "teardown_action": "deleted"},
     {"success": True, "interface": "wg-kgb0", "peer_count": 0, "backend_kind": "generic_netlink", "no_sensitive_data": False},
     False, "no_sensitive_data"),

    ("preflight_can_run_false_fails",
     {"can_run": False, "reason": "wireguard_module_not_loaded", "wg_module_loaded": False, "has_ip_command": True},
     {"interface": "wg-kgb0", "pre_existing": False, "created_by_lab": True, "teardown_action": "deleted"},
     {"success": True, "interface": "wg-kgb0", "peer_count": 0, "backend_kind": "generic_netlink", "no_sensitive_data": True},
     False, "can_run"),
]


# =============================================================================
# Validation logic
# =============================================================================

def validate_preflight(data: Dict[str, Any], mode: str) -> List[str]:
    """Validate preflight.json artifact."""
    errors = []

    if "can_run" not in data:
        errors.append("missing field: can_run")
    elif mode == "linux-proof" and data["can_run"] is not True:
        errors.append("preflight.can_run must be True for linux-proof mode")

    if "reason" not in data:
        errors.append("missing field: reason")

    if mode == "linux-proof":
        if not data.get("wg_module_loaded"):
            errors.append("wireguard module not loaded")
        if not data.get("has_ip_command"):
            errors.append("ip command not available")

    return errors


def validate_interface_state(data: Dict[str, Any]) -> List[str]:
    """Validate interface-state.json artifact."""
    errors = []

    for field in ["interface", "pre_existing", "created_by_lab"]:
        if field not in data:
            errors.append(f"missing field: {field}")

    if data.get("interface") != "wg-kgb0":
        errors.append(f"interface must be 'wg-kgb0', got '{data.get('interface')}'")

    pre_existing = data.get("pre_existing", False)
    created_by_lab = data.get("created_by_lab", False)

    if pre_existing and created_by_lab:
        errors.append("coherence error: pre_existing=True but created_by_lab=True")

    teardown = data.get("teardown_action", "")
    if pre_existing:
        if teardown not in ("preserve", ""):
            errors.append(f"pre_existing should have teardown_action='preserve', got '{teardown}'")
    else:
        if teardown not in ("deleted", "failed_create", "failed_up", ""):
            errors.append(f"created interface should have teardown_action='deleted', got '{teardown}'")

    return errors


def validate_backend_status(data: Dict[str, Any]) -> List[str]:
    """Validate backend-status.json artifact."""
    errors = []

    for field in ["success", "backend_kind", "interface"]:
        if field not in data:
            errors.append(f"missing field: {field}")

    if data.get("success") is not True:
        errors.append(f"backend-status.success must be True, got {data.get('success')}")
        return errors

    if data.get("backend_kind") != "generic_netlink":
        errors.append(f"backend_kind must be 'generic_netlink', got '{data.get('backend_kind')}'")

    if data.get("interface") != "wg-kgb0":
        errors.append(f"interface must be 'wg-kgb0', got '{data.get('interface')}'")

    if data.get("peer_count", 0) != 0:
        errors.append(f"peer_count must be 0 for empty-interface lab, got {data.get('peer_count')}")

    if data.get("no_sensitive_data") is not True:
        errors.append(f"no_sensitive_data must be True, got {data.get('no_sensitive_data')}")

    return errors


def load_json(path: Optional[Path]) -> Tuple[Optional[Dict[str, Any]], Optional[str]]:
    """Load JSON file, return (data, error)."""
    if path is None:
        return None, None
    if not path.exists():
        return None, f"file not found: {path}"
    try:
        with open(path) as f:
            return json.load(f), None
    except json.JSONDecodeError as e:
        return None, f"invalid JSON in {path}: {e}"
    except Exception as e:
        return None, f"error reading {path}: {e}"


def validate_artifacts(
    preflight_path: Optional[Path],
    interface_state_path: Optional[Path],
    backend_status_path: Optional[Path],
    mode: str,
) -> List[str]:
    """Validate all artifacts and return list of errors."""
    errors = []

    preflight, preflight_err = load_json(preflight_path)
    interface_state, istate_err = load_json(interface_state_path)
    backend_status, bstatus_err = load_json(backend_status_path)

    if mode == "linux-proof":
        if preflight_path is None or not preflight_path.exists():
            errors.append("preflight.json required for linux-proof mode")
        if interface_state_path is None or not interface_state_path.exists():
            errors.append("interface-state.json required for linux-proof mode")
        if backend_status_path is None or not backend_status_path.exists():
            errors.append("backend-status.json required for linux-proof mode")

    if preflight_err:
        errors.append(preflight_err)
    if istate_err:
        errors.append(istate_err)
    if bstatus_err:
        errors.append(bstatus_err)

    if preflight is not None:
        errors.extend(validate_preflight(preflight, mode))

    if interface_state is not None:
        errors.extend(validate_interface_state(interface_state))

    if backend_status is not None:
        errors.extend(validate_backend_status(backend_status))

    return errors


def run_self_test() -> bool:
    """Run self-test using fixture cases."""
    print("=" * 60)
    print("Running self-test with fixture cases...")
    print("=" * 60)

    all_passed = True

    for case_name, preflight, interface_state, backend_status, expected_pass, expected_error in SELF_TEST_CASES:
        print(f"\nTest case: {case_name}")

        temp_dir = Path("/tmp/verify_wg_netlink_fixture")
        temp_dir.mkdir(exist_ok=True)

        fixtures = {
            "preflight.json": preflight,
            "interface-state.json": interface_state,
            "backend-status.json": backend_status,
        }

        for filename, data in fixtures.items():
            if data is not None:
                with open(temp_dir / filename, "w") as f:
                    json.dump(data, f, indent=2)

        errors = validate_artifacts(
            preflight_path=temp_dir / "preflight.json",
            interface_state_path=temp_dir / "interface-state.json",
            backend_status_path=temp_dir / "backend-status.json",
            mode="linux-proof",
        )

        passed = len(errors) == 0

        if passed == expected_pass:
            print(f"  PASS: Got expected result")
        else:
            print(f"  FAIL: Expected pass={expected_pass}, got pass={passed}")
            if expected_error:
                print(f"  Expected error containing: {expected_error}")
            print(f"  Actual errors: {errors}")
            all_passed = False

        for filename in fixtures:
            f = temp_dir / filename
            if f.exists():
                f.unlink()

    print("\n" + "=" * 60)
    if all_passed:
        print("SELF-TEST: ALL CASES PASSED")
    else:
        print("SELF-TEST: SOME CASES FAILED")
    return all_passed


# =============================================================================
# Main
# =============================================================================

def main():
    parser = argparse.ArgumentParser(description="WireGuard netlink lab artifact verifier")
    parser.add_argument("--self-test", action="store_true", help="Run self-test with fixture cases")
    parser.add_argument("--artifacts-dir", type=Path, help="Directory containing lab artifacts")
    parser.add_argument("--mode", choices=["linux-proof", "preflight-only"], default="linux-proof")
    parser.add_argument("--verbose", action="store_true")

    args = parser.parse_args()

    if args.self_test:
        passed = run_self_test()
        return 0 if passed else 2

    if args.artifacts_dir:
        preflight_path = args.artifacts_dir / "preflight.json"
        interface_state_path = args.artifacts_dir / "interface-state.json"
        backend_status_path = args.artifacts_dir / "backend-status.json"
    else:
        parser.error("--artifacts-dir is required for validation")
        return 1

    errors = validate_artifacts(
        preflight_path=preflight_path,
        interface_state_path=interface_state_path,
        backend_status_path=backend_status_path,
        mode=args.mode,
    )

    if errors:
        print("VALIDATION FAILED:")
        for error in errors:
            print(f"  - {error}")
        return 1

    print("VALIDATION PASSED")
    if args.verbose:
        print(f"  preflight: {preflight_path}")
        print(f"  interface-state: {interface_state_path}")
        print(f"  backend-status: {backend_status_path}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
