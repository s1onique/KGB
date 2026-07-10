// Package redact provides pure deterministic secret redaction for UVB-76 artifacts.
//
// This file defines typed detection results. Findings never contain secret values.
// Only safe metadata (rule ID, field path) is exposed in diagnostics.
package redact

// Finding represents a detected secret with its location and rule identity.
// This structure is used for diagnostic purposes and never exposes secret values.
type Finding struct {
	// RuleID is the canonical rule identifier from the registry.
	// Examples: "UVB76-SECRET-0001", "UVB76-SECRET-0072"
	RuleID string

	// FieldPath is the location of the finding within a structured document.
	// For JSON: uses dot notation and array indices (e.g., "user.password", "items[0].token")
	// For headers: uses the header name
	// For URLs: uses "query.param_name" notation
	FieldPath string
}

// Finding is safe for diagnostic output because it contains:
// - No secret values
// - No secret prefixes or suffixes
// - No secret lengths
// - No raw matching lines containing secrets
