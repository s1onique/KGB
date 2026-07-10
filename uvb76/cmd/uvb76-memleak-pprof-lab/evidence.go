package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// StartupEvidence captures launch-time metadata for reproducibility.
type StartupEvidence struct {
	LaunchTimestamp     string       `json:"launch_timestamp"`
	PID                int          `json:"pid"`
	ExecutablePath     string       `json:"executable_path"`
	Args               []string     `json:"args"`
	ConfigPath         string       `json:"config_path"`
	ChosenPorts        PortsChoice  `json:"chosen_ports"`
	StartupDurationMs  int64        `json:"startup_duration_ms"`
	ReadinessDurationMs int64       `json:"readiness_duration_ms"`
}

// PortsChoice captures which ports were selected.
type PortsChoice struct {
	UVB76     string `json:"uvb76"`
	PPROF     string `json:"pprof"`
	Tovarisch string `json:"tovarisch"`
}

// CrashEvidence captures what happened if the target exited before collection.
type CrashEvidence struct {
	PID              int    `json:"pid"`
	ExitCode         int    `json:"exit_code"`
	ExitSignal       int    `json:"exit_signal"`
	RuntimeMs        int64  `json:"runtime_ms"`
	PPROFReady       bool   `json:"pprof_ready"`
	CollectorStarted bool   `json:"collector_started"`
	State            string `json:"state_at_crash"`
	StderrExcerpt    string `json:"stderr_excerpt,omitempty"`
}

// buildCrashEvidence constructs crash evidence from current state.
func buildCrashEvidence(evidence StartupEvidence, pprofReady, collectorStarted bool,
	state LifecycleState) CrashEvidence {

	// Read stderr excerpt from log file
	stderrExcerpt := ""
	if logData, err := os.ReadFile(uvb76LogFile); err == nil {
		// Get last 500 chars
		if len(logData) > 500 {
			stderrExcerpt = string(logData[len(logData)-500:])
		} else {
			stderrExcerpt = string(logData)
		}
	}

	return CrashEvidence{
		PID:              evidence.PID,
		ExitCode:         -1,
		ExitSignal:       0,
		RuntimeMs:        0,
		PPROFReady:       pprofReady,
		CollectorStarted: collectorStarted,
		State:            state.String(),
		StderrExcerpt:    stderrExcerpt,
	}
}

// writeStartupEvidence writes launch metadata to artifact dir.
func writeStartupEvidence(evidence StartupEvidence) {
	path := filepath.Join(artifactDir, "startup_evidence.json")
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		log.Printf("[ERROR] Failed to marshal startup evidence: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("[ERROR] Failed to write startup evidence: %v", err)
	}
}

// writeCrashEvidence writes crash metadata when target exits unexpectedly.
func writeCrashEvidence(crash CrashEvidence) {
	path := filepath.Join(artifactDir, "exit.json")
	data, err := json.MarshalIndent(crash, "", "  ")
	if err != nil {
		log.Printf("[ERROR] Failed to marshal crash evidence: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("[ERROR] Failed to write crash evidence: %v", err)
	}
	log.Printf("[CRASH] Evidence written to exit.json")
}

// removeStaleArtifacts cleans up old state files.
func removeStaleArtifacts(dir string) error {
	staleFiles := []string{"exit.json", "startup_evidence.json"}
	for _, f := range staleFiles {
		path := filepath.Join(dir, f)
		os.Remove(path)
	}
	return nil
}

// gatherFinalLogs copies final process output to artifacts.
func gatherFinalLogs() {
	// The logs are already being written to uvb76.log and tovarisch.log
	// This function is for any additional final log gathering if needed
	log.Printf("[SHUTDOWN] Final logs available at: %s", uvb76LogFile)
}
