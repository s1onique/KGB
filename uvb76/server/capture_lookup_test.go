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

// newAnchorLookupRouter creates a test router with both anchor lookup endpoints.
func newAnchorLookupRouter(srv *Server) *mux.Router {
	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/captures/{capture_id}/anchor", http.HandlerFunc(srv.handleGetAnchorCapture))
	protected.Handle("/diagnostics/cooldown/anchors/{peer_name}", http.HandlerFunc(srv.handleGetCooldownAnchorForPeer))
	return router
}

// authenticatedAnchorRequest creates an authenticated GET request for the anchor endpoint.
func authenticatedAnchorRequest(t *testing.T, srv *Server, path string) (*http.Request, *httptest.ResponseRecorder) {
	token, err := srv.sessions.GenerateToken("admin")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	return req, rec
}

// =============================================================================
// Test: handleGetAnchorCapture - requires authentication
// =============================================================================

func TestGetAnchorCapture_RequiresAuth(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:  config.ListenConfig{Addr: ":0"},
		Auth:    config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:  config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency: config.LatencyConfig{},
	}
	cfg.Latency.ApplyDefaults()

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	router := newAnchorLookupRouter(srv)

	// Request without session cookie
	req := httptest.NewRequest(http.MethodGet, "/api/v1/captures/test-event-001/anchor", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for unauthenticated request, got %d", rec.Code)
	}

	// Should NOT have WWW-Authenticate header (JSON-only error)
	if wwwAuth := rec.Header().Get("WWW-Authenticate"); wwwAuth != "" {
		t.Errorf("Should not have WWW-Authenticate header, got '%s'", wwwAuth)
	}
}

// =============================================================================
// Test: handleGetAnchorCapture - returns 404 for nonexistent capture
// =============================================================================

func TestGetAnchorCapture_NotFoundForNonexistentCapture(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:  config.ListenConfig{Addr: ":0"},
		Auth:    config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:  config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency: config.LatencyConfig{},
	}
	cfg.Latency.ApplyDefaults()

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	router := newAnchorLookupRouter(srv)

	req, rec := authenticatedAnchorRequest(t, srv, "/api/v1/captures/nonexistent-capture-id/anchor")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for nonexistent capture, got %d", rec.Code)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "capture not found" {
		t.Errorf("Expected error 'capture not found', got %q", resp["error"])
	}
}

// =============================================================================
// Test: handleGetAnchorCapture - returns 400 for non-anchor capture
// =============================================================================

func TestGetAnchorCapture_BadRequestForNonAnchorCapture(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:  config.ListenConfig{Addr: ":0"},
		Auth:    config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:  config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency: config.LatencyConfig{},
	}
	cfg.Latency.ApplyDefaults()

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	captureStore := st.GetCaptureStore()
	router := newAnchorLookupRouter(srv)

	// Add a capture that is NOT a cooldown anchor (e.g., suppressed by cooldown)
	eventID := "suppressed-capture-001"
	capture := state.DiagCapture{
		Source:               "tovarisch-peer",
		CaptureStartedAt:     time.Now(),
		Status:               state.DiagCaptureStatusOK,
		CaptureStatus:         state.CaptureStatusCaptured,
		SuppressedByCooldown: true, // This makes it NOT an anchor
	}
	captureStore.AddCapture(eventID, capture)

	req, rec := authenticatedAnchorRequest(t, srv, "/api/v1/captures/"+eventID+"/anchor")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for non-anchor capture, got %d", rec.Code)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "not_an_anchor_capture" {
		t.Errorf("Expected error 'not_an_anchor_capture', got %q", resp["error"])
	}
}

// =============================================================================
// Test: handleGetAnchorCapture - returns anchor details for valid anchor capture
// =============================================================================

func TestGetAnchorCapture_ReturnsAnchorForValidCapture(t *testing.T) {
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
			CooldownSeconds: 90,
		},
	}
	cfg.Latency.ApplyDefaults()
	cfg.Diagnostics.ApplyDefaults()

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	captureStore := st.GetCaptureStore()
	router := newAnchorLookupRouter(srv)

	// Add a successful capture that IS a cooldown anchor
	eventID := "anchor-capture-001"
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

	req, rec := authenticatedAnchorRequest(t, srv, "/api/v1/captures/"+eventID+"/anchor")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 for valid anchor capture, got %d", rec.Code)
	}

	var resp AnchorCaptureResponse
	json.NewDecoder(rec.Body).Decode(&resp)

	// Verify response structure
	if resp.Status != AnchorStatusAvailable {
		t.Errorf("Expected status 'available', got %q", resp.Status)
	}
	// Degraded=true when spike event is nil (expected since we didn't add spike event)
	// Capture and anchor should still be populated
	if resp.Capture == nil {
		t.Fatal("Expected Capture field to be populated")
	}
	if resp.Capture.Source != "tovarisch-peer" {
		t.Errorf("Expected capture source 'tovarisch-peer', got %q", resp.Capture.Source)
	}
	if resp.Anchor == nil {
		t.Fatal("Expected Anchor field to be populated")
	}
	if resp.Anchor.AnchorCaptureID != eventID {
		t.Errorf("Expected anchor_capture_id=%q, got %q", eventID, resp.Anchor.AnchorCaptureID)
	}
	if resp.Anchor.AnchorTargetID != targetID {
		t.Errorf("Expected anchor_target_id=%q, got %q", targetID, resp.Anchor.AnchorTargetID)
	}
	if resp.Anchor.AnchorProbeKind != probeKind {
		t.Errorf("Expected anchor_probe_kind=%q, got %q", probeKind, resp.Anchor.AnchorProbeKind)
	}
}

// =============================================================================
// Test: handleGetCooldownAnchorForPeer - requires authentication
// =============================================================================

func TestGetCooldownAnchorForPeer_RequiresAuth(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:  config.ListenConfig{Addr: ":0"},
		Auth:    config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:  config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency: config.LatencyConfig{},
	}
	cfg.Latency.ApplyDefaults()

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	router := newAnchorLookupRouter(srv)

	// Request without session cookie
	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/cooldown/anchors/tovarisch-peer", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for unauthenticated request, got %d", rec.Code)
	}
}

// =============================================================================
// Test: handleGetCooldownAnchorForPeer - returns 404 for peer with no anchor
// =============================================================================

func TestGetCooldownAnchorForPeer_NotFoundForNoAnchor(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:  config.ListenConfig{Addr: ":0"},
		Auth:    config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:  config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency: config.LatencyConfig{},
	}
	cfg.Latency.ApplyDefaults()

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	router := newAnchorLookupRouter(srv)

	req, rec := authenticatedAnchorRequest(t, srv, "/api/v1/diagnostics/cooldown/anchors/unknown-peer")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for peer with no anchor, got %d", rec.Code)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "no_anchor" {
		t.Errorf("Expected error 'no_anchor', got %q", resp["error"])
	}
}

// =============================================================================
// Test: handleGetCooldownAnchorForPeer - returns anchor for peer with cooldown
// =============================================================================

func TestGetCooldownAnchorForPeer_ReturnsAnchorForPeer(t *testing.T) {
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
			CooldownSeconds: 90,
		},
	}
	cfg.Latency.ApplyDefaults()
	cfg.Diagnostics.ApplyDefaults()

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	captureStore := st.GetCaptureStore()
	router := newAnchorLookupRouter(srv)

	// Add a successful capture for a peer
	peerName := "tovarisch-peer"
	eventID := "peer-anchor-001"
	captureStartedAt := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	capture := state.DiagCapture{
		Source:               peerName,
		CaptureStartedAt:     captureStartedAt,
		Status:               state.DiagCaptureStatusOK,
		CaptureStatus:         state.CaptureStatusCaptured,
		SuppressedByCooldown: false,
	}
	targetID := "target-http-1"
	probeKind := "http"

	captureStore.AddCaptureWithProvenance(eventID, capture, targetID, probeKind)

	req, rec := authenticatedAnchorRequest(t, srv, "/api/v1/diagnostics/cooldown/anchors/"+peerName)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 for peer with anchor, got %d", rec.Code)
	}

	var resp struct {
		Anchor        state.CaptureCooldownAnchor `json:"anchor"`
		CaptureExists bool                        `json:"capture_exists"`
		Degraded      bool                        `json:"degraded"`
		CheckedAt     string                      `json:"checked_at"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)

	// Verify response structure
	if resp.Anchor.AnchorCaptureID != eventID {
		t.Errorf("Expected anchor_capture_id=%q, got %q", eventID, resp.Anchor.AnchorCaptureID)
	}
	if resp.Anchor.AnchorSource != peerName {
		t.Errorf("Expected anchor_source=%q, got %q", peerName, resp.Anchor.AnchorSource)
	}
	if !resp.CaptureExists {
		t.Error("Expected capture_exists=true for valid anchor capture")
	}
	if resp.Degraded {
		t.Error("Expected degraded=false for valid anchor")
	}
	if resp.CheckedAt == "" {
		t.Error("Expected non-empty checked_at")
	}
}

// =============================================================================
// Test: handleGetCooldownAnchorForPeer - degraded when capture artifact missing
// =============================================================================

func TestGetCooldownAnchorForPeer_DegradedWhenCaptureMissing(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:  config.ListenConfig{Addr: ":0"},
		Auth:    config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:  config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency: config.LatencyConfig{},
	}
	cfg.Latency.ApplyDefaults()

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	captureStore := st.GetCaptureStore()
	router := newAnchorLookupRouter(srv)

	// Manually set an anchor in the store WITHOUT the actual capture
	// This simulates the case where anchor metadata exists but artifact is purged
	peerName := "tovarisch-peer"
	anchor := state.CaptureCooldownAnchor{
		AnchorCaptureID: "purged-capture-001",
		AnchorSource:    peerName,
		AnchorTargetID:  "target-http-1",
		AnchorProbeKind: "http",
		CreatedFrom:     "diag_capture_success",
	}
	captureStore.SetLastCaptureAnchor(peerName, anchor)

	req, rec := authenticatedAnchorRequest(t, srv, "/api/v1/diagnostics/cooldown/anchors/"+peerName)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 even with missing capture, got %d", rec.Code)
	}

	var resp struct {
		Anchor        state.CaptureCooldownAnchor `json:"anchor"`
		CaptureExists bool                        `json:"capture_exists"`
		Degraded      bool                        `json:"degraded"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)

	// Anchor metadata should be present
	if resp.Anchor.AnchorCaptureID != "purged-capture-001" {
		t.Errorf("Expected anchor_capture_id='purged-capture-001', got %q", resp.Anchor.AnchorCaptureID)
	}
	// But capture artifact should be missing
	if resp.CaptureExists {
		t.Error("Expected capture_exists=false for purged capture")
	}
	// And response should indicate degraded state
	if !resp.Degraded {
		t.Error("Expected degraded=true when capture artifact is missing")
	}
}
