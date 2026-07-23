// cleanup_observation.go — Runtime Cleanup Observation
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03
//
// Provides authoritative Docker cleanup observation through exact ID inspection.
// Uses object-specific Docker commands (docker container inspect, docker network inspect)
// with explicit --type flags to prevent name-based matching.
//
// P0-2 FIX: Container, network, and process cleanup are observed, not asserted.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// =============================================================================
// CONSTANTS
// =============================================================================

const (
	DockerObservationTimeout   = 10 * time.Second
	DockerObservationWaitDelay = 2 * time.Second
	DockerObservationMaxStdout = 64 * 1024
	DockerObservationMaxStderr = 64 * 1024
)

// CanonicalScenarioOrder is declared in reconstruct.go and re-used here.

// =============================================================================
// DOCKER COMMAND LIMITS
// =============================================================================

// DockerCommandLimits bounds Docker command execution.
type DockerCommandLimits struct {
	Timeout        time.Duration
	WaitDelay      time.Duration
	MaxStdoutBytes int64
	MaxStderrBytes int64
}

// DefaultDockerCommandLimits returns sensible defaults for observation commands.
func DefaultDockerCommandLimits() DockerCommandLimits {
	return DockerCommandLimits{
		Timeout:        DockerObservationTimeout,
		WaitDelay:      DockerObservationWaitDelay,
		MaxStdoutBytes: DockerObservationMaxStdout,
		MaxStderrBytes: DockerObservationMaxStderr,
	}
}

// =============================================================================
// DOCKER COMMAND RESULT
// =============================================================================

// DockerCommandResult represents the result of a bounded Docker CLI command.
// P0-3 FIX: Includes WaitDelayExpired to preserve material execution truth.
type DockerCommandResult struct {
	Stdout           []byte
	Stderr           []byte
	ExitCode         int
	TimedOut         bool
	WaitDelayExpired bool // P0-3: Process exited but output pipes exceeded WaitDelay
	StdoutOverflow   bool
	StderrOverflow   bool
}

// =============================================================================
// DOCKER RUNNER SEAM
// =============================================================================

// DockerRunner is a function type for executing Docker commands.
// Used by tests to inject mock Docker runners.
type DockerRunner func(ctx context.Context, limits DockerCommandLimits, args ...string) (DockerCommandResult, error)

// DefaultDockerRunner is a thin adapter that delegates to RunDockerCommand.
// P0-2 FIX: Single executor authority - no independent implementation.
func DefaultDockerRunner(ctx context.Context, limits DockerCommandLimits, args ...string) (DockerCommandResult, error) {
	return RunDockerCommand(ctx, limits, args...)
}

// =============================================================================
// CLEANUP OBSERVER SEAM
// =============================================================================

// CleanupObserver observes cleanup state through Docker inspection.
// Uses injected DockerRunner for testability.
type CleanupObserver struct {
	runDocker DockerRunner
	limits    DockerCommandLimits
}

// NewCleanupObserver creates a new cleanup observer with the default runner.
func NewCleanupObserver() *CleanupObserver {
	return &CleanupObserver{
		runDocker: DefaultDockerRunner,
		limits:    DefaultDockerCommandLimits(),
	}
}

// NewCleanupObserverWithRunner creates a cleanup observer with an injected runner.
// P0 FIX: Rejects nil runner to prevent panic on first inspection.
func NewCleanupObserverWithRunner(runner DockerRunner, limits DockerCommandLimits) (*CleanupObserver, error) {
	if runner == nil {
		return nil, errors.New("docker runner is nil")
	}
	return &CleanupObserver{
		runDocker: runner,
		limits:    limits,
	}, nil
}

// =============================================================================
// CONTAINER OBSERVATION
// =============================================================================

// ObserveContainerCleanup inspects the exact container ID and returns its status.
// Uses "docker container inspect" to avoid name-based matching.
// Returns unavailable on timeout, overflow, or daemon errors.
func (o *CleanupObserver) ObserveContainerCleanup(ctx context.Context, containerID string) (*ContainerIdentityObservation, error) {
	if containerID == "" {
		return nil, errors.New("container ID is empty")
	}

	// Use docker container inspect with exact full ID
	args := []string{"container", "inspect", "--format={{.Id}}", containerID}
	result, err := o.runDocker(ctx, o.limits, args...)
	if err != nil {
		// Docker daemon error, network failure, etc.
		return &ContainerIdentityObservation{
			ID:     containerID,
			Status: ObjectUnavailable,
		}, nil
	}

	// P0-3 FIX: Reject all ambiguous execution results as unavailable
	if result.TimedOut || result.WaitDelayExpired || result.StdoutOverflow || result.StderrOverflow {
		return &ContainerIdentityObservation{
			ID:     containerID,
			Status: ObjectUnavailable,
		}, nil
	}

	// Parse result
	if result.ExitCode != 0 {
		// Non-zero exit code - check for exact not-found
		if classifyContainerInspectAbsence(containerID, result) {
			return &ContainerIdentityObservation{
				ID:     containerID,
				Status: ObjectGone,
			}, nil
		}
		// Other error (daemon, permission, unfamiliar)
		return &ContainerIdentityObservation{
			ID:     containerID,
			Status: ObjectUnavailable,
		}, nil
	}

	// Zero exit code - check what was returned
	observedID := strings.TrimSpace(string(result.Stdout))
	if observedID == "" {
		// P0-3 FIX: Empty successful output is ambiguous, not authoritative "gone"
		// Docker's object absence normally produces non-zero exit with diagnostic
		return &ContainerIdentityObservation{
			ID:     containerID,
			Status: ObjectUnavailable,
		}, nil
	}

	// Compare with expected ID
	if observedID != containerID {
		// Different ID returned - not the expected container
		return &ContainerIdentityObservation{
			ID:     containerID,
			Status: ObjectUnavailable,
		}, nil
	}

	// Exact ID returned - container exists
	return &ContainerIdentityObservation{
		ID:     containerID,
		Status: ObjectExists,
	}, nil
}

// classifyContainerInspectAbsence checks if the error indicates the exact requested
// container was not found. Only Docker's specific "no such container" diagnostic
// for the exact requested ID becomes "gone". Generic errors become "unavailable".
// P0-5 FIX: Parses exact token from diagnostic, not substring matching.
func classifyContainerInspectAbsence(requestedID string, result DockerCommandResult) bool {
	stderr := string(result.Stderr)

	// Must contain the exact "no such container" pattern
	if !strings.Contains(strings.ToLower(stderr), "no such container") {
		return false
	}

	// Parse the Docker diagnostic token: "Error: No such container: <token>"
	lines := strings.Split(stderr, "\n")
	for _, line := range lines {
		// Look for lines containing the diagnostic pattern
		if strings.Contains(strings.ToLower(line), "no such container") {
			// Extract the token after ": "
			parts := strings.SplitN(line, ": ", 3)
			if len(parts) >= 3 {
				token := strings.TrimSpace(parts[2])
				// P0-5 FIX: Exact equality check
				if token == requestedID {
					return true
				}
			}
		}
	}

	return false
}

// =============================================================================
// NETWORK OBSERVATION
// =============================================================================

// ObserveNetworkCleanup inspects the exact network ID and returns its status.
// Uses "docker network inspect" to avoid name-based matching.
// Returns unavailable on timeout, overflow, or daemon errors.
func (o *CleanupObserver) ObserveNetworkCleanup(ctx context.Context, networkID string) (*NetworkIdentityObservation, error) {
	if networkID == "" {
		return nil, errors.New("network ID is empty")
	}

	// Use docker network inspect with exact full ID
	args := []string{"network", "inspect", "--format={{.Id}}", networkID}
	result, err := o.runDocker(ctx, o.limits, args...)
	if err != nil {
		// Docker daemon error, network failure, etc.
		return &NetworkIdentityObservation{
			ID:     networkID,
			Status: ObjectUnavailable,
		}, nil
	}

	// P0-3 FIX: Reject all ambiguous execution results as unavailable
	if result.TimedOut || result.WaitDelayExpired || result.StdoutOverflow || result.StderrOverflow {
		return &NetworkIdentityObservation{
			ID:     networkID,
			Status: ObjectUnavailable,
		}, nil
	}

	// Parse result
	if result.ExitCode != 0 {
		// Non-zero exit code - check for exact not-found
		if classifyNetworkInspectAbsence(networkID, result) {
			return &NetworkIdentityObservation{
				ID:     networkID,
				Status: ObjectGone,
			}, nil
		}
		// Other error (daemon, permission, unfamiliar)
		return &NetworkIdentityObservation{
			ID:     networkID,
			Status: ObjectUnavailable,
		}, nil
	}

	// Zero exit code - check what was returned
	observedID := strings.TrimSpace(string(result.Stdout))
	if observedID == "" {
		// P0-3 FIX: Empty successful output is ambiguous, not authoritative "gone"
		// Docker's object absence normally produces non-zero exit with diagnostic
		return &NetworkIdentityObservation{
			ID:     networkID,
			Status: ObjectUnavailable,
		}, nil
	}

	// Compare with expected ID
	if observedID != networkID {
		// Different ID returned - not the expected network
		return &NetworkIdentityObservation{
			ID:     networkID,
			Status: ObjectUnavailable,
		}, nil
	}

	// Exact ID returned - network exists
	return &NetworkIdentityObservation{
		ID:     networkID,
		Status: ObjectExists,
	}, nil
}

// classifyNetworkInspectAbsence checks if the error indicates the exact requested
// network was not found. Only Docker's specific "no such network" diagnostic
// for the exact requested ID becomes "gone". Generic errors become "unavailable".
// P0-5 FIX: Parses exact token from diagnostic, not substring matching.
func classifyNetworkInspectAbsence(requestedID string, result DockerCommandResult) bool {
	stderr := string(result.Stderr)

	// Must contain the exact "no such network" pattern
	if !strings.Contains(strings.ToLower(stderr), "no such network") {
		return false
	}

	// Parse the Docker diagnostic token: "Error: No such network: <token>"
	lines := strings.Split(stderr, "\n")
	for _, line := range lines {
		// Look for lines containing the diagnostic pattern
		if strings.Contains(strings.ToLower(line), "no such network") {
			// Extract the token after ": "
			parts := strings.SplitN(line, ": ", 3)
			if len(parts) >= 3 {
				token := strings.TrimSpace(parts[2])
				// P0-5 FIX: Exact equality check
				if token == requestedID {
					return true
				}
			}
		}
	}

	return false
}

// =============================================================================
// PROCESS OBSERVATION
// =============================================================================

// ProcessReader is a seam for reading /proc data in tests.
type ProcessReader interface {
	ReadFile(path string) ([]byte, error)
}

// OSProcessReader is the real /proc reader.
type OSProcessReader struct{}

func (OSProcessReader) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// ObserveProcessCleanup checks if the subject process with given PID and start time is gone.
// Preserves the expected start time when the process is confirmed gone.
func (o *CleanupObserver) ObserveProcessCleanup(ctx context.Context, pid int, expectedStart uint64, reader ProcessReader) (*ProcessIdentityObservation, error) {
	if pid <= 0 {
		return &ProcessIdentityObservation{
			PID:    pid,
			Status: ProcessUnavailableCode,
		}, nil
	}

	if reader == nil {
		reader = OSProcessReader{}
	}

	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := reader.ReadFile(statPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Process gone - preserve expected start time
			return &ProcessIdentityObservation{
				PID:       pid,
				StartTime: expectedStart,
				Status:    ProcessGoneCode,
			}, nil
		}
		// Permission denied or other I/O error
		return &ProcessIdentityObservation{
			PID:    pid,
			Status: ProcessUnavailableCode,
		}, nil
	}

	actualStart, parseErr := parseProcStatStartTime(data)
	if parseErr != nil {
		// Malformed stat
		return &ProcessIdentityObservation{
			PID:    pid,
			Status: ProcessUnavailableCode,
		}, nil
	}

	if actualStart == expectedStart {
		// Same start time - original process still alive
		return &ProcessIdentityObservation{
			PID:       pid,
			StartTime: actualStart,
			Status:    ProcessStillAliveCode,
		}, nil
	}

	// P0-4 FIX: PID reused - preserve expected start time to maintain identity binding
	// The newly observed start time belongs to the unrelated replacement process
	return &ProcessIdentityObservation{
		PID:       pid,
		StartTime: expectedStart,
		Status:    ProcessPIDReusedCode,
	}, nil
}

// parseProcStatStartTime extracts the start time (field 22) from /proc/<pid>/stat.
func parseProcStatStartTime(data []byte) (uint64, error) {
	commEnd := bytes.LastIndex(data, []byte{')'})
	if commEnd < 0 {
		return 0, errors.New("malformed /proc stat: missing closing parenthesis")
	}

	remaining := data[commEnd+2:]
	fieldParts := bytes.Split(remaining, []byte{' '})

	// After ')' fields: state(0) ppid(1) pgrp(2) session(3) tty_nr(4) tpgid(5)
	// flags(6) minflt(7) cminflt(8) majflt(9) cmajflt(10) utime(11) stime(12)
	// cutime(13) cstime(14) priority(15) nice(16) num_threads(17)
	// itrealvalue(18) starttime(19) - global field 22
	if len(fieldParts) < 20 {
		return 0, errors.New("malformed /proc stat: insufficient fields")
	}

	startTimeStr := bytes.TrimSpace(fieldParts[19])
	procStartTime, err := strconv.ParseUint(string(startTimeStr), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid starttime: %w", err)
	}

	return procStartTime, nil
}

// =============================================================================
// OBSERVATION COLLECTION
// =============================================================================

// ObserveDeclaredRunCleanup performs complete cleanup observation for all declared runs.
// P0-4 FIX: Validates structural inputs and propagates errors.
func ObserveDeclaredRunCleanup(
	ctx context.Context,
	runs []*VerifiedRun,
	observer *CleanupObserver,
) ([]RunCleanupObservation, error) {
	// Fail-closed input validation
	if runs == nil {
		return nil, errors.New("runs is nil")
	}
	if observer == nil {
		return nil, errors.New("observer is nil")
	}
	expectedCount := len(CanonicalScenarioOrder)
	if len(runs) != expectedCount {
		return nil, fmt.Errorf("expected exactly %d runs, got %d", expectedCount, len(runs))
	}

	observations := make([]RunCleanupObservation, len(runs))

	for i, run := range runs {
		// P0-4 FIX: Reject nil run entries
		if run == nil {
			return nil, fmt.Errorf("run[%d] is nil", i)
		}

		// P0-4 FIX: Validate canonical scenario order
		expectedScenario := CanonicalScenarioOrder[i]
		if run.DeclaredScenario != expectedScenario {
			return nil, fmt.Errorf("run[%d] has scenario %q, expected %q", i, run.DeclaredScenario, expectedScenario)
		}

		// P0-4 FIX: Validate structural identities before dispatch
		if run.ContainerID == "" {
			return nil, fmt.Errorf("run[%d] (%s) has empty container ID", i, run.DeclaredRunID)
		}
		if run.SubjectPID <= 0 {
			return nil, fmt.Errorf("run[%d] (%s) has invalid PID %d", i, run.DeclaredRunID, run.SubjectPID)
		}
		if run.SubjectStartTime == 0 {
			return nil, fmt.Errorf("run[%d] (%s) has zero start time", i, run.DeclaredRunID)
		}

		obs := RunCleanupObservation{
			RunID:    run.DeclaredRunID,
			Scenario: run.DeclaredScenario,
		}

		// Observe container
		containerObs, err := observer.ObserveContainerCleanup(ctx, run.ContainerID)
		if err != nil {
			return nil, fmt.Errorf("run[%d] container observation failed: %w", i, err)
		}
		obs.Container = *containerObs

		// P0-4 FIX: Network observation required - reject empty network ID
		if run.NetworkID == "" {
			return nil, fmt.Errorf("run[%d] (%s) has empty network ID", i, run.DeclaredRunID)
		}
		networkObs, err := observer.ObserveNetworkCleanup(ctx, run.NetworkID)
		if err != nil {
			return nil, fmt.Errorf("run[%d] network observation failed: %w", i, err)
		}
		obs.Network = *networkObs

		// Observe process
		processObs, err := observer.ObserveProcessCleanup(ctx, run.SubjectPID, run.SubjectStartTime, nil)
		if err != nil {
			return nil, fmt.Errorf("run[%d] process observation failed: %w", i, err)
		}
		obs.Process = *processObs

		observations[i] = obs
	}

	return observations, nil
}

// =============================================================================
// BUILD OBSERVED CLEANUP EVIDENCE
// =============================================================================

// ValidContainerStatus defines allowed container cleanup statuses.
var ValidContainerStatus = map[string]bool{
	"gone":        true,
	"exists":      true,
	"unavailable": true,
}

// ValidNetworkStatus defines allowed network cleanup statuses.
var ValidNetworkStatus = map[string]bool{
	"gone":        true,
	"exists":      true,
	"unavailable": true,
}

// ValidProcessStatus defines allowed process cleanup statuses.
var ValidProcessStatus = map[string]bool{
	"gone":           true,
	"pid_reused":     true,
	"still_alive":    true,
	"unavailable":    true,
}

// BuildObservedMatrixCleanupEvidence constructs cleanup evidence exclusively from observations.
// P0-6 FIX: Validates status enums and requires matrix_shared network identity.
func BuildObservedMatrixCleanupEvidence(
	matrixID string,
	networkOwnership string,
	observations []RunCleanupObservation,
	observedAt time.Time,
) (*MatrixCleanupEvidence, error) {
	// P0-6 FIX: Require canonical three-run geometry
	expectedCount := len(CanonicalScenarioOrder)
	if len(observations) != expectedCount {
		return nil, fmt.Errorf("expected exactly %d observations, got %d", expectedCount, len(observations))
	}

	// P0-6 FIX: Require non-empty matrix ID
	if matrixID == "" {
		return nil, errors.New("matrixID is empty")
	}

	// P0-6 FIX: Require valid network ownership mode
	if networkOwnership != "per_run" && networkOwnership != "matrix_shared" {
		return nil, fmt.Errorf("invalid networkOwnership: %s (must be per_run or matrix_shared)", networkOwnership)
	}

	// P0-6 FIX: Require valid observation timestamp
	if observedAt.IsZero() {
		return nil, errors.New("observedAt is zero")
	}

	records := make([]RunCleanupRecord, len(observations))
	seenRunIDs := make(map[string]bool)
	seenNetworkIDs := make(map[string]bool)
	var sharedNetworkID string

	for i, obs := range observations {
		// Validate scenario order matches canonical
		expectedScenario := CanonicalScenarioOrder[i]
		if obs.Scenario != expectedScenario {
			return nil, fmt.Errorf("observation[%d] has wrong scenario %q, expected %q", i, obs.Scenario, expectedScenario)
		}

		// P0-6 FIX: Require unique run IDs
		if obs.RunID == "" {
			return nil, fmt.Errorf("observation[%d] has empty run_id", i)
		}
		if seenRunIDs[obs.RunID] {
			return nil, fmt.Errorf("duplicate run_id %q in observations", obs.RunID)
		}
		seenRunIDs[obs.RunID] = true

		// P0-6 FIX: Require container identity for all runs
		if obs.Container.ID == "" {
			return nil, fmt.Errorf("observation[%d] (run %s) has empty container.id", i, obs.RunID)
		}

		// P0-6 FIX: Validate container status enum
		containerStatus := string(obs.Container.Status)
		if containerStatus == "" || !ValidContainerStatus[containerStatus] {
			return nil, fmt.Errorf("observation[%d] has invalid container status %q", i, containerStatus)
		}

		// P0-6 FIX: Require process identity for all runs
		if obs.Process.PID == 0 {
			return nil, fmt.Errorf("observation[%d] (run %s) has zero process.pid", i, obs.RunID)
		}
		if obs.Process.StartTime == 0 {
			return nil, fmt.Errorf("observation[%d] (run %s) has zero process.start_time", i, obs.RunID)
		}

		// P0-6 FIX: Validate process status enum
		processStatus := string(obs.Process.Status)
		if processStatus == "" || !ValidProcessStatus[processStatus] {
			return nil, fmt.Errorf("observation[%d] has invalid process status %q", i, processStatus)
		}

		// P0-6 FIX: Network validation depends on ownership mode
		switch networkOwnership {
		case "per_run":
			if obs.Network.ID == "" {
				return nil, fmt.Errorf("observation[%d] (run %s) has empty network.id in per_run mode", i, obs.RunID)
			}
			if seenNetworkIDs[obs.Network.ID] {
				return nil, fmt.Errorf("duplicate network.id %q in per_run mode", obs.Network.ID)
			}
			seenNetworkIDs[obs.Network.ID] = true

			// Validate network status enum
			networkStatus := string(obs.Network.Status)
			if networkStatus == "" || !ValidNetworkStatus[networkStatus] {
				return nil, fmt.Errorf("observation[%d] has invalid network status %q", i, networkStatus)
			}

		case "matrix_shared":
			// P0-6 FIX: Require one non-empty shared network identity
			if obs.Network.ID != "" {
				if sharedNetworkID == "" {
					sharedNetworkID = obs.Network.ID
				} else if obs.Network.ID != sharedNetworkID {
					return nil, fmt.Errorf("observation[%d] has network.id %q != shared %q", i, obs.Network.ID, sharedNetworkID)
				}
			}
		}

		records[i] = RunCleanupRecord{
			Index:    i,
			Scenario: obs.Scenario,
			RunID:    obs.RunID,
			Container: ContainerCleanupRecord{
				ID:     obs.Container.ID,
				Name:   obs.Container.Name,
				Status: containerStatus,
			},
			Network: NetworkCleanupRecord{
				ID:     obs.Network.ID,
				Name:   obs.Network.Name,
				Status: string(obs.Network.Status),
			},
			Process: ProcessCleanupRecord{
				PID:       obs.Process.PID,
				StartTime: obs.Process.StartTime,
				Status:    processStatus,
			},
		}
	}

	// P0-6 FIX: Validate matrix_shared requires non-empty shared network identity
	if networkOwnership == "matrix_shared" && sharedNetworkID == "" {
		return nil, errors.New("matrix_shared requires a non-empty network identity")
	}

	// P0-6 FIX: Validate matrix_shared network status if present
	if networkOwnership == "matrix_shared" && sharedNetworkID != "" {
		for i, obs := range observations {
			if obs.Network.ID == sharedNetworkID {
				networkStatus := string(obs.Network.Status)
				if networkStatus == "" || !ValidNetworkStatus[networkStatus] {
					return nil, fmt.Errorf("observation[%d] (shared network) has invalid network status %q", i, networkStatus)
				}
			}
		}
	}

	return &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        matrixID,
		ObservedAt:      observedAt,
		NetworkOwnership: networkOwnership,
		Runs:            records,
	}, nil
}

// BuildRunCleanupRecordFromObservation converts an observation to a cleanup record.
func BuildRunCleanupRecordFromObservation(obs *RunCleanupObservation) RunCleanupRecord {
	return RunCleanupRecord{
		Index:    0, // Will be set by caller
		Scenario: obs.Scenario,
		RunID:    obs.RunID,
		Container: ContainerCleanupRecord{
			ID:     obs.Container.ID,
			Name:   obs.Container.Name,
			Status: string(obs.Container.Status),
		},
		Network: NetworkCleanupRecord{
			ID:     obs.Network.ID,
			Name:   obs.Network.Name,
			Status: string(obs.Network.Status),
		},
		Process: ProcessCleanupRecord{
			PID:       obs.Process.PID,
			StartTime: obs.Process.StartTime,
			Status:    string(obs.Process.Status),
		},
	}
}

// =============================================================================
// PERSIST NETWORK IDENTITY
// =============================================================================

// NetworkIdentity is the canonical type for network identity documents.
// P0-3 FIX: Shared type between writer and reader for round-trip safety.
type NetworkIdentity struct {
	SchemaVersion string `json:"schema_version"`
	ID            string `json:"id"`
	Name          string `json:"name"`
}

// WriteNetworkIdentity writes the network identity to network-identity.json.
func WriteNetworkIdentity(runDir, networkID, networkName string) error {
	identity := NetworkIdentity{
		SchemaVersion: "1.0.0",
		ID:            networkID,
		Name:          networkName,
	}

	data, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal network identity: %w", err)
	}

	path := filepath.Join(runDir, "network-identity.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write network identity: %w", err)
	}

	return nil
}

// ReadNetworkIdentity reads and validates the canonical network identity.
// P0-3 FIX: Uses shared NetworkIdentity type with schema version validation.
func ReadNetworkIdentity(runDir string) (string, string, error) {
	path := filepath.Join(runDir, "network-identity.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read network identity: %w", err)
	}

	// P0-3 FIX: Use strict single-document decoding with shared type
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var identity NetworkIdentity
	if err := dec.Decode(&identity); err != nil {
		return "", "", fmt.Errorf("parse network identity: %w", err)
	}

	// Ensure exactly one JSON document
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return "", "", errors.New("unexpected second JSON value in network identity")
		}
		return "", "", fmt.Errorf("trailing JSON data: %w", err)
	}

	// Validate schema version
	if identity.SchemaVersion != "1.0.0" {
		return "", "", fmt.Errorf("unsupported schema version: %s", identity.SchemaVersion)
	}

	if identity.ID == "" {
		return "", "", errors.New("network identity has empty id")
	}

	return identity.ID, identity.Name, nil
}
