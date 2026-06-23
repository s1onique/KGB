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
	"net/http"
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
	cmd      *exec.Cmd // kept for Wait() and Kill()
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
	pid, stdoutPath, err := r.startService()
	if err != nil {
		return "", fmt.Errorf("start service: %w", err)
	}
	defer r.stopService()

	if err := r.waitForReady(pid, stdoutPath); err != nil {
		return "", err
	}

	firstSnap, err := ReadMemorySnapshot(pid)
	if err != nil {
		return "", fmt.Errorf("first snapshot: %w", err)
	}
	fmt.Printf("Initial RSS: %d KiB, PSS: %d KiB\n", firstSnap.RSSKiB, firstSnap.PSSKiB)

	sampler := NewMemorySampler(pid, 2000)
	ctx, cancel := context.WithCancel(context.Background())
	sampler.Start(ctx)

	fmt.Printf("Warming up for %ds...\n", r.cfg.WarmupSecs)
	time.Sleep(time.Duration(r.cfg.WarmupSecs) * time.Second)

	workloadResult := r.executeWorkload(pid)
	fmt.Printf("Workload: %d ops, %d errors, %dms\n",
		workloadResult.Operations, workloadResult.Errors, workloadResult.DurationMs)

	cancel()
	sampler.Stop()

	lastSnap, err := ReadMemorySnapshot(pid)
	if err != nil {
		return "", fmt.Errorf("last snapshot: %w", err)
	}
	fmt.Printf("Final RSS: %d KiB, PSS: %d KiB\n", lastSnap.RSSKiB, lastSnap.PSSKiB)

	sampledMaxRSS, sampledMaxPSS := sampler.Max()
	maxRSS := maxOf3(firstSnap.RSSKiB, sampledMaxRSS, lastSnap.RSSKiB)
	maxPSS := maxOf3(firstSnap.PSSKiB, sampledMaxPSS, lastSnap.PSSKiB)
	fmt.Printf("Max RSS: %d KiB, Max PSS: %d KiB\n", maxRSS, maxPSS)

	artifact, err := r.buildArtifact(firstSnap, lastSnap, maxRSS, maxPSS, workloadResult)
	if err != nil {
		return "", fmt.Errorf("build artifact: %w", err)
	}

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

func (r *Runner) readinessURL() string {
	if r.cfg.Service == "tovarisch" {
		return fmt.Sprintf("http://127.0.0.1:%d/status", r.cfg.Port)
	}
	return fmt.Sprintf("http://127.0.0.1:%d/api/v1/status", r.cfg.Port)
}

func (r *Runner) startService() (int, string, error) {
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

	r.cmd = r.buildServiceCommand()
	r.cmd.Stdout = stdoutFile
	r.cmd.Stderr = stdoutFile

	if err := r.cmd.Start(); err != nil {
		return 0, "", err
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		_ = r.cmd.Wait()
	}()

	if r.cmd.Process == nil {
		return 0, "", fmt.Errorf("process not started")
	}

	return r.cmd.Process.Pid, stdoutPath, nil
}

func (r *Runner) waitForReady(pid int, stdoutPath string) error {
	deadline := time.Now().Add(15 * time.Second)
	url := r.readinessURL()

	waitCh := make(chan struct{}, 1)
	go func() {
		r.wg.Wait()
		waitCh <- struct{}{}
	}()

	for time.Now().Before(deadline) {
		select {
		case <-waitCh:
			tail := readTail(stdoutPath, 8192)
			return &ServiceExitError{
				PID:        pid,
				Argv:       r.cmd.Args,
				ExitError:  r.cmd.ProcessState,
				StdoutTail: tail,
			}
		default:
		}

		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		time.Sleep(250 * time.Millisecond)
	}

	tail := readTail(stdoutPath, 8192)
	return &ReadinessTimeoutError{
		PID:          pid,
		ReadinessURL: url,
		StdoutTail:   tail,
	}
}

func (r *Runner) stopService() {
	if r.cmd != nil && r.cmd.Process != nil {
		r.cmd.Process.Kill()
		r.wg.Wait()
	}
}

func (r *Runner) executeWorkload(pid int) HTTPWorkloadResult {
	if r.cfg.WorkloadType == WorkloadTovarischIdle || r.cfg.WorkloadType == WorkloadUVB76Idle {
		return HTTPWorkloadResult{
			Operations: 0,
			Errors:     0,
			DurationMs: int64(r.cfg.WarmupSecs * 1000),
		}
	}

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
	serviceInfo := ServiceInfo{
		Name:    r.cfg.Service,
		Version: getBinaryVersion(r.cfg.Binary),
		Commit:  getGitCommit(),
	}

	envInfo := EnvironmentInfo{
		Arch:           GetArch(),
		Kernel:         getKernelVersion(),
		OS:             "Linux",
		HasSmapsRollup: HasSmapsRollup(),
	}

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

	artifact := NewArtifact(serviceInfo, workloadInfo, envInfo)
	artifact.SetMemory(first, last, maxRSS, maxPSS)

	budget, err := LoadBudget(r.cfg.Service)
	if err != nil {
		artifact.SetDecision(true, "Budget not loaded; measurement recorded")
	} else {
		decision := budget.CheckWorkloadBudget(string(r.cfg.WorkloadType), artifact.Memory.Growth.RSSKiB, artifact.Memory.Growth.RSSPercent)
		artifact.SetDecision(decision.Pass, decision.Reason)
	}

	if r.cfg.Service == "tovarisch" {
		artifact.Runtime = RuntimeInfo{Allocator: "zig-default"}
	}

	return artifact, nil
}
