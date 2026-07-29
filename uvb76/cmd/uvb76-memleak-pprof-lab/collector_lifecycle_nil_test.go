package main

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

// TestCollectAndSnapshot_NilDependencyValidation proves the helper:
// 1. Fails closed on nil dependencies (returns error, doesn't panic)
// 2. Uses errors.Is for stable machine classification with contextual wrapping
// 3. Validates dependencies BEFORE invoking cancel (no side effects on failure)
// 4. Returns empty snapshot on failure
// 5. Does not mutate input slices
func TestCollectAndSnapshot_NilDependencyValidation(t *testing.T) {
	var mu sync.Mutex
	validInput := &CollectorInput{
		TovarischSamples: &[]ProcessSample{},
		UVB76Samples:     &[]ProcessSample{},
		CollectorErrors:  &[]string{},
		SamplesMu:        &mu,
	}

	// Track cancel calls for all rows
	cancelCalls := 0
	trackedCancel := func() { cancelCalls++ }

	tests := []struct {
		name      string
		cancel    func()
		wg        *sync.WaitGroup
		input     *CollectorInput
		wantErr   bool
		wantField string // Expected nil field name in error
	}{
		{
			name:      "nil_cancelFn",
			cancel:    nil,
			wg:        &sync.WaitGroup{},
			input:     validInput,
			wantErr:   true,
			wantField: "cancelFn",
		},
		{
			name:      "nil_WaitGroup",
			cancel:    trackedCancel,
			wg:        nil,
			input:     validInput,
			wantErr:   true,
			wantField: "WaitGroup",
		},
		{
			name:      "nil_CollectorInput",
			cancel:    trackedCancel,
			wg:        &sync.WaitGroup{},
			input:     nil,
			wantErr:   true,
			wantField: "CollectorInput",
		},
		{
			name:      "nil_SamplesMu",
			cancel:    trackedCancel,
			wg:        &sync.WaitGroup{},
			input:     &CollectorInput{TovarischSamples: &[]ProcessSample{}, UVB76Samples: &[]ProcessSample{}, CollectorErrors: &[]string{}, SamplesMu: nil},
			wantErr:   true,
			wantField: "SamplesMu",
		},
		{
			name:      "nil_TovarischSamples",
			cancel:    trackedCancel,
			wg:        &sync.WaitGroup{},
			input:     &CollectorInput{TovarischSamples: nil, UVB76Samples: &[]ProcessSample{}, CollectorErrors: &[]string{}, SamplesMu: &mu},
			wantErr:   true,
			wantField: "TovarischSamples",
		},
		{
			name:      "nil_UVB76Samples",
			cancel:    trackedCancel,
			wg:        &sync.WaitGroup{},
			input:     &CollectorInput{TovarischSamples: &[]ProcessSample{}, UVB76Samples: nil, CollectorErrors: &[]string{}, SamplesMu: &mu},
			wantErr:   true,
			wantField: "UVB76Samples",
		},
		{
			name:      "nil_CollectorErrors",
			cancel:    trackedCancel,
			wg:        &sync.WaitGroup{},
			input:     &CollectorInput{TovarischSamples: &[]ProcessSample{}, UVB76Samples: &[]ProcessSample{}, CollectorErrors: nil, SamplesMu: &mu},
			wantErr:   true,
			wantField: "CollectorErrors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset cancel counter for each test
			cancelCalls = 0

			// Track original lengths for mutation check
			var originalTovarischLen, originalUVB76Len, originalErrorsLen int
			if tt.input != nil {
				if tt.input.TovarischSamples != nil {
					originalTovarischLen = len(*tt.input.TovarischSamples)
				}
				if tt.input.UVB76Samples != nil {
					originalUVB76Len = len(*tt.input.UVB76Samples)
				}
				if tt.input.CollectorErrors != nil {
					originalErrorsLen = len(*tt.input.CollectorErrors)
				}
			}

			snapshot, err := CollectAndSnapshot(tt.cancel, tt.wg, tt.input)

			// Verify 1: Error handling - must fail with error
			if (err != nil) != tt.wantErr {
				t.Errorf("CollectAndSnapshot() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify 2: errors.Is contract - must wrap ErrNilCollectorDependency
			if err != nil && !errors.Is(err, ErrNilCollectorDependency) {
				t.Errorf("expected ErrNilCollectorDependency via errors.Is, got %v", err)
			}

			// Verify 3: Contextual error message - must contain field name
			if err != nil && !strings.Contains(err.Error(), tt.wantField) {
				t.Errorf("error message should contain field name %q: got %v", tt.wantField, err)
			}

			// Verify 4: No side effects - cancel must NOT be called
			if tt.cancel != nil && cancelCalls != 0 {
				t.Errorf("cancel called %d times before dependency validation (expected 0)", cancelCalls)
			}

			// Verify 5: Empty snapshot on failure
			if err != nil {
				if len(snapshot.TovarischSamples) != 0 {
					t.Errorf("snapshot TovarischSamples not empty on failure: len=%d", len(snapshot.TovarischSamples))
				}
				if len(snapshot.UVB76Samples) != 0 {
					t.Errorf("snapshot UVB76Samples not empty on failure: len=%d", len(snapshot.UVB76Samples))
				}
				if len(snapshot.CollectorErrors) != 0 {
					t.Errorf("snapshot CollectorErrors not empty on failure: len=%d", len(snapshot.CollectorErrors))
				}
			}

			// Verify 6: Input mutation - check non-nil slices weren't mutated
			if tt.input != nil && err != nil {
				if tt.input.TovarischSamples != nil && len(*tt.input.TovarischSamples) != originalTovarischLen {
					t.Errorf("TovarischSamples mutated: before=%d, after=%d", originalTovarischLen, len(*tt.input.TovarischSamples))
				}
				if tt.input.UVB76Samples != nil && len(*tt.input.UVB76Samples) != originalUVB76Len {
					t.Errorf("UVB76Samples mutated: before=%d, after=%d", originalUVB76Len, len(*tt.input.UVB76Samples))
				}
				if tt.input.CollectorErrors != nil && len(*tt.input.CollectorErrors) != originalErrorsLen {
					t.Errorf("CollectorErrors mutated: before=%d, after=%d", originalErrorsLen, len(*tt.input.CollectorErrors))
				}
			}
		})
	}
}

// TestCollectAndSnapshot_AllNilFieldsDistinctErrors verifies each nil field
// returns a distinct, errors.Is-discoverable error.
func TestCollectAndSnapshot_AllNilFieldsDistinctErrors(t *testing.T) {
	var mu sync.Mutex

	tests := []struct {
		name      string
		cancel    func()
		wg        *sync.WaitGroup
		input     *CollectorInput
		wantField string
	}{
		{name: "cancelFn", cancel: nil, wg: &sync.WaitGroup{}, input: &CollectorInput{TovarischSamples: &[]ProcessSample{}, UVB76Samples: &[]ProcessSample{}, CollectorErrors: &[]string{}, SamplesMu: &mu}, wantField: "cancelFn"},
		{name: "WaitGroup", cancel: func() {}, wg: nil, input: &CollectorInput{TovarischSamples: &[]ProcessSample{}, UVB76Samples: &[]ProcessSample{}, CollectorErrors: &[]string{}, SamplesMu: &mu}, wantField: "WaitGroup"},
		{name: "CollectorInput", cancel: func() {}, wg: &sync.WaitGroup{}, input: nil, wantField: "CollectorInput"},
		{name: "SamplesMu", cancel: func() {}, wg: &sync.WaitGroup{}, input: &CollectorInput{TovarischSamples: &[]ProcessSample{}, UVB76Samples: &[]ProcessSample{}, CollectorErrors: &[]string{}, SamplesMu: nil}, wantField: "SamplesMu"},
		{name: "TovarischSamples", cancel: func() {}, wg: &sync.WaitGroup{}, input: &CollectorInput{TovarischSamples: nil, UVB76Samples: &[]ProcessSample{}, CollectorErrors: &[]string{}, SamplesMu: &mu}, wantField: "TovarischSamples"},
		{name: "UVB76Samples", cancel: func() {}, wg: &sync.WaitGroup{}, input: &CollectorInput{TovarischSamples: &[]ProcessSample{}, UVB76Samples: nil, CollectorErrors: &[]string{}, SamplesMu: &mu}, wantField: "UVB76Samples"},
		{name: "CollectorErrors", cancel: func() {}, wg: &sync.WaitGroup{}, input: &CollectorInput{TovarischSamples: &[]ProcessSample{}, UVB76Samples: &[]ProcessSample{}, CollectorErrors: nil, SamplesMu: &mu}, wantField: "CollectorErrors"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CollectAndSnapshot(tt.cancel, tt.wg, tt.input)

			if err == nil {
				t.Errorf("expected error for nil %s", tt.wantField)
				return
			}

			// Must be errors.Is-discoverable
			if !errors.Is(err, ErrNilCollectorDependency) {
				t.Errorf("error must wrap ErrNilCollectorDependency: got %v", err)
			}

			// Error message must contain field name
			if !strings.Contains(err.Error(), tt.wantField) {
				t.Errorf("error message should contain field name %q: got %v", tt.wantField, err)
			}
		})
	}
}

// TestCollectAndSnapshot_NoSideEffectsOnInvalidDependency proves the helper
// validates all dependencies before any side effects occur.
func TestCollectAndSnapshot_NoSideEffectsOnInvalidDependency(t *testing.T) {
	var mu sync.Mutex

	// Create input with pre-existing data
	tovarischSamples := []ProcessSample{{}, {}}
	uvb76Samples := []ProcessSample{{}}
	collectorErrors := []string{"pre-existing error"}

	input := &CollectorInput{
		TovarischSamples: &tovarischSamples,
		UVB76Samples:     &uvb76Samples,
		CollectorErrors:  &collectorErrors,
		SamplesMu:        &mu,
	}

	// Track cancel calls
	cancelCalls := 0
	trackedCancel := func() { cancelCalls++ }

	// Call with nil WaitGroup
	_, err := CollectAndSnapshot(trackedCancel, nil, input)

	// Should fail with sentinel error
	if !errors.Is(err, ErrNilCollectorDependency) {
		t.Errorf("expected ErrNilCollectorDependency, got %v", err)
	}

	// Cancel should NOT have been called
	if cancelCalls != 0 {
		t.Errorf("cancel called %d times (should be 0 before validation)", cancelCalls)
	}

	// Input slices should be unchanged
	if len(tovarischSamples) != 2 {
		t.Errorf("TovarischSamples mutated: expected 2, got %d", len(tovarischSamples))
	}
	if len(uvb76Samples) != 1 {
		t.Errorf("UVB76Samples mutated: expected 1, got %d", len(uvb76Samples))
	}
	if len(collectorErrors) != 1 {
		t.Errorf("CollectorErrors mutated: expected 1, got %d", len(collectorErrors))
	}
}
