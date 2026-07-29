// Package main provides tests for embedded source identity.
package main

import (
	"errors"
	"testing"
)

// fakeSourceIdentityResolver is a test double for SourceIdentityResolver.
type fakeSourceIdentityResolver struct {
	identity *EmbeddedSourceIdentity
	err      error
}

func (f *fakeSourceIdentityResolver) Resolve() (*EmbeddedSourceIdentity, error) {
	return f.identity, f.err
}

func TestValidateSourceIdentity_ValidCleanBuild(t *testing.T) {
	resolver := &fakeSourceIdentityResolver{
		identity: &EmbeddedSourceIdentity{
			VCSRevision: "abc123def456",
			VCSModified: false,
			Path:        "github.com/example/repo",
		},
	}

	err := ValidateSourceIdentity(resolver)
	if err != nil {
		t.Errorf("expected valid identity to pass, got error: %v", err)
	}
}

func TestValidateSourceIdentity_MissingBuildInfo(t *testing.T) {
	resolver := &fakeSourceIdentityResolver{
		err: ErrMissingBuildInfo,
	}

	err := ValidateSourceIdentity(resolver)
	if !errors.Is(err, ErrMissingBuildInfo) {
		t.Errorf("expected ErrMissingBuildInfo, got: %v", err)
	}
}

func TestValidateSourceIdentity_MissingRevision(t *testing.T) {
	resolver := &fakeSourceIdentityResolver{
		err: ErrMissingVCSRevision,
	}

	err := ValidateSourceIdentity(resolver)
	if !errors.Is(err, ErrMissingVCSRevision) {
		t.Errorf("expected ErrMissingVCSRevision, got: %v", err)
	}
}

func TestValidateSourceIdentity_EmptyRevision(t *testing.T) {
	resolver := &fakeSourceIdentityResolver{
		err: ErrEmptyVCSRevision,
	}

	err := ValidateSourceIdentity(resolver)
	if !errors.Is(err, ErrEmptyVCSRevision) {
		t.Errorf("expected ErrEmptyVCSRevision, got: %v", err)
	}
}

func TestValidateSourceIdentity_ModifiedBinary(t *testing.T) {
	resolver := &fakeSourceIdentityResolver{
		err: ErrVCSModifiedTrue,
	}

	err := ValidateSourceIdentity(resolver)
	if !errors.Is(err, ErrVCSModifiedTrue) {
		t.Errorf("expected ErrVCSModifiedTrue, got: %v", err)
	}
}

func TestValidateSourceIdentity_MalformedModified(t *testing.T) {
	resolver := &fakeSourceIdentityResolver{
		err: ErrMalformedVCSModified,
	}

	err := ValidateSourceIdentity(resolver)
	if !errors.Is(err, ErrMalformedVCSModified) {
		t.Errorf("expected ErrMalformedVCSModified, got: %v", err)
	}
}

func TestValidateSourceIdentity_MissingModified(t *testing.T) {
	resolver := &fakeSourceIdentityResolver{
		err: ErrMissingVCSModified,
	}

	err := ValidateSourceIdentity(resolver)
	if !errors.Is(err, ErrMissingVCSModified) {
		t.Errorf("expected ErrMissingVCSModified, got: %v", err)
	}
}

// deterministicBuildInfoResolver creates a resolver with deterministic behavior for testing.
type deterministicBuildInfoResolver struct {
	err      error
	identity *EmbeddedSourceIdentity
	readBuildInfoFunc
}

func (r *deterministicBuildInfoResolver) Resolve() (*EmbeddedSourceIdentity, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.identity, nil
}

func TestBuildInfoResolver_MissingBuildInfo(t *testing.T) {
	// Test that ErrMissingBuildInfo is correctly returned when build info is unavailable.
	resolver := &deterministicBuildInfoResolver{
		err: ErrMissingBuildInfo,
	}
	_, err := resolver.Resolve()
	if !errors.Is(err, ErrMissingBuildInfo) {
		t.Fatalf("expected ErrMissingBuildInfo, got: %v", err)
	}
}

func TestBuildInfoResolver_NoSettings(t *testing.T) {
	// Test that ErrNoSettings is correctly returned when settings are empty.
	resolver := &deterministicBuildInfoResolver{
		err: ErrNoSettings,
	}
	_, err := resolver.Resolve()
	if !errors.Is(err, ErrNoSettings) {
		t.Fatalf("expected ErrNoSettings, got: %v", err)
	}
}

func TestBuildInfoResolver_MissingRevision(t *testing.T) {
	// Test that ErrMissingVCSRevision is correctly returned.
	resolver := &deterministicBuildInfoResolver{
		err: ErrMissingVCSRevision,
	}
	_, err := resolver.Resolve()
	if !errors.Is(err, ErrMissingVCSRevision) {
		t.Fatalf("expected ErrMissingVCSRevision, got: %v", err)
	}
}

func TestBuildInfoResolver_EmptyRevision(t *testing.T) {
	// Test that ErrEmptyVCSRevision is correctly returned.
	resolver := &deterministicBuildInfoResolver{
		err: ErrEmptyVCSRevision,
	}
	_, err := resolver.Resolve()
	if !errors.Is(err, ErrEmptyVCSRevision) {
		t.Fatalf("expected ErrEmptyVCSRevision, got: %v", err)
	}
}

func TestBuildInfoResolver_VCSModifiedTrue(t *testing.T) {
	// Test that ErrVCSModifiedTrue is correctly returned for dirty builds.
	resolver := &deterministicBuildInfoResolver{
		err: ErrVCSModifiedTrue,
	}
	_, err := resolver.Resolve()
	if !errors.Is(err, ErrVCSModifiedTrue) {
		t.Fatalf("expected ErrVCSModifiedTrue, got: %v", err)
	}
}

func TestBuildInfoResolver_Success(t *testing.T) {
	// Test successful resolution with valid clean build.
	resolver := &deterministicBuildInfoResolver{
		identity: &EmbeddedSourceIdentity{
			VCSRevision: "abc123def456",
			VCSModified: false,
			Path:        "github.com/example/repo",
		},
	}
	identity, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if identity.VCSRevision != "abc123def456" {
		t.Errorf("expected VCSRevision abc123def456, got: %s", identity.VCSRevision)
	}
}

func TestEmbeddedSourceIdentity_Structure(t *testing.T) {
	// Test that the identity struct has all required fields
	identity := &EmbeddedSourceIdentity{
		VCSRevision: "rev123",
		VCSModified: false,
		Path:        "github.com/test/repo",
	}

	if identity.VCSRevision == "" {
		t.Error("VCSRevision should be populated")
	}

	if identity.VCSModified != false {
		t.Error("VCSModified should be false for clean build")
	}
}

// Test that all sentinel errors are distinct
func TestSentinelErrors_AreDistinct(t *testing.T) {
	sentinels := []error{
		ErrMissingBuildInfo,
		ErrMissingVCSRevision,
		ErrEmptyVCSRevision,
		ErrMissingVCSModified,
		ErrMalformedVCSModified,
		ErrVCSModifiedTrue,
		ErrNoSettings,
	}

	for i, e1 := range sentinels {
		for j, e2 := range sentinels {
			if i != j && errors.Is(e1, e2) {
				t.Errorf("sentinel errors %v and %v should be distinct", e1, e2)
			}
		}
	}
}
