package server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
	"github.com/s1onique/KGB/uvb76/probe"
)

// ReachabilityAPIResponse represents the API response for reachability status.
type ReachabilityAPIResponse struct {
	TargetReachabilityStatus  string    `json:"target_reachability_status"`
	ServiceReachabilityStatus string    `json:"service_reachability_status"`
	HTTPProbeStatus          string    `json:"http_probe_status"`
	ICMPProbeStatus          string    `json:"icmp_probe_status"`
	ReachabilityLabel        string    `json:"reachability_label"`
	ReachabilityReason       string    `json:"reachability_reason"`
	ProbeEvidence            Evidence  `json:"probe_evidence"`
	LastHTTPProbeAt          *string   `json:"last_http_probe_at,omitempty"`
	LastICMPProbeAt          *string   `json:"last_icmp_probe_at,omitempty"`
}

// Evidence contains probe evidence details.
type Evidence struct {
	HTTP HTTPProbeEvidence `json:"http"`
	ICMP ICMPProbeEvidence `json:"icmp"`
}

// HTTPProbeEvidence contains HTTP probe evidence.
type HTTPProbeEvidence struct {
	Seen      bool    `json:"seen"`
	Success   bool    `json:"success"`
	Degraded  bool    `json:"degraded"`
	Timestamp *string `json:"timestamp,omitempty"`
}

// ICMPProbeEvidence contains ICMP probe evidence.
type ICMPProbeEvidence struct {
	Seen      bool    `json:"seen"`
	Success   bool    `json:"success"`
	Degraded  bool    `json:"degraded"`
	Timestamp *string `json:"timestamp,omitempty"`
}

// TestReachabilityAPIVocabularyProjection_BothProbesHealthy tests API response vocabulary for healthy probes.
// ACT-UVB76-HULK04-ALLOW-SKIP: vocabulary projection tests call helper, not real server
func TestReachabilityAPIVocabularyProjection_BothProbesHealthy(t *testing.T) {
	// Both probes healthy -> target_reachable, service_reachable
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

	response := buildReachabilityResponse(httpEvidence, icmpEvidence)

	// Verify JSON is valid
	if response.TargetReachabilityStatus == "" {
		t.Error("target_reachability_status should not be empty")
	}

	// Verify status strings are canonical
	if !probe.IsCanonicalStatus(response.TargetReachabilityStatus) {
		t.Errorf("target_reachability_status %q is not canonical", response.TargetReachabilityStatus)
	}
	if !probe.IsCanonicalStatus(response.ServiceReachabilityStatus) {
		t.Errorf("service_reachability_status %q is not canonical", response.ServiceReachabilityStatus)
	}

	// Verify no ambiguous bare "unreachable" label
	if probe.IsLabelForbidden(response.ReachabilityLabel) {
		t.Errorf("label %q contains forbidden wording", response.ReachabilityLabel)
	}

	// Verify expected values
	if response.TargetReachabilityStatus != string(probe.ReachabilityTargetReachable) {
		t.Errorf("expected target_reachability_status=target_reachable, got %s", response.TargetReachabilityStatus)
	}
	if response.ServiceReachabilityStatus != string(probe.ReachabilityServiceReachable) {
		t.Errorf("expected service_reachability_status=service_reachable, got %s", response.ServiceReachabilityStatus)
	}
}

// TestReachabilityAPIVocabularyProjection_HTTPFailedButICMPHealthy tests partially reachable case.
func TestReachabilityAPIVocabularyProjection_HTTPFailedButICMPHealthy(t *testing.T) {
	// HTTP failed but ICMP healthy -> partially_reachable, service_unreachable
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

	response := buildReachabilityResponse(httpEvidence, icmpEvidence)

	// HTTP failure + ICMP success = partially_reachable, NOT network_unreachable
	if response.TargetReachabilityStatus != string(probe.ReachabilityPartiallyReachable) {
		t.Errorf("expected target_reachability_status=partially_reachable, got %s", response.TargetReachabilityStatus)
	}

	// Service should be unreachable
	if response.ServiceReachabilityStatus != string(probe.ReachabilityServiceUnreachable) {
		t.Errorf("expected service_reachability_status=service_unreachable, got %s", response.ServiceReachabilityStatus)
	}
}

// TestReachabilityAPIVocabularyProjection_HTTPHealthyButICMPFailed tests another partial case.
func TestReachabilityAPIVocabularyProjection_HTTPHealthyButICMPFailed(t *testing.T) {
	// HTTP healthy but ICMP failed -> partially_reachable, service_reachable
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
		Success:   false,
		Degraded:  false,
		Timestamp: time.Now(),
	}

	response := buildReachabilityResponse(httpEvidence, icmpEvidence)

	// HTTP success + ICMP failure = partially_reachable
	if response.TargetReachabilityStatus != string(probe.ReachabilityPartiallyReachable) {
		t.Errorf("expected target_reachability_status=partially_reachable, got %s", response.TargetReachabilityStatus)
	}

	// Service should be reachable (HTTP worked)
	if response.ServiceReachabilityStatus != string(probe.ReachabilityServiceReachable) {
		t.Errorf("expected service_reachability_status=service_reachable, got %s", response.ServiceReachabilityStatus)
	}
}

// TestReachabilityAPIVocabularyProjection_BothFailed tests both failed case.
func TestReachabilityAPIVocabularyProjection_BothFailed(t *testing.T) {
	// Both failed -> network_unreachable, service_unreachable
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
		Success:   false,
		Degraded:  false,
		Timestamp: time.Now(),
	}

	response := buildReachabilityResponse(httpEvidence, icmpEvidence)

	if response.TargetReachabilityStatus != string(probe.ReachabilityNetworkUnreachable) {
		t.Errorf("expected target_reachability_status=network_unreachable, got %s", response.TargetReachabilityStatus)
	}
	if response.ServiceReachabilityStatus != string(probe.ReachabilityServiceUnreachable) {
		t.Errorf("expected service_reachability_status=service_unreachable, got %s", response.ServiceReachabilityStatus)
	}
}

// TestReachabilityAPIVocabularyProjection_NoRecentEvidence tests unknown case.
func TestReachabilityAPIVocabularyProjection_NoRecentEvidence(t *testing.T) {
	// No recent evidence -> unknown
	httpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      false,
		Timestamp: time.Time{},
	}
	icmpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      false,
		Timestamp: time.Time{},
	}

	response := buildReachabilityResponse(httpEvidence, icmpEvidence)

	if response.TargetReachabilityStatus != string(probe.ReachabilityUnknown) {
		t.Errorf("expected target_reachability_status=unknown, got %s", response.TargetReachabilityStatus)
	}
	if response.ServiceReachabilityStatus != string(probe.ReachabilityUnknown) {
		t.Errorf("expected service_reachability_status=unknown, got %s", response.ServiceReachabilityStatus)
	}

	// Empty evidence returns unknown, not false failure
	if response.HTTPProbeStatus != "unknown" {
		t.Errorf("expected http_probe_status=unknown, got %s", response.HTTPProbeStatus)
	}
}

// TestReachabilityAPIVocabularyProjection_HTTPDegraded tests degraded case.
func TestReachabilityAPIVocabularyProjection_HTTPDegraded(t *testing.T) {
	// HTTP degraded -> partially_reachable, service_reachable
	httpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   true,
		Degraded:  true,
		Timestamp: time.Now(),
	}
	icmpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: time.Now(),
	}

	response := buildReachabilityResponse(httpEvidence, icmpEvidence)

	if response.HTTPProbeStatus != "degraded" {
		t.Errorf("expected http_probe_status=degraded, got %s", response.HTTPProbeStatus)
	}

	// Degraded != failed, so service should still be reachable
	if response.ServiceReachabilityStatus != string(probe.ReachabilityServiceReachable) {
		t.Errorf("expected service_reachability_status=service_reachable (degraded != failed), got %s", response.ServiceReachabilityStatus)
	}
}

// TestReachabilityAPIVocabularyProjection_ICMPDegraded tests ICMP degraded case.
func TestReachabilityAPIVocabularyProjection_ICMPDegraded(t *testing.T) {
	// ICMP degraded -> partially_reachable, service_reachable
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
		Degraded:  true,
		Timestamp: time.Now(),
	}

	response := buildReachabilityResponse(httpEvidence, icmpEvidence)

	if response.ICMPProbeStatus != "degraded" {
		t.Errorf("expected icmp_probe_status=degraded, got %s", response.ICMPProbeStatus)
	}
}

// TestReachabilityAPIVocabularyProjection_ResponseJSONValid tests JSON serialization.
func TestReachabilityAPIVocabularyProjection_ResponseJSONValid(t *testing.T) {
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

	response := buildReachabilityResponse(httpEvidence, icmpEvidence)

	// Marshal to JSON
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("failed to marshal response to JSON: %v", err)
	}

	// Unmarshal back
	var decoded ReachabilityAPIResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Verify key fields are present
	if decoded.TargetReachabilityStatus == "" {
		t.Error("target_reachability_status missing in JSON")
	}
	if decoded.ServiceReachabilityStatus == "" {
		t.Error("service_reachability_status missing in JSON")
	}
	if decoded.HTTPProbeStatus == "" {
		t.Error("http_probe_status missing in JSON")
	}
	if decoded.ICMPProbeStatus == "" {
		t.Error("icmp_probe_status missing in JSON")
	}
	if decoded.ReachabilityLabel == "" {
		t.Error("reachability_label missing in JSON")
	}
}

// TestReachabilityAPIVocabularyProjection_ProbeEvidenceIncluded verifies evidence is included.
func TestReachabilityAPIVocabularyProjection_ProbeEvidenceIncluded(t *testing.T) {
	now := time.Now()
	httpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: now,
	}
	icmpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   false,
		Degraded:  false,
		Timestamp: now,
	}

	response := buildReachabilityResponse(httpEvidence, icmpEvidence)

	// Verify probe evidence includes probe kind
	if !response.ProbeEvidence.HTTP.Seen {
		t.Error("HTTP evidence should indicate seen")
	}
	if !response.ProbeEvidence.HTTP.Success {
		t.Error("HTTP evidence should indicate success")
	}
	if response.ProbeEvidence.ICMP.Success {
		t.Error("ICMP evidence should indicate failure")
	}
}

// TestReachabilityAPIVocabularyProjection_TimestampsOmittedOrValid tests timestamp handling.
func TestReachabilityAPIVocabularyProjection_TimestampsOmittedOrValid(t *testing.T) {
	now := time.Now()
	httpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: now,
	}
	icmpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   true,
		Degraded:  false,
		Timestamp: now,
	}

	response := buildReachabilityResponse(httpEvidence, icmpEvidence)

	// Timestamps should be valid RFC3339 if present
	if response.LastHTTPProbeAt != nil {
		if _, err := time.Parse(time.RFC3339, *response.LastHTTPProbeAt); err != nil {
			t.Errorf("invalid HTTP timestamp format: %v", err)
		}
	}
	if response.LastICMPProbeAt != nil {
		if _, err := time.Parse(time.RFC3339, *response.LastICMPProbeAt); err != nil {
			t.Errorf("invalid ICMP timestamp format: %v", err)
		}
	}
}

// TestReachabilityAPIVocabularyProjection_EmptyEvidenceReturnsUnknown tests empty evidence case.
func TestReachabilityAPIVocabularyProjection_EmptyEvidenceReturnsUnknown(t *testing.T) {
	// No evidence should return unknown, not false failure
	httpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      false,
		Timestamp: time.Time{},
	}
	icmpEvidence := probe.ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      false,
		Timestamp: time.Time{},
	}

	response := buildReachabilityResponse(httpEvidence, icmpEvidence)

	// Should be unknown, not failure
	if response.HTTPProbeStatus != "unknown" {
		t.Errorf("expected http_probe_status=unknown for empty evidence, got %s", response.HTTPProbeStatus)
	}
	if response.ICMPProbeStatus != "unknown" {
		t.Errorf("expected icmp_probe_status=unknown for empty evidence, got %s", response.ICMPProbeStatus)
	}
}

// buildReachabilityResponse creates an API response from probe evidence.
// This simulates what the server would produce.
func buildReachabilityResponse(http, icmp probe.ProbeEvidence) ReachabilityAPIResponse {
	summary := probe.ClassifyReachability(http, icmp)

	response := ReachabilityAPIResponse{
		TargetReachabilityStatus:  string(summary.TargetStatus),
		ServiceReachabilityStatus: string(summary.ServiceStatus),
		HTTPProbeStatus:          string(summary.HTTPStatus),
		ICMPProbeStatus:          string(summary.ICMPStatus),
		ReachabilityLabel:        summary.Label,
		ReachabilityReason:       summary.Reason,
		ProbeEvidence: Evidence{
			HTTP: HTTPProbeEvidence{
				Seen:     http.Seen,
				Success:  http.Success && !http.Degraded,
				Degraded: http.Degraded,
			},
			ICMP: ICMPProbeEvidence{
				Seen:     icmp.Seen,
				Success:  icmp.Success && !icmp.Degraded,
				Degraded: icmp.Degraded,
			},
		},
	}

	// Add timestamps if seen
	if http.Seen {
		ts := http.Timestamp.Format(time.RFC3339)
		response.LastHTTPProbeAt = &ts
		response.ProbeEvidence.HTTP.Timestamp = &ts
	}
	if icmp.Seen {
		ts := icmp.Timestamp.Format(time.RFC3339)
		response.LastICMPProbeAt = &ts
		response.ProbeEvidence.ICMP.Timestamp = &ts
	}

	return response
}
