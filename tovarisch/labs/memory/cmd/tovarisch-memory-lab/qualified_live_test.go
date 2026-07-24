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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/roots"
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
	imageID, err := docker.ResolveImageIdentity(ctx, liveSmokeImageRef)
	if err != nil {
		t.Fatalf("resolve image: %v", err)
	}

	// Run the production helper. The Run function stops the
	// container boundedly so waitForTerminalState observes the
	// non-running state.
	// CORRECTION27: Use canary binary to test docker exec-based reachability.
	opts := dockerlab.LifecycleOptions{
		ImageReference: liveSmokeImageRef,
		NetworkName:    netName,
		ContainerName:  runID,
		ContainerCmd:   []string{"/app/canary", "--mode=bounded", "--port=8080"},
		TerminalTimeout: 15 * time.Second,
		CleanupTimeout: 10 * time.Second,
		Run: func(runCtx context.Context, containerID string, observations *dockerlab.QualifiedExecutionObservations) error {
			// CORRECTION27: Test docker exec-based reachability
			// Use docker exec since direct HTTP may fail due to Docker bridge issues.
			canaryPort := 8080
			networkID := observations.Network.CreateResponseID

			// Try docker exec-based health check (CORRECTION27 path C)
			execExitCode := -1
			execErr := error(nil)
			for i := 0; i < 20; i++ {
				execExitCode, _, execErr = docker.ContainerExec(runCtx, containerID, []string{
					"sh", "-c",
					fmt.Sprintf("wget -qO- http://localhost:%d/health || wget -qO- http://127.0.0.1:%d/health || exit 1", canaryPort, canaryPort),
				})
				if execErr == nil && execExitCode == 0 {
					break
				}
				select {
				case <-runCtx.Done():
					observations.SetReachabilityFailed(dockerlab.ReachabilityMethodDockerExec, networkID, "timeout")
					return runCtx.Err()
				case <-time.After(500 * time.Millisecond):
				}
			}

			// Record reachability in observations
			if execExitCode == 0 && execErr == nil {
				observations.SetReachabilityDockerExec(networkID, execExitCode)
			} else {
				observations.SetReachabilityFailed(dockerlab.ReachabilityMethodDockerExec, networkID, fmt.Sprintf("exit code %d", execExitCode))
			}

			if err := docker.ContainerStop(runCtx, containerID, 5*time.Second); err != nil {
				return fmt.Errorf("bounded stop: %w", err)
			}
			return nil
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

	// Build provenance from the running controller binary.
	// Use canonical root resolver with explicit env.
	projRoots, explicitErr := roots.ResolveProjectRoots(
		os.Getenv("TOVARISCH_REPO_ROOT"),
		os.Getenv("TOVARISCH_MEMORY_MODULE_ROOT"),
		"", // no start dir - require explicit env
	)
	if explicitErr != nil {
		// Fall back to searching upward from CWD for development.
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			t.Fatalf("resolve project roots: explicit resolution failed: %v; cwd failed: %v", explicitErr, cwdErr)
		}
		projRoots, explicitErr = roots.ResolveProjectRoots("", "", cwd)
		if explicitErr != nil {
			t.Fatalf("resolve project roots: explicit resolution failed: %v; fallback failed: %v", explicitErr, explicitErr)
		}
	}
	cp, err := evidence.CollectControllerProvenance(evidence.ProvenanceOptions{
		RepoDir:        projRoots.Repository,
		ProducerVersion: "qualified-live-smoke/1.0.0",
	})
	if err != nil {
		// Test binaries may not have embedded VCS info. Fall back to a
		// direct git rev-parse on the working tree (the test must
		// pass when the source tree is reachable).
		fallbackCP, ferr := fallbackGitProvenance(projRoots.Repository, "qualified-live-smoke/1.0.0")
		if ferr != nil {
			t.Fatalf("collect controller provenance: %v; git fallback: %v", err, ferr)
		}
		cp = fallbackCP
	}

	// Install provenance into the observations.
	obs := outcome.Observations
	dockerVer := ""
	if v, err := docker.ServerVersion(ctx); err == nil {
		dockerVer = v.Version
	}
	execHash := cp.ExecutableSHA256
	if execHash == "" {
		// Fallback: compute from os.Executable.
		if exe, err := os.Executable(); err == nil {
			if data, err := osReadFile(exe); err == nil {
				sum := sha256.Sum256(data)
				execHash = hex.EncodeToString(sum[:])
			}
		}
	}
	obs.SetProvenance(cp.VCSRevision, cp.VCSTree, cp.GitObjectFormat, dockerVer, cp.ProducerVersion, execHash)
	obs.SetProvenanceDirty(cp.WorkingTreeDirty, cp.SourceCommitDirty)
	obs.SetVCSModified(cp.VCSModified)

	// Build canonical evidence and persist with fail-closed.
	// PersistQualifiedExecutionEvidence compares the supplied
	// derived claims to the recomputed ones, so the producer must
	// stamp the claims on the in-memory artifact before persisting.
	ev := evidence.BuildEvidenceFromObservations(obs)
	ev.SetDerivedFields()
	if err := evidence.PersistQualifiedExecutionEvidence("/tmp", ev); err != nil {
		// Failure close: the smoke FAILS the test on persistence error.
		raw, _ := json.MarshalIndent(ev, "", "  ")
		t.Fatalf("persist evidence: %v\n%s", err, string(raw))
	}
	defer func() { _ = os.Remove("/tmp/qualified-execution-evidence.json") }()
	persisted, err := osReadFile("/tmp/qualified-execution-evidence.json")
	if err != nil {
		t.Fatalf("read persisted evidence: %v", err)
	}
	result, err := evidence.VerifyQualifiedExecutionBytes(persisted)
	if err != nil {
		t.Fatalf("verify bytes: %v", err)
	}
	if !result.Pass {
		raw, _ := json.MarshalIndent(ev, "", "  ")
		t.Fatalf("verifier rejected live evidence:\n%s\nerrors: %v", string(raw), result.Errors)
	}

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


// (CORRECTION22: fallbackGitProvenance, gitOutput,
// gitWorkingTreeDirtyOutput, newGitCmd, osRemove and osReadFile
// moved to main.go so the smoke and the production CLI share the
// exact same provenance helpers. The tests still call them via
// the production symbol.)
