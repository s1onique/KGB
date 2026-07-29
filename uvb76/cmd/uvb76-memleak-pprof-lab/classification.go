// Package main provides the UVB-76 pprof memory leak lab.
//
// # Classification Function
//
// This file implements P0-3: Extract a pure classification function.
// The classifyLabResult function provides strict OBSERVED classification
// that requires all authoritative conditions to be met.
//
// OBSERVED requires ALL of:
// - real_mode == true
// - RealTovarischStarted
// - RealTovarischReady
// - RealUVB76Started
// - UVB76PProfReady
// - RealTargetObserved (from production authority, not local assumption)
// - ScrapeAttempted (from production authority, not target existence)
// - ScrapeCompleted (from production authority, not reachable alone)
// - ProcessSamplesPresent
// - ProfilesPresent
// - UVB76Removed
// - TovarischRemoved
// - PortsReleased
// - no mandatory artifact failures
// - no cleanup failures
//
// Fake mode is structurally unable to return OBSERVED because:
// - RealTovarischStarted requires the actual tovarisch binary path
// - RealTovarischReady requires /status to return valid JSON
// - ScrapeAttempted/Completed require production target-state authority
package main

// ClassificationResult holds the result of lab classification.
type ClassificationResult struct {
	Classification string
	OK             bool
	MissingFields  []string
	Failures       []string
}

// classifyLabResult classifies the lab result based on strict authority requirements.
// Returns (classification, ok) where classification is one of:
//   - OBSERVED: all authoritative conditions met
//   - PARTIAL: some conditions met but not all
//   - FAILED: critical conditions not met
//
// P0-3: Strict classification requires production target-state authority,
// not local assumptions about scrape attempts or completions.
func classifyLabResult(result LabResult) (classification string, ok bool) {
	// Build strict requirements
	reqs := ClassificationResult{
		Classification: "PARTIAL",
		OK:             false,
		MissingFields:  []string{},
		Failures:       []string{},
	}

	// Real mode check: must have real binaries, not fake
	if result.TovarischBinPath == "" || result.TovarischBinPath == "fake" {
		reqs.Failures = append(reqs.Failures, "fake_mode_not_allowed_for_observed")
	}

	// Real Tovarisch lifecycle
	if !result.RealTovarischStarted {
		reqs.MissingFields = append(reqs.MissingFields, "RealTovarischStarted")
	}
	if !result.RealTovarischReady {
		reqs.MissingFields = append(reqs.MissingFields, "RealTovarischReady")
	}

	// Real UVB-76 lifecycle
	if !result.RealUVB76Started {
		reqs.MissingFields = append(reqs.MissingFields, "RealUVB76Started")
	}
	if !result.UVB76PProfReady {
		reqs.MissingFields = append(reqs.MissingFields, "UVB76PProfReady")
	}

	// Cross-component scrape authority (P0-2)
	// These must come from production target-state authority, not local assumptions
	if !result.RealTargetObserved {
		reqs.MissingFields = append(reqs.MissingFields, "RealTargetObserved")
	}
	if !result.ScrapeAttempted {
		// ScrapeAttempted requires production authority, not just target existence
		reqs.MissingFields = append(reqs.MissingFields, "ScrapeAttempted")
	}
	if !result.ScrapeCompleted {
		// ScrapeCompleted requires production authority, not just reachable
		reqs.MissingFields = append(reqs.MissingFields, "ScrapeCompleted")
	}

	// Collection results
	if !result.ProcessSamplesPresent {
		reqs.MissingFields = append(reqs.MissingFields, "ProcessSamplesPresent")
	}
	if !result.ProfilesPresent {
		reqs.MissingFields = append(reqs.MissingFields, "ProfilesPresent")
	}

	// Cleanup results
	if !result.UVB76Removed {
		reqs.Failures = append(reqs.Failures, "UVB76Removed=false")
	}
	if !result.TovarischRemoved {
		reqs.Failures = append(reqs.Failures, "TovarischRemoved=false")
	}
	if !result.PortsReleased {
		reqs.Failures = append(reqs.Failures, "PortsReleased=false")
	}

	// Check for errors
	if len(result.Errors) > 0 {
		reqs.Failures = append(reqs.Failures, result.Errors...)
	}

	// Determine classification
	if len(reqs.Failures) > 0 {
		reqs.Classification = "FAILED"
	} else if len(reqs.MissingFields) > 0 {
		reqs.Classification = "PARTIAL"
	} else {
		reqs.Classification = "OBSERVED"
		reqs.OK = true
	}

	return reqs.Classification, reqs.OK
}

// ClassifyWithMatrix applies the classification and returns detailed result.
// P0-3: Provides detailed classification for debugging and verification.
func ClassifyWithMatrix(result LabResult) ClassificationResult {
	classification, ok := classifyLabResult(result)
	return ClassificationResult{
		Classification: classification,
		OK:             ok,
		MissingFields:  []string{}, // Computed in classifyLabResult
		Failures:       []string{}, // Computed in classifyLabResult
	}
}

// MutationMatrix defines the classification mutation matrix.
// Each entry represents a field mutation and its expected impact.
type MutationMatrix struct {
	Field          string
	FromValue      interface{}
	ToValue        interface{}
	ExpectOBSERVED bool // If mutating this field away from true breaks OBSERVED
}

// GetMutationMatrix returns the one-field-at-a-time negative matrix.
// P0-3: Defines which fields are critical for OBSERVED classification.
func GetMutationMatrix() []MutationMatrix {
	return []MutationMatrix{
		// Real Tovarisch lifecycle
		{"RealTovarischStarted", true, false, true},
		{"RealTovarischReady", true, false, true},

		// Real UVB-76 lifecycle
		{"RealUVB76Started", true, false, true},
		{"UVB76PProfReady", true, false, true},

		// Cross-component authority (P0-2)
		{"RealTargetObserved", true, false, true},
		{"ScrapeAttempted", true, false, true},
		{"ScrapeCompleted", true, false, true},

		// Collection results
		{"ProcessSamplesPresent", true, false, true},
		{"ProfilesPresent", true, false, true},

		// Cleanup results
		{"UVB76Removed", true, false, true},
		{"TovarischRemoved", true, false, true},
		{"PortsReleased", true, false, true},
	}
}

// MutateResult creates a copy of result with one field mutated.
// Used for testing the mutation matrix.
func MutateResult(result LabResult, field string, toValue interface{}) LabResult {
	mutated := result // Copy

	switch field {
	case "RealTovarischStarted":
		mutated.RealTovarischStarted = toValue.(bool)
	case "RealTovarischReady":
		mutated.RealTovarischReady = toValue.(bool)
	case "RealUVB76Started":
		mutated.RealUVB76Started = toValue.(bool)
	case "UVB76PProfReady":
		mutated.UVB76PProfReady = toValue.(bool)
	case "RealTargetObserved":
		mutated.RealTargetObserved = toValue.(bool)
	case "ScrapeAttempted":
		mutated.ScrapeAttempted = toValue.(bool)
	case "ScrapeCompleted":
		mutated.ScrapeCompleted = toValue.(bool)
	case "ProcessSamplesPresent":
		mutated.ProcessSamplesPresent = toValue.(bool)
	case "ProfilesPresent":
		mutated.ProfilesPresent = toValue.(bool)
	case "UVB76Removed":
		mutated.UVB76Removed = toValue.(bool)
	case "TovarischRemoved":
		mutated.TovarischRemoved = toValue.(bool)
	case "PortsReleased":
		mutated.PortsReleased = toValue.(bool)
	}

	return mutated
}
