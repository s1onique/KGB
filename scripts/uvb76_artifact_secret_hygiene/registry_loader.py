"""
Registry loader for UVB-76 Artifact Secret Hygiene.

Loads and validates the canonical secret rule registry.
"""

import json
import os
import re
from typing import Any

# Path to the canonical registry
REGISTRY_PATH = os.path.join(os.path.dirname(__file__), "registry.json")

# Valid scope values
VALID_SCOPES = {"universal", "artifact_context"}

# Valid severity values
VALID_SEVERITIES = {"critical", "high", "medium"}

# Valid allowlistable values
VALID_ALLOWLISTABLE = {True, False}

# Valid detector kinds
VALID_DETECTOR_KINDS = {"pattern", "field_name", "header_name", "url_component", "structured_json"}


class RegistryValidationError(Exception):
    """Raised when registry validation fails."""
    pass


def load_registry() -> dict[str, Any]:
    """Load the canonical registry from JSON."""
    with open(REGISTRY_PATH, 'r') as f:
        return json.load(f)


def validate_registry(registry: dict[str, Any]) -> list[str]:
    """
    Validate the registry structure and contents.
    Returns list of error messages (empty if valid).
    """
    errors = []

    # Check required top-level keys
    required_keys = {"version", "description", "rules"}
    for key in required_keys:
        if key not in registry:
            errors.append(f"Missing required key: {key}")

    if "rules" not in registry:
        return errors

    # Validate each rule
    seen_ids: set[str] = set()
    seen_classes: set[str] = set()

    for i, rule in enumerate(registry.get("rules", [])):
        rule_id = rule.get("rule_id", f"<missing at index {i}>")

        # Check required fields
        required_fields = ["rule_id", "class", "scope", "severity", "allowlistable",
                          "safe_explanation", "safe_remediation", "detector_kind"]
        for field in required_fields:
            if field not in rule:
                errors.append(f"{rule_id}: missing required field '{field}'")

        # Validate scope
        if "scope" in rule and rule["scope"] not in VALID_SCOPES:
            errors.append(f"{rule_id}: invalid scope '{rule['scope']}' (valid: {VALID_SCOPES})")

        # Validate severity
        if "severity" in rule and rule["severity"] not in VALID_SEVERITIES:
            errors.append(f"{rule_id}: invalid severity '{rule['severity']}' (valid: {VALID_SEVERITIES})")

        # Validate allowlistable
        if "allowlistable" in rule and rule["allowlistable"] not in VALID_ALLOWLISTABLE:
            errors.append(f"{rule_id}: invalid allowlistable '{rule['allowlistable']}' (valid: {VALID_ALLOWLISTABLE})")

        # Validate detector_kind
        if "detector_kind" in rule and rule["detector_kind"] not in VALID_DETECTOR_KINDS:
            errors.append(f"{rule_id}: invalid detector_kind '{rule['detector_kind']}' (valid: {VALID_DETECTOR_KINDS})")

        # Check for duplicate rule_id
        if rule_id in seen_ids:
            errors.append(f"Duplicate rule_id: {rule_id}")
        seen_ids.add(rule_id)

        # Check for duplicate class (classes should be unique)
        rule_class = rule.get("class", "")
        if rule_class in seen_classes:
            errors.append(f"{rule_id}: duplicate class '{rule_class}'")
        seen_classes.add(rule_class)

        # Validate explanation and remediation don't contain secret-like content
        explanation = rule.get("safe_explanation", "")
        remediation = rule.get("safe_remediation", "")

        # Should not contain actual secret patterns
        if re.search(r'MII[B-Z]|\bpassword\s*=\s*["\']', explanation, re.IGNORECASE):
            errors.append(f"{rule_id}: explanation may contain secret-like content")
        if re.search(r'MII[B-Z]|\bpassword\s*=\s*["\']', remediation, re.IGNORECASE):
            errors.append(f"{rule_id}: remediation may contain secret-like content")

        # Validate detector_kind-specific requirements
        detector_kind = rule.get("detector_kind", "")

        if detector_kind == "pattern":
            if "pattern" not in rule:
                errors.append(f"{rule_id}: pattern detector requires 'pattern' field")
            # Validate pattern is valid regex
            elif rule["pattern"]:
                try:
                    re.compile(rule["pattern"])
                except re.error as e:
                    errors.append(f"{rule_id}: invalid regex pattern: {e}")

        elif detector_kind == "field_name":
            if "field_names" not in rule:
                errors.append(f"{rule_id}: field_name detector requires 'field_names' field")
            elif not isinstance(rule["field_names"], list):
                errors.append(f"{rule_id}: field_names must be a list")

        elif detector_kind == "header_name":
            if "header_pattern" not in rule:
                errors.append(f"{rule_id}: header_name detector requires 'header_pattern' field")

        elif detector_kind == "url_component":
            if "pattern" not in rule and "query_params" not in rule:
                errors.append(f"{rule_id}: url_component detector requires 'pattern' or 'query_params' field")

    return errors


def get_registry() -> dict[str, Any]:
    """Load and validate the registry. Raises on validation failure."""
    registry = load_registry()
    errors = validate_registry(registry)
    if errors:
        raise RegistryValidationError(f"Registry validation failed:\n" + "\n".join(f"  - {e}" for e in errors))
    return registry


def get_rule_by_id(registry: dict[str, Any], rule_id: str) -> dict[str, Any] | None:
    """Get a rule by its ID."""
    for rule in registry.get("rules", []):
        if rule["rule_id"] == rule_id:
            return rule
    return None


def get_universal_rules(registry: dict[str, Any]) -> list[dict[str, Any]]:
    """Get all universal scope rules."""
    return [r for r in registry.get("rules", []) if r.get("scope") == "universal"]


def get_artifact_context_rules(registry: dict[str, Any]) -> list[dict[str, Any]]:
    """Get all artifact_context scope rules."""
    return [r for r in registry.get("rules", []) if r.get("scope") == "artifact_context"]


def get_all_rule_ids(registry: dict[str, Any]) -> set[str]:
    """Get all rule IDs from registry."""
    return {r["rule_id"] for r in registry.get("rules", [])}


def get_all_classes(registry: dict[str, Any]) -> set[str]:
    """Get all rule classes from registry."""
    return {r["class"] for r in registry.get("rules", [])}
