// Package artifactwriterbaseline provides loading, validation, and generation
// of the artifact-writer-scanner ratchet baseline.
//
// The baseline is sharded into JSON Lines format to comply with LLM-friendliness
// line limits. A manifest file tracks all shards and their metadata.
//
// Schema: ratchet-sharded-v1
package artifactwriterbaseline

import "encoding/json"

// SchemaVersion is the current baseline schema identifier.
const SchemaVersion = "ratchet-sharded-v1"

// Finding represents a single artifact-writer-scanner finding.
type Finding struct {
	FindingID             string `json:"finding_id"`
	SurfaceID            string `json:"surface_id"`
	File                 string `json:"file"`
	Line                 int    `json:"line"`
	Operation            string `json:"operation"`
	DestinationExpression string `json:"destination_expression"`
	EnclosingSymbol      string `json:"enclosing_symbol"`
	ASTFingerprint       string `json:"ast_fingerprint"`
	Justification        string `json:"justification"`
	Owner                string `json:"owner"`
	SuccessorACT         string `json:"successor_act"`
}

// Manifest represents the baseline manifest file.
type Manifest struct {
	SchemaVersion  string   `json:"schema_version"`
	BaselineCommit string   `json:"baseline_commit"`
	Generator      string   `json:"generator"`
	GeneratedAt    string   `json:"generated_at"`
	Shards         []string `json:"shards"`
	SurfaceIDs     []string `json:"surface_ids"`
}

// Validate validates the manifest structure.
// Returns validation errors or nil if valid.
func (m *Manifest) Validate() []error {
	var errs []error

	if m.SchemaVersion == "" {
		errs = append(errs, &ValidationError{Field: "schema_version", Msg: "required"})
	}
	if m.SchemaVersion != SchemaVersion {
		errs = append(errs, &ValidationError{Field: "schema_version", Msg: "must be " + SchemaVersion})
	}
	if m.BaselineCommit == "" {
		errs = append(errs, &ValidationError{Field: "baseline_commit", Msg: "required"})
	}
	if m.Generator == "" {
		errs = append(errs, &ValidationError{Field: "generator", Msg: "required"})
	}

	// Zero findings baseline is valid (empty shards and empty surface_ids)
	// Non-empty: both must be non-empty (one surface may have multiple shards)
	if (len(m.Shards) == 0) != (len(m.SurfaceIDs) == 0) {
		errs = append(errs, &ValidationError{
			Field: "shards/surface_ids",
			Msg:   "must either both be empty or both be non-empty",
		})
	}

	// Check for duplicate shards
	seenShards := make(map[string]bool)
	for _, shard := range m.Shards {
		if shard == "" {
			errs = append(errs, &ValidationError{Field: "shards", Msg: "shard name cannot be empty"})
			continue
		}
		if seenShards[shard] {
			errs = append(errs, &ValidationError{Field: "shards", Msg: "duplicate shard: " + shard})
		}
		seenShards[shard] = true
	}

	// Check for duplicate surface IDs
	seenSurfaces := make(map[string]bool)
	for _, surface := range m.SurfaceIDs {
		if surface == "" {
			errs = append(errs, &ValidationError{Field: "surface_ids", Msg: "surface_id cannot be empty"})
			continue
		}
		if seenSurfaces[surface] {
			errs = append(errs, &ValidationError{Field: "surface_ids", Msg: "duplicate surface_id: " + surface})
		}
		seenSurfaces[surface] = true
	}

	return errs
}

// Validate validates a single finding.
// Returns validation errors or nil if valid.
func (f *Finding) Validate() []error {
	var errs []error

	if f.FindingID == "" {
		errs = append(errs, &ValidationError{Field: "finding_id", Msg: "required"})
	}
	if f.SurfaceID == "" {
		errs = append(errs, &ValidationError{Field: "surface_id", Msg: "required"})
	}
	if f.File == "" {
		errs = append(errs, &ValidationError{Field: "file", Msg: "required"})
	}
	if f.Line <= 0 {
		errs = append(errs, &ValidationError{Field: "line", Msg: "must be positive"})
	}

	return errs
}

// ValidationError represents a validation failure.
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Msg
}

// MarshalJSON implements json.Marshaler for Manifest to ensure field order.
func (m Manifest) MarshalJSON() ([]byte, error) {
	type Alias Manifest
	return json.Marshal(&struct {
		Alias
	}{
		Alias: Alias(m),
	})
}
