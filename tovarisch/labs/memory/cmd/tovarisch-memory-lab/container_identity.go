// container_identity.go — Canonical Container Identity Projection
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03
//
// Provides canonical container identity projection from Docker inspect output.
// The full inspect output contains many unknown fields; this module projects
// only the required identity fields into a schema-versioned document.
//
// P0-6 FIX: Use canonical container-identity.json projection, not raw inspect output.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// =============================================================================
// CANONICAL CONTAINER IDENTITY
// =============================================================================

// ContainerIdentity is the canonical container identity document.
// This is the schema-versioned projection used for binding and verification.
type ContainerIdentity struct {
	SchemaVersion string `json:"schema_version"` // "1.0.0"
	ID            string `json:"id"`             // Full container ID
	Name          string `json:"name"`           // Container name (without leading /)
}

// SupportedContainerIdentityVersions defines supported schema versions.
var SupportedContainerIdentityVersions = []string{"1.0.0"}

// =============================================================================
// IDENTITY PROJECTION
// =============================================================================

// ProjectContainerIdentity extracts the canonical identity from full Docker inspect output.
// The full output contains many unknown fields; we project only the required identity.
func ProjectContainerIdentity(inspectData []byte) (*ContainerIdentity, error) {
	if len(inspectData) == 0 {
		return nil, errors.New("empty inspect data")
	}

	// Docker inspect returns an array with one element
	var rawResults []map[string]interface{}
	if err := json.Unmarshal(inspectData, &rawResults); err != nil {
		return nil, fmt.Errorf("parse inspect output: %w", err)
	}

	if len(rawResults) == 0 {
		return nil, errors.New("inspect output has no elements")
	}

	raw := rawResults[0]

	// Extract ID (required)
	id, ok := raw["Id"].(string)
	if !ok || id == "" {
		return nil, errors.New("inspect output missing or empty Id field")
	}

	// Extract Name (required, may have leading /)
	name, ok := raw["Name"].(string)
	if !ok {
		name = ""
	}
	name = strings.TrimPrefix(name, "/")

	return &ContainerIdentity{
		SchemaVersion: "1.0.0",
		ID:            id,
		Name:          name,
	}, nil
}

// =============================================================================
// IDENTITY PERSISTENCE
// =============================================================================

// WriteContainerIdentity writes the canonical container identity to container-identity.json.
func WriteContainerIdentity(runDir, containerID, containerName string) error {
	identity := ContainerIdentity{
		SchemaVersion: "1.0.0",
		ID:            containerID,
		Name:          containerName,
	}

	data, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal container identity: %w", err)
	}

	path := filepath.Join(runDir, "container-identity.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write container identity: %w", err)
	}

	return nil
}

// ReadContainerIdentity reads and validates the canonical container identity.
func ReadContainerIdentity(runDir string) (*ContainerIdentity, error) {
	path := filepath.Join(runDir, "container-identity.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read container identity: %w", err)
	}

	identity, err := ParseContainerIdentity(data)
	if err != nil {
		return nil, fmt.Errorf("parse container identity: %w", err)
	}

	return identity, nil
}

// ParseContainerIdentity parses and validates the canonical container identity.
func ParseContainerIdentity(data []byte) (*ContainerIdentity, error) {
	if len(data) == 0 {
		return nil, errors.New("empty input")
	}

	var identity ContainerIdentity
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&identity); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	// Validate schema version
	if identity.SchemaVersion == "" {
		return nil, errors.New("missing schema_version")
	}
	schemaValid := false
	for _, v := range SupportedContainerIdentityVersions {
		if identity.SchemaVersion == v {
			schemaValid = true
			break
		}
	}
	if !schemaValid {
		return nil, fmt.Errorf("unsupported schema version %q", identity.SchemaVersion)
	}

	// Validate required fields
	if identity.ID == "" {
		return nil, errors.New("missing required field: id")
	}

	return &identity, nil
}

// =============================================================================
// CANONICAL ARTIFACT INVENTORY
// =============================================================================

// CanonicalChildArtifactInventory defines the exact required artifact filenames for a child run.
// P0-7 FIX: Exact inventory prevents checksum bypass through missing/extra entries.
var CanonicalChildArtifactInventory = []string{
	"container-identity.json", // P0-6 FIX: Canonical projection, not raw inspect
	"events.jsonl",
	"final-canary-state.json",
	"initial-canary-state.json",
	"manifest.json",
	"network-identity.json", // P0-5 FIX: Network identity is mandatory in per_run mode
	"samples.csv",
	"verdict.json",
	"workload-result.json",
}

// ValidateArtifactInventory checks that the checksum inventory matches the canonical list exactly.
// P0-7 FIX: Rejects missing, extra, duplicate, absolute, and parent-traversal entries.
func ValidateArtifactInventory(checksumPaths []string) error {
	// Build sets for comparison and validate path safety
	checksumSet := make(map[string]bool)
	for _, p := range checksumPaths {
		if p == "" {
			return errors.New("empty path in checksum inventory")
		}
		if checksumSet[p] {
			return fmt.Errorf("duplicate entry in checksum inventory: %s", p)
		}
		// P0-7 FIX: Reject absolute paths
		if filepath.IsAbs(p) {
			return fmt.Errorf("absolute path not allowed in checksum inventory: %s", p)
		}
		// P0-7 FIX: Reject parent traversal
		if strings.Contains(p, "..") {
			return fmt.Errorf("parent traversal not allowed in checksum inventory: %s", p)
		}
		checksumSet[p] = true
	}

	canonicalSet := make(map[string]bool)
	for _, p := range CanonicalChildArtifactInventory {
		canonicalSet[p] = true
	}

	// Check for missing entries
	for _, p := range CanonicalChildArtifactInventory {
		if !checksumSet[p] {
			return fmt.Errorf("missing required entry in checksum inventory: %s", p)
		}
	}

	// Check for extra entries
	for _, p := range checksumPaths {
		if !canonicalSet[p] {
			return fmt.Errorf("extra entry in checksum inventory (not in canonical list): %s", p)
		}
	}

	return nil
}

// =============================================================================
// LEGACY COMPATIBILITY
// =============================================================================

// LoadContainerInspectLegacy reads the legacy container-inspect.json (full Docker output).
// This is retained for diagnostic purposes only; verification uses container-identity.json.
func LoadContainerInspectLegacy(runDir string) ([]byte, error) {
	path := filepath.Join(runDir, "container-inspect.json")
	return os.ReadFile(path)
}
