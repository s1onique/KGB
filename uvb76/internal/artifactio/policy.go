// Package artifactio provides a focused, atomic, sanitizing artifact
// persistence boundary for UVB-76 producers.
//
// Every active writer in the UVB-76 lab and tool surface must call
// WriteRedactedJSON / WriteRedactedText / WriteRedactedConfig (or their
// []byte forms) rather than os.WriteFile directly. The package enforces:
//
//   - sanitize-before-persistence for redacted text / JSON / config,
//   - post-sanitization validation (the file on disk must contain no
//     known prohibited secret),
//   - bounded input and output sizes,
//   - same-directory temporary file for atomic publication,
//   - restrictive temporary file permissions,
//   - clean-up of temporary files on every failure path,
//   - explicit file mode applied to the final publication.
//
// Diagnostics never include the secret values themselves.
package artifactio

import (
	"errors"
	"io/fs"
)

// WritePolicy describes how a final-path artifact must be persisted.
type WritePolicy struct {
	// FileMode is the mode applied to the final published artifact.
	// Use 0600 for runtime artifacts and 0644 only for repository-sanitized
	// public fixtures.
	FileMode fs.FileMode

	// MaxInputBytes caps the size of the input bytes before sanitization.
	// Writes exceeding the bound fail without touching the filesystem.
	MaxInputBytes int

	// MaxOutputBytes caps the size of the post-sanitization bytes.
	// Writes exceeding the bound fail without touching the filesystem.
	MaxOutputBytes int

	// RequireAtomic controls whether publication must be atomic (rename).
	// All runtime diagnostics and committed sanitized artifacts require
	// atomic publication. Test fixtures may opt out only when documented.
	RequireAtomic bool

	// PreserveStructure, when true, indicates the producer expects the
	// sanitized bytes to be re-parseable as the open-text kind declared
	// in the producer contract (JSON, plain text, or config). The
	// boundary uses this to perform a structural sanity check.
	PreserveStructure bool

	// OpenTextKind documents the open-text format for diagnostics.
	// Allowed values: "json", "text", "config".
	OpenTextKind string
}

// DefaultRuntimePolicy is the recommended policy for runtime diagnostic
// artifacts. It enforces:
//   - 0600 final mode (Unix read/write for owner only),
//   - 1 MiB input bound,
//   - 4 MiB output bound,
//   - atomic publication,
//   - structural preservation,
//   - JSON open-text kind.
func DefaultRuntimePolicy() WritePolicy {
	return WritePolicy{
		FileMode:          0o600,
		MaxInputBytes:     1 << 20,
		MaxOutputBytes:    4 << 20,
		RequireAtomic:     true,
		PreserveStructure: true,
		OpenTextKind:      "json",
	}
}

// DefaultTextPolicy is the recommended policy for redacted text artifacts.
func DefaultTextPolicy() WritePolicy {
	return WritePolicy{
		FileMode:          0o600,
		MaxInputBytes:     1 << 20,
		MaxOutputBytes:    4 << 20,
		RequireAtomic:     true,
		PreserveStructure: false,
		OpenTextKind:      "text",
	}
}

// DefaultConfigPolicy is the recommended policy for redacted config artifacts.
func DefaultConfigPolicy() WritePolicy {
	return WritePolicy{
		FileMode:          0o600,
		MaxInputBytes:     1 << 20,
		MaxOutputBytes:    4 << 20,
		RequireAtomic:     true,
		PreserveStructure: true,
		OpenTextKind:      "config",
	}
}

// DefaultCommittedFixturePolicy is the policy for committed sanitized
// example/fixture artifacts that must remain visible to readers.
func DefaultCommittedFixturePolicy() WritePolicy {
	return WritePolicy{
		FileMode:          0o644,
		MaxInputBytes:     1 << 20,
		MaxOutputBytes:    4 << 20,
		RequireAtomic:     true,
		PreserveStructure: true,
		OpenTextKind:      "json",
	}
}

// validate ensures the policy is internally consistent.
func (p WritePolicy) validate() error {
	if p.FileMode == 0 {
		return errors.New("artifactio: FileMode must be non-zero (use DefaultRuntimePolicy)")
	}
	if p.MaxInputBytes <= 0 || p.MaxInputBytes > 16<<20 {
		return errors.New("artifactio: MaxInputBytes must be in (0, 16MiB]")
	}
	if p.MaxOutputBytes <= 0 || p.MaxOutputBytes > 32<<20 {
		return errors.New("artifactio: MaxOutputBytes must be in (0, 32MiB]")
	}
	if p.MaxOutputBytes < p.MaxInputBytes {
		return errors.New("artifactio: MaxOutputBytes must be >= MaxInputBytes")
	}
	if p.PreserveStructure {
		switch p.OpenTextKind {
		case "json", "text", "config":
			// ok
		default:
			return errors.New("artifactio: PreserveStructure requires OpenTextKind in {json,text,config}")
		}
	}
	return nil
}
