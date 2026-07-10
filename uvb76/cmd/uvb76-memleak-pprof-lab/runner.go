package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-memleak-pprof-lab/internal/fake"
)

// runLab orchestrates the full memory leak lab.
func runLab() LabResult {
	result := LabResult{
		DurationSeconds: int(flagDuration.Seconds()),
		ArtifactDir:     artifactDir,
	}

	// === SETUP PHASE ===
	log.Printf("[SETUP] Ports reserved: uvb76=%s, pprof=%s, tovarisch=%s",
		uvb76Port, pprofPort, tovarischPort)

	// Validate config exists
	if configFile == "" {
		result.Errors = append(result.Errors, "no config file specified")
		return result
	}

	// Remove stale state
	if err := removeStaleArtifacts(artifactDir); err != nil {
		log.Printf("[SETUP] Warning: failed to clean stale artifacts: %v", err)
	}

	// === LAUNCH PHASE ===
	log.Printf("[LAUNCH] Starting UVB-76...")

	uvb76Bin := findUVB76Binary()
	log.Printf("[LAUNCH] Binary: %s", uvb76Bin)

	launchStart := time.Now()
	evidence := StartupEvidence{
		LaunchTimestamp: launchStart.UTC().Format(time.RFC3339Nano),
		ExecutablePath:  uvb76Bin,
		Args:            []string{"-dev", "-config", configFile},
		ConfigPath:      configFile,
		ChosenPorts: PortsChoice{
			UVB76:     uvb76Port,
			PPROF:     pprofPort,
			Tovarisch: tovarischPort,
		},
	}

	// Shared process state for the monitor goroutine
	processState := &ProcessState{}

	cmd, err := startUVB76(uvb76Bin, processState)
	if err != nil {
		log.Printf("[LAUNCH] FAILED: %v", err)
		writeStartupEvidence(evidence)
		writeCrashEvidence(CrashEvidence{
			PID:              0,
			ExitCode:         -1,
			RuntimeMs:        time.Since(launchStart).Milliseconds(),
			PPROFReady:       false,
			CollectorStarted: false,
			State:            StateFailedStartup.String(),
			StderrExcerpt:    err.Error(),
		})
		result.Errors = append(result.Errors, fmt.Sprintf("startup failed: %v", err))
		return result
	}

	evidence.PID = uvb76PID
	evidence.StartupDurationMs = time.Since(launchStart).Milliseconds()
	log.Printf("[LAUNCH] UVB-76 started: PID=%d", uvb76PID)

	// === READINESS PHASE ===
	log.Printf("[READINESS] Waiting for UVB-76 and pprof to be ready...")

	readinessStart := time.Now()
	pprofReady, pprofErr := waitForPPROFReady(pprofPort, 30*time.Second, processState)
	evidence.ReadinessDurationMs = time.Since(readinessStart).Milliseconds()

		if !pprofReady {
		// Check if process exited
		if processState.Exited() {
			exitCode, exitSignal := processState.ExitInfo()
			crashEvidence := buildCrashEvidence(evidence, pprofReady, false, StateFailedReadiness)
			crashEvidence.ExitCode = exitCode
			crashEvidence.ExitSignal = int(exitSignal)
			crashEvidence.RuntimeMs = time.Since(launchStart).Milliseconds()
			writeCrashEvidence(crashEvidence)
			result.Errors = append(result.Errors,
				fmt.Sprintf("uvb76 exited with code %d before pprof became ready: %v",
					exitCode, pprofErr))
			cleanup(cmd, processState)
			return result
		}

		// Timeout waiting for pprof
		crashEvidence := buildCrashEvidence(evidence, pprofReady, false, StateFailedReadiness)
		crashEvidence.RuntimeMs = time.Since(launchStart).Milliseconds()
		writeCrashEvidence(crashEvidence)
		result.Errors = append(result.Errors,
			fmt.Sprintf("pprof never became reachable after %dms: %v",
				evidence.ReadinessDurationMs, pprofErr))
		cleanup(cmd, processState)
		return result
	}

	result.PProfReachable = true
	log.Printf("[READINESS] pprof ready after %dms", evidence.ReadinessDurationMs)

	// Verify pprof endpoints explicitly
	if !verifyEndpoints(pprofPort) {
		result.Errors = append(result.Errors, "pprof endpoint verification failed")
		cleanup(cmd, processState)
		return result
	}

	// === START TOVARISCH (fake or real) ===
	if *flagUseFakeTovarisch {
		log.Printf("[LAUNCH] Starting fake tovarisch status server on port %s...", tovarischPort)
		if err := startFakeTovarisch(); err != nil {
			log.Printf("[LAUNCH] Warning: failed to start fake tovarisch: %v", err)
		} else {
			result.TovarischReachable = waitForHTTPReady(
				"http://localhost:"+tovarischPort+"/status", 5*time.Second)
			log.Printf("[READY] Fake tovarisch reachable: %v", result.TovarischReachable)
		}
	} else {
		// Real tovarisch mode: check if it's reachable
		result.TovarischReachable = waitForHTTPReady(
			"http://localhost:"+tovarischPort+"/status", 5*time.Second)
		log.Printf("[READY] Tovarisch reachable: %v", result.TovarischReachable)
	}

	// Write startup evidence
	writeStartupEvidence(evidence)
	result.UVB76Started = true
	log.Printf("[READY] All systems ready, starting collection...")

	// === COLLECTION PHASE ===
	// Preflight: verify PID still exists
	if !processState.Running() {
		exitCode, exitSignal := processState.ExitInfo()
		crashEvidence := buildCrashEvidence(evidence, true, false, StateFailedCollection)
		crashEvidence.ExitCode = exitCode
		crashEvidence.ExitSignal = int(exitSignal)
		crashEvidence.RuntimeMs = time.Since(launchStart).Milliseconds()
		writeCrashEvidence(crashEvidence)
		result.Errors = append(result.Errors, "target exited before collection")
		cleanup(cmd, processState)
		return result
	}

	log.Printf("[COLLECTION] Running memory lab collector...")
	if err := runCollector(); err != nil {
		// Check if process died during collection
		if processState.Exited() {
			exitCode, exitSignal := processState.ExitInfo()
			crashEvidence := buildCrashEvidence(evidence, true, false, StateFailedCollection)
			crashEvidence.ExitCode = exitCode
			crashEvidence.ExitSignal = int(exitSignal)
			crashEvidence.RuntimeMs = time.Since(launchStart).Milliseconds()
			writeCrashEvidence(crashEvidence)
		}
		result.Errors = append(result.Errors, fmt.Sprintf("collector failed: %v", err))
		cleanup(cmd, processState)
		return result
	}
	result.CollectorSucceeded = true
	log.Printf("[COLLECTED] Collection complete")

	// === PPROF DIFF PHASE ===
	if !*flagSkipPprofDiff {
		log.Printf("[COLLECTED] Running pprof diff reports...")
		if err := runPprofDiff(); err != nil {
			log.Printf("[COLLECTED] Warning: pprof diff failed: %v", err)
		} else {
			result.PProfDiffSucceeded = true
		}
	}

	// === VERIFICATION PHASE ===
	manifestPath := filepath.Join(artifactDir, "manifest.json")
	if data, err := os.ReadFile(manifestPath); err == nil {
		var m struct {
			SchemaVersion int `json:"schema_version"`
		}
		if json.Unmarshal(data, &m) == nil && m.SchemaVersion == 1 {
			result.ManifestValid = true
		}
	}

	verdictPath := filepath.Join(artifactDir, "verdict.json")
	if _, err := os.Stat(verdictPath); err == nil {
		result.VerdictValid = true
	}

	// === SHUTDOWN PHASE ===
	log.Printf("[SHUTDOWN] Initiating graceful shutdown...")
	shutdownErr := gracefulShutdown(cmd, processState, 10*time.Second)
	if shutdownErr != nil {
		log.Printf("[SHUTDOWN] Graceful shutdown failed, forcing: %v", shutdownErr)
		forceKill(cmd, processState)
	}

	// Gather final logs
	gatherFinalLogs()

	// Overall OK
	result.OK =
		result.UVB76Started &&
		result.PProfReachable &&
		result.TovarischReachable &&
		result.CollectorSucceeded &&
		(result.PProfDiffSucceeded || *flagSkipPprofDiff) &&
		result.ManifestValid &&
		result.VerdictValid

	return result
}

// startFakeTovarisch starts a minimal fake tovarisch status server.
func startFakeTovarisch() error {
	fakeServer = &fake.StatusServer{
		Port:    tovarischPort,
		LogFile: tovarischLogFile,
	}

	if err := fakeServer.Start(); err != nil {
		return err
	}

	go func() {
		fakeServer.Wait()
		close(tovarischDone)
	}()

	return nil
}
