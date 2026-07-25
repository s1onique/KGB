// path.go — Canonical canary image build metadata path resolver.
//
// The resolver centralizes how the production CLI and the compiled
// helper find the canary image build metadata JSON. Three sources
// are tried in priority order:
//
//   1. ExplicitPath (passed by the production CLI from
//      `--canary-build-metadata`).
//   2. Environment (the value of the TOVARISCH_CANARY_METADATA_PATH
//      environment variable, captured once by the caller).
//   3. Repository (compatibility fallback: <Repository>/tovarisch/labs/
//      memory/canary-image-build.json).
//
// The first source whose path resolves to a regular file, parses as
// JSON and validates against CanaryImageBuild (via buildmetadata.Read
// and CanaryImageBuild.Validate) is returned as an absolute,
// symlink-resolved canonical path. Any rejection — missing file,
// broken symlink, directory, socket, device, FIFO, non-canonical
// JSON, malformed identity — produces an error annotated with the
// failing source. The fallback chain never silently substitutes a
// stale path for a configured source.
//
// This file replaces the untracked proposal at
// docs/epics/.../correction47/pending-internal-evidence/canary_metadata_path.go.
// The proposal was reviewed and intentionally rewritten — see
// docs/epics/.../correction48/pending-patch-inventory.txt.
//
// Reference: kgb://factory/workflow.

package buildmetadata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// EnvMetadataPath is the environment variable that names the
// canary-image build metadata file when the production CLI has no
// explicit flag value. Callers MUST capture the value once via
// os.Getenv at startup and pass the result as
// MetadataPathOptions.Environment so tests and live runs share a
// single read path.
const EnvMetadataPath = "TOVARISCH_CANARY_METADATA_PATH"

// RepositoryCompatibilityPath is the fixed repository-relative
// path that historically held the canary image build metadata. It
// is consulted only when neither an explicit flag nor the
// environment variable provides a usable source. It is never
// recomputed by callers — the resolver is the single source of
// truth.
const RepositoryCompatibilityPath = "tovarisch/labs/memory/canary-image-build.json"

// MetadataPathOptions captures the three inputs the resolver will
// rank. A zero value resolves against the repository compatibility
// fallback only when Repository is non-empty.
type MetadataPathOptions struct {
	// ExplicitPath, when non-empty, wins over everything else.
	// Typical source: the production CLI's `--canary-build-metadata`
	// flag (after path.Abs).
	ExplicitPath string
	// Environment is the value associated with EnvMetadataPath at
	// the call site. It is read by the caller (typically via
	// os.Getenv) and passed through so the resolver never reaches
	// back into the process environment.
	Environment string
	// Repository is the KGB repository root whose
	// tovarisch/labs/memory subtree holds the historic
	// canary-image-build.json. When the two higher-priority
	// sources are empty and this is non-empty, the resolver
	// constructs <Repository>/tovarisch/labs/memory/canary-image-build.json
	// as a compatibility fallback.
	Repository string
}

// ErrMetadataUnresolved indicates that no metadata source was
// configured at all. Callers that need to distinguish "no source"
// from "source configured but unusable" should string-match on this
// sentinel.
var ErrMetadataUnresolved = errors.New("canary metadata path is unresolved: set --canary-build-metadata, set TOVARISCH_CANARY_METADATA_PATH, or provide a repository root")

// ErrMetadataBrokenSymlink indicates that a configured source
// pointed at a symlink whose target cannot be resolved. Broken
// symlinks are deliberately rejected so callers cannot silently
// chase a deleted target into a different directory.
var ErrMetadataBrokenSymlink = errors.New("canary metadata source is a broken symlink")

// ErrMetadataNotRegular indicates that a configured source
// resolved to something other than a regular file: a directory, a
// socket, a device, a FIFO, or any other non-regular entry.
var ErrMetadataNotRegular = errors.New("canary metadata source is not a regular file")

// ErrMetadataInvalid indicates that the resolved file does not
// parse as CanaryImageBuild, contains a stale or unknown schema,
// or fails identity validation. The same sentinel covers both
// "unknown schema" and "stale identity" so callers can treat the
// two cases with the same "source is unusable" handling.
var ErrMetadataInvalid = errors.New("canary metadata file failed validation")

// sourceOrigin names the source whose path is being canonicalized.
// It appears in error messages so that operators can locate the
// responsible input field quickly.
type sourceOrigin string

const (
	originExplicit    sourceOrigin = "explicit_flag"
	originEnvironment sourceOrigin = "environment"
	originRepository  sourceOrigin = "repository_compatibility"
)

// ResolveCanaryMetadataPath returns the canonical, symlink-resolved
// absolute path to a canary image build metadata file that already
// parses as a validated CanaryImageBuild. Calling this twice with
// the same options returns the same path or the same error class
// (since resolve is idempotent and Read is deterministic). Any
// rejection at canonicalize() or verify() time is final — the
// resolver does not fall through to lower-priority sources on a
// configured-but-broken input.
func ResolveCanaryMetadataPath(options MetadataPathOptions) (string, error) {
	ranked := []struct {
		raw    string
		origin sourceOrigin
	}{
		{options.ExplicitPath, originExplicit},
		{options.Environment, originEnvironment},
	}
	for _, s := range ranked {
		if s.raw == "" {
			continue
		}
		canonical, err := canonicalize(s.raw, s.origin)
		if err != nil {
			return "", err
		}
		if err := verifyValid(canonical); err != nil {
			return "", err
		}
		return canonical, nil
	}
	if options.Repository == "" {
		return "", ErrMetadataUnresolved
	}
	compat := filepath.Join(options.Repository, RepositoryCompatibilityPath)
	canonical, err := canonicalize(compat, originRepository)
	if err != nil {
		return "", err
	}
	if err := verifyValid(canonical); err != nil {
		return "", err
	}
	return canonical, nil
}

// canonicalize takes a raw path and returns the absolute,
// symlink-resolved path of a regular file. Any non-regular entry
// (including the original path being a directory, a symlink to a
// directory, a socket, a device, a FIFO, or a broken symlink) is
// rejected with a typed error. The returned path is what the
// resolver passes to verifyValid and to callers.
func canonicalize(raw string, origin sourceOrigin) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("%s: empty path", origin)
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("%s: filepath.Abs(%q): %w", origin, raw, err)
	}
	// Lstat first so we can explicitly handle symlinks before
	// the canonical post-stat step. A path that does not exist
	// at all surfaces here.
	lst, err := os.Lstat(abs)
	if err != nil {
		return "", fmt.Errorf("%s: lstat(%q): %w", origin, abs, err)
	}
	resolved := abs
	if lst.Mode()&os.ModeSymlink != 0 {
		// Resolve via EvalSymlinks. A broken symlink fails
		// here; reject it explicitly rather than letting the
		// following os.Stat misreport.
		target, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("%s: %w: %q -> %v", origin, ErrMetadataBrokenSymlink, abs, err)
		}
		resolved = target
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("%s: stat(%q): %w", origin, resolved, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s: %w: %q (mode=%s)", origin, ErrMetadataNotRegular, resolved, info.Mode())
	}
	return resolved, nil
}

// verifyValid reads the resolved file via buildmetadata.Read and
// confirms the parsed CanaryImageBuild survives Validate. The
// returned error is annotated with the source path so operators can
// locate the offending file quickly.
func verifyValid(canonical string) error {
	metadata, err := Read(canonical)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrMetadataInvalid, canonical, err)
	}
	if err := metadata.Validate(); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrMetadataInvalid, canonical, err)
	}
	return nil
}
