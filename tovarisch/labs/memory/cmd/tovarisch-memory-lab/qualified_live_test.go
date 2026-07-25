// qualified_live_test.go — Explicit live Docker smoke for the
// qualified execution path.
//
// CORRECTION18: the live smoke executes the same production
// helper used by runCommand (executeQualifiedDockerLifecycle).
// The smoke fails closed when:
//   - Docker is unavailable;
//   - the local canary image is absent;
//   - a pull is attempted;
//   - provenance is unavailable or dirty;
//   - terminal state is unproven;
//   - container/network cleanup is unproven;
//   - persisted evidence does not pass.

package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/roots"
)

const envLiveSmoke = "TOVARISCH_LIVE_DOCKER_SMOKE"

func liveSmokeImageRef() string {
	if exact := os.Getenv("TOVARISCH_LIVE_SMOKE_IMAGE"); exact != "" {
		return exact
	}
	return "kgb-tovarisch-canary:latest"
}

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
	if _, err := docker.ResolveImageIdentity(ctx, liveSmokeImageRef()); err != nil {
		t.Fatalf("required local canary image %q is not present: %v. Build it with scripts/build_tovarisch_canary_image.sh",
			liveSmokeImageRef(), err)
	}
}

// TestLiveDockerSmoke_QualifiedExecutionPath executes the production
// qualified lifecycle against a real Docker daemon via the same
// helper used by runCommand. The smoke uses the audited runtime
// implicitly via the shared helper; pull observations are
// instrumented.
func TestLiveDockerSmoke_QualifiedExecutionPath(t *testing.T) {
	shouldRunLiveSmoke(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	docker, err := dockerlab.NewClient(ctx)
	if err != nil {
		t.Fatalf("create docker client: %v", err)
	}
	defer docker.Close()

	runID := fmt.Sprintf("kgb-smoke-%d", time.Now().UnixNano())
	netName := fmt.Sprintf("kgb-lab-smoke-%d", time.Now().UnixNano())

	// Pre-resolve the canary image so we know the exact ID the
	// runtime must produce.
	imageID, err := docker.ResolveImageIdentity(ctx, liveSmokeImageRef())
	if err != nil {
		t.Fatalf("resolve image: %v", err)
	}

	// Run the production helper. The Run function stops the
	// container boundedly so waitForTerminalState observes the
	// non-running state.
	// CORRECTION27: Use canary binary to test docker exec-based reachability.
	opts := dockerlab.LifecycleOptions{
		ImageReference:  liveSmokeImageRef(),
		NetworkName:     netName,
		ContainerName:   runID,
		ContainerCmd:    []string{"/app/canary", "--mode=bounded", "--port=8080"},
		TerminalTimeout: 15 * time.Second,
		CleanupTimeout:  10 * time.Second,
		Run: func(runCtx context.Context, input dockerlab.QualifiedWorkloadInput) (*dockerlab.QualifiedWorkloadResult, error) {
			port := 8080
			if cp, ok := os.LookupEnv("TOVARISCH_CANARY_PORT"); ok {
				if v, err := strconv.Atoi(cp); err == nil && v > 0 && v <= 65535 {
					port = v
				}
			}
			expectedRequest := 100
			if cr, ok := os.LookupEnv("TOVARISCH_CANARY_REQUEST"); ok {
				if v, err := strconv.Atoi(cr); err == nil && v > 0 {
					expectedRequest = v
				}
			}
			control, err := dockerlab.NewDockerControl(docker.Client)
			if err != nil {
				return nil, fmt.Errorf("construct canonical control: %w", err)
			}
			workloadObs := &dockerlab.QualifiedExecutionObservations{Reachability: dockerlab.ReachabilityObservations{
				Method: dockerlab.ReachabilityMethodDockerExec, NetworkID: input.NetworkID,
				TargetHost: "127.0.0.1", TargetPort: port,
			}}
			_, _, _, err = RunCanonicalControlSequence(runCtx, control, workloadObs, CanonicalControlSequenceOptions{
				ContainerID: input.ContainerID, Port: port, Operations: expectedRequest, Timeout: 30 * time.Second,
			})
			if err != nil {
				return nil, fmt.Errorf("canonical control sequence: %w", err)
			}
			workloadObs.Reachability.Success = true
			if err := docker.ContainerStop(runCtx, input.ContainerID, 5*time.Second); err != nil {
				return nil, fmt.Errorf("bounded stop: %w", err)
			}
			return &dockerlab.QualifiedWorkloadResult{Observations: dockerlab.QualifiedWorkloadObservations{
				Reachability: workloadObs.Reachability,
			}}, nil
		},
	}
	outcome, err := dockerlab.ExecuteQualifiedDockerLifecycle(ctx, docker, opts, "qualified-live-smoke/1.0.0")
	if err != nil {
		t.Fatalf("execute qualified lifecycle: %v", err)
	}
	if !outcome.Terminal {
		t.Fatal("lifecycle did not reach a terminal state")
	}
	if !outcome.ContainerRemoved || !outcome.NetworkRemoved {
		t.Fatalf("cleanup incomplete: container=%v network=%v", outcome.ContainerRemoved, outcome.NetworkRemoved)
	}

	// Cross-check: the image ID in the outcome matches the
	// pre-resolved exact ID.
	if outcome.ImageID != imageID {
		t.Fatalf("outcome image ID %q != pre-resolved %q", outcome.ImageID, imageID)
	}

	projRoots, explicitErr := roots.ResolveProjectRoots(
		os.Getenv("TOVARISCH_REPO_ROOT"), os.Getenv("TOVARISCH_MEMORY_MODULE_ROOT"), "",
	)
	if explicitErr != nil {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			t.Fatalf("resolve project roots: %v", cwdErr)
		}
		projRoots, explicitErr = roots.ResolveProjectRoots("", "", cwd)
		if explicitErr != nil {
			t.Fatalf("resolve project roots: %v", explicitErr)
		}
	}
	dockerVersion, err := docker.ServerVersion(ctx)
	if err != nil {
		t.Fatalf("Docker server version: %v", err)
	}
	cp, err := evidence.CollectControllerProvenance(evidence.ProvenanceOptions{
		RepoDir: projRoots.Repository, ProducerVersion: "qualified-live-smoke/1.0.0",
		DockerServerVersion: dockerVersion.Version,
	})
	if err != nil {
		t.Fatalf("collect running-binary provenance: %v", err)
	}
	artifactDir := os.Getenv("TOVARISCH_QUALIFIED_EVIDENCE_DIR")
	if artifactDir == "" {
		artifactDir = t.TempDir()
	}
	ev, err := evidence.BuildAndPersistFinalQualifiedEvidence(ctx, outcome, cp, artifactDir)
	if err != nil {
		t.Fatalf("persist final qualified evidence: %v", err)
	}
	result := evidence.VerifyQualifiedExecution(ev)
	if !result.Pass {
		t.Fatalf("verifier rejected live evidence: %v", result.Errors)
	}
	obs := outcome.Observations

	// Print the canonical fields for the close report.
	t.Logf("test executed: true")
	t.Logf("test skipped: false")
	t.Logf("controller source commit: %s", cp.VCSRevision)
	t.Logf("controller source tree: %s", cp.VCSTree)
	t.Logf("controller vcs modified: %v", cp.VCSModified)
	t.Logf("controller executable sha256: %s", cp.ExecutableSHA256)
	t.Logf("pull observation available: %v", obs.Pull.ObservationAvailable)
	t.Logf("pull attempts: %d", obs.Pull.AttemptCount)
	t.Logf("precreate image ID: %s", obs.Image.InspectedBeforeCreate)
	t.Logf("create request image: %s", obs.Image.CreateRequestImage)
	t.Logf("postcreate image ID: %s", obs.Image.ContainerInspectImage)
	t.Logf("postcreate config image: %s", obs.Image.ContainerConfigImage)
	t.Logf("network create ID: %s", obs.Network.CreateResponseID)
	t.Logf("network inspect ID: %s", obs.Network.InspectResponseID)
	t.Logf("container endpoint network ID: %s", obs.Network.ContainerEndpointID)
	t.Logf("container terminal state observed: %v", obs.Container.TerminalStateObserved)
	t.Logf("container removed and absence verified: %v", obs.Container.Removed)
	t.Logf("network removed and absence verified: %v", obs.Network.Removed)
	t.Logf("persisted evidence pass: %v", result.Pass)
}

func parentDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			if i == 0 {
				return "/"
			}
			return p[:i]
		}
	}
	return "."
}

// Controller provenance is collected only by the canonical running-binary
// collector after the lifecycle returns.
