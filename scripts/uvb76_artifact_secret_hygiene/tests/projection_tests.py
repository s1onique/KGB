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
import json
import os
import tempfile
from dataclasses import replace

from ..registry_loader import get_registry
from ..inventory import (
    CANONICAL_SURFACE_FIELDS,
    get_canonical_catalog,
    projection_drift_errors,
    validate_canonical_catalog,
)


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


def test_surface_catalog_full_field_projection() -> tuple[bool, str]:
    catalog = get_canonical_catalog()
    errors = validate_canonical_catalog(catalog)
    errors.extend(projection_drift_errors(catalog))
    if errors:
        return False, f"canonical projection errors: {errors}"
    icmp = next(
        (surface for surface in catalog if surface.id == "icmp-ping-soak-artifacts"),
        None,
    )
    if icmp is None:
        return False, "ICMP surface missing"
    if icmp.enforcement_state != "migrated":
        return False, f"enforcement_state dropped: {icmp.enforcement_state!r}"
    if icmp.ownership_scope != "symbol":
        return False, f"ownership_scope dropped: {icmp.ownership_scope!r}"
    if tuple(icmp.canonical_dict()) != CANONICAL_SURFACE_FIELDS:
        return False, "projected fields differ from canonical field set"
    return True, "all canonical fields and ownership metadata projected"


def test_surface_catalog_field_mutation_detected() -> tuple[bool, str]:
    catalog = get_canonical_catalog()
    mutated = [
        replace(surface, ownership_scope="dedicated_file")
        if surface.id == "icmp-ping-soak-artifacts"
        else surface
        for surface in catalog
    ]
    errors = projection_drift_errors(mutated)
    if any("icmp-ping-soak-artifacts.ownership_scope" in error for error in errors):
        return True, "ownership_scope projection mutation detected"
    return False, f"ownership_scope mutation escaped parity check: {errors}"


def test_surface_catalog_unknown_field_detected() -> tuple[bool, str]:
    catalog_path = os.path.join(
        os.path.dirname(os.path.dirname(__file__)), "surfaces.json"
    )
    with open(catalog_path, "r", encoding="utf-8") as catalog_file:
        raw = json.load(catalog_file)
    raw["surfaces"][0]["shadow_field"] = "unprojected"
    with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as fixture:
        json.dump(raw, fixture)
        fixture_path = fixture.name
    try:
        catalog = get_canonical_catalog(fixture_path)
        errors = projection_drift_errors(catalog, fixture_path)
    finally:
        os.unlink(fixture_path)
    if any("unknown canonical fields" in error for error in errors):
        return True, "unknown canonical field rejected"
    return False, f"unknown field escaped projection check: {errors}"


def test_inventory_has_single_editable_catalog() -> tuple[bool, str]:
    package_dir = os.path.dirname(os.path.dirname(__file__))
    inventory_path = os.path.join(package_dir, "inventory.py")
    with open(inventory_path, "r", encoding="utf-8") as inventory_file:
        inventory_source = inventory_file.read()
    projection = "ARTIFACT_INVENTORY: list[SurfaceRecord] = get_canonical_catalog()"
    if inventory_source.count(projection) != 1:
        return False, "inventory does not contain exactly one canonical projection"
    forbidden = ("ARTIFACT_INVENTORY = [", "ArtifactSurface(")
    present = [token for token in forbidden if token in inventory_source]
    if present:
        return False, f"hand-authored inventory definitions remain: {present}"
    return True, "surfaces.json is the sole editable catalog source"


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
        test_surface_catalog_full_field_projection,
        test_surface_catalog_field_mutation_detected,
        test_surface_catalog_unknown_field_detected,
        test_inventory_has_single_editable_catalog,
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
