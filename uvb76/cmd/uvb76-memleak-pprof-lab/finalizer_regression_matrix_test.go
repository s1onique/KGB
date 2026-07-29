package main

import (
	"errors"
	"testing"
)

// TestFinalizerRegression_CleanupCalledExactlyOnce verifies cleanup is called exactly once.
func TestFinalizerRegression_CleanupCalledExactlyOnce(t *testing.T) {
	cleanupCalls := 0

	input := lifecycleFailureInput{
		TovarischPID:      12345,
		UVB76PID:          67890,
		Identity:          defaultTestIdentity(),
		InitiatingFailure: errors.New("test failure"),
		LabResult:         &LabResult{},
		Processes:         &failedRunProcesses{},
	}

	ops := lifecycleFailureOps{
		Cleanup: func() []error {
			cleanupCalls++
			return nil
		},
		ProcessGone:         func(pid int) (bool, error) { return true, nil },
		VerifyPortsReleased: func() error { return nil },
		RemoveStaleResult:   func(path string) error { return nil },
		PublishFailedResult: func(r *Result) error { return nil },
	}

	finalizeLifecycleFailureWithOps(input, ops)

	if cleanupCalls != 1 {
		t.Errorf("cleanup should be called exactly once, got %d", cleanupCalls)
	}
}

// TestFinalizerRegression_ExactOperationOrder verifies the exact order of operations.
func TestFinalizerRegression_ExactOperationOrder(t *testing.T) {
	var order []string

	input := lifecycleFailureInput{
		TovarischPID:      12345,
		UVB76PID:          67890,
		Identity:          defaultTestIdentity(),
		InitiatingFailure: errors.New("test failure"),
		LabResult:         &LabResult{},
		Processes:         &failedRunProcesses{},
	}

	ops := lifecycleFailureOps{
		Cleanup: func() []error {
			order = append(order, "cleanup")
			return nil
		},
		ProcessGone: func(pid int) (bool, error) {
			order = append(order, "processGone")
			return true, nil
		},
		VerifyPortsReleased: func() error {
			order = append(order, "verifyPorts")
			return nil
		},
		RemoveStaleResult: func(path string) error {
			order = append(order, "removeStale")
			return nil
		},
		PublishFailedResult: func(r *Result) error {
			order = append(order, "publish")
			return nil
		},
	}

	finalizeLifecycleFailureWithOps(input, ops)

	// Verify exact order: cleanup -> processGone -> verifyPorts -> removeStale -> publish
	expected := []string{"cleanup", "processGone", "processGone", "verifyPorts", "removeStale", "publish"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d operations, got %d", len(expected), len(order))
	}
	for i, op := range expected {
		if order[i] != op {
			t.Errorf("operation %d: expected %q, got %q", i, op, order[i])
		}
	}
}

// TestFinalizerRegression_InitiatingFailureErrorsIs verifies initiating failure is discoverable via errors.Is.
func TestFinalizerRegression_InitiatingFailureErrorsIs(t *testing.T) {
	sentinelErr := errors.New("initiating failure sentinel")

	input := lifecycleFailureInput{
		TovarischPID:      0,
		UVB76PID:          0,
		Identity:          defaultTestIdentity(),
		InitiatingFailure: sentinelErr,
		LabResult:         &LabResult{},
		Processes:         &failedRunProcesses{},
	}

	ops := defaultTestOps()
	err := finalizeLifecycleFailureWithOps(input, ops)

	if !errors.Is(err, sentinelErr) {
		t.Error("initiating failure should be discoverable via errors.Is")
	}
}

// TestFinalizerRegression_CleanupFailureErrorsIs verifies cleanup errors are discoverable via errors.Is.
func TestFinalizerRegression_CleanupFailureErrorsIs(t *testing.T) {
	cleanupErr := errors.New("cleanup failure sentinel")

	input := lifecycleFailureInput{
		TovarischPID:      12345,
		UVB76PID:          0,
		Identity:          defaultTestIdentity(),
		InitiatingFailure: errors.New("test"),
		LabResult:         &LabResult{},
		Processes:         &failedRunProcesses{},
	}

	ops := lifecycleFailureOps{
		Cleanup: func() []error {
			return []error{cleanupErr}
		},
		ProcessGone:         func(pid int) (bool, error) { return true, nil },
		VerifyPortsReleased: func() error { return nil },
		RemoveStaleResult:   func(path string) error { return nil },
		PublishFailedResult: func(r *Result) error { return nil },
	}

	err := finalizeLifecycleFailureWithOps(input, ops)

	if !errors.Is(err, cleanupErr) {
		t.Error("cleanup error should be discoverable via errors.Is")
	}
}

// TestFinalizerRegression_BothProcessCausesPreserved verifies both process errors are preserved.
func TestFinalizerRegression_BothProcessCausesPreserved(t *testing.T) {
	input := lifecycleFailureInput{
		TovarischPID:      12345,
		UVB76PID:          67890,
		Identity:          defaultTestIdentity(),
		InitiatingFailure: errors.New("test"),
		LabResult:         &LabResult{},
		Processes:         &failedRunProcesses{},
	}

	ops := lifecycleFailureOps{
		Cleanup:             func() []error { return nil },
		ProcessGone:         func(pid int) (bool, error) { return false, nil }, // Both still present
		VerifyPortsReleased: func() error { return nil },
		RemoveStaleResult:   func(path string) error { return nil },
		PublishFailedResult: func(r *Result) error { return nil },
	}

	err := finalizeLifecycleFailureWithOps(input, ops)

	if !errors.Is(err, ErrTovarischProcessResidual) {
		t.Error("ErrTovarischProcessResidual should be discoverable")
	}
	if !errors.Is(err, ErrUVB76ProcessResidual) {
		t.Error("ErrUVB76ProcessResidual should be discoverable")
	}
}

// TestFinalizerRegression_ProcessCheckErrorPreserved verifies process check errors are preserved.
func TestFinalizerRegression_ProcessCheckErrorPreserved(t *testing.T) {
	checkErr := errors.New("process check error")

	input := lifecycleFailureInput{
		TovarischPID:      12345,
		UVB76PID:          0,
		Identity:          defaultTestIdentity(),
		InitiatingFailure: errors.New("test"),
		LabResult:         &LabResult{},
		Processes:         &failedRunProcesses{},
	}

	ops := lifecycleFailureOps{
		Cleanup: func() []error { return nil },
		ProcessGone: func(pid int) (bool, error) {
			if pid == 12345 {
				return false, checkErr // Error checking process
			}
			return true, nil
		},
		VerifyPortsReleased: func() error { return nil },
		RemoveStaleResult:   func(path string) error { return nil },
		PublishFailedResult: func(r *Result) error { return nil },
	}

	err := finalizeLifecycleFailureWithOps(input, ops)

	if !errors.Is(err, ErrTovarischProcessUnproven) {
		t.Error("ErrTovarischProcessUnproven should be discoverable")
	}
	if !errors.Is(err, checkErr) {
		t.Error("original check error should be discoverable")
	}
}

// TestFinalizerRegression_ProcessFailureDoesNotSkipPortCheck verifies port check still runs after process failure.
func TestFinalizerRegression_ProcessFailureDoesNotSkipPortCheck(t *testing.T) {
	portCheckRan := false

	input := lifecycleFailureInput{
		TovarischPID:      12345,
		UVB76PID:          0,
		Identity:          defaultTestIdentity(),
		InitiatingFailure: errors.New("test"),
		LabResult:         &LabResult{},
		Processes:         &failedRunProcesses{},
	}

	ops := lifecycleFailureOps{
		Cleanup: func() []error { return nil },
		ProcessGone: func(pid int) (bool, error) {
			return false, nil // Process still present
		},
		VerifyPortsReleased: func() error {
			portCheckRan = true
			return errors.New("port error")
		},
		RemoveStaleResult:   func(path string) error { return nil },
		PublishFailedResult: func(r *Result) error { return nil },
	}

	err := finalizeLifecycleFailureWithOps(input, ops)

	if !portCheckRan {
		t.Error("port check should run even after process failure")
	}
	if !errors.Is(err, ErrPortReleaseUnproven) {
		t.Error("ErrPortReleaseUnproven should be discoverable")
	}
}

// TestFinalizerRegression_StaleResultFailureBlocksPublication verifies stale result failure blocks publication.
func TestFinalizerRegression_StaleResultFailureBlocksPublication(t *testing.T) {
	publicationRan := false

	input := lifecycleFailureInput{
		TovarischPID:      0,
		UVB76PID:          0,
		Identity:          defaultTestIdentity(),
		InitiatingFailure: errors.New("test"),
		LabResult:         &LabResult{},
		Processes:         &failedRunProcesses{},
	}

	ops := lifecycleFailureOps{
		Cleanup:             func() []error { return nil },
		ProcessGone:         func(pid int) (bool, error) { return true, nil },
		VerifyPortsReleased: func() error { return nil },
		RemoveStaleResult: func(path string) error {
			return ErrResultStillPresent // Simulate failure to remove
		},
		PublishFailedResult: func(r *Result) error {
			publicationRan = true
			return nil
		},
	}

	finalizeLifecycleFailureWithOps(input, ops)

	if publicationRan {
		t.Error("publication should not run when stale result removal fails")
	}
}

// TestFinalizerRegression_PublicationFailurePreserved verifies publication failure is preserved.
func TestFinalizerRegression_PublicationFailurePreserved(t *testing.T) {
	pubErr := errors.New("publication error")

	input := lifecycleFailureInput{
		TovarischPID:      0,
		UVB76PID:          0,
		Identity:          defaultTestIdentity(),
		InitiatingFailure: errors.New("test"),
		LabResult:         &LabResult{},
		Processes:         &failedRunProcesses{},
	}

	ops := lifecycleFailureOps{
		Cleanup:             func() []error { return nil },
		ProcessGone:         func(pid int) (bool, error) { return true, nil },
		VerifyPortsReleased: func() error { return nil },
		RemoveStaleResult:   func(path string) error { return nil },
		PublishFailedResult: func(r *Result) error {
			return pubErr
		},
	}

	err := finalizeLifecycleFailureWithOps(input, ops)

	if !errors.Is(err, ErrPublicationFailed) {
		t.Error("ErrPublicationFailed should be discoverable")
	}
	if !errors.Is(err, pubErr) {
		t.Error("original publication error should be discoverable")
	}
}

// TestFinalizerRegression_ExactPIDForwarding verifies exact PID values are used.
func TestFinalizerRegression_ExactPIDForwarding(t *testing.T) {
	var tovarischPIDSeen, uvb76PIDSeen int

	input := lifecycleFailureInput{
		TovarischPID:      99999,
		UVB76PID:          88888,
		Identity:          defaultTestIdentity(),
		InitiatingFailure: errors.New("test"),
		LabResult:         &LabResult{},
		Processes:         &failedRunProcesses{},
	}

	ops := lifecycleFailureOps{
		Cleanup: func() []error { return nil },
		ProcessGone: func(pid int) (bool, error) {
			if pid == 99999 {
				tovarischPIDSeen = pid
			}
			if pid == 88888 {
				uvb76PIDSeen = pid
			}
			return true, nil
		},
		VerifyPortsReleased: func() error { return nil },
		RemoveStaleResult:   func(path string) error { return nil },
		PublishFailedResult: func(r *Result) error { return nil },
	}

	finalizeLifecycleFailureWithOps(input, ops)

	if tovarischPIDSeen != 99999 {
		t.Errorf("expected TovarischPID 99999, got %d", tovarischPIDSeen)
	}
	if uvb76PIDSeen != 88888 {
		t.Errorf("expected UVB76PID 88888, got %d", uvb76PIDSeen)
	}
}

// TestFinalizerRegression_IndependentContinuationAfterEarlierFailures verifies operations continue after failures.
func TestFinalizerRegression_IndependentContinuationAfterEarlierFailures(t *testing.T) {
	var operationsCompleted int

	input := lifecycleFailureInput{
		TovarischPID:      12345,
		UVB76PID:          0,
		Identity:          defaultTestIdentity(),
		InitiatingFailure: errors.New("test"),
		LabResult:         &LabResult{},
		Processes:         &failedRunProcesses{},
	}

	ops := lifecycleFailureOps{
		Cleanup: func() []error {
			return []error{errors.New("cleanup error")}
		},
		ProcessGone: func(pid int) (bool, error) {
			operationsCompleted++
			return true, nil
		},
		VerifyPortsReleased: func() error {
			operationsCompleted++
			return errors.New("port error")
		},
		RemoveStaleResult: func(path string) error {
			operationsCompleted++
			return nil
		},
		PublishFailedResult: func(r *Result) error {
			operationsCompleted++
			return nil
		},
	}

	finalizeLifecycleFailureWithOps(input, ops)

	// All operations should complete despite earlier failures
	// cleanup(1) + processGone(1) + verifyPorts(1) + removeStale(1) + publish(1) = 5
	if operationsCompleted < 4 {
		t.Errorf("expected at least 4 operations to complete, got %d", operationsCompleted)
	}
}
