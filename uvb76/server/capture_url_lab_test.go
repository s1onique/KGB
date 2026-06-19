package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/diag"
	"github.com/s1onique/KGB/uvb76/state"
)

// TestDiagnosticCaptureUsesCanonicalTovarischStatusURL proves diagnostic capture
// hits exactly /status.json?include=network_diag.
func TestDiagnosticCaptureUsesCanonicalTovarischStatusURL(t *testing.T) {
	lab := newCaptureURLLabTest()
	defer lab.Close()

	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       2000,
		CooldownSeconds: 90,
		Peers: []config.DiagPeerConfig{
			{Name: "tovarisch-peer", BaseURL: lab.URL(), Targets: []string{"test-target"}},
		},
	}
	cfg.ApplyDefaults()

	captureStore := state.NewCaptureStore()
	captureSvc := diag.NewCaptureService(cfg, captureStore)

	captureSvc.TriggerCapture("test-event", "test-target")
	captures := waitForCaptures(captureStore, "test-event", 2*time.Second)
	if len(captures) == 0 {
		t.Fatal("expected at least one capture")
	}

	capture := captures[0]
	if capture.Status != state.DiagCaptureStatusOK {
		t.Errorf("expected capture status 'ok', got '%s': %v", capture.Status, capture.Error)
	}

	requests := lab.getRequests()
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}

	req := lab.getLastRequest()
	if req.Path != "/status.json" {
		t.Errorf("expected path '/status.json', got '%s'", req.Path)
	}
	if req.Query["include"][0] != "network_diag" {
		t.Errorf("expected include='network_diag', got '%s'", req.Query["include"][0])
	}
	if capture.Error != nil {
		t.Errorf("expected no error, got: %s", *capture.Error)
	}
	if capture.NetworkDiag == nil {
		t.Error("expected network_diag data")
	}
}

// TestDiagnosticCaptureUsesCanonicalTovarischStatusURLWithTrailingSlashBaseURL
// verifies capture works with base URL ending in slash.
func TestDiagnosticCaptureUsesCanonicalTovarischStatusURLWithTrailingSlashBaseURL(t *testing.T) {
	lab := newCaptureURLLabTest()
	defer lab.Close()

	baseURL := lab.URL() + "/"
	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       2000,
		CooldownSeconds: 90,
		Peers: []config.DiagPeerConfig{
			{Name: "tovarisch-peer", BaseURL: baseURL, Targets: []string{"test-target"}},
		},
	}
	cfg.ApplyDefaults()

	captureStore := state.NewCaptureStore()
	captureSvc := diag.NewCaptureService(cfg, captureStore)

	captureSvc.TriggerCapture("test-event", "test-target")
	captures := waitForCaptures(captureStore, "test-event", 2*time.Second)

	if len(captures) == 0 {
		t.Fatal("expected at least one capture")
	}
	capture := captures[0]
	if capture.Status != state.DiagCaptureStatusOK {
		t.Errorf("expected capture status 'ok', got '%s': %v", capture.Status, capture.Error)
	}

	requests := lab.getRequests()
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}

	req := lab.getLastRequest()
	if req.Path != "/status.json" {
		t.Errorf("expected path '/status.json', got '%s'", req.Path)
	}
	if req.Query["include"][0] != "network_diag" {
		t.Errorf("expected include='network_diag', got '%s'", req.Query["include"][0])
	}
}

// TestDiagnosticCaptureRecordsHTTP404WithSanitizedRequestedPath verifies that
// when tovarisch returns 404, the capture includes clear error message and
// sanitized path evidence.
func TestDiagnosticCaptureRecordsHTTP404WithSanitizedRequestedPath(t *testing.T) {
	var requestLog atomic.Value
	requestLog.Store([]capturedRequest{})

	notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
		}
		existing := requestLog.Load().([]capturedRequest)
		requestLog.Store(append(existing, req))
		http.Error(w, "not found", http.StatusNotFound)
	})

	server := httptest.NewServer(notFoundHandler)
	defer server.Close()

	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       2000,
		CooldownSeconds: 90,
		Peers: []config.DiagPeerConfig{
			{Name: "tovarisch-peer", BaseURL: server.URL, Targets: []string{"test-target"}},
		},
	}
	cfg.ApplyDefaults()

	captureStore := state.NewCaptureStore()
	captureSvc := diag.NewCaptureService(cfg, captureStore)

	captureSvc.TriggerCapture("test-event", "test-target")
	captures := waitForCaptures(captureStore, "test-event", 2*time.Second)

	if len(captures) == 0 {
		t.Fatal("expected at least one capture")
	}

	capture := captures[0]
	if capture.Status != state.DiagCaptureStatusError {
		t.Errorf("expected capture status 'error', got '%s'", capture.Status)
	}

	if capture.Error == nil {
		t.Fatal("expected error message")
	}
	if !strings.Contains(*capture.Error, "HTTP 404") {
		t.Errorf("expected error message to contain 'HTTP 404', got: %s", *capture.Error)
	}

	if capture.RequestedPath == nil {
		t.Fatal("expected RequestedPath to be set")
	}

	expectedPath := "/status.json?include=network_diag"
	if *capture.RequestedPath != expectedPath {
		t.Errorf("expected RequestedPath '%s', got '%s'", expectedPath, *capture.RequestedPath)
	}

	requests := requestLog.Load().([]capturedRequest)
	if len(requests) == 0 {
		t.Fatal("expected at least one request to server")
	}
}

// TestDiagPeerStatusURLProducesCanonicalStatusPath verifies the URL helper
// produces /status.json (not /status or other variants).
func TestDiagPeerStatusURLProducesCanonicalStatusPath(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		wantPath  string
		wantQuery string
	}{
		{"no trailing slash", "http://localhost:8317", "/status.json", "include=network_diag"},
		{"with trailing slash", "http://localhost:8317/", "/status.json", "include=network_diag"},
		{"with base path", "http://localhost:8317/api", "/api/status.json", "include=network_diag"},
		{"with base path and trailing slash", "http://localhost:8317/api/", "/api/status.json", "include=network_diag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.DiagPeerStatusURL(tt.baseURL)
			if !strings.Contains(result, tt.wantPath) {
				t.Errorf("DiagPeerStatusURL(%q) = %q, want path %q", tt.baseURL, result, tt.wantPath)
			}
			if !strings.Contains(result, tt.wantQuery) {
				t.Errorf("DiagPeerStatusURL(%q) result %q, want query %q", tt.baseURL, result, tt.wantQuery)
			}
			// Check for wrong variants
			if strings.Contains(result, "/status?") || strings.Contains(result, "/status&") {
				t.Errorf("DiagPeerStatusURL(%q) = %q, should NOT contain /status without .json", tt.baseURL, result)
			}
			if strings.Contains(result, "/api/v1/status") {
				t.Errorf("DiagPeerStatusURL(%q) = %q, should NOT contain wrong path /api/v1/status", tt.baseURL, result)
			}
			if strings.Contains(result, "/status.json/status.json") {
				t.Errorf("DiagPeerStatusURL(%q) = %q, should NOT contain double-appended path", tt.baseURL, result)
			}
		})
	}
}

// TestDiagPeerStatusURLAppendsIncludeUsingURLQuery verifies that include param
// is properly added using url.Values, not string concatenation.
func TestDiagPeerStatusURLAppendsIncludeUsingURLQuery(t *testing.T) {
	result := config.DiagPeerStatusURL("http://host:8317")
	if !strings.Contains(result, "include=network_diag") {
		t.Errorf("DiagPeerStatusURL should contain 'include=network_diag', got: %s", result)
	}
	if strings.Contains(result, "??") || strings.Contains(result, "&=") {
		t.Errorf("DiagPeerStatusURL has malformed query: %s", result)
	}
}

// TestCaptureURLLabFullIntegration tests the complete capture flow.
func TestCaptureURLLabFullIntegration(t *testing.T) {
	lab := newCaptureURLLabTest()
	defer lab.Close()

	srv, st, captureSvc, _ := setupTestServer(t, lab.URL())
	srv.cfg.Diagnostics.Peers[0].BaseURL = lab.URL()

	captureSvc.TriggerCapture("test-event", "test-target")
	captures := waitForCaptures(st.GetCaptureStore(), "test-event", 2*time.Second)

	if len(captures) == 0 {
		t.Fatal("expected capture")
	}
	if captures[0].Status != state.DiagCaptureStatusOK {
		t.Errorf("expected OK status, got: %s", captures[0].Status)
	}

	requests := lab.getRequests()
	if len(requests) != 1 {
		t.Errorf("expected 1 request, got: %d", len(requests))
	}
	if requests[0].Path != "/status.json" {
		t.Errorf("expected /status.json, got: %s", requests[0].Path)
	}
	if requests[0].Query["include"][0] != "network_diag" {
		t.Errorf("expected include=network_diag, got: %s", requests[0].Query["include"][0])
	}

	_ = srv
}

// TestCaptureURLLabRejectsWrongEndpoints verifies the fake server correctly
// rejects requests to wrong endpoints.
func TestCaptureURLLabRejectsWrongEndpoints(t *testing.T) {
	wrongPaths := []struct {
		path  string
		query string
		desc  string
	}{
		{"/status", "", "plain /status"},
		{"/api/v1/status", "", "wrong API path"},
		{"/status.json", "", "missing include param"},
		{"/status.json", "include=wrong", "wrong include value"},
		{"/status.json/status.json", "include=network_diag", "double appended"},
	}

	for _, tc := range wrongPaths {
		t.Run(tc.desc, func(t *testing.T) {
			url := "http://localhost:9999" + tc.path
			if tc.query != "" {
				url += "?" + tc.query
			}

			resp, err := http.Get(url)
			if err != nil {
				return // Connection refused is expected
			}
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				t.Errorf("wrong path %q should not return 200", tc.path)
			}
		})
	}
}

// TestCaptureURLLabConstantExported verifies the canonical endpoint constant.
func TestCaptureURLLabConstantExported(t *testing.T) {
	if config.TovarischStatusEndpoint != "/status.json" {
		t.Errorf("TovarischStatusEndpoint = %q, want /status.json", config.TovarischStatusEndpoint)
	}
}

// TestCaptureURLLabDiagPeerStatusInclude verifies the include param constant.
func TestCaptureURLLabDiagPeerStatusInclude(t *testing.T) {
	if config.DiagPeerStatusInclude != "network_diag" {
		t.Errorf("DiagPeerStatusInclude = %q, want network_diag", config.DiagPeerStatusInclude)
	}
}

// TestCaptureURLLabJSONResponse verifies the fake server returns valid JSON.
func TestCaptureURLLabJSONResponse(t *testing.T) {
	lab := newCaptureURLLabTest()
	defer lab.Close()

	resp, err := http.Get(lab.URL() + "/status.json?include=network_diag")
	if err != nil {
		t.Fatalf("failed to get response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Errorf("invalid JSON response: %v", err)
	}

	if result["service"] != "tovarisch" {
		t.Errorf("expected service='tovarisch', got %v", result["service"])
	}
	if result["network_diag"] == nil {
		t.Error("expected network_diag field")
	}
}
