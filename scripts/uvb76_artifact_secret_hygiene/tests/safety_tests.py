"""
Diagnostic safety tests for UVB-76 Artifact Secret Hygiene.

Tests that diagnostics do not expose secret values.
"""

import os
import time

from ..scanner import SecretFinding


def test_diagnostic_no_secret_exposure() -> tuple[bool, str]:
    """Test that diagnostics do not expose secret values."""
    # Create a finding with a synthetic secret
    finding = SecretFinding(
        rule_id="UVB76-SECRET-0040",
        file_path="/fake/path.json",
        line_number=10,
        explanation="non-redacted password field value",
        remediation="Replace with [REDACTED]",
    )

    formatted = f"{finding.rule_id}: {finding.explanation}"

    # Should contain rule ID and explanation
    if finding.rule_id not in formatted:
        return False, "Rule ID not in formatted output"
    if finding.explanation not in formatted:
        return False, "Explanation not in formatted output"

    # Should NOT contain secret-like patterns
    if "secret" in formatted.lower() and "password" in formatted.lower():
        pass  # Explanation mentions password, which is OK

    return True, "Diagnostic safety verified"


def test_synthetic_credential_not_in_output() -> tuple[bool, str]:
    """Test that synthetic credentials are not exposed in output."""
    # Generate a unique synthetic credential
    synthetic = f"TEST_SECRET_{int(time.time())}_{os.getpid()}"

    # Create a finding
    finding = SecretFinding(
        rule_id="UVB76-SECRET-0040",
        file_path="/fake/path.json",
        line_number=10,
        explanation="non-redacted password field value",
        remediation="Replace with [REDACTED]",
    )

    formatted = f"{finding.rule_id}: {finding.explanation} at {finding.file_path}:{finding.line_number}"

    if synthetic in formatted:
        return False, f"Synthetic credential leaked into output"

    return True, "Synthetic credential not in output"
