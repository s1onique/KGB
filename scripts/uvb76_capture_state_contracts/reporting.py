"""
Reporting utilities for UVB-76 HULK02 Capture State Contract verification.
"""


def print_header() -> None:
    """Print the verification header."""
    print("=== UVB-76 HULK02 Capture State Machine Contract Verifier ===\n")


def print_summary(
    contract_file_count: int,
    canonical_status_count: int,
    tcp_absence_reason_count: int,
    error_count: int
) -> None:
    """Print the verification summary."""
    print("\n" + "=" * 60)
    print("SUMMARY:")
    print(f"  Contract test files: {contract_file_count}")
    print(f"  Canonical statuses: {canonical_status_count}")
    print(f"  TCP absence reasons: {tcp_absence_reason_count}")
    print(f"  Errors: {error_count}")


def print_errors(errors: list[str]) -> None:
    """Print verification errors."""
    print("\nVERIFICATION FAILED:")
    for e in errors:
        print(f"  {e}")


def print_pass() -> None:
    """Print verification passed message."""
    print("\nVERIFICATION PASSED")


def print_self_test_summary(pass_count: int, test_count: int, results: dict[str, bool]) -> None:
    """Print self-test results summary."""
    print("\n" + "=" * 60)
    print(f"SELF-TEST SUMMARY: {pass_count}/{test_count} passed")
    for name, passed in results.items():
        status = "PASS" if passed else "FAIL"
        print(f"  {name}: {status}")


def print_self_test_errors(errors: list[str]) -> None:
    """Print self-test errors."""
    print("\nSELF-TEST ERRORS:")
    for e in errors:
        print(f"  - {e}")
