// Package artifact provides helpers for writing lab artifacts.
package artifact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LabResult captures the lab outcome for machine-readable output.
type LabResult struct {
	OK                      bool   `json:"ok"`
	Mode                    string `json:"mode"`
	RequestedPath           string `json:"requested_path"`
	CapturePacketCount      int    `json:"capture_packet_count"`
	TCPTelemetryPacketCount int    `json:"tcp_telemetry_packet_count"`
	TCPRecordCount         int    `json:"tcp_record_count"`
	TCPEventCount           int    `json:"tcp_event_count"`
	TCPTelemetryExercised   bool   `json:"tcp_telemetry_exercised"`
	ArtifactDir             string `json:"artifact_dir"`
}

// CaptureRequest represents the captured HTTP request details.
type CaptureRequest struct {
	Method string `json:"method"`
	URL    string `json:"url"`
}

// DiagPeerResponse represents the response from the diagnostic peer.
type DiagPeerResponse struct {
	Service     string `json:"service"`
	Version     string `json:"version"`
	NodeID      string `json:"node_id"`
	Status      string `json:"status"`
	NetworkDiag *struct {
		StartedAt   string `json:"started_at"`
		Status      string `json:"status"`
		UnderlayTCP []struct {
			Name           string  `json:"name"`
			State          string  `json:"state"`
			Local          string  `json:"local"`
			Remote         string  `json:"remote"`
			RTTMs          *float64 `json:"rtt_ms,omitempty"`
			RTTVarMs       *float64 `json:"rttvar_ms,omitempty"`
			RTOMs          *int64   `json:"rto_ms,omitempty"`
			Retransmits    *int64   `json:"retransmits,omitempty"`
			Unacked        *int64   `json:"unacked,omitempty"`
			Cwnd           *int32   `json:"cwnd,omitempty"`
			SendQueueBytes *int64   `json:"send_queue_bytes,omitempty"`
			RecvQueueBytes *int64   `json:"recv_queue_bytes,omitempty"`
			Status         string   `json:"status"`
		} `json:"underlay_tcp"`
		Events []struct {
			Timestamp string  `json:"ts"`
			Severity  string  `json:"severity"`
			Source    string  `json:"source"`
			Message   string  `json:"message"`
			Fields    *string `json:"fields,omitempty"`
		} `json:"events"`
	} `json:"network_diag,omitempty"`
}

// CapturedPacket represents a diagnostic capture packet.
type CapturedPacket struct {
	Source               string `json:"source"`
	BaseURL              string `json:"base_url"`
	CaptureStartedAt     string `json:"capture_started_at"`
	CaptureFinishedAt    string `json:"capture_finished_at,omitempty"`
	DurationMs           *int64 `json:"duration_ms,omitempty"`
	Status               string `json:"status"`
	Error                *string `json:"error,omitempty"`
	NetworkDiag          *struct {
		StartedAt   string `json:"started_at"`
		Status      string `json:"status"`
		UnderlayTCP []struct {
			Name           string  `json:"name"`
			State          string  `json:"state"`
			Local          string  `json:"local"`
			Remote         string  `json:"remote"`
			RTTMs          *float64 `json:"rtt_ms,omitempty"`
			RTTVarMs       *float64 `json:"rttvar_ms,omitempty"`
			RTOMs          *int64   `json:"rto_ms,omitempty"`
			Retransmits    *int64   `json:"retransmits,omitempty"`
			Unacked        *int64   `json:"unacked,omitempty"`
			Cwnd           *int32   `json:"cwnd,omitempty"`
			SendQueueBytes *int64   `json:"send_queue_bytes,omitempty"`
			RecvQueueBytes *int64   `json:"recv_queue_bytes,omitempty"`
			Status         string   `json:"status"`
		} `json:"underlay_tcp"`
		Events []struct {
			Timestamp string  `json:"ts"`
			Severity  string  `json:"severity"`
			Source    string  `json:"source"`
			Message   string  `json:"message"`
			Fields    *string `json:"fields,omitempty"`
		} `json:"events"`
	} `json:"network_diag,omitempty"`
	RequestedPath       *string `json:"requested_path,omitempty"`
	EffectiveCaptureURL string  `json:"effective_capture_url,omitempty"`
	HTTPStatusCode      *int    `json:"http_status_code,omitempty"`
}

// VerifierResult represents the verifier's output.
type VerifierResult struct {
	OK                     bool   `json:"ok"`
	Mode                   string `json:"mode"`
	RequestedPath          string `json:"requested_path"`
	CapturePacketCount     int    `json:"capture_packet_count"`
	TCPTelemetryPacketCount int    `json:"tcp_telemetry_packet_count"`
	TCPRecordCount         int    `json:"tcp_record_count"`
	TCPEventCount          int    `json:"tcp_event_count"`
	TCPTelemetryExercised  bool   `json:"tcp_telemetry_exercised"`
	FailureReason          string `json:"failure_reason,omitempty"`
}

// WriteJSON writes a JSON file to the artifact directory.
func WriteJSON(dir, filename string, data interface{}) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", filename, err)
	}

	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filename, err)
	}

	return nil
}

// CreateArtifactDir creates a unique temp directory for lab artifacts.
func CreateArtifactDir(name string) (string, error) {
	dir, err := os.MkdirTemp("/tmp", name+"-*")
	if err != nil {
		return "", fmt.Errorf("mkdtemp: %w", err)
	}
	return dir, nil
}

// CountTCPTelemetry counts TCP telemetry records in a response.
func CountTCPTelemetry(resp *DiagPeerResponse) int {
	if resp == nil || resp.NetworkDiag == nil {
		return 0
	}
	return len(resp.NetworkDiag.UnderlayTCP)
}

// CountTCPEvents counts TCP-related events in a response.
func CountTCPEvents(resp *DiagPeerResponse) int {
	if resp == nil || resp.NetworkDiag == nil {
		return 0
	}
	count := 0
	for _, event := range resp.NetworkDiag.Events {
		if event.Source == "underlay_tcp" {
			count++
		}
	}
	return count
}

// BuildLabResult builds the final lab result from evidence.
func BuildLabResult(mode, requestedPath, artifactDir string, captureCount int, tcpPacketCount int, tcpRecordCount int, tcpEventCount int, tcpExercised bool) *LabResult {
	return &LabResult{
		OK:                      tcpExercised && captureCount > 0,
		Mode:                    mode,
		RequestedPath:           requestedPath,
		CapturePacketCount:      captureCount,
		TCPTelemetryPacketCount: tcpPacketCount,
		TCPRecordCount:         tcpRecordCount,
		TCPEventCount:           tcpEventCount,
		TCPTelemetryExercised:   tcpExercised,
		ArtifactDir:             artifactDir,
	}
}

// BuildCapturedPacket builds a captured packet from a response.
func BuildCapturedPacket(source, baseURL, status, requestedPath, effectiveURL string, httpStatus int, resp *DiagPeerResponse) *CapturedPacket {
	now := time.Now().UTC()
	finished := now
	duration := int64(100) // ms

	packet := &CapturedPacket{
		Source:               source,
		BaseURL:              baseURL,
		CaptureStartedAt:     now.Format(time.RFC3339),
		CaptureFinishedAt:    finished.Format(time.RFC3339),
		DurationMs:           &duration,
		Status:               status,
		RequestedPath:        &requestedPath,
		EffectiveCaptureURL:  effectiveURL,
		HTTPStatusCode:       &httpStatus,
	}

	if resp != nil && resp.NetworkDiag != nil {
		packet.NetworkDiag = resp.NetworkDiag
	}

	return packet
}

// LoadLabResult loads and parses lab-result.json from an artifact directory.
func LoadLabResult(dir string) (*LabResult, error) {
	data, err := os.ReadFile(filepath.Join(dir, "lab-result.json"))
	if err != nil {
		return nil, err
	}

	var result LabResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("malformed JSON: %w", err)
	}

	return &result, nil
}
