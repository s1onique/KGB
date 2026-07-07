"""
Line limit enforcement for UVB-76 HULK02 Capture State verification.

Validates that source files do not exceed the 450-line LLM-friendliness limit.
Must include the newly split verifier files in its checks.
"""

import os

from .constants import (
    CONTRACT_FILES,
    MAX_LINES,
)


def count_lines(file_path: str) -> int:
    """
    Count lines in a file.

    Args:
        file_path: Absolute path to the file.

    Returns:
        Number of lines, or 0 if file cannot be read.
    """
    try:
        with open(file_path, 'r') as f:
            return len(f.readlines())
    except:
        return 0


def check_file_line_limit(relative_path: str, uvb76_dir: str) -> list[str]:
    """
    Check that a file does not exceed the LLM-friendliness line limit.

    Args:
        relative_path: Path relative to uvb76_dir.
        uvb76_dir: Absolute path to the uvb76 package directory.

    Returns:
        List of error messages (empty if no errors).
    """
    full_path = os.path.join(uvb76_dir, relative_path)
    errors = []
    if not os.path.isfile(full_path):
        return errors  # Will be caught by existence check

    line_count = count_lines(full_path)
    if line_count > MAX_LINES:
        errors.append(
            f"ERROR: {relative_path} exceeds {MAX_LINES}-line LLM-friendliness limit: {line_count}"
        )
    return errors


def check_script_line_limit(absolute_path: str) -> list[str]:
    """
    Check that a script file does not exceed the LLM-friendliness line limit.

    Args:
        absolute_path: Absolute path to the script file.

    Returns:
        List of error messages (empty if no errors).
    """
    errors = []
    if not os.path.isfile(absolute_path):
        return errors

    line_count = count_lines(absolute_path)
    if line_count > MAX_LINES:
        errors.append(
            f"ERROR: {absolute_path} exceeds {MAX_LINES}-line LLM-friendliness limit: {line_count}"
        )
    return errors


def validate_uvb76_line_limits(uvb76_dir: str, verbose: bool = True) -> list[str]:
    """
    Validate line limits for all UVB-76 contract test files.

    Args:
        uvb76_dir: Absolute path to the uvb76 package directory.
        verbose: If True, print progress messages.

    Returns:
        List of error messages.
    """
    all_errors = []

    if verbose:
        print("G. Checking line limits (LLM-friendliness)...")
    for relative_path, description in CONTRACT_FILES:
        if verbose:
            print(f"  Checking line limit for: {relative_path}")
        errors = check_file_line_limit(relative_path, uvb76_dir)
        if errors:
            if verbose:
                for e in errors:
                    print(f"    {e}")
            all_errors.extend(errors)
        else:
            line_count = count_lines(os.path.join(uvb76_dir, relative_path))
            if verbose:
                print(f"    OK: {line_count} lines (under {MAX_LINES})")

    return all_errors


def validate_scripts_line_limits(
    script_dir: str,
    verbose: bool = True
) -> tuple[list[str], dict[str, int]]:
    """
    Validate line limits for all verifier script files in the package.

    Args:
        script_dir: Absolute path to the scripts directory.
        verbose: If True, print progress messages.

    Returns:
        Tuple of (error_messages, line_counts_dict).
    """
    import glob
    all_errors = []
    line_counts = {}

    package_dir = os.path.join(script_dir, "uvb76_capture_state_contracts")
    if not os.path.isdir(package_dir):
        return all_errors, line_counts

    if verbose:
        print("H. Checking verifier script line limits...")
    for py_file in sorted(glob.glob(os.path.join(package_dir, "*.py"))):
        rel_path = os.path.relpath(py_file, script_dir)
        line_count = count_lines(py_file)
        line_counts[rel_path] = line_count
        if verbose:
            print(f"  {rel_path}: {line_count} lines")
        if line_count > MAX_LINES:
            error = f"ERROR: {rel_path} exceeds {MAX_LINES}-line limit: {line_count}"
            if verbose:
                print(f"    {error}")
            all_errors.append(error)

    return all_errors, line_counts


def get_max_lines() -> int:
    """Return the maximum allowed line count."""
    return MAX_LINES
