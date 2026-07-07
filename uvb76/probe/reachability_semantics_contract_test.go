package probe

import (
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// TestReachabilitySemanticsContract tests the canonical reachability classification matrix.
// Each test covers a specific HTTP/ICMP probe combination and verifies the expected
// canonical reachability status.
//
// Matrix dimensions:
//   - HTTP status: success, failed, degraded, unknown
//   - ICMP status: success, failed, degraded, unknown
//
// Canonical reachability vocabulary:
//   - "target_reachable": host is reachable (ICMP or HTTP success)
//   - "service_reachable": HTTP service is responding
//   - "partially_reachable": one probe succeeds, one fails
//   - "service_unreachable": HTTP service is down
//   - "network_unreachable": both probes fail
//   - "unknown": insufficient data

// TestReachabilitySemanticsContract_HTTPOK_ICMPOK tests HTTP OK + ICMP OK.
func TestReachabilitySemanticsContract_HTTPOK_ICMPOK(t *testing.T) {
	// HTTP success + ICMP success = "target_reachable" / "service_reachable"
	http := ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: time.Now(),
	}
	icmp := ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: time.Now(),
	}

	summary := ClassifyReachability(http, icmp)

	if summary.TargetStatus != ReachabilityTargetReachable {
		t.Errorf("expected target_status=\"target_reachable\", got %s", summary.TargetStatus)
	}
	if summary.ServiceStatus != ReachabilityServiceReachable {
		t.Errorf("expected service_status=\"service_reachable\", got %s", summary.ServiceStatus)
	}
	if summary.HTTPStatus != ProbeStatusSuccess {
		t.Errorf("expected http_status=success, got %s", summary.HTTPStatus)
	}
	if summary.ICMPStatus != ProbeStatusSuccess {
		t.Errorf("expected icmp_status=success, got %s", summary.ICMPStatus)
	}
	if summary.Label != "OK · OK" {
		t.Errorf("expected label='OK · OK', got %q", summary.Label)
	}
}

// TestReachabilitySemanticsContract_HTTPOK_ICMPFailed tests HTTP OK + ICMP failed.
func TestReachabilitySemanticsContract_HTTPOK_ICMPFailed(t *testing.T) {
	// HTTP success + ICMP failed = "partially_reachable" / "service_reachable"
	// NOT "network_unreachable" - HTTP proves the network path works
	http := ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: time.Now(),
	}
	icmp := ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   false,
		Degraded:  false,
		Timestamp: time.Now(),
	}

	summary := ClassifyReachability(http, icmp)

	if summary.TargetStatus != ReachabilityPartiallyReachable {
		t.Errorf("expected target_status=\"partially_reachable\", got %s", summary.TargetStatus)
	}
	if summary.ServiceStatus != ReachabilityServiceReachable {
		t.Errorf("expected service_status=\"service_reachable\", got %s", summary.ServiceStatus)
	}
	if summary.HTTPStatus != ProbeStatusSuccess {
		t.Errorf("expected http_status=success, got %s", summary.HTTPStatus)
	}
	if summary.ICMPStatus != ProbeStatusFailed {
		t.Errorf("expected icmp_status=failed, got %s", summary.ICMPStatus)
	}
	if summary.Label != "OK · failing" {
		t.Errorf("expected label='OK · failing', got %q", summary.Label)
	}
}

// TestReachabilitySemanticsContract_HTTPFailed_ICMPOK tests HTTP failed + ICMP OK.
func TestReachabilitySemanticsContract_HTTPFailed_ICMPOK(t *testing.T) {
	// HTTP failed + ICMP success = "partially_reachable" / "service_unreachable"
	// NOT "network_unreachable" - ICMP proves the host is up
	// NOT "service_unreachable" without partial qualifier
	http := ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   false,
		Degraded:  false,
		Timestamp: time.Now(),
	}
	icmp := ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: time.Now(),
	}

	summary := ClassifyReachability(http, icmp)

	if summary.TargetStatus != ReachabilityPartiallyReachable {
		t.Errorf("expected target_status=\"partially_reachable\", got %s", summary.TargetStatus)
	}
	if summary.ServiceStatus != ReachabilityServiceUnreachable {
		t.Errorf("expected service_status=\"service_unreachable\", got %s", summary.ServiceStatus)
	}
	if summary.HTTPStatus != ProbeStatusFailed {
		t.Errorf("expected http_status=failed, got %s", summary.HTTPStatus)
	}
	if summary.ICMPStatus != ProbeStatusSuccess {
		t.Errorf("expected icmp_status=success, got %s", summary.ICMPStatus)
	}
	if summary.Label != "failing · OK" {
		t.Errorf("expected label='failing · OK', got %q", summary.Label)
	}
}

// TestReachabilitySemanticsContract_HTTPFailed_ICMPFailed tests both probes failed.
func TestReachabilitySemanticsContract_HTTPFailed_ICMPFailed(t *testing.T) {
	// Both failed = "network_unreachable" / "service_unreachable"
	http := ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   false,
		Degraded:  false,
		Timestamp: time.Now(),
	}
	icmp := ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   false,
		Degraded:  false,
		Timestamp: time.Now(),
	}

	summary := ClassifyReachability(http, icmp)

	if summary.TargetStatus != ReachabilityNetworkUnreachable {
		t.Errorf("expected target_status=\"network_unreachable\", got %s", summary.TargetStatus)
	}
	if summary.ServiceStatus != ReachabilityServiceUnreachable {
		t.Errorf("expected service_status=\"service_unreachable\", got %s", summary.ServiceStatus)
	}
	if summary.Label != "failing · failing" {
		t.Errorf("expected label='failing · failing', got %q", summary.Label)
	}
}

// TestReachabilitySemanticsContract_HTTPUnknown_ICMPOK tests HTTP unknown + ICMP OK.
func TestReachabilitySemanticsContract_HTTPUnknown_ICMPOK(t *testing.T) {
	// HTTP unknown + ICMP success = "target_reachable" / "unknown"
	http := ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      false,
		Timestamp: time.Time{},
	}
	icmp := ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: time.Now(),
	}

	summary := ClassifyReachability(http, icmp)

	if summary.TargetStatus != ReachabilityTargetReachable {
		t.Errorf("expected target_status=\"target_reachable\", got %s", summary.TargetStatus)
	}
	if summary.ServiceStatus != ReachabilityUnknown {
		t.Errorf("expected service_status=\"unknown\", got %s", summary.ServiceStatus)
	}
	if summary.HTTPStatus != ProbeStatusUnknown {
		t.Errorf("expected http_status=unknown, got %s", summary.HTTPStatus)
	}
	if summary.ICMPStatus != ProbeStatusSuccess {
		t.Errorf("expected icmp_status=success, got %s", summary.ICMPStatus)
	}
	if summary.Label != "unknown · OK" {
		t.Errorf("expected label='unknown · OK', got %q", summary.Label)
	}
}

// TestReachabilitySemanticsContract_HTTPOK_ICMPUnknown tests HTTP OK + ICMP unknown.
func TestReachabilitySemanticsContract_HTTPOK_ICMPUnknown(t *testing.T) {
	// HTTP OK + ICMP unknown = "target_reachable" / "service_reachable"
	http := ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: time.Now(),
	}
	icmp := ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      false,
		Timestamp: time.Time{},
	}

	summary := ClassifyReachability(http, icmp)

	if summary.TargetStatus != ReachabilityTargetReachable {
		t.Errorf("expected target_status=\"target_reachable\", got %s", summary.TargetStatus)
	}
	if summary.ServiceStatus != ReachabilityServiceReachable {
		t.Errorf("expected service_status=\"service_reachable\", got %s", summary.ServiceStatus)
	}
	if summary.Label != "OK · unknown" {
		t.Errorf("expected label='OK · unknown', got %q", summary.Label)
	}
}

// TestReachabilitySemanticsContract_HTTPUnknown_ICMPUnknown tests both unknown.
func TestReachabilitySemanticsContract_HTTPUnknown_ICMPUnknown(t *testing.T) {
	// Both unknown = "unknown" / "unknown"
	http := ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      false,
		Timestamp: time.Time{},
	}
	icmp := ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      false,
		Timestamp: time.Time{},
	}

	summary := ClassifyReachability(http, icmp)

	if summary.TargetStatus != ReachabilityUnknown {
		t.Errorf("expected target_status=\"unknown\", got %s", summary.TargetStatus)
	}
	if summary.ServiceStatus != ReachabilityUnknown {
		t.Errorf("expected service_status=\"unknown\", got %s", summary.ServiceStatus)
	}
	if summary.Label != "No recent probe data" {
		t.Errorf("expected label='No recent probe data', got %q", summary.Label)
	}
}

// TestReachabilitySemanticsContract_HTTPDegraded_ICMPOK tests HTTP degraded + ICMP OK.
func TestReachabilitySemanticsContract_HTTPDegraded_ICMPOK(t *testing.T) {
	// HTTP degraded + ICMP success = "partially_reachable" / "service_reachable"
	// Degraded is NOT the same as failed
	http := ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   true,
		Degraded:  true,
		Timestamp: time.Now(),
	}
	icmp := ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: time.Now(),
	}

	summary := ClassifyReachability(http, icmp)

	if summary.TargetStatus != ReachabilityPartiallyReachable {
		t.Errorf("expected target_status=\"partially_reachable\", got %s", summary.TargetStatus)
	}
	if summary.ServiceStatus != ReachabilityServiceReachable {
		t.Errorf("expected service_status=\"service_reachable\", got %s", summary.ServiceStatus)
	}
	if summary.HTTPStatus != ProbeStatusDegraded {
		t.Errorf("expected http_status=degraded, got %s", summary.HTTPStatus)
	}
	if summary.ICMPStatus != ProbeStatusSuccess {
		t.Errorf("expected icmp_status=success, got %s", summary.ICMPStatus)
	}
	if summary.Label != "degraded · OK" {
		t.Errorf("expected label='degraded · OK', got %q", summary.Label)
	}
}

// TestReachabilitySemanticsContract_HTTPOK_ICMPDegraded tests HTTP OK + ICMP degraded.
func TestReachabilitySemanticsContract_HTTPOK_ICMPDegraded(t *testing.T) {
	// HTTP OK + ICMP degraded = "partially_reachable" / "service_reachable"
	// Degraded is NOT the same as failed
	http := ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: time.Now(),
	}
	icmp := ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   true,
		Degraded:  true,
		Timestamp: time.Now(),
	}

	summary := ClassifyReachability(http, icmp)

	if summary.TargetStatus != ReachabilityPartiallyReachable {
		t.Errorf("expected target_status=\"partially_reachable\", got %s", summary.TargetStatus)
	}
	if summary.ServiceStatus != ReachabilityServiceReachable {
		t.Errorf("expected service_status=\"service_reachable\", got %s", summary.ServiceStatus)
	}
	if summary.HTTPStatus != ProbeStatusSuccess {
		t.Errorf("expected http_status=success, got %s", summary.HTTPStatus)
	}
	if summary.ICMPStatus != ProbeStatusDegraded {
		t.Errorf("expected icmp_status=degraded, got %s", summary.ICMPStatus)
	}
	if summary.Label != "OK · degraded" {
		t.Errorf("expected label='OK · degraded', got %q", summary.Label)
	}
}

// TestReachabilitySemanticsContract_CanonicalTermPresence verifies canonical strings are present.
func TestReachabilitySemanticsContract_CanonicalTermPresence(t *testing.T) {
	// This test verifies that canonical string literals are defined and accessible.
	// Canonical terms: "target_reachable", "service_reachable", "partially_reachable",
	// "service_unreachable", "network_unachable", "unknown"
	canonicalTerms := []string{
		"target_reachable",
		"service_reachable",
		"partially_reachable",
		"service_unreachable",
		"network_unreachable",
		"unknown",
	}

	// Verify each canonical term exists in the constants
	for _, term := range canonicalTerms {
		if !IsCanonicalStatus(term) {
			t.Errorf("canonical term %q should be recognized", term)
		}
	}
}
