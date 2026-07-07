"""
Runner orchestration for UVB-76 HULK02 Capture State Contract verification.

This is the thin CLI entrypoint that combines inventory, status, skip,
JSON normalization, and line-limit checks. Provides run() interface for CLI.
"""

import os
import sys

from .check_makefile import check_makefile_has_hulk_gate
from .constants import (
    CONTRACT_FILES,
    MAX_LINES,
)
from .inventory import (
    get_contract_file_count,
    get_helper_file_count,
    validate_inventory,
)
from .line_limits import (
    count_lines,
    get_max_lines,
    validate_scripts_line_limits,
    validate_uvb76_line_limits,
)
from .reporting import (
    print_errors,
    print_header,
    print_pass,
    print_self_test_errors,
    print_self_test_summary,
    print_summary,
)
from .self_tests import run_self_tests
from .skip_allowlist import validate_skip_allowlist
from .status_contract import (
    get_canonical_status_count,
    get_tcp_absence_reason_count,
    validate_status_contracts,
)


# Module-level paths (set by run())
SCRIPT_DIR = None
REPO_ROOT = None
UVB76_DIR = None


def run_verifier() -> list[str]:
    """
    Run the HULK02 capture state contract verifier.

    Returns:
        List of error messages (empty if verification passes).
    """
    all_errors = []
    print_header()

    # A+B: Inventory validation
    inv_errors, _ = validate_inventory(UVB76_DIR, verbose=True)
    all_errors.extend(inv_errors)

    # C: Skip allowlist (includes core service file skip check)
    skip_errors = validate_skip_allowlist(UVB76_DIR, verbose=True)
    all_errors.extend(skip_errors)

    # D+E+F+G: Status contracts (includes fake backend check)
    status_errors = validate_status_contracts(UVB76_DIR, verbose=True)
    all_errors.extend(status_errors)

    # G: UVB-76 line limits
    line_errors = validate_uvb76_line_limits(UVB76_DIR, verbose=True)
    all_errors.extend(line_errors)

    # H: Makefile gate check
    print("\nI. Checking Makefile hulk-uvb76-capture-gate target...")
    makefile_errors = check_makefile_has_hulk_gate(REPO_ROOT)
    if makefile_errors:
        for e in makefile_errors:
            print(f"    {e}")
        all_errors.extend(makefile_errors)
    else:
        print(f"    OK: Makefile contains hulk-uvb76-capture-gate with go test -race")

    print_summary(
        get_contract_file_count(),
        get_canonical_status_count(),
        get_tcp_absence_reason_count(),
        len(all_errors)
    )

    return all_errors


def run(argv: list[str] | None = None) -> int:
    """
    Main entry point for the verifier.

    Args:
        argv: Command-line arguments (defaults to sys.argv if None).

    Returns:
        Exit code: 0 for success, 1 for failure.
    """
    global SCRIPT_DIR, REPO_ROOT, UVB76_DIR

    # Set up paths
    if argv is None:
        argv = sys.argv

    # Use the main script path (sys.argv[0]), not __file__ which points to this module
    main_script = os.path.abspath(argv[0] if argv else sys.argv[0])
    SCRIPT_DIR = os.path.dirname(main_script)
    REPO_ROOT = os.path.dirname(SCRIPT_DIR)
    UVB76_DIR = os.path.join(REPO_ROOT, "uvb76")

    if "--self-test" in argv:
        # Import modules here to avoid circular imports
        from . import inventory as validate_inventory_module
        from . import skip_allowlist as validate_skip_allowlist_module

        errors, results, test_count, pass_count = run_self_tests(
            count_lines,
            MAX_LINES,
            validate_inventory_module,
            validate_skip_allowlist_module,
            verbose=True
        )
        print_self_test_summary(pass_count, test_count, results)

        if errors:
            print_self_test_errors(errors)
            return 1
        else:
            print("\nAll self-tests passed!")
            return 0

    errors = run_verifier()

    if errors:
        print_errors(errors)
        return 1
    else:
        print_pass()
        return 0
