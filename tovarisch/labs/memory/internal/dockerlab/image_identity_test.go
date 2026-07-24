// image_identity_test.go — Unit Tests for Exact Image Identity
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03-P0-10-EXACT-IMAGE-ID-NO-PULL-SMOKE01
//
// Tests the canonical image ID validation and resolution.
//
// Mandatory automated tests from P0-10:
// - TestValidateExactImageID_AcceptsCanonicalSHA256
// - TestValidateExactImageID_RejectsEmpty
// - TestValidateExactImageID_RejectsTag
// - TestValidateExactImageID_RejectsRepositoryDigestInImageIDField
// - TestValidateExactImageID_RejectsShortID
// - TestValidateExactImageID_RejectsUppercase
// - TestValidateExactImageID_RejectsWhitespace
// - TestValidateExactImageID_RejectsNonHex
// - TestValidateExactImageID_RejectsMultipleLines

package dockerlab

import (
	"testing"
)

// =============================================================================
// IDENTITY PARSING TESTS
// =============================================================================

func TestValidateExactImageID_AcceptsCanonicalSHA256(t *testing.T) {
	// Valid canonical image ID
	validID := "sha256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	err := ValidateExactImageID(validID)
	if err != nil {
		t.Errorf("expected valid ID %q to pass, got: %v", validID, err)
	}
}

func TestValidateExactImageID_RejectsEmpty(t *testing.T) {
	err := ValidateExactImageID("")
	if err == nil {
		t.Error("expected empty ID to fail")
	}
	if err != ErrEmptyImageID {
		t.Errorf("expected ErrEmptyImageID, got: %v", err)
	}
}

func TestValidateExactImageID_RejectsTag(t *testing.T) {
	// Tag alone is not a valid image ID
	err := ValidateExactImageID("latest")
	if err == nil {
		t.Error("expected tag-only reference to fail")
	}

	// Tag with registry is also not valid
	err = ValidateExactImageID("registry.example/repo:latest")
	if err == nil {
		t.Error("expected registry tag reference to fail")
	}
}

func TestValidateExactImageID_RejectsRepositoryDigestInImageIDField(t *testing.T) {
	// Repository digest format should not be used in image ID field
	// The image ID field should contain the full ID, not a digest reference
	err := ValidateExactImageID("sha256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2@sha256:anotherdigest")
	if err == nil {
		t.Error("expected repository digest format to fail")
	}
}

func TestValidateExactImageID_RejectsShortID(t *testing.T) {
	// Short ID (12 chars) is not valid
	shortID := "sha256:a1b2c3d4e5f6"
	err := ValidateExactImageID(shortID)
	if err == nil {
		t.Error("expected short ID to fail")
	}
}

func TestValidateExactImageID_RejectsUppercase(t *testing.T) {
	// Image IDs must be lowercase
	upperID := "sha256:A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2"
	err := ValidateExactImageID(upperID)
	if err == nil {
		t.Error("expected uppercase ID to fail")
	}
}

func TestValidateExactImageID_RejectsWhitespace(t *testing.T) {
	// ID with leading whitespace
	err := ValidateExactImageID("  sha256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")
	if err == nil {
		t.Error("expected ID with leading whitespace to fail")
	}

	// ID with trailing whitespace
	err = ValidateExactImageID("sha256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2  ")
	if err == nil {
		t.Error("expected ID with trailing whitespace to fail")
	}
}

func TestValidateExactImageID_RejectsNonHex(t *testing.T) {
	// ID with non-hexadecimal characters
	nonHexID := "sha256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6g1h2"
	err := ValidateExactImageID(nonHexID)
	if err == nil {
		t.Error("expected ID with non-hex chars to fail")
	}
}

func TestValidateExactImageID_RejectsMultipleLines(t *testing.T) {
	// ID with embedded newline
	multiLineID := "sha256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2\nsha256:0000000000000000000000000000000000000000000000000000000000000000"
	err := ValidateExactImageID(multiLineID)
	if err == nil {
		t.Error("expected multi-line ID to fail")
	}
}

// =============================================================================
// RESOLVEDIMAGEIDENTITY STRUCT TESTS
// =============================================================================

func TestResolvedImageIdentity_Structure(t *testing.T) {
	// Test that the struct can be populated correctly
	identity := ResolvedImageIdentity{
		DescriptiveReference: "kgb-tovarisch-canary:latest",
		ImageID:             "sha256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		RepositoryDigest:     "kgb-tovarisch-canary@sha256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
	}

	if identity.DescriptiveReference == "" {
		t.Error("DescriptiveReference should be set")
	}
	if identity.ImageID == "" {
		t.Error("ImageID should be set")
	}
	// RepositoryDigest can be empty for local images
}

func TestResolvedImageIdentity_EmptyRepositoryDigest(t *testing.T) {
	// Test that RepositoryDigest can be empty (local-only images)
	identity := ResolvedImageIdentity{
		DescriptiveReference: "local-image",
		ImageID:             "sha256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		RepositoryDigest:     "", // Can be empty for local images
	}

	if identity.ImageID == "" {
		t.Error("ImageID must be set")
	}
	// RepositoryDigest being empty is valid
}

// =============================================================================
// ERROR TYPE TESTS
// =============================================================================

func TestErrInvalidImageIDFormat_Error(t *testing.T) {
	err := &ErrInvalidImageIDFormat{
		Input:  "invalid",
		Reason: "must match sha256:<64 lowercase hex>",
	}
	expected := `invalid image ID format "invalid": must match sha256:<64 lowercase hex>`
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestErrImageNotFound_Error(t *testing.T) {
	err := &ErrImageNotFound{Reference: "nonexistent:latest"}
	expected := "image not found: nonexistent:latest"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestErrAmbiguousOutput_Error(t *testing.T) {
	err := &ErrAmbiguousOutput{
		Reference: "ambiguous-tag",
		Output:    "multiple lines",
	}
	expected := `ambiguous image inspect output for "ambiguous-tag": multiple lines`
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

// =============================================================================
// CANONICAL PATTERN TESTS
// =============================================================================

func TestCanonicalImageIDPattern_Valid(t *testing.T) {
	// Test various valid formats
	validIDs := []string{
		"sha256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"sha256:0000000000000000000000000000000000000000000000000000000000000000",
		"sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	}

	for _, id := range validIDs {
		if !canonicalImageIDPattern.MatchString(id) {
			t.Errorf("expected %q to match pattern", id)
		}
	}
}

func TestCanonicalImageIDPattern_Invalid(t *testing.T) {
	// Test various invalid formats
	invalidIDs := []string{
		"latest",
		"registry/image:latest",
		"sha256:abc",
		"SHA256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"sha256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6g1h2",
		"",
	}

	for _, id := range invalidIDs {
		if canonicalImageIDPattern.MatchString(id) {
			t.Errorf("expected %q to NOT match pattern", id)
		}
	}
}
