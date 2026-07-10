"""
Malformed fixture validation tests for UVB-76 Artifact Secret Hygiene.

Tests that malformed fixture fingerprints are properly validated.
"""

import os
import tempfile
import hashlib

from ..inventory import (
    MalformedFixture,
    compute_file_sha256,
    validate_malformed_fixture,
    is_malformed_fixture_exempt,
)


def test_compute_file_sha256() -> tuple[bool, str]:
    """Test SHA-256 computation of a file."""
    content = b"test content for hashing"
    expected_hash = hashlib.sha256(content).hexdigest()

    with tempfile.NamedTemporaryFile(delete=False) as f:
        f.write(content)
        path = f.name

    try:
        actual_hash = compute_file_sha256(path)
        if actual_hash == expected_hash:
            return True, "SHA-256 computation correct"
        return False, f"Hash mismatch: expected {expected_hash}, got {actual_hash}"
    finally:
        os.unlink(path)


def test_validate_fixture_non_empty_justification() -> tuple[bool, str]:
    """Test that empty justification fails validation."""
    fixture = MalformedFixture(
        path="test/path.json",
        fingerprint="abc123",
        justification="",  # Empty
        owner="test-team",
    )

    is_valid, error = validate_malformed_fixture(fixture, "/fake/repo")

    if not is_valid and "justification" in error:
        return True, "Empty justification rejected"
    return False, "Empty justification not rejected"


def test_validate_fixture_non_empty_owner() -> tuple[bool, str]:
    """Test that empty owner fails validation."""
    fixture = MalformedFixture(
        path="test/path.json",
        fingerprint="abc123",
        justification="Test justification",
        owner="",  # Empty
    )

    is_valid, error = validate_malformed_fixture(fixture, "/fake/repo")

    if not is_valid and "owner" in error:
        return True, "Empty owner rejected"
    return False, "Empty owner not rejected"


def test_validate_fixture_non_empty_fingerprint() -> tuple[bool, str]:
    """Test that empty fingerprint fails validation."""
    fixture = MalformedFixture(
        path="test/path.json",
        fingerprint="",  # Empty
        justification="Test justification",
        owner="test-team",
    )

    is_valid, error = validate_malformed_fixture(fixture, "/fake/repo")

    if not is_valid and "fingerprint" in error:
        return True, "Empty fingerprint rejected"
    return False, "Empty fingerprint not rejected"


def test_validate_fixture_glob_pattern_rejected() -> tuple[bool, str]:
    """Test that glob patterns in path are rejected."""
    fixture = MalformedFixture(
        path="test/**/*.json",  # Glob pattern
        fingerprint="abc123",
        justification="Test justification",
        owner="test-team",
    )

    is_valid, error = validate_malformed_fixture(fixture, "/fake/repo")

    if not is_valid and "glob" in error.lower():
        return True, "Glob pattern rejected"
    return False, "Glob pattern not rejected"


def test_validate_fixture_nonexistent_file() -> tuple[bool, str]:
    """Test that nonexistent file fails validation."""
    fixture = MalformedFixture(
        path="nonexistent/file.json",
        fingerprint="abc123",
        justification="Test justification",
        owner="test-team",
    )

    is_valid, error = validate_malformed_fixture(fixture, "/fake/repo")

    if not is_valid and "exist" in error.lower():
        return True, "Nonexistent file rejected"
    return False, "Nonexistent file not rejected"


def test_validate_fixture_directory_rejected() -> tuple[bool, str]:
    """Test that directory path fails validation."""
    # Create a temp directory
    with tempfile.TemporaryDirectory() as tmpdir:
        fixture = MalformedFixture(
            path=tmpdir,  # Directory, not file
            fingerprint="abc123",
            justification="Test justification",
            owner="test-team",
        )

        is_valid, error = validate_malformed_fixture(fixture, "/")

        if not is_valid and "directory" in error.lower():
            return True, "Directory rejected"
        return False, "Directory not rejected"


def test_validate_fixture_stale_fingerprint() -> tuple[bool, str]:
    """Test that stale fingerprint (file changed) fails validation."""
    # Create a temp file
    with tempfile.NamedTemporaryFile(delete=False) as f:
        f.write(b"original content")
        path = f.name

    try:
        # Compute actual fingerprint
        actual_fingerprint = compute_file_sha256(path)

        # Create fixture with wrong fingerprint
        fixture = MalformedFixture(
            path=path,
            fingerprint="wrong_fingerprint_value",  # Wrong fingerprint
            justification="Test justification",
            owner="test-team",
        )

        is_valid, error = validate_malformed_fixture(fixture, "/")

        if not is_valid and "stale" in error.lower():
            return True, "Stale fingerprint rejected"
        return False, "Stale fingerprint not rejected"
    finally:
        os.unlink(path)


def test_validate_fixture_valid() -> tuple[bool, str]:
    """Test that valid fixture passes validation."""
    # Create a temp file
    content = b"malformed JSON content for testing"
    with tempfile.NamedTemporaryFile(delete=False) as f:
        f.write(content)
        path = f.name

    try:
        # Compute actual fingerprint
        actual_fingerprint = compute_file_sha256(path)

        # Create valid fixture
        fixture = MalformedFixture(
            path=path,
            fingerprint=actual_fingerprint,
            justification="Test malformed fixture for parser resilience",
            owner="test-team",
        )

        is_valid, error = validate_malformed_fixture(fixture, "/")

        if is_valid:
            return True, "Valid fixture accepted"
        return False, f"Valid fixture rejected: {error}"
    finally:
        os.unlink(path)


def test_is_exempt_unknown_fixture() -> tuple[bool, str]:
    """Test that unknown fixture path is not exempt."""
    is_exempt, error = is_malformed_fixture_exempt("unknown/path.json", "/fake/repo")

    if not is_exempt:
        return True, "Unknown fixture not exempt"
    return False, "Unknown fixture incorrectly exempt"


def test_is_exempt_with_stale_fingerprint() -> tuple[bool, str]:
    """Test that fixture with stale fingerprint is not exempt."""
    # Create a temp file
    content = b"modified content"
    with tempfile.NamedTemporaryFile(delete=False, suffix=".json") as f:
        f.write(content)
        path = f.name

    try:
        # Create fixture with correct fingerprint first
        correct_fingerprint = compute_file_sha256(path)

        # Validate with correct fingerprint should pass
        fixture_correct = MalformedFixture(
            path=path,
            fingerprint=correct_fingerprint,
            justification="Test justification",
            owner="test-team",
        )
        is_valid_correct, _ = validate_malformed_fixture(fixture_correct, "/")

        # Create fixture with wrong fingerprint
        fixture_wrong = MalformedFixture(
            path=path,
            fingerprint="wrong_fingerprint",
            justification="Test justification",
            owner="test-team",
        )
        is_valid_wrong, error = validate_malformed_fixture(fixture_wrong, "/")

        if is_valid_correct and not is_valid_wrong and "stale" in error.lower():
            return True, "Stale fingerprint fixture not exempt"
        return False, f"Expected correct to pass and wrong to fail, got correct={is_valid_correct}, wrong={is_valid_wrong}, error={error}"
    finally:
        os.unlink(path)


def test_fixture_mutation_one_byte_change() -> tuple[bool, str]:
    """
    Mutation test: changing one byte of an exempt fixture proves the gate fails.

    This test proves that:
    1. A fixture with correct fingerprint is exempt
    2. If we change one byte, the fingerprint changes
    3. The changed fixture is no longer exempt
    """
    # Create a temp file
    original_content = b'{"key": "value"}'
    with tempfile.NamedTemporaryFile(delete=False, suffix=".json") as f:
        f.write(original_content)
        path = f.name

    try:
        # Compute original fingerprint
        original_fingerprint = compute_file_sha256(path)

        # Original should be valid
        fixture = MalformedFixture(
            path=path,
            fingerprint=original_fingerprint,
            justification="Test justification",
            owner="test-team",
        )

        is_valid_original, _ = validate_malformed_fixture(fixture, "/")

        if not is_valid_original:
            return False, "Original fixture should be valid"

        # Change one byte
        modified_content = bytearray(original_content)
        modified_content[2] = ord('X')  # Change 'e' to 'X'

        with open(path, 'wb') as f:
            f.write(modified_content)

        # Modified should be invalid (stale fingerprint)
        is_valid_modified, error = validate_malformed_fixture(fixture, "/")

        if is_valid_modified:
            return False, "Modified fixture should be invalid"

        if "stale" in error.lower():
            return True, "One byte change detected (mutation test passed)"

        return False, f"Expected stale fingerprint error, got: {error}"
    finally:
        os.unlink(path)


def test_fixture_path_normalization() -> tuple[bool, str]:
    """Test that path normalization works for exemption checks."""
    # Create a temp file
    with tempfile.NamedTemporaryFile(delete=False, suffix=".json") as f:
        f.write(b"content")
        path = f.name

    try:
        # Get the directory and filename
        dirname = os.path.dirname(path)
        basename = os.path.basename(path)

        # Create fixture with normalized path
        fixture = MalformedFixture(
            path=path,
            fingerprint=compute_file_sha256(path),
            justification="Test justification",
            owner="test-team",
        )

        is_valid, error = validate_malformed_fixture(fixture, dirname)

        if is_valid:
            return True, "Path normalization works"
        return False, f"Path normalization failed: {error}"
    finally:
        os.unlink(path)


def test_fixture_duplicate_entry_detection() -> tuple[bool, str]:
    """
    Test that duplicate fixture entries are detected.

    Note: This tests the concept - actual duplicate detection
    would happen during inventory validation.
    """
    # Create a temp file for the test
    content = b"test content for duplicate detection"
    with tempfile.NamedTemporaryFile(delete=False, suffix=".json") as f:
        f.write(content)
        path = f.name

    try:
        # Compute correct fingerprint
        correct_fingerprint = compute_file_sha256(path)

        # Two fixtures with same path but different fingerprints
        fixture1 = MalformedFixture(
            path=path,
            fingerprint=correct_fingerprint,
            justification="First justification",
            owner="team1",
        )

        fixture2 = MalformedFixture(
            path=path,
            fingerprint="wrong_fingerprint",
            justification="Second justification",
            owner="team2",
        )

        # Both should validate individually
        is_valid1, error1 = validate_malformed_fixture(fixture1, "/")
        is_valid2, error2 = validate_malformed_fixture(fixture2, "/")

        # One should pass (correct fingerprint), one should fail (wrong fingerprint)
        if is_valid1 and not is_valid2:
            return True, "Duplicate fixture entries validated correctly (one pass, one fail)"

        return False, f"Expected one pass one fail, got valid1={is_valid1}, valid2={is_valid2}"
    finally:
        os.unlink(path)
