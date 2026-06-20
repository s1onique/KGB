package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/auth"
	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// =============================================================================
// Test Helpers
// =============================================================================

// newCaptureCooldownDiagnosticsRouter creates a test router with the diagnostics endpoint protected.
func newCaptureCooldownDiagnosticsRouter(srv *Server) *mux.Router {
	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/diagnostics/capture-cooldown", http.HandlerFunc(srv.handleCaptureCooldownDiagnostics))
	return router
}

// authenticatedCooldownDiagnosticsRequest creates an authenticated GET request.
func authenticatedCooldownDiagnosticsRequest(t *testing.T, srv *Server) (*http.Request, *httptest.ResponseRecorder) {
	token, err := srv.sessions.GenerateToken("admin")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/capture-cooldown", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	return req, rec
}

// decodeCooldownDiagnostics decodes the response body into CaptureCooldownDiagnostics.
func decodeCooldownDiagnostics(t *testing.T, rec *httptest.ResponseRecorder) CaptureCooldownDiagnostics {
	var diag CaptureCooldownDiagnostics
	if err := json.NewDecoder(rec.Body).Decode(&diag); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	return diag
}

// containsString checks if a string slice contains a value.
func containsString(slice []string, value string) bool {
	for _, s := range slice {
		if s == value {
			return true
		}
	}
	return false
}

// newTestServerWithDiagnostics creates a test server with diagnostics config.
func newTestServerWithDiagnostics(t *testing.T) (*Server, *state.Manager) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:  config.ListenConfig{Addr: ":0"},
		Auth:    config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:  config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency: config.LatencyConfig{},
		Diagnostics: config.DiagnosticsConfig{
			Enabled:         true,
			CaptureOnSpike:  true,
			TimeoutMs:       2000,
			CooldownSeconds: 90,
		},
	}
	cfg.Latency.ApplyDefaults()
	cfg.Diagnostics.ApplyDefaults()

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	return srv, st
}

// =============================================================================
// Test 1: Auth Required
// =============================================================================

func TestCaptureCooldownDiagnostics_RequiresAuth(t *testing.T) {
	srv, _ := newTestServerWithDiagnostics(t)
	router := newCaptureCooldownDiagnosticsRouter(srv)

	// Request without session cookie
	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/capture-cooldown", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for unauthenticated request, got %d", rec.Code)
	}

	// Should NOT have WWW-Authenticate header (JSON-only error)
	if wwwAuth := rec.Header().Get("WWW-Authenticate"); wwwAuth != "" {
		t.Errorf("Should not have WWW-Authenticate header, got '%s'", wwwAuth)
	}

	// Should be JSON response
	if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

// =============================================================================
// Test 2: Empty Store Returns Safe Bounded Payload
// =============================================================================

func TestCaptureCooldownDiagnostics_EmptyStoreSafe(t *testing.T) {
	srv, _ := newTestServerWithDiagnostics(t)
	router := newCaptureCooldownDiagnosticsRouter(srv)

	req, rec := authenticatedCooldownDiagnosticsRequest(t, srv)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 for authenticated request, got %d", rec.Code)
	}

	diag := decodeCooldownDiagnostics(t, rec)

	// cooldown_anchors should be empty or length 0
	if diag.CooldownAnchors != nil && len(diag.CooldownAnchors) != 0 {
		t.Errorf("Expected empty cooldown_anchors, got %d entries", len(diag.CooldownAnchors))
	}

	// active_cooldown_keys should be empty or length 0
	if len(diag.ActiveCooldownKeys) != 0 {
		t.Errorf("Expected empty active_cooldown_keys, got %d entries", len(diag.ActiveCooldownKeys))
	}

	// total_captures should be 0
	if diag.TotalCaptures != 0 {
		t.Errorf("Expected total_captures=0, got %d", diag.TotalCaptures)
	}

	// server_started_at should be non-empty
	if diag.ServerStartedAt == "" {
		t.Error("Expected non-empty server_started_at")
	}

	// current_time should be non-empty
	if diag.CurrentTime == "" {
		t.Error("Expected non-empty current_time")
	}
}

// =============================================================================
// Test 3: Successful Capture Serializes Target/Probe Provenance
// =============================================================================

func TestCaptureCooldownDiagnostics_SuccessfulCaptureSerializesProvenance(t *testing.T) {
	srv, st := newTestServerWithDiagnostics(t)
	captureStore := st.GetCaptureStore()
	router := newCaptureCooldownDiagnosticsRouter(srv)

	// Add a successful captured diagnostic using AddCaptureWithProvenance
	eventID := "event-test-001"
	captureStartedAt := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	capture := state.DiagCapture{
		Source:               "tovarisch-peer",
		CaptureStartedAt:     captureStartedAt,
		Status:               state.DiagCaptureStatusOK,
		CaptureStatus:         state.CaptureStatusCaptured,
		SuppressedByCooldown: false,
	}
	targetID := "target-http-1"
	probeKind := "http"

	captureStore.AddCaptureWithProvenance(eventID, capture, targetID, probeKind)

	// Request the diagnostics endpoint
	req, rec := authenticatedCooldownDiagnosticsRequest(t, srv)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 for authenticated request, got %d", rec.Code)
	}

	diag := decodeCooldownDiagnostics(t, rec)

	// cooldown_anchors["tovarisch-peer"] should exist
	anchor, exists := diag.CooldownAnchors["tovarisch-peer"]
	if !exists {
		t.Fatal("Expected cooldown_anchors[\"tovarisch-peer\"] to exist")
	}

	// Anchor should have correct provenance
	if anchor.AnchorCaptureID != eventID {
		t.Errorf("Expected anchor_capture_id=%q, got %q", eventID, anchor.AnchorCaptureID)
	}
	if anchor.AnchorTargetID != targetID {
		t.Errorf("Expected anchor_target_id=%q, got %q", targetID, anchor.AnchorTargetID)
	}
	if anchor.AnchorProbeKind != probeKind {
		t.Errorf("Expected anchor_probe_kind=%q, got %q", probeKind, anchor.AnchorProbeKind)
	}
	if anchor.AnchorSource != "tovarisch-peer" {
		t.Errorf("Expected anchor_source=%q, got %q", "tovarisch-peer", anchor.AnchorSource)
	}
	if anchor.CreatedFrom != "diag_capture_success" {
		t.Errorf("Expected created_from=%q, got %q", "diag_capture_success", anchor.CreatedFrom)
	}
	if anchor.AnchorUpdatedByStatus != string(state.CaptureStatusCaptured) {
		t.Errorf("Expected anchor_updated_by_status=%q, got %q", string(state.CaptureStatusCaptured), anchor.AnchorUpdatedByStatus)
	}

	// anchor_created_at should match the fixed timestamp
	if anchor.AnchorCreatedAt.Format(time.RFC3339) != captureStartedAt.Format(time.RFC3339) {
		t.Errorf("Expected anchor_created_at=%q, got %q", captureStartedAt.Format(time.RFC3339), anchor.AnchorCreatedAt.Format(time.RFC3339))
	}

	// active_cooldown_keys should contain "tovarisch-peer"
	if !containsString(diag.ActiveCooldownKeys, "tovarisch-peer") {
		t.Errorf("Expected active_cooldown_keys to contain %q", "tovarisch-peer")
	}

	// total_captures should be 1
	if diag.TotalCaptures != 1 {
		t.Errorf("Expected total_captures=1, got %d", diag.TotalCaptures)
	}
}
