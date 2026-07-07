"""
Constants for UVB-76 HULK02 Capture State Machine Contract verification.

Canonical data extracted from ACT-UVB76-HULK02 specification.
Do not introduce dynamic construction; keep names boring and explicit.
"""

import re

# HULK02 full inventory - all required contract test files (split files)
CONTRACT_FILES = [
    # State package - capture status matrix
    ("state/capture_status_matrix_contract_test.go", "Capture status matrix contract tests"),
    # State package - state machine decision
    ("state/capture_state_machine_decision_contract_test.go", "Capture state machine decision contract tests"),
    # State package - state machine invariant
    ("state/capture_state_invariant_contract_test.go", "Capture state machine invariant contract tests"),
    # State package - spike capture projection canonical
    ("state/spike_capture_projection_canonical_contract_test.go", "Spike capture projection canonical contract tests"),
    # State package - spike capture projection matrix
    ("state/spike_capture_projection_matrix_contract_test.go", "Spike capture projection matrix contract tests"),
    # State package - spike capture projection JSON
    ("state/spike_capture_projection_json_contract_test.go", "Spike capture projection JSON contract tests"),
    # Server package - capture status canonical
    ("server/capture_status_canonical_test.go", "Canonical capture status API contract tests"),
    # Server package - capture status constraints
    ("server/capture_status_constraints_test.go", "Capture status field constraint API contract tests"),
    # Diag package - capture service success
    ("diag/capture_service_success_contract_test.go", "Capture service success contract tests"),
    # Diag package - capture service error
    ("diag/capture_service_error_contract_test.go", "Capture service error contract tests"),
    # Diag package - capture service TCP absence
    ("diag/capture_service_tcp_absence_contract_test.go", "Capture service TCP absence contract tests"),
    # Diag package - capture service JSON
    ("diag/capture_service_json_contract_test.go", "Capture service JSON contract tests"),
]

# Helper files (optional - no func Test required)
HELPER_FILES = [
    ("state/capture_contract_helpers_test.go", "Shared test helpers for capture state contracts"),
    ("diag/capture_service_contract_helpers_test.go", "Shared test helpers for capture service contracts"),
]

# Canonical capture statuses (from ACT-UVB76-HULK02 specification)
CANONICAL_STATUSES = [
    "captured",
    "skipped_cooldown",
    "failed",
    "disabled",
    "not_configured",
    "not_attempted",
    "in_progress",
    "missing",
]

# TCP absence reason allowlist (from ACT-UVB76-HULK02 specification) - canonical 8
TCP_ABSENCE_REASONS = [
    "no_matching_socket",
    "socket_closed_before_capture",
    "command_failed",
    "not_configured",
    "permission_denied",
    "target_not_tcp",
    "target_mapping_missing",
    "unsupported_platform",
]

# Allowlisted skip pattern - ACT comments that permit skips
# Matches: comment on same line OR comment on previous line(s) within 100 chars of t.Skip
ALLOWLIST_SKIP_PATTERN = re.compile(
    r'//\s*ACT-UVB76-HULK02-ALLOW-SKIP:'
)

# LLM-friendliness line limit
MAX_LINES = 450

# Core HULK02 service contract files - these MUST NOT contain t.Skip
# even with ACT-UVB76-HULK02-ALLOW-SKIP comments
CORE_SERVICE_CONTRACT_FILES = [
    "diag/capture_service_success_contract_test.go",
    "diag/capture_service_error_contract_test.go",
    "diag/capture_service_tcp_absence_contract_test.go",
    "diag/capture_service_json_contract_test.go",
]
