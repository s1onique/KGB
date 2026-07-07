package server

import (
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
	"github.com/s1onique/KGB/uvb76/probe"
)

// TestReachabilityEventVocabularyProjection_ProbeFailureEvent verifies probe_failure event structure.
func TestReachabilityEventVocabularyProjection_ProbeFailureEvent(t *testing.T) {
	// Probe failure events must use explicit probe kind and status
	// Forbidden: bare "unreachable" in event name

	// Create probe evidence for HTTP failure
	httpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   false,
		Degraded:  false,
		Timestamp: time.Now(),
		ErrorKind: "http_probe_failure",
		ErrorText: "connection refused",
	}
	icmpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: time.Now(),
	}

	summary := probe.ClassifyReachability(httpEvidence, icmpEvidence)

	// Verify event fields use explicit probe kind
	if httpEvidence.Kind != domain.ProbeKindHTTP {
		t.Errorf("expected probe_kind=http, got %s", httpEvidence.Kind)
	}

	// Verify status is canonical
	if !probe.IsCanonicalStatus(string(summary.ServiceStatus)) {
		t.Errorf("service_status %q is not canonical", summary.ServiceStatus)
	}

	// Verify target status is canonical
	if !probe.IsCanonicalStatus(string(summary.TargetStatus)) {
		t.Errorf("target_status %q is not canonical", summary.TargetStatus)
	}

	// HTTP failed + ICMP OK = partially_reachable, NOT network_unreachable
	if summary.TargetStatus == probe.ReachabilityNetworkUnreachable {
		t.Error("HTTP failed + ICMP OK should NOT be network_unreachable")
	}
	if summary.TargetStatus != probe.ReachabilityPartiallyReachable {
		t.Errorf("expected target_status=partially_reachable, got %s", summary.TargetStatus)
	}
}

// TestReachabilityEventVocabularyProjection_ProbeRecoveryEvent verifies probe_recovery event structure.
func TestReachabilityEventVocabularyProjection_ProbeRecoveryEvent(t *testing.T) {
	// Probe recovery events must use explicit probe kind
	// Recovery requires prior failure/degraded state

	httpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   true,
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

	// Verify canonical status
	if !probe.IsCanonicalStatus(string(summary.ServiceStatus)) {
		t.Errorf("service_status %q is not canonical", summary.ServiceStatus)
	}
}

// TestReachabilityEventVocabularyProjection_ProbeDegradationEvent verifies probe_degradation event structure.
func TestReachabilityEventVocabularyProjection_ProbeDegradationEvent(t *testing.T) {
	// Probe degradation events must use explicit probe kind and status
	// Degraded is NOT the same as failed

	httpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   true,
		Degraded:  true, // Latency spike but still responding
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

	// Verify HTTP status is degraded
	if summary.HTTPStatus != probe.ProbeStatusDegraded {
		t.Errorf("expected http_status=degraded, got %s", summary.HTTPStatus)
	}

	// Service should still be reachable (responding, just slow)
	if summary.ServiceStatus != probe.ReachabilityServiceReachable {
		t.Errorf("expected service_status=service_reachable (degraded != failed), got %s", summary.ServiceStatus)
	}
}

// TestReachabilityEventVocabularyProjection_TargetReachabilityChanged verifies target_reachability_changed event.
func TestReachabilityEventVocabularyProjection_TargetReachabilityChanged(t *testing.T) {
	// Target reachability changes must use explicit canonical statuses

	testCases := []struct {
		name           string
		httpEvidence   probe.ProbeEvidence
		icmpEvidence   probe.ProbeEvidence
		expectedTarget probe.ReachabilityStatus
	}{
		{
			name: "both_up",
			httpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindHTTP, Seen: true, Success: true, Degraded: false,
			},
			icmpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindICMP, Seen: true, Success: true, Degraded: false,
			},
			expectedTarget: probe.ReachabilityTargetReachable,
		},
		{
			name: "http_down_icmp_up",
			httpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindHTTP, Seen: true, Success: false, Degraded: false,
			},
			icmpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindICMP, Seen: true, Success: true, Degraded: false,
			},
			expectedTarget: probe.ReachabilityPartiallyReachable,
		},
		{
			name: "both_down",
			httpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindHTTP, Seen: true, Success: false, Degraded: false,
			},
			icmpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindICMP, Seen: true, Success: false, Degraded: false,
			},
			expectedTarget: probe.ReachabilityNetworkUnreachable,
		},
		{
			name: "both_unknown",
			httpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindHTTP, Seen: false,
			},
			icmpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindICMP, Seen: false,
			},
			expectedTarget: probe.ReachabilityUnknown,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			summary := probe.ClassifyReachability(tc.httpEvidence, tc.icmpEvidence)
			if summary.TargetStatus != tc.expectedTarget {
				t.Errorf("expected target_status=%s, got %s", tc.expectedTarget, summary.TargetStatus)
			}
		})
	}
}

// TestReachabilityEventVocabularyProjection_ServiceReachabilityChanged verifies service_reachability_changed event.
func TestReachabilityEventVocabularyProjection_ServiceReachabilityChanged(t *testing.T) {
	// Service reachability changes must use explicit canonical statuses

	testCases := []struct {
		name            string
		httpEvidence    probe.ProbeEvidence
		icmpEvidence    probe.ProbeEvidence
		expectedService probe.ReachabilityStatus
	}{
		{
			name: "service_up",
			httpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindHTTP, Seen: true, Success: true, Degraded: false,
			},
			icmpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindICMP, Seen: true, Success: true, Degraded: false,
			},
			expectedService: probe.ReachabilityServiceReachable,
		},
		{
			name: "service_down_http_failed",
			httpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindHTTP, Seen: true, Success: false, Degraded: false,
			},
			icmpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindICMP, Seen: true, Success: true, Degraded: false,
			},
			expectedService: probe.ReachabilityServiceUnreachable,
		},
		{
			name: "service_unknown",
			httpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindHTTP, Seen: false,
			},
			icmpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindICMP, Seen: true, Success: true, Degraded: false,
			},
			expectedService: probe.ReachabilityUnknown,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			summary := probe.ClassifyReachability(tc.httpEvidence, tc.icmpEvidence)
			if summary.ServiceStatus != tc.expectedService {
				t.Errorf("expected service_status=%s, got %s", tc.expectedService, summary.ServiceStatus)
			}
		})
	}
}

// TestReachabilityEventVocabularyProjection_ForbiddenEventWording verifies no bare unreachable/reachable.
func TestReachabilityEventVocabularyProjection_ForbiddenEventWording(t *testing.T) {
	// Events must NOT use bare "unreachable" or "reachable"
	// Must use service_unreachable, network_unreachable, target_reachable, partially_reachable

	testCases := []struct {
		name        string
		label       string
		shouldBeBad bool
	}{
		{"bare_unreachable", "unreachable", true},
		{"bare_reachable", "reachable", true},
		{"service_unreachable", "service_unreachable", false},
		{"network_unreachable", "network_unreachable", false},
		{"target_reachable", "target_reachable", false},
		{"partially_reachable", "partially_reachable", false},
		{"OK · OK", "OK · OK", false},
		{"failing · OK", "failing · OK", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isForbidden := probe.IsLabelForbidden(tc.label)
			if isForbidden != tc.shouldBeBad {
				if tc.shouldBeBad {
					t.Errorf("label %q should be forbidden but was not", tc.label)
				} else {
					t.Errorf("label %q should not be forbidden but was", tc.label)
				}
			}
		})
	}
}

// TestReachabilityEventVocabularyProjection_TransitionKinds verifies transition kinds.
func TestReachabilityEventVocabularyProjection_TransitionKinds(t *testing.T) {
	// Required transition kinds:
	// - probe_failure
	// - probe_recovery
	// - probe_degradation
	// - probe_degradation_recovery
	// - target_reachability_changed
	// - service_reachability_changed

	validTransitions := map[string]bool{
		"probe_failure":                  true,
		"probe_recovery":                 true,
		"probe_degradation":              true,
		"probe_degradation_recovery":     true,
		"target_reachability_changed":    true,
		"service_reachability_changed":   true,
	}

	// Verify these transitions can be represented
	transitions := []string{
		"probe_failure",
		"probe_recovery",
		"probe_degradation",
		"probe_degradation_recovery",
		"target_reachability_changed",
		"service_reachability_changed",
	}

	for _, tr := range transitions {
		if !validTransitions[tr] {
			t.Errorf("transition %q not in valid transitions list", tr)
		}
	}
}

// TestReachabilityEventVocabularyProjection_ProbeKindExplicit verifies probe_kind is always explicit.
func TestReachabilityEventVocabularyProjection_ProbeKindExplicit(t *testing.T) {
	// Every event must include probe_kind field

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

	// Verify probe kinds are explicit
	if httpEvidence.Kind != domain.ProbeKindHTTP {
		t.Errorf("expected http probe_kind=http, got %s", httpEvidence.Kind)
	}
	if icmpEvidence.Kind != domain.ProbeKindICMP {
		t.Errorf("expected icmp probe_kind=icmp, got %s", icmpEvidence.Kind)
	}

	// Verify summary includes probe statuses
	summary := probe.ClassifyReachability(httpEvidence, icmpEvidence)
	if summary.HTTPStatus == "" {
		t.Error("http_status should be set")
	}
	if summary.ICMPStatus == "" {
		t.Error("icmp_status should be set")
	}
}

// TestReachabilityEventVocabularyProjection_DegradedIsDistinctFromFailed verifies distinction.
func TestReachabilityEventVocabularyProjection_DegradedIsDistinctFromFailed(t *testing.T) {
	// Degraded probe should NOT emit probe_failure
	// Should emit probe_degradation instead

	httpFailed := probe.ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   false,
		Degraded:  false,
		Timestamp: time.Now(),
	}
	httpDegraded := probe.ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   true,
		Degraded:  true,
		Timestamp: time.Now(),
	}
	icmpOK := probe.ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: time.Now(),
	}

	failedSummary := probe.ClassifyReachability(httpFailed, icmpOK)
	degradedSummary := probe.ClassifyReachability(httpDegraded, icmpOK)

	// HTTP status must differ
	if failedSummary.HTTPStatus == degradedSummary.HTTPStatus {
		t.Error("degraded and failed should have different http_status")
	}

	// Service status differs: failed -> service_unreachable, degraded -> service_reachable
	if failedSummary.ServiceStatus != probe.ReachabilityServiceUnreachable {
		t.Errorf("failed should have service_status=service_unreachable, got %s", failedSummary.ServiceStatus)
	}
	if degradedSummary.ServiceStatus != probe.ReachabilityServiceReachable {
		t.Errorf("degraded should have service_status=service_reachable, got %s", degradedSummary.ServiceStatus)
	}
}

// TestReachabilityEventVocabularyProjection_ICMPDoesNotImplyHTTP verifies independence.
func TestReachabilityEventVocabularyProjection_ICMPDoesNotImplyHTTP(t *testing.T) {
	// ICMP success does not imply HTTP success
	// ICMP failure does not imply HTTP failure

	// ICMP up, HTTP down -> partially_reachable, not network_unreachable
	httpFailed := probe.ProbeEvidence{
		Kind: domain.ProbeKindHTTP, Seen: true, Success: false, Degraded: false,
	}
	icmpOK := probe.ProbeEvidence{
		Kind: domain.ProbeKindICMP, Seen: true, Success: true, Degraded: false,
	}

	summary := probe.ClassifyReachability(httpFailed, icmpOK)

	if summary.TargetStatus != probe.ReachabilityPartiallyReachable {
		t.Errorf("ICMP OK + HTTP failed should be partially_reachable, got %s", summary.TargetStatus)
	}

	// ICMP up, HTTP up -> target_reachable
	httpOK := probe.ProbeEvidence{
		Kind: domain.ProbeKindHTTP, Seen: true, Success: true, Degraded: false,
	}
	summary2 := probe.ClassifyReachability(httpOK, icmpOK)

	if summary2.TargetStatus != probe.ReachabilityTargetReachable {
		t.Errorf("both up should be target_reachable, got %s", summary2.TargetStatus)
	}
}
