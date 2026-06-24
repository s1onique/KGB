// attribution_runner.go — UVB-76 memory attribution lab orchestrator
//
// Coordinates long-running UVB-76 memory attribution labs with checkpoint-based
// evidence capture at start, midpoint, and end phases.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// AttributionRunner orchestrates the attribution lab execution.
type AttributionRunner struct {
	cfg         AttributionConfig
	runCfg      RunConfig
	runner      *Runner
	sampler     *AttributionSampler
	stopChan    chan struct{}
	httpClient  *http.Client
	artifactDir string
	startTime   time.Time
}

// RunAttribution executes the UVB-76 memory attribution lab.
func RunAttribution(runCfg RunConfig, attrCfg AttributionConfig) (string, error) {
	r := &AttributionRunner{
		cfg:         attrCfg,
		runCfg:      runCfg,
		stopChan:    make(chan struct{}),
		artifactDir: attributionArtifactDir(runCfg),
	}
	return r.run()
}

// FIX: Initialize persistent runner and use it for all lifecycle operations.
func (r *AttributionRunner) run() (string, error) {
	// Initialize persistent runner for lifecycle ownership
	r.runner = &Runner{cfg: r.runCfg}

	// Preflight check
	if err := r.preflightServiceCommand(); err != nil {
		return "", fmt.Errorf("preflight: %w", err)
	}

	// Prepare TLS for UVB-76
	if err := r.prepareUVB76TLS(); err != nil {
		return "", fmt.Errorf("prepare TLS: %w", err)
	}
	defer r.cleanupRuntimeDir()

	pid, stdoutPath, err := r.startService()
	if err != nil {
		return "", fmt.Errorf("start service: %w", err)
	}
	defer r.stopService()

	if err := r.waitForReady(pid, stdoutPath); err != nil {
		return "", err
	}

	r.startTime = time.Now()

	// Check PSS availability
	pssAvailable := HasSmapsRollup()

	// Initialize HTTP client for status API
	r.httpClient = r.httpClientForReadiness()

	// Start RSS/PSS sampler
	sampler := NewAttributionSampler(pid, r.cfg.SampleIntervalMs)
	ctx, cancel := context.WithCancel(context.Background())
	sampler.Start(ctx)

	// Create artifact directory
	if err := os.MkdirAll(r.artifactDir, 0755); err != nil {
		cancel()
		return "", err
	}

	// Calculate checkpoint times
	durationSecs := r.cfg.DurationSeconds
	midpointCheckpoint := durationSecs / 2
	endCheckpoint := durationSecs

	fmt.Printf("=== UVB-76 Memory Attribution Lab ===\n")
	fmt.Printf("Duration: %d seconds (%d minutes)\n", durationSecs, durationSecs/60)
	fmt.Printf("Sample interval: %d ms\n", r.cfg.SampleIntervalMs)
	fmt.Printf("Checkpoints: start(0s), midpoint(%ds), end(%ds)\n", midpointCheckpoint, endCheckpoint)
	fmt.Printf("PID: %d\n", pid)
	fmt.Printf("PSS available: %v\n", pssAvailable)
	fmt.Printf("Artifact dir: %s\n", r.artifactDir)

	// Take START checkpoint
	fmt.Printf("\n[T+0s] Taking START checkpoint...\n")
	startCheckpointTime := time.Now()
	if err := r.takeCheckpoint(pid, CheckpointStart, true); err != nil {
		return "", fmt.Errorf("START checkpoint failed: %w", err)
	}
	fmt.Printf("[T+%ds] START checkpoint complete\n", int(time.Since(startCheckpointTime).Seconds()))

	// Wait for midpoint
	fmt.Printf("\nWaiting for midpoint checkpoint at T+%ds...\n", midpointCheckpoint)
	r.sleepWithProgress(midpointCheckpoint)

	// Take MIDPOINT checkpoint
	fmt.Printf("\n[T+%ds] Taking MIDPOINT checkpoint...\n", midpointCheckpoint)
	midpointCheckpointTime := time.Now()
	if err := r.takeCheckpoint(pid, CheckpointMidpoint, true); err != nil {
		return "", fmt.Errorf("MIDPOINT checkpoint failed: %w", err)
	}
	fmt.Printf("[T+%ds] MIDPOINT checkpoint complete\n", int(time.Since(startCheckpointTime).Seconds()))

	// Wait for end
	remaining := endCheckpoint - midpointCheckpoint
	fmt.Printf("\nWaiting for end checkpoint at T+%ds...\n", endCheckpoint)
	r.sleepWithProgress(remaining)

	// Take END checkpoint
	fmt.Printf("\n[T+%ds] Taking END checkpoint...\n", endCheckpoint)
	endCheckpointTime := time.Now()
	if err := r.takeCheckpoint(pid, CheckpointEnd, true); err != nil {
		return "", fmt.Errorf("END checkpoint failed: %w", err)
	}
	fmt.Printf("[T+%ds] END checkpoint complete\n", int(time.Since(startCheckpointTime).Seconds()))

	// Stop sampler
	cancel()
	sampler.Stop()

	// Write manifest
	manifest := &AttributionManifest{
		SchemaVersion:        "1.0",
		RunTimestamp:         r.startTime.UTC().Format(time.RFC3339),
		GitCommit:           getGitCommit(),
		UVB76Version:         getBinaryVersion(r.runCfg.Binary),
		ConfiguredDurationS:   r.cfg.DurationSeconds,
		SampleIntervalMs:     r.cfg.SampleIntervalMs,
		Checkpoints: []CheckpointInfo{
			{Phase: "start", Timestamp: startCheckpointTime.UTC().Format(time.RFC3339)},
			{Phase: "midpoint", Timestamp: midpointCheckpointTime.UTC().Format(time.RFC3339)},
			{Phase: "end", Timestamp: endCheckpointTime.UTC().Format(time.RFC3339)},
		},
		PID:           pid,
		PSSAvailable:  pssAvailable,
		PSSFallbackUsed: !pssAvailable,
		Service:       r.runCfg.Service,
		WorkloadType:  string(r.runCfg.WorkloadType),
	}

	if err := WriteManifest(manifest, r.artifactDir); err != nil {
		return "", fmt.Errorf("write manifest: %w", err)
	}

	// Write RSS/PSS samples
	if err := WriteRSSSamples(sampler.Samples(), r.artifactDir); err != nil {
		return "", fmt.Errorf("write RSS samples: %w", err)
	}

	// Write lab result JSON (summary artifact)
	if err := r.writeLabResult(manifest, sampler.Samples()); err != nil {
		return "", fmt.Errorf("write lab result: %w", err)
	}

	fmt.Printf("\n=== Attribution Lab Complete ===\n")
	fmt.Printf("Artifact dir: %s\n", r.artifactDir)

	return r.artifactDir, nil
}

// takeCheckpoint captures memstats, heap profile, and goroutine dump from UVB-76.
// CRITICAL: This calls UVB-76's diagnostic HTTP endpoints, not memory-lab's own runtime.
func (r *AttributionRunner) takeCheckpoint(pid int, phase CheckpointPhase, forceGC bool) error {
	client := r.httpClientForReadiness()
	baseURL := r.diagnosticBaseURL()

	// Build diagnostic URLs for UVB-76
	memstatsURL := baseURL + "/diagnostics/memstats?force_gc=" + boolStr(forceGC)
	heapURL := baseURL + "/diagnostics/heap-profile"
	goroutineURL := baseURL + "/diagnostics/goroutine-dump"

	// Capture memstats from UVB-76 process (not memory-lab)
	cp, err := CaptureMemStatsFromUVB76(client, memstatsURL, pid, phase, forceGC)
	if err != nil {
		return fmt.Errorf("fetch memstats from UVB-76: %w", err)
	}
	if err := WriteMemStatsCheckpoint(cp, r.artifactDir); err != nil {
		return fmt.Errorf("write memstats: %w", err)
	}

	// Download heap profile from UVB-76
	if err := DownloadHeapProfileFromUVB76(client, heapURL, r.artifactDir, string(phase)); err != nil {
		return fmt.Errorf("download heap profile: %w", err)
	}

	// Download goroutine dump from UVB-76
	if err := DownloadGoroutineDumpFromUVB76(client, goroutineURL, r.artifactDir, string(phase)); err != nil {
		return fmt.Errorf("download goroutine dump: %w", err)
	}

	return nil
}

// diagnosticBaseURL returns the base URL for UVB-76 diagnostic endpoints.
func (r *AttributionRunner) diagnosticBaseURL() string {
	return fmt.Sprintf("https://127.0.0.1:%d/api/v1", r.runCfg.Port)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// sleepWithProgress sleeps for the specified duration, printing progress.
func (r *AttributionRunner) sleepWithProgress(seconds int) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	elapsed := 0
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)

	for {
		select {
		case <-ticker.C:
			elapsed += 30
			remaining := time.Until(deadline).Seconds()
			if remaining > 0 {
				fmt.Printf("  ... %ds elapsed, ~%.0fs remaining\n", elapsed, remaining)
			}
		case <-time.After(time.Until(deadline)):
			return
		}
	}
}

// attributionLabResult represents the lab result JSON structure.
type attributionLabResult struct {
	SchemaVersion string              `json:"schema_version"`
	EvidenceKind  string             `json:"evidence_kind"`
	Service       ServiceInfo        `json:"service"`
	Environment   EnvironmentInfo    `json:"environment"`
	Workload      WorkloadInfo       `json:"workload"`
	Memory        MemoryInfo         `json:"memory"`
	Attribution   AttributionSummary  `json:"attribution"`
	Decision      Decision           `json:"decision"`
}

// AttributionSummary contains attribution-specific metadata.
type AttributionSummary struct {
	CheckpointCount    int    `json:"checkpoint_count"`
	SampleCount        int    `json:"sample_count"`
	PSSAvailable       bool   `json:"pss_available"`
	ForcedGCPerformed  bool   `json:"forced_gc_performed"`
	ManifestPath       string `json:"manifest_path"`
}

// writeLabResult creates a summary JSON artifact for the lab.
func (r *AttributionRunner) writeLabResult(manifest *AttributionManifest, samples []RSSSample) error {
	// Calculate memory summary from samples
	var firstRSS, lastRSS, maxRSS int64
	var firstPSS, lastPSS, maxPSS int64
	if len(samples) > 0 {
		firstRSS, firstPSS = samples[0].RSSKiB, samples[0].PSSKiB
		lastRSS, lastPSS = samples[len(samples)-1].RSSKiB, samples[len(samples)-1].PSSKiB
		maxRSS, maxPSS = firstRSS, firstPSS
		for _, s := range samples {
			if s.RSSKiB > maxRSS {
				maxRSS = s.RSSKiB
			}
			if s.PSSKiB > maxPSS {
				maxPSS = s.PSSKiB
			}
		}
	}

	result := attributionLabResult{
		SchemaVersion: "1.0",
		EvidenceKind:  "real_evidence",
		Service: ServiceInfo{
			Name:    r.runCfg.Service,
			Version: getBinaryVersion(r.runCfg.Binary),
			Commit:  getGitCommit(),
		},
		Environment: EnvironmentInfo{
			Arch:           GetArch(),
			Kernel:         getKernelVersion(),
			OS:             "Linux",
			HasSmapsRollup: manifest.PSSAvailable,
		},
		Workload: WorkloadInfo{
			Type:          string(r.runCfg.WorkloadType),
			Operations:    0,
			Errors:        0,
			DurationMs:    int64(r.cfg.DurationSeconds) * 1000,
			WarmupSeconds: r.runCfg.WarmupSecs,
			Description:   "UVB-76 memory attribution lab - long-running observation",
		},
		Memory: MemoryInfo{
			First:  MemorySnapshot{RSSKiB: firstRSS, PSSKiB: firstPSS},
			Max:    MemorySnapshot{RSSKiB: maxRSS, PSSKiB: maxPSS},
			Last:   MemorySnapshot{RSSKiB: lastRSS, PSSKiB: lastPSS},
			Growth: MemoryGrowth{RSSKiB: lastRSS - firstRSS, PSSKiB: lastPSS - firstPSS},
		},
		Attribution: AttributionSummary{
			CheckpointCount:   len(manifest.Checkpoints),
			SampleCount:       len(samples),
			PSSAvailable:      manifest.PSSAvailable,
			ForcedGCPerformed: true,
			ManifestPath:      "manifest.yaml",
		},
		Decision: Decision{
			Pass:   true,
			Reason: fmt.Sprintf("Attribution lab complete: %d samples over %d seconds", len(samples), r.cfg.DurationSeconds),
		},
	}

	path := filepath.Join(r.artifactDir, "lab-result.json")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

// FIX: Use run-specific subdirectory with timestamp to avoid overwriting evidence
func attributionArtifactDir(runCfg RunConfig) string {
	baseDir := runCfg.ArtifactDir
	if baseDir == "" {
		baseDir = filepath.Join(findRepoRootOrCWD(), "artifacts", "memory-labs", runCfg.Service, "attribution")
	}
	// Create run-specific subdir with timestamp to avoid overwriting previous runs
	timestamp := time.Now().UTC().Format("20060102-150405")
	return filepath.Join(baseDir, "run-"+timestamp)
}

// FIX: Use persistent r.runner for all lifecycle operations.
func (r *AttributionRunner) prepareUVB76TLS() error {
	if r.runCfg.Service != "uvb76" {
		return nil
	}
	return r.runner.prepareUVB76TLS()
}

func (r *AttributionRunner) preflightServiceCommand() error {
	return r.runner.preflightServiceCommand()
}

func (r *AttributionRunner) startService() (int, string, error) {
	return r.runner.startService()
}

func (r *AttributionRunner) waitForReady(pid int, stdoutPath string) error {
	return r.runner.waitForReady(pid, stdoutPath)
}

func (r *AttributionRunner) stopService() {
	r.runner.stopService()
}

func (r *AttributionRunner) httpClientForReadiness() *http.Client {
	return r.runner.httpClientForReadiness()
}

func (r *AttributionRunner) cleanupRuntimeDir() {
	r.runner.cleanupRuntimeDir()
}
