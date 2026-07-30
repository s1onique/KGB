// Package main provides the UVB-76 pprof memory leak lab.
//
// # Memory Lab Runner with Generated Authority
//
// P0-1 through P0-15: All authorities wired into production path.
// P0-2: Creates GeneratedLabAuthority ONCE before any process startup.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	fake "github.com/s1onique/KGB/uvb76/cmd/uvb76-memleak-pprof-lab/internal/fake"
)

// runLab orchestrates the full memory leak lab with generated authority.
// P0-6: runLab consumes the validated authority explicitly.
// P0-7-fix: Accept ephemeralPassword for post-startup authentication.
func runLab(authority *GeneratedLabAuthority, ephemeralPassword []byte) LabResult {
	if authority == nil {
		return LabResult{
			OK:             false,
			Classification: "FAILED",
			Errors:         []string{"nil authority"},
		}
	}

	// === PHASE 0: Pre-start embedded source identity validation ===
	embeddedIdentity, sourceErr := ProductionSourceIdentityResolver.Resolve()
	if sourceErr != nil {
		log.Printf("[SOURCE] Embedded source identity resolution failed: %v", sourceErr)
		return LabResult{
			OK:             false,
			Classification: "FAILED",
			Errors:         []string{fmt.Sprintf("embedded source identity: %v", sourceErr)},
		}
	}

	// P0-6: Use authority fields, not flag package globals
	// P0-1: Extract decimal ports from URLs using proper URL parsing
	tovarischPort := extractPortFromURL(authority.Target.BaseURL)
	uvb76APIPort := extractPortFromURL(authority.UVB76APIBaseURL)
	pprofPort := extractPortFromURL("http://" + authority.Config.Diagnostics.PProf.Listen)

	identity := &runExecutionIdentity{
		RunID:            fmt.Sprintf("run-%d", time.Now().Unix()),
		SourceCommit:     embeddedIdentity.VCSRevision,
		RunStartedAt:     time.Now(),
		ArtifactDir:      artifactDir,
		TovarischBinPath: *flagTovarischBin,
		UVB76BinPath:     *flagUVB76Bin,
		Endpoints: RuntimeEndpoints{
			TovarischBaseURL: authority.Target.BaseURL,
			TovarischPort:    tovarischPort,
			UVB76APIBaseURL:  authority.UVB76APIBaseURL,
			UVB76Port:        uvb76APIPort,
			PProfBaseURL:     "http://" + authority.Config.Diagnostics.PProf.Listen,
			PProfPort:        pprofPort,
		},
	}

	// P0-5: Validate identity using mode from authority
	identityValidationErrors := validateRunExecutionIdentity(identity, authority.Mode == ExecutionModeFake)
	if len(identityValidationErrors) > 0 {
		var errMsgs []string
		for _, e := range identityValidationErrors {
			errMsgs = append(errMsgs, fmt.Sprintf("identity validation: %v", e))
		}
		return LabResult{
			OK:             false,
			Classification: "FAILED",
			Errors:         errMsgs,
		}
	}

	result := LabResult{
		DurationSeconds:     int(flagDuration.Seconds()),
		ArtifactDir:         artifactDir,
		Classification:      "PARTIAL", // Default until proven otherwise
		RuntimeArtifactsDir: artifactDir,
	}

	tovarischCmd, uvb76Cmd := (*exec.Cmd)(nil), (*exec.Cmd)(nil)
	var tovarischPS, uvb76PS *ProcessState

	log.Printf("[IDENTITY] RunID=%s, Commit=%s, TargetID=%s, Mode=%s",
		identity.RunID, identity.SourceCommit, authority.Target.TargetID, authority.Mode)

	// === PHASE 1: Start Tovarisch (or fake server) ===
	if authority.Mode == ExecutionModeFake {
		log.Printf("[SETUP] Starting fake tovarisch on port %s", authority.Target.BaseURL)
		if err := startFakeTovarischFromAuthority(authority); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("start fake tovarisch: %v", err))
			result.Classification = "FAILED"
			return result
		}
		result.RealTovarischStarted = true
		result.TovarischBinPath = identity.TovarischBinPath
	} else {
		// Start real Tovarisch
		log.Printf("[SETUP] Starting real Tovarisch: %s", *flagTovarischBin)

		port := extractPortFromURL(authority.Target.BaseURL)
		var err error
		tovarischCmd, tovarischPS, err = startTovarisch(*flagTovarischBin, *flagTovarischArgs, port)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("start tovarisch: %v", err))
			result.Classification = "FAILED"
			return result
		}

		result.TovarischStartTime = &tovarischPS.StartTime
		result.RealTovarischStarted = true
		result.TovarischPID = tovarischPID
		result.TovarischBinPath = *flagTovarischBin
		if tovarischPS != nil && tovarischPS.Argv != nil {
			result.TovarischArgv = tovarischPS.Argv
		}
	}

	// === PHASE 2: Wait for Tovarisch readiness ===
	tovarischReady := waitForTovarischReadyFromAuthority(authority)
	now := time.Now()
	if tovarischReady {
		result.RealTovarischReady = true
		result.TovarischReadyTime = &now
		log.Printf("[READY] Tovarisch ready")
	} else {
		result.Errors = append(result.Errors, "tovarisch did not become ready")
		result.Classification = "FAILED"
		// P0-4: Use canonical finalizer for universal finalization authority
		processes := &failedRunProcesses{
			TovarischCmd: tovarischCmd,
			UVB76Cmd:     uvb76Cmd,
			TovarischPS:  tovarischPS,
			UVB76PS:      uvb76PS,
		}
		finalizationErr := finalizeLifecycleFailure(&result, errors.New("tovarisch readiness timeout"), identity, processes)
		if finalizationErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("finalization: %v", finalizationErr))
		}
		return result
	}

	// === PHASE 3: Start UVB-76 ===
	log.Printf("[LAUNCH] Starting UVB-76: %s", *flagUVB76Bin)

	var err error
	uvb76Cmd, uvb76PS, err = startUVB76WithConfigPath(*flagUVB76Bin, authority.ConfigPath)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("start uvb76: %v", err))
		result.Classification = "FAILED"

		processes := &failedRunProcesses{
			TovarischCmd: tovarischCmd,
			UVB76Cmd:     uvb76Cmd,
			TovarischPS:  tovarischPS,
			UVB76PS:      uvb76PS,
		}
		finalizationErr := finalizeLifecycleFailure(&result, err, identity, processes)
		if finalizationErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("finalization: %v", finalizationErr))
		}
		return result
	}

	result.UVB76StartTime = &uvb76PS.StartTime
	result.RealUVB76Started = true
	result.UVB76PID = uvb76PID
	result.UVB76BinPath = *flagUVB76Bin
	if uvb76PS != nil && uvb76PS.Argv != nil {
		result.UVB76Argv = uvb76PS.Argv
	}

	// === PHASE 4: Wait for UVB-76 pprof readiness ===
	pprofReady, pprofErr := waitForPPROFReady(pprofPort, 30*time.Second, uvb76PS)
	if pprofReady {
		result.UVB76PProfReady = true
		now := time.Now()
		result.UVB76PProfReadyTime = &now
		log.Printf("[READY] UVB-76 pprof ready")
	} else {
		if uvb76PS.Exited() {
			exitCode, _ := uvb76PS.ExitInfo()
			result.Errors = append(result.Errors, fmt.Sprintf("uvb76 exited with code %d before pprof ready: %v", exitCode, pprofErr))
		} else {
			result.Errors = append(result.Errors, fmt.Sprintf("pprof never ready: %v", pprofErr))
		}
		result.Classification = "FAILED"
		// P0-4: Use canonical finalizer for universal finalization authority
		var terminalErr error
		if pprofErr != nil {
			terminalErr = fmt.Errorf("pprof not ready: %w", pprofErr)
		} else {
			terminalErr = errors.New("pprof readiness timeout")
		}
		processes := &failedRunProcesses{
			TovarischCmd: tovarischCmd,
			UVB76Cmd:     uvb76Cmd,
			TovarischPS:  tovarischPS,
			UVB76PS:      uvb76PS,
		}
		finalizationErr := finalizeLifecycleFailure(&result, terminalErr, identity, processes)
		if finalizationErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("finalization: %v", finalizationErr))
		}
		return result
	}

	// Verify pprof endpoints
	if !verifyPPROFEndpoints(pprofPort) {
		result.Errors = append(result.Errors, "pprof endpoint verification failed")
		result.Classification = "FAILED"
		// P0-4: Use canonical finalizer for universal finalization authority
		processes := &failedRunProcesses{
			TovarischCmd: tovarischCmd,
			UVB76Cmd:     uvb76Cmd,
			TovarischPS:  tovarischPS,
			UVB76PS:      uvb76PS,
		}
		finalizationErr := finalizeLifecycleFailure(&result, errors.New("pprof endpoint verification failed"), identity, processes)
		if finalizationErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("finalization: %v", finalizationErr))
		}
		return result
	}

	// === PHASE 4b: Wait for UVB-76 API readiness and resolve authentication ===
	// P0-3: Use typed API readiness with process exit detection
	// P0-4: Route readiness failure through canonical finalizer
	log.Printf("[AUTH] Waiting for UVB-76 API readiness...")

	readinessResult := CheckAPIReadiness(labCtx, APIReadinessInput{
		URL:          authority.UVB76APIBaseURL,
		Deadline:     30 * time.Second,
		PollInterval: 250 * time.Millisecond,
		ProcessExited: func() bool {
			return uvb76PS != nil && uvb76PS.Exited()
		},
	})

	if !readinessResult.Ready {
		// P0-4: Readiness failure uses canonical finalizer
		var terminalErr error
		if readinessResult.TerminalError != nil {
			terminalErr = readinessResult.TerminalError
		} else {
			terminalErr = fmt.Errorf("UVB-76 API readiness deadline exceeded after %d attempts", readinessResult.Attempts)
		}

		processes := &failedRunProcesses{
			TovarischCmd: tovarischCmd,
			UVB76Cmd:     uvb76Cmd,
			TovarischPS:  tovarischPS,
			UVB76PS:      uvb76PS,
		}
		finalizationErr := finalizeLifecycleFailure(&result, terminalErr, identity, processes)
		if finalizationErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("finalization: %v", finalizationErr))
		}
		return result
	}
	log.Printf("[AUTH] UVB-76 API ready after %d attempts, resolving authentication...", readinessResult.Attempts)

	// P0-5: Perform actual login with ephemeral password
	// P0-5: Route auth failure through canonical finalizer
	targetAuth, authErr := ResolveTargetAuth(labCtx, authority, ephemeralPassword)
	if authErr != nil {
		// P0-5: Auth failure uses canonical finalizer
		processes := &failedRunProcesses{
			TovarischCmd: tovarischCmd,
			UVB76Cmd:     uvb76Cmd,
			TovarischPS:  tovarischPS,
			UVB76PS:      uvb76PS,
		}
		finalizationErr := finalizeLifecycleFailure(&result, authErr, identity, processes)
		if finalizationErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("finalization: %v", finalizationErr))
		}
		return result
	}
	authority.TargetStateAuth = targetAuth
	log.Printf("[AUTH] Authentication resolved: cookie=%s", targetAuth.CookieName)

	// === PHASE 5: Collection phase using RunCollectionLifecycle ===
	// P0-7: Single production authority for collection + polling
	log.Printf("[COLLECTION] Starting collection phase...")
	collectionStart := time.Now()
	result.CollectionStartTime = &collectionStart

	// P0-7: Separate deadlines for observation window and final profile completion.
	// P0-7: Both contexts derive from labCtx so cancellation propagates.
	// P0-7: ProfileCtx outlives ObservationCtx to avoid deadline collision.
	observationEnd := collectionStart.Add(*flagDuration)
	finalProfileBudget := 30 * time.Second // Budget for final profile requests after observation ends
	finalProfileDeadline := observationEnd.Add(finalProfileBudget)

	// P0-7: Create two derived contexts from labCtx with separate deadlines
	observationCtx, observationCancel := context.WithDeadline(labCtx, observationEnd)
	defer observationCancel()

	profileCtx, profileCancel := context.WithDeadline(labCtx, finalProfileDeadline)
	defer profileCancel()

	// Start goroutines for collection
	var wg sync.WaitGroup
	var samplesMu sync.Mutex
	var tovarischSamples []ProcessSample
	var uvb76Samples []ProcessSample
	var collectorErrors []string

	// P0-12: Collect process samples with mandatory field presence
	wg.Add(1)
	go func() {
		defer wg.Done()
		collectProcessSamplesStrict(observationCtx, tovarischPID, *flagSampleInterval, &tovarischSamples, &samplesMu, &collectorErrors)
	}()

	// P0-14: Collect UVB-76 samples with typed goroutine observation
	wg.Add(1)
	go func() {
		defer wg.Done()
		collectUVB76SamplesStrict(observationCtx, uvb76PID, pprofPort, *flagSampleInterval, &uvb76Samples, &samplesMu, &collectorErrors)
	}()

	// P0-1: Build poll input for PollTargetAuthority
	pollInput := TargetPollInput{
		Client:          &http.Client{Timeout: 5 * time.Second},
		UVB76APIBaseURL: authority.UVB76APIBaseURL,
		Target:          authority.Target,
		Auth:            authority.TargetStateAuth,
		PollInterval:    5 * time.Second,
		RequestTimeout:  5 * time.Second,
		Deadline:        *flagDuration,
	}

	// P0-1: Run collection lifecycle using exact PollTargetAuthority via PollInput
	// P0-6: Runner owns collector goroutine lifecycle - collectors already started above
	lifecycleResult := RunCollectionLifecycle(CollectionLifecycleInput{
		ObservationCtx:    observationCtx,
		ProfileCtx:        profileCtx,
		ObservationCancel: observationCancel,
		WaitGroup:         &wg,
		CollectorInput: &CollectorInput{
			TovarischSamples: &tovarischSamples,
			UVB76Samples:     &uvb76Samples,
			CollectorErrors:  &collectorErrors,
			SamplesMu:        &samplesMu,
		},
		PollInput:        pollInput,
		PollDrainTimeout: 5 * time.Second,
		CaptureProfilesFn: func(ctx context.Context) error {
			// P0-7: ctx is ProfileCtx with extended deadline for final profiles
			httpClient := &http.Client{Timeout: 30 * time.Second}
			return captureProfilesWithValidation(ctx, httpClient, pprofPort, artifactDir, collectionStart, *flagDuration, *flagProfileInterval, observationEnd)
		},
	})

	pollResult := lifecycleResult.PollResult

	// P0-7: Propagate poll terminal error from lifecycle
	if lifecycleResult.PollTerminalError != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("poll lifecycle failed: %v", lifecycleResult.PollTerminalError))
		result.OK = false
		result.Classification = "FAILED"
	}

	// P0-11: Target observation is mandatory
	if pollResult.BestAuthority != nil {
		result.RealTargetObserved = true
		result.ScrapeAttempted = pollResult.BestAuthority.IsScrapeAttempted()
		result.ScrapeCompleted = pollResult.BestAuthority.IsScrapeCompleted()

		// P0-10: Identity was validated during polling
		// Additional validation for safety
		if err := validateSnapshotIdentity(pollResult.BestAuthority, authority.Target); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("target identity validation failed: %v", err))
			result.Classification = "FAILED"
			result.OK = false
		}
	}

	// P0-11: Fail closed if no target observation
	if !result.RealTargetObserved {
		result.Errors = append(result.Errors, "target observation mandatory: no target observed")
		result.Classification = "FAILED"
		result.OK = false
	}

	// P0-11: Fail closed if poll failed with terminal error
	if pollResult.TerminalError != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("target poll terminal error: %v", pollResult.TerminalError))
		result.Classification = "FAILED"
		result.OK = false
	}

	// P0-6: Use snapshot from RunCollectionLifecycle (CollectAndSnapshot already called inside)
	snapshot := lifecycleResult.Snapshot
	if lifecycleResult.SnapshotErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("collector lifecycle failed: %v", lifecycleResult.SnapshotErr))
		result.Classification = "FAILED"
		result.OK = false

		processes := &failedRunProcesses{
			TovarischCmd: tovarischCmd,
			UVB76Cmd:     uvb76Cmd,
			TovarischPS:  tovarischPS,
			UVB76PS:      uvb76PS,
		}
		finalizationErr := finalizeLifecycleFailure(&result, lifecycleResult.SnapshotErr, identity, processes)
		if finalizationErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("finalization: %v", finalizationErr))
		}
		return result
	}
	tovarischFinal := snapshot.TovarischSamples
	uvb76Final := snapshot.UVB76Samples
	collectorErrorsFinal := snapshot.CollectorErrors

	collectionEnd := time.Now()
	result.CollectionEndTime = &collectionEnd
	log.Printf("[COLLECTION] Collection phase complete")

	// Check process samples
	result.ProcessSamplesPresent = len(tovarischFinal) > 0 && len(uvb76Final) > 0

	// Validate profiles
	profileErr := ValidateAllProfiles(artifactDir)
	result.ProfilesPresent = profileErr == nil
	if profileErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("profile validation failed: %v", profileErr))
	}

	// Propagate profile capture errors from lifecycle
	if lifecycleResult.ProfileErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("profile capture failed: %v", lifecycleResult.ProfileErr))
	}

	// Compute deltas
	tovarischDelta := ComputeProcessDeltas(tovarischFinal)
	uvb76Delta := ComputeProcessDeltas(uvb76Final)

	// Write series files
	var artifactErrors []string
	if err := writeProcessSeries("tovarisch", tovarischFinal); err != nil {
		artifactErrors = append(artifactErrors, fmt.Sprintf("tovarisch series write: %v", err))
	}
	if err := writeProcessSeries("uvb76", uvb76Final); err != nil {
		artifactErrors = append(artifactErrors, fmt.Sprintf("uvb76 series write: %v", err))
	}

	// Write delta summaries
	if err := writeDeltaSummary("tovarisch", tovarischDelta); err != nil {
		artifactErrors = append(artifactErrors, fmt.Sprintf("tovarisch delta write: %v", err))
	}
	if err := writeDeltaSummary("uvb76", uvb76Delta); err != nil {
		artifactErrors = append(artifactErrors, fmt.Sprintf("uvb76 delta write: %v", err))
	}

	// Accumulate artifact errors
	result.Errors = append(result.Errors, artifactErrors...)

	// Propagate collector errors
	result.Errors = append(result.Errors, collectorErrorsFinal...)

	// === PHASE 6: Cleanup ===
	cleanupErrors := cleanup(tovarischCmd, uvb76Cmd, tovarischPS, uvb76PS)
	result.Errors = append(result.Errors, cleanupErrors...)

	// Verify cleanup
	result.UVB76Removed = uvb76PID == 0 || processIsGone(uvb76PID)
	result.TovarischRemoved = tovarischPID == 0 || processIsGone(tovarischPID)

	// Verify port release
	portErr := verifyPortsReleasedFromAuthority(authority)
	if portErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("port release verification failed: %v", portErr))
	}
	result.PortsReleased = portErr == nil

	// === PHASE 7: Final classification ===
	result.Classification, result.OK = classifyLabResult(result)

	// === PHASE 8: Persist result ===
	builtResult, buildErr := BuildResultFromLabResult(identity, result)
	if buildErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("result construction: %v", buildErr))
		result.Classification = "FAILED"
		result.OK = false
		return result
	}

	// P0-15: Publish target authority and poll diagnostics
	if pollResult.BestAuthority != nil {
		builtResult.ScrapeAuthority = &ScrapeAuthorityObservation{
			TargetID:            pollResult.BestAuthority.TargetID,
			AttemptObserved:     pollResult.BestAuthority.AttemptObserved,
			AttemptTimestamp:    &pollResult.BestAuthority.AttemptTimestamp,
			CompletionObserved:  pollResult.BestAuthority.CompletionObserved,
			CompletionTimestamp: &pollResult.BestAuthority.CompletionTimestamp,
			Reachable:           pollResult.BestAuthority.Reachable,
			Status:              pollResult.BestAuthority.Status,
			Error:               pollResult.BestAuthority.Error,
		}
		builtResult.TovarischSeriesFile = "tovarisch-process-series.csv"
		builtResult.UVB76SeriesFile = "uvb76-process-series.csv"
		builtResult.TovarischSampleCount = len(tovarischFinal)
		builtResult.UVB76SampleCount = len(uvb76Final)
	}

	// Persist result
	if persistErr := persistResult(builtResult, artifactDir); persistErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("result persistence failed: %v", persistErr))
		result.Classification = "FAILED"
		result.OK = false
	}

	return result
}

// extractPortFromURL extracts the port from a URL using proper URL parsing.
// P0-3: Uses net/url for structured URL parsing instead of string manipulation.
func extractPortFromURL(urlStr string) string {
	u, err := parseURL(urlStr)
	if err != nil {
		return ""
	}
	return u.Port()
}

// parseURL parses a URL string using the standard library.
// P0-3: Centralized URL parsing for testability.
var parseURL = func(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}

// startFakeTovarischFromAuthority starts fake tovarisch using authority.
func startFakeTovarischFromAuthority(authority *GeneratedLabAuthority) error {
	fakePort := extractPortFromURL(authority.Target.BaseURL)

	fakeServer = &fake.StatusServer{
		Port:    fakePort,
		LogFile: tovarischLogFile,
	}

	if err := fakeServer.Start(); err != nil {
		return fmt.Errorf("start fake server: %w", err)
	}

	fakePID := 99999
	tovarischPID = fakePID
	pidFile := filepath.Join(artifactDir, "tovarisch.pid")
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", fakePID)), 0644)

	log.Printf("[LAUNCH] Fake Tovarisch started on port %s, log=%s", fakePort, tovarischLogFile)
	return nil
}

// waitForTovarischReadyFromAuthority waits for tovarisch using authority.
func waitForTovarischReadyFromAuthority(authority *GeneratedLabAuthority) bool {
	fakePort := extractPortFromURL(authority.Target.BaseURL)
	deadline := time.Now().Add(15 * time.Second)
	url := fmt.Sprintf("http://localhost:%s/status", fakePort)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

// startUVB76WithConfigPath starts UVB-76 with the specified config path.
func startUVB76WithConfigPath(bin, configPath string) (*exec.Cmd, *ProcessState, error) {
	args := []string{
		"-dev",
		"-config", configPath,
	}

	cmd := exec.Command(bin, args...)

	logOut, err := os.OpenFile(uvb76LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	cmd.Stdout = logOut
	cmd.Stderr = logOut

	startTime := time.Now()

	if err := cmd.Start(); err != nil {
		logOut.Close()
		return nil, nil, fmt.Errorf("start: %w", err)
	}
	uvb76PID = cmd.Process.Pid

	pidFile := filepath.Join(artifactDir, "uvb76.pid")
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", uvb76PID)), 0644)

	clonedArgv := make([]string, len(cmd.Args))
	copy(clonedArgv, cmd.Args)
	ps := &ProcessState{
		StartTime: startTime,
		Argv:      clonedArgv,
	}
	ps.done = make(chan struct{})

	ps.mu.Lock()
	ps.running = true
	ps.exited = false
	ps.mu.Unlock()

	go func() {
		_ = cmd.Wait()
		_ = logOut.Close()

		ps.mu.Lock()
		defer ps.mu.Unlock()

		ps.running = false
		ps.exited = true

		if cmd.ProcessState != nil {
			ps.exitCode = cmd.ProcessState.ExitCode()
		}

		close(ps.done)
	}()

	log.Printf("[LAUNCH] UVB-76 started: PID=%d, log=%s", uvb76PID, uvb76LogFile)
	return cmd, ps, nil
}

// verifyPortsReleasedFromAuthority verifies ports are released using authority.
func verifyPortsReleasedFromAuthority(authority *GeneratedLabAuthority) error {
	ports := []string{
		extractPortFromURL(authority.Target.BaseURL),
		extractPortFromURL("http://" + authority.Config.Listen.Addr),
		extractPortFromURL("http://" + authority.Config.Diagnostics.PProf.Listen),
	}

	for _, port := range ports {
		addr := fmt.Sprintf("localhost:%s", port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("port %s still in use", port)
		}
		ln.Close()
	}
	return nil
}

// SamplingError represents a structured sampling failure.
type SamplingError struct {
	Process  string
	Phase    string
	PID      int
	InnerErr error
}

func (e *SamplingError) Error() string {
	return fmt.Sprintf("%s %s error (PID %d): %v", e.Process, e.Phase, e.PID, e.InnerErr)
}

// collectProcessSamplesStrict collects process metrics with mandatory field presence.
// P0-12: Requires all mandatory fields before appending sample.
// P0-13: Reports typed disappearance before first sample.
func collectProcessSamplesStrict(ctx context.Context, pid int, interval time.Duration, samples *[]ProcessSample, mu *sync.Mutex, errors *[]string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	firstSampleSeen := false
	for {
		select {
		case <-ticker.C:
			// P0-13: Check process before first sample
			if processIsGone(pid) {
				mu.Lock()
				if !firstSampleSeen {
					// P0-13: Process disappeared before first sample
					*errors = append(*errors, (&SamplingError{
						Process:  "tovarisch",
						Phase:    "disappearance_before_first_sample",
						PID:      pid,
						InnerErr: ErrProcessDisappeared,
					}).Error())
				}
				mu.Unlock()
				return
			}

			// P0-12: Use strict sampler that enforces mandatory fields
			sample, err := sampleProcessMetricsWithPresence(pid)
			if err != nil {
				log.Printf("[METRICS] Tovarisch sampling error: %v", err)
				mu.Lock()
				*errors = append(*errors, (&SamplingError{
					Process:  "tovarisch",
					Phase:    "sampling",
					PID:      pid,
					InnerErr: err,
				}).Error())
				mu.Unlock()
				continue
			}

			firstSampleSeen = true
			mu.Lock()
			*samples = append(*samples, *sample)
			mu.Unlock()

		case <-ctx.Done():
			return
		}
	}
}

// collectUVB76SamplesStrict collects UVB-76 metrics with typed goroutine observation.
// P0-14: Goroutine observation returns typed result, not zero on error.
func collectUVB76SamplesStrict(ctx context.Context, pid int, pprofPort string, interval time.Duration, samples *[]ProcessSample, mu *sync.Mutex, errors *[]string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	client := &http.Client{Timeout: 2 * time.Second}
	firstSampleSeen := false
	pprofBaseURL := fmt.Sprintf("http://localhost:%s", pprofPort)

	for {
		select {
		case <-ticker.C:
			// P0-13: Check process before first sample
			if processIsGone(pid) {
				mu.Lock()
				if !firstSampleSeen {
					// P0-13: Process disappeared before first sample
					*errors = append(*errors, (&SamplingError{
						Process:  "uvb76",
						Phase:    "disappearance_before_first_sample",
						PID:      pid,
						InnerErr: ErrProcessDisappeared,
					}).Error())
				}
				mu.Unlock()
				return
			}

			// P0-12: Use strict sampler for process metrics
			sample, err := sampleProcessMetricsWithPresence(pid)
			if err != nil {
				log.Printf("[METRICS] UVB-76 sampling error: %v", err)
				mu.Lock()
				*errors = append(*errors, (&SamplingError{
					Process:  "uvb76",
					Phase:    "sampling",
					PID:      pid,
					InnerErr: err,
				}).Error())
				mu.Unlock()
				continue
			}

			// P0-14: Use typed goroutine observation
			gc, gcErr := FetchGoroutineCount(ctx, client, pprofBaseURL)
			if gcErr != nil {
				log.Printf("[METRICS] Goroutine observation error: %v", gcErr)
				mu.Lock()
				*errors = append(*errors, (&SamplingError{
					Process:  "uvb76",
					Phase:    "goroutine",
					PID:      pid,
					InnerErr: gcErr,
				}).Error())
				mu.Unlock()
				// P0-14: Do not append sample without goroutine authority
				continue
			}
			sample.GoroutineCount = gc

			firstSampleSeen = true
			mu.Lock()
			*samples = append(*samples, *sample)
			mu.Unlock()

		case <-ctx.Done():
			return
		}
	}
}

// fetchGoroutineCount is kept for backward compatibility with tests.
// P0-14: Deprecated - use FetchGoroutineCount instead.
func fetchGoroutineCount(client *http.Client, pprofPort string) int64 {
	count, err := FetchGoroutineCount(context.Background(), client, fmt.Sprintf("http://localhost:%s", pprofPort))
	if err != nil {
		return 0
	}
	return count
}

// captureProfilesWithValidation captures and validates all required profiles.
// P0-7: Context-aware profile capture - observes context cancellation.
// P0-7: Uses the passed context (which has extended deadline for final profiles).
func captureProfilesWithValidation(
	ctx context.Context,
	client *http.Client,
	pprofPort, artifactDir string,
	start time.Time,
	duration, interval time.Duration,
	observationEnd time.Time, // P0-7: When observation phase ends
) error {
	profiles := []struct {
		name string
		url  string
	}{
		{"heap", fmt.Sprintf("http://localhost:%s/debug/pprof/heap", pprofPort)},
		{"allocs", fmt.Sprintf("http://localhost:%s/debug/pprof/allocs", pprofPort)},
		{"goroutine", fmt.Sprintf("http://localhost:%s/debug/pprof/goroutine?debug=1", pprofPort)},
	}

	var errs []error

	// Capture at start
	for _, p := range profiles {
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), errors.New("profile capture cancelled at start"))
		default:
		}
		outPath := filepath.Join(artifactDir, fmt.Sprintf("%s-start.%s", p.name, getExtension(p.name)))
		if err := CaptureProfile(ctx, client, p.url, outPath, p.name); err != nil {
			log.Printf("[PROFILE] Capture failed at start for %s: %v", p.name, err)
			errs = append(errs, fmt.Errorf("%s-start: %w", p.name, err))
		}
	}

	// Capture at midpoint (using collection context)
	midTime := start.Add(interval)
	if midTime.Before(start.Add(duration)) {
		if err := sleepUntilWithContext(ctx, midTime); err != nil {
			return errors.Join(err, errors.New("profile capture cancelled at midpoint"))
		}
		for _, p := range profiles {
			select {
			case <-ctx.Done():
				return errors.Join(ctx.Err(), errors.New("profile capture cancelled at midpoint"))
			default:
			}
			outPath := filepath.Join(artifactDir, fmt.Sprintf("%s-mid.%s", p.name, getExtension(p.name)))
			if err := CaptureProfile(ctx, client, p.url, outPath, p.name); err != nil {
				log.Printf("[PROFILE] Capture failed at mid for %s: %v", p.name, err)
				errs = append(errs, fmt.Errorf("%s-mid: %w", p.name, err))
			}
		}
	}

	// P0-7: Sleep until observation end using the passed-in context (which has extended deadline).
	// P0-7: ctx derives from labCtx so parent cancellation still propagates.
	// P0-7: ctx has finalProfileDeadline so final profiles can complete.
	if err := sleepUntilWithContext(ctx, observationEnd); err != nil {
		return errors.Join(err, errors.New("profile capture cancelled before final"))
	}

	// P0-7: Capture at end using the same context (ctx = ProfileCtx with extended deadline).
	// P0-7: ProfileCtx derives from labCtx, so parent cancellation still propagates.
	// P0-7: No separate finalCtx needed - ctx already has the correct deadline.
	for _, p := range profiles {
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), errors.New("profile capture cancelled at final"))
		default:
		}
		outPath := filepath.Join(artifactDir, fmt.Sprintf("%s-final.%s", p.name, getExtension(p.name)))
		if err := CaptureProfile(ctx, client, p.url, outPath, p.name); err != nil {
			log.Printf("[PROFILE] Capture failed at final for %s: %v", p.name, err)
			errs = append(errs, fmt.Errorf("%s-final: %w", p.name, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// sleepUntilWithContext sleeps until the specified absolute time or context is cancelled.
// P0-7: Context-aware sleep for cancellation support.
func sleepUntilWithContext(ctx context.Context, deadline time.Time) error {
	now := time.Now()
	if deadline.After(now) {
		waitDuration := deadline.Sub(now)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
			return nil
		}
	}
	return nil
}

// getExtension returns file extension based on profile type.
func getExtension(name string) string {
	switch name {
	case "heap", "allocs":
		return "pb.gz"
	case "goroutine":
		return "txt"
	default:
		return "bin"
	}
}

// writeDeltaSummary writes delta summary to JSON.
func writeDeltaSummary(prefix string, delta *ProcessDelta) error {
	if delta == nil {
		return nil
	}

	delta.Metric = "rss_kib"

	path := filepath.Join(artifactDir, fmt.Sprintf("%s-delta.json", prefix))
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create delta file: %w", err)
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, `{
  "metric": "%s",
  "first_ts": "%s",
  "last_ts": "%s",
  "first_value": %d,
  "last_value": %d,
  "delta": %d,
  "min": %d,
  "max": %d,
  "slope_kib_per_min": %f
}
`, delta.Metric,
		delta.FirstTimestamp.UTC().Format(time.RFC3339),
		delta.LastTimestamp.UTC().Format(time.RFC3339),
		delta.FirstValue, delta.LastValue,
		delta.Delta,
		delta.Min, delta.Max,
		delta.SlopeKibPerMinute)
	if err != nil {
		return fmt.Errorf("write delta: %w", err)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync delta: %w", err)
	}

	log.Printf("[WRITE] %s delta written to %s", prefix, filepath.Base(path))
	return nil
}

// sleepUntil sleeps until the specified absolute time.
func sleepUntil(deadline time.Time) {
	now := time.Now()
	if deadline.After(now) {
		time.Sleep(deadline.Sub(now))
	}
}

// writeProcessSeries writes process samples to a CSV file.
func writeProcessSeries(prefix string, samples []ProcessSample) error {
	if len(samples) == 0 {
		return nil
	}

	path := filepath.Join(artifactDir, fmt.Sprintf("%s-process-series.csv", prefix))
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create series file: %w", err)
	}
	defer f.Close()

	fmt.Fprintf(f, "pid,timestamp,rss_kib,vm_size_kib,pss_kib,pss_anon_kib,private_dirty_kib,anonymous_kib,threads,fd_count,goroutines\n")

	for _, s := range samples {
		fmt.Fprintf(f, "%d,%s,%d,%d,%d,%d,%d,%d,%d,%d,%d\n",
			s.PID,
			s.Timestamp.UTC().Format(time.RFC3339),
			s.RSSKIB, s.VMSizeKIB, s.PSS_KIB, s.PSSAnonKIB,
			s.PrivateDirtyKIB, s.AnonymousKIB,
			s.Threads, s.FDCount, s.GoroutineCount)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync series: %w", err)
	}

	log.Printf("[WRITE] %s series written to %s (%d samples)", prefix, filepath.Base(path), len(samples))
	return nil
}

// finalizeLifecycleFailure finalizes lifecycle failure with complete cleanup.
func finalizeLifecycleFailure(
	result *LabResult,
	initiatingFailure error,
	identity *runExecutionIdentity,
	processes *failedRunProcesses,
) error {
	input := lifecycleFailureInput{
		TovarischPID:      result.TovarischPID,
		UVB76PID:          result.UVB76PID,
		Processes:         processes,
		Identity:          identity,
		InitiatingFailure: initiatingFailure,
		LabResult:         result,
	}

	ops := lifecycleFailureOps{
		Cleanup: func() []error {
			return cleanupOwnedProcesses(
				processes.TovarischCmd,
				processes.UVB76Cmd,
				processes.TovarischPS,
				processes.UVB76PS,
			)
		},
		ProcessGone: func(pid int) (bool, error) {
			return processIsGone(pid), nil
		},
		VerifyPortsReleased: func() error {
			return verifyPortsReleased()
		},
		RemoveStaleResult: func(path string) error {
			return removeResultFile(path)
		},
		PublishFailedResult: func(r *Result) error {
			return persistResult(r, identity.ArtifactDir)
		},
	}

	return finalizeLifecycleFailureWithOps(input, ops)
}
