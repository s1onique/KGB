// Package runner orchestrates the TCP diagnostic telemetry lab.
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-tcp-diag-telemetry-lab/internal/artifact"
	"github.com/s1onique/KGB/uvb76/cmd/uvb76-tcp-diag-telemetry-lab/internal/diagpeer"
	"github.com/s1onique/KGB/uvb76/cmd/uvb76-tcp-diag-telemetry-lab/internal/verifier"
)

const (
	LabName         = "kgb-uvb76-tcp-diag-telemetry"
	DefaultPeerPort = 18317
	CaptureTimeout  = 5 * time.Second
)

// Result captures the lab outcome.
type Result struct {
	OK                     bool   `json:"ok"`
	Mode                   string `json:"mode"`
	RequestedPath          string `json:"requested_path"`
	CapturePacketCount     int    `json:"capture_packet_count"`
	TCPTelemetryPacketCount int    `json:"tcp_telemetry_packet_count"`
	TCPRecordCount         int    `json:"tcp_record_count"`
	TCPEventCount          int    `json:"tcp_event_count"`
	TCPTelemetryExercised  bool   `json:"tcp_telemetry_exercised"`
	ArtifactDir            string `json:"artifact_dir"`
	FailureReason          string `json:"failure_reason,omitempty"`
}

// Run executes the TCP diagnostic telemetry lab with a new temp directory.
func Run() (*Result, error) {
	artifactDir, err := artifact.CreateArtifactDir(LabName)
	if err != nil {
		return nil, fmt.Errorf("failed to create artifact dir: %w", err)
	}
	return RunWithDir(artifactDir)
}

// RunWithDir executes the lab with a specific artifact directory.
func RunWithDir(artifactDir string) (*Result, error) {
	log.Printf("=== UVB-76 TCP Diagnostic Telemetry Lab ===")
	log.Printf("Artifact dir: %s", artifactDir)

	// Create artifact directory if it doesn't exist
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create artifact dir: %w", err)
	}

	// Start hermetic diagnostic peer
	peer := diagpeer.NewServer(DefaultPeerPort, true) // includeTCP=true
	if err := peer.Start(); err != nil {
		return nil, fmt.Errorf("failed to start diagnostic peer: %w", err)
	}
	defer peer.Stop()

	// Build the canonical diagnostic capture URL
	peerBaseURL := fmt.Sprintf("http://localhost:%d", DefaultPeerPort)
	statusURL := fmt.Sprintf("%s/status.json?include=network_diag", peerBaseURL)
	expectedPath := "/status.json?include=network_diag"

	// Extract path for request logging
	u, _ := url.Parse(statusURL)
	sanitizedPath := u.Path
	if u.RawQuery != "" {
		sanitizedPath += "?" + u.RawQuery
	}

	// Save the capture request
	captureReq := artifact.CaptureRequest{
		Method: "GET",
		URL:    sanitizedPath,
	}
	if err := artifact.WriteJSON(artifactDir, "capture-request.json", captureReq); err != nil {
		return buildFailureResult(artifactDir, expectedPath, fmt.Sprintf("failed to write capture-request.json: %w", err))
	}

	// Perform the diagnostic capture
	log.Printf("Performing diagnostic capture to %s...", statusURL)

	ctx, cancel := context.WithTimeout(context.Background(), CaptureTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return buildFailureResult(artifactDir, expectedPath, fmt.Sprintf("request creation failed: %v", err))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return buildFailureResult(artifactDir, expectedPath, fmt.Sprintf("request failed: %v", err))
	}
	defer resp.Body.Close()

	httpStatus := resp.StatusCode
	if httpStatus != http.StatusOK {
		return buildFailureResult(artifactDir, expectedPath, fmt.Sprintf("unexpected status code: %d", httpStatus))
	}

	// Read response body
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return buildFailureResult(artifactDir, expectedPath, fmt.Sprintf("failed to read response: %v", err))
	}

	// Parse the response
	var peerResp artifact.DiagPeerResponse
	if err := json.Unmarshal(body, &peerResp); err != nil {
		return buildFailureResult(artifactDir, expectedPath, fmt.Sprintf("failed to parse response: %v", err))
	}

	// Save the diagnostic peer response
	if err := os.WriteFile(filepath.Join(artifactDir, "diagnostic-peer-response.json"), body, 0644); err != nil {
		log.Printf("Warning: failed to write diagnostic-peer-response.json: %v", err)
	}

	// Build captured packet
	packet := artifact.BuildCapturedPacket(
		"test-peer",
		peerBaseURL,
		"ok",
		sanitizedPath,
		statusURL,
		httpStatus,
		&peerResp,
	)

	// Save the captured diagnostic packet
	if err := artifact.WriteJSON(artifactDir, "captured-diagnostic-packet.json", packet); err != nil {
		return buildFailureResult(artifactDir, expectedPath, fmt.Sprintf("failed to write captured-diagnostic-packet.json: %w", err))
	}

	// Count TCP telemetry from peer response
	tcpRecordCount := artifact.CountTCPTelemetry(&peerResp)
	tcpEventCount := artifact.CountTCPEvents(&peerResp)

	log.Printf("TCP telemetry: %d records, %d events", tcpRecordCount, tcpEventCount)

	// Run verifier to confirm evidence from captured packet
	verifierResult, err := verifier.VerifyArtifacts(artifactDir)
	if err != nil {
		return buildFailureResult(artifactDir, expectedPath, fmt.Sprintf("verifier error: %v", err))
	}

	// Save verifier result
	if err := artifact.WriteJSON(artifactDir, "verifier-result.json", verifierResult); err != nil {
		log.Printf("Warning: failed to write verifier-result.json: %v", err)
	}

	if !verifierResult.OK {
		return buildFailureResult(artifactDir, expectedPath, fmt.Sprintf("verifier rejected: %s", verifierResult.FailureReason))
	}

	// Build lab result DERIVED FROM verifier result
	tcpPacketCount := 0
	if verifierResult.TCPTelemetryPacketCount > 0 {
		tcpPacketCount = 1
	}
	labResult := artifact.BuildLabResult(
		"hermetic-diagnostic-peer",
		sanitizedPath,
		artifactDir,
		verifierResult.CapturePacketCount,
		tcpPacketCount,
		verifierResult.TCPRecordCount,
		verifierResult.TCPEventCount,
		verifierResult.TCPTelemetryExercised,
	)

	// Save lab result
	if err := artifact.WriteJSON(artifactDir, "lab-result.json", labResult); err != nil {
		return buildFailureResult(artifactDir, expectedPath, fmt.Sprintf("failed to write lab-result.json: %w", err))
	}

	// Build final result
	result := &Result{
		OK:                      labResult.OK,
		Mode:                    "hermetic-diagnostic-peer",
		RequestedPath:           sanitizedPath,
		CapturePacketCount:      verifierResult.CapturePacketCount,
		TCPTelemetryPacketCount: tcpPacketCount,
		TCPRecordCount:          verifierResult.TCPRecordCount,
		TCPEventCount:           verifierResult.TCPEventCount,
		TCPTelemetryExercised:   verifierResult.TCPTelemetryExercised,
		ArtifactDir:             artifactDir,
	}

	log.Printf("")
	log.Printf("=== Lab Result ===")
	log.Printf("OK: %v", result.OK)
	log.Printf("TCP telemetry exercised: %v", result.TCPTelemetryExercised)
	log.Printf("TCP record count: %d", verifierResult.TCPRecordCount)
	log.Printf("TCP event count: %d", verifierResult.TCPEventCount)
	log.Printf("Artifact dir: %s", artifactDir)

	return result, nil
}

// Verify runs the verifier on an existing artifact directory.
func Verify(artifactDir string) (*Result, error) {
	log.Printf("=== UVB-76 TCP Diagnostic Telemetry Lab (Verify Mode) ===")
	log.Printf("Verifying artifacts in: %s", artifactDir)

	// Run verifier
	verifierResult, err := verifier.VerifyArtifacts(artifactDir)
	if err != nil {
		return buildFailureResult(artifactDir, "/status.json?include=network_diag", fmt.Sprintf("verifier error: %v", err))
	}

	// Save verifier result
	if err := artifact.WriteJSON(artifactDir, "verifier-result.json", verifierResult); err != nil {
		log.Printf("Warning: failed to write verifier-result.json: %v", err)
	}

	if !verifierResult.OK {
		return buildFailureResult(artifactDir, "/status.json?include=network_diag", fmt.Sprintf("verifier rejected: %s", verifierResult.FailureReason))
	}

	// Build result from verifier
	tcpPacketCount := 0
	if verifierResult.TCPTelemetryPacketCount > 0 {
		tcpPacketCount = 1
	}
	result := &Result{
		OK:                      verifierResult.OK,
		Mode:                    "hermetic-diagnostic-peer",
		RequestedPath:           verifierResult.RequestedPath,
		CapturePacketCount:       verifierResult.CapturePacketCount,
		TCPTelemetryPacketCount:  tcpPacketCount,
		TCPRecordCount:           verifierResult.TCPRecordCount,
		TCPEventCount:            verifierResult.TCPEventCount,
		TCPTelemetryExercised:    verifierResult.TCPTelemetryExercised,
		ArtifactDir:              artifactDir,
	}

	log.Printf("")
	log.Printf("=== Verify Result ===")
	log.Printf("OK: %v", result.OK)
	log.Printf("TCP telemetry exercised: %v", result.TCPTelemetryExercised)
	log.Printf("TCP record count: %d", verifierResult.TCPRecordCount)
	log.Printf("TCP event count: %d", verifierResult.TCPEventCount)

	return result, nil
}

func buildFailureResult(artifactDir, requestedPath, failureReason string) (*Result, error) {
	// Create a minimal lab result that will fail verification
	labResult := &artifact.LabResult{
		OK:                      false,
		Mode:                    "hermetic-diagnostic-peer",
		RequestedPath:           requestedPath,
		CapturePacketCount:      0,
		TCPTelemetryPacketCount: 0,
		TCPRecordCount:          0,
		TCPEventCount:           0,
		TCPTelemetryExercised:  false,
		ArtifactDir:             artifactDir,
	}

	// Try to write the lab result if artifact dir exists
	if artifactDir != "" {
		artifact.WriteJSON(artifactDir, "lab-result.json", labResult)
	}

	return &Result{
		OK:                     false,
		Mode:                   "hermetic-diagnostic-peer",
		RequestedPath:          requestedPath,
		CapturePacketCount:     0,
		TCPTelemetryPacketCount: 0,
		TCPRecordCount:         0,
		TCPEventCount:          0,
		TCPTelemetryExercised:  false,
		ArtifactDir:            artifactDir,
		FailureReason:          failureReason,
	}, fmt.Errorf(failureReason)
}
