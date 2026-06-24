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
	
	// Attribution-specific flags
	attributionDuration := fs.Int("attribution-duration", 600, "Attribution lab duration in seconds (default: 600 for 10 min)")
	attributionSampleInterval := fs.Int("attribution-sample-ms", 5000, "Attribution RSS/PSS sampling interval in ms")
	
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
	
	// Check if this is an attribution workload
	if isAttributionWorkload(wt) {
		// Attribution workloads require uvb76
		if *service != "uvb76" {
			return fmt.Errorf("attribution workloads are only supported for uvb76")
		}
		
		// Determine duration based on workload type (can be overridden by --attribution-duration flag)
		durationSecs := *attributionDuration
		sampleIntervalMs := *attributionSampleInterval
		
		switch wt {
		case WorkloadUVB76AttributionSoak30:
			if *attributionDuration == 600 { // Default was not changed
				durationSecs = 30 * 60 // 30 minutes
				sampleIntervalMs = 10000
			}
		case WorkloadUVB76AttributionSoak60:
			if *attributionDuration == 600 { // Default was not changed
				durationSecs = 60 * 60 // 60 minutes
				sampleIntervalMs = 10000
			}
		}
		
		attrCfg := AttributionConfig{
			DurationSeconds:   durationSecs,
			SampleIntervalMs: sampleIntervalMs,
		}
		
		fmt.Printf("Attribution mode: duration=%ds (%dm), sample_interval=%dms\n", 
			attrCfg.DurationSeconds, attrCfg.DurationSeconds/60, attrCfg.SampleIntervalMs)
		_, err := RunAttribution(cfg, attrCfg)
		return err
	}
	
	// Run the lab
	_, err := Run(cfg)
	return err
}

// isAttributionWorkload returns true if the workload type is an attribution workload.
func isAttributionWorkload(wt WorkloadType) bool {
	return wt == WorkloadUVB76Attribution ||
		wt == WorkloadUVB76AttributionSoak30 ||
		wt == WorkloadUVB76AttributionSoak60
}

func validWorkload(service string, wt WorkloadType) bool {
	switch service {
	case "tovarisch":
		return wt == WorkloadTovarischIdle ||
			wt == WorkloadTovarischStatusJSON ||
			wt == WorkloadTovarischStatusJSONNetDiag ||
			wt == WorkloadTovarischLeakSlope ||
			wt == WorkloadTovarischLeakSlopeNetDiag
	case "uvb76":
		return wt == WorkloadUVB76Idle ||
			wt == WorkloadUVB76StatusAPIPolling ||
			wt == WorkloadUVB76DiagnosticCaptureLoop ||
			wt == WorkloadUVB76LeakSlope ||
			wt == WorkloadUVB76LeakSlopeNetDiag ||
			wt == WorkloadUVB76Attribution ||
			wt == WorkloadUVB76AttributionSoak30 ||
			wt == WorkloadUVB76AttributionSoak60
	default:
		return false
	}
}

func printUsage() {
	fmt.Print(`memory-lab — Go-based memory lab runner

Usage: memory-lab [flags]

Flags:
  --service NAME              Service to test: tovarisch or uvb76 (required)
  --workload TYPE             Workload type (required)
  --binary PATH               Path to service binary
  --config PATH               Path to config file (uvb76 only)
  --port PORT                 Listen port (default: 18080/18081)
  --warmup-secs N             Warmup period in seconds (default: 60)
  --operations N              Number of HTTP operations (default: 100)
  --interval-ms N             Interval between operations in ms (default: 100)
  --artifacts-dir DIR         Artifact output directory
  --attribution-duration N    Attribution lab duration in seconds (default: 600)
  --attribution-sample-ms N   Attribution RSS/PSS sampling interval in ms (default: 5000)
  --help                      Show this help

Tovarisch workloads:
  tovarisch-idle-warmup                  Idle memory footprint after warmup
  status-json-warmup                     Repeated /status calls
  status-json-network-diag               Repeated /status.json?include=network_diag

UVB-76 workloads:
  uvb76-idle-warmup                      Idle memory footprint after warmup
  status-api-polling                     Repeated /api/v1/status polling
  diagnostic-capture-loop                 Repeated status with network_diag
  uvb76-leak-slope                       Leak slope measurement (short window)
  uvb76-attribution                      Long-running attribution (default: 10 min)
  uvb76-attribution-30min                Long-running attribution (30 min)
  uvb76-attribution-60min                Long-running attribution (60 min)

Attribution labs capture:
  - Forced-GC memstats at start/midpoint/end checkpoints
  - pprof heap profiles at each checkpoint
  - Goroutine dumps at each checkpoint
  - RSS/PSS samples over time
  - YAML manifest with metadata

Examples:
  memory-lab --service tovarisch --workload tovarisch-idle-warmup
  memory-lab --service uvb76 --workload status-api-polling --operations 200
  memory-lab --service uvb76 --workload uvb76-attribution --attribution-duration 600
  memory-lab --service uvb76 --workload uvb76-attribution --attribution-duration 1800
`)
}
