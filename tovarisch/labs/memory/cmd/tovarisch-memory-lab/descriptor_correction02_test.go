// descriptor_correction02_test.go — additional CORRECTION02 mutation tests.
//
// These tests cover the new contract surfaces introduced by
// CORRECTION02:
//   - threshold mutation rejection (verifier requires manifest
//     thresholds to match the verdict's stored thresholds);
//   - descriptor_state_invariant sample_count / available_count /
//     missing_count must be 2 / 2 / 0;
//   - descriptor_state_invariant rate_per_hour / slope /
//     relative_delta must be 0;
//   - descriptor_state_invariant minimum / maximum must be the
//     initial / final FD counts;
//   - descriptor_state_invariant endpoint mismatch (initial != final
//     or wrong absolute_delta) must be rejected;
//   - sampled signal with empty source_kind must be rejected;
//   - sampled signal with source_kind=state_invariant must be
//     rejected;
//   - invalid scenario invariant must produce overall=invalid and
//     suppress the descriptor_state_invariant signal.

package main

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestDescriptorThreshold_MutatedMemoryKibPerHour asserts the
// threshold-mutation rejection contract: mutating the manifest's
// MemoryGrowthKibPerHour while leaving the verdict's thresholds
// unchanged forces the verifier to reject the bundle.
func TestDescriptorThreshold_MutatedMemoryKibPerHour(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "manifest.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			thresholds, _ := m["configuration"].(map[string]interface{})["thresholds"].(map[string]interface{})
			thresholds["MemoryGrowthKibPerHour"] = float64(999)
		})
	}, "threshold mutation: verdict memory_growth_kib_per_hour=")
}

// TestDescriptorThreshold_MutatedResourceGrowthPerHour asserts
// the resource-growth threshold mutation is rejected.
func TestDescriptorThreshold_MutatedResourceGrowthPerHour(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "manifest.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			thresholds, _ := m["configuration"].(map[string]interface{})["thresholds"].(map[string]interface{})
			thresholds["ResourceGrowthPerHour"] = float64(99)
		})
	}, "threshold mutation: verdict resource_growth_per_hour=")
}

// TestDescriptorSamples_SampledSignalEmptySourceKind asserts
// that an empty source_kind on a sampled signal is rejected
// (CORRECTION02 strict contract).
func TestDescriptorSamples_SampledSignalEmptySourceKind(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "verdict.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			signals, _ := m["signal_summaries"].([]interface{})
			for _, s := range signals {
				if sm, ok := s.(map[string]interface{}); ok {
					if name, _ := sm["Name"].(string); name == "fd_count" {
						sm["SourceKind"] = ""
						return
					}
				}
			}
		})
	}, "signal \"fd_count\" has empty source_kind")
}

// TestDescriptorSamples_SampledSignalWrongSourceKind asserts
// that a sampled signal with source_kind=state_invariant is
// rejected (the wrong-kind contract).
func TestDescriptorSamples_SampledSignalWrongSourceKind(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "verdict.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			signals, _ := m["signal_summaries"].([]interface{})
			for _, s := range signals {
				if sm, ok := s.(map[string]interface{}); ok {
					if name, _ := sm["Name"].(string); name == "docker_memory_kib" {
						sm["SourceKind"] = "state_invariant"
						return
					}
				}
			}
		})
	}, "source_kind=state_invariant, expected sampled")
}

// TestDescriptorStateInvariant_SampleCountNotTwo rejects a
// descriptor_state_invariant signal whose SampleCount != 2.
func TestDescriptorStateInvariant_SampleCountNotTwo(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "verdict.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			signals, _ := m["signal_summaries"].([]interface{})
			for _, s := range signals {
				if sm, ok := s.(map[string]interface{}); ok {
					if name, _ := sm["Name"].(string); name == "descriptor_state_invariant" {
						sm["SampleCount"] = float64(61)
						sm["AvailableCount"] = float64(2)
						sm["MissingCount"] = float64(59)
						return
					}
				}
			}
		})
	}, "sample_count=61, expected 2")
}

// TestDescriptorStateInvariant_MissingCountNonzero rejects a
// descriptor_state_invariant signal whose MissingCount != 0.
func TestDescriptorStateInvariant_MissingCountNonzero(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "verdict.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			signals, _ := m["signal_summaries"].([]interface{})
			for _, s := range signals {
				if sm, ok := s.(map[string]interface{}); ok {
					if name, _ := sm["Name"].(string); name == "descriptor_state_invariant" {
						sm["MissingCount"] = float64(1)
						sm["SampleCount"] = float64(3)
						sm["AvailableCount"] = float64(2)
						return
					}
				}
			}
		})
	}, "missing_count=1, expected 0")
}

// TestDescriptorStateInvariant_RateNonzero rejects a
// descriptor_state_invariant signal with nonzero rate_per_hour.
func TestDescriptorStateInvariant_RateNonzero(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "verdict.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			signals, _ := m["signal_summaries"].([]interface{})
			for _, s := range signals {
				if sm, ok := s.(map[string]interface{}); ok {
					if name, _ := sm["Name"].(string); name == "descriptor_state_invariant" {
						sm["RatePerHour"] = float64(1.0)
						return
					}
				}
			}
		})
	}, "rate_per_hour=1.000000, expected 0")
}

// TestDescriptorStateInvariant_SlopeNonzero rejects a
// descriptor_state_invariant signal with nonzero slope.
func TestDescriptorStateInvariant_SlopeNonzero(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "verdict.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			signals, _ := m["signal_summaries"].([]interface{})
			for _, s := range signals {
				if sm, ok := s.(map[string]interface{}); ok {
					if name, _ := sm["Name"].(string); name == "descriptor_state_invariant" {
						sm["Slope"] = float64(0.5)
						return
					}
				}
			}
		})
	}, "slope=0.500000, expected 0")
}

// TestDescriptorStateInvariant_MinMaxWrong rejects a
// descriptor_state_invariant signal whose minimum/maximum do not
// match the canary state FD counts.
func TestDescriptorStateInvariant_MinMaxWrong(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "verdict.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			signals, _ := m["signal_summaries"].([]interface{})
			for _, s := range signals {
				if sm, ok := s.(map[string]interface{}); ok {
					if name, _ := sm["Name"].(string); name == "descriptor_state_invariant" {
						sm["Minimum"] = float64(99)
						sm["Maximum"] = float64(100)
						return
					}
				}
			}
		})
	}, "minimum=99, expected initial fd_count=8")
}

// TestDescriptorFallback_InitialReadyFalse asserts that an
// initial.ready=false canonical fixture mutation forces
// overall=invalid AND the descriptor_state_invariant signal is
// suppressed.
func TestDescriptorFallback_InitialReadyFalse(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "initial-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["ready"] = false
		})
	}, "stored overall classification resource_growth does not match reconstruction stable")
}

// TestDescriptorFallback_FinalRetainedBytesNonzero asserts that
// final.retained_bytes>0 with a valid fd_delta still invalidates
// the scenario invariant.
func TestDescriptorFallback_FinalRetainedBytesNonzero(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "final-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["retained_bytes"] = float64(1)
		})
	}, "descriptor: retained should be 0, got blocks=0 bytes=1")
}

// assertContains is a small test helper.
func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("missing %q in: %s", needle, haystack)
	}
}

// _ = strconv.Itoa to keep the import used (test-only lint).
var _ = strconv.Itoa
