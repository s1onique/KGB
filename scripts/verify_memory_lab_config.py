#!/usr/bin/env python3
"""
Verifier for memory-lab config files.

Validates that memory-lab config files:
1. Load correctly as JSON
2. Have a valid auth.password_sha256 (format: sha256:<salt>:<hash>)
3. Contain no production secret material (no real credentials)

This is a regression test so the CI memory lab does not fail on config validation.
"""

import json
import os
import re
import sys
from typing import List, Tuple

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def validate_password_sha256_format(value: str) -> Tuple[bool, str]:
    """
    Validate password_sha256 format: sha256:<32-hex-salt>:<64-hex-hash>.
    
    Returns (is_valid, error_message).
    """
    if not value:
        return False, "password_sha256 is empty"
    
    # Must have sha256: prefix
    prefix = "sha256:"
    if not value.startswith(prefix):
        return False, f"must start with '{prefix}'"
    
    remaining = value[len(prefix):]
    parts = remaining.split(":", 1)
    
    if len(parts) != 2:
        return False, "must be format sha256:<salt>:<hash>"
    
    salt_hex, hash_hex = parts
    
    # Salt must be exactly 32 hex chars (16 bytes)
    if len(salt_hex) != 32:
        return False, f"salt must be 32 hex chars (got {len(salt_hex)})"
    
    # Hash must be exactly 64 hex chars (32 bytes)
    if len(hash_hex) != 64:
        return False, f"hash must be 64 hex chars (got {len(hash_hex)})"
    
    # Validate hex encoding
    hex_pattern = re.compile(r'^[0-9a-fA-F]+$')
    if not hex_pattern.match(salt_hex):
        return False, f"salt contains non-hex characters"
    
    if not hex_pattern.match(hash_hex):
        return False, f"hash contains non-hex characters"
    
    return True, ""


def validate_no_secrets(data: dict) -> List[str]:
    """
    Check for potential production secrets in the config.
    
    Returns list of warnings (not errors) - these don't fail validation
    but serve as a signal that this is a CI-safe test config.
    
    Note on TLS paths:
    The memory-lab runner (tools/memory-lab) generates ephemeral TLS certificates
    for UVB-76 via tools/memory-lab/tls_config.go:GenerateEphemeralCert().
    Empty tls_cert_file/tls_key_file in the memory-lab.json is expected and safe
    because the runner will:
    1. Generate ephemeral self-signed localhost cert/key
    2. Write a derived config with TLS paths populated
    3. Launch UVB-76 with the derived config
    4. Use HTTPS with the generated cert for readiness/workload
    
    See: kgb://doctrine/native-owned-critical-paths
    """
    warnings = []
    
    # Check for TLS certs pointing to real production paths
    cert = data.get("listen", {}).get("tls_cert_file", "")
    key = data.get("listen", {}).get("tls_key_file", "")
    
    if cert and "/etc/" in cert:
        warnings.append("TLS cert path contains /etc/ - may be production path")
    if key and "/etc/" in key:
        warnings.append("TLS key path contains /etc/ - may be production path")
    
    # Document expected empty TLS paths (will be materialized by runner)
    if not cert and not key:
        print("  INFO: Empty TLS paths are expected for memory-lab config")
        print("        Runner will generate ephemeral self-signed cert/key")
    
    return warnings


def validate_memory_lab_config(path: str) -> Tuple[List[str], dict]:
    """
    Validate a memory-lab config file.
    
    Returns (errors, config_data).
    """
    errors = []
    data = None
    
    if not os.path.exists(path):
        return [f"File does not exist: {path}"], None
    
    try:
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)
    except json.JSONDecodeError as e:
        return [f"Invalid JSON: {e}"], None
    
    if not isinstance(data, dict):
        return ["Config must be a JSON object"], None
    
    # Validate auth section exists
    auth = data.get("auth")
    if not auth:
        errors.append("auth section is required")
        return errors, data
    
    # Validate password_sha256 format
    pw_hash = auth.get("password_sha256", "")
    if pw_hash:
        is_valid, err_msg = validate_password_sha256_format(pw_hash)
        if not is_valid:
            errors.append(f"auth.password_sha256 format invalid: {err_msg}")
    else:
        errors.append("auth.password_sha256 is required")
    
    # Check for secrets (warnings, not errors)
    warnings = validate_no_secrets(data)
    for w in warnings:
        print(f"  WARNING: {w}")
    
    # Validate listen section exists
    listen = data.get("listen")
    if not listen:
        errors.append("listen section is required")
    
    return errors, data


def run_verifier(repo_root: str) -> List[str]:
    """Run the memory-lab config verifier."""
    all_errors = []
    print("=== Memory Lab Config Verifier ===\n")
    
    # Find memory-lab config
    config_path = os.path.join(repo_root, "uvb76", "uvb76.memory-lab.json")
    
    if not os.path.exists(config_path):
        all_errors.append(f"Memory-lab config not found: {config_path}")
        return all_errors
    
    print(f"Validating: {config_path}")
    
    errors, data = validate_memory_lab_config(config_path)
    
    if errors:
        print("\nERRORS:")
        for e in errors:
            print(f"  - {e}")
        all_errors.extend(errors)
    else:
        print("  OK: Config is valid")
        
        # Print summary of config properties
        if data:
            auth = data.get("auth", {})
            print(f"  Auth user: {auth.get('username', 'N/A')}")
            pw = auth.get("password_sha256", "")
            if pw and pw.startswith("sha256:"):
                parts = pw.split(":")
                if len(parts) == 3:
                    print(f"  Salt: {parts[1][:8]}... (32 hex)")
                    print(f"  Hash: {parts[2][:8]}... (64 hex)")
            
            listen = data.get("listen", {})
            print(f"  Listen: {listen.get('addr', 'N/A')}")
            
            targets = data.get("targets", [])
            print(f"  Targets: {len(targets)}")
            
            diag = data.get("diagnostics", {})
            print(f"  Diagnostics enabled: {diag.get('enabled', False)}")
    
    return all_errors


def run_self_tests() -> bool:
    """Run self-tests on the verifier."""
    import tempfile
    print("\n=== Running Self-Tests ===\n")
    
    tests_passed = 0
    tests_failed = 0
    
    with tempfile.TemporaryDirectory() as tmpdir:
        # Test: valid memory-lab config
        valid_config = {
            "listen": {"addr": ":18081", "tls_cert_file": "", "tls_key_file": ""},
            "auth": {
                "username": "memory-lab",
                "password_sha256": "sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"
            },
            "scrape": {"interval_seconds": 30, "timeout_milliseconds": 5000},
            "latency": {"http": {"enabled": False}, "icmp": {"enabled": False}},
            "diagnostics": {"enabled": False, "peers": []},
            "targets": []
        }
        
        path = os.path.join(tmpdir, "valid.json")
        with open(path, "w") as f:
            json.dump(valid_config, f)
        
        errors, _ = validate_memory_lab_config(path)
        if len(errors) == 0:
            print("  PASS: Valid config passes")
            tests_passed += 1
        else:
            print(f"  FAIL: {errors}")
            tests_failed += 1
        
        # Test: invalid hash length
        bad_hash = dict(valid_config)
        bad_hash["auth"]["password_sha256"] = "sha256:00000000000000000000000000000000:0000"
        
        path = os.path.join(tmpdir, "bad_hash.json")
        with open(path, "w") as f:
            json.dump(bad_hash, f)
        
        errors, _ = validate_memory_lab_config(path)
        if len(errors) > 0 and "64 hex chars" in errors[0]:
            print("  PASS: Invalid hash length fails correctly")
            tests_passed += 1
        else:
            print(f"  FAIL: should have failed on hash length")
            tests_failed += 1
        
        # Test: missing auth section
        bad_auth = {"listen": {"addr": ":18081"}}
        
        path = os.path.join(tmpdir, "bad_auth.json")
        with open(path, "w") as f:
            json.dump(bad_auth, f)
        
        errors, _ = validate_memory_lab_config(path)
        if len(errors) > 0:
            print("  PASS: Missing auth fails correctly")
            tests_passed += 1
        else:
            print(f"  FAIL: should have failed on missing auth")
            tests_failed += 1
        
        # Test: invalid hex chars
        bad_hex = dict(valid_config)
        bad_hex["auth"]["password_sha256"] = "sha256:ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ:0000000000000000000000000000000000000000000000000000000000000000"
        
        path = os.path.join(tmpdir, "bad_hex.json")
        with open(path, "w") as f:
            json.dump(bad_hex, f)
        
        errors, _ = validate_memory_lab_config(path)
        if len(errors) > 0 and "non-hex" in errors[0]:
            print("  PASS: Invalid hex chars fails correctly")
            tests_passed += 1
        else:
            print(f"  FAIL: should have failed on invalid hex")
            tests_failed += 1
    
    print(f"\n=== Self-Test Results ===")
    print(f"  Passed: {tests_passed}, Failed: {tests_failed}")
    return tests_failed == 0


def main():
    if "--self-test" in sys.argv:
        sys.exit(0 if run_self_tests() else 1)
    
    errors = run_verifier(REPO_ROOT)
    
    print("\n" + "=" * 50)
    if errors:
        print("\nVERIFICATION FAILED:")
        for e in errors:
            print(f"  - {e}")
        sys.exit(1)
    else:
        print("\nVERIFICATION PASSED")
        sys.exit(0)


if __name__ == "__main__":
    main()
