// runner.go — Memory lab runner with concurrent sampling
//
// Coordinates service startup, memory sampling, workload execution,
// and artifact generation. True max RSS/PSS is sampled concurrently
// during workload execution.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// RunConfig configures the memory lab run.
type RunConfig struct {
	Service      string // "tovarisch" or "uvb76"
	WorkloadType WorkloadType
	Binary       string
	ConfigPath   string // Only used for uvb76
	Port         int
	WarmupSecs   int
	Operations   int
	IntervalMs   int
	ArtifactDir  string
}

// Runner orchestrates the memory lab execution.
type Runner struct {
	cfg      RunConfig
	sampler  *MemorySampler
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// MemorySampler concurrently samples memory during workload execution.
type MemorySampler struct {
	pid       int
	samples   []MemorySnapshot
	mu        sync.Mutex
	wg        sync.WaitGroup
	stopChan  chan struct{}
	sampleInt time.Duration
}

// NewMemorySampler creates a new memory sampler for the given PID.
func NewMemorySampler(pid int, intervalMs int) *MemorySampler {
	return &MemorySampler{
		pid:       pid,
		samples:   make([]MemorySnapshot, 0, 100),
		stopChan:  make(chan struct{}),
		sampleInt: time.Duration(intervalMs) * time.Millisecond,
	}
}

// Start begins concurrent memory sampling.
func (s *MemorySampler) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.sampleInt)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopChan:
				return
			case <-ticker.C:
				if snap, err := ReadMemorySnapshot(s.pid); err == nil {
					s.mu.Lock()
					s.samples = append(s.samples, snap)
					s.mu.Unlock()
				}
			}
		}
	}()
}

// Stop halts sampling and waits for completion.
func (s *MemorySampler) Stop() {
	close(s.stopChan)
	s.wg.Wait()
}

// Max returns the maximum RSS and PSS observed.
func (s *MemorySampler) Max() (maxRSS, maxPSS int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, snap := range s.samples {
		if snap.RSSKiB > maxRSS {
			maxRSS = snap.RSSKiB
		}
		if snap.PSSKiB > maxPSS {
			maxPSS = snap.PSSKiB
		}
	}
	return maxRSS, maxPSS
}

// Run executes the memory lab for the configured service and workload.
func Run(cfg RunConfig) (string, error) {
	r := &Runner{
		cfg:      cfg,
		stopChan: make(chan struct{}),
	}
	return r.run()
}

func (r *Runner) run() (string, error) {
	// Start the service
	pid, _, err := r.startService()
	if err != nil {
		return "", fmt.Errorf("start service: %w", err)
	}
	defer r.stopService(pid)

	// Wait briefly for service to be ready
	time.Sleep(2 * time.Second)

	// Take first snapshot immediately after readiness
	firstSnap, err := ReadMemorySnapshot(pid)
	if err != nil {
		return "", fmt.Errorf("first snapshot: %w", err)
	}
	fmt.Printf("Initial RSS: %d KiB, PSS: %d KiB\n", firstSnap.RSSKiB, firstSnap.PSSKiB)

	// Create memory sampler and start concurrent sampling DURING warmup
	sampler := NewMemorySampler(pid, 2000) // Sample every 2s
	ctx, cancel := context.WithCancel(context.Background())
	sampler.Start(ctx)

	// Wait for warmup
	fmt.Printf("Warming up for %ds...\n", r.cfg.WarmupSecs)
	time.Sleep(time.Duration(r.cfg.WarmupSecs) * time.Second)

	// Execute workload (for non-idle workloads)
	workloadResult := r.executeWorkload(pid)
	fmt.Printf("Workload: %d ops, %d errors, %dms\n",
		workloadResult.Operations, workloadResult.Errors, workloadResult.DurationMs)

	// Stop sampling
	cancel()
	sampler.Stop()

	// Take last snapshot
	lastSnap, err := ReadMemorySnapshot(pid)
	if err != nil {
		return "", fmt.Errorf("last snapshot: %w", err)
	}
	fmt.Printf("Final RSS: %d KiB, PSS: %d KiB\n", lastSnap.RSSKiB, lastSnap.PSSKiB)

	// Calculate max from: first, sampled, last
	sampledMaxRSS, sampledMaxPSS := sampler.Max()
	maxRSS := maxOf3(firstSnap.RSSKiB, sampledMaxRSS, lastSnap.RSSKiB)
	maxPSS := maxOf3(firstSnap.PSSKiB, sampledMaxPSS, lastSnap.PSSKiB)
	fmt.Printf("Max RSS: %d KiB, Max PSS: %d KiB\n", maxRSS, maxPSS)

	// Build artifact
	artifact, err := r.buildArtifact(firstSnap, lastSnap, maxRSS, maxPSS, workloadResult)
	if err != nil {
		return "", fmt.Errorf("build artifact: %w", err)
	}

	// Write artifact
	path, err := artifact.Write(r.artifactDir(), r.cfg.Service, string(r.cfg.WorkloadType))
	if err != nil {
		return "", fmt.Errorf("write artifact: %w", err)
	}

	fmt.Printf("Artifact written to: %s\n", path)
	return path, nil
}

func (r *Runner) artifactDir() string {
	if r.cfg.ArtifactDir != "" {
		return r.cfg.ArtifactDir
	}
	return filepath.Join(findRepoRootOrCWD(), "artifacts", "memory-labs", r.cfg.Service)
}

// buildServiceCommand constructs the exec.Command for starting a service.
// This is a pure function for testability.
func (r *Runner) buildServiceCommand() *exec.Cmd {
	if r.cfg.Service == "tovarisch" {
		return exec.Command(r.cfg.Binary, "serve", fmt.Sprintf("--listen=127.0.0.1:%d", r.cfg.Port))
	}
	configPath := r.cfg.ConfigPath
	if configPath == "" {
		configPath = "./uvb76/uvb76.example.json"
	}
	return exec.Command(r.cfg.Binary, "-config="+configPath)
}

func (r *Runner) startService() (int, string, error) {
	// Resolve artifact dir once and use for both stdout and artifacts
	artifactDir := r.artifactDir()
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return 0, "", err
	}

	stdoutPath := filepath.Join(artifactDir, "stdout.log")
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return 0, "", err
	}
	defer stdoutFile.Close()

	cmd := r.buildServiceCommand()
	cmd.Stdout = stdoutFile
	cmd.Stderr = stdoutFile

	if err := cmd.Start(); err != nil {
		return 0, "", err
	}

	// Wait for process to start
	time.Sleep(2 * time.Second)

	// Verify process is running
	if cmd.Process == nil {
		return 0, "", fmt.Errorf("process not started")
	}

	return cmd.Process.Pid, stdoutPath, nil
}

func (r *Runner) stopService(pid int) {
	proc, _ := os.FindProcess(pid)
	if proc != nil {
		proc.Kill()
		proc.Wait()
	}
}

func (r *Runner) executeWorkload(pid int) HTTPWorkloadResult {
	// For idle-warmup, no HTTP workload
	if r.cfg.WorkloadType == WorkloadTovarischIdle || r.cfg.WorkloadType == WorkloadUVB76Idle {
		return HTTPWorkloadResult{
			Operations: 0,
			Errors:     0,
			DurationMs: int64(r.cfg.WarmupSecs * 1000),
		}
	}

	// Build URL
	var url string
	if r.cfg.Service == "tovarisch" {
		urls := TovarischWorkloadURLs(r.cfg.Port)
		url = urls[r.cfg.WorkloadType]
	} else {
		urls := UVB76WorkloadURLs(r.cfg.Port)
		url = urls[r.cfg.WorkloadType]
	}

	return RunHTTPWorkload(HTTPWorkloadConfig{
		URL:        url,
		Operations: r.cfg.Operations,
		IntervalMs: r.cfg.IntervalMs,
		Name:       string(r.cfg.WorkloadType),
	})
}

func (r *Runner) buildArtifact(first, last MemorySnapshot, maxRSS, maxPSS int64, workload HTTPWorkloadResult) (*Artifact, error) {
	// Get service info
	serviceInfo := ServiceInfo{
		Name:    r.cfg.Service,
		Version: getBinaryVersion(r.cfg.Binary),
		Commit:  getGitCommit(),
	}

	// Get environment info
	envInfo := EnvironmentInfo{
		Arch:           GetArch(),
		Kernel:         getKernelVersion(),
		OS:             "Linux",
		HasSmapsRollup: HasSmapsRollup(),
	}

	// Build workload info
	workloadInfo := WorkloadInfo{
		Type:          string(r.cfg.WorkloadType),
		Operations:    workload.Operations,
		Errors:        workload.Errors,
		DurationMs:    workload.DurationMs,
		IntervalMs:    r.cfg.IntervalMs,
		Endpoint:      EndpointFor(r.cfg.WorkloadType),
		WarmupSeconds: r.cfg.WarmupSecs,
		Description:   describeWorkload(r.cfg.Service, r.cfg.WorkloadType, r.cfg.WarmupSecs),
	}

	// Create artifact
	artifact := NewArtifact(serviceInfo, workloadInfo, envInfo)
	artifact.SetMemory(first, last, maxRSS, maxPSS)

	// Load budget and make decision
	budget, err := LoadBudget(r.cfg.Service)
	if err != nil {
		// Budget not available - measurement only
		artifact.SetDecision(true, "Budget not loaded; measurement recorded")
	} else {
		decision := budget.CheckWorkloadBudget(string(r.cfg.WorkloadType), artifact.Memory.Growth.RSSKiB, artifact.Memory.Growth.RSSPercent)
		artifact.SetDecision(decision.Pass, decision.Reason)
	}

	// Set runtime info
	if r.cfg.Service == "tovarisch" {
		artifact.Runtime = RuntimeInfo{Allocator: "zig-default"}
	}
	// For uvb76, runtime info would come from HTTP API (future enhancement)

	return artifact, nil
}

// Helper functions

func findRepoRootOrCWD() string {
	if root, err := findRepoRoot(); err == nil {
		return root
	}
	cwd, _ := os.Getwd()
	return cwd
}

func getBinaryVersion(binary string) string {
	out, err := exec.Command(binary, "--version").Output()
	if err != nil {
		return "unknown"
	}
	// Return first line, trimmed
	return trimNL(string(out))
}

func getGitCommit() string {
	out, err := exec.Command("git", "rev-parse", "--short=7", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return trimNL(string(out))
}

func getKernelVersion() string {
	out, err := os.ReadFile("/proc/version")
	if err != nil {
		return "unknown"
	}
	return trimNL(string(out))
}

func trimNL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func describeWorkload(service string, wt WorkloadType, warmupSecs int) string {
	switch wt {
	case WorkloadTovarischStatusJSON, WorkloadUVB76StatusAPIPolling:
		return fmt.Sprintf("Repeated status calls after %ds warmup", warmupSecs)
	case WorkloadTovarischStatusJSONNetDiag, WorkloadUVB76DiagnosticCaptureLoop:
		return fmt.Sprintf("Repeated status with network_diag after %ds warmup", warmupSecs)
	default:
		return fmt.Sprintf("Idle memory footprint after %ds warmup", warmupSecs)
	}
}

// maxOf3 returns the maximum of three int64 values.
func maxOf3(a, b, c int64) int64 {
	if a > b {
		if a > c {
			return a
		}
		return c
	}
	if b > c {
		return b
	}
	return c
}
