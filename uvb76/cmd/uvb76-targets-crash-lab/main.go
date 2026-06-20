// Package main implements the UVB-76 Targets Crash Lab command.
package main

import (
	"crypto/tls"
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

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-targets-crash-lab/internal/certgen"
	"github.com/s1onique/KGB/uvb76/cmd/uvb76-targets-crash-lab/internal/configgen"
	"github.com/s1onique/KGB/uvb76/cmd/uvb76-targets-crash-lab/internal/workload"
)

const (
	LabName         = "kgb-uvb76-targets-crash"
	DefaultDuration = 60
	DefaultWorkers  = 8
	AdminUser       = "admin"
	AdminPass       = "testpass123"
)

type Summary struct {
	Status              string `json:"status"`
	StartedAt           string `json:"started_at"`
	CompletedAt         string `json:"completed_at"`
	DurationSecs       int    `json:"duration_seconds"`
	Workers            int    `json:"workers"`
	RequestCount       int    `json:"request_count"`
	SuccessCount       int    `json:"success_count"`
	ErrorCount         int    `json:"error_count"`
	Mode               string `json:"mode"`
	HTTP2Disabled      bool   `json:"http2_disabled"`
	ProcessExited      bool   `json:"process_exited"`
	ProcessExitCode    *int   `json:"process_exit_code,omitempty"`
	SawSIGSEGV         bool   `json:"saw_sigsegv"`
	SawPanic           bool   `json:"saw_panic"`
	SawFatalError      bool   `json:"saw_fatal_error"`
	SampleResponsePath string `json:"sample_response_path"`
	ArtifactDir        string `json:"artifact_dir"`
}

type ProcessState struct {
	exited   bool
	exitCode int
	mu       sync.Mutex
}

var (
	artifactDir string
	configFile string
	stdoutLog string
	stderrLog string
	pidFile   string
	uvb76PID  int
	procState ProcessState
	labPort   = configgen.LabPort
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	duration := getEnvInt("UVB76_TARGETS_CRASH_LAB_DURATION", DefaultDuration)
	workers := getEnvInt("UVB76_TARGETS_CRASH_LAB_WORKERS", DefaultWorkers)
	http2Disabled := os.Getenv("UVB76_TARGETS_CRASH_LAB_HTTP2_DISABLED") == "1"

	log.Printf("=== UVB-76 Targets Crash Lab ===")
	log.Printf("Duration: %ds, Workers: %d, HTTP/2 disabled: %v", duration, workers, http2Disabled)

	var err error
	artifactDir, err = createArtifactDir()
	if err != nil {
		log.Fatalf("Failed to create artifact dir: %v", err)
	}

	configFile = filepath.Join(artifactDir, "config.json")
	stdoutLog = filepath.Join(artifactDir, "uvb76.stdout.log")
	stderrLog = filepath.Join(artifactDir, "uvb76.stderr.log")
	pidFile = filepath.Join(artifactDir, "uvb76.pid")

	log.Printf("Artifact dir: %s", artifactDir)

	certFiles, err := certgen.GenerateSelfSigned(artifactDir)
	if err != nil {
		log.Fatalf("Failed to generate TLS certs: %v", err)
	}
	if err := certgen.ValidateCertFiles(certFiles.CertFile, certFiles.KeyFile); err != nil {
		log.Fatalf("Generated certs invalid: %v", err)
	}
	log.Printf("Generated TLS certs: %s, %s", certFiles.CertFile, certFiles.KeyFile)

	cfg, err := configgen.GenerateHTTPS(AdminUser, AdminPass, certFiles.CertFile, certFiles.KeyFile)
	if err != nil {
		log.Fatalf("Failed to generate config: %v", err)
	}
	cfgBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal config: %v", err)
	}
	if err := os.WriteFile(configFile, cfgBytes, 0644); err != nil {
		log.Fatalf("Failed to write config: %v", err)
	}

	uvb76Bin := findUVB76Binary()
	log.Printf("Starting UVB-76 HTTPS server on port %s...", labPort)

	env := os.Environ()
	if http2Disabled {
		env = append(env, "GODEBUG=http2server=0")
		log.Printf("HTTP/2 disabled via GODEBUG")
	}

	cmd := exec.Command(uvb76Bin, "-config", configFile)
	cmd.Env = env

	stdoutFile, err := os.OpenFile(stdoutLog, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalf("Failed to open stdout log: %v", err)
	}
	defer stdoutFile.Close()

	stderrFile, err := os.OpenFile(stderrLog, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalf("Failed to open stderr log: %v", err)
	}
	defer stderrFile.Close()

	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start UVB-76: %v", err)
	}
	uvb76PID = cmd.Process.Pid

	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(uvb76PID)), 0644); err != nil {
		log.Printf("Warning: failed to write PID file: %v", err)
	}

	startedAt := time.Now()
	daemonStarted := true

	done := make(chan struct{})
	go monitorProcess(cmd, &procState, done)
	defer close(done)

	port, err := waitForHTTPSReady(uvb76PID, 15*time.Second)
	if err != nil {
		log.Printf("Warning: HTTPS not ready within timeout: %v", err)
		daemonStarted = false
	}

	var wl workload.Result
	if daemonStarted {
		diag := workload.ExpectedDiagnosticTarget{
			ID:                  "target-with-diag",
			DiagnosticPeerName:  "diag-peer-home",
			DiagnosticBaseURL:   "http://127.0.0.1:19980",
			EffectiveCaptureURL: "",
		}
		wlRunner := workload.New(port, AdminUser, AdminPass, certFiles.CertFile, artifactDir, workers, diag)
		wl = wlRunner.Run(duration)
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
		log.Printf("Warning: daemon exited early with code %v", exitCode)
	}

	sawSIGSEGV, sawPanic, sawFatalError := checkFatalLogs(stdoutLog, stderrLog)

	modeName := "https-http2-default"
	if http2Disabled {
		modeName = "https-http2-disabled"
	}

	samplePath := ""
	if sampleData, err := os.ReadFile(filepath.Join(artifactDir, "targets-response-sample.json")); err == nil && len(sampleData) > 0 {
		samplePath = filepath.Join(artifactDir, "targets-response-sample.json")
	}

	completedAt := time.Now()
	durationSecs := int(completedAt.Sub(startedAt).Seconds())

	status := "pass"
	if !daemonStarted || exitedEarly || sawSIGSEGV || sawPanic || sawFatalError || wl.ErrorCount > 0 || wl.SuccessCount == 0 {
		status = "fail"
	}

	summary := Summary{
		Status:              status,
		StartedAt:           startedAt.Format(time.RFC3339),
		CompletedAt:         completedAt.Format(time.RFC3339),
		DurationSecs:       durationSecs,
		Workers:            workers,
		RequestCount:       wl.RequestCount,
		SuccessCount:       wl.SuccessCount,
		ErrorCount:         wl.ErrorCount,
		Mode:               modeName,
		HTTP2Disabled:      http2Disabled,
		ProcessExited:      exitedEarly,
		ProcessExitCode:    exitCode,
		SawSIGSEGV:         sawSIGSEGV,
		SawPanic:           sawPanic,
		SawFatalError:      sawFatalError,
		SampleResponsePath: samplePath,
		ArtifactDir:        artifactDir,
	}

	summaryBytes, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal summary: %v", err)
	}
	summaryFile := filepath.Join(artifactDir, "summary.json")
	if err := os.WriteFile(summaryFile, summaryBytes, 0644); err != nil {
		log.Fatalf("Failed to write summary: %v", err)
	}

	log.Printf("")
	log.Printf("=== Lab Summary ===")
	log.Printf("ARTIFACT_DIR=%s", artifactDir)
	log.Printf("Status: %s", summary.Status)
	log.Printf("Mode: %s", summary.Mode)
	log.Printf("Duration: %ds", summary.DurationSecs)
	log.Printf("Workers: %d", summary.Workers)
	log.Printf("Requests: %d total, %d success, %d errors", summary.RequestCount, summary.SuccessCount, summary.ErrorCount)
	log.Printf("Process exited early: %v", summary.ProcessExited)
	log.Printf("SIGSEGV: %v, Panic: %v, Fatal: %v", summary.SawSIGSEGV, summary.SawPanic, summary.SawFatalError)

	cleanup()

	if summary.Status == "fail" {
		log.Fatalf("Lab FAILED")
	}
}

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

func waitForHTTPSReady(pid int, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		proc, err := os.FindProcess(pid)
		if err != nil || proc.Signal(syscall.Signal(0)) != nil {
			return "", fmt.Errorf("process not running")
		}
		resp, err := client.Get(fmt.Sprintf("https://localhost:%s/api/v1/healthz", labPort))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return labPort, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", fmt.Errorf("timeout waiting for HTTPS readiness")
}

func checkFatalLogs(stdoutLog, stderrLog string) (sawSIGSEGV, sawPanic, sawFatalError bool) {
	patterns := []struct {
		pattern string
		flagPtr *bool
	}{
		{"SIGSEGV", &sawSIGSEGV},
		{"panic:", &sawPanic},
		{"fatal error", &sawFatalError},
	}
	for _, logFile := range []string{stdoutLog, stderrLog} {
		data, err := os.ReadFile(logFile)
		if err != nil {
			continue
		}
		content := string(data)
		for _, p := range patterns {
			if strings.Contains(content, p.pattern) {
				*p.flagPtr = true
			}
		}
	}
	return
}

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

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}
