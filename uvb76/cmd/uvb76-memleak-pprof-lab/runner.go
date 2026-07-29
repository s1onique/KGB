package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// runLab orchestrates the full memory leak lab with real binaries.
// P0-1 through P0-8: All authorities wired into production path.
// P0-2: Creates runExecutionIdentity ONCE before any process startup.
func runLab() LabResult {
	// === PHASE 0: Pre-start embedded source identity validation ===
	// P0-1: Resolve embedded source identity and RETAIN it for authoritative use.
	// P0-2: Validate embedded source identity BEFORE any process startup side effects.
	// This ensures the controller binary itself is a clean, reproducible build.
	embeddedIdentity, sourceErr := ProductionSourceIdentityResolver.Resolve()
	if sourceErr != nil {
		log.Printf("[SOURCE] Embedded source identity resolution failed: %v", sourceErr)
		return LabResult{
			OK:             false,
			Classification: "FAILED",
			Errors:         []string{fmt.Sprintf("embedded source identity: %v", sourceErr)},
		}
	}

	// P0-2: Create run identity BEFORE any process startup - shared by success and failure
	// P0-1: Use embedded vcs.revision as canonical SourceCommit authority
	// P0-5: Populate binary paths - always populate, validation is mode-aware
	identity := &runExecutionIdentity{
		RunID:            fmt.Sprintf("run-%d", time.Now().Unix()),
		SourceCommit:     embeddedIdentity.VCSRevision, // P0-1: Use embedded vcs.revision as canonical authority
		RunStartedAt:     time.Now(),
		ArtifactDir:      artifactDir,
		TovarischBinPath: *flagTovarischBin, // P0-5: Always populated (fake path allowed in fake mode)
		UVB76BinPath:     *flagUVB76Bin,     // P0-5: Always required (UVB76 never faked)
		TovarischPort:    *flagTovarischPort,
		UVB76Port:        *flagUVB76Port,
		PProfPort:        *flagPProfPort,
	}

	// P0-5: Validate complete identity BEFORE starting any processes
	// P0-5: Binary path validation is mode-aware (fake path allowed for fake mode)
	identityValidationErrors := validateRunExecutionIdentity(identity, *flagUseFakeTovarisch)
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

	log.Printf("[SETUP] Ports: Tovarisch=%s, UVB-76=%s, pprof=%s",
		*flagTovarischPort, *flagUVB76Port, *flagPProfPort)
	log.Printf("[IDENTITY] RunID=%s, Commit=%s, TovarischBin=%s, UVB76Bin=%s",
		identity.RunID, identity.SourceCommit, identity.TovarischBinPath, identity.UVB76BinPath)

	// === PHASE 1: Start Tovarisch (or fake server) ===
	if *flagUseFakeTovarisch {
		log.Printf("[SETUP] Starting fake tovarisch on port %s", *flagTovarischPort)
		if err := startFakeTovarisch(); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("start fake tovarisch: %v", err))
			result.Classification = "FAILED"
			return result
		}
		result.RealTovarischStarted = true                  // Fake counts as started for fake mode
		result.TovarischBinPath = identity.TovarischBinPath // P0-5: Use canonical identity path
	} else {
		// Start real Tovarisch
		log.Printf("[SETUP] Starting real Tovarisch: %s", *flagTovarischBin)

		var err error
		tovarischCmd, tovarischPS, err = startTovarisch(*flagTovarischBin, *flagTovarischArgs, *flagTovarischPort)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("start tovarisch: %v", err))
			result.Classification = "FAILED"
			return result
		}

		// P0-3: Use canonical ProcessState.StartTime as single authority
		result.TovarischStartTime = &tovarischPS.StartTime
		result.RealTovarischStarted = true
		result.TovarischPID = tovarischPID
		result.TovarischBinPath = *flagTovarischBin
		// P0-3: Copy argv from ProcessState for process identity (cloned in process.go)
		if tovarischPS != nil && tovarischPS.Argv != nil {
			result.TovarischArgv = tovarischPS.Argv
		}
	}

	// === PHASE 2: Wait for Tovarisch readiness ===
	tovarischReady := waitForTovarischReady(*flagTovarischPort, 15*time.Second)

	now := time.Now()
	if tovarischReady {
		result.RealTovarischReady = true
		result.TovarischReadyTime = &now
		log.Printf("[READY] Tovarisch ready on port %s", *flagTovarischPort)
	} else {
		result.Errors = append(result.Errors, "tovarisch did not become ready")
		result.Classification = "FAILED"
		cleanup(tovarischCmd, uvb76Cmd, tovarischPS, uvb76PS)
		return result
	}

	// === PHASE 3: Start UVB-76 ===
	log.Printf("[LAUNCH] Starting UVB-76: %s", *flagUVB76Bin)

	var err error
	uvb76Cmd, uvb76PS, err = startUVB76(*flagUVB76Bin)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("start uvb76: %v", err))
		result.Classification = "FAILED"

		// P0-5: Route UVB-76 startup failure through canonical finalizer
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

	// P0-3: Use canonical ProcessState.StartTime as single authority
	result.UVB76StartTime = &uvb76PS.StartTime
	result.RealUVB76Started = true
	result.UVB76PID = uvb76PID
	result.UVB76BinPath = *flagUVB76Bin
	// P0-3: Copy argv from ProcessState for process identity (cloned in process.go)
	if uvb76PS != nil && uvb76PS.Argv != nil {
		result.UVB76Argv = uvb76PS.Argv
	}

	// === PHASE 4: Wait for UVB-76 pprof readiness ===
	pprofReady, pprofErr := waitForPPROFReady(*flagPProfPort, 30*time.Second, uvb76PS)
	if pprofReady {
		result.UVB76PProfReady = true
		now := time.Now()
		result.UVB76PProfReadyTime = &now
		log.Printf("[READY] UVB-76 pprof ready on port %s", *flagPProfPort)
	} else {
		if uvb76PS.Exited() {
			exitCode, _ := uvb76PS.ExitInfo()
			result.Errors = append(result.Errors, fmt.Sprintf("uvb76 exited with code %d before pprof ready: %v", exitCode, pprofErr))
		} else {
			result.Errors = append(result.Errors, fmt.Sprintf("pprof never ready: %v", pprofErr))
		}
		result.Classification = "FAILED"
		cleanup(tovarischCmd, uvb76Cmd, tovarischPS, uvb76PS)
		return result
	}

	// Verify pprof endpoints
	if !verifyPPROFEndpoints(*flagPProfPort) {
		result.Errors = append(result.Errors, "pprof endpoint verification failed")
		result.Classification = "FAILED"
		cleanup(tovarischCmd, uvb76Cmd, tovarischPS, uvb76PS)
		return result
	}

	// === PHASE 5: Collection phase ===
	log.Printf("[COLLECTION] Starting collection phase...")
	collectionStart := time.Now()
	result.CollectionStartTime = &collectionStart

	// Create bounded collection context for collector goroutines
	collectionCtx, collectionCancel := context.WithTimeout(labCtx, *flagDuration)
	defer collectionCancel()

	// Start goroutines for collection
	var wg sync.WaitGroup
	var samplesMu sync.Mutex
	var tovarischSamples []ProcessSample
	var uvb76Samples []ProcessSample
	var collectorErrors []string // P0-4: Track collector errors for LabResult

	// P0-4: Collect process samples using strict authority
	wg.Add(1)
	go func() {
		defer wg.Done()
		collectProcessSamplesStrict(collectionCtx, tovarischPID, *flagSampleInterval, &tovarischSamples, &samplesMu, &collectorErrors)
	}()

	// P0-4: Collect UVB-76 samples with goroutine count
	wg.Add(1)
	go func() {
		defer wg.Done()
		collectUVB76SamplesStrict(collectionCtx, uvb76PID, *flagPProfPort, *flagSampleInterval, &uvb76Samples, &samplesMu, &collectorErrors)
	}()

	// P0-1/P0-2: Poll for target authority CONCURRENTLY with profile capture
	// Target polling must not block the profile schedule
	var targetAuthority *TargetStateAuthority
	var targetIdentityErr error // P0-2: Track identity validation errors
	targetCtx, targetCancel := context.WithCancel(collectionCtx)
	defer targetCancel()

	// P0-1: Build expected URL from tovarisch port for identity binding
	expectedTargetURL := fmt.Sprintf("http://localhost:%s", *flagTovarischPort)

	targetWg := sync.WaitGroup{}
	targetWg.Add(1)
	go func() {
		defer targetWg.Done()
		targetAuthority = pollTargetAuthorityWithCompletion(targetCtx, *flagUVB76Port, "real-tovarisch", 5*time.Second)
	}()

	// P0-6: Capture profiles using validated CaptureProfile (runs in parallel)
	httpClient := &http.Client{Timeout: 30 * time.Second}
	captureErr := captureProfilesWithValidation(httpClient, *flagPProfPort, artifactDir, collectionStart, *flagDuration, *flagProfileInterval)

	// Cancel target polling and wait for result
	targetCancel()
	targetWg.Wait()

	// P0-1: Use extracted CollectAndSnapshot seam for lifecycle authority
	snapshot, err := CollectAndSnapshot(
		collectionCancel,
		&wg,
		&CollectorInput{
			TovarischSamples: &tovarischSamples,
			UVB76Samples:     &uvb76Samples,
			CollectorErrors:  &collectorErrors,
			SamplesMu:        &samplesMu,
		},
	)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("collector lifecycle failed: %v", err))
		result.Classification = "FAILED"
		result.OK = false

		// P0-2: Use the PRE-CREATED identity - not a new one
		// P0-4: Finalize lifecycle failure - returns joined error compatible with errors.Is
		// P0-8: Pass initiating error and real process handles
		processes := &failedRunProcesses{
			TovarischCmd: tovarischCmd,
			UVB76Cmd:     uvb76Cmd,
			TovarischPS:  tovarischPS,
			UVB76PS:      uvb76PS,
		}
		finalizationErr := finalizeLifecycleFailure(&result, err, identity, processes)
		if finalizationErr != nil {
			// P0-4: Terminal error satisfies errors.Is for all causes
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

	// === PHASE 6: Verify cross-component interaction ===
	// P0-1/P0-2: Use authoritative target state
	if targetAuthority != nil {
		result.RealTargetObserved = true
		result.ScrapeAttempted = targetAuthority.IsScrapeAttempted()
		result.ScrapeCompleted = targetAuthority.IsScrapeCompleted()

		// P0-2: Validate target identity binding - fail closed if URL mismatch
		targetIdentityErr = ValidateTargetIdentity(targetAuthority, "real-tovarisch", expectedTargetURL)
		if targetIdentityErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("target identity validation failed: %v", targetIdentityErr))
			result.Classification = "FAILED"
			result.OK = false
		}
	}

	// Check if we have process samples (P0-4)
	result.ProcessSamplesPresent = len(tovarischFinal) > 0 && len(uvb76Final) > 0

	// P0-6: Check profiles using ValidateAllProfiles
	profileErr := ValidateAllProfiles(artifactDir)
	result.ProfilesPresent = profileErr == nil
	if profileErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("profile validation failed: %v", profileErr))
	}

	// P0-6: Propagate profile capture errors to LabResult
	if captureErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("profile capture failed: %v", captureErr))
	}

	// P0-8: Compute deltas for both series
	tovarischDelta := ComputeProcessDeltas(tovarischFinal)
	uvb76Delta := ComputeProcessDeltas(uvb76Final)

	// P0-7: Write series files - track write errors
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

	// P0-4: Propagate collector errors to LabResult
	result.Errors = append(result.Errors, collectorErrorsFinal...)

	// === PHASE 7: Cleanup ===
	cleanupErrors := cleanup(tovarischCmd, uvb76Cmd, tovarischPS, uvb76PS)
	result.Errors = append(result.Errors, cleanupErrors...)

	// Verify cleanup
	result.UVB76Removed = uvb76PID == 0 || processIsGone(uvb76PID)
	result.TovarischRemoved = tovarischPID == 0 || processIsGone(tovarischPID)

	// P0-7: Independently verify port release - not just absence of cleanup errors
	portErr := verifyPortsReleased()
	if portErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("port release verification failed: %v", portErr))
	}
	result.PortsReleased = portErr == nil

	// === PHASE 8: Final classification using strict authority (P0-3) ===
	result.Classification, result.OK = classifyLabResult(result)

	// === PHASE 9: Persist result.json (P0-7) ===
	// P0-2: Use the pre-created identity for success path
	// P0-7: BuildResultFromLabResult now returns error for validation failures
	builtResult, buildErr := BuildResultFromLabResult(identity, result)
	if buildErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("result construction: %v", buildErr))
		result.Classification = "FAILED"
		result.OK = false
		return result
	}
	if targetAuthority != nil {
		builtResult.ScrapeAuthority = &ScrapeAuthorityObservation{
			TargetID:            targetAuthority.TargetID,
			AttemptObserved:     targetAuthority.AttemptObserved,
			AttemptTimestamp:    &targetAuthority.AttemptTimestamp,
			CompletionObserved:  targetAuthority.CompletionObserved,
			CompletionTimestamp: &targetAuthority.CompletionTimestamp,
			Reachable:           targetAuthority.Reachable,
			Status:              targetAuthority.Status,
			Error:               targetAuthority.Error,
		}
		builtResult.TovarischSeriesFile = "tovarisch-process-series.csv"
		builtResult.UVB76SeriesFile = "uvb76-process-series.csv"
		builtResult.TovarischSampleCount = len(tovarischFinal)
		builtResult.UVB76SampleCount = len(uvb76Final)
	}

	// P0-7: Persist result with strict reread validation
	if persistErr := persistResult(builtResult, artifactDir); persistErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("result persistence failed: %v", persistErr))
		result.Classification = "FAILED"
		result.OK = false
	}

	return result
}

// SamplingError represents a structured sampling failure.
type SamplingError struct {
	Process  string // "tovarisch" or "uvb76"
	Phase    string // "sampling", "goroutine", "disappearance"
	PID      int
	InnerErr error
}

func (e *SamplingError) Error() string {
	return fmt.Sprintf("%s %s error (PID %d): %v", e.Process, e.Phase, e.PID, e.InnerErr)
}

// collectProcessSamplesStrict collects process metrics using strict authority.
// P0-4: Uses sampleProcessMetricsFull which returns errors; propagates errors to LabResult.
// P0-4: Treats premature process disappearance as a collector failure.
func collectProcessSamplesStrict(ctx context.Context, pid int, interval time.Duration, samples *[]ProcessSample, mu *sync.Mutex, errors *[]string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	firstSampleSeen := false
	for {
		select {
		case <-ticker.C:
			// P0-4: Check if process disappeared prematurely (after we started collecting)
			if processIsGone(pid) {
				if firstSampleSeen {
					// Process disappeared prematurely - this is a failure condition
					mu.Lock()
					*errors = append(*errors, (&SamplingError{
						Process:  "tovarisch",
						Phase:    "disappearance",
						PID:      pid,
						InnerErr: fmt.Errorf("process exited before collection completed"),
					}).Error())
					mu.Unlock()
				}
				return
			}
			firstSampleSeen = true

			// P0-4: Use strict sampler that returns errors
			sample, err := sampleProcessMetricsFull(pid)
			if err != nil {
				log.Printf("[METRICS] Tovarisch sampling error: %v", err)
				// P0-4: Propagate error to LabResult instead of silently continuing
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
			mu.Lock()
			*samples = append(*samples, *sample)
			mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

// collectUVB76SamplesStrict collects UVB-76 process metrics and goroutine count.
// P0-4: Uses strict sampler for process metrics; propagates errors to LabResult.
// P0-4: Treats premature process disappearance as a collector failure.
func collectUVB76SamplesStrict(ctx context.Context, pid int, pprofPort string, interval time.Duration, samples *[]ProcessSample, mu *sync.Mutex, errors *[]string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	client := &http.Client{Timeout: 2 * time.Second}
	firstSampleSeen := false

	for {
		select {
		case <-ticker.C:
			// P0-4: Check if process disappeared prematurely
			if processIsGone(pid) {
				if firstSampleSeen {
					// Process disappeared prematurely - this is a failure condition
					mu.Lock()
					*errors = append(*errors, (&SamplingError{
						Process:  "uvb76",
						Phase:    "disappearance",
						PID:      pid,
						InnerErr: fmt.Errorf("process exited before collection completed"),
					}).Error())
					mu.Unlock()
				}
				return
			}
			firstSampleSeen = true

			// P0-4: Use strict sampler
			sample, err := sampleProcessMetricsFull(pid)
			if err != nil {
				log.Printf("[METRICS] UVB-76 sampling error: %v", err)
				// P0-4: Propagate error to LabResult instead of silently continuing
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
			// P0-5: Only GoroutineCount has real authority
			if gc := fetchGoroutineCount(client, pprofPort); gc > 0 {
				sample.GoroutineCount = gc
			}
			mu.Lock()
			*samples = append(*samples, *sample)
			mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

// pollTargetAuthorityWithCompletion polls UVB-76 for authoritative target state.
// Retains observations until either a completed scrape is observed or context ends.
// P0-1/P0-2: Retains authority observation until completion.
func pollTargetAuthorityWithCompletion(ctx context.Context, uvb76Port, targetID string, interval time.Duration) *TargetStateAuthority {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sessionCookie := os.Getenv("UVB76_SESSION_COOKIE")
	var bestAuthority *TargetStateAuthority

	for {
		select {
		case <-ticker.C:
			auth, err := FetchTargetState(uvb76Port, targetID, sessionCookie)
			if err != nil {
				log.Printf("[TARGET] FetchTargetState error: %v", err)
				continue
			}
			if auth != nil {
				// Keep updating until we see completion or context ends
				bestAuthority = auth
				if auth.IsScrapeCompleted() {
					// Return immediately on completion
					return auth
				}
			}
		case <-ctx.Done():
			// Return the best authority we observed (even if incomplete)
			return bestAuthority
		}
	}
}

// fetchGoroutineCount fetches goroutine count from pprof endpoint.
// P0-5: Only GoroutineCount has real authority.
func fetchGoroutineCount(client *http.Client, pprofPort string) int64 {
	resp, err := client.Get(fmt.Sprintf("http://localhost:%s/debug/pprof/goroutine?debug=1", pprofPort))
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	lines := strings.Split(string(body), "\n")
	var count int64
	for _, line := range lines {
		if strings.HasPrefix(line, "goroutine ") && strings.HasSuffix(line, ":") {
			count++
		}
	}
	return count
}

// captureProfilesWithValidation captures and validates all required profiles.
// P0-6: Uses CaptureProfile with full validation. Accumulates all errors.
func captureProfilesWithValidation(client *http.Client, pprofPort, artifactDir string, start time.Time, duration, interval time.Duration) error {
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
		outPath := filepath.Join(artifactDir, fmt.Sprintf("%s-start.%s", p.name, getExtension(p.name)))
		if err := CaptureProfile(client, p.url, outPath, p.name); err != nil {
			log.Printf("[PROFILE] Capture failed at start for %s: %v", p.name, err)
			errs = append(errs, fmt.Errorf("%s-start: %w", p.name, err))
		}
	}

	// Capture at midpoint
	midTime := start.Add(interval)
	if midTime.Before(start.Add(duration)) {
		sleepUntil(midTime)
		for _, p := range profiles {
			outPath := filepath.Join(artifactDir, fmt.Sprintf("%s-mid.%s", p.name, getExtension(p.name)))
			if err := CaptureProfile(client, p.url, outPath, p.name); err != nil {
				log.Printf("[PROFILE] Capture failed at mid for %s: %v", p.name, err)
				errs = append(errs, fmt.Errorf("%s-mid: %w", p.name, err))
			}
		}
	}

	// Capture at end
	sleepUntil(start.Add(duration))
	for _, p := range profiles {
		outPath := filepath.Join(artifactDir, fmt.Sprintf("%s-final.%s", p.name, getExtension(p.name)))
		if err := CaptureProfile(client, p.url, outPath, p.name); err != nil {
			log.Printf("[PROFILE] Capture failed at final for %s: %v", p.name, err)
			errs = append(errs, fmt.Errorf("%s-final: %w", p.name, err))
		}
	}

	// Return all accumulated errors joined
	if len(errs) > 0 {
		return errors.Join(errs...)
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

// writeDeltaSummary writes delta summary to JSON and returns any error.
func writeDeltaSummary(prefix string, delta *ProcessDelta) error {
	if delta == nil {
		return nil
	}

	// P0-8: Set metric name
	delta.Metric = "rss_kib"

	path := filepath.Join(artifactDir, fmt.Sprintf("%s-delta.json", prefix))
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create delta file: %w", err)
	}
	defer f.Close()

	// Format with UTC timestamps
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

// writeProcessSeries writes process samples to a CSV file and returns any error.
// P0-4: Writes truthy process metrics from /proc including PID.
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

	// CSV header including PID
	fmt.Fprintf(f, "pid,timestamp,rss_kib,vm_size_kib,pss_kib,pss_anon_kib,private_dirty_kib,anonymous_kib,threads,fd_count,goroutines\n")

	// CSV rows
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

// finalizeLifecycleFailure finalizes lifecycle failure with complete cleanup and verification.
// P0-4: Returns single joined error compatible with errors.Is for all causes.
// P0-6: Uses only supplied PIDs, never package globals.
// P0-7: Passes initiating error and real process handles.
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
			// P0-1: Use typed cleanup authority that returns []error directly
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
