"""
Projection tests for build_rule_ownership().

Tests that the rule ownership building and validation logic correctly:
- Validates entries BEFORE dictionary projection
- Raises OwnershipValidationError for invalid entries
- Does not silently overwrite entries

Required invariant:
- validate_ownership iterates OWNERSHIP_ENTRIES directly
- no invalid entry sequence is collapsed before validation
- build_rule_ownership validates before projection
"""

from ..ownership import (
    build_rule_ownership,
    OwnershipValidationError,
    count_ownership_assignments,
    count_unique_rules,
    OWNERSHIP_ENTRIES,
    OwnershipEntry,
)
from ..registry_loader import get_registry


def test_valid_entries_projection_succeeds() -> tuple[bool, str]:
    """Test that build_rule_ownership returns a dict with valid entries."""
    result = build_rule_ownership(OWNERSHIP_ENTRIES)
    if not isinstance(result, dict):
        return False, f"Expected dict, got {type(result).__name__}"
    return True, "Valid entries projection returned a dict"


def test_duplicate_rule_id_raises_error() -> tuple[bool, str]:
    """Test that duplicate rule IDs raise OwnershipValidationError."""
    # Create entries with duplicate rule ID
    entries = (
        OwnershipEntry("UVB76-SECRET-0001", ("private_key_marker_redactor",)),
        OwnershipEntry("UVB76-SECRET-0001", ("typed_header_redactor",)),  # Duplicate
    )
    try:
        build_rule_ownership(entries)
        return False, "Expected OwnershipValidationError for duplicate rule ID"
    except OwnershipValidationError as e:
        error_str = str(e)
        if "duplicate" in error_str.lower():
            return True, "Duplicate rule ID raised OwnershipValidationError"
        return False, f"Wrong error message: {e}"


def test_empty_ownership_raises_error() -> tuple[bool, str]:
    """Test that empty ownership raises OwnershipValidationError."""
    # Create an entry with empty ownership kinds
    entries = (
        OwnershipEntry("UVB76-SECRET-0001", ()),  # Empty ownership
    )
    try:
        build_rule_ownership(entries)
        return False, "Expected OwnershipValidationError for empty ownership"
    except OwnershipValidationError as e:
        error_str = str(e)
        if "empty" in error_str.lower():
            return True, "Empty ownership raised OwnershipValidationError"
        return False, f"Wrong error message: {e}"


def test_unknown_rule_raises_error() -> tuple[bool, str]:
    """Test that unknown rule IDs raise OwnershipValidationError."""
    # Create an entry with unknown rule ID
    entries = (
        OwnershipEntry("UVB76-SECRET-UNKNOWN", ("private_key_marker_redactor",)),
    )
    try:
        build_rule_ownership(entries)
        return False, "Expected OwnershipValidationError for unknown rule ID"
    except OwnershipValidationError as e:
        error_str = str(e)
        if "unknown" in error_str.lower():
            return True, "Unknown rule ID raised OwnershipValidationError"
        return False, f"Wrong error message: {e}"


def test_unsupported_kind_raises_error() -> tuple[bool, str]:
    """Test that unsupported ownership kinds raise OwnershipValidationError."""
    # Create an entry with unsupported ownership kind
    entries = (
        OwnershipEntry("UVB76-SECRET-0001", ("unknown_redactor",)),
    )
    try:
        build_rule_ownership(entries)
        return False, "Expected OwnershipValidationError for unsupported kind"
    except OwnershipValidationError as e:
        error_str = str(e)
        if "unknown" in error_str.lower():
            return True, "Unsupported kind raised OwnershipValidationError"
        return False, f"Wrong error message: {e}"


def test_exact_assignment_count() -> tuple[bool, str]:
    """Test that count_ownership_assignments() returns 26."""
    count = count_ownership_assignments()
    if count == 26:
        return True, f"count_ownership_assignments() == {count}"
    return False, f"Expected 26, got {count}"


def test_exact_unique_rule_count() -> tuple[bool, str]:
    """Test that count_unique_rules() returns 23."""
    count = count_unique_rules()
    if count == 23:
        return True, f"count_unique_rules() == {count}"
    return False, f"Expected 23, got {count}"


def test_no_silent_overwrite() -> tuple[bool, str]:
    """Test that build_rule_ownership does not silently overwrite entries."""
    # If duplicate rule IDs are present, the first entry should NOT be silently
    # overwritten by subsequent entries with the same rule ID
    entries = (
        OwnershipEntry("UVB76-SECRET-0001", ("private_key_marker_redactor",)),
        OwnershipEntry("UVB76-SECRET-0001", ("url_redactor",)),  # Duplicate with different kinds
    )
    try:
        result = build_rule_ownership(entries)
        # If we get here, entries were silently overwritten (bad)
        # Check if it kept the first or second value
        kinds = result.get("UVB76-SECRET-0001")
        if kinds == ("url_redactor",):
            return False, "Silent overwrite detected: second entry overwrote first"
        elif kinds == ("private_key_marker_redactor",):
            return False, "Silent overwrite detected: first entry kept but should have raised error"
        return False, f"Unexpected result: {result}"
    except OwnershipValidationError:
        # Good - validation error raised, no silent overwrite
        return True, "No silent overwrite: validation error raised"


def run_all_tests() -> list[tuple[str, bool, str]]:
    """Run all projection tests and return results."""
    tests = [
        test_valid_entries_projection_succeeds,
        test_duplicate_rule_id_raises_error,
        test_empty_ownership_raises_error,
        test_unknown_rule_raises_error,
        test_unsupported_kind_raises_error,
        test_exact_assignment_count,
        test_exact_unique_rule_count,
        test_no_silent_overwrite,
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
