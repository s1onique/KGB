package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestVerifyPassFixture tests the verifier against a passing fixture.
func TestVerifyPassFixture(t *testing.T) {
	// Create temp artifact directory
	dir, err := os.MkdirTemp("", "verify-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	// Create passing summary.json
	summary := Summary{
		Status:        "pass",
		StartedAt:     "2024-01-01T00:00:00Z",
		CompletedAt:   "2024-01-01T00:01:00Z",
		DurationSecs:  60,
		Workers:       8,
		RequestCount:  100,
		SuccessCount:  100,
		ErrorCount:    0,
		Mode:          "https-http2-default",
		HTTP2Disabled: false,
		ProcessExited: false,
		SawSIGSEGV:   false,
		SawPanic:     false,
		SawFatalError: false,
		ArtifactDir:   dir,
	}

	summaryData, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), summaryData, 0644); err != nil {
		t.Fatalf("Failed to write summary: %v", err)
	}

	// Create config.json with diagnostics.peers[0].effective_capture_url
	configData := []byte(`{
  "listen": {"addr": ":19443"},
  "auth": {"username": "admin"},
  "diagnostics": {
    "peers": [
      {
        "name": "diag-peer-home",
        "effective_capture_url": "http://127.0.0.1:19980/status.json?include=network_diag"
      }
    ]
  }
}`)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), configData, 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Create cert.pem and key.pem
	certData := []byte(`-----BEGIN CERTIFICATE-----
TEST CERTIFICATE
-----END CERTIFICATE-----`)
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), certData, 0644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	keyData := []byte(`-----BEGIN RSA PRIVATE KEY-----
TEST KEY
-----END RSA PRIVATE KEY-----`)
	if err := os.WriteFile(filepath.Join(dir, "key.pem"), keyData, 0644); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	// Create log files
	logData := []byte("2024/01/01 00:00:00 Starting server on :19443")
	if err := os.WriteFile(filepath.Join(dir, "uvb76.stdout.log"), logData, 0644); err != nil {
		t.Fatalf("Failed to write stdout log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "uvb76.stderr.log"), logData, 0644); err != nil {
		t.Fatalf("Failed to write stderr log: %v", err)
	}

	// Create sample response with diagnostic target
	targets := []TargetResponse{
		{
			ID:                  "target-with-diag",
			Name:                "Target With Diagnostic Peer",
			DiagnosticPeerName:  "diag-peer-home",
			DiagnosticBaseURL:   "http://127.0.0.1:19980",
			EffectiveCaptureURL: "http://127.0.0.1:19980/status.json?include=network_diag",
		},
		{
			ID:   "target-plain",
			Name: "Target Without Diagnostic",
		},
	}
	sampleData, _ := json.MarshalIndent(targets, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "targets-response-sample.json"), sampleData, 0644); err != nil {
		t.Fatalf("Failed to write sample: %v", err)
	}

	// Create workload summary
	workloadData := []byte(`{"workers":8,"success_count":100,"error_count":0,"request_count":100}`)
	if err := os.WriteFile(filepath.Join(dir, "workload-summary.json"), workloadData, 0644); err != nil {
		t.Fatalf("Failed to write workload summary: %v", err)
	}

	// Run verifier
	if err := verify(dir); err != nil {
		t.Errorf("Verifier failed for passing fixture: %v", err)
	}
}

// TestVerifyFailSIGSEGV tests fail-closed behavior for SIGSEGV.
func TestVerifyFailSIGSEGV(t *testing.T) {
	dir, err := os.MkdirTemp("", "verify-fail-sigsegv-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	// Create summary with saw_sigsegv=true
	summary := Summary{
		Status:        "fail",
		StartedAt:     "2024-01-01T00:00:00Z",
		CompletedAt:   "2024-01-01T00:01:00Z",
		DurationSecs:  60,
		Workers:       8,
		RequestCount:  0,
		SuccessCount:  0,
		ErrorCount:    1,
		Mode:          "https-http2-default",
		HTTP2Disabled: false,
		ProcessExited: true,
		SawSIGSEGV:   true,
		SawPanic:     false,
		SawFatalError: false,
		ArtifactDir:   dir,
	}

	summaryData, _ := json.MarshalIndent(summary, "", "  ")
	os.WriteFile(filepath.Join(dir, "summary.json"), summaryData, 0644)
	os.MkdirAll(filepath.Join(dir, "targets-response-sample.json"), 0755) // empty dir to pass file check

	// Should fail
	if err := verify(dir); err == nil {
		t.Error("Expected verifier to fail for SIGSEGV, but it passed")
	}
}

// TestVerifyFailZeroRequests tests fail-closed behavior for zero requests.
func TestVerifyFailZeroRequests(t *testing.T) {
	dir, err := os.MkdirTemp("", "verify-fail-zero-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	// Create summary with zero requests
	summary := Summary{
		Status:       "pass", // Intentionally wrong status
		StartedAt:    "2024-01-01T00:00:00Z",
		CompletedAt:  "2024-01-01T00:01:00Z",
		DurationSecs: 60,
		Workers:      8,
		RequestCount: 0, // Zero requests!
		SuccessCount: 0,
		ErrorCount:   0,
		Mode:         "https-http2-default",
		ArtifactDir:  dir,
	}

	summaryData, _ := json.MarshalIndent(summary, "", "  ")
	os.WriteFile(filepath.Join(dir, "summary.json"), summaryData, 0644)

	// Create required artifacts
	for _, f := range []string{"config.json", "cert.pem", "key.pem", "uvb76.stdout.log", "uvb76.stderr.log"} {
		os.WriteFile(filepath.Join(dir, f), []byte("test"), 0644)
	}

	// Create sample with diagnostic fields
	targets := []TargetResponse{
		{
			ID:                  "target-with-diag",
			DiagnosticPeerName:  "diag-peer",
			DiagnosticBaseURL:   "http://localhost",
			EffectiveCaptureURL: "http://localhost/status.json",
		},
		{ID: "target-plain"},
	}
	sampleData, _ := json.MarshalIndent(targets, "", "  ")
	os.WriteFile(filepath.Join(dir, "targets-response-sample.json"), sampleData, 0644)
	os.WriteFile(filepath.Join(dir, "workload-summary.json"), []byte("{}"), 0644)

	// Should fail due to zero request_count
	if err := verify(dir); err == nil {
		t.Error("Expected verifier to fail for zero requests, but it passed")
	}
}

// TestVerifyFailMissingCaptureURL tests fail-closed behavior for missing effective_capture_url.
func TestVerifyFailMissingCaptureURL(t *testing.T) {
	dir, err := os.MkdirTemp("", "verify-fail-captureurl-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	// Create summary
	summary := Summary{
		Status:       "pass",
		StartedAt:    "2024-01-01T00:00:00Z",
		CompletedAt:  "2024-01-01T00:01:00Z",
		DurationSecs: 60,
		Workers:      8,
		RequestCount: 100,
		SuccessCount: 100,
		ErrorCount:   0,
		Mode:         "https-http2-default",
		ArtifactDir:  dir,
	}

	summaryData, _ := json.MarshalIndent(summary, "", "  ")
	os.WriteFile(filepath.Join(dir, "summary.json"), summaryData, 0644)

	// Create required artifacts
	for _, f := range []string{"config.json", "cert.pem", "key.pem", "uvb76.stdout.log", "uvb76.stderr.log"} {
		os.WriteFile(filepath.Join(dir, f), []byte("test"), 0644)
	}

	// Create sample with EMPTY effective_capture_url
	targets := []TargetResponse{
		{
			ID:                  "target-with-diag",
			DiagnosticPeerName:  "diag-peer",
			DiagnosticBaseURL:   "http://localhost",
			EffectiveCaptureURL: "", // EMPTY - should fail
		},
		{ID: "target-plain"},
	}
	sampleData, _ := json.MarshalIndent(targets, "", "  ")
	os.WriteFile(filepath.Join(dir, "targets-response-sample.json"), sampleData, 0644)
	os.WriteFile(filepath.Join(dir, "workload-summary.json"), []byte("{}"), 0644)

	// Should fail due to empty EffectiveCaptureURL
	if err := verify(dir); err == nil {
		t.Error("Expected verifier to fail for missing EffectiveCaptureURL, but it passed")
	}
}

// TestVerifyFailMalformedJSON tests fail-closed behavior for malformed sample response.
func TestVerifyFailMalformedJSON(t *testing.T) {
	dir, err := os.MkdirTemp("", "verify-fail-malformed-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	// Create summary
	summary := Summary{
		Status:       "pass",
		StartedAt:    "2024-01-01T00:00:00Z",
		CompletedAt:  "2024-01-01T00:01:00Z",
		DurationSecs: 60,
		Workers:      8,
		RequestCount: 100,
		SuccessCount: 100,
		ErrorCount:   0,
		Mode:         "https-http2-default",
		ArtifactDir:  dir,
	}

	summaryData, _ := json.MarshalIndent(summary, "", "  ")
	os.WriteFile(filepath.Join(dir, "summary.json"), summaryData, 0644)

	// Create required artifacts
	for _, f := range []string{"config.json", "cert.pem", "key.pem", "uvb76.stdout.log", "uvb76.stderr.log"} {
		os.WriteFile(filepath.Join(dir, f), []byte("test"), 0644)
	}

	// Create MALFORMED JSON sample
	malformed := []byte(`{ "id": "target-with-diag", invalid json }`)
	os.WriteFile(filepath.Join(dir, "targets-response-sample.json"), malformed, 0644)
	os.WriteFile(filepath.Join(dir, "workload-summary.json"), []byte("{}"), 0644)

	// Should fail due to malformed JSON
	if err := verify(dir); err == nil {
		t.Error("Expected verifier to fail for malformed JSON, but it passed")
	}
}

// TestVerifyFailNonPassStatus tests fail-closed behavior for non-pass status.
func TestVerifyFailNonPassStatus(t *testing.T) {
	dir, err := os.MkdirTemp("", "verify-fail-status-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	// Create summary with status="fail"
	summary := Summary{
		Status:       "fail", // Not "pass"
		StartedAt:    "2024-01-01T00:00:00Z",
		CompletedAt:  "2024-01-01T00:01:00Z",
		DurationSecs: 60,
		Workers:      8,
		RequestCount: 100,
		SuccessCount: 100,
		ErrorCount:   0,
		Mode:         "https-http2-default",
		ArtifactDir:  dir,
	}

	summaryData, _ := json.MarshalIndent(summary, "", "  ")
	os.WriteFile(filepath.Join(dir, "summary.json"), summaryData, 0644)

	// Create required artifacts
	for _, f := range []string{"config.json", "cert.pem", "key.pem", "uvb76.stdout.log", "uvb76.stderr.log"} {
		os.WriteFile(filepath.Join(dir, f), []byte("test"), 0644)
	}

	// Create sample with diagnostic fields
	targets := []TargetResponse{
		{
			ID:                  "target-with-diag",
			DiagnosticPeerName:  "diag-peer",
			DiagnosticBaseURL:   "http://localhost",
			EffectiveCaptureURL: "http://localhost/status.json",
		},
		{ID: "target-plain"},
	}
	sampleData, _ := json.MarshalIndent(targets, "", "  ")
	os.WriteFile(filepath.Join(dir, "targets-response-sample.json"), sampleData, 0644)
	os.WriteFile(filepath.Join(dir, "workload-summary.json"), []byte("{}"), 0644)

	// Should fail due to status != "pass"
	if err := verify(dir); err == nil {
		t.Error("Expected verifier to fail for non-pass status, but it passed")
	}
}
