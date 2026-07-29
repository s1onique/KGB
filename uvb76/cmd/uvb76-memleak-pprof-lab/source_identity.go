// Package main provides the UVB-76 pprof memory leak lab.
//
// # Embedded Source Identity
//
// This file implements P0-1: Embedded source-identity authority using runtime/debug.ReadBuildInfo.
// The resolver extracts and normalizes VCS revision and modification status from the running binary.
package main

import (
	"errors"
	"runtime/debug"
	"strings"
)

// EmbeddedSourceIdentity represents the canonical source identity from the running binary.
type EmbeddedSourceIdentity struct {
	VCSRevision string
	VCSModified bool
	Path        string
}

// Sentinel errors for embedded source identity resolution.
var (
	// ErrMissingBuildInfo is returned when build info is not available.
	ErrMissingBuildInfo = errors.New("missing build info: binary not built with modules")

	// ErrMissingVCSRevision is returned when vcs.revision is not present.
	ErrMissingVCSRevision = errors.New("missing vcs.revision in build info")

	// ErrEmptyVCSRevision is returned when vcs.revision is present but empty.
	ErrEmptyVCSRevision = errors.New("vcs.revision is empty")

	// ErrMissingVCSModified is returned when vcs.modified is not present.
	ErrMissingVCSModified = errors.New("missing vcs.modified in build info")

	// ErrMalformedVCSModified is returned when vcs.modified has an unrecognized value.
	ErrMalformedVCSModified = errors.New("malformed vcs.modified value (expected 'true' or 'false')")

	// ErrVCSModifiedTrue is returned when vcs.modified is 'true' (dirty build).
	ErrVCSModifiedTrue = errors.New("vcs.modified=true: binary has uncommitted changes")

	// ErrNoSettings is returned when build info has no settings at all.
	ErrNoSettings = errors.New("build info has no settings")
)

// SourceIdentityResolver defines the interface for resolving embedded source identity.
type SourceIdentityResolver interface {
	Resolve() (*EmbeddedSourceIdentity, error)
}

// readBuildInfoFunc is the function type for reading build info.
// P0-5: Injectable for deterministic testing.
type readBuildInfoFunc func() (*debug.BuildInfo, bool)

// defaultReadBuildInfo is the production implementation.
func defaultReadBuildInfo() (*debug.BuildInfo, bool) {
	return debug.ReadBuildInfo()
}

// buildInfoResolver implements SourceIdentityResolver using runtime/debug.ReadBuildInfo.
// P0-5: readBuildInfo is injectable for deterministic testing.
type buildInfoResolver struct {
	readBuildInfo readBuildInfoFunc
}

// NewBuildInfoResolver creates a new build info resolver.
func NewBuildInfoResolver() SourceIdentityResolver {
	return &buildInfoResolver{
		readBuildInfo: defaultReadBuildInfo,
	}
}

// Resolve extracts embedded source identity from the running binary.
func (r *buildInfoResolver) Resolve() (*EmbeddedSourceIdentity, error) {
	bi, ok := r.readBuildInfo()
	if !ok {
		return nil, ErrMissingBuildInfo
	}

	if len(bi.Settings) == 0 {
		return nil, ErrNoSettings
	}

	var revision string
	var revisionSeen bool
	var modified string
	var modifiedSeen bool
	var path string

	for _, setting := range bi.Settings {
		switch setting.Key {
		case "vcs.revision":
			revisionSeen = true
			revision = setting.Value
		case "vcs.modified":
			modifiedSeen = true
			modified = setting.Value
		case "vcs.source":
			// Capture source URL for diagnostics
			path = setting.Value
		}
	}

	// P0-6: Distinguish missing vs empty vcs.revision
	if !revisionSeen {
		return nil, ErrMissingVCSRevision
	}
	if revision == "" {
		return nil, ErrEmptyVCSRevision
	}

	// Validate modified status
	if !modifiedSeen {
		return nil, ErrMissingVCSModified
	}

	// Normalize modified status
	switch strings.ToLower(modified) {
	case "true":
		return nil, ErrVCSModifiedTrue
	case "false":
		// Valid clean build
	default:
		return nil, ErrMalformedVCSModified
	}

	return &EmbeddedSourceIdentity{
		VCSRevision: revision,
		VCSModified: false, // Normalized to boolean
		Path:        path,
	}, nil
}

// ValidateSourceIdentity performs pre-start validation of embedded source identity.
// P0-2: Must be called before any process startup side effects.
func ValidateSourceIdentity(resolver SourceIdentityResolver) error {
	identity, err := resolver.Resolve()
	if err != nil {
		return err
	}

	// Additional structural validation
	if identity.VCSRevision == "" {
		return ErrMissingVCSRevision
	}

	return nil
}

// ProductionSourceIdentityResolver is the singleton production resolver.
var ProductionSourceIdentityResolver = NewBuildInfoResolver()
