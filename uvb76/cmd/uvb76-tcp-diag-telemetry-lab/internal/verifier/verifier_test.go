package verifier

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyArtifacts_PassTCP(t *testing.T) {
	dir := filepath.Join("testdata", "pass_tcp_telemetry")
	result, err := VerifyArtifacts(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.OK {
		t.Errorf("expected OK=true, got false. Failure reason: %s", result.FailureReason)
	}
	if !result.TCPTelemetryExercised {
		t.Error("expected TCPTelemetryExercised=true")
	}
	if result.TCPRecordCount == 0 {
		t.Error("expected TCPRecordCount > 0")
	}
}

func TestVerifyArtifacts_FailNoTCPTelemetry(t *testing.T) {
	dir := filepath.Join("testdata", "fail_no_tcp_telemetry")
	result, err := VerifyArtifacts(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false for no TCP telemetry")
	}
	if result.TCPTelemetryExercised {
		t.Error("expected TCPTelemetryExercised=false")
	}
	if result.FailureReason == "" {
		t.Error("expected non-empty failure reason")
	}
}

func TestVerifyArtifacts_FailUnderlayTCPEmptyObject(t *testing.T) {
	// This is the critical test: underlay_tcp with empty objects should fail
	dir := filepath.Join("testdata", "fail_underlay_tcp_empty_object")
	result, err := VerifyArtifacts(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false for underlay_tcp with empty objects")
	}
	if result.TCPTelemetryExercised {
		t.Error("expected TCPTelemetryExercised=false")
	}
	if result.FailureReason == "" {
		t.Error("expected non-empty failure reason")
	}
	// Verify the specific failure reason
	if result.FailureReason != "no underlay_tcp record has required fields (name, state, local, remote)" {
		t.Errorf("unexpected failure reason: %s", result.FailureReason)
	}
}

func TestVerifyArtifacts_FailWrongLocation(t *testing.T) {
	dir := filepath.Join("testdata", "fail_wrong_location")
	result, err := VerifyArtifacts(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false for TCP in wrong location")
	}
	if result.TCPTelemetryExercised {
		t.Error("expected TCPTelemetryExercised=false")
	}
}

func TestVerifyArtifacts_FailWrongPath(t *testing.T) {
	dir := filepath.Join("testdata", "fail_wrong_path")
	result, err := VerifyArtifacts(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false for wrong request path")
	}
	if result.FailureReason == "" {
		t.Error("expected non-empty failure reason")
	}
}

func TestVerifyArtifacts_FailSummaryLies(t *testing.T) {
	dir := filepath.Join("testdata", "fail_summary_lies")
	result, err := VerifyArtifacts(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false when summary claims TCP but evidence is missing")
	}
	if result.TCPTelemetryExercised {
		t.Error("expected TCPTelemetryExercised=false")
	}
}

func TestVerifyArtifacts_FailMalformedJSON(t *testing.T) {
	dir := filepath.Join("testdata", "fail_malformed_json")
	result, err := VerifyArtifacts(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false for malformed JSON")
	}
	if result.FailureReason == "" {
		t.Error("expected non-empty failure reason")
	}
}

func TestVerifyArtifacts_FailEmpty(t *testing.T) {
	// Create a temp empty directory
	tmpDir, err := os.MkdirTemp("", "uvb76-verifier-empty-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	result, err := VerifyArtifacts(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false for empty directory")
	}
	if result.FailureReason == "" {
		t.Error("expected non-empty failure reason")
	}
}

func TestContainsTCPTelemetry(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		expected bool
	}{
		{
			name: "valid TCP payload",
			payload: `{
				"network_diag": {
					"underlay_tcp": [{"name": "wg0", "state": "ESTABLISHED", "local": "10.0.0.1:51820", "remote": "10.0.0.2:51820"}]
				}
			}`,
			expected: true,
		},
		{
			name: "empty underlay_tcp record",
			payload: `{
				"network_diag": {
					"underlay_tcp": [{}]
				}
			}`,
			expected: false,
		},
		{
			name: "underlay_tcp record missing remote",
			payload: `{
				"network_diag": {
					"underlay_tcp": [{"name": "wg0", "state": "ESTABLISHED", "local": "10.0.0.1:51820"}]
				}
			}`,
			expected: false,
		},
		{
			name: "no network_diag",
			payload: `{"other": "data"}`,
			expected: false,
		},
		{
			name: "empty underlay_tcp",
			payload: `{"network_diag": {"underlay_tcp": []}}`,
			expected: false,
		},
		{
			name:     "malformed JSON",
			payload:  `{invalid}`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsTCPTelemetry([]byte(tt.payload))
			if result != tt.expected {
				t.Errorf("ContainsTCPTelemetry() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestVerifyCapturePacket(t *testing.T) {
	tests := []struct {
		name      string
		packet    CapturePacket
		wantOK    bool
		wantError string
	}{
		{
			name: "valid TCP packet",
			packet: CapturePacket{
				NetworkDiag: &NetworkDiagData{
					UnderlayTCP: []TcpSocketDiagData{
						{Name: "wg0", State: "ESTABLISHED", Local: "10.0.0.1:51820", Remote: "10.0.0.2:51820"},
					},
				},
			},
			wantOK: true,
		},
		{
			name: "no network_diag",
			packet: CapturePacket{
				NetworkDiag: nil,
			},
			wantOK:    false,
			wantError: "no network_diag field",
		},
		{
			name: "empty underlay_tcp",
			packet: CapturePacket{
				NetworkDiag: &NetworkDiagData{
					UnderlayTCP: []TcpSocketDiagData{},
				},
			},
			wantOK:    false,
			wantError: "network_diag has no underlay_tcp records",
		},
		{
			name: "TCP record missing required fields",
			packet: CapturePacket{
				NetworkDiag: &NetworkDiagData{
					UnderlayTCP: []TcpSocketDiagData{
						{Name: "wg0"}, // missing state, local, remote
					},
				},
			},
			wantOK:    false,
			wantError: "no underlay_tcp record has required fields (name, state, local, remote)",
		},
		{
			name: "TCP record with only name",
			packet: CapturePacket{
				NetworkDiag: &NetworkDiagData{
					UnderlayTCP: []TcpSocketDiagData{
						{Name: "wg0", State: "", Local: "", Remote: ""},
					},
				},
			},
			wantOK:    false,
			wantError: "no underlay_tcp record has required fields (name, state, local, remote)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := VerifyCapturePacket(&tt.packet)
			if ok != tt.wantOK {
				t.Errorf("VerifyCapturePacket() ok = %v, want %v", ok, tt.wantOK)
			}
			if err != tt.wantError {
				t.Errorf("VerifyCapturePacket() error = %v, want %v", err, tt.wantError)
			}
		})
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"/status.json?include=network_diag", "/status.json"},
		{"/status.json", "/status.json"},
		{"http://localhost:8317/status.json?include=network_diag", "http://localhost:8317/status.json"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := SanitizePath(tt.url)
			if result != tt.expected {
				t.Errorf("SanitizePath(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

// TestAllFixturesHaveTCPRecordCount is a regression test to ensure all fixture
// lab-result.json files use the current schema with tcp_record_count field.
// This test does not verify acceptance semantics - it only enforces fixture hygiene.
func TestAllFixturesHaveTCPRecordCount(t *testing.T) {
	testdataDir := filepath.Join(".", "testdata")
	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		t.Fatalf("failed to read testdata directory: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		fixtureName := entry.Name()
		labResultPath := filepath.Join(testdataDir, fixtureName, "lab-result.json")

		// Skip if no lab-result.json exists in this fixture directory
		if _, err := os.Stat(labResultPath); os.IsNotExist(err) {
			continue
		}

		data, err := os.ReadFile(labResultPath)
		if err != nil {
			t.Errorf("fixture %s: failed to read lab-result.json: %v", fixtureName, err)
			continue
		}

		// Unmarshal into a minimal struct to check for tcp_record_count presence
		var result struct {
			OK                    bool   `json:"ok"`
			TCPRecordCount        *int   `json:"tcp_record_count"`
			TCPTelemetryExercised bool   `json:"tcp_telemetry_exercised"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Errorf("fixture %s: failed to parse lab-result.json: %v", fixtureName, err)
			continue
		}

		if result.TCPRecordCount == nil {
			t.Errorf("fixture %s: lab-result.json is missing tcp_record_count field", fixtureName)
		}
	}
}

func TestVerifyArtifacts_FailMissingInclude(t *testing.T) {
	// Create a temp directory with wrong path (missing include param)
	tmpDir, err := os.MkdirTemp("", "uvb76-verifier-missing-include-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write a valid packet with proper TCP telemetry
	packet := map[string]interface{}{
		"source":    "test-peer",
		"base_url":  "http://localhost:18317",
		"status":    "ok",
		"network_diag": map[string]interface{}{
			"underlay_tcp": []map[string]interface{}{
				{"name": "wg0", "state": "ESTABLISHED", "local": "10.0.0.1:51820", "remote": "10.0.0.2:51820"},
			},
		},
	}
	packetJSON, _ := json.Marshal(packet)
	os.WriteFile(filepath.Join(tmpDir, "captured-diagnostic-packet.json"), packetJSON, 0644)

	// Write wrong request path
	req := map[string]string{"method": "GET", "url": "/status.json"}
	reqJSON, _ := json.Marshal(req)
	os.WriteFile(filepath.Join(tmpDir, "capture-request.json"), reqJSON, 0644)

	// Write lab result
	result := map[string]interface{}{
		"ok":                    true,
		"mode":                  "hermetic-diagnostic-peer",
		"requested_path":        "/status.json",
		"capture_packet_count":  1,
		"tcp_telemetry_exercised": true,
	}
	resultJSON, _ := json.Marshal(result)
	os.WriteFile(filepath.Join(tmpDir, "lab-result.json"), resultJSON, 0644)

	// Verify should fail because path is wrong
	verifierResult, err := VerifyArtifacts(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verifierResult.OK {
		t.Error("expected OK=false when include=network_diag is missing from path")
	}
}
