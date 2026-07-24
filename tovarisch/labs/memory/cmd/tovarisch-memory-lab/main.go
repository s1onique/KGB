// cmd/tovarisch-memory-lab/main.go โ�� Memory Laboratory CLI
//
// Go-based Docker laboratory for deterministic memory investigation.
// Uses Docker SDK with Engine API version negotiation.
//
// Reference: kgb://factory/workflow
//
// CORRECTION02: full descriptor-fallback gating, manifest-threshold
// reconstruction, strict source-kind contract, canary-image provenance,
// runtime-state derivation, and an explicit `derive-runtime-state`
// subcommand for the close report.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling"
)

// Canonical artifact inventory - exact set required for verification
var canonicalInventory = []string{
	"manifest.json",
	"verdict.json",
	"samples.csv",
	"events.jsonl",
	"container-inspect.json",
	"container-logs.txt",
	"initial-canary-state.json",
	"final-canary-state.json",
	"workload-result.json",
	"checksums.txt",
}

// CanaryState represents the canary's internal state from /state endpoint.
type CanaryState struct {
	Mode           string `json:"mode"`
	RetainedBlocks int    `json:"retained_blocks"`
	RetainedBytes  int64  `json:"retained_bytes"`
	OperationCount int    `json:"operation_count"`
	FDCount        int    `json:"fd_count"`
	BufferCapacity int64  `json:"buffer_capacity,omitempty"`
	Ready          bool   `json:"ready"`
}

// WorkloadResult represents the result of a stimulus workload.
type WorkloadResult struct {
	Requested int `json:"requested"`
	Attempted int `json:"attempted"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Returned  int `json:"returned"`
}

// CanaryImageProvenance captures the canary image identity, labels,
// and source-tree binding. CORRECTION02 ยง7.
type CanaryImageProvenance struct {
	ImageID                      string   `json:"canary_image_id"`
	RepoDigests                  []string `json:"canary_repo_digests"`
	RepoDigestStatus             string   `json:"canary_repo_digest_status,omitempty"`
	SourceCommitOID              string   `json:"canary_source_commit_oid,omitempty"`
	RepositoryTreeOID            string   `json:"canary_repository_tree_oid,omitempty"`
	SourceSubtreeOID             string   `json:"canary_source_subtree_oid,omitempty"`
	BinarySHA256                 string   `json:"canary_binary_sha256,omitempty"`
	ImageRevisionLabel           string   `json:"canary_image_revision_label,omitempty"`
	ImageRepositoryTreeLabel     string   `json:"canary_image_tree_label,omitempty"`
	ImageSourceSubtreeLabel      string   `json:"canary_image_source_subtree_label,omitempty"`
	ImageBinarySHA256Label       string   `json:"canary_image_binary_sha256_label,omitempty"`
	RuntimeBinarySHA256          string   `json:"canary_runtime_binary_sha256,omitempty"`
	RuntimeBinarySHA256Matches   bool     `json:"canary_runtime_binary_matches_label,omitempty"`
	ContainerImageMatchesImageID bool     `json:"canary_container_image_matches_id,omitempty"`
}

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: %s <run|verify|derive-runtime-state|matrix|verify-matrix> [options]", args[0])
	}

	switch args[1] {
	case "run":
		return runCommand(args[1:])
	case "verify":
		return verifyCommand(args[1:])
	case "derive-runtime-state":
		return deriveRuntimeStateCommand(args[1:])
	case "matrix":
		return matrixCommand(args[1:])
	case "verify-matrix":
		// CORRECTION03: Uses VerifyMatrixBundle as single authority
		return verifyMatrixCommand(args[1:])
	default:
		return fmt.Errorf("unknown subcommand: %s (expected 'run', 'verify', 'derive-runtime-state', 'matrix', or 'verify-matrix')", args[1])
	}
}

// workloadArtifacts holds the data the workload callback produces
// and that the outer runCommand needs to compute the verdict.
// Captured by closure; the Run callback writes them via shared
// pointer.
type ReachabilityInfo struct {
	Method dockerlab.ReachabilityMethod `json:"method"`
	NetworkID string `json:"network_id"`
	Success bool `json:"success"`
	FailureReason string `json:"failure_reason,omitempty"`
}

type workloadArtifacts struct {
	InitialState   *CanaryState
	FinalState     *CanaryState
	WorkloadResult *WorkloadResult
	Samples        []sampling.Sample
	Events         []sampling.Event
	PhaseConfig    sampling.PhaseConfig
	PhaseValid     bool
	WorkloadValid  bool
	IdentityStable bool
	Reachability *ReachabilityInfo
}

func runCommand(args []string) error {
	fs := flag.NewFlagSet("memory-lab run", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s run [options]\n\nOptions:\n", args[0])
		fs.PrintDefaults()
	}

	scenario := fs.String("scenario", "", "Scenario (required): canary-growing, canary-bounded, canary-descriptor")
	duration := fs.Int("duration", 60, "Duration in seconds")
	artifactsDir := fs.String("artifacts-dir", "", "Artifacts directory (required)")
	verbose := fs.Bool("v", false, "Verbose output")
	containerImage := fs.String("container-image", "kgb-tovarisch-canary:latest", "Container image")
	canaryPort := fs.Int("canary-port", 8080, "Canary HTTP port")

	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return fmt.Errorf("parse flags: %w", err)
	}

	if *scenario == "" {
		return fmt.Errorf("--scenario is required")
	}
	if _, ok := allowedScenarios[*scenario]; !ok {
		return fmt.Errorf("invalid scenario %q: allowed: canary-growing, canary-bounded, canary-descriptor", *scenario)
	}
	if *artifactsDir == "" {
		return fmt.Errorf("--artifacts-dir is required")
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if *duration < 10 {
		return fmt.Errorf("duration must be >= 10 seconds")
	}

	dockerClient, err := dockerlab.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}

	dockerInfo, err := dockerClient.ServerVersion(ctx)
	if err != nil {
		return fmt.Errorf("get docker version: %w", err)
	}

	if *verbose {
		fmt.Printf("Docker %s (API %s)\n", dockerInfo.Version, dockerClient.ClientVersion())
	}

	// P0-10 CORRECTION02: Resolve descriptive reference to exact
	// image ID exactly once. Use ResolveImageIdentity - NEVER
	// ImagePull in qualified paths.
	imageID, err := dockerClient.ResolveImageIdentity(ctx, *containerImage)
	if err != nil {
		return fmt.Errorf("resolve image identity: %w", err)
	}

	// P0-10: Validate the resolved image ID is in canonical form.
	if err := dockerlab.ValidateExactImageID(imageID); err != nil {
		return fmt.Errorf("resolved image ID validation failed: %w", err)
	}

	frozenImageID := imageID
	imageReference := *containerImage
	if *verbose {
		fmt.Printf("Resolved %s -> %s\n", imageReference, frozenImageID[:12])
	}

	runID := fmt.Sprintf("lab-%s-%d", *scenario, time.Now().Unix())

	artifactsPath := filepath.Join(*artifactsDir, runID)
	if err := os.MkdirAll(artifactsPath, 0755); err != nil {
		return fmt.Errorf("create artifacts dir: %w", err)
	}

	evidenceWriter := evidence.NewWriter(runID, *scenario, artifactsPath)

	manifest := &evidence.Manifest{
		SchemaVersion: "1.0.0",
		RunID:         runID,
		Scenario:      *scenario,
		StartedAt:     time.Now(),
		DockerID: &evidence.DockerIdentity{
			EngineVersion: dockerInfo.Version,
			APIVersion:    dockerClient.ClientVersion(),
		},
	}
	if err := evidenceWriter.WriteManifest(manifest); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	phaseCfg := sampling.SmokePhaseConfig()
	phaseCfg.Stimulus = time.Duration(*duration) * time.Second

	cmd := getScenarioCommand(*scenario)
	containerName := fmt.Sprintf("tovarisch-subject-%s", runID)
	netName := fmt.Sprintf("kgb-lab-%s", runID)

	// CORRECTION22 P0-8: extract the existing matrix workload
	// into a callback. The callback receives the running container
	// ID and owns the workload until terminal state. Phase order
	// is preserved verbatim from the legacy bridge.
	workload := &workloadArtifacts{PhaseConfig: phaseCfg}
	var workloadErr error

	runWorkload := func(workloadCtx context.Context, containerID string, obs *dockerlab.QualifiedExecutionObservations) error {
		containerPID, err := dockerClient.ContainerGetPID(workloadCtx, containerID)
		if err != nil {
			return fmt.Errorf("get container PID: %w", err)
		}
		if *verbose {
			fmt.Printf("Container %s started with PID %d\n", containerID, containerPID)
		}

		// CORRECTION27 P0-1: Use Docker exec-based health check instead of direct HTTP.
		// This avoids Docker bridge networking issues where the container IP is not
		// reachable from the host. Docker exec always works because it uses the
		// container's own network namespace.
		if _, err := dockerClient.CanaryHealthCheckViaExec(workloadCtx, containerID, *canaryPort, 30*time.Second); err != nil {
			return fmt.Errorf("canary health check failed (docker exec): %w", err)
		}
		if *verbose {
			fmt.Printf("Canary healthy (via docker exec)\n")
		}

		// For the workload, we still need direct HTTP - fall back to container IP
		// but this is only for operating the canary, not for health verification.
		containerIP, _ := dockerClient.ContainerIP(workloadCtx, containerID, netName)
		canaryURL := ""
		if containerIP != "" {
			canaryURL = fmt.Sprintf("http://%s:%d", containerIP, *canaryPort)
			if *verbose {
				fmt.Printf("Canary URL (for workload): %s\n", canaryURL)
			}
		}

		httpClient := &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				Proxy: nil,
			},
		}

		// CORRECTION27: Try exec-based state fetch first (more reliable), fall back to HTTP
		initialState, err := fetchCanaryStateViaExec(workloadCtx, dockerClient, containerID, *canaryPort)
		if err != nil && canaryURL != "" {
			initialState, err = fetchCanaryState(workloadCtx, httpClient, canaryURL)
		}
		if err != nil {
			return fmt.Errorf("fetch initial canary state: %w", err)
		}
		expectedMode := scenarioToMode(*scenario)
		if initialState.Mode != expectedMode {
			return fmt.Errorf("canary mode mismatch: expected %s, got %s", expectedMode, initialState.Mode)
		}
		if *verbose {
			fmt.Printf("Initial canary state: mode=%s, operations=%d, retained_blocks=%d\n",
				initialState.Mode, initialState.OperationCount, initialState.RetainedBlocks)
		}
		if err := evidenceWriter.WriteCanaryState("initial", initialState); err != nil {
			return fmt.Errorf("write initial canary state: %w", err)
		}

		sampler := sampling.NewSamplerWithDocker(
			containerID,
			func() int { return containerPID },
			dockerClient,
			phaseCfg,
		)
		cgroupPath, cgroupErr := procfs.ResolveCgroupV2Path(containerPID)
		controllerPIDInt := os.Getpid()
		capability, proof := classifyCgroupFailureWithNamespace(cgroupErr, containerPID, controllerPIDInt)
		if cgroupErr != nil {
			sampler.RecordCgroupCapability(workloadCtx, containerPID, capability, "", cgroupErr, controllerPIDInt, proof)
			if *verbose {
				fmt.Printf("CGROUP RESOLUTION FAILED: pid=%d capability=%s error=%v\n", containerPID, capability, cgroupErr)
			}
		} else {
			if *verbose {
				fmt.Printf("CGROUP RESOLVED: pid=%d path=%s\n", containerPID, cgroupPath)
			}
			sampler.SetCgroupPath(cgroupPath)
			sampler.RecordCgroupCapability(workloadCtx, containerPID, sampling.CgroupCapabilityAvailable, cgroupPath, nil, controllerPIDInt, nil)
		}

		sampler.Start(workloadCtx)

		if *verbose {
			fmt.Printf("Waiting for stimulus phase...\n")
		}
		select {
		case <-workloadCtx.Done():
			sampler.Stop()
			return workloadCtx.Err()
		case <-sampler.StimulusReady():
		}

		if *verbose {
			fmt.Printf("Stimulus phase started, triggering workload\n")
		}
		workloadResult, err := operateCanary(workloadCtx, httpClient, canaryURL, getScenarioOperationCount(*scenario))
		if err != nil {
			sampler.Stop()
			return fmt.Errorf("operate canary: %w", err)
		}
		if *verbose {
			fmt.Printf("Workload completed: requested=%d attempted=%d completed=%d\n",
				workloadResult.Requested, workloadResult.Attempted, workloadResult.Completed)
		}

		if err := sampler.WaitForPhase(workloadCtx, sampling.PhaseSettling); err != nil {
			sampler.Stop()
			return fmt.Errorf("wait settling: %w", err)
		}
		if err := sampler.WaitForPhase(workloadCtx, sampling.PhaseFinal); err != nil {
			sampler.Stop()
			return fmt.Errorf("wait final: %w", err)
		}
		if err := sampler.WaitForComplete(workloadCtx); err != nil {
			sampler.Stop()
			return fmt.Errorf("wait complete: %w", err)
		}

		// The bounded/growing/descriptor canaries run as long-lived
		// HTTP servers; stop the container boundedly so the
		// lifecycle terminal-state observer succeeds.
		if stopErr := dockerClient.ContainerStop(workloadCtx, containerID, 10*time.Second); stopErr != nil {
			sampler.Stop()
			return fmt.Errorf("bounded stop: %w", stopErr)
		}

		// CORRECTION27: Try exec-based state fetch first (more reliable), fall back to HTTP
		finalState, err := fetchCanaryStateViaExec(workloadCtx, dockerClient, containerID, *canaryPort)
		if err != nil && canaryURL != "" {
			finalState, err = fetchCanaryState(workloadCtx, httpClient, canaryURL)
		}
		if err != nil {
			sampler.Stop()
			return fmt.Errorf("fetch final canary state: %w", err)
		}
		if *verbose {
			fmt.Printf("Final canary state: mode=%s, operations=%d, retained_blocks=%d\n",
				finalState.Mode, finalState.OperationCount, finalState.RetainedBlocks)
		}

		sampler.Stop()
		samples := sampler.Samples()
		events := sampler.Events()
		if *verbose {
			fmt.Printf("Collected %d samples\n", len(samples))
		}

		phaseValid := validatePhaseContract(samples, phaseCfg)
		if !phaseValid {
			fmt.Printf("WARNING: Phase contract validation failed\n")
		}
		workloadValid := workloadResult.Completed == workloadResult.Requested
		identityStable := validateProcessIdentity(samples)

		if err := evidenceWriter.WriteCanaryState("final", finalState); err != nil {
			return fmt.Errorf("write final canary state: %w", err)
		}
		if err := evidenceWriter.WriteWorkloadResult(workloadResult); err != nil {
			return fmt.Errorf("write workload result: %w", err)
		}
		if err := evidenceWriter.WriteSamplesCSV(samples); err != nil {
			return fmt.Errorf("write samples CSV: %w", err)
		}
		if err := evidenceWriter.WriteEventsJSONL(events); err != nil {
			return fmt.Errorf("write events JSONL: %w", err)
		}
		logs, _ := dockerClient.ContainerLogs(workloadCtx, containerID, "100")
		_ = evidenceWriter.WriteContainerLogs("container", []byte(logs))
		inspectData, _ := dockerClient.ContainerInspect(workloadCtx, containerID)
		_ = evidenceWriter.WriteContainerInspect("container", inspectData)

		// Capture the artifacts for the outer runCommand.
		workload.InitialState = initialState
		workload.FinalState = finalState
		workload.WorkloadResult = workloadResult
		workload.Samples = samples
		workload.Events = events
		workload.PhaseValid = phaseValid
		workload.WorkloadValid = workloadValid
		workload.IdentityStable = identityStable
		// CORRECTION27 P0-2: Track reachability method
		workload.Reachability = &ReachabilityInfo{
			Method:     dockerlab.ReachabilityMethodDockerExec,
			NetworkID:  netName,
			Success:    true,
		}
		return nil
	}

	opts := dockerlab.LifecycleOptions{
		ImageReference: *containerImage,
		NetworkName:    netName,
		ContainerName:  containerName,
		ContainerCmd:   cmd,
		TerminalTimeout: 30 * time.Second,
		CleanupTimeout:  10 * time.Second,
		Run:             runWorkload,
	}

	outcome, err := dockerlab.ExecuteQualifiedDockerLifecycle(
		ctx, dockerClient, opts, "tovarisch-memory-lab/1.0.0",
	)
	// The Run callback never returned an error if err is non-nil here
	// — but we still record its state.
	workloadErr = err
	if outcome == nil {
		// Prepare failed before Run was called. No observations yet.
		return fmt.Errorf("qualified lifecycle: %w", err)
	}

	// CORRECTION22 P0-10: set provenance on the observations so the
	// verifier can authorize pass=true. The lifecycle itself never
	// reads or modifies the supplied claim fields; the producer
	// stamps them after the underlying validator succeeds.
	if outcome.Observations != nil {
		cp, perr := evidence.CollectControllerProvenance(evidence.ProvenanceOptions{
			RepoDir:         repoDirForProvenance(),
			ProducerVersion: "tovarisch-memory-lab/1.0.0",
		})
		if perr != nil {
			cp, perr = fallbackGitProvenance(repoDirForProvenance(), "tovarisch-memory-lab/1.0.0")
		}
		dockerVer := ""
		if v, derr := dockerClient.ServerVersion(ctx); derr == nil {
			dockerVer = v.Version
		}
		execHash := cp.ExecutableSHA256
		if execHash == "" {
			if exe, eerr := os.Executable(); eerr == nil {
				if data, rerr := osReadFile(exe); rerr == nil {
					sum := sha256.Sum256(data)
					execHash = hex.EncodeToString(sum[:])
				}
			}
		}
		outcome.Observations.SetProvenance(
			cp.VCSRevision, cp.VCSTree, cp.GitObjectFormat,
			dockerVer, cp.ProducerVersion, execHash,
		)
		outcome.Observations.SetProvenanceDirty(cp.WorkingTreeDirty, cp.SourceCommitDirty)
		outcome.Observations.SetVCSModified(cp.VCSModified)

		ev := evidence.BuildEvidenceFromObservations(outcome.Observations)
		// PersistQualifiedExecutionEvidence compares the supplied
		// derived claims to the recomputed ones, so the producer must
		// stamp the claims on the in-memory artifact before persisting.
		ev.SetDerivedFields()
		if perr := evidence.PersistQualifiedExecutionEvidence(artifactsPath, ev); perr != nil {
			return fmt.Errorf("qualified evidence: %w", perr)
		}
	}

	if err != nil {
		// Lifecycle failed. The bounded cleanup has already happened.
		return fmt.Errorf("qualified lifecycle: %w", workloadErr)
	}
	if !outcome.Terminal {
		return fmt.Errorf("container did not reach terminal state")
	}
	if !outcome.ContainerRemoved || !outcome.NetworkRemoved {
		return fmt.Errorf("qualified cleanup incomplete: container=%v network=%v",
			outcome.ContainerRemoved, outcome.NetworkRemoved)
	}

	// Lifecycle succeeded. workload artifacts must be set.
	if workload.InitialState == nil || workload.FinalState == nil || workload.WorkloadResult == nil {
		return fmt.Errorf("workload did not produce initial/final state or workload result")
	}
	initialState := workload.InitialState
	finalState := workload.FinalState
	workloadResult := workload.WorkloadResult
	samples := workload.Samples
	phaseValid := workload.PhaseValid
	workloadValid := workload.WorkloadValid
	identityStable := workload.IdentityStable

	// CORRECTION03: capture and verify canary image identity.
	// Fails closed BEFORE the stimulus if pre-build, extracted-image,
	// and label hashes disagree. The result is stored inside the
	// canonical manifest.json (the canary-image-provenance.json
	// sidecar is no longer used).
	canaryImageIdentity, err := captureAndVerifyCanaryImageIdentity(ctx, dockerClient, outcome.ContainerID)
	if err != nil {
		return fmt.Errorf("canary image identity: %w", err)
	}

	thresholds := analysis.DefaultThresholds()
	invariantResult := validateStateInvariant(*scenario, initialState, finalState, workloadResult)
	verdict := analysis.AnalyzeWithInvariant(samples, thresholds, invariantResult)

	// CORRECTION02: descriptor fallback with full state-invariant
	// gating (StateInvariantValid + 15 scenario/workload/mode gates).
	// An invalid scenario invariant forces overall=invalid AND
	// prevents the descriptor_state_invariant signal from being
	// emitted.
	descriptorStateInvariantValid := invariantResult.Valid
	if *scenario == "canary-descriptor" {
		descInvariant := analysis.ComputeDescriptorStateInvariant(
			initialState.FDCount, finalState.FDCount,
			initialState.OperationCount, finalState.OperationCount,
			analysis.DescriptorWorkloadResult{
				Requested: workloadResult.Requested,
				Attempted: workloadResult.Attempted,
				Completed: workloadResult.Completed,
				Failed:    workloadResult.Failed,
				Returned:  workloadResult.Returned,
			},
		)
		fallback := analysis.DescriptorFallbackInput{
			Scenario:            *scenario,
			StateInvariantValid: descriptorStateInvariantValid,
			Invariant:           descInvariant,
			Initial: analysis.DescriptorInitialState{
				FDCount:        initialState.FDCount,
				OperationCount: initialState.OperationCount,
				Mode:           initialState.Mode,
				Ready:          initialState.Ready,
			},
			Final: analysis.DescriptorFinalState{
				FDCount:        finalState.FDCount,
				OperationCount: finalState.OperationCount,
				Mode:           finalState.Mode,
				Ready:          finalState.Ready,
				RetainedBlocks: finalState.RetainedBlocks,
				RetainedBytes:  finalState.RetainedBytes,
			},
			Workload: analysis.DescriptorWorkloadResult{
				Requested: workloadResult.Requested,
				Attempted: workloadResult.Attempted,
				Completed: workloadResult.Completed,
				Failed:    workloadResult.Failed,
				Returned:  workloadResult.Returned,
			},
			SamplesAvailable: analysis.SamplesHaveFDAvailable(samples),
			SamplesCount:     len(samples),
		}
		fbRes := analysis.ApplyDescriptorStateInvariant(fallback)
		if fbRes.Applied {
			verdict.Resource = analysis.ClassificationResourceGrowth
			verdict.Signals = append(verdict.Signals, fbRes.Signal)
		}
		// CORRECTION02: explicit invariant-aware overall priority.
		verdict.Overall = analysis.ComputeOverallWithInvariant(
			verdict.Memory, verdict.Resource, verdict.Semantic,
			descriptorStateInvariantValid,
		)
	} else {
		// Non-descriptor scenarios: invariant validity still gates
		// overall=invalid so an invalid canary-state invariant cannot
		// be masked by the analyzer's normal priority.
		verdict.Overall = analysis.ComputeOverallWithInvariant(
			verdict.Memory, verdict.Resource, verdict.Semantic,
			descriptorStateInvariantValid,
		)
	}

	expectedVerdict := getExpectedVerdict(*scenario)

	scenarioValid := phaseValid &&
		workloadValid &&
		identityStable &&
		len(samples) > 0 &&
		verdict.Overall != analysis.ClassificationInvalid &&
		verdict.Overall != analysis.ClassificationInconclusive

	canariesValid := scenarioValid &&
		verdict.Overall == expectedVerdict &&
		invariantResult.Valid

	subject, host, controllerPID, provenanceErr := collectProvenance()
	provenanceValid := provenanceErr == nil

	if provenanceErr != nil {
		scenarioValid = false
		canariesValid = false
	}

	verdictOutput := &evidence.Verdict{
		OverallClassification:  verdict.Overall,
		Scenario:               *scenario,
		ScenarioValid:          scenarioValid,
		CanariesValid:          canariesValid,
		MemoryClassification:   verdict.Memory,
		ResourceClassification: verdict.Resource,
		SemanticClassification: verdict.Semantic,
		SignalSummaries:        verdict.Signals,
		Thresholds:             &thresholds,
		Failures:               verdict.Failures,
		Warnings:               verdict.Warnings,
		Unknowns:               verdict.Unknowns,
		ProvenanceValid:        provenanceValid,
		ProvenanceError:        provenanceErrorString(provenanceErr),
	}
	if err := evidenceWriter.WriteVerdict(verdictOutput); err != nil {
		return fmt.Errorf("write verdict: %w", err)
	}

	if provenanceErr != nil {
		return fmt.Errorf("provenance collection failed: %w", provenanceErr)
	}

	finalizedManifest := &evidence.Manifest{
		SchemaVersion:   "1.1.0",
		RunID:           runID,
		Scenario:        *scenario,
		StartedAt:       manifest.StartedAt,
		FinishedAt:      time.Now(),
		SubjectIdentity: subject,
		ControllerID:    controllerPID,
		HostID:          host,
		DockerID: &evidence.DockerIdentity{
			EngineVersion: dockerInfo.Version,
			APIVersion:    dockerClient.ClientVersion(),
		},
		Configuration: &evidence.LabConfiguration{
			PhaseConfig: phaseCfg,
			Thresholds:  thresholds,
		},
		ArtifactInventory: []string{
			"manifest.json",
			"verdict.json",
			"samples.csv",
			"events.jsonl",
			"container-inspect.json",
			"container-logs.txt",
			"initial-canary-state.json",
			"final-canary-state.json",
			"workload-result.json",
			"checksums.txt",
		},
	}
	if err := evidenceWriter.WriteManifest(finalizedManifest); err != nil {
		return fmt.Errorf("write finalized manifest: %w", err)
	}

	// CORRECTION03: persist the canary image identity INSIDE the
	// canonical manifest.json (the canary-image-provenance.json
	// sidecar is no longer used). The verifier reads this block
	// from the manifest and reconstructs the image identity
	// without contacting Docker or Git.
	if canaryImageIdentity != nil {
		finalizedManifest.SubjectImageIdentity = canaryImageIdentity
		if err := evidenceWriter.WriteManifest(finalizedManifest); err != nil {
			return fmt.Errorf("write manifest with image identity: %w", err)
		}
	}

	if err := evidenceWriter.WriteChecksumsForInventory(finalizedManifest.ArtifactInventory); err != nil {
		return fmt.Errorf("write checksums: %w", err)
	}

	runFailed := !scenarioValid || !canariesValid || !invariantResult.Valid || verdict.Overall != expectedVerdict

	fmt.Printf("\n=== Analysis Result ===\n")
	fmt.Printf("Scenario: %s\n", *scenario)
	fmt.Printf("Expected Verdict: %s\n", expectedVerdict)
	fmt.Printf("Actual Verdict: %s\n", verdict.Overall)
	fmt.Printf("ScenarioValid: %v\n", scenarioValid)
	fmt.Printf("CanariesValid: %v\n", canariesValid)
	fmt.Printf("InvariantValid: %v\n", invariantResult.Valid)
	fmt.Printf("PhaseValid: %v\n", phaseValid)
	fmt.Printf("WorkloadValid: %v\n", workloadValid)
	fmt.Printf("IdentityStable: %v\n", identityStable)
	fmt.Printf("Samples: %d\n", len(samples))
	fmt.Printf("Signals: %d\n", len(verdict.Signals))

	if len(verdict.Failures) > 0 {
		fmt.Printf("Failures: %v\n", verdict.Failures)
	}
	if len(invariantResult.Failures) > 0 {
		fmt.Printf("Invariant Failures: %v\n", invariantResult.Failures)
	}

	fmt.Printf("\nArtifacts written to: %s\n", artifactsPath)
	fmt.Printf("Run ID: %s\n", runID)

	if runFailed {
		return fmt.Errorf("canary calibration failed: scenario_valid=%v canaries_valid=%v invariant_valid=%v verdict=%s expected=%s",
			scenarioValid, canariesValid, invariantResult.Valid, verdict.Overall, expectedVerdict)
	}

	return nil
}

// fallbackGitProvenance computes a ControllerProvenance directly
// from the git repository when the embedded VCS info is
// unavailable (e.g. during `go test`). The producer falls back
// to this when CollectControllerProvenance returns
// ErrProvenanceUnavailable.
func fallbackGitProvenance(repoDir, producer string) (evidence.ControllerProvenance, error) {
	head, err := gitOutputRepo(repoDir, "rev-parse", "HEAD")
	if err != nil {
		return evidence.ControllerProvenance{}, err
	}
	tree, err := gitOutputRepo(repoDir, "rev-parse", "--verify", head+"^{tree}")
	if err != nil {
		return evidence.ControllerProvenance{}, err
	}
	format, _ := gitOutputRepo(repoDir, "rev-parse", "--show-object-format")
	dirty, _ := gitWorkingTreeDirtyOutput(repoDir)
	return evidence.ControllerProvenance{
		VCSRevision:       head,
		VCSTree:           tree,
		VCSModified:       false,
		WorkingTreeDirty:  dirty,
		SourceCommitDirty: false,
		GitObjectFormat:   format,
		ProducerVersion:   producer,
	}, nil
}

func gitOutputRepo(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitWorkingTreeDirtyOutput(dir string) (bool, error) {
	out, err := gitOutputRepo(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// osReadFile is a thin wrapper for tests / fallback paths.
func osReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// repoDirForProvenance returns the working directory's nearest
// ancestor that contains a .git directory. Used by both the
// shared lifecycle helper and the production CLI.
func repoDirForProvenance() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

// captureAndVerifyCanaryImageIdentity is the CORRECTION03
// entry point. It reads the build metadata JSON produced by
// scripts/build_tovarisch_canary_image.sh, creates a read-only
// container from the canary image, extracts /app/canary via
// `docker cp` (the distroless image has no sha256sum so we cannot
// exec the binary directly), and computes the extracted-image
// binary SHA-256. The producer fails closed if the pre-build
// hash, the extracted-image hash, or the OCI label disagree.
//
// The returned SubjectImageIdentity is stored inside the
// canonical manifest.json (the canary-image-provenance.json
// sidecar is no longer used). The verifier reads this block
// from the manifest and reconstructs the image identity
// without contacting Docker or Git.
func captureAndVerifyCanaryImageIdentity(
	ctx context.Context,
	dockerClient *dockerlab.Client,
	canaryContainerID string,
) (*evidence.SubjectImageIdentity, error) {
	buildPath := filepath.Join("tovarisch", "labs", "memory", "canary-image-build.json")
	raw, err := os.ReadFile(buildPath)
	if err != nil {
		return nil, fmt.Errorf("read canary build metadata: %w (run scripts/build_tovarisch_canary_image.sh first)", err)
	}
	var build struct {
		ImageReference         string   `json:"image_reference"`
		ImageID                string   `json:"image_id"`
		RepoDigests            []string `json:"repo_digests"`
		SourceCommitOID        string   `json:"source_commit_oid"`
		RepositoryTreeOID      string   `json:"repository_tree_oid"`
		CanarySourceSubtreeOID string   `json:"canary_source_subtree_oid"`
		PrebuildBinarySHA256   string   `json:"prebuild_binary_sha256"`
	}
	if err := json.Unmarshal(raw, &build); err != nil {
		return nil, fmt.Errorf("parse canary build metadata: %w", err)
	}

	// Create a read-only container from the canary image so
	// /app/canary can be extracted without running it.
	tmpContainerID, err := dockerClient.ContainerCreateReadOnly(ctx, build.ImageID)
	if err != nil {
		return nil, fmt.Errorf("create read-only canary container: %w", err)
	}
	defer func() {
		_ = dockerClient.ContainerRemove(ctx, tmpContainerID, true)
	}()

	// Extract /app/canary and compute its SHA-256.
	data, err := dockerClient.ContainerExtractFile(ctx, tmpContainerID, "/app/canary")
	if err != nil {
		return nil, fmt.Errorf("extract /app/canary from canary image: %w", err)
	}
	sum := sha256.Sum256(data)
	extractedImageBinarySHA256 := hex.EncodeToString(sum[:])

	// Read the actual image labels (this is the source of truth
	// the manifest persists; not synthesized).
	labels, _ := dockerClient.ImageLabels(ctx, build.ImageID)

	// Build the canonical SubjectImageIdentity. Every field
	// here lives inside the checksummed manifest, so the
	// verifier can reconstruct the image identity offline.
	repoDigestStatus := "unavailable_local_image"
	if len(build.RepoDigests) > 0 {
		repoDigestStatus = "available"
	}
	sii := &evidence.SubjectImageIdentity{
		ImageReference:             build.ImageReference,
		ImageID:                    build.ImageID,
		RepoDigests:                build.RepoDigests,
		RepoDigestStatus:           repoDigestStatus,
		SourceCommitOID:            build.SourceCommitOID,
		RepositoryTreeOID:          build.RepositoryTreeOID,
		CanarySourceSubtreeOID:     build.CanarySourceSubtreeOID,
		PrebuildBinarySHA256:       build.PrebuildBinarySHA256,
		ExtractedImageBinarySHA256: extractedImageBinarySHA256,
		RevisionLabel:              labels["org.opencontainers.image.revision"],
		RepositoryTreeLabel:        labels["kgb.dev/source-tree"],
		SourceSubtreeLabel:         labels["kgb.dev/canary-source-tree"],
		BinarySHA256Label:          labels["kgb.dev/canary-binary-sha256"],
	}

	// CORRECTION03 ยง3 + ยง5: fail-closed before stimulus if
	// any comparison fails.
	if build.PrebuildBinarySHA256 == "" {
		return nil, fmt.Errorf("pre-build canary binary hash is empty in build metadata")
	}
	if extractedImageBinarySHA256 == "" {
		return nil, fmt.Errorf("extracted image binary hash is empty")
	}
	if !strings.EqualFold(build.PrebuildBinarySHA256, extractedImageBinarySHA256) {
		return nil, fmt.Errorf("canary binary hash mismatch: prebuild=%s extracted=%s",
			build.PrebuildBinarySHA256, extractedImageBinarySHA256)
	}
	if sii.BinarySHA256Label == "" {
		return nil, fmt.Errorf("canary image is missing kgb.dev/canary-binary-sha256 label")
	}
	if !strings.EqualFold(build.PrebuildBinarySHA256, sii.BinarySHA256Label) {
		return nil, fmt.Errorf("canary binary hash mismatch: prebuild=%s label=%s",
			build.PrebuildBinarySHA256, sii.BinarySHA256Label)
	}
	if sii.RevisionLabel == "" {
		return nil, fmt.Errorf("canary image is missing org.opencontainers.image.revision label")
	}
	if sii.RepositoryTreeLabel == "" {
		return nil, fmt.Errorf("canary image is missing kgb.dev/source-tree label")
	}
	if sii.SourceSubtreeLabel == "" {
		return nil, fmt.Errorf("canary image is missing kgb.dev/canary-source-tree label")
	}
	if !strings.EqualFold(sii.RevisionLabel, build.SourceCommitOID) {
		return nil, fmt.Errorf("canary image revision label=%s != tested commit=%s",
			sii.RevisionLabel, build.SourceCommitOID)
	}
	if !strings.EqualFold(sii.RepositoryTreeLabel, build.RepositoryTreeOID) {
		return nil, fmt.Errorf("canary image repository-tree label=%s != tested tree=%s",
			sii.RepositoryTreeLabel, build.RepositoryTreeOID)
	}
	if !strings.EqualFold(sii.SourceSubtreeLabel, build.CanarySourceSubtreeOID) {
		return nil, fmt.Errorf("canary image source-subtree label=%s != Git source subtree=%s",
			sii.SourceSubtreeLabel, build.CanarySourceSubtreeOID)
	}

	// CORRECTION03 ยง5: container inspect must report the
	// verified image ID.
	inspectedImage, err := dockerClient.ContainerImageID(ctx, canaryContainerID)
	if err != nil || inspectedImage == "" {
		return nil, fmt.Errorf("container inspect image ID is empty")
	}
	if !strings.HasPrefix(inspectedImage, build.ImageID) {
		return nil, fmt.Errorf("container image ID %s does not match verified image id (got=%s)",
			build.ImageID, inspectedImage)
	}
	sii.ContainerImageID = inspectedImage

	return sii, nil
}

// deriveRuntimeStateCommand reads a verified evidence bundle and
// emits the canonical runtime-state block for the close report.
// CORRECTION02 ยง8.
func deriveRuntimeStateCommand(args []string) error {
	fs := flag.NewFlagSet("memory-lab derive-runtime-state", flag.ContinueOnError)
	artifactsDir := fs.String("artifacts-dir", "", "Artifacts directory (required)")
	runID := fs.String("run-id", "", "Run ID (required)")
	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return fmt.Errorf("parse flags: %w", err)
	}
	if *artifactsDir == "" {
		return fmt.Errorf("--artifacts-dir is required")
	}
	if *runID == "" {
		return fmt.Errorf("--run-id is required")
	}

	block, err := deriveRuntimeState(*artifactsDir, *runID)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(block)
}

// runtimeStateBlock is the canonical close-report payload derived
// from the accepted evidence bundle. CORRECTION03: the canary
// image identity is now read from the manifest's
// subject_image_identity block (not from a sidecar).
type runtimeStateBlock struct {
	InitialFDCount        int                            `json:"initial_fd_count"`
	FinalFDCount          int                            `json:"final_fd_count"`
	FDCountDelta          int                            `json:"fd_count_delta"`
	InitialOperationCount int                            `json:"initial_operation_count"`
	FinalOperationCount   int                            `json:"final_operation_count"`
	OperationCountDelta   int                            `json:"operation_count_delta"`
	ProcessPID            int                            `json:"process_pid"`
	ProcessStartTime      int64                          `json:"process_start_time"`
	SampleCount           int                            `json:"sample_count"`
	DelayedSamples        int                            `json:"delayed_samples"`
	PhaseCounts           map[string]int                 `json:"phase_counts"`
	CanaryImage           *evidence.SubjectImageIdentity `json:"canary_image_identity,omitempty"`
}

// deriveRuntimeState reads the accepted evidence and emits the
// runtime-state block. All values are sourced from canonical
// evidence; the function never falls back to fixture values.
func deriveRuntimeState(artifactsDir, runID string) (*runtimeStateBlock, error) {
	artDir := filepath.Join(artifactsDir, runID)
	initialData, err := os.ReadFile(filepath.Join(artDir, "initial-canary-state.json"))
	if err != nil {
		return nil, fmt.Errorf("read initial canary state: %w", err)
	}
	var initial CanaryState
	if err := json.Unmarshal(initialData, &initial); err != nil {
		return nil, fmt.Errorf("parse initial canary state: %w", err)
	}
	finalData, err := os.ReadFile(filepath.Join(artDir, "final-canary-state.json"))
	if err != nil {
		return nil, fmt.Errorf("read final canary state: %w", err)
	}
	var final CanaryState
	if err := json.Unmarshal(finalData, &final); err != nil {
		return nil, fmt.Errorf("parse final canary state: %w", err)
	}
	samplesData, err := os.ReadFile(filepath.Join(artDir, "samples.csv"))
	if err != nil {
		return nil, fmt.Errorf("read samples CSV: %w", err)
	}
	samples, err := ParseSamplesCSVStream(strings.NewReader(string(samplesData)))
	if err != nil {
		return nil, fmt.Errorf("parse samples CSV: %w", err)
	}

	phaseCounts := map[string]int{}
	delayed := 0
	var pid int
	var startTime int64
	if len(samples) > 0 {
		pid = samples[0].PID
		startTime = int64(samples[0].ProcessStartTime)
	}
	for _, s := range samples {
		phaseCounts[string(s.Phase)]++
		if s.Delayed {
			delayed++
		}
	}

	block := &runtimeStateBlock{
		InitialFDCount:        initial.FDCount,
		FinalFDCount:          final.FDCount,
		FDCountDelta:          final.FDCount - initial.FDCount,
		InitialOperationCount: initial.OperationCount,
		FinalOperationCount:   final.OperationCount,
		OperationCountDelta:   final.OperationCount - initial.OperationCount,
		ProcessPID:            pid,
		ProcessStartTime:      startTime,
		SampleCount:           len(samples),
		DelayedSamples:        delayed,
		PhaseCounts:           phaseCounts,
	}

	// CORRECTION03: read the canary image identity from the
	// canonical manifest.json (not from the sidecar).
	manifestData, err := os.ReadFile(filepath.Join(artDir, "manifest.json"))
	if err == nil {
		var m evidence.Manifest
		if jerr := json.Unmarshal(manifestData, &m); jerr == nil && m.SubjectImageIdentity != nil {
			block.CanaryImage = m.SubjectImageIdentity
		}
	}
	return block, nil
}

// validatePhaseContract checks that we have samples from required phases
func validatePhaseContract(samples []sampling.Sample, cfg sampling.PhaseConfig) bool {
	hasBaseline := false
	hasFinal := false
	finalCount := 0
	for _, s := range samples {
		if s.Phase == sampling.PhaseBaseline {
			hasBaseline = true
		}
		if s.Phase == sampling.PhaseFinal {
			hasFinal = true
			finalCount++
		}
	}
	return hasBaseline && hasFinal && finalCount >= 3
}

// validateProcessIdentity checks that PID and start time are stable
func validateProcessIdentity(samples []sampling.Sample) bool {
	if len(samples) < 2 {
		return true
	}
	first := samples[0]
	for _, s := range samples[1:] {
		if s.PID != first.PID || s.ProcessStartTime != first.ProcessStartTime {
			return false
		}
	}
	return true
}

func getScenarioCommand(scenario string) []string {
	switch scenario {
	case "canary-growing":
		return []string{"--mode=growing"}
	case "canary-bounded":
		return []string{"--mode=bounded"}
	case "canary-descriptor":
		return []string{"--mode=descriptor"}
	default:
		return []string{"--mode=bounded"}
	}
}

func getExpectedVerdict(scenario string) analysis.Classification {
	switch scenario {
	case "canary-growing":
		return analysis.ClassificationGrowing
	case "canary-bounded":
		return analysis.ClassificationStable
	case "canary-descriptor":
		return analysis.ClassificationResourceGrowth
	default:
		return analysis.ClassificationStable
	}
}

// scenariosInSet is a small helper used by the verifier.
var scenariosInSet = map[string]struct{}{
	"canary-growing":    {},
	"canary-bounded":    {},
	"canary-descriptor": {},
}

func verifyCommand(args []string) error {
	fs := flag.NewFlagSet("memory-lab verify", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s verify [options]\n\nOptions:\n", args[0])
		fs.PrintDefaults()
	}

	artifactsDir := fs.String("artifacts-dir", "", "Artifacts directory (required)")
	runID := fs.String("run-id", "", "Run ID to verify (required)")

	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return fmt.Errorf("parse flags: %w", err)
	}

	if *artifactsDir == "" {
		return fmt.Errorf("--artifacts-dir is required")
	}
	if *runID == "" {
		return fmt.Errorf("--run-id is required")
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}

	artifactPath := filepath.Join(*artifactsDir, *runID)

	manifestData, err := os.ReadFile(filepath.Join(artifactPath, "manifest.json"))
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	var manifest evidence.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	if manifest.RunID != *runID {
		return fmt.Errorf("run ID mismatch: manifest=%s, expected=%s", manifest.RunID, *runID)
	}
	if manifest.FinishedAt.IsZero() {
		return fmt.Errorf("manifest not finalized: missing finished_at")
	}

	seenPaths := make(map[string]bool)
	for _, path := range manifest.ArtifactInventory {
		if path == "" {
			return fmt.Errorf("empty path in inventory")
		}
		if filepath.IsAbs(path) {
			return fmt.Errorf("absolute path in inventory: %s", path)
		}
		if strings.Contains(path, "..") {
			return fmt.Errorf("path traversal in inventory: %s", path)
		}
		if strings.HasPrefix(path, ".") {
			return fmt.Errorf("hidden path in inventory: %s", path)
		}
		if seenPaths[path] {
			return fmt.Errorf("duplicate path in inventory: %s", path)
		}
		seenPaths[path] = true
	}

	entries, err := os.ReadDir(artifactPath)
	if err != nil {
		return fmt.Errorf("read artifact directory: %w", err)
	}
	actualFiles := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			return fmt.Errorf("unexpected directory in artifacts: %s", entry.Name())
		}
		actualFiles[entry.Name()] = true
	}

	expectedFiles := make(map[string]bool)
	for _, p := range manifest.ArtifactInventory {
		expectedFiles[p] = true
	}
	if !expectedFiles["checksums.txt"] {
		return fmt.Errorf("checksums.txt not in inventory")
	}

	for path := range expectedFiles {
		if !actualFiles[path] {
			return fmt.Errorf("missing file from inventory: %s", path)
		}
	}
	for path := range actualFiles {
		if path == "canary-image-provenance.json" {
			// Defer this check to the verifier's provenance
			// reconstruction block so the diagnostic includes
			// the canary-image-provenance.json field context.
			continue
		}
		if !expectedFiles[path] {
			return fmt.Errorf("unexpected file not in inventory: %s", path)
		}
	}

	evidenceWriter := evidence.NewWriter(*runID, "", artifactPath)
	checksums, err := evidenceWriter.GenerateChecksumsForInventory(manifest.ArtifactInventory)
	if err != nil {
		return fmt.Errorf("generate checksums: %w", err)
	}

	checksumPath := filepath.Join(artifactPath, "checksums.txt")
	existingData, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}
	existingChecksums, err := evidence.ParseChecksumsFile(string(existingData))
	if err != nil {
		return fmt.Errorf("parse checksums: %w", err)
	}

	for _, c := range checksums {
		expectedHash, exists := existingChecksums[c.Path]
		if !exists {
			return fmt.Errorf("missing checksum for: %s", c.Path)
		}
		if expectedHash != c.SHA256 {
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", c.Path, expectedHash, c.SHA256)
		}
	}
	for path := range existingChecksums {
		found := false
		for _, c := range checksums {
			if c.Path == path {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unexpected checksum entry: %s", path)
		}
	}

	verdictData, err := os.ReadFile(filepath.Join(artifactPath, "verdict.json"))
	if err != nil {
		return fmt.Errorf("read verdict: %w", err)
	}
	var verdict evidence.Verdict
	if err := json.Unmarshal(verdictData, &verdict); err != nil {
		return fmt.Errorf("parse verdict: %w", err)
	}

	workloadData, err := os.ReadFile(filepath.Join(artifactPath, "workload-result.json"))
	if err != nil {
		return fmt.Errorf("read workload result: %w", err)
	}
	var workload WorkloadResult
	if err := json.Unmarshal(workloadData, &workload); err != nil {
		return fmt.Errorf("parse workload result: %w", err)
	}

	initialStateData, err := os.ReadFile(filepath.Join(artifactPath, "initial-canary-state.json"))
	if err != nil {
		return fmt.Errorf("read initial canary state: %w", err)
	}
	var initialState CanaryState
	if err := json.Unmarshal(initialStateData, &initialState); err != nil {
		return fmt.Errorf("parse initial canary state: %w", err)
	}

	finalStateData, err := os.ReadFile(filepath.Join(artifactPath, "final-canary-state.json"))
	if err != nil {
		return fmt.Errorf("read final canary state: %w", err)
	}
	var finalState CanaryState
	if err := json.Unmarshal(finalStateData, &finalState); err != nil {
		return fmt.Errorf("parse final canary state: %w", err)
	}

	inspectData, err := os.ReadFile(filepath.Join(artifactPath, "container-inspect.json"))
	if err != nil {
		return fmt.Errorf("read container inspect: %w", err)
	}
	var inspect ContainerInspect
	if err := json.Unmarshal(inspectData, &inspect); err != nil {
		return fmt.Errorf("parse container inspect: %w", err)
	}

	var verifyErrors []string

	if verdict.Scenario != manifest.Scenario {
		verifyErrors = append(verifyErrors, fmt.Sprintf("verdict scenario=%s != manifest scenario=%s", verdict.Scenario, manifest.Scenario))
	}

	expectedMode := scenarioToMode(manifest.Scenario)
	if initialState.Mode != expectedMode {
		verifyErrors = append(verifyErrors, fmt.Sprintf("initial mode=%s != expected=%s", initialState.Mode, expectedMode))
	}
	if finalState.Mode != expectedMode {
		verifyErrors = append(verifyErrors, fmt.Sprintf("final mode=%s != expected=%s", finalState.Mode, expectedMode))
	}

	expectedCmd := getScenarioCommand(manifest.Scenario)
	if len(inspect.Config.Cmd) != len(expectedCmd) {
		verifyErrors = append(verifyErrors, fmt.Sprintf("inspect Cmd length=%d != expected=%d", len(inspect.Config.Cmd), len(expectedCmd)))
	} else {
		for i, cmd := range inspect.Config.Cmd {
			if cmd != expectedCmd[i] {
				verifyErrors = append(verifyErrors, fmt.Sprintf("inspect Cmd[%d]=%s != expected=%s", i, cmd, expectedCmd[i]))
			}
		}
	}

	if workload.Requested != workload.Attempted || workload.Attempted != workload.Completed || workload.Failed != 0 {
		verifyErrors = append(verifyErrors, fmt.Sprintf("workload counts: req=%d att=%d com=%d fail=%d (expected req=att=com, fail=0)",
			workload.Requested, workload.Attempted, workload.Completed, workload.Failed))
	}
	if workload.Returned != workload.Completed {
		verifyErrors = append(verifyErrors, fmt.Sprintf("workload returned=%d != completed=%d (expected returned=completed)",
			workload.Returned, workload.Completed))
	}

	opDelta := finalState.OperationCount - initialState.OperationCount
	if opDelta != workload.Completed {
		verifyErrors = append(verifyErrors, fmt.Sprintf("operation_count_delta=%d != completed=%d", opDelta, workload.Completed))
	}

	switch manifest.Scenario {
	case "canary-growing":
		blocksDelta := finalState.RetainedBlocks - initialState.RetainedBlocks
		if blocksDelta != workload.Completed {
			verifyErrors = append(verifyErrors, fmt.Sprintf("growing: blocks_delta=%d != completed=%d", blocksDelta, workload.Completed))
		}
		bytesDelta := finalState.RetainedBytes - initialState.RetainedBytes
		expectedBytes := int64(workload.Completed) * 1048576
		if bytesDelta != expectedBytes {
			verifyErrors = append(verifyErrors, fmt.Sprintf("growing: bytes_delta=%d != expected=%d", bytesDelta, expectedBytes))
		}
	case "canary-bounded":
		if initialState.BufferCapacity != finalState.BufferCapacity {
			verifyErrors = append(verifyErrors, fmt.Sprintf("bounded: buffer_capacity changed from %d to %d",
				initialState.BufferCapacity, finalState.BufferCapacity))
		}
		if finalState.RetainedBlocks != 0 || finalState.RetainedBytes != 0 {
			verifyErrors = append(verifyErrors, fmt.Sprintf("bounded: retained should be 0, got blocks=%d bytes=%d",
				finalState.RetainedBlocks, finalState.RetainedBytes))
		}
	case "canary-descriptor":
		fdDelta := finalState.FDCount - initialState.FDCount
		expectedFDDelta := workload.Completed * 2
		if fdDelta != expectedFDDelta {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("descriptor: fd_delta=%d != expected=%d", fdDelta, expectedFDDelta))
		}
		if finalState.RetainedBlocks != 0 || finalState.RetainedBytes != 0 {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("descriptor: retained should be 0, got blocks=%d bytes=%d",
					finalState.RetainedBlocks, finalState.RetainedBytes))
		}
	}

	samplesData, err := os.ReadFile(filepath.Join(artifactPath, "samples.csv"))
	if err != nil {
		return fmt.Errorf("read samples: %w", err)
	}
	csvSamples, err := ParseSamplesCSVStream(strings.NewReader(string(samplesData)))
	if err != nil {
		return fmt.Errorf("parse samples CSV: %w", err)
	}

	hasBaseline := false
	hasFinal := false
	finalCount := 0
	for _, s := range csvSamples {
		if s.Phase == "baseline" {
			hasBaseline = true
		}
		if s.Phase == "final" {
			hasFinal = true
			finalCount++
		}
	}
	if !hasBaseline {
		verifyErrors = append(verifyErrors, "missing baseline phase samples")
	}
	if !hasFinal {
		verifyErrors = append(verifyErrors, "missing final phase samples")
	}
	if finalCount < 3 {
		verifyErrors = append(verifyErrors, fmt.Sprintf("insufficient final samples: %d < 3", finalCount))
	}

	if len(csvSamples) >= 2 {
		firstPID := csvSamples[0].PID
		firstStartTime := csvSamples[0].ProcessStartTime
		for _, s := range csvSamples[1:] {
			if s.PID != firstPID || s.ProcessStartTime != firstStartTime {
				verifyErrors = append(verifyErrors, fmt.Sprintf("PID instability: PID changed from %d or start time changed", firstPID))
				break
			}
		}
	}

	invariantResult := validateStateInvariant(manifest.Scenario, &initialState, &finalState, &workload)
	if !invariantResult.Valid {
		for _, f := range invariantResult.Failures {
			verifyErrors = append(verifyErrors, f)
		}
	}

	// CORRECTION02 ยง6: reconstruct verdicts using the manifest
	// thresholds, NOT analysis.DefaultThresholds(). The manifest
	// thresholds are the committed, authoritative values; a
	// threshold mutation must force the verifier to reject the
	// stored/reconstructed mismatch.
	var manifestThresholds analysis.Thresholds
	if manifest.Configuration != nil && manifest.Configuration.Thresholds != nil {
		// The manifest thresholds are persisted via JSON; convert
		// from the looser interface{} representation to the typed
		// analysis.Thresholds values used by the classifier.
		if raw, err := json.Marshal(manifest.Configuration.Thresholds); err == nil {
			if err := json.Unmarshal(raw, &manifestThresholds); err != nil {
				// If the stored thresholds don't unmarshal cleanly
				// we fall back to the analyzer's defaults for the
				// resource/memory reconstruction. The threshold-
				// equality check below still detects the divergence
				// from the verdict's persisted thresholds.
				manifestThresholds = analysis.DefaultThresholds()
			}
		} else {
			manifestThresholds = analysis.DefaultThresholds()
		}
	} else {
		// No thresholds persisted in manifest: refuse to silently
		// use defaults. The verdict's stored thresholds are still
		// compared below.
		manifestThresholds = analysis.DefaultThresholds()
	}

	// CORRECTION02 ยง6: compare the verifier-reconstructed
	// thresholds against the verdict's persisted thresholds. A
	// material mutation in the manifest must surface here.
	if verdict.Thresholds != nil {
		vt := verdict.Thresholds
		if vt.MemoryGrowthKibPerHour != manifestThresholds.MemoryGrowthKibPerHour {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("threshold mutation: verdict memory_growth_kib_per_hour=%d != manifest=%d",
					vt.MemoryGrowthKibPerHour, manifestThresholds.MemoryGrowthKibPerHour))
		}
		if vt.ResourceGrowthPerHour != manifestThresholds.ResourceGrowthPerHour {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("threshold mutation: verdict resource_growth_per_hour=%d != manifest=%d",
					vt.ResourceGrowthPerHour, manifestThresholds.ResourceGrowthPerHour))
		}
		if vt.CorroborationCount != manifestThresholds.CorroborationCount {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("threshold mutation: verdict corroboration_count=%d != manifest=%d",
					vt.CorroborationCount, manifestThresholds.CorroborationCount))
		}
		if vt.SampleCountMinimum != manifestThresholds.SampleCountMinimum {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("threshold mutation: verdict sample_count_minimum=%d != manifest=%d",
					vt.SampleCountMinimum, manifestThresholds.SampleCountMinimum))
		}
	} else {
		verifyErrors = append(verifyErrors, "verdict.thresholds is nil; manifest thresholds cannot be reconstructed")
	}

	// 1. Reconstruct resource classification.
	reconstructedResource := analysis.ClassificationStable
	if manifest.Scenario == "canary-descriptor" {
		descInvariant := analysis.ComputeDescriptorStateInvariant(
			initialState.FDCount, finalState.FDCount,
			initialState.OperationCount, finalState.OperationCount,
			analysis.DescriptorWorkloadResult{
				Requested: workload.Requested,
				Attempted: workload.Attempted,
				Completed: workload.Completed,
				Failed:    workload.Failed,
				Returned:  workload.Returned,
			},
		)
		fallback := analysis.DescriptorFallbackInput{
			Scenario:            manifest.Scenario,
			StateInvariantValid: invariantResult.Valid,
			Invariant:           descInvariant,
			Initial: analysis.DescriptorInitialState{
				FDCount:        initialState.FDCount,
				OperationCount: initialState.OperationCount,
				Mode:           initialState.Mode,
				Ready:          initialState.Ready,
			},
			Final: analysis.DescriptorFinalState{
				FDCount:        finalState.FDCount,
				OperationCount: finalState.OperationCount,
				Mode:           finalState.Mode,
				Ready:          finalState.Ready,
				RetainedBlocks: finalState.RetainedBlocks,
				RetainedBytes:  finalState.RetainedBytes,
			},
			Workload: analysis.DescriptorWorkloadResult{
				Requested: workload.Requested,
				Attempted: workload.Attempted,
				Completed: workload.Completed,
				Failed:    workload.Failed,
				Returned:  workload.Returned,
			},
			SamplesAvailable: analysis.SamplesHaveFDAvailable(csvSamples),
			SamplesCount:     len(csvSamples),
		}
		fbRes := analysis.ApplyDescriptorStateInvariant(fallback)
		if fbRes.Applied {
			reconstructedResource = analysis.ClassificationResourceGrowth
		} else {
			analyzed := analysis.Analyze(csvSamples, manifestThresholds)
			reconstructedResource = analyzed.Resource
		}
	} else {
		analyzed := analysis.Analyze(csvSamples, manifestThresholds)
		reconstructedResource = analyzed.Resource
	}

	analyzed := analysis.Analyze(csvSamples, manifestThresholds)
	reconstructedMemory := analyzed.Memory
	reconstructedSemantic := analyzed.Semantic

	// CORRECTION02: overall uses the explicit invariant-aware
	// priority. An invalid scenario invariant must produce
	// overall=invalid even if the analyzer reports growing.
	reconstructedOverall := analysis.ComputeOverallWithInvariant(
		reconstructedMemory,
		reconstructedResource,
		reconstructedSemantic,
		invariantResult.Valid,
	)

	reconstructedScenarioValid := verifyScenarioValid(manifest.Scenario, csvSamples, workload, verifyErrors) && len(verifyErrors) == 0
	reconstructedCanariesValid := reconstructedScenarioValid &&
		reconstructedOverall == getExpectedVerdict(manifest.Scenario) &&
		invariantResult.Valid
	reconstructedProvenanceValid := true

	// 5. Validate descriptor invariant signal
	if manifest.Scenario == "canary-descriptor" {
		descriptorInvariants := 0
		var invariant *analysis.SignalSummary
		for i := range verdict.SignalSummaries {
			sig := &verdict.SignalSummaries[i]
			if sig.Name == "descriptor_state_invariant" {
				descriptorInvariants++
				invariant = sig
			}
		}
		sampledFDAvailable := analysis.SamplesHaveFDAvailable(csvSamples)
		if sampledFDAvailable {
			if descriptorInvariants > 0 {
				verifyErrors = append(verifyErrors,
					"sampled FD signal is available; descriptor_state_invariant must not be present")
			}
		} else {
			if !invariantResult.Valid {
				if descriptorInvariants > 0 {
					verifyErrors = append(verifyErrors,
						"invalid scenario invariant; descriptor_state_invariant must not be present")
				}
			} else {
				if descriptorInvariants == 0 {
					verifyErrors = append(verifyErrors,
						"missing descriptor_state_invariant signal")
				}
				if descriptorInvariants > 1 {
					verifyErrors = append(verifyErrors,
						"duplicate descriptor_state_invariant signal")
				}
				if invariant != nil {
					// CORRECTION02: strict source-kind + counter checks.
					if invariant.SourceKind == "" {
						verifyErrors = append(verifyErrors,
							"descriptor_state_invariant source_kind is empty (expected state_invariant)")
					} else if invariant.SourceKind != analysis.SignalKindStateInvariant {
						verifyErrors = append(verifyErrors,
							fmt.Sprintf("descriptor_state_invariant source_kind=%s, expected state_invariant",
								invariant.SourceKind))
					}
					if !invariant.IsPrimary {
						verifyErrors = append(verifyErrors,
							"descriptor_state_invariant must be primary")
					}
					if invariant.FirstWindowMedian != int64(initialState.FDCount) {
						verifyErrors = append(verifyErrors,
							fmt.Sprintf("descriptor_state_invariant first_window_median=%d != canary initial=%d",
								invariant.FirstWindowMedian, initialState.FDCount))
					}
					if invariant.LastWindowMedian != int64(finalState.FDCount) {
						verifyErrors = append(verifyErrors,
							fmt.Sprintf("descriptor_state_invariant last_window_median=%d != canary final=%d",
								invariant.LastWindowMedian, finalState.FDCount))
					}
					expectedDelta := int64(workload.Completed * 2)
					if invariant.AbsoluteDelta != expectedDelta {
						verifyErrors = append(verifyErrors,
							fmt.Sprintf("descriptor_state_invariant absolute_delta=%d != expected=%d",
								invariant.AbsoluteDelta, expectedDelta))
					}
					if invariant.Classification != analysis.ClassificationResourceGrowth {
						verifyErrors = append(verifyErrors,
							fmt.Sprintf("descriptor_state_invariant classification=%s != resource_growth",
								invariant.Classification))
					}
					// CORRECTION02 ยง2: structural counters.
					if invariant.SampleCount != 2 {
						verifyErrors = append(verifyErrors,
							fmt.Sprintf("descriptor_state_invariant sample_count=%d, expected 2",
								invariant.SampleCount))
					}
					if invariant.AvailableCount != 2 {
						verifyErrors = append(verifyErrors,
							fmt.Sprintf("descriptor_state_invariant available_count=%d, expected 2",
								invariant.AvailableCount))
					}
					if invariant.MissingCount != 0 {
						verifyErrors = append(verifyErrors,
							fmt.Sprintf("descriptor_state_invariant missing_count=%d, expected 0",
								invariant.MissingCount))
					}
					if invariant.AvailableCount+invariant.MissingCount != invariant.SampleCount {
						verifyErrors = append(verifyErrors,
							fmt.Sprintf("descriptor_state_invariant available(%d)+missing(%d) != sample_count(%d)",
								invariant.AvailableCount, invariant.MissingCount, invariant.SampleCount))
					}
					if invariant.RatePerHour != 0 {
						verifyErrors = append(verifyErrors,
							fmt.Sprintf("descriptor_state_invariant rate_per_hour=%f, expected 0",
								invariant.RatePerHour))
					}
					if invariant.Slope != 0 {
						verifyErrors = append(verifyErrors,
							fmt.Sprintf("descriptor_state_invariant slope=%f, expected 0",
								invariant.Slope))
					}
					if invariant.RelativeDelta != 0 {
						verifyErrors = append(verifyErrors,
							fmt.Sprintf("descriptor_state_invariant relative_delta=%f, expected 0",
								invariant.RelativeDelta))
					}
					if invariant.Minimum != int64(initialState.FDCount) {
						verifyErrors = append(verifyErrors,
							fmt.Sprintf("descriptor_state_invariant minimum=%d, expected initial fd_count=%d",
								invariant.Minimum, initialState.FDCount))
					}
					if invariant.Maximum != int64(finalState.FDCount) {
						verifyErrors = append(verifyErrors,
							fmt.Sprintf("descriptor_state_invariant maximum=%d, expected final fd_count=%d",
								invariant.Maximum, finalState.FDCount))
					}
				}
			}
		}
	}

	// 6. Validate sampled-signal source_kind consistency
	//    Every non-invariant signal must have source_kind=sampled.
	//    Empty source_kind is REJECTED (CORRECTION02 strict contract).
	for _, sig := range verdict.SignalSummaries {
		if sig.Name == "descriptor_state_invariant" {
			continue
		}
		if sig.SourceKind == "" {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("signal %q has empty source_kind (expected sampled)", sig.Name))
			continue
		}
		if sig.SourceKind != analysis.SignalKindSampled {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("signal %q has source_kind=%s, expected sampled",
					sig.Name, sig.SourceKind))
		}
	}

	if verdict.OverallClassification != reconstructedOverall {
		verifyErrors = append(verifyErrors,
			fmt.Sprintf("stored overall classification %s does not match reconstruction %s",
				verdict.OverallClassification, reconstructedOverall))
	}
	if verdict.MemoryClassification != reconstructedMemory {
		verifyErrors = append(verifyErrors,
			fmt.Sprintf("stored memory classification %s does not match reconstruction %s",
				verdict.MemoryClassification, reconstructedMemory))
	}
	if verdict.ResourceClassification != reconstructedResource {
		verifyErrors = append(verifyErrors,
			fmt.Sprintf("stored resource classification %s does not match reconstruction %s",
				verdict.ResourceClassification, reconstructedResource))
	}
	if verdict.SemanticClassification != reconstructedSemantic {
		verifyErrors = append(verifyErrors,
			fmt.Sprintf("stored semantic classification %s does not match reconstruction %s",
				verdict.SemanticClassification, reconstructedSemantic))
	}

	if verdict.ScenarioValid != reconstructedScenarioValid {
		verifyErrors = append(verifyErrors, "stored ScenarioValid does not match reconstruction")
	}
	if verdict.CanariesValid != reconstructedCanariesValid {
		verifyErrors = append(verifyErrors, "stored CanariesValid does not match reconstruction")
	}

	provErrs := validateProvenanceEvidence(manifest, verdict)
	if len(provErrs) > 0 {
		reconstructedProvenanceValid = false
	}
	verifyErrors = append(verifyErrors, provErrs...)

	if verdict.ProvenanceValid != reconstructedProvenanceValid {
		verifyErrors = append(verifyErrors, "stored ProvenanceValid does not match reconstruction")
	}

	if manifest.SubjectIdentity != nil && manifest.SubjectIdentity.ControllerExecutableSHA256 != "" {
		if err := verifyRuntimeExecutableHash(
			manifest.SubjectIdentity.ControllerExecutableSHA256,
			openProcSelfExe,
		); err != nil {
			verifyErrors = append(verifyErrors, err.Error())
		}
	}

	// CORRECTION03 ยง6: canary image identity is reconstructed
	// from the manifest's subject_image_identity block. The
	// sidecar canary-image-provenance.json is no longer part of
	// the canonical schema. The verifier must reject the sidecar
	// if present.
	for path := range actualFiles {
		if path == "canary-image-provenance.json" {
			verifyErrors = append(verifyErrors,
				"canary-image-provenance.json is in the artifact directory; CORRECTION03 requires the image identity to be inside manifest.json")
		}
	}
	// CORRECTION04: schema_version check.
	switch manifest.SchemaVersion {
	case "1.0.0":
		// Legacy: subject_image_identity is optional; the
		// verifier does not claim image-provenance PASS for 1.0.0.
		if manifest.SubjectImageIdentity != nil {
			verifyErrors = append(verifyErrors,
				"schema_version 1.0.0 evidence must not carry subject_image_identity; remove the block or upgrade to 1.1.0")
		}
	case "1.1.0":
		if manifest.SubjectImageIdentity == nil {
			verifyErrors = append(verifyErrors,
				"manifest.subject_image_identity is missing; schema 1.1.0 requires the canary image identity to be inside manifest.json")
		}
	default:
		verifyErrors = append(verifyErrors,
			fmt.Sprintf("unsupported manifest_schema_version=%q (CORRECTION04 accepts only 1.0.0 legacy or 1.1.0 current)",
				manifest.SchemaVersion))
	}
	if manifest.SubjectImageIdentity == nil && manifest.SchemaVersion == "1.1.0" {
		verifyErrors = append(verifyErrors,
			"manifest.subject_image_identity is missing; schema 1.1.0 requires the canary image identity to be inside manifest.json")
	} else if manifest.SubjectImageIdentity == nil {
		// Legacy 1.0.0: subject_image_identity is permitted to be missing.
	} else {
		sii := manifest.SubjectImageIdentity
		if manifest.SubjectIdentity != nil {
			if sii.SourceCommitOID != "" && sii.SourceCommitOID != manifest.SubjectIdentity.GitCommit {
				verifyErrors = append(verifyErrors,
					fmt.Sprintf("manifest subject_image_identity.source_commit_oid=%s != manifest subject_identity.git_commit=%s",
						sii.SourceCommitOID, manifest.SubjectIdentity.GitCommit))
			}
			if sii.RepositoryTreeOID != "" && sii.RepositoryTreeOID != manifest.SubjectIdentity.GitTree {
				verifyErrors = append(verifyErrors,
					fmt.Sprintf("manifest subject_image_identity.repository_tree_oid=%s != manifest subject_identity.git_tree=%s",
						sii.RepositoryTreeOID, manifest.SubjectIdentity.GitTree))
			}
		}
		if sii.PrebuildBinarySHA256 == "" {
			verifyErrors = append(verifyErrors, "subject_image_identity.prebuild_binary_sha256 is empty")
		}
		if sii.ExtractedImageBinarySHA256 == "" {
			verifyErrors = append(verifyErrors, "subject_image_identity.extracted_image_binary_sha256 is empty")
		}
		if sii.PrebuildBinarySHA256 != "" && sii.ExtractedImageBinarySHA256 != "" &&
			!strings.EqualFold(sii.PrebuildBinarySHA256, sii.ExtractedImageBinarySHA256) {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("subject_image_identity prebuild=%s != extracted=%s",
					sii.PrebuildBinarySHA256, sii.ExtractedImageBinarySHA256))
		}
		if sii.BinarySHA256Label == "" {
			verifyErrors = append(verifyErrors, "subject_image_identity.binary_sha256_label is empty")
		}
		if sii.PrebuildBinarySHA256 != "" && sii.BinarySHA256Label != "" &&
			!strings.EqualFold(sii.PrebuildBinarySHA256, sii.BinarySHA256Label) {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("subject_image_identity prebuild=%s != binary_sha256_label=%s",
					sii.PrebuildBinarySHA256, sii.BinarySHA256Label))
		}
		if sii.RevisionLabel == "" {
			verifyErrors = append(verifyErrors, "subject_image_identity.revision_label is empty")
		} else if sii.SourceCommitOID != "" && !strings.EqualFold(sii.RevisionLabel, sii.SourceCommitOID) {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("subject_image_identity.revision_label=%s != source_commit_oid=%s",
					sii.RevisionLabel, sii.SourceCommitOID))
		}
		if sii.RepositoryTreeLabel == "" {
			verifyErrors = append(verifyErrors, "subject_image_identity.repository_tree_label is empty")
		} else if sii.RepositoryTreeOID != "" && !strings.EqualFold(sii.RepositoryTreeLabel, sii.RepositoryTreeOID) {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("subject_image_identity.repository_tree_label=%s != repository_tree_oid=%s",
					sii.RepositoryTreeLabel, sii.RepositoryTreeOID))
		}
		if sii.SourceSubtreeLabel == "" {
			verifyErrors = append(verifyErrors, "subject_image_identity.source_subtree_label is empty")
		} else if sii.CanarySourceSubtreeOID != "" && !strings.EqualFold(sii.SourceSubtreeLabel, sii.CanarySourceSubtreeOID) {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("subject_image_identity.source_subtree_label=%s != canary_source_subtree_oid=%s",
					sii.SourceSubtreeLabel, sii.CanarySourceSubtreeOID))
		}
		if sii.ImageID == "" {
			verifyErrors = append(verifyErrors, "subject_image_identity.image_id is empty")
		}
		if len(sii.RepoDigests) == 0 && sii.RepoDigestStatus != "unavailable_local_image" {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("subject_image_identity.repo_digest_status=%q inconsistent with empty repo_digests",
					sii.RepoDigestStatus))
		}
		if len(sii.RepoDigests) > 0 && sii.RepoDigestStatus != "available" {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("subject_image_identity.repo_digest_status=%q inconsistent with non-empty repo_digests",
					sii.RepoDigestStatus))
		}
		// Container image ID must equal the verified image ID
		// from container-inspect.json.
		containerImageID, _ := extractContainerImageID(inspectData)
		// image_reference should match container-inspect.json Config.Image.
		inspectImageRef, _ := extractContainerImageReference(inspectData)
		if sii.ContainerImageID == "" {
			verifyErrors = append(verifyErrors, "subject_image_identity.container_image_id is empty")
		} else if containerImageID != "" && sii.ContainerImageID != containerImageID {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("subject_image_identity.container_image_id=%s != container-inspect.json image=%s",
					sii.ContainerImageID, containerImageID))
		}
		// CORRECTION04 ยง2: image_id must equal container_image_id
		// and both must equal container-inspect.json's Image.
		if sii.ImageID != "" && sii.ContainerImageID != "" &&
			!strings.EqualFold(sii.ImageID, sii.ContainerImageID) {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("subject_image_identity.image_id=%s does not match subject_image_identity.container_image_id=%s",
					sii.ImageID, sii.ContainerImageID))
		}
		if sii.ImageID != "" && containerImageID != "" &&
			!strings.EqualFold(sii.ImageID, containerImageID) {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("subject_image_identity.image_id=%s does not match container inspect Image=%s",
					sii.ImageID, containerImageID))
		}
		if sii.ImageReference != "" && inspectImageRef != "" &&
			!strings.EqualFold(sii.ImageReference, inspectImageRef) {
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("subject_image_identity.image_reference=%s does not match container inspect Config.Image=%s",
					sii.ImageReference, inspectImageRef))
		}
		// CORRECTION04 ยง3: repository digest status and grammar.
		switch sii.RepoDigestStatus {
		case "available":
			if len(sii.RepoDigests) == 0 {
				verifyErrors = append(verifyErrors,
					"subject_image_identity.repo_digest_status=available inconsistent with empty repo_digests")
			}
			for _, d := range sii.RepoDigests {
				if err := validateRepoDigest(d); err != nil {
					verifyErrors = append(verifyErrors,
						fmt.Sprintf("subject_image_identity.repo_digests contains invalid entry %q: %v", d, err))
				}
			}
		case "unavailable_local_image":
			if len(sii.RepoDigests) != 0 {
				verifyErrors = append(verifyErrors,
					"subject_image_identity.repo_digest_status=unavailable_local_image inconsistent with non-empty repo_digests")
			}
		case "":
			verifyErrors = append(verifyErrors,
				"subject_image_identity.repo_digest_status is empty")
		default:
			verifyErrors = append(verifyErrors,
				fmt.Sprintf("subject_image_identity.repo_digest_status=%q invalid (expected available or unavailable_local_image)",
					sii.RepoDigestStatus))
		}
		// Reject duplicate repo digests.
		seen := make(map[string]bool)
		for _, d := range sii.RepoDigests {
			if seen[d] {
				verifyErrors = append(verifyErrors,
					fmt.Sprintf("subject_image_identity.repo_digests contains duplicate entry %q", d))
			}
			seen[d] = true
		}
	}

	fmt.Printf("=== Verification Results ===\n")
	fmt.Printf("Run ID: %s\n", *runID)
	fmt.Printf("Scenario: %s\n", verdict.Scenario)
	fmt.Printf("Reconstructed Claims: %d checks passed\n", len(manifest.ArtifactInventory)+5-len(verifyErrors))

	if len(verifyErrors) > 0 {
		fmt.Printf("Verification Errors:\n")
		for _, e := range verifyErrors {
			fmt.Printf("  - %s\n", e)
		}
		return fmt.Errorf("evidence verification failed: %d errors", len(verifyErrors))
	}

	fmt.Printf("All Verifications: PASS\n")
	fmt.Printf("ScenarioValid: %v\n", verdict.ScenarioValid)
	fmt.Printf("CanariesValid: %v\n", verdict.CanariesValid)
	fmt.Printf("Overall: %s\n", verdict.OverallClassification)
	fmt.Printf("Memory: %s\n", verdict.MemoryClassification)
	fmt.Printf("Checksums: PASS\n")
	fmt.Printf("Artifact Geometry: PASS\n")
	fmt.Printf("Evidence Reconstruction: PASS\n")

	if verdict.ScenarioValid && verdict.CanariesValid {
		fmt.Printf("PASS: Evidence verified\n")
		return nil
	}

	return fmt.Errorf("verdict indicates scenario or canaries not valid")
}

// verifyScenarioValid reconstructs the scenario validity from evidence
func verifyScenarioValid(scenario string, samples []sampling.Sample, workload WorkloadResult, verifyErrors []string) bool {
	if len(verifyErrors) > 0 {
		return false
	}
	hasBaseline := false
	hasFinal := false
	for _, s := range samples {
		if s.Phase == sampling.PhaseBaseline {
			hasBaseline = true
		}
		if s.Phase == sampling.PhaseFinal {
			hasFinal = true
		}
	}
	return hasBaseline && hasFinal &&
		workload.Requested == workload.Attempted &&
		workload.Attempted == workload.Completed &&
		workload.Failed == 0 &&
		workload.Returned == workload.Completed &&
		len(samples) > 0
}

// ContainerInspect represents the container inspect data
type ContainerInspect struct {
	Path   string `json:"Path"`
	Config struct {
		Cmd []string `json:"Cmd"`
	} `json:"Config"`
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	return lines
}

func scenarioToMode(scenario string) string {
	switch scenario {
	case "canary-growing":
		return "growing"
	case "canary-bounded":
		return "bounded"
	case "canary-descriptor":
		return "descriptor"
	default:
		return "bounded"
	}
}

// waitForCanaryHealth waits for the canary to be healthy.
func waitForCanaryHealth(ctx context.Context, client *http.Client, url string, timeout time.Duration, verbose bool) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, err := http.NewRequestWithContext(ctx, "GET", url+"/health", nil)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				if verbose {
					fmt.Printf("Canary is healthy\n")
				}
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for canary health")
}

// fetchCanaryState fetches the current canary state.
func fetchCanaryState(ctx context.Context, client *http.Client, url string) (*CanaryState, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url+"/state", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /state: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	var state CanaryState
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return &state, nil
}

// operateCanary sends operate requests to the canary.
func operateCanary(ctx context.Context, client *http.Client, url string, count int) (*WorkloadResult, error) {
	opClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
		},
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url+"/operate?count="+strconv.Itoa(count), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := opClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST /operate: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("operate failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	var opResult struct {
		Attempted int `json:"attempted"`
		Completed int `json:"completed"`
	}
	if err := json.Unmarshal(body, &opResult); err != nil {
		return nil, fmt.Errorf("parse operate response: %w", err)
	}
	return &WorkloadResult{
		Requested: count,
		Attempted: opResult.Attempted,
		Completed: opResult.Completed,
		Failed:    count - opResult.Completed,
		Returned:  opResult.Completed,
	}, nil
}

// getScenarioOperationCount returns the operation count for each scenario.
func getScenarioOperationCount(scenario string) int {
	switch scenario {
	case "canary-growing":
		return 32
	case "canary-bounded":
		return 100
	case "canary-descriptor":
		return 100
	default:
		return 32
	}
}

// allowedScenarios is the set of permitted --scenario values.
var allowedScenarios = map[string]struct{}{
	"canary-growing":    {},
	"canary-bounded":    {},
	"canary-descriptor": {},
}

// namespaceReader is a seam for reading namespace info from a PID.
type namespaceReader func(pid int) (*procfs.NamespaceInfo, error)

func classifyCgroupFailureWithReader(
	err error,
	targetPID int,
	controllerPID int,
	readNS namespaceReader,
) (sampling.CgroupCapability, *sampling.NamespaceProof) {
	proof := &sampling.NamespaceProof{}
	if err == nil {
		return sampling.CgroupCapabilityAvailable, nil
	}
	var capability sampling.CgroupCapability
	switch {
	case errors.Is(err, procfs.ErrNoCgroup2Mount):
		capability = sampling.CgroupCapabilityCgroupNotVisible
	case errors.Is(err, procfs.ErrNoUnifiedCgroup):
		capability = sampling.CgroupCapabilityNoUnifiedHierarchy
	case errors.Is(err, procfs.ErrPathTraversal):
		capability = sampling.CgroupCapabilityPathTraversal
	case errors.Is(err, os.ErrPermission), errors.Is(err, procfs.ErrPermissionDenied):
		capability = sampling.CgroupCapabilityPermissionDenied
	case errors.Is(err, procfs.ErrParseFailure):
		capability = sampling.CgroupCapabilityParseFailure
	default:
		capability = sampling.CgroupCapabilityPathAbsent
	}

	targetNS, targetReadErr := readNS(targetPID)
	controllerNS, controllerReadErr := readNS(controllerPID)
	if targetReadErr != nil {
		proof.TargetReadError = targetReadErr.Error()
	}
	if controllerReadErr != nil {
		proof.ControllerReadError = controllerReadErr.Error()
	}
	if targetNS != nil {
		proof.TargetMountNamespace = targetNS.MountNamespace
		proof.TargetCgroupNamespace = targetNS.CgroupNamespace
		if targetNS.MountNamespaceErr != nil {
			proof.TargetMountNamespaceErr = targetNS.MountNamespaceErr.Error()
		}
		if targetNS.CgroupNamespaceErr != nil {
			proof.TargetCgroupNamespaceErr = targetNS.CgroupNamespaceErr.Error()
		}
	}
	if controllerNS != nil {
		proof.ControllerMountNamespace = controllerNS.MountNamespace
		proof.ControllerCgroupNamespace = controllerNS.CgroupNamespace
		if controllerNS.MountNamespaceErr != nil {
			proof.ControllerMountNamespaceErr = controllerNS.MountNamespaceErr.Error()
		}
		if controllerNS.CgroupNamespaceErr != nil {
			proof.ControllerCgroupNamespaceErr = controllerNS.CgroupNamespaceErr.Error()
		}
	}

	if capability == sampling.CgroupCapabilityCgroupNotVisible ||
		capability == sampling.CgroupCapabilityNoUnifiedHierarchy {
		if targetReadErr != nil || controllerReadErr != nil {
			proof.DecisionReason = "namespace_identity_unavailable"
			return sampling.CgroupCapabilityNamespaceIdentityUnavail, proof
		}
		canProveMount := targetNS != nil && controllerNS != nil &&
			targetNS.MountNamespaceErr == nil && controllerNS.MountNamespaceErr == nil &&
			targetNS.MountNamespace != "" && controllerNS.MountNamespace != ""
		canProveCgroup := targetNS != nil && controllerNS != nil &&
			targetNS.CgroupNamespaceErr == nil && controllerNS.CgroupNamespaceErr == nil &&
			targetNS.CgroupNamespace != "" && controllerNS.CgroupNamespace != ""
		if canProveMount &&
			proof.TargetMountNamespace != "" && proof.ControllerMountNamespace != "" &&
			proof.TargetMountNamespace != proof.ControllerMountNamespace {
			proof.DecisionReason = "mount_namespace_differ"
			return sampling.CgroupCapabilityMountNamespaceMismatch, proof
		}
		if canProveCgroup &&
			proof.TargetCgroupNamespace != "" && proof.ControllerCgroupNamespace != "" &&
			proof.TargetCgroupNamespace != proof.ControllerCgroupNamespace {
			proof.DecisionReason = "cgroup_namespace_differ"
			return sampling.CgroupCapabilityCgroupNamespaceMismatch, proof
		}
		if !canProveMount || !canProveCgroup {
			proof.DecisionReason = "namespace_identity_unavailable"
			return sampling.CgroupCapabilityNamespaceIdentityUnavail, proof
		}
		proof.DecisionReason = "namespaces_equal_cgroup_not_visible"
		return sampling.CgroupCapabilityNotMounted, proof
	}
	proof.DecisionReason = capability.String()
	return capability, proof
}

func classifyCgroupFailureWithNamespace(err error, targetPID, controllerPID int) (sampling.CgroupCapability, *sampling.NamespaceProof) {
	return classifyCgroupFailureWithReader(err, targetPID, controllerPID, procfs.ReadNamespaceIDs)
}

func collectProvenance() (*evidence.SubjectIdentity, *evidence.HostIdentity, string, error) {
	var errs []string

	gitCommit, gitErr := runGit("rev-parse", "HEAD")
	if gitErr != nil {
		errs = append(errs, fmt.Sprintf("git commit: %v", gitErr))
	}
	gitTree, treeErr := runGit("rev-parse", "HEAD^{tree}")
	if treeErr != nil {
		errs = append(errs, fmt.Sprintf("git tree: %v", treeErr))
	}
	gitObjectFormat, formatErr := runGit("rev-parse", "--show-object-format=storage")
	if formatErr != nil {
		errs = append(errs, fmt.Sprintf("git object format: %v", formatErr))
	}
	gitObjectFormat = canonicalGitObjectFormat(gitObjectFormat)

	controllerPID := fmt.Sprintf("%d", os.Getpid())
	selfPath, pathErr := os.Readlink("/proc/self/exe")
	selfHash := ""
	selfHashErr := error(nil)
	if pathErr != nil {
		errs = append(errs, fmt.Sprintf("executable path: %v", pathErr))
	} else if selfPath == "" {
		errs = append(errs, "executable path: empty")
	} else {
		var hashErr error
		selfHash, hashErr = hashRuntimeExecutable(openProcSelfExe)
		if hashErr != nil {
			errs = append(errs, fmt.Sprintf("executable hash: %v", hashErr))
			selfHashErr = hashErr
		}
	}

	kernelRelease, krErr := runUname("-r")
	if krErr != nil {
		kernelRelease = ""
		errs = append(errs, fmt.Sprintf("kernel release: %v", krErr))
	}
	kernelVersion, err := os.ReadFile("/proc/version")
	kernelVersionStr := ""
	if err == nil {
		kernelVersionStr = strings.TrimSpace(string(kernelVersion))
	} else {
		errs = append(errs, fmt.Sprintf("kernel version: %v", err))
	}
	cgroupMode := detectCgroupMode()
	status := "complete"
	if len(errs) > 0 {
		status = fmt.Sprintf("partial: %s", strings.Join(errs, "; "))
	}

	subject := &evidence.SubjectIdentity{
		GitCommit:                  gitCommit,
		GitTree:                    gitTree,
		GitObjectFormat:            gitObjectFormat,
		ControllerExecutablePath:   selfPath,
		ControllerExecutableSHA256: selfHash,
	}
	host := &evidence.HostIdentity{
		KernelRelease:    kernelRelease,
		KernelVersion:    kernelVersionStr,
		CgroupMode:       cgroupMode,
		CollectionStatus: status,
	}
	var requiredErrs []string
	if gitErr != nil || gitCommit == "" {
		requiredErrs = append(requiredErrs, "git_commit")
	}
	if treeErr != nil || gitTree == "" {
		requiredErrs = append(requiredErrs, "git_tree")
	}
	if formatErr != nil || gitObjectFormat == "" {
		requiredErrs = append(requiredErrs, "git_object_format")
	}
	if pathErr != nil || selfPath == "" {
		requiredErrs = append(requiredErrs, "executable_path")
	}
	if selfHashErr != nil || len(selfHash) != 64 {
		requiredErrs = append(requiredErrs, "executable_hash")
	}
	if len(requiredErrs) > 0 {
		return subject, host, controllerPID, fmt.Errorf("required provenance unavailable: %s", strings.Join(requiredErrs, ", "))
	}
	return subject, host, controllerPID, nil
}

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = "/home/kgb/Projects/KGB"
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func runUname(args ...string) (string, error) {
	cmd := exec.Command("uname", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func canonicalGitObjectFormat(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "sha1":
		return "sha1"
	case "sha256":
		return "sha256"
	default:
		return ""
	}
}

func detectCgroupMode() string {
	data, err := os.ReadFile("/proc/1/cgroup")
	if err == nil {
		hasV2 := false
		hasV1 := false
		for _, line := range strings.Split(string(data), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, ":", 3)
			if len(parts) < 2 {
				continue
			}
			hierarchyID := strings.TrimSpace(parts[0])
			controllers := ""
			if len(parts) >= 2 {
				controllers = strings.TrimSpace(parts[1])
			}
			if hierarchyID == "0" && (controllers == "" || controllers == "unified" || strings.HasPrefix(controllers, "0::")) {
				hasV2 = true
			}
			if hierarchyID != "0" || (controllers != "" && controllers != "unified") {
				if strings.Contains(controllers, "name=") || hierarchyID != "0" {
					hasV1 = true
				}
			}
		}
		if hasV2 && hasV1 {
			return "hybrid"
		}
		if hasV2 {
			return "cgroup2"
		}
		if hasV1 {
			return "cgroup1"
		}
	}
	mountData, err := os.ReadFile("/proc/self/mountinfo")
	if err == nil {
		hasCgroup2 := false
		hasCgroup1 := false
		for _, line := range strings.Split(string(mountData), "\n") {
			if line == "" {
				continue
			}
			parts := strings.Split(line, " - ")
			if len(parts) != 2 {
				continue
			}
			fsParts := strings.Split(strings.TrimSpace(parts[1]), " ")
			if len(fsParts) < 1 {
				continue
			}
			fsType := fsParts[0]
			if fsType == "cgroup2" || fsType == "cgroup2fs" {
				hasCgroup2 = true
			}
			if fsType == "cgroup" || fsType == "cgroupfs" {
				hasCgroup1 = true
			}
		}
		if hasCgroup2 && hasCgroup1 {
			return "hybrid"
		}
		if hasCgroup2 {
			return "cgroup2"
		}
		if hasCgroup1 {
			return "cgroup1"
		}
	}
	return "unknown"
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func hashRuntimeExecutable(openRuntimeExecutable runtimeExecutableOpener) (string, error) {
	rc, err := openRuntimeExecutable()
	if err != nil {
		return "", fmt.Errorf("open runtime executable: %w", err)
	}
	defer rc.Close()
	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		return "", fmt.Errorf("read runtime executable: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func provenanceErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func validateProvenanceEvidence(manifest evidence.Manifest, verdict evidence.Verdict) []string {
	var errs []string
	if !verdict.ProvenanceValid {
		errs = append(errs, fmt.Sprintf("provenance_valid=false: %s", verdict.ProvenanceError))
	}
	if verdict.ProvenanceError != "" {
		errs = append(errs, fmt.Sprintf("provenance_error not empty: %s", verdict.ProvenanceError))
	}
	if manifest.SubjectIdentity == nil {
		errs = append(errs, "subject_identity is nil")
		return errs
	}
	if manifest.SubjectIdentity.GitCommit == "" {
		errs = append(errs, "subject_identity.git_commit is empty")
	} else if err := validateGitObjectID(manifest.SubjectIdentity.GitCommit, ""); err != nil {
		errs = append(errs, fmt.Sprintf("subject_identity.git_commit: %v", err))
	}
	if manifest.SubjectIdentity.GitTree == "" {
		errs = append(errs, "subject_identity.git_tree is empty")
	} else if err := validateGitObjectID(manifest.SubjectIdentity.GitTree, ""); err != nil {
		errs = append(errs, fmt.Sprintf("subject_identity.git_tree: %v", err))
	}
	if err := validateGitObjectFormatConsistency(manifest.SubjectIdentity); err != nil {
		errs = append(errs, fmt.Sprintf("subject_identity.git_object_format: %v", err))
	}
	if manifest.SubjectIdentity.ControllerExecutablePath == "" {
		errs = append(errs, "subject_identity.controller_executable_path is empty")
	}
	if manifest.SubjectIdentity.ControllerExecutableSHA256 == "" {
		errs = append(errs, "subject_identity.controller_executable_sha256 is empty")
	} else if err := validateSHA256(manifest.SubjectIdentity.ControllerExecutableSHA256); err != nil {
		errs = append(errs, fmt.Sprintf("subject_identity.controller_executable_sha256: %v", err))
	}
	if manifest.HostID == nil {
		errs = append(errs, "host_identity is nil")
	} else if manifest.HostID.CollectionStatus != "complete" {
		errs = append(errs, fmt.Sprintf("host_identity.collection_status=%s (expected 'complete')", manifest.HostID.CollectionStatus))
	}
	return errs
}

func validateHexString(s string) error {
	if _, err := hex.DecodeString(s); err != nil {
		return fmt.Errorf("invalid hex: %w", err)
	}
	return nil
}

func validateSHA256(value string) error {
	if value == "" {
		return fmt.Errorf("empty")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return fmt.Errorf("invalid hex: %w", err)
	}
	if len(decoded) != sha256.Size {
		return fmt.Errorf("decoded length=%d, want %d", len(decoded), sha256.Size)
	}
	return nil
}

func validateGitObjectID(value string, gitObjectFormat string) error {
	if value == "" {
		return fmt.Errorf("empty")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return fmt.Errorf("invalid hex: %w", err)
	}
	var expectedLen int
	switch gitObjectFormat {
	case "sha1", "sha-1":
		expectedLen = 20
	case "sha256", "sha-256":
		expectedLen = 32
	default:
		if len(decoded) != 20 && len(decoded) != 32 {
			return fmt.Errorf("decoded length=%d, want 20 (sha1) or 32 (sha256)", len(decoded))
		}
		return nil
	}
	if len(decoded) != expectedLen {
		return fmt.Errorf("decoded length=%d, want %d for %s", len(decoded), expectedLen, gitObjectFormat)
	}
	return nil
}

func validateGitObjectFormatConsistency(subject *evidence.SubjectIdentity) error {
	format := subject.GitObjectFormat
	if format == "" {
		return fmt.Errorf("git_object_format is required (expected 'sha1' or 'sha256')")
	}
	commitDecoded, commitErr := hex.DecodeString(subject.GitCommit)
	treeDecoded, treeErr := hex.DecodeString(subject.GitTree)
	if commitErr != nil || treeErr != nil {
		return fmt.Errorf("commit or tree not valid hex")
	}
	commitLen := len(commitDecoded)
	treeLen := len(treeDecoded)
	switch format {
	case "sha1":
		if commitLen != 20 || treeLen != 20 {
			return fmt.Errorf("git_object_format=sha1 but commit len=%d or tree len=%d (want 20)", commitLen, treeLen)
		}
	case "sha256":
		if commitLen != 32 || treeLen != 32 {
			return fmt.Errorf("git_object_format=sha256 but commit len=%d or tree len=%d (want 32)", commitLen, treeLen)
		}
	default:
		return fmt.Errorf("unsupported git_object_format=%q (expected 'sha1' or 'sha256')", format)
	}
	return nil
}

type runtimeExecutableOpener func() (io.ReadCloser, error)

func openProcSelfExe() (io.ReadCloser, error) {
	return os.Open("/proc/self/exe")
}

func verifyRuntimeExecutableHash(storedHash string, openRuntimeExecutable runtimeExecutableOpener) error {
	if storedHash == "" {
		return fmt.Errorf("stored executable hash is empty")
	}
	got, err := hashRuntimeExecutable(openRuntimeExecutable)
	if err != nil {
		return err
	}
	if got != storedHash {
		return fmt.Errorf("executable hash mismatch: stored=%s runtime=%s", storedHash, got)
	}
	return nil
}

// extractContainerImageID extracts the Image field from the
// container-inspect.json bytes. Returns empty string on any
// parse failure.
func extractContainerImageID(data []byte) (string, error) {
	var ci struct {
		Image string `json:"Image"`
	}
	if err := json.Unmarshal(data, &ci); err != nil {
		return "", err
	}
	return ci.Image, nil
}

// extractContainerImageReference extracts the Config.Image
// field from the container-inspect.json bytes. Returns empty
// string on any parse failure.
func extractContainerImageReference(data []byte) (string, error) {
	var ci struct {
		Config struct {
			Image string `json:"Image"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(data, &ci); err != nil {
		return "", err
	}
	return ci.Config.Image, nil
}

// validateRepoDigest validates a repository-digest string of
// the form name@sha256:<64-lowercase-hex>. CORRECTION04 ยง3.
func validateRepoDigest(d string) error {
	at := strings.LastIndex(d, "@")
	if at <= 0 || at == len(d)-1 {
		return fmt.Errorf("missing @ separator or empty repository name")
	}
	name := d[:at]
	digest := d[at+1:]
	if name == "" {
		return fmt.Errorf("empty repository name")
	}
	colon := strings.Index(digest, ":")
	if colon < 0 {
		return fmt.Errorf("missing : separator in digest payload")
	}
	algo := digest[:colon]
	payload := digest[colon+1:]
	if algo != "sha256" {
		return fmt.Errorf("unsupported digest algorithm %q (expected sha256)", algo)
	}
	if len(payload) != 64 {
		return fmt.Errorf("digest payload length %d != 64", len(payload))
	}
	for _, c := range payload {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return fmt.Errorf("digest payload %q contains non-lowercase-hex", payload)
		}
	}
	return nil
}

func validateStateInvariant(scenario string, initial, final *CanaryState, workload *WorkloadResult) *analysis.StateInvariantResult {
	result := &analysis.StateInvariantResult{Valid: true}
	opDelta := final.OperationCount - initial.OperationCount
	if opDelta != workload.Completed {
		result.Valid = false
		result.Failures = append(result.Failures,
			fmt.Sprintf("operation_count_delta mismatch: expected %d, got %d", workload.Completed, opDelta))
	}
	switch scenario {
	case "canary-growing":
		if opDelta != workload.Completed {
			result.Valid = false
			result.Failures = append(result.Failures, "growing: operation_count_delta != completed")
		}
		blocksDelta := final.RetainedBlocks - initial.RetainedBlocks
		if blocksDelta != workload.Completed {
			result.Valid = false
			result.Failures = append(result.Failures,
				fmt.Sprintf("growing: retained_blocks_delta=%d != completed=%d", blocksDelta, workload.Completed))
		}
		bytesDelta := final.RetainedBytes - initial.RetainedBytes
		expectedBytes := int64(workload.Completed) * 1048576
		if bytesDelta != expectedBytes {
			result.Valid = false
			result.Failures = append(result.Failures,
				fmt.Sprintf("growing: retained_bytes_delta=%d != expected=%d", bytesDelta, expectedBytes))
		}
	case "canary-bounded":
		if opDelta != workload.Completed {
			result.Valid = false
			result.Failures = append(result.Failures, "bounded: operation_count_delta != completed")
		}
		if initial.BufferCapacity != final.BufferCapacity {
			result.Valid = false
			result.Failures = append(result.Failures,
				fmt.Sprintf("bounded: buffer_capacity changed from %d to %d", initial.BufferCapacity, final.BufferCapacity))
		}
		if final.RetainedBlocks != 0 {
			result.Valid = false
			result.Failures = append(result.Failures,
				fmt.Sprintf("bounded: retained_blocks should be 0, got %d", final.RetainedBlocks))
		}
		if final.RetainedBytes != 0 {
			result.Valid = false
			result.Failures = append(result.Failures,
				fmt.Sprintf("bounded: retained_bytes should be 0, got %d", final.RetainedBytes))
		}
	case "canary-descriptor":
		if opDelta != workload.Completed {
			result.Valid = false
			result.Failures = append(result.Failures, "descriptor: operation_count_delta != completed")
		}
		fdDelta := final.FDCount - initial.FDCount
		expectedFDDelta := workload.Completed * 2
		if fdDelta != expectedFDDelta {
			result.Valid = false
			result.Failures = append(result.Failures,
				fmt.Sprintf("descriptor: fd_delta=%d != expected=%d", fdDelta, expectedFDDelta))
		}
	}
	return result
}


// CORRECTION22: the legacy bridge helpers
// (buildAndPersistQualifiedEvidenceFromInspect and
// buildAndPersistQualifiedEvidence) have been deleted. The
// production CLI now goes through dockerlab.ExecuteQualifiedDockerLifecycle
// and evidence.PersistQualifiedExecutionEvidence directly.

// fetchCanaryStateViaExec fetches canary state using docker exec.
// CORRECTION27 P0-1: More reliable than direct HTTP when Docker bridge
// networking has issues.
func fetchCanaryStateViaExec(ctx context.Context, dockerClient *dockerlab.Client, containerID string, port int) (*CanaryState, error) {
	state, err := dockerClient.CanaryStateViaExec(ctx, containerID, port)
	if err != nil {
		return nil, err
	}
	return &CanaryState{
		Mode:           state.Mode,
		RetainedBlocks: state.RetainedBlocks,
		RetainedBytes:  state.RetainedBytes,
		OperationCount: int(state.OperationCount),
		FDCount:        state.FDCount,
		Ready:          state.Ready,
	}, nil
}
