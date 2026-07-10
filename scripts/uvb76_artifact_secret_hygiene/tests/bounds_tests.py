"""
Bounds tests for UVB-76 Artifact Secret Hygiene.

Tests for oversized and binary file detection.
"""

import os
import tempfile

from ..scanner import scan_file_for_secrets, MAX_FILE_SIZE


def test_oversized_file_detection() -> tuple[bool, str]:
    """Test that oversized files are flagged."""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.json', delete=False) as f:
        f.write("x" * (MAX_FILE_SIZE + 1))
        temp_path = f.name

    try:
        findings = scan_file_for_secrets(temp_path)
        found_bound_error = any(f.rule_id == "UVB76-SIZE-0001" for f in findings)

        if found_bound_error:
            return True, "Oversized file detection works"
        return False, "Oversized file NOT flagged"
    finally:
        os.unlink(temp_path)


def test_binary_file_detection() -> tuple[bool, str]:
    """Test that binary files in artifact surfaces are flagged."""
    with tempfile.NamedTemporaryFile(mode='wb', suffix='.key', delete=False) as f:
        f.write(b'\x00\x01\x02' + b'x' * 100)
        temp_path = f.name

    try:
        findings = scan_file_for_secrets(temp_path, artifact_surface=True)
        found_binary_error = any(f.rule_id == "UVB76-BINARY-0001" for f in findings)

        if found_binary_error:
            return True, "Binary file detection works"
        return False, "Binary file NOT flagged"
    finally:
        os.unlink(temp_path)
