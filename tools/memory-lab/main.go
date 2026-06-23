// main.go — Memory lab CLI entrypoint
//
// Command-line interface for the Go-based memory lab runner.
// Replaces Bash-based memory lab scripts.
//
// Usage:
//   memory-lab tovarisch --workload idle-warmup
//   memory-lab tovarisch --workload status-json-network-diag --operations 100
//   memory-lab uvb76 --workload idle-warmup --warmup-secs 120
//
// Reference: kgb://doctrine/embedded-memory-frugality

package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// Parse flags
	fs := flag.NewFlagSet("memory-lab", flag.ContinueOnError)
	
	service := fs.String("service", "", "Service: tovarisch or uvb76 (required)")
	workload := fs.String("workload", "", "Workload type (required)")
	binary := fs.String("binary", "", "Path to service binary")
	configPath := fs.String("config", "", "Path to config file (uvb76 only)")
	port := fs.Int("port", 0, "Listen port (default: 18080 for tovarisch, 18081 for uvb76)")
	warmupSecs := fs.Int("warmup-secs", 60, "Warmup period in seconds")
	operations := fs.Int("operations", 100, "Number of HTTP operations")
	intervalMs := fs.Int("interval-ms", 100, "Interval between operations in ms")
	artifactDir := fs.String("artifacts-dir", "", "Artifact output directory")
	
	// Help flag
	help := fs.Bool("help", false, "Show help")
	
	if err := fs.Parse(args); err != nil {
		return err
	}
	
	if *help {
		printUsage()
		return nil
	}
	
	// Validate required flags
	if *service == "" {
		return fmt.Errorf("--service is required (tovarisch or uvb76)")
	}
	if *workload == "" {
		return fmt.Errorf("--workload is required")
	}
	
	// Set default port if not specified
	if *port == 0 {
		if *service == "tovarisch" {
			*port = 18080
		} else {
			*port = 18081
		}
	}
	
	// Set default binary if not specified
	if *binary == "" {
		if *service == "tovarisch" {
			*binary = "./tovarisch/zig-out/bin/tovarisch"
		} else {
			*binary = "./uvb76/uvb76"
		}
	}
	
	// Parse workload type
	wt := WorkloadType(*workload)
	if !validWorkload(*service, wt) {
		return fmt.Errorf("invalid workload %q for service %q", *workload, *service)
	}
	
	// Build run config
	cfg := RunConfig{
		Service:      *service,
		WorkloadType: wt,
		Binary:       *binary,
		ConfigPath:   *configPath,
		Port:         *port,
		WarmupSecs:   *warmupSecs,
		Operations:   *operations,
		IntervalMs:   *intervalMs,
		ArtifactDir:  *artifactDir,
	}
	
	fmt.Printf("=== Memory Lab ===\n")
	fmt.Printf("Service: %s\n", cfg.Service)
	fmt.Printf("Workload: %s\n", cfg.WorkloadType)
	fmt.Printf("Binary: %s\n", cfg.Binary)
	fmt.Printf("Port: %d\n", cfg.Port)
	fmt.Printf("Warmup: %ds\n", cfg.WarmupSecs)
	
	// Run the lab
	_, err := Run(cfg)
	return err
}

func validWorkload(service string, wt WorkloadType) bool {
	switch service {
	case "tovarisch":
		return wt == WorkloadTovarischIdle ||
			wt == WorkloadTovarischStatusJSON ||
			wt == WorkloadTovarischStatusJSONNetDiag
	case "uvb76":
		return wt == WorkloadUVB76Idle ||
			wt == WorkloadUVB76StatusAPIPolling ||
			wt == WorkloadUVB76DiagnosticCaptureLoop
	default:
		return false
	}
}

func printUsage() {
	fmt.Print(`memory-lab — Go-based memory lab runner

Usage: memory-lab [flags]

Flags:
  --service NAME       Service to test: tovarisch or uvb76 (required)
  --workload TYPE      Workload type (required)
  --binary PATH        Path to service binary
  --config PATH        Path to config file (uvb76 only, default: ./uvb76/uvb76.example.json)
  --port PORT          Listen port (default: 18080 for tovarisch, 18081 for uvb76)
  --warmup-secs N      Warmup period in seconds (default: 60)
  --operations N       Number of HTTP operations (default: 100)
  --interval-ms N      Interval between operations in ms (default: 100)
  --artifacts-dir DIR  Artifact output directory
  --help               Show this help

Tovarisch workloads:
  idle-warmup                  Idle memory footprint after warmup
  status-json-warmup           Repeated /status calls
  status-json-network-diag     Repeated /status.json?include=network_diag

UVB-76 workloads:
  idle-warmup                  Idle memory footprint after warmup
  status-api-polling           Repeated /api/v1/status polling
  diagnostic-capture-loop      Repeated status with network_diag

Examples:
  memory-lab --service tovarisch --workload tovarisch-idle-warmup
  memory-lab --service uvb76 --workload status-api-polling --operations 200
`)
}
