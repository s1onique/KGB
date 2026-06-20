package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// newTestServerWithDiagForCooldown creates a test server with diagnostics config.
// Reuses the helper from the main test file via package access.
func newTestServerWithDiagForCooldown(t *testing.T) (*Server, *state.Manager, *mux.Router) {
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
	router := newCaptureCooldownDiagnosticsRouter(srv)
	return srv, st, router
}

// =============================================================================
// Test 4: Skipped Cooldown Does Not Become an Anchor
// =============================================================================

func TestCaptureCooldownDiagnostics_SkippedCooldownDoesNotBecomeAnchor(t *testing.T) {
	srv, st, router := newTestServerWithDiagForCooldown(t)
	captureStore := st.GetCaptureStore()

	// Add a skipped cooldown capture
	eventID := "event-skipped-001"
	captureStartedAt := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	capture := state.DiagCapture{
		Source:               "tovarisch-peer",
		CaptureStartedAt:     captureStartedAt,
		Status:               state.DiagCaptureStatusOK,
		CaptureStatus:        state.CaptureStatusSkippedCooldown,
		SuppressedByCooldown: true,
	}

	// Use AddCapture (skipped cooldown should NOT update anchor)
	captureStore.AddCapture(eventID, capture)

	// Request the diagnostics endpoint
	req, rec := authenticatedCooldownDiagnosticsRequest(t, srv)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 for authenticated request, got %d", rec.Code)
	}

	diag := decodeCooldownDiagnostics(t, rec)

	// cooldown_anchors should NOT have "tovarisch-peer" entry
	if _, exists := diag.CooldownAnchors["tovarisch-peer"]; exists {
		t.Error("Expected cooldown_anchors to NOT have \"tovarisch-peer\" entry for skipped cooldown")
	}

	// active_cooldown_keys should NOT contain "tovarisch-peer"
	if containsString(diag.ActiveCooldownKeys, "tovarisch-peer") {
		t.Errorf("Expected active_cooldown_keys to NOT contain %q", "tovarisch-peer")
	}

	// total_captures should be 1 (record is still stored)
	if diag.TotalCaptures != 1 {
		t.Errorf("Expected total_captures=1, got %d", diag.TotalCaptures)
	}
}

// =============================================================================
// Test 5: Successful Anchor Plus Skipped Cooldown Retains Original Anchor
// =============================================================================

func TestCaptureCooldownDiagnostics_SkippedCooldownDoesNotAdvanceExistingAnchor(t *testing.T) {
	srv, st, router := newTestServerWithDiagForCooldown(t)
	captureStore := st.GetCaptureStore()

	// Add a successful captured event at T0
	t0 := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	anchorEventID := "anchor-event"
	successfulCapture := state.DiagCapture{
		Source:               "tovarisch-peer",
		CaptureStartedAt:     t0,
		Status:               state.DiagCaptureStatusOK,
		CaptureStatus:        state.CaptureStatusCaptured,
		SuppressedByCooldown: false,
	}
	captureStore.AddCaptureWithProvenance(anchorEventID, successfulCapture, "target-a", "http")

	// Add multiple skipped cooldown captures at T0+10s and T0+20s
	t1 := t0.Add(10 * time.Second)
	t2 := t0.Add(20 * time.Second)

	skippedCapture1 := state.DiagCapture{
		Source:               "tovarisch-peer",
		CaptureStartedAt:     t1,
		Status:               state.DiagCaptureStatusOK,
		CaptureStatus:        state.CaptureStatusSkippedCooldown,
		SuppressedByCooldown: true,
	}
	captureStore.AddCapture("skipped-event-1", skippedCapture1)

	skippedCapture2 := state.DiagCapture{
		Source:               "tovarisch-peer",
		CaptureStartedAt:     t2,
		Status:               state.DiagCaptureStatusOK,
		CaptureStatus:        state.CaptureStatusSkippedCooldown,
		SuppressedByCooldown: true,
	}
	captureStore.AddCapture("skipped-event-2", skippedCapture2)

	// Request the diagnostics endpoint
	req, rec := authenticatedCooldownDiagnosticsRequest(t, srv)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 for authenticated request, got %d", rec.Code)
	}

	diag := decodeCooldownDiagnostics(t, rec)

	// Anchor should still point to anchorEventID
	anchor, exists := diag.CooldownAnchors["tovarisch-peer"]
	if !exists {
		t.Fatal("Expected cooldown_anchors[\"tovarisch-peer\"] to exist")
	}

	if anchor.AnchorCaptureID != anchorEventID {
		t.Errorf("Expected anchor_capture_id=%q, got %q", anchorEventID, anchor.AnchorCaptureID)
	}

	// anchor_target_id should be "target-a"
	if anchor.AnchorTargetID != "target-a" {
		t.Errorf("Expected anchor_target_id=%q, got %q", "target-a", anchor.AnchorTargetID)
	}

	// anchor_probe_kind should be "http"
	if anchor.AnchorProbeKind != "http" {
		t.Errorf("Expected anchor_probe_kind=%q, got %q", "http", anchor.AnchorProbeKind)
	}

	// anchor_created_at should remain T0, not a skipped timestamp
	if anchor.AnchorCreatedAt.Format(time.RFC3339) != t0.Format(time.RFC3339) {
		t.Errorf("Expected anchor_created_at=%q (T0), got %q", t0.Format(time.RFC3339), anchor.AnchorCreatedAt.Format(time.RFC3339))
	}

	// total_captures should include all 3 captures (1 successful + 2 skipped)
	if diag.TotalCaptures != 3 {
		t.Errorf("Expected total_captures=3, got %d", diag.TotalCaptures)
	}

	// active_cooldown_keys should have exactly "tovarisch-peer"
	if len(diag.ActiveCooldownKeys) != 1 {
		t.Errorf("Expected exactly 1 active_cooldown_key, got %d", len(diag.ActiveCooldownKeys))
	}
	if !containsString(diag.ActiveCooldownKeys, "tovarisch-peer") {
		t.Errorf("Expected active_cooldown_keys to contain %q", "tovarisch-peer")
	}
}

// =============================================================================
// Test 6: Response Has Evidence Fields (for router evidence procedure)
// =============================================================================

func TestCaptureCooldownDiagnostics_ResponseHasEvidenceFields(t *testing.T) {
	srv, st, router := newTestServerWithDiagForCooldown(t)
	captureStore := st.GetCaptureStore()

	// Add a successful capture so we have anchors
	eventID := "event-evidence-001"
	capture := state.DiagCapture{
		Source:               "peer-alpha",
		CaptureStartedAt:     time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC),
		Status:               state.DiagCaptureStatusOK,
		CaptureStatus:        state.CaptureStatusCaptured,
		SuppressedByCooldown: false,
	}
	captureStore.AddCaptureWithProvenance(eventID, capture, "target-x", "icmp")

	req, rec := authenticatedCooldownDiagnosticsRequest(t, srv)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 for authenticated request, got %d", rec.Code)
	}

	// Decode into a map to verify all required fields are present
	var diagMap map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&diagMap); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify all top-level evidence fields are present
	requiredFields := []string{
		"server_started_at",
		"current_time",
		"cooldown_anchors",
		"active_cooldown_keys",
		"total_captures",
	}

	for _, field := range requiredFields {
		if _, exists := diagMap[field]; !exists {
			t.Errorf("Missing required evidence field: %s", field)
		}
	}

	// Verify types
	if _, ok := diagMap["server_started_at"].(string); !ok {
		t.Error("server_started_at should be a string")
	}
	if _, ok := diagMap["current_time"].(string); !ok {
		t.Error("current_time should be a string")
	}
	if _, ok := diagMap["cooldown_anchors"].(map[string]interface{}); !ok {
		t.Error("cooldown_anchors should be an object/map")
	}
	if _, ok := diagMap["active_cooldown_keys"].([]interface{}); !ok {
		t.Error("active_cooldown_keys should be an array")
	}
	if _, ok := diagMap["total_captures"].(float64); !ok {
		t.Error("total_captures should be a number")
	}

	// Verify anchor structure
	anchors := diagMap["cooldown_anchors"].(map[string]interface{})
	peerAlphaAnchor, exists := anchors["peer-alpha"]
	if !exists {
		t.Fatal("Expected cooldown_anchors[\"peer-alpha\"] to exist")
	}
	anchorMap := peerAlphaAnchor.(map[string]interface{})

	// Verify anchor provenance fields
	anchorFields := []string{
		"anchor_capture_id",
		"anchor_target_id",
		"anchor_probe_kind",
		"anchor_source",
		"anchor_created_at",
		"anchor_updated_by_status",
		"created_from",
	}

	for _, field := range anchorFields {
		if _, exists := anchorMap[field]; !exists {
			t.Errorf("Missing required anchor field: %s", field)
		}
	}
}
