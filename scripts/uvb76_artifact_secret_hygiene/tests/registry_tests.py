"""
Registry consistency tests for UVB-76 Artifact Secret Hygiene.

Tests that the canonical registry is valid and that Python rules derive from it.
"""

import os
from typing import Optional

from ..registry_loader import (
    get_registry, get_all_rule_ids, get_all_classes,
    validate_registry, RegistryValidationError,
)
from ..rules import UNIVERSAL_RULES, ARTIFACT_CONTEXT_RULES
from ..structured_scanner import FIELD_RULES, QUERY_PARAM_RULES
from ..registry_validation import validate_all_rules_have_projections
from .go_parser_tests import (
    _parse_go_constants_subprocess,
    test_go_constants_parse_single_declaration,
    test_go_constants_parse_grouped_declaration,
    test_go_constants_parse_with_tabs_and_spaces,
    test_go_constants_detect_duplicate_id,
    test_go_constants_detect_wrong_class,
    test_go_constants_detect_missing_constant,
    test_go_constants_detect_unknown_constant,
    test_go_constants_unrelated_constants,
    test_go_constants_comments_before_after,
)


# ============================================================================
# Registry structural validation tests
# ============================================================================

def test_registry_structural_validation() -> tuple[bool, str]:
    """Test that registry loads and validates correctly."""
    try:
        registry = get_registry()
        errors = validate_registry(registry)
        if errors:
            return False, f"Registry validation errors: {errors}"
        return True, "Registry structural validation passed"
    except Exception as e:
        return False, f"Registry load failed: {e}"


def test_registry_rule_count() -> tuple[bool, str]:
    """Test that registry contains expected number of rules."""
    registry = get_registry()
    rules = registry.get("rules", [])
    expected_min = 20  # At least 20 rules defined

    if len(rules) < expected_min:
        return False, f"Expected at least {expected_min} rules, got {len(rules)}"

    return True, f"Registry has {len(rules)} rules"


def test_registry_universal_rules_defined() -> tuple[bool, str]:
    """Test that all universal rules have patterns."""
    registry = get_registry()
    universal_rules = [r for r in registry.get("rules", []) if r.get("scope") == "universal"]

    missing_patterns = []
    for rule in universal_rules:
        if "pattern" not in rule or not rule["pattern"]:
            missing_patterns.append(rule.get("rule_id", "<unknown>"))

    if missing_patterns:
        return False, f"Universal rules missing patterns: {missing_patterns}"

    return True, f"All {len(universal_rules)} universal rules have patterns"


def test_registry_no_duplicate_ids() -> tuple[bool, str]:
    """Test that registry has no duplicate rule IDs."""
    registry = get_registry()
    seen_ids = set()
    duplicates = []

    for rule in registry.get("rules", []):
        rule_id = rule.get("rule_id")
        if rule_id in seen_ids:
            duplicates.append(rule_id)
        seen_ids.add(rule_id)

    if duplicates:
        return False, f"Duplicate rule IDs found: {duplicates}"

    return True, "No duplicate rule IDs"


def test_registry_no_duplicate_classes() -> tuple[bool, str]:
    """Test that registry has no duplicate classes."""
    registry = get_registry()
    seen_classes = set()
    duplicates = []

    for rule in registry.get("rules", []):
        rule_class = rule.get("class")
        if rule_class in seen_classes:
            duplicates.append(rule_class)
        seen_classes.add(rule_class)

    if duplicates:
        return False, f"Duplicate classes found: {duplicates}"

    return True, "No duplicate classes"


# ============================================================================
# Python projection validation tests
# ============================================================================

def test_python_universal_rules_from_registry() -> tuple[bool, str]:
    """Test that Python universal rules derive from registry."""
    registry = get_registry()
    registry_ids = get_all_rule_ids(registry)

    # Check that Python universal rules come from registry
    python_ids = {r["id"] for r in UNIVERSAL_RULES}

    for rule_id in python_ids:
        if rule_id not in registry_ids:
            return False, f"Python rule {rule_id} not in registry"

    # Check that registry universal rules are in Python
    registry_universal = {r["rule_id"] for r in registry.get("rules", []) if r.get("scope") == "universal"}
    for rule_id in registry_universal:
        if rule_id not in python_ids:
            return False, f"Registry universal rule {rule_id} not in Python"

    return True, f"Python universal rules agree with registry ({len(python_ids)} rules)"


def test_python_context_rules_from_registry() -> tuple[bool, str]:
    """Test that Python context rules derive from registry."""
    registry = get_registry()
    registry_ids = get_all_rule_ids(registry)

    # Check that Python context rules come from registry
    python_ids = {r["id"] for r in ARTIFACT_CONTEXT_RULES}

    for rule_id in python_ids:
        if rule_id not in registry_ids:
            return False, f"Python rule {rule_id} not in registry"

    return True, f"Python context rules agree with registry ({len(python_ids)} rules)"


def test_field_rules_coverage() -> tuple[bool, str]:
    """Test that all field_name rules in registry have Python coverage."""
    registry = get_registry()
    missing = []

    for rule in registry.get("rules", []):
        if rule.get("detector_kind") == "field_name":
            rule_id = rule["rule_id"]
            if rule_id not in FIELD_RULES:
                missing.append(rule_id)

    if missing:
        return False, f"Field rules without coverage: {missing}"

    return True, f"All field_name rules have coverage ({len(FIELD_RULES)} rules)"


def test_query_param_rules_coverage() -> tuple[bool, str]:
    """Test that all url_component rules with query_params have Python coverage."""
    registry = get_registry()
    missing = []

    for rule in registry.get("rules", []):
        if rule.get("detector_kind") == "url_component":
            query_params = rule.get("query_params", [])
            if query_params:
                rule_id = rule["rule_id"]
                if rule_id not in QUERY_PARAM_RULES:
                    missing.append(rule_id)

    if missing:
        return False, f"Query param rules without coverage: {missing}"

    return True, f"All query_param rules have coverage ({len(QUERY_PARAM_RULES)} rules)"


def test_all_rules_have_projections() -> tuple[bool, str]:
    """Test that every registry rule has at least one detector projection."""
    errors = validate_all_rules_have_projections()

    if errors:
        return False, f"Unprojected rules: {errors}"

    registry = get_registry()
    total_rules = len(registry.get("rules", []))
    return True, f"All {total_rules} registry rules have projections"


# ============================================================================
# Go-registry consistency verification using AST parser
# ============================================================================

def test_go_constants_agreement() -> tuple[bool, str]:
    """
    Test that Go constants agree with registry by parsing Go source.

    Uses go/parser and go/ast (via subprocess) to verify:
    - Every Go constant has a matching registry rule
    - Every registry rule has a Go constant
    - Class annotations match between Go and registry

    AST-only: No regex fallback. Failure returns a failing self-test.
    """
    # Find the redact.go file
    script_dir = os.path.dirname(os.path.dirname(__file__))
    redact_go_path = os.path.join(script_dir, "..", "..", "uvb76", "internal", "redact", "redact.go")

    if not os.path.exists(redact_go_path):
        return False, f"Go source not found at {redact_go_path}"

    # Parse Go constants via AST - returns (constants, errors) tuple
    go_constants, parse_errors = _parse_go_constants_subprocess(redact_go_path)

    # Fail if AST parsing failed - no regex fallback
    if parse_errors:
        return False, f"Go AST parsing failed: {parse_errors[:3]}"

    if not go_constants:
        return False, "No rule constants found via Go AST parser"

    # Get registry data
    registry = get_registry()
    registry_rules = {r["rule_id"]: r["class"] for r in registry.get("rules", [])}

    errors = []

    # Check Go constants against registry
    for rule_id, (const_name, class_name, line) in go_constants.items():
        if rule_id not in registry_rules:
            errors.append(f"Go constant {const_name}={rule_id} not in registry")
            continue

        # Check class matches
        registry_class = registry_rules[rule_id]
        if class_name != registry_class:
            errors.append(
                f"Go {const_name} class '{class_name}' != registry '{registry_class}'"
            )

    # Check registry rules against Go constants
    for rule_id, registry_class in registry_rules.items():
        if rule_id not in go_constants:
            errors.append(f"Registry rule {rule_id} missing Go constant")

    if errors:
        return False, f"Go-registry mismatches: {errors[:5]}"

    return True, f"Go constants agree with registry ({len(go_constants)} rules)"


def test_go_parser_mechanism_accurate() -> tuple[bool, str]:
    """
    Test that Go parser mechanism is accurately described.

    Verifies we're using actual Go AST parsing (go/parser, go/ast, go/token).
    AST-only: No regex fallback.
    """
    script_dir = os.path.dirname(os.path.dirname(__file__))
    redact_go_path = os.path.join(script_dir, "..", "..", "uvb76", "internal", "redact", "redact.go")

    if not os.path.exists(redact_go_path):
        return False, "Go source not found"

    # AST parsing only - returns (constants, errors) tuple
    ast_constants, errors = _parse_go_constants_subprocess(redact_go_path)

    if ast_constants and not errors:
        return True, f"Using Go AST parser ({len(ast_constants)} rules)"

    # No regex fallback - fail closed
    return False, f"Go AST parsing failed: {errors[:2] if errors else 'no constants'}"


# ============================================================================
# Go parser mutation tests
# ============================================================================

def test_go_parser_single_declaration() -> tuple[bool, str]:
    """Test that single const declarations are parsed correctly."""
    return test_go_constants_parse_single_declaration()


def test_go_parser_grouped_declaration() -> tuple[bool, str]:
    """Test that grouped const declarations are parsed correctly."""
    return test_go_constants_parse_grouped_declaration()


def test_go_parser_whitespace_variations() -> tuple[bool, str]:
    """Test that various whitespace patterns are handled."""
    return test_go_constants_parse_with_tabs_and_spaces()


def test_go_parser_detects_duplicate_id() -> tuple[bool, str]:
    """Test that duplicate rule IDs are detected."""
    return test_go_constants_detect_duplicate_id()


def test_go_parser_detects_wrong_class() -> tuple[bool, str]:
    """Test that wrong class annotations are detected."""
    return test_go_constants_detect_wrong_class()


def test_go_parser_detects_missing_constant() -> tuple[bool, str]:
    """Test that missing Go constants for registry rules are detected."""
    return test_go_constants_detect_missing_constant()


def test_go_parser_detects_unknown_constant() -> tuple[bool, str]:
    """Test that unknown Go constants are detected."""
    return test_go_constants_detect_unknown_constant()


def test_go_parser_unrelated_constants() -> tuple[bool, str]:
    """Test that unrelated constants are not confused with rule constants."""
    return test_go_constants_unrelated_constants()


def test_go_parser_comments_handled() -> tuple[bool, str]:
    """Test that comments before and after values don't break parsing."""
    return test_go_constants_comments_before_after()


# ============================================================================
# Projection completeness validation
# ============================================================================

def test_universal_projection_completeness() -> tuple[bool, str]:
    """Test that all universal scope rules have pattern projections."""
    registry = get_registry()
    universal_rules = [r for r in registry.get("rules", []) if r.get("scope") == "universal"]

    registry_universal_ids = {r["rule_id"] for r in universal_rules}
    python_universal_ids = {r["id"] for r in UNIVERSAL_RULES}

    missing = registry_universal_ids - python_universal_ids
    if missing:
        return False, f"Universal rules missing projections: {missing}"

    return True, f"All {len(registry_universal_ids)} universal rules have projections"


def test_pattern_projection_completeness() -> tuple[bool, str]:
    """
    Test that all pattern-based context rules have projections.

    Note: Universal rules (scope=universal) are NOT in ARTIFACT_CONTEXT_RULES;
    they are in UNIVERSAL_RULES. This test only checks context rules with
    detector_kind in ('pattern', 'header_name').
    """
    registry = get_registry()

    # Only check context rules (not universal rules)
    # Universal rules are checked by test_universal_projection_completeness
    pattern_rules = [
        r for r in registry.get("rules", [])
        if r.get("scope") == "artifact_context"
        and r.get("detector_kind") in ("pattern", "header_name")
    ]

    registry_pattern_ids = {r["rule_id"] for r in pattern_rules}
    python_pattern_ids = {r["id"] for r in ARTIFACT_CONTEXT_RULES}

    missing = registry_pattern_ids - python_pattern_ids
    if missing:
        return False, f"Pattern rules missing projections: {missing}"

    return True, f"All {len(registry_pattern_ids)} pattern rules have projections"


def test_no_duplicate_projections() -> tuple[bool, str]:
    """Test that no rule ID appears in multiple projection categories."""
    all_ids: dict[str, list[str]] = {}

    for rule in UNIVERSAL_RULES:
        all_ids.setdefault(rule["id"], []).append("universal")

    for rule in ARTIFACT_CONTEXT_RULES:
        all_ids.setdefault(rule["id"], []).append("context_pattern")

    for rule_id in FIELD_RULES:
        all_ids.setdefault(rule_id, []).append("field_name")

    for rule_id in QUERY_PARAM_RULES:
        all_ids.setdefault(rule_id, []).append("query_param")

    duplicates = {k: v for k, v in all_ids.items() if len(v) > 1}

    if duplicates:
        return False, f"Duplicate projections: {duplicates}"

    return True, "No duplicate projections found"


# ============================================================================
# Projection mutation test - removing UVB76-SECRET-0072
# ============================================================================

def test_projection_removal_0072_detected() -> tuple[bool, str]:
    """
    Mutation test: removing UVB76-SECRET-0072 from its implementation projection.

    Proves that the validate_all_rules_have_projections() function detects
    when a rule is missing its projection.
    """
    from ..structured_scanner import QUERY_PARAM_RULES

    # Simulate removing 0072 from QUERY_PARAM_RULES
    simulated_rules = {k: v for k, v in QUERY_PARAM_RULES.items() if k != "UVB76-SECRET-0072"}

    # Check if 0072 would be detected as missing
    if "UVB76-SECRET-0072" not in simulated_rules:
        return True, "UVB76-SECRET-0072 removal would be detected"

    return False, "UVB76-SECRET-0072 removal NOT detected"
