// classifier_correction02_test.go — Unit tests for the CORRECTION02
// state-invariant contract.
//
// CORRECTION02 changes the descriptor_state_invariant signal:
//   - sample_count is exactly 2 (initial + final canary-state
//     observations), NOT the host-side sample count
//   - available_count == 2, missing_count == 0
//   - rate_per_hour == 0, slope == 0, relative_delta == 0
//   - minimum == initial.fd_count, maximum == final.fd_count
//
// The ApplyDescriptorStateInvariant gates are now sixteen (was
// five). An invalid state invariant OR any single mismatched gate
// must prevent the fallback from being applied.

package analysis

import (
	"strings"
	"testing"
)

// makeValidDescriptorInput builds a DescriptorFallbackInput that
// passes every CORRECTION02 gate. Tests mutate one field to drive
// each negative path.
func makeValidDescriptorInput() DescriptorFallbackInput {
	return DescriptorFallbackInput{
		Scenario:            "canary-descriptor",
		StateInvariantValid: true,
		Invariant: DescriptorStateInvariant{
			FDDelta:          200,
			ExpectedFDDelta:  200,
			OpDelta:          100,
			WorkloadComplete: 100,
			WorkloadFailed:   0,
			WorkloadReturned: 100,
		},
		Initial: DescriptorInitialState{
			FDCount:        8,
			OperationCount: 0,
			Mode:           "descriptor",
			Ready:          true,
		},
		Final: DescriptorFinalState{
			FDCount:        208,
			OperationCount: 100,
			Mode:           "descriptor",
			Ready:          true,
			RetainedBlocks: 0,
			RetainedBytes:  0,
		},
		Workload: DescriptorWorkloadResult{
			Requested: 100,
			Attempted: 100,
			Completed: 100,
			Failed:    0,
			Returned:  100,
		},
		SamplesAvailable: false,
		SamplesCount:     61,
	}
}

// TestApplyDescriptorStateInvariant_Positive asserts the
// canonical CORRECTION02 happy path: all sixteen gates pass and
// the signal is emitted with the exact canonical geometry.
func TestApplyDescriptorStateInvariant_Positive(t *testing.T) {
	res := ApplyDescriptorStateInvariant(makeValidDescriptorInput())
	if !res.Applied {
		t.Fatalf("expected Applied=true, got failures: %v", res.Failures)
	}
	sig := res.Signal
	if sig.Name != "descriptor_state_invariant" {
		t.Errorf("name=%q, want descriptor_state_invariant", sig.Name)
	}
	if sig.SourceKind != SignalKindStateInvariant {
		t.Errorf("source_kind=%s, want state_invariant", sig.SourceKind)
	}
	if sig.SampleCount != 2 {
		t.Errorf("sample_count=%d, want 2", sig.SampleCount)
	}
	if sig.AvailableCount != 2 {
		t.Errorf("available_count=%d, want 2", sig.AvailableCount)
	}
	if sig.MissingCount != 0 {
		t.Errorf("missing_count=%d, want 0", sig.MissingCount)
	}
	if sig.RatePerHour != 0 {
		t.Errorf("rate_per_hour=%f, want 0", sig.RatePerHour)
	}
	if sig.Slope != 0 {
		t.Errorf("slope=%f, want 0", sig.Slope)
	}
	if sig.RelativeDelta != 0 {
		t.Errorf("relative_delta=%f, want 0", sig.RelativeDelta)
	}
	if sig.FirstWindowMedian != 8 {
		t.Errorf("first_window_median=%d, want 8", sig.FirstWindowMedian)
	}
	if sig.LastWindowMedian != 208 {
		t.Errorf("last_window_median=%d, want 208", sig.LastWindowMedian)
	}
	if sig.AbsoluteDelta != 200 {
		t.Errorf("absolute_delta=%d, want 200", sig.AbsoluteDelta)
	}
	if sig.Minimum != 8 {
		t.Errorf("minimum=%d, want 8 (initial fd_count)", sig.Minimum)
	}
	if sig.Maximum != 208 {
		t.Errorf("maximum=%d, want 208 (final fd_count)", sig.Maximum)
	}
	if sig.Classification != ClassificationResourceGrowth {
		t.Errorf("classification=%s, want resource_growth", sig.Classification)
	}
	if !sig.IsPrimary {
		t.Errorf("is_primary=false, want true")
	}
}

// TestApplyDescriptorStateInvariant_RejectsInvalidInvariant
// asserts the gate-0 contract: an invalid scenario invariant
// cannot trigger the fallback. No descriptor_state_invariant
// signal is emitted.
func TestApplyDescriptorStateInvariant_RejectsInvalidInvariant(t *testing.T) {
	in := makeValidDescriptorInput()
	in.StateInvariantValid = false
	res := ApplyDescriptorStateInvariant(in)
	if res.Applied {
		t.Errorf("expected Applied=false when state_invariant_valid=false")
	}
	if len(res.Failures) == 0 {
		t.Errorf("expected failure diagnostic for invalid invariant")
	}
	if !strings.Contains(strings.Join(res.Failures, ";"), "state_invariant_valid=false") {
		t.Errorf("diagnostic should mention state_invariant_valid=false; got: %v", res.Failures)
	}
}

// TestApplyDescriptorStateInvariant_RejectsInitialReadyFalse
// asserts gate 11: initial.ready must be true.
func TestApplyDescriptorStateInvariant_RejectsInitialReadyFalse(t *testing.T) {
	in := makeValidDescriptorInput()
	in.Initial.Ready = false
	res := ApplyDescriptorStateInvariant(in)
	if res.Applied {
		t.Errorf("expected Applied=false when initial.ready=false")
	}
	if !strings.Contains(strings.Join(res.Failures, ";"), "initial.ready=false") {
		t.Errorf("diagnostic should mention initial.ready=false; got: %v", res.Failures)
	}
}

// TestApplyDescriptorStateInvariant_RejectsFinalReadyFalse
// asserts gate 12: final.ready must be true.
func TestApplyDescriptorStateInvariant_RejectsFinalReadyFalse(t *testing.T) {
	in := makeValidDescriptorInput()
	in.Final.Ready = false
	res := ApplyDescriptorStateInvariant(in)
	if res.Applied {
		t.Errorf("expected Applied=false when final.ready=false")
	}
	if !strings.Contains(strings.Join(res.Failures, ";"), "final.ready=false") {
		t.Errorf("diagnostic should mention final.ready=false; got: %v", res.Failures)
	}
}

// TestApplyDescriptorStateInvariant_RejectsIncorrectMode
// asserts gates 9-10: both initial and final mode must be
// "descriptor".
func TestApplyDescriptorStateInvariant_RejectsIncorrectMode(t *testing.T) {
	t.Run("initial_mode", func(t *testing.T) {
		in := makeValidDescriptorInput()
		in.Initial.Mode = "bounded"
		res := ApplyDescriptorStateInvariant(in)
		if res.Applied {
			t.Errorf("expected Applied=false when initial.mode=bounded")
		}
		if !strings.Contains(strings.Join(res.Failures, ";"), "initial.mode") {
			t.Errorf("diagnostic should mention initial.mode; got: %v", res.Failures)
		}
	})
	t.Run("final_mode", func(t *testing.T) {
		in := makeValidDescriptorInput()
		in.Final.Mode = "growing"
		res := ApplyDescriptorStateInvariant(in)
		if res.Applied {
			t.Errorf("expected Applied=false when final.mode=growing")
		}
		if !strings.Contains(strings.Join(res.Failures, ";"), "final.mode") {
			t.Errorf("diagnostic should mention final.mode; got: %v", res.Failures)
		}
	})
}

// TestApplyDescriptorStateInvariant_RejectsRetainedBlocks
// asserts gate 13: final.retained_blocks must be 0.
func TestApplyDescriptorStateInvariant_RejectsRetainedBlocks(t *testing.T) {
	in := makeValidDescriptorInput()
	in.Final.RetainedBlocks = 1
	res := ApplyDescriptorStateInvariant(in)
	if res.Applied {
		t.Errorf("expected Applied=false when final.retained_blocks=1")
	}
	if !strings.Contains(strings.Join(res.Failures, ";"), "retained_blocks") {
		t.Errorf("diagnostic should mention retained_blocks; got: %v", res.Failures)
	}
}

// TestApplyDescriptorStateInvariant_RejectsRetainedBytes
// asserts gate 14: final.retained_bytes must be 0.
func TestApplyDescriptorStateInvariant_RejectsRetainedBytes(t *testing.T) {
	in := makeValidDescriptorInput()
	in.Final.RetainedBytes = 1
	res := ApplyDescriptorStateInvariant(in)
	if res.Applied {
		t.Errorf("expected Applied=false when final.retained_bytes=1")
	}
	if !strings.Contains(strings.Join(res.Failures, ";"), "retained_bytes") {
		t.Errorf("diagnostic should mention retained_bytes; got: %v", res.Failures)
	}
}

// TestApplyDescriptorStateInvariant_RejectsAttemptedMismatch
// asserts gate 3: workload.attempted must equal workload.requested.
func TestApplyDescriptorStateInvariant_RejectsAttemptedMismatch(t *testing.T) {
	in := makeValidDescriptorInput()
	in.Workload.Attempted = 99
	res := ApplyDescriptorStateInvariant(in)
	if res.Applied {
		t.Errorf("expected Applied=false when workload.attempted=99")
	}
	if !strings.Contains(strings.Join(res.Failures, ";"), "workload.attempted") {
		t.Errorf("diagnostic should mention workload.attempted; got: %v", res.Failures)
	}
}

// TestApplyDescriptorStateInvariant_RejectsFailedNonzero
// asserts gate 5: workload.failed must be 0.
func TestApplyDescriptorStateInvariant_RejectsFailedNonzero(t *testing.T) {
	in := makeValidDescriptorInput()
	in.Workload.Failed = 1
	res := ApplyDescriptorStateInvariant(in)
	if res.Applied {
		t.Errorf("expected Applied=false when workload.failed=1")
	}
	if !strings.Contains(strings.Join(res.Failures, ";"), "workload.failed") {
		t.Errorf("diagnostic should mention workload.failed; got: %v", res.Failures)
	}
}

// TestApplyDescriptorStateInvariant_RejectsReturnedMismatch
// asserts gate 6: workload.returned must equal workload.completed.
func TestApplyDescriptorStateInvariant_RejectsReturnedMismatch(t *testing.T) {
	in := makeValidDescriptorInput()
	in.Workload.Returned = 99
	res := ApplyDescriptorStateInvariant(in)
	if res.Applied {
		t.Errorf("expected Applied=false when workload.returned=99")
	}
	if !strings.Contains(strings.Join(res.Failures, ";"), "workload.returned") {
		t.Errorf("diagnostic should mention workload.returned; got: %v", res.Failures)
	}
}

// TestApplyDescriptorStateInvariant_RejectsSampledFDAvailable
// asserts gate 15: sampled FD data must be unavailable.
func TestApplyDescriptorStateInvariant_RejectsSampledFDAvailable(t *testing.T) {
	in := makeValidDescriptorInput()
	in.SamplesAvailable = true
	res := ApplyDescriptorStateInvariant(in)
	if res.Applied {
		t.Errorf("expected Applied=false when sampled FD is available")
	}
	if !strings.Contains(strings.Join(res.Failures, ";"), "sampled FD signal is available") {
		t.Errorf("diagnostic should mention sampled FD availability; got: %v", res.Failures)
	}
}

// TestComputeOverallWithInvariant_InvalidInvariantBeatsMemory
// asserts the new priority: an invalid scenario invariant
// produces overall=invalid even when memory is growing.
func TestComputeOverallWithInvariant_InvalidInvariantBeatsMemory(t *testing.T) {
	got := ComputeOverallWithInvariant(ClassificationGrowing,
		ClassificationResourceGrowth, ClassificationStable, false)
	if got != ClassificationInvalid {
		t.Errorf("overall=%s, want invalid (invalid invariant must beat memory growing)", got)
	}
}

// TestComputeOverallWithInvariant_ValidInvariantMemoryGrowing
// asserts that a valid invariant does not mask memory growth.
func TestComputeOverallWithInvariant_ValidInvariantMemoryGrowing(t *testing.T) {
	got := ComputeOverallWithInvariant(ClassificationGrowing,
		ClassificationResourceGrowth, ClassificationStable, true)
	if got != ClassificationGrowing {
		t.Errorf("overall=%s, want growing (memory growth has priority)", got)
	}
}

// TestComputeOverallWithInvariant_DescriptorPath asserts the
// canonical descriptor scenario: stable memory, growing resource,
// valid invariant → overall=resource_growth.
func TestComputeOverallWithInvariant_DescriptorPath(t *testing.T) {
	got := ComputeOverallWithInvariant(ClassificationStable,
		ClassificationResourceGrowth, ClassificationStable, true)
	if got != ClassificationResourceGrowth {
		t.Errorf("overall=%s, want resource_growth", got)
	}
}
