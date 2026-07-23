// docker_cleanup_integration_test.go — Docker Lifecycle Smoke Tests
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03-TERMINAL-QUALIFICATION01
//
// Environment-gated integration tests for Docker cleanup observation.
// Uses exact ID inspection to verify container and network lifecycle.

package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Docker test environment variable - must be explicitly enabled.
const envDockerTest = "TOVARISCH_MEMORY_LAB_DOCKER_TEST"

// shouldRunDockerTest returns true if Docker integration tests should run.
func shouldRunDockerTest() bool {
	return os.Getenv(envDockerTest) == "1"
}

// skipIfNoDockerTest skips the test if Docker integration tests are not enabled.
func skipIfNoDockerTest(t *testing.T) {
	if !shouldRunDockerTest() {
		t.Skip("Docker integration tests not enabled (set " + envDockerTest + "=1)")
	}
}

// dockerAvailable checks if Docker is available.
func dockerAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := RunDockerCommand(ctx, DefaultDockerCommandLimits(), "version")
	if err != nil {
		return false
	}
	return result.ExitCode == 0
}

// TestDockerCleanupObservationIntegration proves exact ID-based Docker lifecycle observation.
func TestDockerCleanupObservationIntegration(t *testing.T) {
	skipIfNoDockerTest(t)

	if !dockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx := context.Background()
	observer := NewCleanupObserver()

	// Generate unique test identifiers
	testID := "tovarisch-test-" + time.Now().Format("20060102-150405")
	networkName := testID + "-net"
	containerName := testID + "-ctr"

	// Track cleanup for best-effort teardown
	var createdNetworkID, createdContainerID string

	// Cleanup on test exit
	defer func() {
		// Remove container if exists
		if createdContainerID != "" {
			RunDockerCommand(ctx, DefaultDockerCommandLimits(), "rm", "-f", createdContainerID)
		}
		// Remove network if exists
		if createdNetworkID != "" {
			RunDockerCommand(ctx, DefaultDockerCommandLimits(), "network", "rm", createdNetworkID)
		}
	}()

	// Step 1: Create ephemeral Docker network
	createNetResult, err := RunDockerCommand(ctx, DefaultDockerCommandLimits(),
		"network", "create", "--driver", "bridge", networkName)
	if err != nil {
		t.Fatalf("create network failed: %v", err)
	}
	if createNetResult.ExitCode != 0 {
		t.Fatalf("create network exited %d: %s", createNetResult.ExitCode, string(createNetResult.Stderr))
	}

	// Step 2: Capture exact full network ID
	inspectNetResult, err := RunDockerCommand(ctx, DefaultDockerCommandLimits(),
		"network", "inspect", "--format={{.Id}}", networkName)
	if err != nil {
		t.Fatalf("inspect network failed: %v", err)
	}
	if inspectNetResult.ExitCode != 0 {
		t.Fatalf("inspect network exited %d: %s", inspectNetResult.ExitCode, string(inspectNetResult.Stderr))
	}

	createdNetworkID = strings.TrimSpace(string(inspectNetResult.Stdout))
	if createdNetworkID == "" {
		t.Fatal("network ID is empty")
	}

	// Step 3: Observe network as exists
	netObs, err := observer.ObserveNetworkCleanup(ctx, createdNetworkID)
	if err != nil {
		t.Fatalf("observe network failed: %v", err)
	}
	if netObs.Status != ObjectExists {
		t.Errorf("expected network exists, got %s", netObs.Status)
	}
	if netObs.ID != createdNetworkID {
		t.Errorf("network ID mismatch: expected %s, got %s", createdNetworkID, netObs.ID)
	}

	// Step 4: Create test container attached to network
	// Use alpine:latest if available, otherwise busybox
	imageName := "alpine:latest"
	pingResult, _ := RunDockerCommand(ctx, DefaultDockerCommandLimits(), "pull", imageName)
	if pingResult.ExitCode != 0 {
		imageName = "busybox:latest"
	}

	createCtrResult, err := RunDockerCommand(ctx, DefaultDockerCommandLimits(),
		"create", "--name", containerName, "--network", networkName, imageName, "sleep", "60")
	if err != nil {
		t.Fatalf("create container failed: %v", err)
	}
	if createCtrResult.ExitCode != 0 {
		t.Fatalf("create container exited %d: %s", createCtrResult.ExitCode, string(createCtrResult.Stderr))
	}

	// Step 5: Capture exact full container ID
	inspectCtrResult, err := RunDockerCommand(ctx, DefaultDockerCommandLimits(),
		"container", "inspect", "--format={{.Id}}", containerName)
	if err != nil {
		t.Fatalf("inspect container failed: %v", err)
	}
	if inspectCtrResult.ExitCode != 0 {
		t.Fatalf("inspect container exited %d: %s", inspectCtrResult.ExitCode, string(inspectCtrResult.Stderr))
	}

	createdContainerID = strings.TrimSpace(string(inspectCtrResult.Stdout))
	if createdContainerID == "" {
		t.Fatal("container ID is empty")
	}

	// Step 6: Observe container as exists
	ctrObs, err := observer.ObserveContainerCleanup(ctx, createdContainerID)
	if err != nil {
		t.Fatalf("observe container failed: %v", err)
	}
	if ctrObs.Status != ObjectExists {
		t.Errorf("expected container exists, got %s", ctrObs.Status)
	}
	if ctrObs.ID != createdContainerID {
		t.Errorf("container ID mismatch: expected %s, got %s", createdContainerID, ctrObs.ID)
	}

	// Step 7: Remove container
	rmCtrResult, err := RunDockerCommand(ctx, DefaultDockerCommandLimits(),
		"rm", "-f", containerName)
	if err != nil {
		t.Fatalf("remove container failed: %v", err)
	}
	if rmCtrResult.ExitCode != 0 {
		t.Fatalf("remove container exited %d: %s", rmCtrResult.ExitCode, string(rmCtrResult.Stderr))
	}

	// Step 8: Observe container as gone
	ctrGoneObs, err := observer.ObserveContainerCleanup(ctx, createdContainerID)
	if err != nil {
		t.Fatalf("observe container after removal failed: %v", err)
	}
	if ctrGoneObs.Status != ObjectGone {
		t.Errorf("expected container gone, got %s", ctrGoneObs.Status)
	}

	// Step 9: Remove network
	rmNetResult, err := RunDockerCommand(ctx, DefaultDockerCommandLimits(),
		"network", "rm", networkName)
	if err != nil {
		t.Fatalf("remove network failed: %v", err)
	}
	if rmNetResult.ExitCode != 0 {
		t.Fatalf("remove network exited %d: %s", rmNetResult.ExitCode, string(rmNetResult.Stderr))
	}

	// Step 10: Observe network as gone
	netGoneObs, err := observer.ObserveNetworkCleanup(ctx, createdNetworkID)
	if err != nil {
		t.Fatalf("observe network after removal failed: %v", err)
	}
	if netGoneObs.Status != ObjectGone {
		t.Errorf("expected network gone, got %s", netGoneObs.Status)
	}

	// P0-10 FIX: Log IDs before clearing for test verification
	t.Logf("container ID: %s", createdContainerID)
	t.Logf("network ID: %s", createdNetworkID)

	// Clear IDs to prevent duplicate cleanup in defer
	createdContainerID = ""
	createdNetworkID = ""
}

// TestDockerCleanupObservation_NonexistentIDs proves exact not-found handling.
func TestDockerCleanupObservation_NonexistentIDs(t *testing.T) {
	skipIfNoDockerTest(t)

	if !dockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx := context.Background()
	observer := NewCleanupObserver()

	// Use clearly nonexistent IDs
	nonexistentContainer := "0000000000000000000000000000000000000000000000000000000000000000"
	nonexistentNetwork := "0000000000000000000000000000000000000000000000000000000000000000"

	// Observe container as gone
	ctrObs, err := observer.ObserveContainerCleanup(ctx, nonexistentContainer)
	if err != nil {
		t.Fatalf("observe container failed: %v", err)
	}
	if ctrObs.Status != ObjectGone {
		t.Errorf("expected container gone, got %s", ctrObs.Status)
	}

	// Observe network as gone
	netObs, err := observer.ObserveNetworkCleanup(ctx, nonexistentNetwork)
	if err != nil {
		t.Fatalf("observe network failed: %v", err)
	}
	if netObs.Status != ObjectGone {
		t.Errorf("expected network gone, got %s", netObs.Status)
	}
}

// TestDockerCleanupObservation_ExactArgumentVectors proves exact Docker command arguments.
func TestDockerCleanupObservation_ExactArgumentVectors(t *testing.T) {
	skipIfNoDockerTest(t)

	if !dockerAvailable() {
		t.Skip("Docker not available")
	}

	// Record all Docker commands
	var recordedArgs [][]string
	originalRunner := func(ctx context.Context, limits DockerCommandLimits, args ...string) (DockerCommandResult, error) {
		recordedArgs = append(recordedArgs, args)
		return DockerCommandResult{ExitCode: 1, Stderr: []byte("no such object")}, nil
	}

	observer, err := NewCleanupObserverWithRunner(originalRunner, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	observer.ObserveContainerCleanup(ctx, "abc123def456")
	observer.ObserveNetworkCleanup(ctx, "net123")

	// Verify container inspect uses exact arguments
	if len(recordedArgs) < 1 {
		t.Fatal("no commands recorded")
	}

	containerArgs := recordedArgs[0]
	if containerArgs[0] != "container" || containerArgs[1] != "inspect" {
		t.Errorf("container args: %v", containerArgs)
	}

	networkArgs := recordedArgs[1]
	if networkArgs[0] != "network" || networkArgs[1] != "inspect" {
		t.Errorf("network args: %v", networkArgs)
	}
}
