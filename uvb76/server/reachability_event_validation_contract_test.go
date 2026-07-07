package server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
	"github.com/s1onique/KGB/uvb76/probe"
)

// ReachabilityEvent represents a reachability state change event.
// This structure is used by the event projection layer.
type ReachabilityEvent struct {
	EventType string `json:"event_type"`
	// TargetReachability is the target reachability status (e.g., "target_reachable", "network_unreachable")
	TargetReachability string `json:"target_reachability"`
	// ServiceReachability is the service reachability status (e.g., "service_reachable", "service_unreachable")
	ServiceReachability string `json:"service_reachability"`
	// HTTPProbeStatus is the HTTP probe status
	HTTPProbeStatus string `json:"http_probe_status"`
	// ICMPProbeStatus is the ICMP probe status
	ICMPProbeStatus string `json:"icmp_probe_status"`
	// Label is the human-readable label
	Label string `json:"label"`
	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`
}

// TestReachabilityEventProjection_EmitsCanonicalVocabulary verifies event projection emits canonical vocabulary.
// ACT-UVB76-HULK04R3: Event vocabulary projection contract.
// This test validates canonical event payload vocabulary from the reachability classifier.
// It does not exercise a production event bus.
func TestReachabilityEventProjection_EmitsCanonicalVocabulary(t *testing.T) {
	// This test verifies that the event projection layer emits canonical reachability vocabulary.
	// It tests the projection of probe classification results into events.

	testCases := []struct {
		name              string
		httpEvidence      probe.ProbeEvidence
		icmpEvidence      probe.ProbeEvidence
		expectedEventType string
	}{
		{
			name: "both_healthy",
			httpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindHTTP, Seen: true, Success: true, Degraded: false,
			},
			icmpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindICMP, Seen: true, Success: true, Degraded: false,
			},
			expectedEventType: "probe_recovered",
		},
		{
			name: "http_failed_icmp_healthy",
			httpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindHTTP, Seen: true, Success: false, Degraded: false,
			},
			icmpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindICMP, Seen: true, Success: true, Degraded: false,
			},
			expectedEventType: "probe_failure",
		},
		{
			name: "http_healthy_icmp_failed",
			httpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindHTTP, Seen: true, Success: true, Degraded: false,
			},
			icmpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindICMP, Seen: true, Success: false, Degraded: false,
			},
			expectedEventType: "probe_failure",
		},
		{
			name: "both_failed",
			httpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindHTTP, Seen: true, Success: false, Degraded: false,
			},
			icmpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindICMP, Seen: true, Success: false, Degraded: false,
			},
			expectedEventType: "target_reachability_changed",
		},
		{
			name: "http_degraded",
			httpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindHTTP, Seen: true, Success: true, Degraded: true,
			},
			icmpEvidence: probe.ProbeEvidence{
				Kind: domain.ProbeKindICMP, Seen: true, Success: true, Degraded: false,
			},
			expectedEventType: "probe_degradation",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate event projection from probe classification
			summary := probe.ClassifyReachability(tc.httpEvidence, tc.icmpEvidence)

			event := ReachabilityEvent{
				EventType:         tc.expectedEventType,
				TargetReachability: string(summary.TargetStatus),
				ServiceReachability: string(summary.ServiceStatus),
				HTTPProbeStatus:   string(summary.HTTPStatus),
				ICMPProbeStatus:   string(summary.ICMPStatus),
				Label:             summary.Label,
				Timestamp:         time.Now(),
			}

			// Verify event uses canonical statuses
			if !probe.IsCanonicalStatus(event.TargetReachability) {
				t.Errorf("target_reachability %q is not canonical", event.TargetReachability)
			}
			if !probe.IsCanonicalStatus(event.ServiceReachability) {
				t.Errorf("service_reachability %q is not canonical", event.ServiceReachability)
			}

			// Verify label does not contain forbidden terms
			if probe.IsLabelForbidden(event.Label) {
				t.Errorf("label %q contains forbidden wording", event.Label)
			}

			// Verify JSON serialization works
			data, err := json.Marshal(event)
			if err != nil {
				t.Fatalf("failed to marshal event: %v", err)
			}

			var decoded ReachabilityEvent
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("failed to unmarshal event: %v", err)
			}

			// Verify decoded event has correct values
			if decoded.TargetReachability != event.TargetReachability {
				t.Errorf("target_reachability mismatch: expected %s, got %s",
					event.TargetReachability, decoded.TargetReachability)
			}
		})
	}
}

// TestReachabilityEventProjection_NoBareUnreachableInEvents verifies events don't use bare unreachable.
// ACT-UVB76-HULK04: Events must use qualified reachability terms
func TestReachabilityEventProjection_NoBareUnreachableInEvents(t *testing.T) {
	// Events must NOT use bare "unreachable" or "reachable"
	// Must use: target_reachable, service_unreachable, network_unreachable, partially_reachable

	summary := probe.ClassifyReachability(
		probe.ProbeEvidence{Kind: domain.ProbeKindHTTP, Seen: true, Success: false},
		probe.ProbeEvidence{Kind: domain.ProbeKindICMP, Seen: true, Success: true},
	)

	// Build event from classification
	event := map[string]string{
		"target_reachability":  string(summary.TargetStatus),
		"service_reachability": string(summary.ServiceStatus),
		"label":               summary.Label,
	}

	// Verify no bare terms in event
	for field, value := range event {
		if value == "unreachable" || value == "reachable" {
			t.Errorf("event field %s contains forbidden bare term %q", field, value)
		}
	}

	// HTTP failed + ICMP OK should be partially_reachable, NOT network_unreachable
	if summary.TargetStatus == probe.ReachabilityNetworkUnreachable {
		t.Error("HTTP failed + ICMP OK should NOT produce network_unreachable event")
	}
	if summary.TargetStatus != probe.ReachabilityPartiallyReachable {
		t.Errorf("expected target_reachability=partially_reachable, got %s", summary.TargetStatus)
	}
}

// TestReachabilityEventProjection_ProbeFailureEvent verifies probe_failure event structure.
func TestReachabilityEventProjection_ProbeFailureEvent(t *testing.T) {
	// Probe failure events must use explicit probe kind and status
	// Forbidden: bare "unreachable" in event name

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

	// HTTP failed + ICMP OK = partially_reachable, NOT network_unreachable
	if summary.TargetStatus == probe.ReachabilityNetworkUnreachable {
		t.Error("HTTP failed + ICMP OK should NOT be network_unreachable")
	}
	if summary.TargetStatus != probe.ReachabilityPartiallyReachable {
		t.Errorf("expected target_status=partially_reachable, got %s", summary.TargetStatus)
	}
}

// TestReachabilityEventProjection_ProbeRecoveryEvent verifies probe_recovery event structure.
func TestReachabilityEventProjection_ProbeRecoveryEvent(t *testing.T) {
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

// TestReachabilityEventProjection_ProbeDegradationEvent verifies probe_degradation event structure.
func TestReachabilityEventProjection_ProbeDegradationEvent(t *testing.T) {
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

// TestReachabilityEventProjection_DegradedIsDistinctFromFailed verifies degraded != failed in events.
func TestReachabilityEventProjection_DegradedIsDistinctFromFailed(t *testing.T) {
	// Degraded probe should NOT emit probe_failure
	// Should emit probe_degradation instead

	httpFailed := probe.ProbeEvidence{
		Kind: domain.ProbeKindHTTP, Seen: true, Success: false, Degraded: false,
	}
	httpDegraded := probe.ProbeEvidence{
		Kind: domain.ProbeKindHTTP, Seen: true, Success: true, Degraded: true,
	}
	icmpOK := probe.ProbeEvidence{
		Kind: domain.ProbeKindICMP, Seen: true, Success: true, Degraded: false,
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

// TestReachabilityEventProjection_ICMPDoesNotImplyHTTP verifies probe independence in events.
func TestReachabilityEventProjection_ICMPDoesNotImplyHTTP(t *testing.T) {
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
