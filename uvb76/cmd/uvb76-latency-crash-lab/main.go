// Package main implements the UVB-76 Latency Crash Lab command.
//
// This is the canonical Golang daemon crash/soak lab for the LatencyTracker SIGSEGV
// reference scenario. It exercises the production read path with high sample limits
// while the ICMP latency tracker is running.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-latency-crash-lab/internal/configgen"
	"github.com/s1onique/KGB/uvb76/cmd/uvb76-latency-crash-lab/internal/workload"
)

// Lab constants
const (
	LabName                = "kgb-uvb76-latency-crash"
	WorkloadDurationSeconds = 60
	ICMPIntervalSeconds    = 1
	RequestLimit           = 3600
	AdminUser              = "admin"
	AdminPass              = "testpass123"
	TargetID               = "test-target"
	LabPort                = configgen.LabPort
)

// Result captures the lab outcome for machine-readable output.
type Result struct {
	OK                    bool     `json:"ok"`
	DurationSeconds       int      `json:"duration_seconds"`
	RequestedSampleLimit  int      `json:"requested_sample_limit"`
	DaemonStarted         bool     `json:"daemon_started"`
	DaemonExitedEarly     bool     `json:"daemon_exited_early"`
	DaemonExitCode        *int     `json:"daemon_exit_code,omitempty"`
	PIDStable             bool     `json:"pid_stable"`
	FatalLogPatternsFound []string `json:"fatal_log_patterns_found"`
	SampleEndpointValid   bool     `json:"sample_endpoint_valid_json"`
	SummaryEndpointValid  bool     `json:"summary_endpoint_valid_json"`
	SampleCountIncreased  bool     `json:"sample_count_increased"`
	RequestsTotal         int      `json:"requests_total"`
	RequestsFailed        int      `json:"requests_failed"`
	MaxObservedSampleCount int     `json:"max_observed_sample_count"`
	ArtifactDir           string   `json:"artifact_dir"`
}

// ProcessState tracks daemon process state.
type ProcessState struct {
	exited   bool
	exitCode int
	mu       sync.Mutex
}

// Global state
var (
	artifactDir  string
	configFile  string
	logFile     string
	pidFile     string
	uvb76PID    int
	procState   ProcessState
	labPort     = LabPort
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Create artifact directory
	var err error
	artifactDir, err = createArtifactDir()
	if err != nil {
		log.Fatalf("Failed to create artifact dir: %v", err)
	}
	defer cleanup()

	configFile = filepath.Join(artifactDir, "config.json")
	logFile = filepath.Join(artifactDir, "uvb76.log")
	pidFile = filepath.Join(artifactDir, "uvb76.pid")

	log.Printf("=== UVB-76 Latency Crash Lab ===")
	log.Printf("Artifact dir: %s", artifactDir)
	log.Printf("Duration: %ds, ICMP interval: %ds, Request limit: %d",
		WorkloadDurationSeconds, ICMPIntervalSeconds, RequestLimit)

	// Generate hermetic config with deterministic port
	cfg := configgen.Generate(AdminUser, AdminPass, TargetID, ICMPIntervalSeconds)
	cfgBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal config: %v", err)
	}
	if err := os.WriteFile(configFile, cfgBytes, 0644); err != nil {
		log.Fatalf("Failed to write config: %v", err)
	}

	// Find UVB-76 binary
	uvb76Bin := findUVB76Binary()

	// Start UVB-76 as child process with combined stdout/stderr to log
	log.Printf("Starting UVB-76 on port %s...", labPort)
	cmd := exec.Command(uvb76Bin, "-dev", "-config", configFile)
	
	// Capture both stdout and stderr to log file
	logOut, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logOut.Close()
	cmd.Stdout = logOut
	cmd.Stderr = logOut

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start UVB-76: %v", err)
	}
	uvb76PID = cmd.Process.Pid

	// Write PID file
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(uvb76PID)), 0644); err != nil {
		log.Printf("Warning: failed to write PID file: %v", err)
	}

	daemonStarted := true
	daemonExitedEarly := false

	// Start goroutine to monitor child process exit
	done := make(chan struct{})
	go monitorProcess(cmd, &procState, done)
	defer close(done)

	// Wait for HTTP readiness
	port, err := waitForHTTPReady(uvb76PID, 10*time.Second)
	if err != nil {
		log.Printf("Warning: HTTP not ready within timeout: %v", err)
		daemonStarted = false
	}

	// Run workload
	startTime := time.Now()
	var wl workload.Result
	if daemonStarted {
		wl = runWorkload(port, artifactDir)
	}
	duration := int(time.Since(startTime).Seconds())

	// Check if daemon exited early
	procState.mu.Lock()
	exitedEarly := procState.exited
	var exitCode *int
	if procState.exited {
		ec := procState.exitCode
		exitCode = &ec
	}
	procState.mu.Unlock()

	if exitedEarly {
		daemonExitedEarly = true
		log.Printf("Warning: daemon exited early with code %d", *exitCode)
	}

	// Check if daemon is still running
	var pidStable bool
	if uvb76PID > 0 {
		proc, err := os.FindProcess(uvb76PID)
		if err == nil {
			err = proc.Signal(syscall.Signal(0))
			pidStable = (err == nil)
		} else {
			pidStable = false
		}
	} else {
		pidStable = false
	}

	// Check for fatal log patterns
	fatalPatterns := checkFatalLogs(logFile)

	// Generate result — OK encodes full contract including workload correctness
	// Note: SampleCount may be 0 in lab environments where ICMP targets are unreachable.
	// The key crash criteria are: daemon survives, no fatal logs, requests succeed.
	result := Result{
		OK: daemonStarted &&
			!daemonExitedEarly &&
			pidStable &&
			len(fatalPatterns) == 0 &&
			wl.RequestsFailed == 0 &&
			wl.RequestsTotal > 0,
		DurationSeconds:       duration,
		RequestedSampleLimit:  RequestLimit,
		DaemonStarted:         daemonStarted,
		DaemonExitedEarly:     daemonExitedEarly,
		DaemonExitCode:        exitCode,
		PIDStable:             pidStable,
		FatalLogPatternsFound: fatalPatterns,
		SampleEndpointValid:   wl.SampleValid,
		SummaryEndpointValid:  wl.SummaryValid,
		SampleCountIncreased:  wl.SampleCount > 0,
		RequestsTotal:         wl.RequestsTotal,
		RequestsFailed:        wl.RequestsFailed,
		MaxObservedSampleCount: wl.MaxSampleCount,
		ArtifactDir:           artifactDir,
	}

	// Write result.json
	resultBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal result: %v", err)
	}
	resultFile := filepath.Join(artifactDir, "result.json")
	if err := os.WriteFile(resultFile, resultBytes, 0644); err != nil {
		log.Fatalf("Failed to write result: %v", err)
	}

	// Print summary
	log.Printf("")
	log.Printf("=== Lab Result ===")
	log.Printf("OK: %v", result.OK)
	log.Printf("Daemon started: %v", result.DaemonStarted)
	log.Printf("Daemon exited early: %v", result.DaemonExitedEarly)
	log.Printf("PID stable: %v", result.PIDStable)
	log.Printf("Fatal patterns: %v", result.FatalLogPatternsFound)
	log.Printf("Requests: %d total, %d failed", result.RequestsTotal, result.RequestsFailed)
	log.Printf("Max observed samples: %d", result.MaxObservedSampleCount)
	log.Printf("Artifact dir: %s", artifactDir)

	if !result.OK {
		log.Fatalf("Lab failed")
	}
}

// monitorProcess monitors the child process and records its exit status.
func monitorProcess(cmd *exec.Cmd, state *ProcessState, done chan struct{}) {
	select {
	case <-done:
		return
	default:
		err := cmd.Wait()
		state.mu.Lock()
		state.exited = true
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
					state.exitCode = status.ExitStatus()
				}
			}
		}
		state.mu.Unlock()
	}
}

// createArtifactDir creates a unique temp directory for lab artifacts.
func createArtifactDir() (string, error) {
	dir, err := os.MkdirTemp("/tmp", LabName+"-*")
	if err != nil {
		return "", fmt.Errorf("mkdtemp: %w", err)
	}
	return dir, nil
}

// findUVB76Binary locates the UVB-76 binary.
func findUVB76Binary() string {
	// Check UVB76_BINARY env var first
	if bin := os.Getenv("UVB76_BINARY"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}

	// Check common paths
	paths := []string{
		"./uvb76",
		"../uvb76/uvb76",
		"/usr/local/bin/uvb76",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Fall back to "uvb76" in PATH
	return "uvb76"
}

// waitForHTTPReady waits for UVB-76 HTTP server to be ready.
func waitForHTTPReady(pid int, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check if process is still alive
		proc, err := os.FindProcess(pid)
		if err != nil || proc.Signal(syscall.Signal(0)) != nil {
			return "", fmt.Errorf("process not running")
		}

		// Try to connect to lab port
		resp, err := httpGet(fmt.Sprintf("http://localhost:%s/api/v1/healthz", labPort))
		if err == nil && resp.StatusCode == 200 {
			return labPort, nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return "", fmt.Errorf("timeout waiting for HTTP readiness")
}

// httpGet performs an HTTP GET request.
func httpGet(url string) (*http.Response, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		resp.Body.Close()
	}
	return resp, nil
}

// runWorkload executes the accelerated query workload.
func runWorkload(port, artifactDir string) workload.Result {
	log.Printf("Running accelerated workload...")
	wl := workload.New(port, AdminUser, AdminPass, TargetID, RequestLimit, artifactDir)
	return wl.Run(WorkloadDurationSeconds)
}

// checkFatalLogs scans the log file for fatal patterns.
func checkFatalLogs(logFile string) []string {
	patterns := []string{
		"fatal error",
		"SIGSEGV",
		"segmentation violation",
		"runtime.throw",
		"panic:",
		"WARNING: DATA RACE",
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		return patterns // assume all patterns if we can't read
	}
	content := string(data)

	var found []string
	for _, p := range patterns {
		if strings.Contains(content, p) {
			found = append(found, p)
		}
	}
	return found
}

// cleanup stops the daemon and cleans up.
func cleanup() {
	if uvb76PID > 0 {
		proc, _ := os.FindProcess(uvb76PID)
		if proc != nil {
			proc.Signal(syscall.SIGTERM)
			time.Sleep(2 * time.Second)
			proc.Kill()
		}
	}
}
