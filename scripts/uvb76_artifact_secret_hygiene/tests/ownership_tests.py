"""
Ownership mapping tests for UVB-76 Artifact Secret Hygiene.

Tests that the canonical ownership mapping is valid and complete.
"""

from ..ownership import (
    RULE_OWNERSHIP,
    VALID_OWNERSHIP_KINDS,
    validate_ownership,
    get_ownership_for_rule,
    get_all_ownership_rule_ids,
    count_ownership_entries,
    count_unique_rules,
)


# ============================================================================
# Ownership structural validation tests
# ============================================================================

def test_ownership_validates() -> tuple[bool, str]:
    """Test that ownership mapping passes validation."""
    errors = validate_ownership()
    if errors:
        return False, f"Ownership validation failed: {errors}"
    return True, "Ownership validation passed"


def test_ownership_count_matches_registry() -> tuple[bool, str]:
    """Test that ownership has entry for every registry rule."""
    from ..registry_loader import get_registry

    registry = get_registry()
    registry_rule_ids = {r["rule_id"] for r in registry.get("rules", [])}
    ownership_rule_ids = get_all_ownership_rule_ids()

    missing = registry_rule_ids - ownership_rule_ids
    if missing:
        return False, f"Missing ownership for rules: {sorted(missing)}"

    extra = ownership_rule_ids - registry_rule_ids
    if extra:
        return False, f"Unknown rule IDs in ownership: {sorted(extra)}"

    return True, f"Ownership covers all {len(ownership_rule_ids)} registry rules"


def test_ownership_count_23() -> tuple[bool, str]:
    """Test that ownership has 23 unique rule IDs."""
    unique_rules = count_unique_rules()
    if unique_rules != 23:
        return False, f"Expected 23 unique rule IDs, got {unique_rules}"
    return True, f"Ownership has {unique_rules} unique rule IDs"


def test_ownership_entries_count() -> tuple[bool, str]:
    """Test that ownership has 23 total entries."""
    entries = count_ownership_entries()
    # Each rule has at least one ownership kind
    if entries < 23:
        return False, f"Expected at least 23 ownership entries, got {entries}"
    return True, f"Ownership has {entries} total entries"


# ============================================================================
# Ownership kind validation tests
# ============================================================================

def test_all_ownership_kinds_valid() -> tuple[bool, str]:
    """Test that all ownership kinds are from the closed vocabulary."""
    invalid_kinds: list[tuple[str, str]] = []

    for rule_id, kinds in RULE_OWNERSHIP.items():
        for kind in kinds:
            if kind not in VALID_OWNERSHIP_KINDS:
                invalid_kinds.append((rule_id, kind))

    if invalid_kinds:
        return False, f"Invalid ownership kinds: {invalid_kinds}"

    return True, "All ownership kinds are valid"


def test_no_empty_ownership() -> tuple[bool, str]:
    """Test that no rule has empty ownership."""
    empty_rules = []

    for rule_id, kinds in RULE_OWNERSHIP.items():
        if not kinds:
            empty_rules.append(rule_id)

    if empty_rules:
        return False, f"Rules with empty ownership: {empty_rules}"

    return True, "No empty ownership entries"


# ============================================================================
# Ownership mapping tests
# ============================================================================

def test_private_key_rules_have_correct_ownership() -> tuple[bool, str]:
    """Test that private key rules have private_key_marker_redactor."""
    expected = "private_key_marker_redactor"
    private_key_ids = [
        "UVB76-SECRET-0001",
        "UVB76-SECRET-0002",
        "UVB76-SECRET-0003",
        "UVB76-SECRET-0004",
        "UVB76-SECRET-0005",
    ]

    for rule_id in private_key_ids:
        ownership = get_ownership_for_rule(rule_id)
        if expected not in ownership:
            return False, f"{rule_id} missing {expected}"

    return True, "Private key rules have correct ownership"


def test_header_rules_have_typed_ownership() -> tuple[bool, str]:
    """Test that header rules have typed_header_redactor."""
    expected = "typed_header_redactor"
    header_ids = [
        "UVB76-SECRET-0010",
        "UVB76-SECRET-0011",
        "UVB76-SECRET-0012",
        "UVB76-SECRET-0013",
        "UVB76-SECRET-0020",
    ]

    for rule_id in header_ids:
        ownership = get_ownership_for_rule(rule_id)
        if expected not in ownership:
            return False, f"{rule_id} missing {expected}"

    return True, "Header rules have typed ownership"


def test_cookie_rules_have_correct_ownership() -> tuple[bool, str]:
    """Test that cookie rules have correct ownership."""
    request_cookie_ids = ["UVB76-SECRET-0030", "UVB76-SECRET-0032"]
    set_cookie_ids = ["UVB76-SECRET-0031"]

    for rule_id in request_cookie_ids:
        ownership = get_ownership_for_rule(rule_id)
        if "request_cookie_redactor" not in ownership:
            return False, f"{rule_id} missing request_cookie_redactor"

    for rule_id in set_cookie_ids:
        ownership = get_ownership_for_rule(rule_id)
        if "set_cookie_redactor" not in ownership:
            return False, f"{rule_id} missing set_cookie_redactor"

    return True, "Cookie rules have correct ownership"


def test_url_rules_have_url_ownership() -> tuple[bool, str]:
    """Test that URL rules have url_redactor."""
    expected = "url_redactor"
    url_ids = [
        "UVB76-SECRET-0070",
        "UVB76-SECRET-0071",
        "UVB76-SECRET-0072",
    ]

    for rule_id in url_ids:
        ownership = get_ownership_for_rule(rule_id)
        if expected not in ownership:
            return False, f"{rule_id} missing {expected}"

    return True, "URL rules have url_redactor ownership"


def test_detection_only_rules() -> tuple[bool, str]:
    """Test that JWT/bearer rules have repository_detection_only."""
    expected = "repository_detection_only"
    detection_ids = [
        "UVB76-SECRET-0080",
        "UVB76-SECRET-0081",
    ]

    for rule_id in detection_ids:
        ownership = get_ownership_for_rule(rule_id)
        if expected not in ownership:
            return False, f"{rule_id} missing {expected}"

    return True, "Detection-only rules have correct ownership"


# ============================================================================
# Legacy ownership mutation tests (baseline detection)
# These tests check the RULE_OWNERSHIP dict directly.
# New mutation tests use validate_ownership() with explicit entries.
# ============================================================================

def test_baseline_missing_ownership_detected() -> tuple[bool, str]:
    """
    Baseline test: verify all registry rules have ownership.

    This checks that the canonical RULE_OWNERSHIP covers all registry rules.
    It does not mutate entries - it validates the projection.
    """
    errors = validate_ownership()

    # Validation should pass for the correct mapping
    if errors:
        return False, f"Baseline validation failed: {errors}"

    # Simulate missing entry by checking that all registry rules are covered
    from ..registry_loader import get_registry
    registry = get_registry()
    registry_rule_ids = {r["rule_id"] for r in registry.get("rules", [])}
    ownership_rule_ids = get_all_ownership_rule_ids()

    missing = registry_rule_ids - ownership_rule_ids
    if missing:
        return False, f"Mutation detected: missing ownership for {missing}"

    return True, "All registry rules have ownership (baseline)"


def test_baseline_unknown_ownership_kind_detected() -> tuple[bool, str]:
    """
    Baseline test: verify all ownership kinds are valid.
    """
    # Check that all kinds are valid
    all_kinds = set()
    for kinds in RULE_OWNERSHIP.values():
        all_kinds.update(kinds)

    unknown = all_kinds - VALID_OWNERSHIP_KINDS
    if unknown:
        return False, f"Unknown ownership kinds found: {unknown}"

    return True, "All ownership kinds are valid (baseline)"


def test_baseline_empty_ownership_detected() -> tuple[bool, str]:
    """
    Baseline test: verify no rule has empty ownership.
    """
    for rule_id, kinds in RULE_OWNERSHIP.items():
        if not kinds:
            return False, f"Empty ownership for {rule_id} would be detected"

    return True, "No empty ownership entries (baseline)"


def test_baseline_duplicate_ownership_detected() -> tuple[bool, str]:
    """
    Baseline test: verify no duplicate ownership kinds.
    """
    seen: dict[str, set[str]] = {}
    duplicates: list[tuple[str, str]] = []

    for rule_id, kinds in RULE_OWNERSHIP.items():
        for kind in kinds:
            if kind in seen.get(rule_id, set()):
                duplicates.append((rule_id, kind))
            else:
                seen.setdefault(rule_id, set()).add(kind)

    if duplicates:
        return False, f"Duplicate ownership found: {duplicates}"

    return True, "No duplicate ownership kinds (baseline)"


# ============================================================================
# Test runner
# ============================================================================

def run_all_tests() -> list[tuple[str, bool, str]]:
    """Run all ownership tests and return results."""
    tests = [
        test_ownership_validates,
        test_ownership_count_matches_registry,
        test_ownership_count_23,
        test_ownership_entries_count,
        test_all_ownership_kinds_valid,
        test_no_empty_ownership,
        test_private_key_rules_have_correct_ownership,
        test_header_rules_have_typed_ownership,
        test_cookie_rules_have_correct_ownership,
        test_url_rules_have_url_ownership,
        test_detection_only_rules,
        test_baseline_missing_ownership_detected,
        test_baseline_unknown_ownership_kind_detected,
        test_baseline_empty_ownership_detected,
        test_baseline_duplicate_ownership_detected,
    ]

    results = []
    for test in tests:
        try:
            passed, msg = test()
            results.append((test.__name__, passed, msg))
        except Exception as e:
            results.append((test.__name__, False, f"Exception: {e}"))

    return results
