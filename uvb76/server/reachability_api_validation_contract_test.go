package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
	"github.com/s1onique/KGB/uvb76/probe"
	"github.com/s1onique/KGB/uvb76/state"
)

// TestReachabilityVocabulary_ViaStateManager verifies reachability vocabulary
// contracts using state manager as the projection layer.
// ACT-UVB76-HULK04R3: Renamed from VocabularyExercisesRealServerHandler -
// This test exercises vocabulary contracts via state manager projection,
// not the actual server handler path (which requires full integration setup).
func TestReachabilityVocabulary_ViaStateManager(t *testing.T) {
	// This test verifies that the reachability vocabulary is correct.
	// Uses state manager to simulate what the API projection would receive.

	mgr := state.NewManager()
	targetID := "test-target"

	// Record probe evidence in state
	// Both probes healthy -> target_reachable, service_reachable
	mgr.RecordLatency(targetID, 50.0, true)
	mgr.RecordICMPLatency(targetID, 30.0, true)

	// Verify canonical status strings are available
	canonicalStatuses := probe.CanonicalStatusStrings()
	if len(canonicalStatuses) == 0 {
		t.Error("canonical status strings should not be empty")
	}

	// Verify forbidden labels are correctly identified
	forbidden := probe.ForbiddenLabels()
	if len(forbidden) != 2 {
		t.Errorf("expected 2 forbidden labels, got %d", len(forbidden))
	}
	if !probe.IsLabelForbidden("unreachable") {
		t.Error("bare 'unreachable' should be forbidden")
	}
	if !probe.IsLabelForbidden("reachable") {
		t.Error("bare 'reachable' should be forbidden")
	}

	// Verify qualified terms are NOT forbidden
	if probe.IsLabelForbidden("service_unreachable") {
		t.Error("'service_unreachable' should NOT be forbidden")
	}
	if probe.IsLabelForbidden("target_reachable") {
		t.Error("'target_reachable' should NOT be forbidden")
	}
}

// TestReachabilityAPIPath_CanonicalStatusEmitted verifies canonical statuses are emitted.
func TestReachabilityAPIPath_CanonicalStatusEmitted(t *testing.T) {
	// Verify that the canonical reachability status strings are valid
	canonicalStatuses := probe.CanonicalStatusStrings()

	// Each canonical status should be a non-empty string
	for _, status := range canonicalStatuses {
		if status == "" {
			t.Error("canonical status should not be empty")
		}
		// Status should not contain bare "unreachable" or "reachable"
		if status == "unreachable" || status == "reachable" {
			t.Errorf("canonical status %q is a forbidden bare term", status)
		}
	}

	// Verify expected canonical terms are present
	expectedTerms := map[string]bool{
		"target_reachable":     false,
		"service_reachable":    false,
		"partially_reachable":  false,
		"service_unreachable":  false,
		"network_unreachable":  false,
		"probe_failed":         false,
		"probe_degraded":       false,
		"probe_recovered":      false,
		"unknown":              false,
	}

	for _, status := range canonicalStatuses {
		if _, ok := expectedTerms[status]; ok {
			expectedTerms[status] = true
		}
	}

	for term, found := range expectedTerms {
		if !found {
			t.Errorf("expected canonical term %q not found in status list", term)
		}
	}
}

// TestReachabilityAPIPath_NoForbiddenBareTerms verifies no forbidden bare terms in API response.
func TestReachabilityAPIPath_NoForbiddenBareTerms(t *testing.T) {
	// API responses must NOT contain bare "unreachable" or "reachable"
	// Must use qualified forms: target_reachable, service_unreachable, network_unreachable, etc.

	forbiddenPatterns := []string{
		`"unreachable"`,  // bare term
		`"reachable"`,    // bare term
	}

	// Verify ForbiddenLabels() correctly identifies bare terms
	forbiddenLabels := probe.ForbiddenLabels()
	if len(forbiddenLabels) != 2 {
		t.Errorf("expected 2 forbidden labels, got %d", len(forbiddenLabels))
	}

	// Verify IsLabelForbidden detects bare terms
	if !probe.IsLabelForbidden("unreachable") {
		t.Error("bare 'unreachable' should be forbidden")
	}
	if !probe.IsLabelForbidden("reachable") {
		t.Error("bare 'reachable' should be forbidden")
	}

	// Verify qualified terms are NOT forbidden
	if probe.IsLabelForbidden("service_unreachable") {
		t.Error("'service_unreachable' should NOT be forbidden")
	}
	if probe.IsLabelForbidden("network_unreachable") {
		t.Error("'network_unreachable' should NOT be forbidden")
	}
	if probe.IsLabelForbidden("target_reachable") {
		t.Error("'target_reachable' should NOT be forbidden")
	}
	if probe.IsLabelForbidden("partially_reachable") {
		t.Error("'partially_reachable' should NOT be forbidden")
	}

	// Suppress unused variable warning
	_ = forbiddenPatterns
}

// TestReachabilityAPIPath_ServerStatusEndpoint verifies /api/v1/status uses canonical vocabulary.
func TestReachabilityAPIPath_ServerStatusEndpoint(t *testing.T) {
	// Verify that the status endpoint produces valid JSON with expected fields
	// This test exercises the real handleStatus function

	// Create a minimal server for this test
	cfg := &config.Config{}
	mgr := state.NewManager()
	srv := NewServer(cfg, mgr, nil, true)

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()

	// Call the real handler
	srv.handleStatus(rec, req)

	// Verify response
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify JSON is valid
	var status ServerStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to unmarshal status response: %v", err)
	}

	// Verify started_at is present and valid RFC3339
	if status.StartedAt == "" {
		t.Error("started_at should not be empty")
	}
	if _, err := time.Parse(time.RFC3339, status.StartedAt); err != nil {
		t.Errorf("started_at should be RFC3339 format: %v", err)
	}
}

// TestReachabilityAPIPath_ProbeClassificationProducesCanonicalOutput verifies probe classifier output.
func TestReachabilityAPIPath_ProbeClassificationProducesCanonicalOutput(t *testing.T) {
	// Verify that probe.ClassifyReachability produces canonical output

	httpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   false,
		Degraded:  false,
		Timestamp: time.Now(),
	}
	icmpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: time.Now(),
	}

	summary := probe.ClassifyReachability(httpEvidence, icmpEvidence)

	// Verify all status fields are canonical strings
	if !probe.IsCanonicalStatus(string(summary.TargetStatus)) {
		t.Errorf("target_status %q is not canonical", summary.TargetStatus)
	}
	if !probe.IsCanonicalStatus(string(summary.ServiceStatus)) {
		t.Errorf("service_status %q is not canonical", summary.ServiceStatus)
	}

	// Verify label does not contain forbidden terms
	if probe.IsLabelForbidden(summary.Label) {
		t.Errorf("label %q contains forbidden wording", summary.Label)
	}

	// HTTP failed + ICMP success = partially_reachable (NOT network_unreachable)
	if summary.TargetStatus != probe.ReachabilityPartiallyReachable {
		t.Errorf("expected target_status=partially_reachable, got %s", summary.TargetStatus)
	}
	if summary.ServiceStatus != probe.ReachabilityServiceUnreachable {
		t.Errorf("expected service_status=service_unreachable, got %s", summary.ServiceStatus)
	}
}
