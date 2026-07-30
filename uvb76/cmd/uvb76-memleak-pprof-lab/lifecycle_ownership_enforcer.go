package main

import "errors"

// lifecycleOwnershipResult holds the result of inspecting lifecycle ownership.
type lifecycleOwnershipResult struct {
	RunLabFound                  bool // true if runLab function was found
	CollectAndSnapshotCalls      int  // Direct calls to CollectAndSnapshot
	RunCollectionLifecycleCalls  int  // Calls to RunCollectionLifecycle (which internally calls CollectAndSnapshot)
	DeferredCancelCalls          int
	OrdinaryCancelCalls          int
	DirectWaitCalls              int
	DirectCloneCalls             int
	ImportsSlices                bool
	WaitGroupIdentifier          string // Identifier name for WaitGroup binding
}

// ErrLifecycleOwnershipViolation is the sentinel for lifecycle ownership violations.
var ErrLifecycleOwnershipViolation = errors.New("lifecycle ownership violation")

// ErrMissingRunLab is returned when runLab function is not found.
var ErrMissingRunLab = errors.New("runLab function not found")

// ErrMissingCollectAndSnapshot is returned when CollectAndSnapshot is not called exactly once.
var ErrMissingCollectAndSnapshot = errors.New("CollectAndSnapshot must be called exactly once")

// ErrDuplicateCollectAndSnapshot is returned when CollectAndSnapshot is called more than once.
var ErrDuplicateCollectAndSnapshot = errors.New("CollectAndSnapshot must be called exactly once")

// ErrOrdinaryCancel is returned when collectionCancel is called without defer.
var ErrOrdinaryCancel = errors.New("collectionCancel must be deferred, not called ordinarily")

// ErrDirectWait is returned when WaitGroup.Wait is called directly.
var ErrDirectWait = errors.New("WaitGroup.Wait must not be called directly")

// ErrDirectClone is returned when slices.Clone is called directly.
var ErrDirectClone = errors.New("slices.Clone must not be called directly")

// ErrSlicesImport is returned when slices package is imported.
var ErrSlicesImport = errors.New("slices package must not be imported")

// ErrMissingLifecycleAuthority is returned when neither CollectAndSnapshot nor RunCollectionLifecycle is called.
var ErrMissingLifecycleAuthority = errors.New("CollectAndSnapshot or RunCollectionLifecycle must be called exactly once")

// ErrLifecycleAuthorityCount is returned when lifecycle authority count is not exactly one.
var ErrLifecycleAuthorityCount = errors.New("exactly one lifecycle authority required")

// validateLifecycleOwnership checks the lifecycle ownership result and returns a typed error
// if any violation is detected.
func validateLifecycleOwnership(result lifecycleOwnershipResult) error {
	// Distinguish missing runLab from missing helper call
	if !result.RunLabFound {
		return errors.Join(ErrLifecycleOwnershipViolation, ErrMissingRunLab)
	}

	// Exactly one lifecycle authority must be called
	// Check specific duplicates before checking total > 1
	switch {
	case result.CollectAndSnapshotCalls == 0 && result.RunCollectionLifecycleCalls == 0:
		return errors.Join(ErrLifecycleOwnershipViolation, ErrMissingLifecycleAuthority)
	case result.CollectAndSnapshotCalls > 1:
		return errors.Join(ErrLifecycleOwnershipViolation, ErrDuplicateCollectAndSnapshot)
	case result.RunCollectionLifecycleCalls > 1:
		return errors.Join(ErrLifecycleOwnershipViolation, errors.New("RunCollectionLifecycle must be called at most once"))
	case result.CollectAndSnapshotCalls == 1 && result.RunCollectionLifecycleCalls == 1:
		// Having both is forbidden - must be exactly one
		return errors.Join(ErrLifecycleOwnershipViolation, ErrLifecycleAuthorityCount)
	}

	if result.OrdinaryCancelCalls > 0 {
		return errors.Join(ErrLifecycleOwnershipViolation, ErrOrdinaryCancel)
	}

	if result.DirectWaitCalls > 0 {
		return errors.Join(ErrLifecycleOwnershipViolation, ErrDirectWait)
	}

	if result.DirectCloneCalls > 0 {
		return errors.Join(ErrLifecycleOwnershipViolation, ErrDirectClone)
	}

	if result.ImportsSlices {
		return errors.Join(ErrLifecycleOwnershipViolation, ErrSlicesImport)
	}

	return nil
}
