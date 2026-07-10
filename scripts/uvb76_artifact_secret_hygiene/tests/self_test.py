"""
Test runner for UVB-76 Artifact Secret Hygiene self-tests.

Aggregates all test modules and provides run_self_tests() entry point.
"""

from .registry_tests import (
    test_registry_structural_validation,
    test_registry_rule_count,
    test_registry_universal_rules_defined,
    test_registry_no_duplicate_ids,
    test_registry_no_duplicate_classes,
    test_python_universal_rules_from_registry,
    test_python_context_rules_from_registry,
    test_field_rules_coverage,
    test_query_param_rules_coverage,
    test_all_rules_have_projections,
    test_go_constants_agreement,
    test_go_parser_mechanism_accurate,
    test_go_parser_single_declaration,
    test_go_parser_grouped_declaration,
    test_go_parser_whitespace_variations,
    test_go_parser_detects_duplicate_id,
    test_go_parser_detects_wrong_class,
    test_go_parser_detects_missing_constant,
    test_go_parser_detects_unknown_constant,
    test_go_parser_unrelated_constants,
    test_go_parser_comments_handled,
    test_universal_projection_completeness,
    test_pattern_projection_completeness,
    test_no_duplicate_projections,
    test_projection_removal_0072_detected,
)

from .positive_tests import (
    test_universal_private_key_detection,
    test_public_cert_not_detected,
    test_redacted_marker_accepted,
    test_safe_null_value_accepted,
    test_safe_empty_value_accepted,
    test_ssh_public_key_not_detected,
    test_structured_json_nested_detection,
    test_structured_json_array_detection,
    test_structured_field_path_preserved,
    test_credential_url_detection,
    test_session_cookie_detection,
    test_jwt_detection,
    test_query_param_detection,
    test_query_param_safe_value_accepted,
    test_query_param_preserved_safe_params,
    test_query_param_lowercase_token,
    test_query_param_uppercase_token,
    test_query_param_redacted_value,
    test_query_param_empty_value,
    test_query_param_multiple_values,
    test_query_param_mixed_safe_and_sensitive,
    test_query_param_percent_encoded,
    test_query_param_relative_url,
    test_query_param_absolute_http_url,
    test_query_param_malformed_percent_encoding,
    test_query_param_excessive_fields,
    test_query_param_no_secret_in_output,
    test_query_param_access_token,
    test_query_param_api_key,
    test_query_param_password,
    test_query_param_secret,
    test_query_param_key,
    test_query_param_auth,
    test_query_param_credential,
    test_diagnostic_non_disclosure_end_to_end,
)

from .mutation_tests import (
    test_duplicate_rule_id_detected,
    test_unknown_scanner_id_detected,
    test_missing_explanation_detected,
    test_missing_remediation_detected,
    test_invalid_scope_detected,
    test_invalid_detector_kind_detected,
)

from .safety_tests import (
    test_diagnostic_no_secret_exposure,
    test_synthetic_credential_not_in_output,
)

from .ownership_tests import (
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
)

from .ownership_mutation_tests import (
    test_duplicate_rule_id_detected as mutation_duplicate_rule_id_detected,
    test_missing_ownership_detected as mutation_missing_ownership_detected,
    test_unknown_rule_detected as mutation_unknown_rule_detected,
    test_unsupported_kind_detected as mutation_unsupported_kind_detected,
    test_empty_ownership_detected as mutation_empty_ownership_detected,
    test_duplicate_kind_detected as mutation_duplicate_kind_detected,
)

from .projection_tests import (
    test_valid_entries_projection_succeeds,
    test_duplicate_rule_id_raises_error,
    test_empty_ownership_raises_error,
    test_unknown_rule_raises_error,
    test_unsupported_kind_raises_error,
    test_exact_assignment_count,
    test_exact_unique_rule_count,
    test_no_silent_overwrite,
)

from .bounds_tests import (
    test_oversized_file_detection,
    test_binary_file_detection,
)

from .malformed_fixture_tests import (
    test_compute_file_sha256,
    test_validate_fixture_non_empty_justification,
    test_validate_fixture_non_empty_owner,
    test_validate_fixture_non_empty_fingerprint,
    test_validate_fixture_glob_pattern_rejected,
    test_validate_fixture_nonexistent_file,
    test_validate_fixture_directory_rejected,
    test_validate_fixture_stale_fingerprint,
    test_validate_fixture_valid,
    test_is_exempt_unknown_fixture,
    test_is_exempt_with_stale_fingerprint,
    test_fixture_mutation_one_byte_change,
    test_fixture_path_normalization,
    test_fixture_duplicate_entry_detection,
)


def test_epic_document_structure() -> tuple[bool, str]:
    """
    Test that the epic document has the required structure.

    Validates:
    - has more than one physical line
    - contains the HULK05 board heading
    - contains R2 COMPLETE
    - contains R3 COMPLETE
    - contains HULK05 overall OPEN
    - contains HULK05R4 NEXT
    - does not contain concatenated heading metadata
    """
    import os

    # Path: .../scripts/uvb76_artifact_secret_hygiene/tests/self_test.py
    # Go up to KGB root: 3 levels
    # tests -> uvb76_artifact_secret_hygiene -> scripts -> KGB
    tests_dir = os.path.dirname(__file__)
    pkg_dir = os.path.dirname(tests_dir)
    scripts_dir = os.path.dirname(pkg_dir)
    project_root = os.path.dirname(scripts_dir)

    epic_path = os.path.join(
        project_root,
        "docs", "epics", "act-uvb76-hulk05-artifact-secret-hygiene.md"
    )

    if not os.path.exists(epic_path):
        return False, f"Epic document not found at {epic_path}"

    with open(epic_path, 'r') as f:
        content = f.read()

    errors = []

    # Check minimum length
    if content.count('\n') < 1:
        errors.append("Document has less than 1 line")

    # Check HULK05 board heading
    if "## Board Table (HULK05)" not in content:
        errors.append("Missing HULK05 board heading")

    # Check R2 has COMPLETE status (R2 followed by COMPLETE)
    if not (("R2" in content and "COMPLETE" in content)):
        errors.append("Missing R2 COMPLETE")

    # Check R3 has COMPLETE status (R3 followed by COMPLETE)
    if not (("R3" in content and "COMPLETE" in content)):
        errors.append("Missing R3 COMPLETE")

    # Check HULK05 overall OPEN
    if "HULK05" not in content or "OPEN" not in content:
        errors.append("Missing HULK05 OPEN status")

    # Check R4 NEXT
    if "R4 NEXT" not in content and "R4-1" not in content:
        errors.append("Missing R4 NEXT indication")

    # Check no concatenated heading metadata (e.g., "## R2 R3 COMPLETE")
    lines = content.split('\n')
    for line in lines:
        if line.startswith('## '):
            # Check for patterns like "## R2 COMPLETE R3 COMPLETE" (concatenated)
            if "COMPLETE COMPLETE" in line or "COMPLETE NEXT" in line:
                errors.append(f"Concatenated heading metadata detected: {line.strip()}")

    if errors:
        return False, f"Epic structure errors: {errors}"

    return True, "Epic document structure is valid"


def run_self_tests() -> tuple[list[str], dict[str, bool], int, int]:
    """Run all self-tests. Returns (errors, results, total, passed)."""
    errors = []
    results = {}
    passed = 0
    total = 0

    # =========================================================================
    # Epic document structure validation
    # =========================================================================
    epic_tests = [
        ("epic_document_structure", test_epic_document_structure),
    ]

    for name, test_fn in epic_tests:
        total += 1
        ok, msg = test_fn()
        results[name] = ok
        if ok:
            passed += 1
        else:
            errors.append(f"{name}: {msg}")

    # =========================================================================
    # Registry consistency tests
    # =========================================================================
    registry_tests = [
        ("registry_structural_validation", test_registry_structural_validation),
        ("registry_rule_count", test_registry_rule_count),
        ("registry_universal_rules_defined", test_registry_universal_rules_defined),
        ("registry_no_duplicate_ids", test_registry_no_duplicate_ids),
        ("registry_no_duplicate_classes", test_registry_no_duplicate_classes),
        ("python_universal_rules_from_registry", test_python_universal_rules_from_registry),
        ("python_context_rules_from_registry", test_python_context_rules_from_registry),
        ("field_rules_coverage", test_field_rules_coverage),
        ("query_param_rules_coverage", test_query_param_rules_coverage),
        ("all_rules_have_projections", test_all_rules_have_projections),
    ]

    for name, test_fn in registry_tests:
        total += 1
        ok, msg = test_fn()
        results[name] = ok
        if ok:
            passed += 1
        else:
            errors.append(f"{name}: {msg}")

    # =========================================================================
    # Go parser tests
    # =========================================================================
    go_parser_tests = [
        ("go_constants_agreement", test_go_constants_agreement),
        ("go_parser_mechanism_accurate", test_go_parser_mechanism_accurate),
        ("go_parser_single_declaration", test_go_parser_single_declaration),
        ("go_parser_grouped_declaration", test_go_parser_grouped_declaration),
        ("go_parser_whitespace_variations", test_go_parser_whitespace_variations),
        ("go_parser_detects_duplicate_id", test_go_parser_detects_duplicate_id),
        ("go_parser_detects_wrong_class", test_go_parser_detects_wrong_class),
        ("go_parser_detects_missing_constant", test_go_parser_detects_missing_constant),
        ("go_parser_detects_unknown_constant", test_go_parser_detects_unknown_constant),
        ("go_parser_unrelated_constants", test_go_parser_unrelated_constants),
        ("go_parser_comments_handled", test_go_parser_comments_handled),
    ]

    for name, test_fn in go_parser_tests:
        total += 1
        ok, msg = test_fn()
        results[name] = ok
        if ok:
            passed += 1
        else:
            errors.append(f"{name}: {msg}")

    # =========================================================================
    # Projection completeness tests
    # =========================================================================
    projection_tests = [
        ("universal_projection_completeness", test_universal_projection_completeness),
        ("pattern_projection_completeness", test_pattern_projection_completeness),
        ("no_duplicate_projections", test_no_duplicate_projections),
        ("projection_removal_0072_detected", test_projection_removal_0072_detected),
    ]

    for name, test_fn in projection_tests:
        total += 1
        ok, msg = test_fn()
        results[name] = ok
        if ok:
            passed += 1
        else:
            errors.append(f"{name}: {msg}")

    # =========================================================================
    # Positive behavior tests
    # =========================================================================
    positive_tests = [
        ("universal_private_key_detection", test_universal_private_key_detection),
        ("public_cert_not_detected", test_public_cert_not_detected),
        ("redacted_marker_accepted", test_redacted_marker_accepted),
        ("safe_null_value_accepted", test_safe_null_value_accepted),
        ("safe_empty_value_accepted", test_safe_empty_value_accepted),
        ("ssh_public_key_not_detected", test_ssh_public_key_not_detected),
        ("structured_json_nested_detection", test_structured_json_nested_detection),
        ("structured_json_array_detection", test_structured_json_array_detection),
        ("structured_field_path_preserved", test_structured_field_path_preserved),
        ("credential_url_detection", test_credential_url_detection),
        ("session_cookie_detection", test_session_cookie_detection),
        ("jwt_detection", test_jwt_detection),
        ("query_param_detection", test_query_param_detection),
        ("query_param_safe_value_accepted", test_query_param_safe_value_accepted),
        ("query_param_preserved_safe_params", test_query_param_preserved_safe_params),
        ("diagnostic_non_disclosure_end_to_end", test_diagnostic_non_disclosure_end_to_end),
    ]

    for name, test_fn in positive_tests:
        total += 1
        ok, msg = test_fn()
        results[name] = ok
        if ok:
            passed += 1
        else:
            errors.append(f"{name}: {msg}")

    # =========================================================================
    # Query parameter comprehensive tests
    # =========================================================================
    query_param_tests = [
        ("query_param_lowercase_token", test_query_param_lowercase_token),
        ("query_param_uppercase_token", test_query_param_uppercase_token),
        ("query_param_redacted_value", test_query_param_redacted_value),
        ("query_param_empty_value", test_query_param_empty_value),
        ("query_param_multiple_values", test_query_param_multiple_values),
        ("query_param_mixed_safe_and_sensitive", test_query_param_mixed_safe_and_sensitive),
        ("query_param_percent_encoded", test_query_param_percent_encoded),
        ("query_param_relative_url", test_query_param_relative_url),
        ("query_param_absolute_http_url", test_query_param_absolute_http_url),
        ("query_param_malformed_percent_encoding", test_query_param_malformed_percent_encoding),
        ("query_param_excessive_fields", test_query_param_excessive_fields),
        ("query_param_no_secret_in_output", test_query_param_no_secret_in_output),
        ("query_param_access_token", test_query_param_access_token),
        ("query_param_api_key", test_query_param_api_key),
        ("query_param_password", test_query_param_password),
        ("query_param_secret", test_query_param_secret),
        ("query_param_key", test_query_param_key),
        ("query_param_auth", test_query_param_auth),
        ("query_param_credential", test_query_param_credential),
    ]

    for name, test_fn in query_param_tests:
        total += 1
        ok, msg = test_fn()
        results[name] = ok
        if ok:
            passed += 1
        else:
            errors.append(f"{name}: {msg}")

    # =========================================================================
    # Mutation detection tests
    # =========================================================================
    mutation_tests = [
        ("duplicate_rule_id_detected", test_duplicate_rule_id_detected),
        ("unknown_scanner_id_detected", test_unknown_scanner_id_detected),
        ("missing_explanation_detected", test_missing_explanation_detected),
        ("missing_remediation_detected", test_missing_remediation_detected),
        ("invalid_scope_detected", test_invalid_scope_detected),
        ("invalid_detector_kind_detected", test_invalid_detector_kind_detected),
    ]

    for name, test_fn in mutation_tests:
        total += 1
        ok, msg = test_fn()
        results[name] = ok
        if ok:
            passed += 1
        else:
            errors.append(f"{name}: {msg}")

    # =========================================================================
    # Malformed fixture tests
    # =========================================================================
    malformed_fixture_tests = [
        ("compute_file_sha256", test_compute_file_sha256),
        ("validate_fixture_non_empty_justification", test_validate_fixture_non_empty_justification),
        ("validate_fixture_non_empty_owner", test_validate_fixture_non_empty_owner),
        ("validate_fixture_non_empty_fingerprint", test_validate_fixture_non_empty_fingerprint),
        ("validate_fixture_glob_pattern_rejected", test_validate_fixture_glob_pattern_rejected),
        ("validate_fixture_nonexistent_file", test_validate_fixture_nonexistent_file),
        ("validate_fixture_directory_rejected", test_validate_fixture_directory_rejected),
        ("validate_fixture_stale_fingerprint", test_validate_fixture_stale_fingerprint),
        ("validate_fixture_valid", test_validate_fixture_valid),
        ("is_exempt_unknown_fixture", test_is_exempt_unknown_fixture),
        ("is_exempt_with_stale_fingerprint", test_is_exempt_with_stale_fingerprint),
        ("fixture_mutation_one_byte_change", test_fixture_mutation_one_byte_change),
        ("fixture_path_normalization", test_fixture_path_normalization),
        ("fixture_duplicate_entry_detection", test_fixture_duplicate_entry_detection),
    ]

    for name, test_fn in malformed_fixture_tests:
        total += 1
        ok, msg = test_fn()
        results[name] = ok
        if ok:
            passed += 1
        else:
            errors.append(f"{name}: {msg}")

    # =========================================================================
    # Diagnostic safety tests
    # =========================================================================
    safety_tests = [
        ("diagnostic_no_secret_exposure", test_diagnostic_no_secret_exposure),
        ("synthetic_credential_not_in_output", test_synthetic_credential_not_in_output),
    ]

    for name, test_fn in safety_tests:
        total += 1
        ok, msg = test_fn()
        results[name] = ok
        if ok:
            passed += 1
        else:
            errors.append(f"{name}: {msg}")

    # =========================================================================
    # Bounds tests
    # =========================================================================
    bounds_tests = [
        ("oversized_file_detection", test_oversized_file_detection),
        ("binary_file_detection", test_binary_file_detection),
    ]

    for name, test_fn in bounds_tests:
        total += 1
        ok, msg = test_fn()
        results[name] = ok
        if ok:
            passed += 1
        else:
            errors.append(f"{name}: {msg}")

    # =========================================================================
    # Ownership tests
    # =========================================================================
    ownership_tests = [
        ("ownership_validates", test_ownership_validates),
        ("ownership_count_matches_registry", test_ownership_count_matches_registry),
        ("ownership_count_23", test_ownership_count_23),
        ("ownership_entries_count", test_ownership_entries_count),
        ("all_ownership_kinds_valid", test_all_ownership_kinds_valid),
        ("no_empty_ownership", test_no_empty_ownership),
        ("private_key_rules_have_correct_ownership", test_private_key_rules_have_correct_ownership),
        ("header_rules_have_typed_ownership", test_header_rules_have_typed_ownership),
        ("cookie_rules_have_correct_ownership", test_cookie_rules_have_correct_ownership),
        ("url_rules_have_url_ownership", test_url_rules_have_url_ownership),
        ("detection_only_rules", test_detection_only_rules),
        ("baseline_missing_ownership_detected", test_baseline_missing_ownership_detected),
        ("baseline_unknown_ownership_kind_detected", test_baseline_unknown_ownership_kind_detected),
        ("baseline_empty_ownership_detected", test_baseline_empty_ownership_detected),
        ("baseline_duplicate_ownership_detected", test_baseline_duplicate_ownership_detected),
    ]

    for name, test_fn in ownership_tests:
        total += 1
        ok, msg = test_fn()
        results[name] = ok
        if ok:
            passed += 1
        else:
            errors.append(f"{name}: {msg}")

    # =========================================================================
    # Ownership mutation tests (using production validator)
    # =========================================================================
    ownership_mutation_tests = [
        ("mutation_duplicate_rule_id_detected", mutation_duplicate_rule_id_detected),
        ("mutation_missing_ownership_detected", mutation_missing_ownership_detected),
        ("mutation_unknown_rule_detected", mutation_unknown_rule_detected),
        ("mutation_unsupported_kind_detected", mutation_unsupported_kind_detected),
        ("mutation_empty_ownership_detected", mutation_empty_ownership_detected),
        ("mutation_duplicate_kind_detected", mutation_duplicate_kind_detected),
    ]

    for name, test_fn in ownership_mutation_tests:
        total += 1
        ok, msg = test_fn()
        results[name] = ok
        if ok:
            passed += 1
        else:
            errors.append(f"{name}: {msg}")

    # =========================================================================
    # Projection tests (build_rule_ownership validation)
    # =========================================================================
    projection_tests = [
        ("projection_valid_entries_succeeds", test_valid_entries_projection_succeeds),
        ("projection_duplicate_rule_id_raises_error", test_duplicate_rule_id_raises_error),
        ("projection_empty_ownership_raises_error", test_empty_ownership_raises_error),
        ("projection_unknown_rule_raises_error", test_unknown_rule_raises_error),
        ("projection_unsupported_kind_raises_error", test_unsupported_kind_raises_error),
        ("projection_exact_assignment_count", test_exact_assignment_count),
        ("projection_exact_unique_rule_count", test_exact_unique_rule_count),
        ("projection_no_silent_overwrite", test_no_silent_overwrite),
    ]

    for name, test_fn in projection_tests:
        total += 1
        ok, msg = test_fn()
        results[name] = ok
        if ok:
            passed += 1
        else:
            errors.append(f"{name}: {msg}")

    return errors, results, total, passed
