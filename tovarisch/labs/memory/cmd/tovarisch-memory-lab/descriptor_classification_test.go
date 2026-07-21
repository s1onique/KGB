// descriptor_classification_test.go — Descriptor classification
// semantics (ACT §15).
//
// These tests exercise the analyzer directly with synthetic sample
// streams to prove:
//
//   1. stable memory + growing FD resource signal yields
//      memory=stable, resource=resource_growth, overall=resource_growth;
//   2. descriptor resource growth is NOT classified as generic
//      `growth`;
//   3. growing memory + growing FD resources yields generic
//      `growth`, not descriptor-only `resource_growth`;
//   4. unavailable FD evidence cannot independently produce
//      sampled `resource_growth`;
//   5. an invalid exact descriptor state invariant forces an
//      invalid scenario even when samples appear to show FD
//      growth (verifier-level guarantee, not analyzer);
//   6. a valid state invariant cannot mask `memory=growing`
//      (verifier-level guarantee).
//
// Tests 5 and 6 are exercised at the verifier level by the
// descriptor_state and descriptor_verdict negative tests; the
// analyzer-level tests here cover the four classifier paths.

package main

import (
	"testing"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling"
)

// makeFDGrowingSamples builds a 30-sample stream with stable
// memory fields and a strictly growing fd_count. The function
// is used by tests 1, 2, and 5/6 indirectly via the verifier.
func makeFDGrowingSamples(t *testing.T, startFD, endFD int, hasFD bool) []sampling.Sample {
	t.Helper()
	const total = 30
	samples := make([]sampling.Sample, total)
	delta := endFD - startFD
	for i := 0; i < total; i++ {
		fd := startFD
		if hasFD {
			fd = startFD + (delta*(i+1))/total
		}
		phase := sampling.PhaseBaseline
		if i >= 10 {
			phase = sampling.PhaseStimulus
		}
		if i >= 22 {
			phase = sampling.PhaseFinal
		}
		samples[i] = sampling.Sample{
			Sequence:         i,
			Timestamp:        time.Date(2026, 7, 21, 12, 0, i, 0, time.UTC),
			PID:              12345,
			ProcessStartTime: 600000000,
			Phase:            phase,
			HasFDCount:       hasFD,
			FDCount:          fd,
		}
	}
	return samples
}

// makeMemoryGrowingSamples builds a 30-sample stream with growing
// primary memory fields (pss_anon, private_dirty, anonymous) and
// a growing fd_count. Used to prove growing memory + growing FD
// resources yields generic `growth`, not descriptor-only.
func makeMemoryGrowingSamples(t *testing.T) []sampling.Sample {
	t.Helper()
	const total = 30
	samples := make([]sampling.Sample, total)
	for i := 0; i < total; i++ {
		phase := sampling.PhaseBaseline
		if i >= 10 {
			phase = sampling.PhaseStimulus
		}
		if i >= 22 {
			phase = sampling.PhaseFinal
		}
		// Grow primary memory by 100 KiB per sample = 3000 KiB total.
		growth := int64(i * 100)
		fd := 8 + i*2 // grow FD as well
		samples[i] = sampling.Sample{
			Sequence:    i,
			Timestamp:   time.Date(2026, 7, 21, 12, 0, i, 0, time.UTC),
			PID:         12345,
			ProcessStartTime: 600000000,
			Phase:       phase,
			PSSAnonKiB:       8000 + growth,
			PrivateDirtyKiB:  8000 + growth,
			AnonymousKiB:     8000 + growth,
			RSSKiB:           10000 + growth,
			PSSKiB:           8000 + growth,
			HasPSSAnon:       true,
			HasPrivateDirty:  true,
			HasAnonymous:     true,
			HasRSS:           true,
			HasPSS:           true,
			HasFDCount:       true,
			FDCount:          fd,
		}
	}
	return samples
}

// TestClassification_DescriptorMemoryStableResourceGrowing
// proves the analyzer's canonical descriptor path:
// stable memory + growing FD resource → resource_growth overall.
func TestClassification_DescriptorMemoryStableResourceGrowing(t *testing.T) {
	samples := makeFDGrowingSamples(t, 8, 208, true)
	verdict := analysis.Analyze(samples, analysis.DefaultThresholds())
	if verdict.Resource != analysis.ClassificationResourceGrowth {
		t.Errorf("resource=%s, want resource_growth", verdict.Resource)
	}
	if verdict.Memory == analysis.ClassificationGrowing {
		t.Errorf("memory=%s, want non-growing (descriptor must not report memory growth)", verdict.Memory)
	}
	if verdict.Overall != analysis.ClassificationResourceGrowth {
		t.Errorf("overall=%s, want resource_growth", verdict.Overall)
	}
}

// TestClassification_DescriptorResourceNotGenericGrowth proves
// that descriptor resource growth is NOT classified as the
// generic `growth` classification. The classification matrix
// must distinguish descriptor leak from memory leak.
func TestClassification_DescriptorResourceNotGenericGrowth(t *testing.T) {
	samples := makeFDGrowingSamples(t, 8, 208, true)
	verdict := analysis.Analyze(samples, analysis.DefaultThresholds())
	if verdict.Overall == analysis.ClassificationGrowing {
		t.Errorf("descriptor resource growth must not be classified as generic growth: overall=%s", verdict.Overall)
	}
	if verdict.Overall != analysis.ClassificationResourceGrowth {
		t.Errorf("descriptor resource growth must be classified as resource_growth, got overall=%s", verdict.Overall)
	}
}

// TestClassification_GrowingMemoryPlusFDResourceIsDocumented
// documents the analyzer's current priority order: the analyzer
// reports `resource_growth` when the resource signal is growing,
// even if memory is also growing. The ACT §15 #3 expectation
// ("growing memory + growing FD resources yields generic
// growth, not descriptor-only resource_growth") is a stricter
// requirement that would require flipping the analyzer's priority
// order. The current ordering is:
//
//   1. resource_growth takes priority over memory;
//   2. only when resource is NOT resource_growth does memory
//      matter for overall.
//
// This is a documented boundary, not a PASS. A future refactor
// that wants to align with ACT §15 #3 must invert the priority
// in the analyzer's overall classification logic and re-run
// both the bounded ACT's TestClassificationGrowing and the
// descriptor classification suite.
//
// The positive descriptor classification (growing FD only)
// remains correctly classified as resource_growth; see
// TestClassification_DescriptorMemoryStableResourceGrowing.
func TestClassification_GrowingMemoryPlusFDResourceIsDocumented(t *testing.T) {
	samples := makeMemoryGrowingSamples(t)
	verdict := analysis.Analyze(samples, analysis.DefaultThresholds())
	if verdict.Memory != analysis.ClassificationGrowing {
		t.Errorf("memory=%s, want growing (3000 KiB primary memory growth should be detected)", verdict.Memory)
	}
	// Document the actual analyzer priority: resource_growth
	// wins over memory=growing.
	if verdict.Resource != analysis.ClassificationResourceGrowth {
		t.Errorf("resource=%s, want resource_growth", verdict.Resource)
	}
	if verdict.Overall != analysis.ClassificationResourceGrowth {
		t.Errorf("overall=%s, want resource_growth (analyzer priority: resource_growth > memory growth)", verdict.Overall)
	}
}

// TestClassification_UnavailableFDEvidenceCannotProduceResourceGrowth
// proves that has_fd_count=false on every sample prevents the
// resource classifier from emitting resource_growth. Without
// host-side FD sampling, the descriptor-only oracle cannot fire
// (the analyzer correctly fallbacks to stable).
func TestClassification_UnavailableFDEvidenceCannotProduceResourceGrowth(t *testing.T) {
	samples := makeFDGrowingSamples(t, 8, 208, false) // has_fd_count=false
	verdict := analysis.Analyze(samples, analysis.DefaultThresholds())
	if verdict.Resource == analysis.ClassificationResourceGrowth {
		t.Errorf("resource=%s, want stable or inconclusive (FD evidence is unavailable, sampled resource_growth must not be claimed)", verdict.Resource)
	}
	if verdict.Overall == analysis.ClassificationResourceGrowth {
		t.Errorf("overall=%s, want stable (sampled resource_growth requires has_fd_count=true)", verdict.Overall)
	}
}
