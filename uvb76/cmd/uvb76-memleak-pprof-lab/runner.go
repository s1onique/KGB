package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-memleak-pprof-lab/internal/fake"
	"github.com/s1onique/KGB/uvb76/cmd/uvb76-memleak-pprof-lab/internal/pprofdiff"
)

// runLab orchestrates the full memory leak lab.
func runLab() LabResult {
	result := LabResult{
		DurationSeconds: int(flagDuration.Seconds()),
		ArtifactDir:     artifactDir,
	}

	// Start fake tovarisch if configured
	if *flagUseFakeTovarisch {
		log.Printf("Starting fake tovarisch status server on port %s...", tovarischPort)
		if err := startFakeTovarisch(); err != nil {
			log.Printf("Warning: failed to start fake tovarisch: %v", err)
		} else {
			result.TovarischReachable = waitForHTTPReady("http://localhost:"+tovarischPort+"/status", 5*time.Second)
			log.Printf("Fake tovarisch reachable: %v", result.TovarischReachable)
		}
	}

	// Find and start UVB-76
	uvb76Bin := findUVB76Binary()
	log.Printf("Starting UVB-76 with pprof on port %s...", uvb76Port)

	if err := startUVB76(uvb76Bin); err != nil {
		log.Printf("Warning: failed to start UVB-76: %v", err)
		cleanup()
		return result
	}
	result.UVB76Started = true

	// Wait for pprof to be reachable
	result.PProfReachable = waitForHTTPReady("http://localhost:"+pprofPort+"/debug/pprof/heap", 10*time.Second)
	log.Printf("pprof reachable: %v", result.PProfReachable)

	if !result.PProfReachable {
		log.Printf("Warning: pprof not reachable, continuing anyway")
	}

	// Wait for tovarisch endpoint to be reachable (if using real tovarisch)
	if !*flagUseFakeTovarisch {
		result.TovarischReachable = waitForHTTPReady("http://localhost:"+tovarischPort+"/status", 5*time.Second)
	}

	// Run collector
	log.Printf("Running memory lab collector...")
	if err := runCollector(); err != nil {
		log.Printf("Warning: collector failed: %v", err)
		cleanup()
		return result
	}
	result.CollectorSucceeded = true

	// Run pprof diff
	if !*flagSkipPprofDiff {
		log.Printf("Running pprof diff reports...")
		if err := runPprofDiff(); err != nil {
			log.Printf("Warning: pprof diff failed: %v", err)
		} else {
			result.PProfDiffSucceeded = true
		}
	}

	// Validate artifacts
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

	// Cleanup processes
	cleanup()

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

// startUVB76 starts UVB-76 as a child process.
func startUVB76(bin string) error {
	args := []string{
		"-dev",
		"-config", configFile,
	}

	cmd := exec.Command(bin, args...)

	// Redirect output to log file
	logOut, err := os.OpenFile(uvb76LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	cmd.Stdout = logOut
	cmd.Stderr = logOut

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	uvb76PID = cmd.Process.Pid

	// Write PID file
	pidFile := filepath.Join(artifactDir, "uvb76.pid")
	os.WriteFile(pidFile, []byte(strconv.Itoa(uvb76PID)), 0644)

	// Monitor process
	go func() {
		cmd.Wait()
		close(uvb76Done)
	}()

	return nil
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
		"../../uvb76",
		"/usr/local/bin/uvb76",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return "uvb76"
}

// waitForHTTPReady waits for an HTTP endpoint to be ready.
func waitForHTTPReady(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return true
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// runCollector runs the memory lab collector.
func runCollector() error {
	collectorBin := findCollectorBinary()
	if collectorBin == "" {
		return fmt.Errorf("collector binary not found")
	}

	// Build collector args explicitly
	pidStr := strconv.Itoa(uvb76PID)
	pprofURL := fmt.Sprintf("http://localhost:%s", pprofPort)

	args := []string{
		collectorBin,
		"--pprof-url", pprofURL,
		"--pid", pidStr,
		"--duration", flagDuration.String(),
		"--sample-interval", flagSampleInterval.String(),
		"--profile-interval", flagProfileInterval.String(),
		"--artifact-dir", artifactDir,
	}

	log.Printf("Collector argv: %v", args)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case <-labCtx.Done():
		cmd.Process.Kill()
		return labCtx.Err()
	case err := <-done:
		return err
	}
}

// findCollectorBinary locates the memory lab collector binary.
func findCollectorBinary() string {
	// Check environment variable first
	if bin := os.Getenv("COLLECTOR_BINARY"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}

	// Check common paths relative to this binary
	baseDir := filepath.Dir(filepath.Dir(filepath.Dir(filepath.FromSlash("./uvb76-memory-lab"))))
	paths := []string{
		"./uvb76-memory-lab",
		"../../uvb76-memory-lab",
		filepath.Join(baseDir, "uvb76-memory-lab"),
		"/tmp/uvb76-memory-lab",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// runPprofDiff executes pprof diff reports.
func runPprofDiff() error {
	durationSec := int(flagDuration.Seconds())
	finalSuffix := fmt.Sprintf("t%03d", durationSec)

	heapBase := filepath.Join(artifactDir, "heap-t000.pb.gz")
	heapFinal := filepath.Join(artifactDir, fmt.Sprintf("heap-%s.pb.gz", finalSuffix))
	allocsFinal := filepath.Join(artifactDir, fmt.Sprintf("allocs-%s.pb.gz", finalSuffix))

	// Check required files exist
	for _, f := range []string{heapBase, heapFinal, allocsFinal} {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			return fmt.Errorf("required profile missing: %s", f)
		}
	}

	// Execute pprof diff reports
	diff := pprofdiff.NewRunner(artifactDir)

	reports := []pprofdiff.ReportConfig{
		{
			Name:          "heap-diff-inuse-space.txt",
			BaseProfile:   "heap-t000.pb.gz",
			TargetProfile: fmt.Sprintf("heap-%s.pb.gz", finalSuffix),
			OutputType:    "top",
			SampleType:    "inuse_space",
			DiffBase:      true,
		},
		{
			Name:          "heap-diff-inuse-objects.txt",
			BaseProfile:   "heap-t000.pb.gz",
			TargetProfile: fmt.Sprintf("heap-%s.pb.gz", finalSuffix),
			OutputType:    "top",
			SampleType:    "inuse_objects",
			DiffBase:      true,
		},
		{
			Name:          "allocs-final-alloc-space.txt",
			BaseProfile:   "",
			TargetProfile: fmt.Sprintf("allocs-%s.pb.gz", finalSuffix),
			OutputType:    "top",
			SampleType:    "alloc_space",
			DiffBase:      false,
		},
	}

	return diff.RunReports(labCtx, reports)
}

// cleanup stops child processes and fake server.
func cleanup() {
	// Shutdown fake server gracefully first
	if fakeServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		fakeServer.Shutdown(ctx)
		fakeServer = nil
	}

	// Then kill uvb76 process
	if uvb76PID > 0 {
		if proc, err := os.FindProcess(uvb76PID); err == nil {
			proc.Signal(syscall.SIGTERM)
			time.Sleep(2 * time.Second)
			proc.Kill()
		}
	}
}
