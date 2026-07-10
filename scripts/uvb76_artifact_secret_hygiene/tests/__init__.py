"""
Test modules for UVB-76 Artifact Secret Hygiene.

Split by responsibility for LLM-friendliness:
- registry_tests: Registry consistency tests
- positive_tests: Positive behavior tests
- mutation_tests: Mutation detection tests
- safety_tests: Diagnostic safety tests
- bounds_tests: Bounds tests
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
    test_go_constants_agreement,
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
    test_credential_url_detection,
    test_session_cookie_detection,
    test_jwt_detection,
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

from .bounds_tests import (
    test_oversized_file_detection,
    test_binary_file_detection,
)

__all__ = [
    # Registry tests
    "test_registry_structural_validation",
    "test_registry_rule_count",
    "test_registry_universal_rules_defined",
    "test_registry_no_duplicate_ids",
    "test_registry_no_duplicate_classes",
    "test_python_universal_rules_from_registry",
    "test_python_context_rules_from_registry",
    "test_field_rules_coverage",
    "test_go_constants_agreement",
    # Positive tests
    "test_universal_private_key_detection",
    "test_public_cert_not_detected",
    "test_redacted_marker_accepted",
    "test_safe_null_value_accepted",
    "test_safe_empty_value_accepted",
    "test_ssh_public_key_not_detected",
    "test_structured_json_nested_detection",
    "test_structured_json_array_detection",
    "test_credential_url_detection",
    "test_session_cookie_detection",
    "test_jwt_detection",
    # Mutation tests
    "test_duplicate_rule_id_detected",
    "test_unknown_scanner_id_detected",
    "test_missing_explanation_detected",
    "test_missing_remediation_detected",
    "test_invalid_scope_detected",
    "test_invalid_detector_kind_detected",
    # Safety tests
    "test_diagnostic_no_secret_exposure",
    "test_synthetic_credential_not_in_output",
    # Bounds tests
    "test_oversized_file_detection",
    "test_binary_file_detection",
]
