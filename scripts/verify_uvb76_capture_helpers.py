#!/usr/bin/env python3
"""Verify UVB-76 capture helper functions.

Contract-preserving port from shell/JQ to Python stdlib.
Tests normalize_capture_status and save_phase_capture_packet_from_raw_row
using the same fixture patterns as the original shell verifier.

Usage:
    python3 scripts/verify_uvb76_capture_helpers.py [--self-test]

Options:
    --self-test    Run regression tests (delegates to split test modules)

Exit codes:
    0   All validations passed
    1   Validation failure
    2   File not found or unreadable
    3   Invalid JSON
"""

import json
import sys
import subprocess
from pathlib import Path
from typing import Optional, Tuple

# === Contract Constants ===

# Status normalization mapping (from shell source)
STATUS_NORMALIZATION = {
    "ok": "captured",
    "captured": "captured",
    "timeout": "failed",
    "error": "failed",
    "failed": "failed",
    "skipped_cooldown": "skipped_cooldown",
    "disabled": "disabled",
    "not_configured": "not_configured",
    "not_attempted": "not_attempted",
    "in_progress": "pending",
    "pending": "pending",
}


# === Dataclasses ===

from dataclasses import dataclass

@dataclass
class ValidationError:
    """Represents a validation failure with context."""
    path: str
    expected: str
    actual: str

    def __str__(self) -> str:
        return f"[capture-helpers] FAIL: {self.path}: expected {self.expected}, got {self.actual}"


@dataclass
class NormalizedCaptureContract:
    """Normalized capture contract output."""
    capture_status: str
    capture_exists: bool
    is_protected: bool
    cooldown_info: Optional[dict]
    suppressed_by_cooldown: bool


# === Status Normalization ===

def normalize_capture_status(api_status: Optional[str]) -> str:
    """Normalize API capture status to contract canonical status."""
    if not api_status:
        return "pending"
    return STATUS_NORMALIZATION.get(api_status, "unknown")


def extract_raw_status(capture_info: dict) -> str:
    """Extract raw status from capture info using extraction rules."""
    capture_status = capture_info.get("capture_status")
    if capture_status is not None and capture_status != "":
        return str(capture_status)
    status = capture_info.get("status")
    if status is not None and status != "":
        return str(status)
    return "unknown"


def normalize_spike_row_capture_contract(raw_row: dict) -> NormalizedCaptureContract:
    """Normalize spike row capture info to contract verifier shape."""
    captures = raw_row.get("captures")
    
    if not captures or len(captures) == 0:
        return NormalizedCaptureContract(
            capture_status="not_attempted",
            capture_exists=False,
            is_protected=False,
            cooldown_info=None,
            suppressed_by_cooldown=False,
        )
    
    capture_info = captures[0]
    raw_status = extract_raw_status(capture_info)
    capture_status = normalize_capture_status(raw_status)
    
    if capture_status == "captured":
        capture_exists = capture_info.get("capture_exists", True)
        is_protected = capture_info.get("is_protected", True)
    else:
        capture_exists = capture_info.get("capture_exists", False)
        is_protected = capture_info.get("is_protected", False)
    
    cooldown_info = capture_info.get("cooldown_info")
    suppressed_by_cooldown = cooldown_info is not None
    
    return NormalizedCaptureContract(
        capture_status=capture_status,
        capture_exists=capture_exists if capture_exists is not None else False,
        is_protected=is_protected if is_protected is not None else False,
        cooldown_info=cooldown_info,
        suppressed_by_cooldown=suppressed_by_cooldown,
    )


# === Packet Extraction ===

def get_network_diag_from_row(row: dict) -> Tuple[Optional[dict], Optional[str]]:
    """Extract network_diag from spike row using multiple possible paths."""
    try:
        captures = row.get("captures")
        if captures and len(captures) > 0:
            cap = captures[0]
            if cap.get("network_diag") is not None:
                return cap["network_diag"], "captures[0].network_diag"
    except (TypeError, KeyError):
        pass
    
    try:
        captures = row.get("captures")
        if captures and len(captures) > 0:
            cap = captures[0]
            packet = cap.get("packet")
            if packet and packet.get("network_diag") is not None:
                return packet["network_diag"], "captures[0].packet.network_diag"
    except (TypeError, KeyError):
        pass
    
    try:
        captures = row.get("captures")
        if captures and len(captures) > 0:
            cap = captures[0]
            diagnostics = cap.get("diagnostics")
            if diagnostics and diagnostics.get("network_diag") is not None:
                return diagnostics["network_diag"], "captures[0].diagnostics.network_diag"
    except (TypeError, KeyError):
        pass
    
    try:
        captures = row.get("captures")
        if captures and len(captures) > 0:
            cap = captures[0]
            ndp = cap.get("network_diag_packet")
            if ndp and ndp.get("network_diag") is not None:
                return ndp["network_diag"], "captures[0].network_diag_packet.network_diag"
    except (TypeError, KeyError):
        pass
    
    if row.get("network_diag") is not None:
        return row["network_diag"], "network_diag"
    
    return None, None


def save_phase_capture_packet_from_raw_row(
    phase_num: str,
    packet_file: str,
    spike_row_file: str
) -> bool:
    """Save capture packet from raw spike row."""
    if not packet_file:
        print("[capture-helpers] ERROR: missing packet_file argument", file=sys.stderr)
        return False
    if not spike_row_file:
        print("[capture-helpers] ERROR: missing spike_row_file argument", file=sys.stderr)
        return False
    
    try:
        spike_row = json.loads(Path(spike_row_file).read_text())
    except FileNotFoundError:
        print(f"[capture-helpers] ERROR: file not found: {spike_row_file}", file=sys.stderr)
        return False
    except json.JSONDecodeError as e:
        print(f"[capture-helpers] ERROR: invalid JSON in {spike_row_file}: {e}", file=sys.stderr)
        return False
    
    network_diag, found_path = get_network_diag_from_row(spike_row)
    
    if network_diag is None:
        print(f"[capture-helpers] ERROR: Phase {phase_num}: no network_diag found in spike row", file=sys.stderr)
        captures = spike_row.get("captures")
        if captures:
            print(f"[capture-helpers] INFO: captures[] exists with {len(captures)} entries", file=sys.stderr)
            cap = captures[0] if captures else {}
            keys = list(cap.keys()) if isinstance(cap, dict) else []
            print(f"[capture-helpers] INFO: captures[0] keys: {keys}", file=sys.stderr)
        else:
            print("[capture-helpers] INFO: no captures[] in row", file=sys.stderr)
        return False
    
    if not isinstance(network_diag, dict):
        print(f"[capture-helpers] ERROR: Phase {phase_num}: network_diag is not an object", file=sys.stderr)
        return False
    
    from datetime import datetime, timezone
    timestamp = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    
    packet = {
        "phase": f"phase{phase_num}",
        "network_diag": network_diag,
        "timestamp": timestamp,
    }
    
    try:
        Path(packet_file).write_text(json.dumps(packet, indent=2) + "\n")
    except OSError as e:
        print(f"[capture-helpers] ERROR: cannot write {packet_file}: {e}", file=sys.stderr)
        return False
    
    print(f"[capture-helpers] INFO: Phase {phase_num} capture packet saved: {packet_file}")
    return True


# === Verification ===

def run_self_tests() -> int:
    """Run all split test modules."""
    script_dir = Path(__file__).parent
    test_modules = [
        "verify_uvb76_capture_status_test.py",
        "verify_uvb76_capture_row_normalization_test.py",
        "verify_uvb76_capture_network_diag_test.py",
        "verify_uvb76_capture_packet_save_test.py",
    ]
    
    all_passed = True
    for module in test_modules:
        module_path = script_dir / module
        if not module_path.exists():
            print(f"[capture-helpers] ERROR: test module not found: {module_path}", file=sys.stderr)
            all_passed = False
            continue
        
        print(f"\n=== Running {module} ===")
        result = subprocess.run([sys.executable, str(module_path)], capture_output=True, text=True)
        print(result.stdout)
        if result.stderr:
            print(result.stderr, file=sys.stderr)
        if result.returncode != 0:
            all_passed = False
    
    return 0 if all_passed else 1


def verify() -> int:
    """Run verification (placeholder - actual tests are in --self-test mode)."""
    print("[capture-helpers] Verification requires --self-test to run tests")
    print("[capture-helpers] Run: python3 scripts/verify_uvb76_capture_helpers.py --self-test")
    return 0


def main() -> int:
    """Main entry point."""
    args = sys.argv[1:]
    
    if "--self-test" in args or len(args) == 0:
        print("[capture-helpers] Running self-test suite (split modules)")
        result = run_self_tests()
        if result == 0:
            print("\n[capture-helpers] Self-test PASS")
            return 0
        else:
            print("\n[capture-helpers] Self-test FAIL", file=sys.stderr)
            return 1
    
    if "--help" in args or "-h" in args:
        print(__doc__)
        return 0
    
    return verify()


if __name__ == "__main__":
    sys.exit(main())
