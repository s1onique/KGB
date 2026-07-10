"""
Registry projection validation for UVB-76 Artifact Secret Hygiene.

Verifies all registry rules have detector implementations.
"""

from .registry_loader import get_registry, get_artifact_context_rules
from .structured_scanner import FIELD_RULES, QUERY_PARAM_RULES


def validate_field_rules_coverage() -> list[str]:
    """Verify all field_name rules in registry have detector coverage."""
    errors = []
    registry = get_registry()

    for rule in get_artifact_context_rules(registry):
        if rule.get("detector_kind") == "field_name":
            rule_id = rule["rule_id"]
            if rule_id not in FIELD_RULES:
                errors.append(f"{rule_id}: field_name detector has no coverage")

    return errors


def validate_query_param_rules_coverage() -> list[str]:
    """Verify all url_component rules with query_params have detector coverage."""
    errors = []
    registry = get_registry()

    for rule in get_artifact_context_rules(registry):
        if rule.get("detector_kind") == "url_component":
            query_params = rule.get("query_params", [])
            if query_params:
                rule_id = rule["rule_id"]
                if rule_id not in QUERY_PARAM_RULES:
                    errors.append(f"{rule_id}: query_params detector has no coverage")

    return errors


def validate_all_rules_have_projections() -> list[str]:
    """
    Verify every registry rule has at least one projection.

    Returns list of rule_ids that have no implementation.
    """
    errors = []

    # Get all rule IDs from registry
    registry = get_registry()
    registry_rule_ids = {rule["rule_id"] for rule in registry.get("rules", [])}

    # Track which rules have projections
    projected_rules: set[str] = set()

    # Universal pattern rules
    from .rules import UNIVERSAL_RULES
    for rule in UNIVERSAL_RULES:
        projected_rules.add(rule["id"])

    # Context pattern rules
    from .rules import ARTIFACT_CONTEXT_RULES
    for rule in ARTIFACT_CONTEXT_RULES:
        projected_rules.add(rule["id"])

    # Field name rules
    for rule_id in FIELD_RULES:
        projected_rules.add(rule_id)

    # Query param rules
    for rule_id in QUERY_PARAM_RULES:
        projected_rules.add(rule_id)

    # Find unprojected rules
    unprojected = registry_rule_ids - projected_rules
    for rule_id in sorted(unprojected):
        errors.append(f"{rule_id}: no detector projection found")

    return errors
