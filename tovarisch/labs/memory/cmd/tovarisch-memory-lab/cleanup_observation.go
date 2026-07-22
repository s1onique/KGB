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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// =============================================================================
// CLEANUP OBSERVER SEAM
// =============================================================================

// CleanupObserver observes cleanup state through Docker inspection.
type CleanupObserver struct {
	dockerPath string
}

// NewCleanupObserver creates a new cleanup observer.
func NewCleanupObserver() *CleanupObserver {
	return &CleanupObserver{
		dockerPath: "docker",
	}
}

// =============================================================================
// CONTAINER OBSERVATION
// =============================================================================

// ObserveContainerCleanup inspects the exact container ID and returns its status.
// Uses "docker container inspect" to avoid name-based matching.
func (o *CleanupObserver) ObserveContainerCleanup(ctx context.Context, containerID string) (*ContainerIdentityObservation, error) {
	if containerID == "" {
		return nil, errors.New("container ID is empty")
	}

	// P0-2 FIX: Use docker container inspect (--type is not valid on object-specific command)
	args := []string{"container", "inspect", containerID}
	result, err := o.runDockerCommand(ctx, args)
	if err != nil {
		// Docker daemon error, network failure, etc.
		return &ContainerIdentityObservation{
			ID:     containerID,
			Status: ObjectUnavailable,
		}, nil // Return available status, not error
	}

	// Parse result
	if result.ExitCode != 0 {
		// Non-zero exit code - check for "not found"
		if isNotFoundError(string(result.Stderr)) {
			return &ContainerIdentityObservation{
				ID:     containerID,
				Status: ObjectGone,
			}, nil
		}
		// Other error
		return &ContainerIdentityObservation{
			ID:     containerID,
			Status: ObjectUnavailable,
		}, nil
	}

	// Zero exit code - container exists
	var containers []ContainerInspectResult
	if err := json.Unmarshal(result.Stdout, &containers); err != nil {
		return &ContainerIdentityObservation{
			ID:     containerID,
			Status: ObjectUnavailable,
		}, nil
	}

	if len(containers) == 0 {
		// Empty array means not found
		return &ContainerIdentityObservation{
			ID:     containerID,
			Status: ObjectGone,
		}, nil
	}

	// Container exists
	return &ContainerIdentityObservation{
		ID:     containers[0].ID,
		Name:   strings.TrimPrefix(containers[0].Name, "/"),
		Status: ObjectExists,
	}, nil
}

// ContainerInspectResult represents Docker container inspect output.
type ContainerInspectResult struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
}

// =============================================================================
// NETWORK OBSERVATION
// =============================================================================

// ObserveNetworkCleanup inspects the exact network ID and returns its status.
// Uses "docker network inspect" to avoid name-based matching.
func (o *CleanupObserver) ObserveNetworkCleanup(ctx context.Context, networkID string) (*NetworkIdentityObservation, error) {
	if networkID == "" {
		return nil, errors.New("network ID is empty")
	}

	// P0-2 FIX: Use docker inspect --type=network (--type only valid on generic inspect)
	args := []string{"inspect", "--type=network", networkID}
	result, err := o.runDockerCommand(ctx, args)
	if err != nil {
		// Docker daemon error, network failure, etc.
		return &NetworkIdentityObservation{
			ID:     networkID,
			Status: ObjectUnavailable,
		}, nil
	}

	// Parse result
	if result.ExitCode != 0 {
		// Non-zero exit code - check for "not found"
		if isNotFoundError(string(result.Stderr)) {
			return &NetworkIdentityObservation{
				ID:     networkID,
				Status: ObjectGone,
			}, nil
		}
		// Other error
		return &NetworkIdentityObservation{
			ID:     networkID,
			Status: ObjectUnavailable,
		}, nil
	}

	// Zero exit code - network exists
	var networks []NetworkInspectResult
	if err := json.Unmarshal(result.Stdout, &networks); err != nil {
		return &NetworkIdentityObservation{
			ID:     networkID,
			Status: ObjectUnavailable,
		}, nil
	}

	if len(networks) == 0 {
		// Empty array means not found
		return &NetworkIdentityObservation{
			ID:     networkID,
			Status: ObjectGone,
		}, nil
	}

	// Network exists
	return &NetworkIdentityObservation{
		ID:     networks[0].ID,
		Name:   networks[0].Name,
		Status: ObjectExists,
	}, nil
}

// NetworkInspectResult represents Docker network inspect output.
type NetworkInspectResult struct {
	ID   string `json:"Id"`
	Name string `json:"Name"`
}

// =============================================================================
// PROCESS OBSERVATION
// =============================================================================

// ObserveProcessCleanup checks if the subject process with given PID and start time is gone.
func (o *CleanupObserver) ObserveProcessCleanup(ctx context.Context, pid int, expectedStart uint64) (*ProcessIdentityObservation, error) {
	if pid <= 0 {
		return &ProcessIdentityObservation{
			PID:    pid,
			Status: ProcessUnavailableCode,
		}, nil
	}

	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// P0-3 FIX: Preserve expected start time for gone process
			// This allows validation to succeed when process is actually gone
			return &ProcessIdentityObservation{
				PID:       pid,
				StartTime: expectedStart,
				Status:    ProcessGoneCode,
			}, nil
		}
		return &ProcessIdentityObservation{
			PID:    pid,
			Status: ProcessUnavailableCode,
		}, nil
	}

	actualStart, parseErr := parseProcStatStartTime(data)
	if parseErr != nil {
		return &ProcessIdentityObservation{
			PID:    pid,
			Status: ProcessUnavailableCode,
		}, nil
	}

	if actualStart == expectedStart {
		return &ProcessIdentityObservation{
			PID:       pid,
			StartTime: actualStart,
			Status:    ProcessStillAliveCode,
		}, nil
	}

	// PID reused
	return &ProcessIdentityObservation{
		PID:       pid,
		StartTime: actualStart,
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
	procStartTime, err := parseUint64(startTimeStr)
	if err != nil {
		return 0, fmt.Errorf("invalid starttime: %w", err)
	}

	return procStartTime, nil
}

// parseUint64 parses a byte slice as uint64.
func parseUint64(data []byte) (uint64, error) {
	var result uint64
	for _, b := range data {
		if b < '0' || b > '9' {
			return 0, errors.New("invalid digit")
		}
		result = result*10 + uint64(b-'0')
	}
	return result, nil
}

// =============================================================================
// DOCKER COMMAND RUNNER
// =============================================================================

// runDockerCommand executes a docker command with timeout.
func (o *CleanupObserver) runDockerCommand(ctx context.Context, args []string) (*DockerCommandResult, error) {
	cmd := exec.CommandContext(ctx, o.dockerPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return &DockerCommandResult{
				Stdout:   stdout.Bytes(),
				Stderr:   stderr.Bytes(),
				ExitCode: exitErr.ExitCode(),
			}, nil
		}
		return &DockerCommandResult{
			Stdout: stdout.Bytes(),
			Stderr: stderr.Bytes(),
		}, err
	}

	return &DockerCommandResult{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: 0,
	}, nil
}

// isNotFoundError checks if the error indicates "not found".
func isNotFoundError(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "no such") ||
		strings.Contains(lower, "not found") ||
		strings.Contains(lower, "does not exist")
}

// =============================================================================
// OBSERVER FACTORY
// =============================================================================

// DefaultCleanupObserver is the default cleanup observer instance.
var DefaultCleanupObserver = NewCleanupObserver()

// ObserveRunCleanup performs complete cleanup observation for a single run.
func ObserveRunCleanup(ctx context.Context, run *VerifiedRun, observer *CleanupObserver) (*RunCleanupObservation, error) {
	if observer == nil {
		observer = DefaultCleanupObserver
	}

	obs := &RunCleanupObservation{
		RunID:    run.DeclaredRunID,
		Scenario: run.DeclaredScenario,
	}

	// Observe container
	containerObs, err := observer.ObserveContainerCleanup(ctx, run.ContainerID)
	if err == nil && containerObs != nil {
		obs.Container = ContainerIdentityObservation{
			ID:     containerObs.ID,
			Name:   containerObs.Name,
			Status: containerObs.Status,
		}
	}

	// Observe network
	networkObs, err := observer.ObserveNetworkCleanup(ctx, run.NetworkID)
	if err == nil && networkObs != nil {
		obs.Network = NetworkIdentityObservation{
			ID:     networkObs.ID,
			Name:   networkObs.Name,
			Status: networkObs.Status,
		}
	}

	// Observe process
	processObs, err := observer.ObserveProcessCleanup(ctx, run.SubjectPID, run.SubjectStartTime)
	if err == nil && processObs != nil {
		obs.Process = ProcessIdentityObservation{
			PID:       processObs.PID,
			StartTime: processObs.StartTime,
			Status:    processObs.Status,
		}
	}

	return obs, nil
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

// WriteNetworkIdentity writes the network identity to network-identity.json.
func WriteNetworkIdentity(runDir, networkID, networkName string) error {
	identity := struct {
		SchemaVersion string `json:"schema_version"`
		ID            string `json:"id"`
		Name          string `json:"name"`
	}{
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
