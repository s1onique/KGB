"""
Ownership mutation tests for UVB-76 Artifact Secret Hygiene.

Tests that validate_ownership() correctly rejects various mutated ownership entries.
Each mutation test:
1. Constructs an invalid entry sequence
2. Calls validate_ownership(mutated, registry) directly
3. Asserts the result is non-empty with expected error category

Required invariant:
- OWNERSHIP_ENTRIES is validated as an ordered sequence first.
- Every mutation test actually changes the input.
- All mutations call the production validator.
"""

from ..ownership import (
    OwnershipEntry,
    OWNERSHIP_ENTRIES,
    validate_ownership,
)
from ..registry_loader import get_registry


# ============================================================================
# Ownership mutation tests
# ============================================================================


def test_duplicate_rule_id_detected() -> tuple[bool, str]:
    """
    Mutation test: duplicate rule ID should be detected.

    Add a duplicate rule ID (existing rule) to entries and assert
    validate_ownership() rejects it before dictionary projection.
    """
    registry = get_registry()

    # Create mutated entries with a duplicate rule ID
    # The duplicate uses an EXISTING rule ID - this is the mutation
    mutated = OWNERSHIP_ENTRIES + (
        OwnershipEntry(
            "UVB76-SECRET-0001",
            ("url_redactor",),
        ),
    )

    errors = validate_ownership(mutated, registry)

    if not errors:
        return False, "Expected validation error for duplicate rule ID, but got none"

    # Check that the error mentions duplicate rule ID
    has_duplicate_error = any(
        "duplicate" in e.lower() and "UVB76-SECRET-0001" in e for e in errors
    )
    if not has_duplicate_error:
        return False, f"Expected duplicate rule ID error, got: {errors}"

    return True, "Duplicate rule ID correctly detected"


def test_missing_ownership_detected() -> tuple[bool, str]:
    """
    Mutation test: missing ownership entry should be detected.

    Remove one entry (UVB76-SECRET-0072) and assert validate_ownership()
    detects missing ownership.
    """
    registry = get_registry()

    # Create mutated entries with one entry removed
    mutated = tuple(
        entry for entry in OWNERSHIP_ENTRIES
        if entry.rule_id != "UVB76-SECRET-0072"
    )

    errors = validate_ownership(mutated, registry)

    if not errors:
        return False, "Expected validation error for missing ownership, but got none"

    # Check that the error mentions missing ownership
    has_missing_error = any("missing" in e.lower() for e in errors)
    if not has_missing_error:
        return False, f"Expected missing ownership error, got: {errors}"

    return True, "Missing ownership correctly detected"


def test_unknown_rule_detected() -> tuple[bool, str]:
    """
    Mutation test: unknown rule ID should be detected.

    Append an unknown rule ID (not in registry) and assert
    validate_ownership() rejects it.
    """
    registry = get_registry()

    # Create mutated entries with an unknown rule ID
    mutated = OWNERSHIP_ENTRIES + (
        OwnershipEntry(
            "UVB76-SECRET-9999",
            ("repository_detection_only",),
        ),
    )

    errors = validate_ownership(mutated, registry)

    if not errors:
        return False, "Expected validation error for unknown rule ID, but got none"

    # Check that the error mentions unknown rule
    has_unknown_error = any(
        "unknown" in e.lower() and "UVB76-SECRET-9999" in e for e in errors
    )
    if not has_unknown_error:
        return False, f"Expected unknown rule ID error, got: {errors}"

    return True, "Unknown rule ID correctly detected"


def test_unsupported_kind_detected() -> tuple[bool, str]:
    """
    Mutation test: unsupported ownership kind should be detected.

    Replace one entry's kinds with an unsupported kind and assert rejection.
    """
    registry = get_registry()

    # Create mutated entries with unsupported kind
    mutated = tuple(
        OwnershipEntry(entry.rule_id, ("unknown_redactor",))
        if entry.rule_id == "UVB76-SECRET-0001"
        else entry
        for entry in OWNERSHIP_ENTRIES
    )

    errors = validate_ownership(mutated, registry)

    if not errors:
        return False, "Expected validation error for unknown kind, but got none"

    # Check that the error mentions unknown kind
    has_unknown_kind_error = any(
        "unknown" in e.lower() and "unknown_redactor" in e for e in errors
    )
    if not has_unknown_kind_error:
        return False, f"Expected unknown kind error, got: {errors}"

    return True, "Unsupported ownership kind correctly detected"


def test_empty_ownership_detected() -> tuple[bool, str]:
    """
    Mutation test: empty ownership should be detected.

    Replace one entry's kinds with empty tuple and assert rejection.
    """
    registry = get_registry()

    # Create mutated entries with empty ownership
    mutated = tuple(
        OwnershipEntry(entry.rule_id, ())
        if entry.rule_id == "UVB76-SECRET-0001"
        else entry
        for entry in OWNERSHIP_ENTRIES
    )

    errors = validate_ownership(mutated, registry)

    if not errors:
        return False, "Expected validation error for empty ownership, but got none"

    # Check that the error mentions empty ownership
    has_empty_error = any(
        "empty" in e.lower() and "UVB76-SECRET-0001" in e for e in errors
    )
    if not has_empty_error:
        return False, f"Expected empty ownership error, got: {errors}"

    return True, "Empty ownership correctly detected"


def test_duplicate_kind_detected() -> tuple[bool, str]:
    """
    Mutation test: duplicate ownership kind within same entry should be detected.

    Use ("url_redactor", "url_redactor") for one entry and assert rejection.
    """
    registry = get_registry()

    # Create mutated entries with duplicate kinds in one entry
    mutated = tuple(
        OwnershipEntry(entry.rule_id, ("url_redactor", "url_redactor"))
        if entry.rule_id == "UVB76-SECRET-0070"
        else entry
        for entry in OWNERSHIP_ENTRIES
    )

    errors = validate_ownership(mutated, registry)

    if not errors:
        return False, "Expected validation error for duplicate kind, but got none"

    # Check that the error mentions duplicate kind
    has_duplicate_kind_error = any(
        "duplicate" in e.lower() and "url_redactor" in e for e in errors
    )
    if not has_duplicate_kind_error:
        return False, f"Expected duplicate kind error, got: {errors}"

    return True, "Duplicate ownership kind correctly detected"


# ============================================================================
# Test runner
# ============================================================================


def run_all_tests() -> list[tuple[str, bool, str]]:
    """Run all ownership mutation tests and return results."""
    tests = [
        test_duplicate_rule_id_detected,
        test_missing_ownership_detected,
        test_unknown_rule_detected,
        test_unsupported_kind_detected,
        test_empty_ownership_detected,
        test_duplicate_kind_detected,
    ]

    results = []
    for test in tests:
        try:
            passed, msg = test()
            results.append((test.__name__, passed, msg))
        except Exception as e:
            results.append((test.__name__, False, f"Exception: {e}"))

    return results


if __name__ == "__main__":
    results = run_all_tests()
    for name, passed, msg in results:
        status = "PASS" if passed else "FAIL"
        print(f"[{status}] {name}: {msg}")

    failed = sum(1 for _, passed, _ in results if not passed)
    print(f"\n{len(results) - failed}/{len(results)} tests passed")
