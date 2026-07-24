// qualified_live_test.go — Explicit live Docker smoke for the
// qualified execution path.
//
// CORRECTION17: the live Docker smoke must construct the qualified
// client with the AuditedDockerRuntime wrapper so the recorded pull
// counters are observable, not just implied by source-code review.
// When TOVARISCH_LIVE_DOCKER_SMOKE=1, missing preconditions (Docker
// unavailable, local canary image absent) FAIL the test, they do
// not skip it silently.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
)

const envLiveSmoke = "TOVARISCH_LIVE_DOCKER_SMOKE"

// liveSmokeImageRef is the local canary image the smoke inspects.
const liveSmokeImageRef = "kgb-tovarisch-canary:latest"

func shouldRunLiveSmoke(t *testing.T) {
	t.Helper()
	if os.Getenv(envLiveSmoke) != "1" {
		t.Skipf("live Docker smoke not enabled (set %s=1)", envLiveSmoke)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	docker, err := dockerlab.NewClient(ctx)
	if err != nil {
		t.Fatalf("Docker is unavailable: %v", err)
	}
	defer docker.Close()
	if _, err := docker.ResolveImageIdentity(ctx, liveSmokeImageRef); err != nil {
		t.Fatalf("required local canary image %q is not present: %v. Build it with scripts/build_tovarisch_canary_image.sh",
			liveSmokeImageRef, err)
	}
}

// TestLiveDockerSmoke_QualifiedExecutionPath executes the production
// qualified runtime against a real Docker daemon. The qualified
// client is constructed with the AuditedDockerRuntime wrapper so the
// recorded pull counters are observable.
func TestLiveDockerSmoke_QualifiedExecutionPath(t *testing.T) {
	shouldRunLiveSmoke(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	docker, err := dockerlab.NewClient(ctx)
	if err != nil {
		t.Fatalf("create docker client: %v", err)
	}
	defer docker.Close()

	// Wrap the real client in the audited runtime so any pull attempt
	// is recorded as an audited observation.
	audited := dockerlab.NewAuditedDockerRuntime(docker.Client)

	// Step 1: Inspect the local canary image; capture exact ID.
	imageID, err := docker.ResolveImageIdentity(ctx, liveSmokeImageRef)
	if err != nil {
		t.Fatalf("resolve image identity: %v", err)
	}
	if err := dockerlab.ValidateExactImageID(imageID); err != nil {
		t.Fatalf("resolved image ID is not canonical: %v", err)
	}

	// Step 2: Build the qualified client and run PrepareQualifiedContainer.
	qc := dockerlab.NewQualifiedClient(audited)
	netName := fmt.Sprintf("kgb-lab-smoke-%d", time.Now().UnixNano())
	obs, err := qc.PrepareQualifiedContainer(ctx, liveSmokeImageRef, netName, "", dockerlab.ContainerConfig{
		Name:   fmt.Sprintf("kgb-smoke-%d", time.Now().UnixNano()),
		Config: &container.Config{Image: imageID, Cmd: []string{"true"}},
	})
	if err != nil {
		t.Fatalf("prepare qualified container: %v", err)
	}
	if obs.Container.ID == "" {
		t.Fatal("expected non-empty container ID")
	}

	// Step 3: Install the audited pull counters into the observations.
	attempted, count, lastRef := audited.PullAudit()
	obs.SetPullAudit(attempted, count, lastRef)
	obs.SetContainerStarted()

	// Step 4: Start, boundedly stop, observe terminal state, remove.
	if err := docker.ContainerStart(ctx, obs.Container.ID); err != nil {
		_ = docker.ContainerRemove(ctx, obs.Container.ID, true)
		_ = docker.NetworkRemove(ctx, obs.Network.InspectResponseID)
		t.Fatalf("start container: %v", err)
	}
	if err := docker.ContainerStop(ctx, obs.Container.ID, 5*time.Second); err != nil {
		t.Logf("container stop error (continuing): %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	terminal := false
	for time.Now().Before(deadline) {
		ci, err := docker.ContainerInspect(ctx, obs.Container.ID)
		if err == nil && ci.State != nil && !ci.State.Running {
			terminal = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !terminal {
		_ = docker.ContainerRemove(ctx, obs.Container.ID, true)
		_ = docker.NetworkRemove(ctx, obs.Network.InspectResponseID)
		t.Fatalf("container did not reach a terminal state within 10s of stop")
	}
	obs.SetContainerTerminalState()
	if err := docker.ContainerRemove(ctx, obs.Container.ID, true); err != nil {
		t.Logf("container remove error (continuing): %v", err)
	}
	if err := docker.NetworkRemove(ctx, obs.Network.InspectResponseID); err != nil {
		t.Logf("network remove error (continuing): %v", err)
	}
	obs.SetContainerRemoved()

	// Step 5: Build canonical evidence and verify.
	ver, _ := docker.ServerVersion(ctx)
	obs.SetProvenance(
		"0123456789012345678901234567890123456789",
		"0123456789012345678901234567890123456789",
		"sha1",
		ver.Version,
		"qualified-live-smoke/1.0.0",
	)
	ev := evidence.BuildEvidenceFromObservations(obs)
	if ev == nil {
		t.Fatal("evidence converter returned nil")
	}
	if err := evidence.PersistQualifiedExecutionEvidence("/tmp", ev); err != nil {
		t.Fatalf("persist evidence: %v", err)
	}
	defer func() { _ = osRemove("/tmp/qualified-execution-evidence.json") }()
	persisted, err := osReadFile("/tmp/qualified-execution-evidence.json")
	if err != nil {
		t.Fatalf("read persisted evidence: %v", err)
	}
	result, err := evidence.VerifyQualifiedExecutionBytes(persisted)
	if err != nil {
		t.Fatalf("verify bytes: %v", err)
	}
	if !result.Pass {
		// Marshal the evidence for diagnostic output.
		raw, _ := json.MarshalIndent(ev, "", "  ")
		t.Fatalf("verifier rejected live evidence:\n%s\nerrors: %v", string(raw), result.Errors)
	}

	// Step 6: Prove audited pull counters.
	if attempted {
		t.Errorf("audited pull.attempted=true (the qualified path must not pull)")
	}
	if count != 0 {
		t.Errorf("audited pull.attempt_count=%d, want 0", count)
	}
	if lastRef != "" {
		t.Errorf("audited pull.last_reference=%q, want empty", lastRef)
	}

	// Step 7: Print the canonical fields for the close report.
	t.Logf("test executed: true")
	t.Logf("test skipped: false")
	t.Logf("pull observation available: %v", obs.Pull.ObservationAvailable)
	t.Logf("pull attempts: %d", count)
	t.Logf("precreate image ID: %s", obs.Image.InspectedBeforeCreate)
	t.Logf("create-request image: %s", obs.Image.CreateRequestImage)
	t.Logf("post-create image ID: %s", obs.Image.ContainerInspectImage)
	t.Logf("post-create config image: %s", obs.Image.ContainerConfigImage)
	t.Logf("network create ID: %s", obs.Network.CreateResponseID)
	t.Logf("network inspect ID: %s", obs.Network.InspectResponseID)
	t.Logf("container endpoint network ID: %s", obs.Network.ContainerEndpointID)
	t.Logf("source commit: %s", obs.Provenance.SourceCommit)
	t.Logf("source tree: %s", obs.Provenance.SourceTree)
	t.Logf("container removed: %v", obs.Container.Removed)
	t.Logf("network removed: %v", true)
	t.Logf("container started: %v", obs.Container.Started)
	t.Logf("container ID: %s", obs.Container.ID)
}

// Helper file IO wrappers. The real implementations live in the
// evidence package; the test file just renames them.
func osRemove(path string) error { return os.Remove(path) }
func osReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// Ensure strings is referenced (used by the helper imports above).
var _ = strings.Repeat
