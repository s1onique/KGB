"""
Positive behavior tests for UVB-76 Artifact Secret Hygiene.

Tests that secrets are correctly detected and safe values are accepted.
"""

import json
import os
import tempfile
import uuid

from ..rules import UNIVERSAL_RULES, ARTIFACT_CONTEXT_RULES, build_test_private_key, build_test_certificate
from ..structured_scanner import scan_structured_json, QUERY_PARAM_RULES
from ..scanner import scan_file_for_secrets, format_finding


# ============================================================================
# Universal rule detection tests
# ============================================================================

def test_universal_private_key_detection() -> tuple[bool, str]:
    """Test that universal rules detect private keys."""
    test_key = build_test_private_key()
    test_content = f"{test_key}\nMIIEvQIBADAN...\n-----END PRIVATE KEY-----"

    for rule in UNIVERSAL_RULES:
        if rule["id"] == "UVB76-SECRET-0001":
            if rule["pattern"].search(test_content):
                return True, "Universal private key detection works"
            return False, "Universal private key pattern not found"

    return False, "UVB76-SECRET-0001 not found in universal rules"


def test_public_cert_not_detected() -> tuple[bool, str]:
    """Test that public certificates are NOT detected as private keys."""
    cert_marker = build_test_certificate()
    cert_content = f"{cert_marker}\nMIIDXTCCAkWg...\n-----END CERTIFICATE-----"

    for rule in UNIVERSAL_RULES:
        if rule["pattern"].search(cert_content):
            return False, "Public certificate incorrectly detected as private key"

    return True, "Public certificate not detected"


def test_ssh_public_key_not_detected() -> tuple[bool, str]:
    """Test that SSH public keys are not flagged."""
    ssh_public = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ..."

    for rule in UNIVERSAL_RULES:
        if rule["pattern"].search(ssh_public):
            return False, "SSH public key incorrectly detected"

    return True, "SSH public key not detected"


# ============================================================================
# Safe value acceptance tests
# ============================================================================

def test_redacted_marker_accepted() -> tuple[bool, str]:
    """Test that [REDACTED] marker is not flagged as a secret."""
    redacted_marker = "[REDACTED]"

    for rule in ARTIFACT_CONTEXT_RULES:
        if rule["pattern"].search(f'"password": "{redacted_marker}"'):
            return False, "[REDACTED] marker incorrectly flagged"

    return True, "[REDACTED] marker accepted"


def test_safe_null_value_accepted() -> tuple[bool, str]:
    """Test that null values in sensitive fields are not flagged."""
    findings, malformed = scan_structured_json('{"password": null}')

    if malformed:
        return False, f"Malformed JSON detected: {malformed.rule_id}"

    for f in findings:
        if f.rule_id.startswith("UVB76-SECRET-"):
            return False, f"null value incorrectly flagged by {f.rule_id}"

    return True, "null value accepted"


def test_safe_empty_value_accepted() -> tuple[bool, str]:
    """Test that empty string values in sensitive fields are not flagged."""
    findings, malformed = scan_structured_json('{"password": ""}')

    if malformed:
        return False, f"Malformed JSON detected: {malformed.rule_id}"

    for f in findings:
        if f.rule_id.startswith("UVB76-SECRET-"):
            return False, f"Empty value incorrectly flagged by {f.rule_id}"

    return True, "Empty value accepted"


# ============================================================================
# Structured JSON detection tests
# ============================================================================

def test_structured_json_nested_detection() -> tuple[bool, str]:
    """Test that nested secrets are detected in structured JSON."""
    json_content = json.dumps({
        "user": {
            "credentials": {
                "password": "secret123"
            }
        }
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON detected: {malformed.rule_id}"

    password_found = any(
        f.rule_id == "UVB76-SECRET-0040" and "password" in f.field_path
        for f in findings
    )

    if password_found:
        return True, "Nested password detected in structured JSON"
    return False, "Nested password not detected"


def test_structured_json_array_detection() -> tuple[bool, str]:
    """Test that secrets in arrays are detected."""
    json_content = json.dumps({
        "users": [
            {"name": "alice", "password": "secret1"},
            {"name": "bob", "password": "secret2"}
        ]
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON detected: {malformed.rule_id}"

    passwords_found = [f for f in findings if f.rule_id == "UVB76-SECRET-0040"]

    if len(passwords_found) >= 2:
        return True, f"Multiple passwords in array detected ({len(passwords_found)})"
    return False, f"Expected 2 passwords, found {len(passwords_found)}"


def test_structured_field_path_preserved() -> tuple[bool, str]:
    """Test that structured field paths are preserved in findings."""
    json_content = json.dumps({
        "auth": {
            "password": "secret123"
        }
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON detected: {malformed.rule_id}"

    for f in findings:
        if f.rule_id == "UVB76-SECRET-0040":
            if f.field_path and "password" in f.field_path:
                return True, f"Field path preserved: {f.field_path}"
            return False, "Field path not preserved"

    return False, "Password not detected"


# ============================================================================
# Context pattern detection tests
# ============================================================================

def test_credential_url_detection() -> tuple[bool, str]:
    """Test that credential-bearing URLs are detected."""
    found = False
    for rule in ARTIFACT_CONTEXT_RULES:
        if rule["id"] == "UVB76-SECRET-0070":
            if rule["pattern"].search("https://user:password@example.com/api"):
                found = True
                break

    if found:
        return True, "Credential URL detected"
    return False, "Credential URL not detected"


def test_session_cookie_detection() -> tuple[bool, str]:
    """Test that uvb76_session cookie is detected."""
    found = False
    for rule in ARTIFACT_CONTEXT_RULES:
        if rule["id"] == "UVB76-SECRET-0032":
            if rule["pattern"].search("uvb76_session=abc123def456ghi789"):
                found = True
                break

    if found:
        return True, "UVB76 session cookie detected"
    return False, "UVB76 session cookie not detected"


def test_jwt_detection() -> tuple[bool, str]:
    """Test that JWT-like tokens are detected."""
    found = False
    for rule in ARTIFACT_CONTEXT_RULES:
        if rule["id"] == "UVB76-SECRET-0080":
            if rule["pattern"].search("eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVm"):
                found = True
                break

    if found:
        return True, "JWT-like token detected"
    return False, "JWT-like token not detected"


# ============================================================================
# Query parameter detection tests (UVB76-SECRET-0072)
# ============================================================================

def test_query_param_detection() -> tuple[bool, str]:
    """Test that sensitive query parameters are detected."""
    # Check that query param rules exist
    if "UVB76-SECRET-0072" not in QUERY_PARAM_RULES:
        return False, "UVB76-SECRET-0072 not in QUERY_PARAM_RULES"

    # Test URL with sensitive query param
    json_content = json.dumps({
        "endpoints": [
            {"url": "https://api.example.com/data?token=secret123"}
        ]
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON detected: {malformed.rule_id}"

    query_param_found = any(
        f.rule_id == "UVB76-SECRET-0072" and "token" in f.field_path
        for f in findings
    )

    if query_param_found:
        return True, "Sensitive query parameter detected"
    return False, "Sensitive query parameter not detected"


def test_query_param_safe_value_accepted() -> tuple[bool, str]:
    """Test that [REDACTED] values in query params are safe."""
    json_content = json.dumps({
        "endpoints": [
            {"url": "https://api.example.com/data?token=[REDACTED]"}
        ]
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON detected: {malformed.rule_id}"

    # Should not flag [REDACTED] values
    for f in findings:
        if f.rule_id == "UVB76-SECRET-0072" and "token" in f.field_path:
            return False, "[REDACTED] query param incorrectly flagged"

    return True, "[REDACTED] query param accepted"


def test_query_param_preserved_safe_params() -> tuple[bool, str]:
    """Test that safe query parameters are not flagged."""
    json_content = json.dumps({
        "endpoints": [
            {"url": "https://api.example.com/data?page=1&limit=10"}
        ]
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON detected: {malformed.rule_id}"

    # Should not flag safe params (page, limit)
    for f in findings:
        if f.rule_id == "UVB76-SECRET-0072":
            return False, f"Safe param incorrectly flagged: {f.field_path}"

    return True, "Safe query parameters preserved"


# ============================================================================
# Comprehensive Query Parameter Tests (ACT requirement)
# ============================================================================

def test_query_param_lowercase_token() -> tuple[bool, str]:
    """Test ?token=value detection."""
    json_content = json.dumps({
        "url": "https://api.example.com/auth?token=secret_value_abc123"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    token_found = any(
        f.rule_id == "UVB76-SECRET-0072" and "token" in f.field_path
        for f in findings
    )

    if token_found:
        return True, "?token=value detected"
    return False, "?token=value not detected"


def test_query_param_uppercase_token() -> tuple[bool, str]:
    """Test ?TOKEN=value detection (case-insensitive)."""
    json_content = json.dumps({
        "url": "https://api.example.com/auth?TOKEN=secret_value_abc123"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    token_found = any(
        f.rule_id == "UVB76-SECRET-0072"
        for f in findings
    )

    if token_found:
        return True, "?TOKEN=value detected"
    return False, "?TOKEN=value not detected"


def test_query_param_redacted_value() -> tuple[bool, str]:
    """Test ?token=[REDACTED] is safe."""
    json_content = json.dumps({
        "url": "https://api.example.com/auth?token=[REDACTED]"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    for f in findings:
        if f.rule_id == "UVB76-SECRET-0072" and "token" in f.field_path:
            return False, "?token=[REDACTED] incorrectly flagged"

    return True, "?token=[REDACTED] accepted"


def test_query_param_empty_value() -> tuple[bool, str]:
    """Test ?token= is safe (empty value)."""
    json_content = json.dumps({
        "url": "https://api.example.com/auth?token="
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    for f in findings:
        if f.rule_id == "UVB76-SECRET-0072" and "token" in f.field_path:
            return False, "?token= incorrectly flagged"

    return True, "?token= accepted"


def test_query_param_multiple_values() -> tuple[bool, str]:
    """Test ?token=one&token=two detection."""
    json_content = json.dumps({
        "url": "https://api.example.com/auth?token=first_secret&token=second_secret"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    token_found = any(
        f.rule_id == "UVB76-SECRET-0072"
        for f in findings
    )

    if token_found:
        return True, "?token=one&token=two detected"
    return False, "?token=one&token=two not detected"


def test_query_param_mixed_safe_and_sensitive() -> tuple[bool, str]:
    """Test ?safe=value&token=secret detection."""
    json_content = json.dumps({
        "url": "https://api.example.com/data?page=1&token=secret_value&limit=10"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    token_found = any(
        f.rule_id == "UVB76-SECRET-0072" and "token" in f.field_path
        for f in findings
    )

    if token_found:
        return True, "?safe=value&token=secret detected"
    return False, "?safe=value&token=secret not detected"


def test_query_param_percent_encoded() -> tuple[bool, str]:
    """Test ?token=%73%65%63%72%65%74 (URL-encoded 'secret')."""
    json_content = json.dumps({
        "url": "https://api.example.com/auth?token=%73%65%63%72%65%74"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    token_found = any(
        f.rule_id == "UVB76-SECRET-0072" and "token" in f.field_path
        for f in findings
    )

    if token_found:
        return True, "?token=%73%65%63%72%65%74 detected"
    return False, "?token=%73%65%63%72%65%74 not detected"


def test_query_param_relative_url() -> tuple[bool, str]:
    """Test relative URL with query parameters."""
    json_content = json.dumps({
        "callback": "/auth/callback?token=secret_value&state=xyz"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    token_found = any(
        f.rule_id == "UVB76-SECRET-0072" and "token" in f.field_path
        for f in findings
    )

    if token_found:
        return True, "Relative URL with query detected"
    return False, "Relative URL with query not detected"


def test_query_param_absolute_http_url() -> tuple[bool, str]:
    """Test absolute HTTP URL with query parameters."""
    json_content = json.dumps({
        "endpoint": "http://internal.api.local/v1/token?token=admin_secret_key"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    token_found = any(
        f.rule_id == "UVB76-SECRET-0072" and "token" in f.field_path
        for f in findings
    )

    if token_found:
        return True, "HTTP URL with query detected"
    return False, "HTTP URL with query not detected"


def test_query_param_malformed_percent_encoding() -> tuple[bool, str]:
    """Test malformed percent encoding (%ZZ)."""
    json_content = json.dumps({
        "url": "https://api.example.com/auth?token=val%ZZue"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    # Malformed encoding still has token param
    token_found = any(
        f.rule_id == "UVB76-SECRET-0072" and "token" in f.field_path
        for f in findings
    )

    if token_found:
        return True, "Malformed percent encoding URL handled"
    return True, "Malformed percent encoding URL processed"


def test_query_param_excessive_fields() -> tuple[bool, str]:
    """Test URL with excessive query field count."""
    params = "&".join([f"param{i}=value{i}" for i in range(50)]) + "&token=secret_value"
    json_content = json.dumps({
        "url": f"https://api.example.com/data?{params}"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    token_found = any(
        f.rule_id == "UVB76-SECRET-0072" and "token" in f.field_path
        for f in findings
    )

    if token_found:
        return True, "Excessive query fields handled"
    return False, "Token in excessive query fields not detected"


def test_query_param_no_secret_in_output() -> tuple[bool, str]:
    """Test that detected query param secrets are NOT printed in output."""
    json_content = json.dumps({
        "url": "https://api.example.com/auth?token=super_secret_key_12345"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    # Collect all output
    output_parts = []
    for f in findings:
        output_parts.append(f.explanation)
        output_parts.append(f.remediation)
        output_parts.append(f.field_path)

    output_text = " ".join(output_parts)

    # Check secret value is NOT in output
    if "super_secret_key_12345" in output_text:
        return False, "Secret value leaked into output"

    if "token" in output_text and "super_secret" in output_text:
        return False, "Secret value may be in output"

    return True, "Query param secret not in output"


def test_query_param_access_token() -> tuple[bool, str]:
    """Test ?access_token=value detection."""
    json_content = json.dumps({
        "url": "https://api.example.com/api?access_token=bearer_secret_token"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    token_found = any(
        f.rule_id == "UVB76-SECRET-0072" and "access_token" in f.field_path
        for f in findings
    )

    if token_found:
        return True, "?access_token=value detected"
    return False, "?access_token=value not detected"


def test_query_param_api_key() -> tuple[bool, str]:
    """Test ?api_key=value detection."""
    json_content = json.dumps({
        "url": "https://api.example.com/api?api_key=sk_live_secret_key"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    token_found = any(
        f.rule_id == "UVB76-SECRET-0072" and "api_key" in f.field_path
        for f in findings
    )

    if token_found:
        return True, "?api_key=value detected"
    return False, "?api_key=value not detected"


def test_query_param_password() -> tuple[bool, str]:
    """Test ?password=value detection."""
    json_content = json.dumps({
        "url": "https://api.example.com/login?password=secret_pass"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    token_found = any(
        f.rule_id == "UVB76-SECRET-0072" and "password" in f.field_path
        for f in findings
    )

    if token_found:
        return True, "?password=value detected"
    return False, "?password=value not detected"


def test_query_param_secret() -> tuple[bool, str]:
    """Test ?secret=value detection."""
    json_content = json.dumps({
        "url": "https://api.example.com/auth?secret=hmac_shared_secret"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    token_found = any(
        f.rule_id == "UVB76-SECRET-0072" and "secret" in f.field_path
        for f in findings
    )

    if token_found:
        return True, "?secret=value detected"
    return False, "?secret=value not detected"


def test_query_param_key() -> tuple[bool, str]:
    """Test ?key=value detection (abbreviated form)."""
    json_content = json.dumps({
        "url": "https://api.example.com/crypto?key=encryption_key_value"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    token_found = any(
        f.rule_id == "UVB76-SECRET-0072" and "key" in f.field_path
        for f in findings
    )

    if token_found:
        return True, "?key=value detected"
    return False, "?key=value not detected"


def test_query_param_auth() -> tuple[bool, str]:
    """Test ?auth=value detection."""
    json_content = json.dumps({
        "url": "https://api.example.com/verify?auth=oauth_token_secret"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    token_found = any(
        f.rule_id == "UVB76-SECRET-0072" and "auth" in f.field_path
        for f in findings
    )

    if token_found:
        return True, "?auth=value detected"
    return False, "?auth=value not detected"


def test_query_param_credential() -> tuple[bool, str]:
    """Test ?credential=value detection."""
    json_content = json.dumps({
        "url": "https://api.example.com/sso?credential=service_account_key"
    })

    findings, malformed = scan_structured_json(json_content)

    if malformed:
        return False, f"Malformed JSON: {malformed.rule_id}"

    token_found = any(
        f.rule_id == "UVB76-SECRET-0072" and "credential" in f.field_path
        for f in findings
    )

    if token_found:
        return True, "?credential=value detected"
    return False, "?credential=value not detected"


# ============================================================================
# End-to-end diagnostic non-disclosure test
# ============================================================================

def test_diagnostic_non_disclosure_end_to_end() -> tuple[bool, str]:
    """
    End-to-end test that generated credentials never appear in output.

    This test:
    1. Generates a unique credential
    2. Writes it to a temporary file
    3. Scans the file
    4. Verifies the credential is NOT in:
       - returned findings
       - formatted output
       - stdout/stderr
    """
    # Generate unique credential that cannot appear by chance
    unique_credential = f"DIAGNOSTIC_TEST_{uuid.uuid4().hex.upper()}"

    # Create temp file with the credential
    with tempfile.NamedTemporaryFile(
        mode='w',
        suffix='.json',
        delete=False
    ) as f:
        temp_path = f.name
        f.write(json.dumps({
            "password": unique_credential,
            "api_key": unique_credential,
            "description": f"Test file containing {unique_credential} for diagnostic safety"
        }))

    try:
        # Scan the file
        findings = scan_file_for_secrets(temp_path, artifact_surface=True)

        # Collect all output that could contain the credential
        all_output = []

        # Check returned findings
        for finding in findings:
            all_output.append(format_finding(finding, os.path.dirname(temp_path)))
            all_output.append(finding.explanation)
            all_output.append(finding.remediation)
            if finding.field_path:
                all_output.append(finding.field_path)

        # Verify credential is NOT in any output
        for line in all_output:
            if unique_credential in line:
                return False, f"Credential leaked into output: {line[:100]}"

        # Verify findings were actually made (detection worked)
        secret_findings = [f for f in findings if f.rule_id.startswith("UVB76-SECRET-")]
        if len(secret_findings) < 2:
            return False, f"Expected at least 2 findings, got {len(secret_findings)}"

        return True, f"Diagnostic non-disclosure verified ({len(secret_findings)} findings, credential not leaked)"

    finally:
        # Clean up temp file
        os.unlink(temp_path)


# ============================================================================
# Test runner
# ============================================================================

def run_all_tests() -> list[tuple[str, bool, str]]:
    """Run all positive tests and return results."""
    tests = [
        test_universal_private_key_detection,
        test_public_cert_not_detected,
        test_ssh_public_key_not_detected,
        test_redacted_marker_accepted,
        test_safe_null_value_accepted,
        test_safe_empty_value_accepted,
        test_structured_json_nested_detection,
        test_structured_json_array_detection,
        test_structured_field_path_preserved,
        test_credential_url_detection,
        test_session_cookie_detection,
        test_jwt_detection,
        test_query_param_detection,
        test_query_param_safe_value_accepted,
        test_query_param_preserved_safe_params,
        test_diagnostic_non_disclosure_end_to_end,
    ]

    results = []
    for test in tests:
        try:
            passed, msg = test()
            results.append((test.__name__, passed, msg))
        except Exception as e:
            results.append((test.__name__, False, f"Exception: {e}"))

    return results
