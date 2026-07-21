// bounded_provenance_negative_test.go — Bounded provenance mutations.
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01-CORRECTION01
// §5.4: provenance mutations must recompute the manifest checksum so
// the targeted rejection (not a checksum mismatch) fires.

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestProvenance_GitObjectFormatAlias rejects a non-canonical
// git_object_format alias (e.g. "sha-1" instead of "sha1").
func TestProvenance_GitObjectFormatAlias(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "manifest.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			si := m["subject_identity"].(map[string]interface{})
			si["git_object_format"] = "sha-1"
		})
	}, "git_object_format: unsupported git_object_format=\"sha-1\"")
}

// TestProvenance_ChangedExecutableHash rejects a manifest whose
// stored controller_executable_sha256 is a different valid hex
// (the live-inode binding must match the running binary's actual
// SHA-256).
func TestProvenance_ChangedExecutableHash(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "manifest.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			si := m["subject_identity"].(map[string]interface{})
			si["controller_executable_sha256"] = strings.Repeat("b", 64)
		})
	}, "executable hash mismatch")
}

// TestProvenance_MissingGitCommit rejects a manifest with an empty
// git_commit field.
func TestProvenance_MissingGitCommit(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "manifest.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			si := m["subject_identity"].(map[string]interface{})
			si["git_commit"] = ""
		})
	}, "subject_identity.git_commit is empty")
}

// TestProvenance_MalformedExecutableHash rejects a manifest whose
// controller_executable_sha256 is not valid hex.
func TestProvenance_MalformedExecutableHash(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "manifest.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			si := m["subject_identity"].(map[string]interface{})
			si["controller_executable_sha256"] = "not-a-valid-hash"
		})
	}, "subject_identity.controller_executable_sha256")
}

// TestProvenance_ZeroFinishedTime rejects a manifest whose
// finished_at is set to the Go zero time.
func TestProvenance_ZeroFinishedTime(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "manifest.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["finished_at"] = "0001-01-01T00:00:00Z"
		})
	}, "manifest not finalized: missing finished_at")
}
