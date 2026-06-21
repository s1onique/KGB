// Package verifier provides structural verification of TCP telemetry in diagnostic packets.
package verifier

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Result captures the verification outcome for machine-readable output.
type Result struct {
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

// NetworkDiagData mirrors the production type for structural parsing.
type NetworkDiagData struct {
	StartedAt   string              `json:"started_at"`
	Status      string              `json:"status"`
	Wireguard   *WireguardDiagData `json:"wireguard,omitempty"`
	Interfaces  []InterfaceDiagData `json:"interfaces"`
	Routes      []RouteDiagData    `json:"routes"`
	UnderlayTCP []TcpSocketDiagData `json:"underlay_tcp"`
	Events      []DiagEventData    `json:"events"`
}

type WireguardDiagData struct {
	Status     string                 `json:"status"`
	Interfaces []WgInterfaceDiagData `json:"interfaces"`
}

type WgInterfaceDiagData struct {
	Name   string           `json:"name"`
	Status string           `json:"status"`
	Peers  []WgPeerDiagData `json:"peers"`
}

type WgPeerDiagData struct {
	PublicKey             string  `json:"public_key"`
	Endpoint              string  `json:"endpoint"`
	AllowedIPs            string  `json:"allowed_ips"`
	LatestHandshakeAt     *string `json:"latest_handshake_at,omitempty"`
	LatestHandshakeAgeSec *int64  `json:"latest_handshake_age_seconds,omitempty"`
	TransferRxBytes       int64   `json:"transfer_rx_bytes"`
	TransferTxBytes       int64   `json:"transfer_tx_bytes"`
}

type InterfaceDiagData struct {
	Name      string `json:"name"`
	OperState string `json:"operstate"`
	Carrier   *bool  `json:"carrier,omitempty"`
	RxBytes   int64  `json:"rx_bytes"`
	TxBytes   int64  `json:"tx_bytes"`
	RxPackets int64  `json:"rx_packets"`
	TxPackets int64  `json:"tx_packets"`
	RxErrors  int64  `json:"rx_errors"`
	TxErrors  int64  `json:"tx_errors"`
	RxDropped int64  `json:"rx_dropped"`
	TxDropped int64  `json:"tx_dropped"`
}

type RouteDiagData struct {
	Target    string  `json:"target"`
	Interface string  `json:"interface"`
	Source    string  `json:"source"`
	Gateway   *string `json:"gateway,omitempty"`
	Status    string  `json:"status"`
}

type TcpSocketDiagData struct {
	Name           string   `json:"name"`
	State          string   `json:"state"`
	Local          string   `json:"local"`
	Remote         string   `json:"remote"`
	RTTMs          *float64 `json:"rtt_ms,omitempty"`
	RTTVarMs       *float64 `json:"rttvar_ms,omitempty"`
	RTOMs          *int64   `json:"rto_ms,omitempty"`
	Retransmits    *int64   `json:"retransmits,omitempty"`
	Unacked        *int64   `json:"unacked,omitempty"`
	Cwnd           *int32   `json:"cwnd,omitempty"`
	SendQueueBytes *int64   `json:"send_queue_bytes,omitempty"`
	RecvQueueBytes *int64   `json:"recv_queue_bytes,omitempty"`
	Status         string   `json:"status"`
}

type DiagEventData struct {
	Timestamp string  `json:"ts"`
	Severity  string  `json:"severity"`
	Source    string  `json:"source"`
	Message   string  `json:"message"`
	Fields    *string `json:"fields,omitempty"`
}

// TovarischStatusResponse is the expected response format from tovarisch.
type TovarischStatusResponse struct {
	NetworkDiag *NetworkDiagData `json:"network_diag,omitempty"`
}

// CapturePacket represents a diagnostic capture packet artifact.
type CapturePacket struct {
	Source               string          `json:"source"`
	BaseURL              string          `json:"base_url"`
	CaptureStartedAt     string          `json:"capture_started_at"`
	CaptureFinishedAt    string          `json:"capture_finished_at,omitempty"`
	DurationMs           *int64          `json:"duration_ms,omitempty"`
	Status               string          `json:"status"`
	Error                *string         `json:"error,omitempty"`
	NetworkDiag          *NetworkDiagData `json:"network_diag,omitempty"`
	RequestedPath        *string         `json:"requested_path,omitempty"`
	EffectiveCaptureURL  string          `json:"effective_capture_url,omitempty"`
	HTTPStatusCode       *int            `json:"http_status_code,omitempty"`
}

// VerifyArtifacts verifies that the artifact directory contains proper TCP telemetry evidence.
// It performs structural validation and derives tcp_telemetry_exercised from parsed packet evidence.
func VerifyArtifacts(dir string) (Result, error) {
	result := Result{Mode: "hermetic-diagnostic-peer"}

	// Check artifact directory exists
	if dir == "" {
		result.OK = false
		result.FailureReason = "artifact directory is empty"
		return result, nil
	}

	// Verify directory is accessible
	if _, err := os.ReadDir(dir); err != nil {
		result.OK = false
		result.FailureReason = fmt.Sprintf("failed to read artifact dir: %v", err)
		return result, err
	}

	// Load capture-request.json to verify the requested path
	captureReq, err := loadCaptureRequest(dir)
	if err != nil {
		result.OK = false
		result.FailureReason = fmt.Sprintf("failed to load capture-request.json: %v", err)
		return result, nil
	}

	// Verify requested path is correct
	expectedPath := "/status.json?include=network_diag"
	if captureReq.URL != expectedPath {
		result.OK = false
		result.FailureReason = fmt.Sprintf("capture request path is '%s', expected '%s'", captureReq.URL, expectedPath)
		return result, nil
	}
	result.RequestedPath = expectedPath

	// Load captured-diagnostic-packet.json
	packet, err := loadCapturePacket(dir)
	if err != nil {
		result.OK = false
		result.FailureReason = fmt.Sprintf("failed to load captured-diagnostic-packet.json: %v", err)
		return result, nil
	}

	// Verify at least one packet exists
	if packet == nil {
		result.OK = false
		result.FailureReason = "no diagnostic packet artifact found"
		return result, nil
	}
	result.CapturePacketCount = 1

	// Verify network_diag exists
	if packet.NetworkDiag == nil {
		result.OK = false
		result.FailureReason = "packet has no network_diag field"
		return result, nil
	}

	// CRITICAL: Verify TCP telemetry in underlay_tcp with valid required fields
	// This is the core invariant: tcp_telemetry_exercised requires typed TCP records, not just events
	ok, reason := VerifyCapturePacket(packet)
	if !ok {
		result.OK = false
		result.FailureReason = reason
		return result, nil
	}

	// Count valid TCP records and events
	tcpRecords := packet.NetworkDiag.UnderlayTCP
	tcpEvents := countTCPEvents(packet.NetworkDiag)

	// Count only records that satisfy hasRequiredTCPTelemetryFields
	validRecordCount := 0
	for _, tcp := range tcpRecords {
		if hasRequiredTCPTelemetryFields(&tcp) {
			validRecordCount++
		}
	}

	result.TCPRecordCount = validRecordCount
	result.TCPEventCount = tcpEvents
	result.TCPTelemetryPacketCount = 1
	result.TCPTelemetryExercised = true
	result.OK = true

	return result, nil
}

// countTCPEvents counts TCP-related events in the network_diag.
func countTCPEvents(nd *NetworkDiagData) int {
	count := 0
	for _, event := range nd.Events {
		if event.Source == "underlay_tcp" {
			count++
		}
	}
	return count
}

// CaptureRequest represents the captured HTTP request details.
type CaptureRequest struct {
	Method string `json:"method"`
	URL    string `json:"url"`
}

// loadCaptureRequest loads and parses capture-request.json.
func loadCaptureRequest(dir string) (*CaptureRequest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "capture-request.json"))
	if err != nil {
		return nil, err
	}

	var req CaptureRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("malformed JSON: %w", err)
	}

	return &req, nil
}

// loadCapturePacket loads and parses captured-diagnostic-packet.json.
func loadCapturePacket(dir string) (*CapturePacket, error) {
	data, err := os.ReadFile(filepath.Join(dir, "captured-diagnostic-packet.json"))
	if err != nil {
		return nil, err
	}

	var packet CapturePacket
	if err := json.Unmarshal(data, &packet); err != nil {
		return nil, fmt.Errorf("malformed JSON: %w", err)
	}

	return &packet, nil
}

// VerifyCapturePacket structurally verifies a single capture packet for TCP telemetry.
// Returns (true, "") if valid TCP telemetry is found, (false, reason) otherwise.
func VerifyCapturePacket(packet *CapturePacket) (bool, string) {
	// Must have network_diag
	if packet.NetworkDiag == nil {
		return false, "no network_diag field"
	}

	// Must have underlay_tcp with at least one record
	if len(packet.NetworkDiag.UnderlayTCP) == 0 {
		return false, "network_diag has no underlay_tcp records"
	}

	// Verify at least one TCP record has required fields (name, state, local, remote)
	// This ensures we're verifying the CONTENTS of the TCP records, not just their presence
	validRecordFound := false
	for _, tcp := range packet.NetworkDiag.UnderlayTCP {
		if hasRequiredTCPTelemetryFields(&tcp) {
			validRecordFound = true
			break
		}
	}

	if !validRecordFound {
		return false, "no underlay_tcp record has required fields (name, state, local, remote)"
	}

	return true, ""
}

// hasRequiredTCPTelemetryFields checks if a TCP socket record has all required fields.
// A valid TCP record must have: name, state, local, remote
func hasRequiredTCPTelemetryFields(tcp *TcpSocketDiagData) bool {
	return tcp != nil &&
		tcp.Name != "" &&
		tcp.State != "" &&
		tcp.Local != "" &&
		tcp.Remote != ""
}

// ContainsTCPTelemetry checks if the given JSON contains TCP telemetry in the expected location.
// This is used for test fixtures only - the real verifier uses structural parsing.
func ContainsTCPTelemetry(jsonData []byte) bool {
	// Structural check: must parse as valid JSON
	var resp TovarischStatusResponse
	if err := json.Unmarshal(jsonData, &resp); err != nil {
		return false
	}

	// Must have network_diag
	if resp.NetworkDiag == nil {
		return false
	}

	// Must have underlay_tcp with at least one valid record
	for _, tcp := range resp.NetworkDiag.UnderlayTCP {
		if hasRequiredTCPTelemetryFields(&tcp) {
			return true
		}
	}
	return false
}

// IsValidTCPPayload checks if the given JSON is a valid tovarisch status response with TCP telemetry.
func IsValidTCPPayload(jsonData []byte) bool {
	return ContainsTCPTelemetry(jsonData)
}

// SanitizePath extracts the path+query from a full URL for logging.
func SanitizePath(fullURL string) string {
	// Simple extraction - just get the path portion
	parts := strings.SplitN(fullURL, "?", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return fullURL
}
