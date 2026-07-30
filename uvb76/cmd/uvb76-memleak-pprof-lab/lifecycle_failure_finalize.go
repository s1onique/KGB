package main

import (
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"time"
)

// Sentinel errors for lifecycle finalization.
// P0-4: Distinct from collector lifecycle and result removal sentinels.

// ErrNilFinalizationDependency is returned when lifecycle finalization receives a nil dependency.
var ErrNilFinalizationDependency = errors.New("nil lifecycle finalization dependency")

// ErrTovarischProcessResidual is returned when Tovarisch process is still present.
var ErrTovarischProcessResidual = errors.New("tovarisch process residual")

// ErrUVB76ProcessResidual is returned when UVB-76 process is still present.
var ErrUVB76ProcessResidual = errors.New("uvb76 process residual")

// ErrTovarischProcessUnproven is returned when Tovarisch process absence cannot be proven.
var ErrTovarischProcessUnproven = errors.New("tovarisch process absence unproven")

// ErrUVB76ProcessUnproven is returned when UVB-76 process absence cannot be proven.
var ErrUVB76ProcessUnproven = errors.New("uvb76 process absence unproven")

// ErrPortReleaseUnproven is returned when port release cannot be verified.
var ErrPortReleaseUnproven = errors.New("port release unproven")

// ErrPublicationFailed is returned when result publication fails.
var ErrPublicationFailed = errors.New("result publication failed")

// ErrStaleResultRemovalFailed is returned when stale result removal fails.
var ErrStaleResultRemovalFailed = errors.New("stale result removal failed")

// ErrStaleResultAbsenceUnproven is returned when stale result absence cannot be proven.
var ErrStaleResultAbsenceUnproven = errors.New("stale result absence unproven")

// ErrNilLabResult is returned when LabResult is nil.
var ErrNilLabResult = errors.New("nil lab result")

// ErrNilInitiatingFailure is returned when initiating failure is required but nil.
var ErrNilInitiatingFailure = errors.New("nil initiating failure")

// ErrEmptyArtifactDir is returned when artifact directory is empty.
var ErrEmptyArtifactDir = errors.New("empty artifact directory")

// ErrEmptyRunID is returned when run ID is empty.
var ErrEmptyRunID = errors.New("empty run id")

// ErrEmptySourceCommit is returned when source commit is required but empty.
var ErrEmptySourceCommit = errors.New("empty source commit")

// ErrEmptyRunStartedAt is returned when run start time is zero.
var ErrEmptyRunStartedAt = errors.New("empty run started at")

// ErrEmptyPort is returned when a required port is empty.
var ErrEmptyPort = errors.New("empty port")

// ErrNilProcesses is returned when Processes is nil.
var ErrNilProcesses = errors.New("nil processes")

// ErrNilIdentity is returned when Identity is nil.
var ErrNilIdentity = errors.New("nil identity")

// ErrInvalidPort is returned when a port is invalid.
var ErrInvalidPort = errors.New("invalid port")

// ErrEmptyTovarischBinPath is returned when Tovarisch binary path is empty for owned process.
var ErrEmptyTovarischBinPath = errors.New("empty tovarisch binary path")

// ErrEmptyUVB76BinPath is returned when UVB-76 binary path is empty for owned process.
var ErrEmptyUVB76BinPath = errors.New("empty uvb76 binary path")

// ErrEmptyTovarischPort is returned when tovarisch port is empty.
var ErrEmptyTovarischPort = errors.New("empty tovarisch port")

// ErrEmptyUVB76Port is returned when uvb76 port is empty.
var ErrEmptyUVB76Port = errors.New("empty uvb76 port")

// RuntimeEndpoints represents the separate URL and port authorities for a run.
// P0-1: URLs and ports are distinct typed authorities, never mixed.
type RuntimeEndpoints struct {
	TovarischBaseURL string // Full URL like "http://localhost:18317"
	TovarischPort    string // Decimal port like "18317"
	UVB76APIBaseURL  string // Full URL like "http://localhost:18444"
	UVB76Port        string // Decimal port like "18444"
	PProfBaseURL     string // Full URL like "http://localhost:16060"
	PProfPort        string // Decimal port like "16060"
}

// runExecutionIdentity holds identity information for a run.
// P0-2: Created once before process startup, shared by success and failure paths.
// P0-5: Includes binary paths for complete owned-process identity.
type runExecutionIdentity struct {
	RunID            string
	SourceCommit     string
	RunStartedAt     time.Time
	ArtifactDir      string
	TovarischBinPath string
	UVB76BinPath     string
	Endpoints        RuntimeEndpoints
}

// failedRunProcesses holds the process resources for cleanup.
// P0-4: Includes ProcessState with StartTime for complete process identity.
type failedRunProcesses struct {
	TovarischCmd *exec.Cmd
	UVB76Cmd     *exec.Cmd
	TovarischPS  *ProcessState // Contains StartTime for identity
	UVB76PS      *ProcessState // Contains StartTime for identity
}

// lifecycleFailureInput holds all inputs for lifecycle failure finalization.
// P0-4: Explicitly owns all identities and handles required by finalization.
// P0-6: No package-global PIDs - all PIDs come from this input.
type lifecycleFailureInput struct {
	// Process PIDs - zero means no owned process
	TovarischPID int
	UVB76PID     int

	// Process resources for cleanup
	Processes *failedRunProcesses

	// Identity for result publication
	Identity *runExecutionIdentity

	// Original lifecycle failure to preserve
	InitiatingFailure error

	// Current LabResult to finalize
	LabResult *LabResult
}

// lifecycleFailureOps defines the operations for lifecycle failure finalization.
// P0-4: All authorities injectable for deterministic testing.
type lifecycleFailureOps struct {
	// Cleanup performs bounded cleanup and returns all cleanup errors.
	// P0-1: Returns []error to preserve exact causes via errors.Is.
	Cleanup func() []error

	// ProcessGone checks if a process with the given PID is gone.
	// Returns (gone, error). If error is non-nil, absence is unproven.
	ProcessGone func(pid int) (bool, error)

	// VerifyPortsReleased verifies all owned ports are released.
	VerifyPortsReleased func() error

	// RemoveStaleResult removes a stale result file and verifies absence.
	// P0-4: Injected for deterministic testing.
	RemoveStaleResult func(path string) error

	// PublishFailedResult publishes the final failed result and returns any error.
	PublishFailedResult func(result *Result) error
}

// validateLifecycleFailureInput validates all inputs before any side effects.
// P0-3: Returns all validation errors without side effects.
func validateLifecycleFailureInput(input lifecycleFailureInput) []error {
	var validationErrors []error

	// LabResult != nil
	if input.LabResult == nil {
		validationErrors = append(validationErrors, ErrNilLabResult)
	}

	// InitiatingFailure != nil
	if input.InitiatingFailure == nil {
		validationErrors = append(validationErrors, ErrNilInitiatingFailure)
	}

	// Identity != nil
	if input.Identity == nil {
		validationErrors = append(validationErrors, ErrNilIdentity)
	} else {
		// RunID non-empty
		if input.Identity.RunID == "" {
			validationErrors = append(validationErrors, ErrEmptyRunID)
		}
		// SourceCommit non-empty
		if input.Identity.SourceCommit == "" {
			validationErrors = append(validationErrors, ErrEmptySourceCommit)
		}
		// RunStartedAt non-zero
		if input.Identity.RunStartedAt.IsZero() {
			validationErrors = append(validationErrors, ErrEmptyRunStartedAt)
		}
		// ArtifactDir non-empty
		if input.Identity.ArtifactDir == "" {
			validationErrors = append(validationErrors, ErrEmptyArtifactDir)
		}
		// All ports non-empty - use Endpoints field
		if input.Identity.Endpoints.TovarischPort == "" {
			validationErrors = append(validationErrors, fmt.Errorf("%w: tovarisch", ErrEmptyPort))
		}
		if input.Identity.Endpoints.UVB76Port == "" {
			validationErrors = append(validationErrors, fmt.Errorf("%w: uvb76", ErrEmptyPort))
		}
		if input.Identity.Endpoints.PProfPort == "" {
			validationErrors = append(validationErrors, fmt.Errorf("%w: pprof", ErrEmptyPort))
		}
	}

	// Processes != nil
	if input.Processes == nil {
		validationErrors = append(validationErrors, ErrNilProcesses)
	}

	// Validate operation callbacks
	// (Cleanup, ProcessGone, etc. are validated separately in finalizeLifecycleFailureWithOps)

	return validationErrors
}

// finalizeLifecycleFailureWithOps finalizes lifecycle failure using the provided operations.
// P0-4: Returns one joined error compatible with errors.Is for all causes.
// P0-5: Exact tested order:
//  1. validate all inputs
//  2. preserve the initiating lifecycle failure
//  3. attempt cleanup exactly once
//  4. independently determine Tovarisch process absence
//  5. independently determine UVB-76 process absence
//  6. independently verify all owned ports are released
//  7. remove stale result and verify absence
//  8. set terminal classification to FAILED
//  9. set OK=false
//  10. construct the final failed physical result
//  11. attempt failed-result publication
//  12. return one joined error with all causes
//
// P0-6: Uses only supplied PIDs, never package globals.
func finalizeLifecycleFailureWithOps(input lifecycleFailureInput, ops lifecycleFailureOps) error {
	var finalizationErrors []error

	// Step 1: Validate all inputs before any side effects
	// P0-3: Input validation
	validationErrors := validateLifecycleFailureInput(input)
	if len(validationErrors) > 0 {
		return errors.Join(validationErrors...)
	}

	// Validate operations
	if ops.Cleanup == nil {
		finalizationErrors = append(finalizationErrors, fmt.Errorf("%w: Cleanup", ErrNilFinalizationDependency))
	}
	if ops.ProcessGone == nil {
		finalizationErrors = append(finalizationErrors, fmt.Errorf("%w: ProcessGone", ErrNilFinalizationDependency))
	}
	if ops.VerifyPortsReleased == nil {
		finalizationErrors = append(finalizationErrors, fmt.Errorf("%w: VerifyPortsReleased", ErrNilFinalizationDependency))
	}
	if ops.RemoveStaleResult == nil {
		finalizationErrors = append(finalizationErrors, fmt.Errorf("%w: RemoveStaleResult", ErrNilFinalizationDependency))
	}
	if ops.PublishFailedResult == nil {
		finalizationErrors = append(finalizationErrors, fmt.Errorf("%w: PublishFailedResult", ErrNilFinalizationDependency))
	}

	// If validation errors exist, fail without side effects
	if len(finalizationErrors) > 0 {
		return errors.Join(finalizationErrors...)
	}

	// Step 2: Preserve the initiating lifecycle failure
	finalizationErrors = append(finalizationErrors, input.InitiatingFailure)

	// Step 3: Attempt cleanup exactly once
	cleanupErrors := ops.Cleanup()
	finalizationErrors = append(finalizationErrors, cleanupErrors...)

	// Step 4: Independently determine Tovarisch process absence
	tovarischRemoved := input.TovarischPID <= 0
	if input.TovarischPID > 0 {
		gone, checkErr := ops.ProcessGone(input.TovarischPID)
		if checkErr != nil {
			// Process absence unproven - fail closed
			finalizationErrors = append(finalizationErrors, fmt.Errorf("%w: PID %d: %w", ErrTovarischProcessUnproven, input.TovarischPID, checkErr))
		} else if gone {
			tovarischRemoved = true
		} else {
			// Process still present
			finalizationErrors = append(finalizationErrors, fmt.Errorf("%w: PID %d", ErrTovarischProcessResidual, input.TovarischPID))
		}
	}

	// Step 5: Independently determine UVB-76 process absence
	uvb76Removed := input.UVB76PID <= 0
	if input.UVB76PID > 0 {
		gone, checkErr := ops.ProcessGone(input.UVB76PID)
		if checkErr != nil {
			// Process absence unproven - fail closed
			finalizationErrors = append(finalizationErrors, fmt.Errorf("%w: PID %d: %w", ErrUVB76ProcessUnproven, input.UVB76PID, checkErr))
		} else if gone {
			uvb76Removed = true
		} else {
			// Process still present
			finalizationErrors = append(finalizationErrors, fmt.Errorf("%w: PID %d", ErrUVB76ProcessResidual, input.UVB76PID))
		}
	}

	// Step 6: Independently verify all owned ports are released
	portErr := ops.VerifyPortsReleased()
	portsReleased := portErr == nil
	if portErr != nil {
		finalizationErrors = append(finalizationErrors, fmt.Errorf("%w: %w", ErrPortReleaseUnproven, portErr))
	}

	// Step 7: Remove stale result and verify absence
	// P0-4: Stale OBSERVED result must not survive failed finalization
	resultFile := input.Identity.ArtifactDir + "/result.json"
	staleRemovalErr := ops.RemoveStaleResult(resultFile)
	staleRemoved := staleRemovalErr == nil
	if staleRemovalErr != nil {
		// P0-4: Fail-closed - do not publish if stale result cannot be removed
		finalizationErrors = append(finalizationErrors, fmt.Errorf("%w: %w", ErrStaleResultRemovalFailed, staleRemovalErr))

		// Even if removal failed, check if absence was proven
		if !errors.Is(staleRemovalErr, ErrResultAbsenceUnproven) && !errors.Is(staleRemovalErr, ErrResultStillPresent) {
			// Unexpected error - add sentinel
			finalizationErrors = append(finalizationErrors, ErrStaleResultAbsenceUnproven)
		}
	}

	// Step 8: Set terminal classification to FAILED
	// Step 9: Set OK=false
	input.LabResult.Classification = "FAILED"
	input.LabResult.OK = false

	// Step 10: Update cleanup results
	// P0-5: CleanupSuccess requires ALL cleanup conditions
	cleanupSuccess := len(cleanupErrors) == 0 && tovarischRemoved && uvb76Removed && portsReleased
	input.LabResult.TovarischRemoved = tovarischRemoved
	input.LabResult.UVB76Removed = uvb76Removed
	input.LabResult.PortsReleased = portsReleased

	// Step 11: Attempt failed-result publication
	// P0-4: Only publish if stale result was successfully removed or never existed
	if staleRemoved {
		failedResult := buildFailedResult(input.LabResult, input.Identity, cleanupSuccess, cleanupErrors, finalizationErrors)
		if pubErr := ops.PublishFailedResult(failedResult); pubErr != nil {
			// P0-5: Publication failure must be preserved alongside all earlier causes
			finalizationErrors = append(finalizationErrors, fmt.Errorf("%w: %w", ErrPublicationFailed, pubErr))

			// P0-7: Attempt to remove any invalid/stale final file
			if rmErr := ops.RemoveStaleResult(resultFile); rmErr != nil {
				finalizationErrors = append(finalizationErrors, fmt.Errorf("%w: %w", ErrStaleResultRemovalFailed, rmErr))
			}
		}
	}

	// Step 12: Return one joined error with all causes
	// P0-4: errors.Join creates one error that satisfies errors.Is for all causes
	return errors.Join(finalizationErrors...)
}

// buildFailedResult constructs the final failed result from the LabResult and identity.
// P0-4: Includes complete error projection from LabResult.Errors + finalizationErrors.
// P0-6: Uses supplied identity, never package globals.
func buildFailedResult(lab *LabResult, identity *runExecutionIdentity, cleanupSuccess bool, cleanupErrors []error, finalizationErrors []error) *Result {
	// P0-4: Clone existing LabResult.Errors to avoid mutation
	var existingErrors []string
	if lab.Errors != nil {
		existingErrors = make([]string, len(lab.Errors))
		copy(existingErrors, lab.Errors)
	}

	// Collect all finalization error strings
	var finalizationErrStrings []string
	for _, e := range finalizationErrors {
		finalizationErrStrings = append(finalizationErrStrings, e.Error())
	}

	// P0-4: Construct Errors as deterministic projection of existing + finalization
	allErrors := append(existingErrors, finalizationErrStrings...)

	// P0-1: Use Endpoints field for ports
	r := &Result{
		SchemaVersion:    ResultSchemaVersion,
		RunID:            identity.RunID,
		SourceCommit:     identity.SourceCommit,
		RunStartedAt:     identity.RunStartedAt.Format(time.RFC3339),
		TovarischPort:    identity.Endpoints.TovarischPort,
		UVB76Port:        identity.Endpoints.UVB76Port,
		PProfPort:        identity.Endpoints.PProfPort,
		Classification:   "FAILED",
		OK:               false,
		CleanupSuccess:   cleanupSuccess,
		UVB76Removed:     lab.UVB76Removed,
		TovarischRemoved: lab.TovarischRemoved,
		PortsReleased:    lab.PortsReleased,
		// P0-4: Include all errors (existing + finalization)
		Errors: allErrors,
	}

	// P0-4: Include cleanup errors separately
	for _, e := range cleanupErrors {
		r.CleanupErrors = append(r.CleanupErrors, e.Error())
	}

	// P0-3: Populate complete ProcessIdentity with ALL fields
	if lab.TovarischPID > 0 {
		r.TovarischIdentity = &ProcessIdentity{
			PID:            lab.TovarischPID,
			Port:           identity.Endpoints.TovarischPort,
			ExecutablePath: lab.TovarischBinPath,
		}
		// P0-3: Include StartTime if available
		if lab.TovarischStartTime != nil {
			r.TovarischIdentity.StartTime = *lab.TovarischStartTime
		}
		// P0-3: Clone argv to prevent mutation
		if lab.TovarischArgv != nil {
			r.TovarischIdentity.Argv = slices.Clone(lab.TovarischArgv)
		}
	}

	if lab.UVB76PID > 0 {
		r.UVB76Identity = &ProcessIdentity{
			PID:            lab.UVB76PID,
			Port:           identity.Endpoints.UVB76Port,
			ExecutablePath: lab.UVB76BinPath,
		}
		// P0-3: Include StartTime if available
		if lab.UVB76StartTime != nil {
			r.UVB76Identity.StartTime = *lab.UVB76StartTime
		}
		// P0-3: Clone argv to prevent mutation
		if lab.UVB76Argv != nil {
			r.UVB76Identity.Argv = slices.Clone(lab.UVB76Argv)
		}
	}

	return r
}

// validateRunExecutionIdentity validates the complete runExecutionIdentity before any process startup.
// P0-2: Returns all validation errors; fails closed if any field is invalid.
// P0-2: Must be called BEFORE starting any processes.
// P0-5: fakeMode parameter controls whether Tovarisch binary path validation is skipped.
func validateRunExecutionIdentity(identity *runExecutionIdentity, fakeMode bool) []error {
	var validationErrors []error

	if identity == nil {
		return []error{ErrNilIdentity}
	}

	// RunID non-empty
	if identity.RunID == "" {
		validationErrors = append(validationErrors, ErrEmptyRunID)
	}

	// SourceCommit non-empty (from embedded identity)
	if identity.SourceCommit == "" {
		validationErrors = append(validationErrors, ErrEmptySourceCommit)
	}

	// RunStartedAt non-zero
	if identity.RunStartedAt.IsZero() {
		validationErrors = append(validationErrors, ErrEmptyRunStartedAt)
	}

	// ArtifactDir non-empty
	if identity.ArtifactDir == "" {
		validationErrors = append(validationErrors, ErrEmptyArtifactDir)
	}

	// P0-5: Binary path validation is mode-aware
	// In fake mode, Tovarisch binary path is not used
	if !fakeMode && identity.TovarischBinPath == "" {
		validationErrors = append(validationErrors, ErrEmptyTovarischBinPath)
	}
	// UVB-76 binary path is always required (UVB-76 is never faked)
	if identity.UVB76BinPath == "" {
		validationErrors = append(validationErrors, ErrEmptyUVB76BinPath)
	}

	// P0-1: Validate RuntimeEndpoints - all ports non-empty and valid
	if identity.Endpoints.TovarischPort == "" {
		validationErrors = append(validationErrors, fmt.Errorf("%w: tovarisch", ErrEmptyPort))
	} else if !isValidPort(identity.Endpoints.TovarischPort) {
		validationErrors = append(validationErrors, fmt.Errorf("%w: tovarisch=%s", ErrInvalidPort, identity.Endpoints.TovarischPort))
	}

	if identity.Endpoints.UVB76Port == "" {
		validationErrors = append(validationErrors, fmt.Errorf("%w: uvb76", ErrEmptyPort))
	} else if !isValidPort(identity.Endpoints.UVB76Port) {
		validationErrors = append(validationErrors, fmt.Errorf("%w: uvb76=%s", ErrInvalidPort, identity.Endpoints.UVB76Port))
	}

	if identity.Endpoints.PProfPort == "" {
		validationErrors = append(validationErrors, fmt.Errorf("%w: pprof", ErrEmptyPort))
	} else if !isValidPort(identity.Endpoints.PProfPort) {
		validationErrors = append(validationErrors, fmt.Errorf("%w: pprof=%s", ErrInvalidPort, identity.Endpoints.PProfPort))
	}

	return validationErrors
}

// isValidPort checks if a port string is a valid port number.
// P0-1: Uses strconv.ParseUint for exact decimal string validation.
// Rejects non-numeric prefixes/suffixes (e.g., "80a", " 80", "80 ", "+80").
func isValidPort(port string) bool {
	if port == "" {
		return false
	}
	// ParseUint requires exact decimal match (no trailing/leading whitespace)
	value, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return false
	}
	// Ports must be non-zero
	return value != 0
}
