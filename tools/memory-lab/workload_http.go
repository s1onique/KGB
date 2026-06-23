// workload_http.go — HTTP workload execution for memory labs
//
// Executes HTTP requests against tovarisch or uvb76 endpoints.
// Tracks errors and timing for artifact metadata.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package main

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPWorkloadResult contains the result of an HTTP workload run.
type HTTPWorkloadResult struct {
	Operations int
	Errors     int
	DurationMs int64
}

// HTTPWorkloadConfig configures an HTTP workload.
type HTTPWorkloadConfig struct {
	URL         string
	Operations  int
	IntervalMs  int
	Name        string
	Client      *http.Client // Optional custom client (for TLS-aware UVB-76)
}

// RunHTTPWorkload executes an HTTP workload with controlled interval.
func RunHTTPWorkload(cfg HTTPWorkloadConfig) HTTPWorkloadResult {
	var errors int
	start := time.Now()

	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}

	for i := 0; i < cfg.Operations; i++ {
		if err := fetchURLWithClient(client, cfg.URL); err != nil {
			errors++
		}

		// Sleep between operations (not after last)
		if i < cfg.Operations-1 && cfg.IntervalMs > 0 {
			time.Sleep(time.Duration(cfg.IntervalMs) * time.Millisecond)
		}
	}

	return HTTPWorkloadResult{
		Operations: cfg.Operations,
		Errors:     errors,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

// fetchURLWithClient performs a single HTTP GET request with a custom client.
func fetchURLWithClient(client *http.Client, url string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Drain body to allow connection reuse
	_, err = io.Copy(io.Discard, resp.Body)
	return err
}

// WorkloadType defines available workload types.
type WorkloadType string

const (
	// Tovarisch workloads
	WorkloadTovarischIdle              WorkloadType = "tovarisch-idle-warmup"
	WorkloadTovarischStatusJSON        WorkloadType = "status-json-warmup"
	WorkloadTovarischStatusJSONNetDiag WorkloadType = "status-json-network-diag"

	// UVB-76 workloads
	WorkloadUVB76Idle                  WorkloadType = "uvb76-idle-warmup"
	WorkloadUVB76StatusAPIPolling      WorkloadType = "status-api-polling"
	WorkloadUVB76DiagnosticCaptureLoop WorkloadType = "diagnostic-capture-loop"
)

// ServiceConfig holds service-specific configuration.
type ServiceConfig struct {
	Binary       string
	Port         int
	BaseURL      string
	IsTovarisch  bool
}

// TovarischWorkloadURLs returns URLs for tovarisch workloads.
func TovarischWorkloadURLs(port int) map[WorkloadType]string {
	return map[WorkloadType]string{
		WorkloadTovarischIdle:              fmt.Sprintf("http://127.0.0.1:%d/", port),
		WorkloadTovarischStatusJSON:        fmt.Sprintf("http://127.0.0.1:%d/status", port),
		WorkloadTovarischStatusJSONNetDiag: fmt.Sprintf("http://127.0.0.1:%d/status.json?include=network_diag", port),
	}
}

// UVB76WorkloadURLs returns URLs for uvb76 workloads.
// UVB-76 uses HTTPS with self-signed localhost cert.
func UVB76WorkloadURLs(port int) map[WorkloadType]string {
	return map[WorkloadType]string{
		WorkloadUVB76Idle:                  fmt.Sprintf("https://127.0.0.1:%d/", port),
		WorkloadUVB76StatusAPIPolling:      fmt.Sprintf("https://127.0.0.1:%d/api/v1/status", port),
		WorkloadUVB76DiagnosticCaptureLoop: fmt.Sprintf("https://127.0.0.1:%d/api/v1/status?include=network_diag", port),
	}
}

// EndpointFor returns the endpoint path for a workload type.
func EndpointFor(wt WorkloadType) string {
	switch wt {
	case WorkloadTovarischStatusJSON, WorkloadUVB76StatusAPIPolling:
		return "/status"
	case WorkloadTovarischStatusJSONNetDiag, WorkloadUVB76DiagnosticCaptureLoop:
		return "/status.json?include=network_diag"
	default:
		return "/"
	}
}
