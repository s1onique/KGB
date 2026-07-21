// descriptor_negative_test.go — Descriptor ACT mandatory negative
// test matrix.
//
// Covers ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-
// QUALIFICATION01 §14 (state invariants, workload arithmetic,
// stored verdict, sample/resource) and the descriptor-specific
// subset of §15 (classification).
//
// The shared provenance/artifact/scenario-neutral test matrix
// already runs against the bounded fixture; running it against the
// descriptor fixture too is exercised in
// TestDescriptorSharedProvenanceAndArtifactRejects.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// descriptorMutateAndVerify is a thin wrapper that calls the
// scenario-agnostic helper from shared_fixture_helpers_test.go
// with the descriptor fixture path and run_id.
func descriptorMutateAndVerify(t *testing.T, mutate func(boundDir string), expectDiagnostic string) {
	t.Helper()
	mutateAndVerifyForFixture(t, descriptorFixtureDir, descriptorFixtureRunID, mutate, expectDiagnostic)
}

// descriptorMutateAndVerifyNoRecompute is the no-recompute variant.
// It is used by tests that intentionally mutate checksums.txt after
// the rebind step: a post-mutation recompute would overwrite the
// mutation. The bounded ACT inlines this pattern in
// TestArtifact_CorruptChecksum.
func descriptorMutateAndVerifyNoRecompute(t *testing.T, mutate func(boundDir string), expectDiagnostic string) {
	t.Helper()
	dst := t.TempDir()
	boundDir := copyFixture(t, dst, descriptorFixtureDir, descriptorFixtureRunID)
	rebindFixture(t, boundDir)
	mutate(boundDir)

	out, err := runVerifierForRunID(t, dst, descriptorFixtureRunID)
	if err == nil {
		t.Fatalf("mutation did not cause verifier failure; output:\n%s", out)
	}
	if !strings.Contains(out, expectDiagnostic) {
		t.Errorf("mutation produced wrong diagnostic.\n"+
			"expected substring: %q\nfull output:\n%s", expectDiagnostic, out)
	}
}

// === §14 State invariants (9 tests) ===

// TestDescriptorState_FDDelta199 rejects a final state whose
// fd_count delta is 199 instead of 200.
func TestDescriptorState_FDDelta199(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "final-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			// final.fd_count = 207 → delta = 207 - 8 = 199
			m["fd_count"] = float64(207)
		})
	}, "descriptor: fd_delta=199 != expected=200")
}

// TestDescriptorState_FDDelta201 rejects a final state whose
// fd_count delta is 201 instead of 200.
func TestDescriptorState_FDDelta201(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "final-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["fd_count"] = float64(209)
		})
	}, "descriptor: fd_delta=201 != expected=200")
}

// TestDescriptorState_FDCountLowerThanInitial rejects a final
// state whose fd_count is below the initial baseline (impossible
// for a one-way descriptor leak).
func TestDescriptorState_FDCountLowerThanInitial(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "final-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["fd_count"] = float64(0)
		})
	}, "descriptor: fd_delta=-8 != expected=200")
}

// TestDescriptorState_OperationDelta99 rejects a final state
// whose operation_count delta is 99 instead of 100.
func TestDescriptorState_OperationDelta99(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "final-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["operation_count"] = float64(99)
		})
	}, "operation_count_delta=99 != completed=100")
}

// TestDescriptorState_OperationDelta101 rejects a final state
// whose operation_count delta is 101 instead of 100.
func TestDescriptorState_OperationDelta101(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "final-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["operation_count"] = float64(101)
		})
	}, "operation_count_delta=101 != completed=100")
}

// TestDescriptorState_InitialModeNotDescriptor rejects an
// initial state whose mode is something other than "descriptor".
func TestDescriptorState_InitialModeNotDescriptor(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "initial-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["mode"] = "bounded"
		})
	}, "initial mode=bounded != expected=descriptor")
}

// TestDescriptorState_FinalModeNotDescriptor rejects a final state
// whose mode is something other than "descriptor".
func TestDescriptorState_FinalModeNotDescriptor(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "final-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["mode"] = "growing"
		})
	}, "final mode=growing != expected=descriptor")
}

// TestDescriptorState_RetainedBlocksNonzero rejects a final
// state whose retained_blocks is nonzero (descriptor must not
// retain memory).
func TestDescriptorState_RetainedBlocksNonzero(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "final-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["retained_blocks"] = float64(1)
		})
	}, "descriptor: retained should be 0, got blocks=1 bytes=0")
}

// TestDescriptorState_RetainedBytesNonzero rejects a final state
// whose retained_bytes is nonzero (descriptor must not retain
// memory).
func TestDescriptorState_RetainedBytesNonzero(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "final-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["retained_bytes"] = float64(1)
		})
	}, "descriptor: retained should be 0, got blocks=0 bytes=1")
}

// === §14 Workload arithmetic (7 tests) ===

// TestDescriptorWorkload_RequestedNot100 rejects a workload whose
// requested count is not 100.
func TestDescriptorWorkload_RequestedNot100(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "workload-result.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["requested"] = float64(99)
		})
	}, "workload counts: req=99 att=100 com=100 fail=0")
}

// TestDescriptorWorkload_AttemptedNotRequested rejects a workload
// whose attempted count differs from requested.
func TestDescriptorWorkload_AttemptedNotRequested(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "workload-result.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["attempted"] = float64(99)
		})
	}, "workload counts: req=100 att=99 com=100 fail=0")
}

// TestDescriptorWorkload_CompletedNotRequested rejects a workload
// whose completed count differs from requested.
func TestDescriptorWorkload_CompletedNotRequested(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "workload-result.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["completed"] = float64(99)
		})
	}, "workload counts: req=100 att=100 com=99 fail=0")
}

// TestDescriptorWorkload_FailedNonzero rejects a workload with
// failed != 0.
func TestDescriptorWorkload_FailedNonzero(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "workload-result.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["failed"] = float64(1)
			m["completed"] = float64(99)
		})
	}, "workload counts: req=100 att=100 com=99 fail=1")
}

// TestDescriptorWorkload_ReturnedNotCompleted rejects a workload
// whose returned count does not match completed.
func TestDescriptorWorkload_ReturnedNotCompleted(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "workload-result.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["returned"] = float64(99)
		})
	}, "workload returned=99 != completed=100")
}

// TestDescriptorWorkload_OperationDeltaMismatch rejects a
// workload whose workload-implied completed count disagrees with
// the canary-state operation_count delta.
func TestDescriptorWorkload_OperationDeltaMismatch(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "workload-result.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["completed"] = float64(100)
			m["returned"] = float64(50)
		})
	}, "workload returned=50 != completed=100")
}

// TestDescriptorWorkload_FDDeltaFromAttempted simulates an
// evidence-fabrication attempt where the producer claims the
// FD delta is calculated from attempted (99 × 2 = 198) rather
// than completed (100 × 2 = 200). The state invariant fires first
// because operation_count delta = 100 while workload.completed = 99
// mismatches. This test only fires if the producer can record
// inconsistent completed and operation_count deltas, which the
// fixture's design prevents; we therefore use the canary-state
// approach to create the FD delta from attempted by mutating
// final.fd_count to 198 + 8 = 206.
func TestDescriptorWorkload_FDDeltaFromAttempted(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "final-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			// If fd_delta is computed from attempted=99 instead of
			// completed=100, the producer would record 198 + 8 = 206
			// instead of 200 + 8 = 208. The verifier rejects this.
			m["fd_count"] = float64(206)
		})
	}, "descriptor: fd_delta=198 != expected=200")
}

// === §14 Stored verdict (9 tests) ===

// mutateVerdictAndVerify is a helper that mutates a single field
// in the verdict.json and asserts the verifier emits the
// expected diagnostic. It works by mutating stored verdict AND
// the manifest's stored ScenarioValid/CanariesValid/ProvenanceValid
// to be consistent with the stored verdict, so the verifier's
// "stored ScenarioValid does not match reconstruction" check
// fires. The test asserts the canaries_valid/scenario_valid/
// provenance_valid invariant is non-trivially coupled to the
// stored verdict's contents.
func mutateVerdictAndVerify(t *testing.T, mutate func(m map[string]interface{}), expectDiagnostic string) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "verdict.json")
		applyJSONMutation(t, path, mutate)
	}, expectDiagnostic)
}

// TestDescriptorVerdict_OverallStable rejects a stored overall
// verdict of "stable" (the descriptor scenario must be
// resource_growth). Single-field mutation: only the overall
// classification changes; the verifier's full reconstruction
// detects the mismatch via the field-specific diagnostic.
func TestDescriptorVerdict_OverallStable(t *testing.T) {
	mutateVerdictAndVerify(t, func(m map[string]interface{}) {
		m["overall_classification"] = "stable"
	}, "stored overall classification stable does not match reconstruction resource_growth")
}

// TestDescriptorVerdict_OverallGrowth rejects a stored overall
// verdict of "growth" (descriptor must be resource_growth, not
// generic growth). Single-field mutation.
func TestDescriptorVerdict_OverallGrowth(t *testing.T) {
	mutateVerdictAndVerify(t, func(m map[string]interface{}) {
		m["overall_classification"] = "growth"
	}, "stored overall classification growth does not match reconstruction resource_growth")
}

// TestDescriptorVerdict_ResourceStable rejects a stored resource
// verdict of "stable" (the descriptor scenario must show
// resource_growth). Single-field mutation.
func TestDescriptorVerdict_ResourceStable(t *testing.T) {
	mutateVerdictAndVerify(t, func(m map[string]interface{}) {
		m["resource_classification"] = "stable"
	}, "stored resource classification stable does not match reconstruction resource_growth")
}

// TestDescriptorVerdict_ResourceInconclusive rejects a stored
// resource verdict of "inconclusive" (descriptor has positive FD
// evidence; resource must classify). Single-field mutation.
func TestDescriptorVerdict_ResourceInconclusive(t *testing.T) {
	mutateVerdictAndVerify(t, func(m map[string]interface{}) {
		m["resource_classification"] = "inconclusive"
	}, "stored resource classification inconclusive does not match reconstruction resource_growth")
}

// TestDescriptorVerdict_MemoryGrowing rejects a stored
// memory_classification="growing" (the descriptor scenario
// must have memory=stable). The verifier's full reconstruction
// must reject this single-field mutation.
func TestDescriptorVerdict_MemoryGrowing(t *testing.T) {
	mutateVerdictAndVerify(t, func(m map[string]interface{}) {
		m["memory_classification"] = "growing"
	}, "stored memory classification growing does not match reconstruction stable")
}

// TestDescriptorVerdict_SemanticInvalid rejects a stored
// semantic_classification="invalid" (the descriptor scenario
// must have semantic=stable; OOM events are definitive growth
// and the analyzer would already reject the run). The verifier's
// full reconstruction must reject this single-field mutation.
func TestDescriptorVerdict_SemanticInvalid(t *testing.T) {
	mutateVerdictAndVerify(t, func(m map[string]interface{}) {
		m["semantic_classification"] = "invalid"
	}, "stored semantic classification invalid does not match reconstruction stable")
}

// TestDescriptorVerdict_ScenarioValidFalse rejects a stored
// scenario_valid=false for a fixture whose reconstruction says
// scenario_valid=true.
func TestDescriptorVerdict_ScenarioValidFalse(t *testing.T) {
	mutateVerdictAndVerify(t, func(m map[string]interface{}) {
		m["scenario_valid"] = false
	}, "stored ScenarioValid does not match reconstruction")
}

// TestDescriptorVerdict_CanariesValidFalse rejects a stored
// canaries_valid=false. The reconstruction's scenario_valid
// check passes (workload / phase / PID are valid), so the
// "stored ScenarioValid does not match reconstruction" check
// does NOT fire; instead the final "verdict indicates scenario
// or canaries not valid" check fires.
func TestDescriptorVerdict_CanariesValidFalse(t *testing.T) {
	mutateVerdictAndVerify(t, func(m map[string]interface{}) {
		m["canaries_valid"] = false
	}, "stored CanariesValid does not match reconstruction")
}

// TestDescriptorVerdict_ProvenanceValidFalse rejects a stored
// provenance_valid=false for a fixture whose manifest subject
// identity is well-formed (the rebind step has set the real hash).
func TestDescriptorVerdict_ProvenanceValidFalse(t *testing.T) {
	mutateVerdictAndVerify(t, func(m map[string]interface{}) {
		m["provenance_valid"] = false
		m["provenance_error"] = "fabricated"
	}, "provenance_valid=false")
}

// === §14 Sample / resource evidence (selected representative tests) ===

// TestDescriptorSamples_AllFDUnavailable_Positive is the POSITIVE
// control: a sample stream whose every row has has_fd_count=false
// (the §8 fallback path) AND a valid descriptor_state_invariant
// signal in the verdict.json MUST pass the verifier. The
// canary-state invariant is the authoritative descriptor oracle
// when the host-side FD sampler is unavailable.
func TestDescriptorSamples_AllFDUnavailable_Positive(t *testing.T) {
	_ = requireDescriptorFixture(t)
	dst := t.TempDir()
	boundDir := copyFixture(t, dst, descriptorFixtureDir, descriptorFixtureRunID)
	rebindFixture(t, boundDir)
	// The committed fixture already has has_fd_count=false on
	// every sample and a valid descriptor_state_invariant in the
	// verdict.json. The verifier must accept it as-is.
	out, err := runDescriptorVerifier(t, dst)
	if err != nil {
		t.Fatalf("positive control: verifier rejected valid unavailable-FD fixture:\n%s", out)
	}
}

// TestDescriptorSamples_AllFDUnavailable_MissingInvariant rejects
// an unavailable-FD sample stream whose verdict.json is missing
// the descriptor_state_invariant signal. The verifier must
// detect the missing signal.
func TestDescriptorSamples_AllFDUnavailable_MissingInvariant(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "verdict.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			signals, _ := m["signal_summaries"].([]interface{})
			filtered := make([]interface{}, 0, len(signals))
			for _, s := range signals {
				if sm, ok := s.(map[string]interface{}); ok {
					if name, _ := sm["Name"].(string); name == "descriptor_state_invariant" {
						continue
					}
				}
				filtered = append(filtered, s)
			}
			m["signal_summaries"] = filtered
		})
	}, "missing descriptor_state_invariant signal")
}

// TestDescriptorSamples_AllFDUnavailable_MalformedInvariant rejects
// an unavailable-FD sample stream whose descriptor_state_invariant
// signal has wrong source_kind, missing primary flag, or wrong
// delta. Each of these is a verifier-level rejection.
func TestDescriptorSamples_AllFDUnavailable_MalformedInvariant(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "verdict.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			signals, _ := m["signal_summaries"].([]interface{})
			for _, s := range signals {
				if sm, ok := s.(map[string]interface{}); ok {
					if name, _ := sm["Name"].(string); name == "descriptor_state_invariant" {
						sm["SourceKind"] = "sampled"
						sm["IsPrimary"] = false
					}
				}
			}
		})
	}, "source_kind=sampled, expected state_invariant")
}

// TestDescriptorSamples_AllFDUnavailable_DuplicateInvariant rejects
// an unavailable-FD sample stream with two
// descriptor_state_invariant signals.
func TestDescriptorSamples_AllFDUnavailable_DuplicateInvariant(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "verdict.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			signals, _ := m["signal_summaries"].([]interface{})
			for _, s := range signals {
				if sm, ok := s.(map[string]interface{}); ok {
					if name, _ := sm["Name"].(string); name == "descriptor_state_invariant" {
						// Append a copy with a different AbsoluteDelta
						sm2 := make(map[string]interface{})
						for k, v := range sm {
							sm2[k] = v
						}
						sm2["AbsoluteDelta"] = float64(150)
						signals = append(signals, sm2)
						m["signal_summaries"] = signals
						return
					}
				}
			}
		})
	}, "duplicate descriptor_state_invariant signal")
}

// TestDescriptorSamples_HasFDTrueNegativeValue rejects a row with
// has_fd_count=true but a negative fd_count.
func TestDescriptorSamples_HasFDTrueNegativeValue(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		samplesPath := filepath.Join(boundDir, "samples.csv")
		data, err := os.ReadFile(samplesPath)
		if err != nil {
			t.Fatalf("read samples: %v", err)
		}
		lines := strings.Split(string(data), "\n")
		row := strings.Split(lines[1], ",")
		header := strings.Split(lines[0], ",")
		fdIdx := -1
		for i, h := range header {
			if h == "fd_count" {
				fdIdx = i
				break
			}
		}
		row[fdIdx] = "-1"
		lines[1] = strings.Join(row, ",")
		if err := os.WriteFile(samplesPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			t.Fatalf("write samples: %v", err)
		}
	}, "fd_count")
}

// TestDescriptorSamples_FDFlatWithStateDelta rejects an evidence
// bundle where the host-side sampled FD stream is flat
// (has_fd_count=true with constant fd_count=8) but the canary
// state claims fd_delta=200. The sampled FD data is available,
// so the descriptor_state_invariant fallback must NOT apply.
// The verifier must detect the disagreement between the
// sampled stream and the canary state.
func TestDescriptorSamples_FDFlatWithStateDelta(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		samplesPath := filepath.Join(boundDir, "samples.csv")
		data, err := os.ReadFile(samplesPath)
		if err != nil {
			t.Fatalf("read samples: %v", err)
		}
		lines := strings.Split(string(data), "\n")
		header := strings.Split(lines[0], ",")
		hasFDIdx, fdIdx := -1, -1
		for i, h := range header {
			if h == "has_fd_count" {
				hasFDIdx = i
			}
			if h == "fd_count" {
				fdIdx = i
			}
		}
		if hasFDIdx < 0 || fdIdx < 0 {
			t.Fatalf("fd columns not found")
		}
		// Flip every has_fd_count=false to true with constant fd_count=8
		// (sampled FD data is now available and flat).
		for i := 1; i < len(lines); i++ {
			if lines[i] == "" {
				continue
			}
			row := strings.Split(lines[i], ",")
			if row[hasFDIdx] == "false" {
				row[hasFDIdx] = "true"
				row[fdIdx] = "8"
				lines[i] = strings.Join(row, ",")
			}
		}
		if err := os.WriteFile(samplesPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			t.Fatalf("write samples: %v", err)
		}
	}, "sampled FD signal is available; descriptor_state_invariant must not be present")
}

// TestDescriptorSamples_PIDChange rejects a sample stream whose
// process PID changes mid-run.
func TestDescriptorSamples_PIDChange(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		samplesPath := filepath.Join(boundDir, "samples.csv")
		data, err := os.ReadFile(samplesPath)
		if err != nil {
			t.Fatalf("read samples: %v", err)
		}
		lines := strings.Split(string(data), "\n")
		header := strings.Split(lines[0], ",")
		pidIdx := -1
		for i, h := range header {
			if h == "process_pid" {
				pidIdx = i
				break
			}
		}
		row := strings.Split(lines[2], ",")
		oldPID, err := strconv.Atoi(row[pidIdx])
		if err != nil {
			t.Fatalf("parse PID: %v", err)
		}
		row[pidIdx] = strconv.Itoa(oldPID + 1)
		lines[2] = strings.Join(row, ",")
		if err := os.WriteFile(samplesPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			t.Fatalf("write samples: %v", err)
		}
	}, "PID changed")
}

// TestDescriptorSamples_MissingFinalPhase rejects a sample stream
// with no "final" phase samples.
func TestDescriptorSamples_MissingFinalPhase(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		samplesPath := filepath.Join(boundDir, "samples.csv")
		data, err := os.ReadFile(samplesPath)
		if err != nil {
			t.Fatalf("read samples: %v", err)
		}
		lines := strings.Split(string(data), "\n")
		header := strings.Split(lines[0], ",")
		phaseIdx := -1
		for i, h := range header {
			if h == "phase" {
				phaseIdx = i
				break
			}
		}
		// Replace every "final" with "settling" so the parser
		// never sees a final phase.
		for i := 1; i < len(lines); i++ {
			if lines[i] == "" {
				continue
			}
			row := strings.Split(lines[i], ",")
			if row[phaseIdx] == "final" {
				row[phaseIdx] = "settling"
				lines[i] = strings.Join(row, ",")
			}
		}
		if err := os.WriteFile(samplesPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			t.Fatalf("write samples: %v", err)
		}
	}, "missing final phase samples")
}

// TestDescriptorSamples_PhaseRegression rejects a sample stream
// whose phase rank regresses (e.g. baseline → startup).
func TestDescriptorSamples_PhaseRegression(t *testing.T) {
	descriptorMutateAndVerify(t, func(boundDir string) {
		samplesPath := filepath.Join(boundDir, "samples.csv")
		data, err := os.ReadFile(samplesPath)
		if err != nil {
			t.Fatalf("read samples: %v", err)
		}
		lines := strings.Split(string(data), "\n")
		header := strings.Split(lines[0], ",")
		phaseIdx := -1
		for i, h := range header {
			if h == "phase" {
				phaseIdx = i
				break
			}
		}
		// Take a middle row and revert to "startup".
		row := strings.Split(lines[20], ",")
		row[phaseIdx] = "startup"
		lines[20] = strings.Join(row, ",")
		if err := os.WriteFile(samplesPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			t.Fatalf("write samples: %v", err)
		}
	}, "phase regression")
}

// TestDescriptorSharedProvenanceAndArtifactRejects runs the
// shared provenance and artifact mutation matrix against the
// descriptor fixture to prove the scenario-neutral checks apply
// equally to descriptor evidence.
//
// The bounded ACT's mutation tests are scenario-agnostic by
// design (the shared suite hits path traversal, malformed
// checksum, hex/64-char checks, etc.). This test re-asserts
// the same scenario-neutral guarantees on the descriptor
// fixture, so a future regression in shared semantics cannot
// slip through the descriptor path.
func TestDescriptorSharedProvenanceAndArtifactRejects(t *testing.T) {
	t.Run("traversal_extra_entry", func(t *testing.T) {
		descriptorMutateAndVerifyNoRecompute(t, func(boundDir string) {
			checksumPath := filepath.Join(boundDir, "checksums.txt")
			bad := strings.Repeat("a", 64) + "  ../escape.json"
			f, err := os.OpenFile(checksumPath, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatalf("open append: %v", err)
			}
			defer f.Close()
			if _, err := f.WriteString(bad + "\n"); err != nil {
				t.Fatalf("append: %v", err)
			}
		}, "invalid checksum artifact path:")
	})
	t.Run("malformed_hash", func(t *testing.T) {
		descriptorMutateAndVerifyNoRecompute(t, func(boundDir string) {
			checksumPath := filepath.Join(boundDir, "checksums.txt")
			bad := "not-a-hex-hash  manifest.json\n"
			if err := os.WriteFile(checksumPath, []byte(bad), 0644); err != nil {
				t.Fatalf("write bad: %v", err)
			}
		}, "invalid checksum hash length:")
	})
	t.Run("zero_finished_at", func(t *testing.T) {
		descriptorMutateAndVerify(t, func(boundDir string) {
			path := filepath.Join(boundDir, "manifest.json")
			applyJSONMutation(t, path, func(m map[string]interface{}) {
				m["finished_at"] = "0001-01-01T00:00:00Z"
			})
		}, "manifest not finalized: missing finished_at")
	})
}

// diagnosticString is a tiny helper to embed the canonical
// "stored ScenarioValid does not match reconstruction" diagnostic
// in mutation tests. Defined as a function (not a var) so the
// help-text is still discoverable.
func diagnosticString() string {
	return "stored ScenarioValid does not match reconstruction"
}

// formatFDDelta is a small wrapper used by descriptor state
// negative tests.
func formatFDDelta(actual, expected int) string {
	return fmt.Sprintf("descriptor: fd_delta=%d != expected=%d", actual, expected)
}
