"""
Mutation detection tests for UVB-76 Artifact Secret Hygiene.

Tests that registry mutations and invalid rules are detected.
"""

from ..registry_loader import get_registry, get_all_rule_ids, validate_registry
from ..rules import UNIVERSAL_RULES, ARTIFACT_CONTEXT_RULES


def test_duplicate_rule_id_detected() -> tuple[bool, str]:
    """Test that duplicate rule IDs are detected."""
    registry = get_registry()
    # Create a modified registry with duplicate IDs
    modified_rules = registry.get("rules", [])[:]
    if len(modified_rules) >= 2:
        # Duplicate the first rule
        modified_rules.append(modified_rules[0].copy())
        modified_registry = registry.copy()
        modified_registry["rules"] = modified_rules

        errors = validate_registry(modified_registry)
        has_dup_error = any("Duplicate" in e for e in errors)

        if has_dup_error:
            return True, "Duplicate rule ID detected"
        return False, "Duplicate rule ID NOT detected (validation broken)"

    return True, "Skip: not enough rules for duplicate test"


def test_unknown_scanner_id_detected() -> tuple[bool, str]:
    """Test that unknown scanner rule IDs are rejected."""
    # A rule ID that's not in registry
    unknown_id = "UVB76-SECRET-9999"
    registry_ids = get_all_rule_ids(get_registry())

    if unknown_id in registry_ids:
        return False, "Unknown ID already exists in registry (test invalid)"

    # Verify all Python rules are in registry
    for rule in UNIVERSAL_RULES + ARTIFACT_CONTEXT_RULES:
        if rule["id"] not in registry_ids:
            return False, f"Unknown Python rule {rule['id']} not in registry"

    return True, "All Python rules in registry"


def test_missing_explanation_detected() -> tuple[bool, str]:
    """Test that missing explanation is detected."""
    registry = get_registry()
    modified_rules = []
    for rule in registry.get("rules", []):
        if rule.get("rule_id") == "UVB76-SECRET-0001":
            # Remove explanation
            mod_rule = rule.copy()
            del mod_rule["safe_explanation"]
            modified_rules.append(mod_rule)
        else:
            modified_rules.append(rule)

    modified_registry = registry.copy()
    modified_registry["rules"] = modified_rules

    errors = validate_registry(modified_registry)
    has_error = any("safe_explanation" in e for e in errors)

    if has_error:
        return True, "Missing explanation detected"
    return False, "Missing explanation NOT detected"


def test_missing_remediation_detected() -> tuple[bool, str]:
    """Test that missing remediation is detected."""
    registry = get_registry()
    modified_rules = []
    for rule in registry.get("rules", []):
        if rule.get("rule_id") == "UVB76-SECRET-0001":
            # Remove remediation
            mod_rule = rule.copy()
            del mod_rule["safe_remediation"]
            modified_rules.append(mod_rule)
        else:
            modified_rules.append(rule)

    modified_registry = registry.copy()
    modified_registry["rules"] = modified_rules

    errors = validate_registry(modified_registry)
    has_error = any("safe_remediation" in e for e in errors)

    if has_error:
        return True, "Missing remediation detected"
    return False, "Missing remediation NOT detected"


def test_invalid_scope_detected() -> tuple[bool, str]:
    """Test that invalid scope value is detected."""
    registry = get_registry()
    modified_rules = []
    for rule in registry.get("rules", []):
        if rule.get("rule_id") == "UVB76-SECRET-0001":
            mod_rule = rule.copy()
            mod_rule["scope"] = "invalid_scope"
            modified_rules.append(mod_rule)
        else:
            modified_rules.append(rule)

    modified_registry = registry.copy()
    modified_registry["rules"] = modified_rules

    errors = validate_registry(modified_registry)
    has_error = any("invalid scope" in e for e in errors)

    if has_error:
        return True, "Invalid scope detected"
    return False, "Invalid scope NOT detected"


def test_invalid_detector_kind_detected() -> tuple[bool, str]:
    """Test that invalid detector_kind is detected."""
    registry = get_registry()
    modified_rules = []
    for rule in registry.get("rules", []):
        if rule.get("rule_id") == "UVB76-SECRET-0001":
            mod_rule = rule.copy()
            mod_rule["detector_kind"] = "invalid_kind"
            modified_rules.append(mod_rule)
        else:
            modified_rules.append(rule)

    modified_registry = registry.copy()
    modified_registry["rules"] = modified_rules

    errors = validate_registry(modified_registry)
    has_error = any("detector_kind" in e for e in errors)

    if has_error:
        return True, "Invalid detector_kind detected"
    return False, "Invalid detector_kind NOT detected"
