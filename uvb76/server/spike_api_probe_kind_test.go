package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/auth"
	"github.com/s1onique/KGB/uvb76/state"
)

// TestSpikeAPI_ProbeKindHTTP tests that kind=http returns HTTP spikes only.
func TestSpikeAPI_ProbeKindHTTP(t *testing.T) {
	// Create fake tovarisch server
	tovarischServer := httptest.NewServer(fakeTovarischHandler(validTovarischStatusJSON(), http.StatusOK))
	defer tovarischServer.Close()

	// Setup test server
	srv, st, _, _ := setupTestServer(t, tovarischServer.URL)

	// Create router with spike endpoint
	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes)).Methods(http.MethodGet)

	now := time.Now().UTC()
	previousSamples := make([]state.LatencySample, 25)
	for i := 0; i < 25; i++ {
		previousSamples[i] = state.LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		}
	}

	// Create HTTP spike
	httpSpikeTime := now.Add(-10 * time.Second)
	httpSpike := RecordSpikeForTest(st, "test-target", "http", 1500.0, httpSpikeTime, true, nil, nil, nil, previousSamples)
	if httpSpike == nil {
		t.Fatal("expected HTTP spike to be detected")
	}

	// Create ICMP spike
	icmpSpikeTime := now.Add(-5 * time.Second)
	icmpSpike := RecordSpikeForTest(st, "test-target", "icmp", 2500.0, icmpSpikeTime, true, nil, nil, nil, previousSamples)
	if icmpSpike == nil {
		t.Fatal("expected ICMP spike to be detected")
	}

	// Request HTTP spikes only
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&kind=http", nil)
	token, _ := srv.sessions.GenerateToken("admin")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp SpikeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should have exactly 1 spike (HTTP)
	if resp.Count != 1 {
		t.Errorf("expected 1 spike, got %d", resp.Count)
	}

	// Verify it's the HTTP spike
	if len(resp.Spikes) > 0 && resp.Spikes[0].Kind != "http" {
		t.Errorf("expected HTTP spike, got %s", resp.Spikes[0].Kind)
	}
}

// TestSpikeAPI_ProbeKindICMP tests that kind=icmp returns ICMP spikes only.
func TestSpikeAPI_ProbeKindICMP(t *testing.T) {
	// Create fake tovarisch server
	tovarischServer := httptest.NewServer(fakeTovarischHandler(validTovarischStatusJSON(), http.StatusOK))
	defer tovarischServer.Close()

	// Setup test server
	srv, st, _, _ := setupTestServer(t, tovarischServer.URL)

	// Create router with spike endpoint
	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes)).Methods(http.MethodGet)

	now := time.Now().UTC()
	previousSamples := make([]state.LatencySample, 25)
	for i := 0; i < 25; i++ {
		previousSamples[i] = state.LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		}
	}

	// Create HTTP spike
	httpSpikeTime := now.Add(-10 * time.Second)
	httpSpike := RecordSpikeForTest(st, "test-target", "http", 1500.0, httpSpikeTime, true, nil, nil, nil, previousSamples)
	if httpSpike == nil {
		t.Fatal("expected HTTP spike to be detected")
	}

	// Create ICMP spike
	icmpSpikeTime := now.Add(-5 * time.Second)
	icmpSpike := RecordSpikeForTest(st, "test-target", "icmp", 2500.0, icmpSpikeTime, true, nil, nil, nil, previousSamples)
	if icmpSpike == nil {
		t.Fatal("expected ICMP spike to be detected")
	}

	// Request ICMP spikes only
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&kind=icmp", nil)
	token, _ := srv.sessions.GenerateToken("admin")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp SpikeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should have exactly 1 spike (ICMP)
	if resp.Count != 1 {
		t.Errorf("expected 1 spike, got %d", resp.Count)
	}

	// Verify it's the ICMP spike
	if len(resp.Spikes) > 0 && resp.Spikes[0].Kind != "icmp" {
		t.Errorf("expected ICMP spike, got %s", resp.Spikes[0].Kind)
	}
}

// TestSpikeAPI_ProbeKindDefaultIsHTTP tests that default kind is HTTP when not specified.
func TestSpikeAPI_ProbeKindDefaultIsHTTP(t *testing.T) {
	// Create fake tovarisch server
	tovarischServer := httptest.NewServer(fakeTovarischHandler(validTovarischStatusJSON(), http.StatusOK))
	defer tovarischServer.Close()

	// Setup test server
	srv, st, _, _ := setupTestServer(t, tovarischServer.URL)

	// Create router with spike endpoint
	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes)).Methods(http.MethodGet)

	now := time.Now().UTC()
	previousSamples := make([]state.LatencySample, 25)
	for i := 0; i < 25; i++ {
		previousSamples[i] = state.LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		}
	}

	// Create HTTP spike
	httpSpikeTime := now.Add(-10 * time.Second)
	httpSpike := RecordSpikeForTest(st, "test-target", "http", 1500.0, httpSpikeTime, true, nil, nil, nil, previousSamples)
	if httpSpike == nil {
		t.Fatal("expected HTTP spike to be detected")
	}

	// Create ICMP spike
	icmpSpikeTime := now.Add(-5 * time.Second)
	icmpSpike := RecordSpikeForTest(st, "test-target", "icmp", 2500.0, icmpSpikeTime, true, nil, nil, nil, previousSamples)
	if icmpSpike == nil {
		t.Fatal("expected ICMP spike to be detected")
	}

	// Request without specifying kind (should default to HTTP)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target", nil)
	token, _ := srv.sessions.GenerateToken("admin")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp SpikeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Default should return HTTP spikes
	if resp.Count != 1 {
		t.Errorf("expected 1 spike (default HTTP), got %d", resp.Count)
	}

	if len(resp.Spikes) > 0 && resp.Spikes[0].Kind != "http" {
		t.Errorf("expected default HTTP spike, got %s", resp.Spikes[0].Kind)
	}
}

// TestSpikeAPI_CrossProbe_ICMPAnchorAndHTTPSuppressed tests the full cross-probe scenario:
// - ICMP spike at T creates successful diagnostic capture (anchor)
// - HTTP spike at T+Δ is skipped by cooldown
// - ICMP query returns the ICMP anchor spike
// - HTTP query returns the HTTP skipped cooldown row
func TestSpikeAPI_CrossProbe_ICMPAnchorAndHTTPSuppressed(t *testing.T) {
	// Create fake tovarisch server
	tovarischServer := httptest.NewServer(fakeTovarischHandler(validTovarischStatusJSON(), http.StatusOK))
	defer tovarischServer.Close()

	// Setup test server
	srv, st, _, _ := setupTestServer(t, tovarischServer.URL)

	// Create router with spike endpoint
	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes)).Methods(http.MethodGet)

	now := time.Now().UTC()
	previousSamples := make([]state.LatencySample, 25)
	for i := 0; i < 25; i++ {
		previousSamples[i] = state.LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		}
	}

	// Step 1: Create ICMP anchor capture
	icmpAnchorTime := now.Add(-30 * time.Second)
	icmpSpike := RecordSpikeForTest(st, "test-target", "icmp", 3000.0, icmpAnchorTime, true, nil, nil, nil, previousSamples)
	if icmpSpike == nil {
		t.Fatal("expected ICMP spike to be detected")
	}

	// Record successful ICMP capture and set anchor metadata
	st.GetCaptureStore().AddCapture(icmpSpike.EventID, state.DiagCapture{
		Source:           "tovarisch-peer",
		CaptureStartedAt: icmpAnchorTime,
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
	})

	st.GetCaptureStore().SetLastCaptureAnchor("tovarisch-peer", state.CaptureCooldownAnchor{
		AnchorProbeKind: "icmp",
		AnchorCreatedAt:  icmpAnchorTime,
	})

	// Step 2: Create HTTP spike inside cooldown window
	httpSpikeTime := now.Add(-15 * time.Second)
	httpSpike := RecordSpikeForTest(st, "test-target", "http", 2000.0, httpSpikeTime, true, nil, nil, nil, previousSamples)
	if httpSpike == nil {
		t.Fatal("expected HTTP spike to be detected")
	}

	// Simulate suppressed HTTP capture with cross-probe metadata
	cooldownDecision := st.GetCaptureStore().EvaluateCooldown(httpSpikeTime, "tovarisch-peer", 90)
	cooldownInfo := state.BuildCooldownInfoFromDecision(cooldownDecision, "tovarisch-peer")
	if cooldownInfo != nil {
		cooldownInfo.SuppressedProbeKind = "http"
		cooldownInfo.IsCrossProbeSuppression = cooldownInfo.AnchorProbeKind != "" &&
			cooldownInfo.SuppressedProbeKind != "" &&
			cooldownInfo.AnchorProbeKind != cooldownInfo.SuppressedProbeKind
	}

	capture := state.DiagCapture{
		Source:               "tovarisch-peer",
		CaptureStartedAt:     httpSpikeTime,
		Status:               state.DiagCaptureStatusOK,
		CaptureStatus:        state.CaptureStatusSkippedCooldown,
		SuppressedByCooldown: true,
		CooldownInfo:         cooldownInfo,
	}
	st.GetCaptureStore().AddCapture(httpSpike.EventID, capture)

	// Step 3: Query ICMP spikes - should return the ICMP anchor
	icmpReq := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&kind=icmp&include_captures=true", nil)
	token, _ := srv.sessions.GenerateToken("admin")
	icmpReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	icmpRec := httptest.NewRecorder()
	router.ServeHTTP(icmpRec, icmpReq)

	if icmpRec.Code != http.StatusOK {
		t.Fatalf("ICMP query: expected 200, got %d", icmpRec.Code)
	}

	var icmpResp SpikeResponseWithCaptures
	if err := json.NewDecoder(icmpRec.Body).Decode(&icmpResp); err != nil {
		t.Fatalf("ICMP query: failed to decode response: %v", err)
	}

	// Verify ICMP query returns the anchor spike
	if icmpResp.Count != 1 {
		t.Errorf("ICMP query: expected 1 spike, got %d", icmpResp.Count)
	}
	if len(icmpResp.Spikes) > 0 && icmpResp.Spikes[0].EventID != icmpSpike.EventID {
		t.Errorf("ICMP query: expected ICMP anchor spike %s, got %s", icmpSpike.EventID, icmpResp.Spikes[0].EventID)
	}
	if len(icmpResp.Spikes) > 0 && icmpResp.Spikes[0].Kind != "icmp" {
		t.Errorf("ICMP query: expected kind=icmp, got %s", icmpResp.Spikes[0].Kind)
	}

	// Step 4: Query HTTP spikes - should return the skipped spike
	httpReq := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&kind=http&include_captures=true", nil)
	httpReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	httpRec := httptest.NewRecorder()
	router.ServeHTTP(httpRec, httpReq)

	if httpRec.Code != http.StatusOK {
		t.Fatalf("HTTP query: expected 200, got %d", httpRec.Code)
	}

	var httpResp SpikeResponseWithCaptures
	if err := json.NewDecoder(httpRec.Body).Decode(&httpResp); err != nil {
		t.Fatalf("HTTP query: failed to decode response: %v", err)
	}

	// Verify HTTP query returns the skipped spike
	if httpResp.Count != 1 {
		t.Errorf("HTTP query: expected 1 spike, got %d", httpResp.Count)
	}
	if len(httpResp.Spikes) > 0 && httpResp.Spikes[0].EventID != httpSpike.EventID {
		t.Errorf("HTTP query: expected HTTP spike %s, got %s", httpSpike.EventID, httpResp.Spikes[0].EventID)
	}
	if len(httpResp.Spikes) > 0 && httpResp.Spikes[0].Kind != "http" {
		t.Errorf("HTTP query: expected kind=http, got %s", httpResp.Spikes[0].Kind)
	}

	// Verify the HTTP spike has cooldown info with cross-probe metadata
	if len(httpResp.Spikes) > 0 && len(httpResp.Spikes[0].Captures) > 0 {
		httpCapture := httpResp.Spikes[0].Captures[0]
		if !httpCapture.SuppressedByCooldown {
			t.Error("HTTP spike should be suppressed by cooldown")
		}
		if httpCapture.CooldownInfo == nil {
			t.Fatal("cooldown_info must be present for cross-probe suppressed spike")
		}
		if httpCapture.CooldownInfo.AnchorProbeKind != "icmp" {
			t.Errorf("AnchorProbeKind should be icmp, got %q", httpCapture.CooldownInfo.AnchorProbeKind)
		}
		if httpCapture.CooldownInfo.SuppressedProbeKind != "http" {
			t.Errorf("SuppressedProbeKind should be http, got %q", httpCapture.CooldownInfo.SuppressedProbeKind)
		}
		if !httpCapture.CooldownInfo.IsCrossProbeSuppression {
			t.Error("IsCrossProbeSuppression should be true")
		}
	}
}
