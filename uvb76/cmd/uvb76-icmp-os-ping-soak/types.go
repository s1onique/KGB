package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/s1onique/KGB/uvb76/internal/artifactio"
)

// Result captures the lab outcome for machine-readable output.
type Result struct {
	OK                   bool     `json:"ok"`
	LabName              string   `json:"lab_name"`
	DurationSeconds      int      `json:"duration_seconds"`
	ICMPEnabled          bool     `json:"icmp_enabled"`
	ICMPIntervalSeconds  int      `json:"icmp_interval_seconds"`
	ICMPTimeoutSeconds   int      `json:"icmp_timeout_seconds"`
	ICMPMaxConcurrent    int      `json:"icmp_max_concurrent"`
	DaemonStarted         bool     `json:"daemon_started"`
	DaemonExitedEarly    bool     `json:"daemon_exited_early"`
	DaemonExitCode       *int     `json:"daemon_exit_code,omitempty"`
	PIDStable            bool     `json:"pid_stable"`
	FatalLogPatternsFound []string `json:"fatal_log_patterns_found"`

	// Daemon-sourced ICMP telemetry
	DaemonICMPAttempts      uint64 `json:"daemon_icmp_attempts"`
	DaemonICMPSuccesses     uint64 `json:"daemon_icmp_successes"`
	DaemonICMPFailures      uint64 `json:"daemon_icmp_failures"`
	DaemonICMPLastError     string `json:"daemon_icmp_last_error,omitempty"`
	DaemonStatusRaw         string `json:"daemon_status_raw,omitempty"`

	// ICMP probe was exercised in the daemon (requires daemon HTTP API)
	ICMPProbeExercised        bool   `json:"icmp_probe_exercised"`
	ICMPProbeExercisedReason  string `json:"icmp_probe_exercised_reason,omitempty"`
	ICMPEvidenceSource        string `json:"icmp_evidence_source,omitempty"`

	// Memory stats
	MemStatsBefore string `json:"memstats_before"`
	MemStatsAfter  string `json:"memstats_after"`

	// Goroutine stats
	GoroutinesBefore int  `json:"goroutines_before"`
	GoroutinesAfter  int  `json:"goroutines_after"`
	GoroutineLeaked  bool `json:"goroutine_leaked"`

	// Health checks
	HealthEndpointValid bool `json:"health_endpoint_valid"`
	StatusEndpointValid bool `json:"status_endpoint_valid"`

	ArtifactDir string `json:"artifact_dir"`
}

// MemStats holds runtime memory statistics for JSON serialization.
type MemStats struct {
	Alloc      uint64 `json:"alloc_bytes"`
	TotalAlloc uint64 `json:"total_alloc_bytes"`
	Sys        uint64 `json:"sys_bytes"`
	NumGC      uint32 `json:"num_gc"`
	GoVersion  string `json:"go_version"`
}

// captureMemStats captures current runtime memory statistics.
func captureMemStats() MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return MemStats{
		Alloc:      m.Alloc,
		TotalAlloc: m.TotalAlloc,
		Sys:        m.Sys,
		NumGC:      m.NumGC,
		GoVersion:  runtime.Version(),
	}
}

// WriteArtifacts writes all result and artifact files to the artifact directory.
func WriteArtifacts(artifactDir string, result Result, memBefore, memAfter MemStats, goroutinesBefore, goroutinesAfter int) error {
	resultBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := artifactio.WriteRedactedJSONBytes("icmp-ping-soak-artifacts", filepath.Join(artifactDir, "result.json"), resultBytes, artifactio.DefaultRuntimePolicy()); err != nil {
		return err
	}

	memstats := map[string]interface{}{
		"before": memBefore,
		"after":  memAfter,
	}
	memstatsBytes, _ := json.MarshalIndent(memstats, "", "  ")
	if err := artifactio.WriteRedactedJSONBytes("icmp-ping-soak-artifacts", filepath.Join(artifactDir, "memstats.json"), memstatsBytes, artifactio.DefaultRuntimePolicy()); err != nil {
		return err
	}

	goroutinesContent := fmt.Sprintf("before: %d\nafter: %d\nleaked: %v\n",
		goroutinesBefore, goroutinesAfter, result.GoroutineLeaked)
	if err := artifactio.WriteRedactedText("icmp-ping-soak-artifacts", filepath.Join(artifactDir, "goroutines.txt"), goroutinesContent, artifactio.DefaultTextPolicy()); err != nil {
		return err
	}

	return nil
}
