package main

import (
	"errors"
	"testing"
	"time"
)

// defaultTestIdentity provides valid identity for tests.
func defaultTestIdentity() *runExecutionIdentity {
	return &runExecutionIdentity{
		RunID:            "test-run-123",
		SourceCommit:     "abc123",
		RunStartedAt:     time.Now(),
		ArtifactDir:      "/tmp/test-artifacts",
		TovarischBinPath: "/usr/local/bin/tovarisch",
		UVB76BinPath:     "/usr/local/bin/uvb76",
		Endpoints: RuntimeEndpoints{
			TovarischPort: "12345",
			UVB76Port:     "12346",
			PProfPort:     "12347",
		},
	}
}

// defaultTestOps provides minimal valid operations for tests.
func defaultTestOps() lifecycleFailureOps {
	return lifecycleFailureOps{
		Cleanup:             func() []error { return nil },
		ProcessGone:         func(pid int) (bool, error) { return true, nil },
		VerifyPortsReleased: func() error { return nil },
		RemoveStaleResult:   func(path string) error { return nil },
		PublishFailedResult: func(r *Result) error { return nil },
	}
}

// TestFinalizer_AlwaysFailed verifies that lifecycle failure always results in FAILED classification.
func TestFinalizer_AlwaysFailed(t *testing.T) {
	result := &LabResult{
		Classification: "PARTIAL",
		OK:             true,
	}

	input := lifecycleFailureInput{
		TovarischPID:      0,
		UVB76PID:          0,
		Identity:          defaultTestIdentity(),
		InitiatingFailure: errors.New("test failure"),
		LabResult:         result,
		Processes:         &failedRunProcesses{},
	}

	finalizeLifecycleFailureWithOps(input, defaultTestOps())

	if result.Classification != "FAILED" {
		t.Errorf("lifecycle failure must result in FAILED classification: got %q", result.Classification)
	}
}

// TestFinalizer_OKMustBeFalse verifies that OK is always false after lifecycle failure.
func TestFinalizer_OKMustBeFalse(t *testing.T) {
	for _, ok := range []bool{true, false} {
		result := &LabResult{OK: ok}
		input := lifecycleFailureInput{
			TovarischPID:      0,
			UVB76PID:          0,
			Identity:          defaultTestIdentity(),
			InitiatingFailure: errors.New("test failure"),
			LabResult:         result,
			Processes:         &failedRunProcesses{},
		}
		finalizeLifecycleFailureWithOps(input, defaultTestOps())

		if result.OK != false {
			t.Errorf("OK should be false after lifecycle failure, was %v", ok)
		}
	}
}

// TestFinalizer_NoPartialsOrObserved verifies that lifecycle failure cannot result in non-FAILED classification.
func TestFinalizer_NoPartialsOrObserved(t *testing.T) {
	forbidden := []string{"OBSERVED", "PARTIAL", "STABLE", "GROWING", "LEAK", "BOUNDED"}

	for _, cls := range forbidden {
		result := &LabResult{Classification: cls, OK: true}
		input := lifecycleFailureInput{
			TovarischPID:      0,
			UVB76PID:          0,
			Identity:          defaultTestIdentity(),
			InitiatingFailure: errors.New("test failure"),
			LabResult:         result,
			Processes:         &failedRunProcesses{},
		}
		finalizeLifecycleFailureWithOps(input, defaultTestOps())

		if result.Classification == cls {
			t.Errorf("lifecycle failure must not result in %q classification", cls)
		}
	}
}

// TestFinalizer_NilDependencyMatrix verifies no side effects before validation.
func TestFinalizer_NilDependencyMatrix(t *testing.T) {
	tests := []struct {
		name    string
		ops     lifecycleFailureOps
		wantErr bool
		errMsg  string
	}{
		{
			name: "nilCleanup",
			ops: lifecycleFailureOps{
				Cleanup:             nil,
				ProcessGone:         func(pid int) (bool, error) { return true, nil },
				VerifyPortsReleased: func() error { return nil },
				RemoveStaleResult:   func(path string) error { return nil },
				PublishFailedResult: func(r *Result) error { return nil },
			},
			wantErr: true,
			errMsg:  "Cleanup",
		},
		{
			name: "nilProcessGone",
			ops: lifecycleFailureOps{
				Cleanup:             func() []error { return nil },
				ProcessGone:         nil,
				VerifyPortsReleased: func() error { return nil },
				RemoveStaleResult:   func(path string) error { return nil },
				PublishFailedResult: func(r *Result) error { return nil },
			},
			wantErr: true,
			errMsg:  "ProcessGone",
		},
		{
			name: "nilVerifyPorts",
			ops: lifecycleFailureOps{
				Cleanup:             func() []error { return nil },
				ProcessGone:         func(pid int) (bool, error) { return true, nil },
				VerifyPortsReleased: nil,
				RemoveStaleResult:   func(path string) error { return nil },
				PublishFailedResult: func(r *Result) error { return nil },
			},
			wantErr: true,
			errMsg:  "VerifyPortsReleased",
		},
		{
			name: "nilRemoveStaleResult",
			ops: lifecycleFailureOps{
				Cleanup:             func() []error { return nil },
				ProcessGone:         func(pid int) (bool, error) { return true, nil },
				VerifyPortsReleased: func() error { return nil },
				RemoveStaleResult:   nil,
				PublishFailedResult: func(r *Result) error { return nil },
			},
			wantErr: true,
			errMsg:  "RemoveStaleResult",
		},
		{
			name: "nilPublishFailedResult",
			ops: lifecycleFailureOps{
				Cleanup:             func() []error { return nil },
				ProcessGone:         func(pid int) (bool, error) { return true, nil },
				VerifyPortsReleased: func() error { return nil },
				RemoveStaleResult:   func(path string) error { return nil },
				PublishFailedResult: nil,
			},
			wantErr: true,
			errMsg:  "PublishFailedResult",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := &LabResult{}
			input := lifecycleFailureInput{
				TovarischPID:      0,
				UVB76PID:          0,
				Identity:          defaultTestIdentity(),
				InitiatingFailure: errors.New("test failure"),
				LabResult:         result,
				Processes:         &failedRunProcesses{},
			}

			err := finalizeLifecycleFailureWithOps(input, tc.ops)

			if tc.wantErr && err == nil {
				t.Error("expected error for nil dependency")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.wantErr {
				if !errors.Is(err, ErrNilFinalizationDependency) {
					t.Errorf("expected ErrNilFinalizationDependency: %v", err)
				}
			}
		})
	}
}

// ErrTestInitiatingFailure is a sentinel error for testing ErrorJoin contract.
var ErrTestInitiatingFailure = errors.New("initiating failure")

// TestFinalizer_ErrorJoinContract verifies errors.Join-compatible return value.
func TestFinalizer_ErrorJoinContract(t *testing.T) {
	input := lifecycleFailureInput{
		TovarischPID:      12345,
		UVB76PID:          0,
		Identity:          defaultTestIdentity(),
		InitiatingFailure: ErrTestInitiatingFailure,
		LabResult:         &LabResult{},
		Processes:         &failedRunProcesses{},
	}

	ops := lifecycleFailureOps{
		Cleanup: func() []error {
			return []error{errors.New("cleanup error 1"), errors.New("cleanup error 2")}
		},
		ProcessGone: func(pid int) (bool, error) {
			if pid == 12345 {
				return false, nil // Process still present
			}
			return true, nil
		},
		VerifyPortsReleased: func() error { return errors.New("port release failed") },
		RemoveStaleResult:   func(path string) error { return nil },
		PublishFailedResult: func(r *Result) error { return nil },
	}

	err := finalizeLifecycleFailureWithOps(input, ops)

	// Verify errors.Join contract: all causes must be discoverable via errors.Is
	if !errors.Is(err, ErrTestInitiatingFailure) {
		t.Error("expected initiating failure to be discoverable via errors.Is")
	}
	if !errors.Is(err, ErrTovarischProcessResidual) {
		t.Error("expected ErrTovarischProcessResidual to be discoverable via errors.Is")
	}
	if !errors.Is(err, ErrPortReleaseUnproven) {
		t.Error("expected ErrPortReleaseUnproven to be discoverable via errors.Is")
	}
}

// TestFinalizer_InputValidationFailsClosed verifies fail-closed behavior for invalid input.
func TestFinalizer_InputValidationFailsClosed(t *testing.T) {
	tests := []struct {
		name  string
		input lifecycleFailureInput
	}{
		{
			name: "nilLabResult",
			input: lifecycleFailureInput{
				TovarischPID:      0,
				UVB76PID:          0,
				Identity:          defaultTestIdentity(),
				InitiatingFailure: errors.New("test"),
				LabResult:         nil,
				Processes:         &failedRunProcesses{},
			},
		},
		{
			name: "nilInitiatingFailure",
			input: lifecycleFailureInput{
				TovarischPID:      0,
				UVB76PID:          0,
				Identity:          defaultTestIdentity(),
				InitiatingFailure: nil,
				LabResult:         &LabResult{},
				Processes:         &failedRunProcesses{},
			},
		},
		{
			name: "nilIdentity",
			input: lifecycleFailureInput{
				TovarischPID:      0,
				UVB76PID:          0,
				Identity:          nil,
				InitiatingFailure: errors.New("test"),
				LabResult:         &LabResult{},
				Processes:         &failedRunProcesses{},
			},
		},
		{
			name: "nilProcesses",
			input: lifecycleFailureInput{
				TovarischPID:      0,
				UVB76PID:          0,
				Identity:          defaultTestIdentity(),
				InitiatingFailure: errors.New("test"),
				LabResult:         &LabResult{},
				Processes:         nil,
			},
		},
		{
			name: "emptyRunID",
			input: lifecycleFailureInput{
				TovarischPID: 0,
				UVB76PID:     0,
				Identity: &runExecutionIdentity{
					RunID:        "",
					SourceCommit: "abc123",
					RunStartedAt: time.Now(),
					ArtifactDir:  "/tmp",
					Endpoints: RuntimeEndpoints{
						TovarischPort: "12345",
						UVB76Port:     "12346",
						PProfPort:     "12347",
					},
				},
				InitiatingFailure: errors.New("test"),
				LabResult:         &LabResult{},
				Processes:         &failedRunProcesses{},
			},
		},
		{
			name: "emptySourceCommit",
			input: lifecycleFailureInput{
				TovarischPID: 0,
				UVB76PID:     0,
				Identity: &runExecutionIdentity{
					RunID:        "test-run",
					SourceCommit: "",
					RunStartedAt: time.Now(),
					ArtifactDir:  "/tmp",
					Endpoints: RuntimeEndpoints{
						TovarischPort: "12345",
						UVB76Port:     "12346",
						PProfPort:     "12347",
					},
				},
				InitiatingFailure: errors.New("test"),
				LabResult:         &LabResult{},
				Processes:         &failedRunProcesses{},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := finalizeLifecycleFailureWithOps(tc.input, defaultTestOps())
			if err == nil {
				t.Errorf("expected validation error for %s, got nil", tc.name)
			}
		})
	}
}

// TestFinalizer_CompleteCleanupVerification verifies complete cleanup verification.
func TestFinalizer_CompleteCleanupVerification(t *testing.T) {
	tests := []struct {
		name          string
		tovarischGone bool
		uvb76Gone     bool
		portsReleased bool
		wantSuccess   bool
	}{
		{"allClean", true, true, true, true},
		{"tovarischResidual", false, true, true, false},
		{"uvb76Residual", true, false, true, false},
		{"portsLeaked", true, true, false, false},
		{"allFailed", false, false, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tovarischPID := 12345
			uvb76PID := 67890

			result := &LabResult{}
			input := lifecycleFailureInput{
				TovarischPID:      tovarischPID,
				UVB76PID:          uvb76PID,
				Identity:          defaultTestIdentity(),
				InitiatingFailure: errors.New("test"),
				LabResult:         result,
				Processes:         &failedRunProcesses{},
			}

			ops := lifecycleFailureOps{
				Cleanup: func() []error { return nil },
				ProcessGone: func(pid int) (bool, error) {
					if pid == tovarischPID {
						return tc.tovarischGone, nil
					}
					if pid == uvb76PID {
						return tc.uvb76Gone, nil
					}
					return true, nil
				},
				VerifyPortsReleased: func() error {
					if tc.portsReleased {
						return nil
					}
					return errors.New("ports not released")
				},
				RemoveStaleResult:   func(path string) error { return nil },
				PublishFailedResult: func(r *Result) error { return nil },
			}

			finalizeLifecycleFailureWithOps(input, ops)

			if result.TovarischRemoved != tc.tovarischGone {
				t.Errorf("TovarischRemoved: want %v, got %v", tc.tovarischGone, result.TovarischRemoved)
			}
			if result.UVB76Removed != tc.uvb76Gone {
				t.Errorf("UVB76Removed: want %v, got %v", tc.uvb76Gone, result.UVB76Removed)
			}
			if result.PortsReleased != tc.portsReleased {
				t.Errorf("PortsReleased: want %v, got %v", tc.portsReleased, result.PortsReleased)
			}
		})
	}
}

// TestValidateRunExecutionIdentity tests pre-start identity validation.
func TestValidateRunExecutionIdentity(t *testing.T) {
	validIdentity := &runExecutionIdentity{
		RunID:            "test-run-123",
		SourceCommit:     "abc123",
		RunStartedAt:     time.Now(),
		ArtifactDir:      "/tmp/test-artifacts",
		TovarischBinPath: "/usr/local/bin/tovarisch",
		UVB76BinPath:     "/usr/local/bin/uvb76",
		Endpoints: RuntimeEndpoints{
			TovarischPort: "12345",
			UVB76Port:     "12346",
			PProfPort:     "12347",
		},
	}

	// Valid identity should have no errors (real mode)
	errs := validateRunExecutionIdentity(validIdentity, false)
	if len(errs) > 0 {
		t.Errorf("valid identity should have no errors: %v", errs)
	}

	// Test nil identity
	errs = validateRunExecutionIdentity(nil, false)
	if len(errs) == 0 || !errors.Is(errs[0], ErrNilIdentity) {
		t.Error("nil identity should return ErrNilIdentity")
	}

	// Test empty RunID
	emptyID := *validIdentity
	emptyID.RunID = ""
	errs = validateRunExecutionIdentity(&emptyID, false)
	if len(errs) == 0 || !errors.Is(errs[0], ErrEmptyRunID) {
		t.Error("empty RunID should return ErrEmptyRunID")
	}

	// Test empty SourceCommit
	emptyCommit := *validIdentity
	emptyCommit.SourceCommit = ""
	errs = validateRunExecutionIdentity(&emptyCommit, false)
	if len(errs) == 0 || !errors.Is(errs[0], ErrEmptySourceCommit) {
		t.Error("empty SourceCommit should return ErrEmptySourceCommit")
	}

	// Test zero RunStartedAt
	zeroTime := *validIdentity
	zeroTime.RunStartedAt = time.Time{}
	errs = validateRunExecutionIdentity(&zeroTime, false)
	if len(errs) == 0 || !errors.Is(errs[0], ErrEmptyRunStartedAt) {
		t.Error("zero RunStartedAt should return ErrEmptyRunStartedAt")
	}

	// Test empty ArtifactDir
	emptyDir := *validIdentity
	emptyDir.ArtifactDir = ""
	errs = validateRunExecutionIdentity(&emptyDir, false)
	if len(errs) == 0 || !errors.Is(errs[0], ErrEmptyArtifactDir) {
		t.Error("empty ArtifactDir should return ErrEmptyArtifactDir")
	}

	// Test empty TovarischPort
	emptyTPort := *validIdentity
	emptyTPort.Endpoints.TovarischPort = ""
	errs = validateRunExecutionIdentity(&emptyTPort, false)
	if len(errs) == 0 {
		t.Error("empty TovarischPort should return error")
	}

	// Test invalid port number
	invalidPort := *validIdentity
	invalidPort.Endpoints.TovarischPort = "99999"
	errs = validateRunExecutionIdentity(&invalidPort, false)
	if len(errs) == 0 {
		t.Error("port > 65535 should return error")
	}
}

// TestIsValidPort tests port validation.
func TestIsValidPort(t *testing.T) {
	tests := []struct {
		port  string
		valid bool
	}{
		{"", false},
		{"0", false},
		{"1", true},
		{"80", true},
		{"8080", true},
		{"65535", true},
		{"65536", false},
		{"-1", false},
		{"abc", false},
		// strconv.ParseUint rejects non-numeric characters
		{"80a", false},
		{" 80", false},
		{"80 ", false},
		{"+80", false},
		{"080", true}, // Leading zeros are valid decimal
	}

	for _, tc := range tests {
		t.Run(tc.port, func(t *testing.T) {
			if got := isValidPort(tc.port); got != tc.valid {
				t.Errorf("isValidPort(%q) = %v, want %v", tc.port, got, tc.valid)
			}
		})
	}
}

// TestValidateSourceIdentity_DistinguishMissingVsEmpty tests P0-6.
func TestValidateSourceIdentity_DistinguishMissingVsEmpty(t *testing.T) {
	// Missing revision
	missingResolver := &fakeSourceIdentityResolver{
		err: ErrMissingVCSRevision,
	}
	err := ValidateSourceIdentity(missingResolver)
	if !errors.Is(err, ErrMissingVCSRevision) {
		t.Errorf("expected ErrMissingVCSRevision, got: %v", err)
	}

	// Empty revision
	emptyResolver := &fakeSourceIdentityResolver{
		err: ErrEmptyVCSRevision,
	}
	err = ValidateSourceIdentity(emptyResolver)
	if !errors.Is(err, ErrEmptyVCSRevision) {
		t.Errorf("expected ErrEmptyVCSRevision, got: %v", err)
	}

	// The two errors must be distinct
	if errors.Is(ErrMissingVCSRevision, ErrEmptyVCSRevision) {
		t.Error("ErrMissingVCSRevision and ErrEmptyVCSRevision should be distinct")
	}
}

// TestValidateRunExecutionIdentity_FakeMode tests that fake mode skips Tovarisch binary validation.
func TestValidateRunExecutionIdentity_FakeMode(t *testing.T) {
	// Fake mode identity with empty TovarischBinPath should be valid
	fakeModeIdentity := &runExecutionIdentity{
		RunID:            "test-run-fake",
		SourceCommit:     "abc123",
		RunStartedAt:     time.Now(),
		ArtifactDir:      "/tmp/test-artifacts",
		TovarischBinPath: "", // Empty is OK in fake mode
		UVB76BinPath:     "/usr/local/bin/uvb76",
		Endpoints: RuntimeEndpoints{
			TovarischPort: "12345",
			UVB76Port:     "12346",
			PProfPort:     "12347",
		},
	}

	// Valid in fake mode (second param = true)
	errs := validateRunExecutionIdentity(fakeModeIdentity, true)
	if len(errs) > 0 {
		t.Errorf("fake mode identity should have no errors: %v", errs)
	}

	// Same identity should fail in real mode (second param = false)
	errs = validateRunExecutionIdentity(fakeModeIdentity, false)
	if len(errs) == 0 {
		t.Error("real mode should reject empty TovarischBinPath")
	}

	// Empty UVB-76 bin path should always fail
	fakeModeNoUVB76 := *fakeModeIdentity
	fakeModeNoUVB76.UVB76BinPath = ""
	errs = validateRunExecutionIdentity(&fakeModeNoUVB76, true)
	if len(errs) == 0 {
		t.Error("UVB76BinPath should always be required")
	}
}
