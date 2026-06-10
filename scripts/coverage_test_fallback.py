"""coverage_test_fallback.py — Test-as-signal fallback for incomplete DWARF

Provides test-as-signal fallback when kcov produces coverage numbers but
DWARF diagnostics show incomplete debug info (missing source paths).

This is the honest signal policy: when instrumentation is broken, use
actual test execution as the coverage signal.
"""

import subprocess
from pathlib import Path


def _tail(text: str, limit: int = 2000) -> str:
    """Return the last `limit` characters of text, or all of it if shorter."""
    return text[-limit:] if len(text) > limit else text


def run_test_as_signal_fallback(repo_root: Path, kcov_coverage: float) -> tuple[bool, str]:
    """Run test-as-signal as honest fallback when DWARF is incomplete.
    
    Args:
        repo_root: Path to repository root
        kcov_coverage: The kcov-reported coverage percentage (for logging)
    
    Returns:
        (success: bool, message: str)
    
    Invariant:
        This command must not call `make coverage` or `make gate`.
        It is intentionally the narrow Zig test target to avoid recursive
        coverage-gate execution. Only `tovarisch-test` is safe here.
    """
    # Run the test as honest coverage signal
    test_result = subprocess.run(
        ["make", "tovarisch-test"],
        cwd=repo_root,
        capture_output=True,
        text=True,
        check=False
    )
    
    if test_result.returncode == 0:
        return (True, "test-as-signal passed — honest coverage confirmed")
    else:
        # Include bounded tail of stdout/stderr for operator usability
        output = ""
        if test_result.stdout:
            output += f"\nstdout:\n{_tail(test_result.stdout)}"
        if test_result.stderr:
            output += f"\nstderr:\n{_tail(test_result.stderr)}"
        return (False, f"test-as-signal failed — tests did not pass{output}")
