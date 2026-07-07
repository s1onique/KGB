package probe

import (
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// TestReachabilitySemanticsContract_DegradedNotEqualFailed verifies degraded ≠ failed.
func TestReachabilitySemanticsContract_DegradedNotEqualFailed(t *testing.T) {
	// HTTP degraded + ICMP OK
	degradedHTTP := ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   true, // Still successful, just slow
		Degraded:  true,
		Timestamp: time.Now(),
	}
	degradedICMP := ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: time.Now(),
	}
	degradedSummary := ClassifyReachability(degradedHTTP, degradedICMP)

	// HTTP failed + ICMP OK
	failedHTTP := ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   false,
		Degraded:  false,
		Timestamp: time.Now(),
	}
	failedSummary := ClassifyReachability(failedHTTP, degradedICMP)

	// HTTP degraded + ICMP OK = partially_reachable
	// HTTP failed + ICMP OK = partially_reachable / service_unreachable
	// These MUST be different service statuses
	if degradedSummary.ServiceStatus == failedSummary.ServiceStatus {
		t.Errorf("degraded and failed should have different service_status: degraded=%s, failed=%s",
			degradedSummary.ServiceStatus, failedSummary.ServiceStatus)
	}

	if degradedSummary.HTTPStatus == ProbeStatusFailed {
		t.Errorf("degraded probe should NOT have http_status=failed, got %s", degradedSummary.HTTPStatus)
	}
}

// TestReachabilitySemanticsContract_CanonicalStatusStrings verifies canonical status strings.
func TestReachabilitySemanticsContract_CanonicalStatusStrings(t *testing.T) {
	canonical := CanonicalStatusStrings()

	// Verify all expected canonical statuses are present
	expected := map[string]bool{
		"target_reachable":    false,
		"service_reachable":   false,
		"partially_reachable": false,
		"service_unreachable": false,
		"network_unreachable": false,
		"probe_failed":       false,
		"probe_degraded":     false,
		"probe_recovered":    false,
		"unknown":           false,
	}

	for _, s := range canonical {
		if _, ok := expected[s]; ok {
			expected[s] = true
		}
	}

	for s, found := range expected {
		if !found {
			t.Errorf("canonical status %q not found in CanonicalStatusStrings()", s)
		}
	}
}

// TestReachabilitySemanticsContract_IsCanonicalStatus verifies status validation.
func TestReachabilitySemanticsContract_IsCanonicalStatus(t *testing.T) {
	// Valid canonical statuses
	validStatuses := []string{
		"target_reachable",
		"service_reachable",
		"partially_reachable",
		"service_unreachable",
		"network_unreachable",
		"probe_failed",
		"probe_degraded",
		"probe_recovered",
		"unknown",
	}

	for _, s := range validStatuses {
		if !IsCanonicalStatus(s) {
			t.Errorf("expected %q to be canonical", s)
		}
	}

	// Invalid/bare statuses that should not be emitted
	invalidStatuses := []string{
		"unreachable",
		"reachable",
		"UP",
		"DOWN",
		"ok",
		"error",
	}

	for _, s := range invalidStatuses {
		if IsCanonicalStatus(s) {
			t.Errorf("expected %q to NOT be canonical", s)
		}
	}
}

// TestReachabilitySemanticsContract_IsLabelForbidden verifies label validation.
func TestReachabilitySemanticsContract_IsLabelForbidden(t *testing.T) {
	// Forbidden bare labels
	forbidden := []string{
		"unreachable",
		"reachable",
	}

	for _, label := range forbidden {
		if !IsLabelForbidden(label) {
			t.Errorf("expected label %q to be forbidden", label)
		}
	}

	// Allowed qualified labels
	allowed := []string{
		"service_unreachable",
		"network_unreachable",
		"target_reachable",
		"partially_reachable",
		"OK",
		"OK · OK",
		"failing · OK",
		"HTTP OK · ICMP OK",
		"No recent probe data",
	}

	for _, label := range allowed {
		if IsLabelForbidden(label) {
			t.Errorf("expected label %q to NOT be forbidden", label)
		}
	}
}

// TestReachabilitySemanticsContract_PureFunction verifies classifier is pure.
func TestReachabilitySemanticsContract_PureFunction(t *testing.T) {
	// The classifier must be deterministic and not depend on wall clock
	http := ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}
	icmp := ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	// Call multiple times - should always return same result
	for i := 0; i < 10; i++ {
		summary := ClassifyReachability(http, icmp)
		if summary.TargetStatus != ReachabilityTargetReachable {
			t.Errorf("call %d: expected target_reachable, got %s", i, summary.TargetStatus)
		}
		if summary.ServiceStatus != ReachabilityServiceReachable {
			t.Errorf("call %d: expected service_reachable, got %s", i, summary.ServiceStatus)
		}
	}
}
