package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-icmp-os-ping-soak/internal/configgen"
	"github.com/s1onique/KGB/uvb76/cmd/uvb76-icmp-os-ping-soak/internal/verifier"
)

const (
	LabName                = "kgb-uvb76-icmp-os-ping-soak"
	DefaultDurationSeconds = 120
	ICMPIntervalSeconds    = 1
	ICMPTimeoutSeconds     = 3
	ICMPMaxConcurrent      = 1
	LabPort                = "18317"
)

var (
	artifactDir string
	configFile  string
	logFile     string
	pidFile     string
	uvb76PID    int
	procState   ProcessState
)

type ProcessState struct {
	exited   bool
	exitCode int
	mu       sync.Mutex
}

// DaemonStatus represents the structure of the daemon's /api/v1/status endpoint.
type DaemonStatus struct {
	StartedAt  string `json:"started_at"`
	ICMPOSPing *ICMPOSPingTelemetry `json:"icmp_os_ping,omitempty"`
}

// ICMPOSPingTelemetry represents the ICMP OS ping telemetry from the daemon.
type ICMPOSPingTelemetry struct {
	Enabled       bool   `json:"enabled"`
	Attempts      uint64 `json:"attempts"`
	Successes     uint64 `json:"successes"`
	Failures      uint64 `json:"failures"`
	LastError     string `json:"last_error,omitempty"`
	MaxConcurrent int    `json:"max_concurrent"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	duration := DefaultDurationSeconds
	if envDur := os.Getenv("SOAK_DURATION_SECONDS"); envDur != "" {
		if d, err := strconv.Atoi(envDur); err == nil && d > 0 {
			duration = d
		}
	}

	var err error
	artifactDir, err = createArtifactDir()
	if err != nil {
		log.Fatalf("Failed to create artifact dir: %v", err)
	}
	defer cleanup()

	configFile = filepath.Join(artifactDir, "config.json")
	logFile = filepath.Join(artifactDir, "uvb76.log")
	pidFile = filepath.Join(artifactDir, "uvb76.pid")

	log.Printf("=== UVB-76 ICMP OS Ping Soak Lab ===")
	log.Printf("Artifact dir: %s", artifactDir)
	log.Printf("Duration: %ds, ICMP: interval=%ds, timeout=%ds, max_concurrent=%d",
		duration, ICMPIntervalSeconds, ICMPTimeoutSeconds, ICMPMaxConcurrent)

	memBefore := captureMemStats()
	goroutinesBefore := runtime.NumGoroutine()
	memBeforeJSON, _ := json.MarshalIndent(memBefore, "", "  ")

	cfg := configgen.Generate("admin", "testpass123", "icmp-test-target",
		ICMPIntervalSeconds, ICMPTimeoutSeconds, ICMPMaxConcurrent)
	cfgBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal config: %v", err)
	}
	os.WriteFile(configFile, cfgBytes, 0644)

	uvb76Bin := findUVB76Binary()
	log.Printf("Starting UVB-76 on port %s...", LabPort)
	cmd := exec.Command(uvb76Bin, "-dev", "-config", configFile)

	logOut, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	cmd.Stdout = logOut
	cmd.Stderr = logOut
	defer logOut.Close()

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start UVB-76: %v", err)
	}
	uvb76PID = cmd.Process.Pid
	os.WriteFile(pidFile, []byte(strconv.Itoa(uvb76PID)), 0644)

	daemonStarted := true
	done := make(chan struct{})
	go monitorProcess(cmd, &procState, done)
	defer close(done)

	_, err = waitForHTTPReady(uvb76PID, 10*time.Second)
	if err != nil {
		log.Printf("Warning: HTTP not ready: %v", err)
		daemonStarted = false
	}

	startTime := time.Now()
	if daemonStarted {
		log.Printf("Running ICMP soak for %ds...", duration)
		time.Sleep(time.Duration(duration) * time.Second)
	}
	actualDuration := int(time.Since(startTime).Seconds())

	memAfter := captureMemStats()
	goroutinesAfter := runtime.NumGoroutine()
	memAfterJSON, _ := json.MarshalIndent(memAfter, "", "  ")

	// Poll daemon status to get authoritative ICMP telemetry
	var daemonStatus DaemonStatus
	var daemonStatusRaw string
	var icmpExercised bool
	var exercisedReason string
	var evidenceSource string

	if daemonStarted {
		daemonStatus, daemonStatusRaw, icmpExercised, exercisedReason, evidenceSource = pollDaemonStatus()
	} else {
		icmpExercised = false
		exercisedReason = "daemon did not start"
		evidenceSource = ""
	}

	procState.mu.Lock()
	exitedEarly := procState.exited
	var exitCode *int
	if procState.exited {
		ec := procState.exitCode
		exitCode = &ec
	}
	procState.mu.Unlock()

	if exitedEarly {
		log.Printf("Warning: daemon exited early with code %d", *exitCode)
	}

	var pidStable bool
	if uvb76PID > 0 {
		if proc, err := os.FindProcess(uvb76PID); err == nil {
			err = proc.Signal(syscall.Signal(0))
			pidStable = err == nil
		}
	}

	fatalPatterns := checkFatalLogs(logFile)
	healthValid := checkEndpoint("/api/v1/healthz")
	statusValid := checkEndpoint("/api/v1/status")
	goroutineLeaked := goroutinesAfter > goroutinesBefore+5

	// Build result - lab passes only if daemon-sourced ICMP attempts > 0
	result := Result{
		OK:                     daemonStarted && !exitedEarly && pidStable && len(fatalPatterns) == 0 && !goroutineLeaked && healthValid && icmpExercised,
		LabName:                LabName,
		DurationSeconds:        actualDuration,
		ICMPEnabled:            cfg.Latency.ICMP.Enabled == nil || *cfg.Latency.ICMP.Enabled,
		ICMPIntervalSeconds:    ICMPIntervalSeconds,
		ICMPTimeoutSeconds:     ICMPTimeoutSeconds,
		ICMPMaxConcurrent:      ICMPMaxConcurrent,
		DaemonStarted:          daemonStarted,
		DaemonExitedEarly:      exitedEarly,
		DaemonExitCode:         exitCode,
		PIDStable:              pidStable,
		FatalLogPatternsFound:  fatalPatterns,
		DaemonICMPAttempts:     func() uint64 { if daemonStatus.ICMPOSPing != nil { return daemonStatus.ICMPOSPing.Attempts }; return 0 }(),
		DaemonICMPSuccesses:    func() uint64 { if daemonStatus.ICMPOSPing != nil { return daemonStatus.ICMPOSPing.Successes }; return 0 }(),
		DaemonICMPFailures:     func() uint64 { if daemonStatus.ICMPOSPing != nil { return daemonStatus.ICMPOSPing.Failures }; return 0 }(),
		DaemonICMPLastError:    func() string { if daemonStatus.ICMPOSPing != nil { return daemonStatus.ICMPOSPing.LastError }; return "" }(),
		DaemonStatusRaw:        daemonStatusRaw,
		ICMPProbeExercised:     icmpExercised,
		ICMPProbeExercisedReason: exercisedReason,
		ICMPEvidenceSource:     evidenceSource,
		MemStatsBefore:         string(memBeforeJSON),
		MemStatsAfter:          string(memAfterJSON),
		GoroutinesBefore:       goroutinesBefore,
		GoroutinesAfter:        goroutinesAfter,
		GoroutineLeaked:        goroutineLeaked,
		HealthEndpointValid:    healthValid,
		StatusEndpointValid:    statusValid,
		ArtifactDir:            artifactDir,
	}

	if err := WriteArtifacts(artifactDir, result, memBefore, memAfter, goroutinesBefore, goroutinesAfter); err != nil {
		log.Printf("Warning: failed to write artifacts: %v", err)
	}

	log.Printf("")
	log.Printf("=== Lab Result ===")
	log.Printf("OK: %v", result.OK)
	log.Printf("ICMP Probe Exercised: %v (%s)", result.ICMPProbeExercised, result.ICMPProbeExercisedReason)
	log.Printf("ICMP Evidence Source: %s", result.ICMPEvidenceSource)
	log.Printf("Daemon ICMP Attempts: %d", result.DaemonICMPAttempts)
	log.Printf("Goroutines: before=%d, after=%d, leaked=%v",
		result.GoroutinesBefore, result.GoroutinesAfter, result.GoroutineLeaked)

	if !result.OK {
		log.Fatalf("Lab failed")
	}
}

// pollDaemonStatus fetches the daemon status endpoint and derives ICMP exercise status.
// Uses the canonical verifier to ensure consistent acceptance logic with tests.
func pollDaemonStatus() (DaemonStatus, string, bool, string, string) {
	statusURL := fmt.Sprintf("http://localhost:%s/api/v1/status", LabPort)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(statusURL)
	if err != nil {
		return DaemonStatus{}, "", false, fmt.Sprintf("failed to fetch daemon status: %v", err), ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return DaemonStatus{}, "", false, fmt.Sprintf("daemon status returned %d", resp.StatusCode), ""
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 65536)) // 64KB limit for safety
	if err != nil {
		return DaemonStatus{}, "", false, fmt.Sprintf("failed to read daemon status: %v", err), ""
	}
	rawJSON := string(body)

	// Use the canonical verifier for acceptance logic
	verifyResult := verifier.VerifyDaemonStatus(rawJSON)

	// Parse status for artifact fields
	var status DaemonStatus
	_ = json.Unmarshal(body, &status)

	return status, rawJSON, verifyResult.ICMPExercised, verifyResult.Reason, verifyResult.EvidenceSource
}

func monitorProcess(cmd *exec.Cmd, state *ProcessState, done chan struct{}) {
	select {
	case <-done:
		return
	default:
		err := cmd.Wait()
		state.mu.Lock()
		state.exited = true
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				state.exitCode = status.ExitStatus()
			}
		}
		state.mu.Unlock()
	}
}

func createArtifactDir() (string, error) {
	dir, err := os.MkdirTemp("/tmp", LabName+"-*")
	if err != nil {
		return "", fmt.Errorf("mkdtemp: %w", err)
	}
	return dir, nil
}

func findUVB76Binary() string {
	if bin := os.Getenv("UVB76_BINARY"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}
	paths := []string{"./uvb76", "../uvb76/uvb76", "/usr/local/bin/uvb76"}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "uvb76"
}

func waitForHTTPReady(pid int, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if proc, err := os.FindProcess(pid); err != nil || proc.Signal(syscall.Signal(0)) != nil {
			return "", fmt.Errorf("process not running")
		}
		if resp, err := httpGet(fmt.Sprintf("http://localhost:%s/api/v1/healthz", LabPort)); err == nil && resp.StatusCode == 200 {
			return LabPort, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", fmt.Errorf("timeout")
}

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

func checkEndpoint(path string) bool {
	resp, err := httpGet(fmt.Sprintf("http://localhost:%s%s", LabPort, path))
	return err == nil && resp.StatusCode == 200
}

func checkFatalLogs(logFile string) []string {
	patterns := []string{"fatal error", "SIGSEGV", "segmentation violation", "runtime.throw", "panic:", "WARNING: DATA RACE"}
	data, err := os.ReadFile(logFile)
	if err != nil {
		return patterns
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

func cleanup() {
	if uvb76PID > 0 {
		if proc, _ := os.FindProcess(uvb76PID); proc != nil {
			proc.Signal(syscall.SIGTERM)
			time.Sleep(2 * time.Second)
			proc.Kill()
		}
	}
}
