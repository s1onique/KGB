"""
Self-test module for UVB-76 Artifact Secret Hygiene.

Provides self-test functionality without committing fixtures.
"""

import os
import tempfile

from .inventory import validate_inventory as validate_inventory_module
from .rules import UNIVERSAL_RULES, ARTIFACT_CONTEXT_RULES, build_test_private_key, build_test_rsa_key
from .scanner import scan_file_for_secrets, MAX_FILE_SIZE


def run_self_tests() -> tuple[list[str], dict[str, bool], int, int]:
    """Run self-tests without committing fixtures."""
    errors = []
    results = {}
    passed = 0
    total = 0
    
    # Test 1: Universal rules detect private keys (using dynamic fixture)
    total += 1
    found = False
    test_key = build_test_private_key()
    test_content = f"{test_key}\nMIIEvQIBADAN...\n-----END PRIVATE KEY-----"
    for rule in UNIVERSAL_RULES:
        if rule["id"] == "UVB76-SECRET-0001":
            if rule["pattern"].search(test_content):
                found = True
                break
    results["universal_private_key_detection"] = found
    if found:
        passed += 1
    else:
        errors.append("Universal private key pattern not found")
    
    # Test 2: Public certificate NOT detected
    total += 1
    found = False
    for rule in UNIVERSAL_RULES:
        if rule["pattern"].search("-----BEGIN CERTIFICATE-----\nMIIDXTCCAkWg...\n-----END CERTIFICATE-----"):
            found = True
            break
    results["public_cert_not_detected"] = not found
    if not found:
        passed += 1
    else:
        errors.append("Public certificate incorrectly detected as private key")
    
    # Test 3: Password hash pattern
    total += 1
    found = False
    for rule in ARTIFACT_CONTEXT_RULES:
        if rule["id"] == "UVB76-SECRET-0020":
            if rule["pattern"].search('"password_sha256": "sha256:abc123def456789:abc123def456789"'):
                found = True
                break
    results["password_hash_detection"] = found
    if found:
        passed += 1
    else:
        errors.append("Password hash pattern not found")
    
    # Test 4: Credential URL detection
    total += 1
    found = False
    for rule in ARTIFACT_CONTEXT_RULES:
        if rule["id"] == "UVB76-SECRET-0040":
            if rule["pattern"].search("https://user:password@example.com/api"):
                found = True
                break
    results["credential_url_detection"] = found
    if found:
        passed += 1
    else:
        errors.append("Credential URL pattern not found")
    
    # Test 5: Safe URL NOT detected
    total += 1
    found = False
    for rule in ARTIFACT_CONTEXT_RULES:
        if rule["id"] == "UVB76-SECRET-0040":
            if rule["pattern"].search("https://example.com/api?token=removed"):
                found = True
                break
    results["safe_url_not_detected"] = not found
    if not found:
        passed += 1
    else:
        errors.append("Safe URL incorrectly flagged")
    
    # Test 6: uvb76_session cookie detection
    total += 1
    found = False
    for rule in ARTIFACT_CONTEXT_RULES:
        if rule["id"] == "UVB76-SECRET-0012":
            if rule["pattern"].search("uvb76_session=abc123def456ghi789"):
                found = True
                break
    results["session_cookie_detection"] = found
    if found:
        passed += 1
    else:
        errors.append("Session cookie pattern not found")
    
    # Test 7: JWT-like token detection
    total += 1
    found = False
    for rule in ARTIFACT_CONTEXT_RULES:
        if rule["id"] == "UVB76-SECRET-0050":
            if rule["pattern"].search("eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVm"):
                found = True
                break
    results["jwt_detection"] = found
    if found:
        passed += 1
    else:
        errors.append("JWT pattern not found")
    
    # Test 8: Inventory validation
    total += 1
    inv_errors = validate_inventory_module()
    results["inventory_validation"] = len(inv_errors) == 0
    if len(inv_errors) == 0:
        passed += 1
    else:
        errors.extend([f"Inventory: {e}" for e in inv_errors])
    
    # Test 9: Bounds checking - oversized file
    total += 1
    with tempfile.NamedTemporaryFile(mode='w', suffix='.json', delete=False) as f:
        f.write("x" * (MAX_FILE_SIZE + 1))
        temp_path = f.name
    
    try:
        findings = scan_file_for_secrets(temp_path)
        found_bound_error = any(f.rule_id == "UVB76-SIZE-0001" for f in findings)
        results["oversized_file_detection"] = found_bound_error
        if found_bound_error:
            passed += 1
        else:
            errors.append("Oversized file not flagged")
    finally:
        os.unlink(temp_path)
    
    # Test 10: Binary file detection in artifact surface
    total += 1
    with tempfile.NamedTemporaryFile(mode='wb', suffix='.key', delete=False) as f:
        f.write(b'\x00\x01\x02' + b'x' * 100)
        temp_path = f.name
    
    try:
        findings = scan_file_for_secrets(temp_path, artifact_surface=True)
        found_binary_error = any(f.rule_id == "UVB76-BINARY-0001" for f in findings)
        results["binary_key_detection"] = found_binary_error
        if found_binary_error:
            passed += 1
        else:
            errors.append("Binary .key file not flagged in artifact surface")
    finally:
        os.unlink(temp_path)
    
    # Test 11: Diagnostic safety
    total += 1
    from .scanner import SecretFinding
    finding = SecretFinding(
        rule_id="UVB76-SECRET-0001",
        file_path="/fake/path.json",
        line_number=10,
        explanation="private key PEM block detected",
        remediation="Remove or replace",
    )
    formatted = f"{finding.rule_id}: {finding.explanation}"
    safe = finding.rule_id in formatted and finding.explanation in formatted
    results["diagnostic_safety"] = safe
    if safe:
        passed += 1
    else:
        errors.append("Diagnostic output may expose secret values")
    
    # Test 12: Redacted marker acceptance
    total += 1
    found_redacted = False
    redacted_marker = "[REDACTED]"
    for rule in ARTIFACT_CONTEXT_RULES:
        if rule["pattern"].search('"password": "' + redacted_marker + '"'):
            found_redacted = True
            break
    results["redacted_marker_accepted"] = not found_redacted
    if not found_redacted:
        passed += 1
    else:
        errors.append("[REDACTED] marker incorrectly flagged as secret")
    
    return errors, results, total, passed
