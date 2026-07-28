package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-memleak-pprof-lab/internal/pprofdiff"
)

// runCollector runs the memory lab collector with proper preflight.
func runCollector() error {
	collectorBin := findCollectorBinary()
	if collectorBin == "" {
		return fmt.Errorf("collector binary not found")
	}

	// Build collector args explicitly
	pidStr := strconv.Itoa(uvb76PID)
	pprofURL := fmt.Sprintf("http://localhost:%s", *flagPProfPort)

	args := []string{
		collectorBin,
		"--pprof-url", pprofURL,
		"--pid", pidStr,
		"--duration", flagDuration.String(),
		"--sample-interval", flagSampleInterval.String(),
		"--profile-interval", flagProfileInterval.String(),
		"--artifact-dir", artifactDir,
	}

	log.Printf("[COLLECTION] Collector argv: %v", args)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
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
