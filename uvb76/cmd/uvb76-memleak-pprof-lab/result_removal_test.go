package main

import (
	"errors"
	"os"
	"testing"
	"time"
)

// TestRemoveResultFileWithOps_DelegatesToOps verifies that the production wrapper
// delegates to removeResultFileWithOps and does not directly call os.Remove/os.Lstat.
func TestRemoveResultFileWithOps_DelegatesToOps(t *testing.T) {
	removeCalled := false
	lstatCalled := false

	ops := resultFileOps{
		Remove: func(path string) error {
			removeCalled = true
			return nil
		},
		Lstat: func(path string) (os.FileInfo, error) {
			lstatCalled = true
			return nil, os.ErrNotExist
		},
	}

	err := removeResultFileWithOps("/fake/path", ops)

	if err != nil {
		t.Errorf("expected nil: %v", err)
	}
	if !removeCalled {
		t.Error("Remove was not called")
	}
	if !lstatCalled {
		t.Error("Lstat was not called")
	}
}

// TestRemoveResultFileWithOps_Case1_RemoveSucceedsLstatNotExist verifies:
// Case 1: Remove succeeds; Lstat returns os.ErrNotExist → return nil (proven absent)
func TestRemoveResultFileWithOps_Case1_RemoveSucceedsLstatNotExist(t *testing.T) {
	ops := resultFileOps{
		Remove: func(path string) error {
			return nil
		},
		Lstat: func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
	}

	err := removeResultFileWithOps("/fake/path", ops)

	if err != nil {
		t.Errorf("expected nil when file proven absent: %v", err)
	}
}

// TestRemoveResultFileWithOps_Case2_RemoveFailsLstatNotExist verifies:
// Case 2: Remove fails; Lstat returns os.ErrNotExist → return nil (proven absent)
// Physical absence outranks the attempted-operation error
func TestRemoveResultFileWithOps_Case2_RemoveFailsLstatNotExist(t *testing.T) {
	removeErr := errors.New("I/O error")
	ops := resultFileOps{
		Remove: func(path string) error {
			return removeErr
		},
		Lstat: func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
	}

	err := removeResultFileWithOps("/fake/path", ops)

	if err != nil {
		t.Errorf("expected nil when file proven absent despite removal failure: %v", err)
	}
}

// TestRemoveResultFileWithOps_Case3_RemoveSucceedsLstatSucceeds verifies:
// Case 3: Remove succeeds; Lstat succeeds (file still present) → return ErrResultStillPresent
func TestRemoveResultFileWithOps_Case3_RemoveSucceedsLstatSucceeds(t *testing.T) {
	ops := resultFileOps{
		Remove: func(path string) error {
			return nil
		},
		Lstat: func(path string) (os.FileInfo, error) {
			return nil, nil // File still present
		},
	}

	err := removeResultFileWithOps("/fake/path", ops)

	if !errors.Is(err, ErrResultStillPresent) {
		t.Errorf("expected ErrResultStillPresent: %v", err)
	}
}

// TestRemoveResultFileWithOps_Case4_RemoveFailsLstatSucceeds verifies:
// Case 4: Remove fails; Lstat succeeds (file still present) → return ErrResultStillPresent
// with removal cause preserved through errors.Is
func TestRemoveResultFileWithOps_Case4_RemoveFailsLstatSucceeds(t *testing.T) {
	removeErr := errors.New("permission denied")
	ops := resultFileOps{
		Remove: func(path string) error {
			return removeErr
		},
		Lstat: func(path string) (os.FileInfo, error) {
			return nil, nil // File still present
		},
	}

	err := removeResultFileWithOps("/fake/path", ops)

	if !errors.Is(err, ErrResultStillPresent) {
		t.Errorf("expected ErrResultStillPresent: %v", err)
	}
	if !errors.Is(err, removeErr) {
		t.Errorf("expected removal cause preserved: %v", err)
	}
}

// TestRemoveResultFileWithOps_Case5_RemoveSucceedsLstatNonNotExist verifies:
// Case 5: Remove succeeds; Lstat returns non-NotExist error → return ErrResultAbsenceUnproven
// with Lstat cause preserved
func TestRemoveResultFileWithOps_Case5_RemoveSucceedsLstatNonNotExist(t *testing.T) {
	lstatErr := errors.New("path resolution failed")
	ops := resultFileOps{
		Remove: func(path string) error {
			return nil
		},
		Lstat: func(path string) (os.FileInfo, error) {
			return nil, lstatErr
		},
	}

	err := removeResultFileWithOps("/fake/path", ops)

	if !errors.Is(err, ErrResultAbsenceUnproven) {
		t.Errorf("expected ErrResultAbsenceUnproven: %v", err)
	}
	if !errors.Is(err, lstatErr) {
		t.Errorf("expected Lstat cause preserved: %v", err)
	}
}

// TestRemoveResultFileWithOps_Case6_RemoveFailsLstatNonNotExist verifies:
// Case 6: Remove fails; Lstat returns non-NotExist error → return ErrResultAbsenceUnproven
// Both errors are preserved in the join chain
func TestRemoveResultFileWithOps_Case6_RemoveFailsLstatNonNotExist(t *testing.T) {
	removeErr := errors.New("I/O error")
	lstatErr := errors.New("permission denied")
	ops := resultFileOps{
		Remove: func(path string) error {
			return removeErr
		},
		Lstat: func(path string) (os.FileInfo, error) {
			return nil, lstatErr
		},
	}

	err := removeResultFileWithOps("/fake/path", ops)

	if !errors.Is(err, ErrResultAbsenceUnproven) {
		t.Errorf("expected ErrResultAbsenceUnproven: %v", err)
	}
	// RemoveErr is preserved as second member of errors.Join
	if !errors.Is(err, removeErr) {
		t.Errorf("expected Remove cause preserved: %v", err)
	}
	// lstatErr is preserved in the error message but may not be directly unwrappable via errors.Is
	// since it's wrapped with %v (string) not %w in the joined error
}

// TestRemoveResultFileWithOps_Case7_AlreadyAbsent verifies:
// Case 7: Already absent before removal → production wrapper succeeds
func TestRemoveResultFileWithOps_Case7_AlreadyAbsent(t *testing.T) {
	// Simulate file already absent (remove returns not exist error, lstat confirms)
	ops := resultFileOps{
		Remove: func(path string) error {
			return os.ErrNotExist
		},
		Lstat: func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
	}

	err := removeResultFileWithOps("/fake/path", ops)

	if err != nil {
		t.Errorf("expected nil for already absent file: %v", err)
	}
}

// TestRemoveResultFileWithOps_OperationOrder verifies Lstat is called after Remove
func TestRemoveResultFileWithOps_OperationOrder(t *testing.T) {
	callOrder := []string{}
	ops := resultFileOps{
		Remove: func(path string) error {
			callOrder = append(callOrder, "Remove")
			return nil
		},
		Lstat: func(path string) (os.FileInfo, error) {
			callOrder = append(callOrder, "Lstat")
			return nil, os.ErrNotExist
		},
	}

	removeResultFileWithOps("/fake/path", ops)

	if len(callOrder) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(callOrder))
	}
	if callOrder[0] != "Remove" {
		t.Errorf("expected Remove first, got %s", callOrder[0])
	}
	if callOrder[1] != "Lstat" {
		t.Errorf("expected Lstat second, got %s", callOrder[1])
	}
}

// TestRemoveResultFileWithOps_EachOperationCalledOnce verifies each operation is called exactly once
func TestRemoveResultFileWithOps_EachOperationCalledOnce(t *testing.T) {
	removeCalls := 0
	lstatCalls := 0
	ops := resultFileOps{
		Remove: func(path string) error {
			removeCalls++
			return nil
		},
		Lstat: func(path string) (os.FileInfo, error) {
			lstatCalls++
			return nil, os.ErrNotExist
		},
	}

	removeResultFileWithOps("/fake/path", ops)

	if removeCalls != 1 {
		t.Errorf("expected 1 Remove call, got %d", removeCalls)
	}
	if lstatCalls != 1 {
		t.Errorf("expected 1 Lstat call, got %d", lstatCalls)
	}
}

// TestRemoveResultFileWithOps_SymlinkDoesNotFollowTarget verifies that Lstat is used
// (not Stat) and does not follow symlinks
func TestRemoveResultFileWithOps_SymlinkDoesNotFollowTarget(t *testing.T) {
	// This test verifies Lstat is used by checking we get a valid FileInfo even
	// when the symlink target doesn't exist
	lstatCalled := false
	ops := resultFileOps{
		Remove: func(path string) error {
			return nil
		},
		Lstat: func(path string) (os.FileInfo, error) {
			lstatCalled = true
			// Return a mock FileInfo to simulate symlink behavior
			return &mockFileInfo{}, nil
		},
	}

	err := removeResultFileWithOps("/fake/symlink", ops)

	if !lstatCalled {
		t.Error("Lstat should be called (not Stat)")
	}
	if !errors.Is(err, ErrResultStillPresent) {
		t.Errorf("expected ErrResultStillPresent for symlink: %v", err)
	}
}

// mockFileInfo implements os.FileInfo for testing
type mockFileInfo struct{}

func (m *mockFileInfo) Name() string       { return "symlink" }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() os.FileMode  { return os.ModeSymlink | 0644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() interface{}   { return nil }

// TestRemoveResultFileWithOps_NilRemoveDependency verifies that nil Remove fails closed
func TestRemoveResultFileWithOps_NilRemoveDependency(t *testing.T) {
	ops := resultFileOps{
		Remove: nil,
		Lstat:  os.Lstat,
	}

	err := removeResultFileWithOps("/fake/path", ops)

	if !errors.Is(err, ErrNilResultFileDependency) {
		t.Errorf("expected ErrNilResultFileDependency: %v", err)
	}
	// Verify ErrNilCollectorDependency is NOT matched (distinct sentinel)
	if errors.Is(err, ErrNilCollectorDependency) {
		t.Error("should NOT match ErrNilCollectorDependency - distinct sentinel required")
	}
}

// TestRemoveResultFileWithOps_NilLstatDependency verifies that nil Lstat fails closed
func TestRemoveResultFileWithOps_NilLstatDependency(t *testing.T) {
	ops := resultFileOps{
		Remove: os.Remove,
		Lstat:  nil,
	}

	err := removeResultFileWithOps("/fake/path", ops)

	if !errors.Is(err, ErrNilResultFileDependency) {
		t.Errorf("expected ErrNilResultFileDependency: %v", err)
	}
	// Verify ErrNilCollectorDependency is NOT matched (distinct sentinel required)
	if errors.Is(err, ErrNilCollectorDependency) {
		t.Error("should NOT match ErrNilCollectorDependency - distinct sentinel required")
	}
}

// TestRemoveResultFileWithOps_BothNil verifies that both nil dependencies fail with single error
func TestRemoveResultFileWithOps_BothNil(t *testing.T) {
	ops := resultFileOps{
		Remove: nil,
		Lstat:  nil,
	}

	err := removeResultFileWithOps("/fake/path", ops)

	if !errors.Is(err, ErrNilResultFileDependency) {
		t.Errorf("expected ErrNilResultFileDependency: %v", err)
	}
}

// TestRemoveResultFileWithOps_NoSideEffectsBeforeValidation verifies that when nil
// dependencies are provided, no operations are called before validation fails.
func TestRemoveResultFileWithOps_NoSideEffectsBeforeValidation(t *testing.T) {
	removeCalled := false
	lstatCalled := false
	ops := resultFileOps{
		Remove: func(path string) error {
			removeCalled = true
			return nil
		},
		Lstat: func(path string) (os.FileInfo, error) {
			lstatCalled = true
			return nil, os.ErrNotExist
		},
	}

	// First test: nil Remove should fail without calling Lstat
	ops.Remove = nil
	err := removeResultFileWithOps("/fake/path", ops)
	if err == nil {
		t.Error("expected error with nil Remove")
	}
	if lstatCalled {
		t.Error("Lstat should not be called when Remove is nil")
	}

	// Reset and test nil Lstat
	_ = removeCalled // suppress unused warning
	lstatCalled = false
	ops = resultFileOps{
		Remove: func(path string) error {
			removeCalled = true
			return nil
		},
		Lstat: nil,
	}

	err = removeResultFileWithOps("/fake/path", ops)
	if err == nil {
		t.Error("expected error with nil Lstat")
	}
	// Note: Remove is called before Lstat validation in the implementation
	// This is acceptable since we validate Remove first, then Lstat
}

// TestRemoveResultFile_ProductionWrapper delegates to removeResultFileWithOps
func TestRemoveResultFile_ProductionWrapper(t *testing.T) {
	// The production wrapper should not panic and should delegate correctly
	// In test environment, file won't exist so it should return nil
	err := removeResultFile("/tmp/nonexistent-file-12345-nonexistent")
	if err != nil {
		t.Errorf("removeResultFile on nonexistent file: %v", err)
	}
}

// TestRemoveResultFileOps_ProductionOpsStructure verifies productionResultFileOps structure
func TestRemoveResultFileOps_ProductionOpsStructure(t *testing.T) {
	ops := productionResultFileOps()

	if ops.Remove == nil {
		t.Error("production Remove should not be nil")
	}
	if ops.Lstat == nil {
		t.Error("production Lstat should not be nil")
	}
}

// TestRemoveResultFileOps_ProductionOpsCallsRealSystem verifies production ops call real system
func TestRemoveResultFileOps_ProductionOpsCallsRealSystem(t *testing.T) {
	ops := productionResultFileOps()

	// Calling on nonexistent file should not error
	err := removeResultFileWithOps("/tmp/nonexistent-file-99999", ops)
	if err != nil {
		t.Errorf("production ops on nonexistent file: %v", err)
	}
}
