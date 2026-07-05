# total_parser_verifier package
"""Total parser verifier for tovarisch external-input parsers.

This package verifies that external-input parsers follow the total parser doctrine:
- No @panic on malformed input
- No unreachable on malformed input
- No unchecked optional unwrap
- No @enumFromInt without bounds validation
- All input produces structured success or structured failure
"""

from .classifications import (
    Classification,
    TOTAL_MODULES,
    BOUNDARY_TOTAL_MODULES,
    STATEFUL_ADAPTER_MODULES,
    DEFERRED_MODULES,
    get_module_classification,
    get_all_registered_modules,
)
from .patterns import (
    FORBIDDEN_PATTERNS,
    MEDIUM_PATTERNS,
    is_forbidden_pattern,
    is_medium_pattern,
)
from .scanner import (
    scan_file,
    scan_modules,
    Finding,
    FindingSeverity,
)
from .self_test import (
    run_all_self_tests,
    SELF_TEST_CASES,
)

__all__ = [
    "Classification",
    "TOTAL_MODULES",
    "BOUNDARY_TOTAL_MODULES", 
    "STATEFUL_ADAPTER_MODULES",
    "DEFERRED_MODULES",
    "get_module_classification",
    "get_all_registered_modules",
    "FORBIDDEN_PATTERNS",
    "MEDIUM_PATTERNS",
    "is_forbidden_pattern",
    "is_medium_pattern",
    "scan_file",
    "scan_modules",
    "Finding",
    "FindingSeverity",
    "run_all_self_tests",
    "SELF_TEST_CASES",
]
