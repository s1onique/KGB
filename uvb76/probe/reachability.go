// Package probe provides HTTP and ICMP probing with explicit reachability semantics.
//
// Canonical reachability vocabulary:
//   - target_reachable: at least one network-level signal indicates the host/peer is reachable
//   - service_reachable: the service-specific HTTP probe succeeds
//   - probe_reachable: a specific probe kind succeeds
//   - probe_failed: a specific probe kind fails
//   - probe_degraded: a specific probe kind succeeds but violates latency/spike threshold
//   - probe_recovered: a specific probe kind transitions from failing/degraded to healthy
//   - service_unreachable: HTTP probe fails; does not imply ICMP failure
//   - network_unreachable: ICMP/network-level probe fails and no other network-level reachability proof exists
//   - partially_reachable: at least one probe kind succeeds while another fails
//   - unknown: no recent probe evidence exists
//
// HTTP/ICMP combination matrix:
//
//	HTTP success + ICMP success = target_reachable / service_reachable
//	HTTP success + ICMP failed = partially_reachable / service_reachable
//	HTTP failed + ICMP success = partially_reachable / service_unreachable
//	HTTP failed + ICMP failed = network_unreachable / service_unreachable
//	HTTP unknown + ICMP success = target_reachable / unknown
//	HTTP success + ICMP unknown = target_reachable / service_reachable
//	HTTP unknown + ICMP unknown = unknown / unknown
//
// Note: "unreachable" without qualifier is forbidden in API, events, and UI labels.
package probe

import (
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// ProbeEvidence represents the evidence from a single probe execution.
type ProbeEvidence struct {
	Kind      domain.ProbeKind
	Seen      bool
	Success   bool
	Degraded  bool
	Timestamp time.Time
	ErrorKind string
	ErrorText string
}

// ReachabilityStatus is the canonical reachability status for a target.
type ReachabilityStatus string

const (
	// ReachabilityTargetReachable indicates at least one network-level signal
	// indicates the host/peer is reachable.
	ReachabilityTargetReachable ReachabilityStatus = "target_reachable"

	// ReachabilityServiceReachable indicates the service-specific HTTP probe succeeds.
	ReachabilityServiceReachable ReachabilityStatus = "service_reachable"

	// ReachabilityPartiallyReachable indicates at least one probe kind succeeds
	// while another fails.
	ReachabilityPartiallyReachable ReachabilityStatus = "partially_reachable"

	// ReachabilityServiceUnreachable indicates HTTP probe fails.
	// Does not imply ICMP failure.
	ReachabilityServiceUnreachable ReachabilityStatus = "service_unreachable"

	// ReachabilityNetworkUnreachable indicates ICMP/network-level probe fails
	// and no other network-level reachability proof exists.
	ReachabilityNetworkUnreachable ReachabilityStatus = "network_unreachable"

	// ReachabilityProbeFailed indicates a specific probe kind fails.
	ReachabilityProbeFailed ReachabilityStatus = "probe_failed"

	// ReachabilityProbeDegraded indicates a specific probe kind succeeds
	// but violates latency/spike threshold.
	ReachabilityProbeDegraded ReachabilityStatus = "probe_degraded"

	// ReachabilityProbeRecovered indicates a specific probe kind transitions
	// from failing/degraded to healthy.
	ReachabilityProbeRecovered ReachabilityStatus = "probe_recovered"

	// ReachabilityUnknown indicates no recent probe evidence exists.
	ReachabilityUnknown ReachabilityStatus = "unknown"
)

// ProbeStatus represents the status of a specific probe kind.
type ProbeStatus string

const (
	ProbeStatusSuccess   ProbeStatus = "success"
	ProbeStatusFailed    ProbeStatus = "failed"
	ProbeStatusDegraded ProbeStatus = "degraded"
	ProbeStatusUnknown  ProbeStatus = "unknown"
)

// ReachabilitySummary contains the classified reachability state.
type ReachabilitySummary struct {
	// TargetStatus is the overall target reachability.
	TargetStatus ReachabilityStatus `json:"target_status"`
	// ServiceStatus is the HTTP service reachability.
	ServiceStatus ReachabilityStatus `json:"service_status"`
	// HTTPStatus is the HTTP probe-specific status.
	HTTPStatus ProbeStatus `json:"http_status"`
	// ICMPStatus is the ICMP probe-specific status.
	ICMPStatus ProbeStatus `json:"icmp_status"`
	// Label is the human-readable UI label.
	Label string `json:"label"`
	// Reason explains the classification.
	Reason string `json:"reason"`
	// Evidence contains probe evidence used for classification.
	Evidence ProbeEvidenceSummary `json:"evidence"`
}

// ProbeEvidenceSummary summarizes the evidence used for classification.
type ProbeEvidenceSummary struct {
	HTTP Seen  `json:"http_seen"`
	ICMP Seen  `json:"icmp_seen"`
}

// Seen indicates whether a probe has been observed.
type Seen struct {
	Timestamp *time.Time `json:"timestamp,omitempty"`
	Success   bool      `json:"success"`
	Degraded  bool      `json:"degraded"`
	Failed    bool      `json:"failed"`
}

// ClassifyReachability classifies the reachability state from HTTP and ICMP probe evidence.
// This function is pure: no network calls, no store access, no wall-clock reads except passed timestamps.
func ClassifyReachability(http, icmp ProbeEvidence) ReachabilitySummary {
	// Compute individual probe statuses
	httpStatus := computeProbeStatus(http)
	icmpStatus := computeProbeStatus(icmp)

	// Compute target and service statuses
	targetStatus, serviceStatus, reason := computeReachabilityStatus(http, icmp)

	// Generate label
	label := generateLabel(httpStatus, icmpStatus)

	// Build evidence summary
	evidence := buildEvidenceSummary(http, icmp)

	return ReachabilitySummary{
		TargetStatus:  targetStatus,
		ServiceStatus: serviceStatus,
		HTTPStatus:   httpStatus,
		ICMPStatus:   icmpStatus,
		Label:        label,
		Reason:       reason,
		Evidence:     evidence,
	}
}

// computeProbeStatus derives the probe-specific status from evidence.
func computeProbeStatus(evidence ProbeEvidence) ProbeStatus {
	if !evidence.Seen {
		return ProbeStatusUnknown
	}
	if evidence.Degraded {
		return ProbeStatusDegraded
	}
	if evidence.Success {
		return ProbeStatusSuccess
	}
	return ProbeStatusFailed
}

// computeReachabilityStatus derives target and service reachability from probe evidence.
// HTTP failure + ICMP success = partially_reachable / service_unreachable (NOT network_unreachable)
// HTTP success + ICMP failure = partially_reachable / service_reachable
// Both failed = network_unreachable / service_unreachable
// Unknown + unknown = unknown / unknown
func computeReachabilityStatus(http, icmp ProbeEvidence) (target, service ReachabilityStatus, reason string) {
	httpSeen := http.Seen
	icmpSeen := icmp.Seen
	httpSuccess := http.Success && !http.Degraded
	icmpSuccess := icmp.Success && !icmp.Degraded

	// Case: both unknown
	if !httpSeen && !icmpSeen {
		return ReachabilityUnknown, ReachabilityUnknown, "no probe evidence available"
	}

	// Case: HTTP unknown, ICMP known
	if !httpSeen && icmpSeen {
		if icmpSuccess {
			return ReachabilityTargetReachable, ReachabilityUnknown, "ICMP reachable, HTTP status unknown"
		}
		// ICMP failed but HTTP unknown - cannot determine network unreachability
		return ReachabilityUnknown, ReachabilityUnknown, "ICMP failed, HTTP status unknown"
	}

	// Case: HTTP known, ICMP unknown
	if httpSeen && !icmpSeen {
		if httpSuccess {
			return ReachabilityTargetReachable, ReachabilityServiceReachable, "HTTP reachable, ICMP status unknown"
		}
		// HTTP failed but ICMP unknown - service unreachable but target status unclear
		return ReachabilityUnknown, ReachabilityServiceUnreachable, "HTTP failed, ICMP status unknown"
	}

	// Both seen
	httpFailed := httpSeen && !http.Success
	icmpFailed := icmpSeen && !icmp.Success

	// Case: both success
	if httpSuccess && icmpSuccess {
		return ReachabilityTargetReachable, ReachabilityServiceReachable, "both HTTP and ICMP probes successful"
	}

	// Case: HTTP success, ICMP failed
	if httpSuccess && !icmpSuccess {
		return ReachabilityPartiallyReachable, ReachabilityServiceReachable, "HTTP reachable, ICMP failed"
	}

	// Case: HTTP failed, ICMP success
	if httpFailed && icmpSuccess {
		return ReachabilityPartiallyReachable, ReachabilityServiceUnreachable, "HTTP failed, ICMP reachable"
	}

	// Case: both failed
	if httpFailed && icmpFailed {
		return ReachabilityNetworkUnreachable, ReachabilityServiceUnreachable, "both HTTP and ICMP probes failed"
	}

	// Edge case: HTTP degraded
	if http.Degraded && icmpSuccess {
		return ReachabilityPartiallyReachable, ReachabilityServiceReachable, "HTTP degraded, ICMP reachable"
	}

	// Edge case: ICMP degraded
	if httpSuccess && icmp.Degraded {
		return ReachabilityPartiallyReachable, ReachabilityServiceReachable, "HTTP reachable, ICMP degraded"
	}

	// Edge case: both degraded
	if http.Degraded && icmp.Degraded {
		return ReachabilityPartiallyReachable, ReachabilityServiceReachable, "both HTTP and ICMP degraded"
	}

	// Fallback
	return ReachabilityUnknown, ReachabilityUnknown, "insufficient probe evidence"
}

// generateLabel creates a human-readable UI label.
// Forbidden: plain "unreachable" without service/network qualifier.
func generateLabel(httpStatus, icmpStatus ProbeStatus) string {
	httpLabel := statusToLabel(httpStatus)
	icmpLabel := statusToLabel(icmpStatus)

	if httpStatus == ProbeStatusUnknown && icmpStatus == ProbeStatusUnknown {
		return "No recent probe data"
	}

	return httpLabel + " · " + icmpLabel
}

// statusToLabel converts a probe status to a human-readable label.
func statusToLabel(status ProbeStatus) string {
	switch status {
	case ProbeStatusSuccess:
		return "OK"
	case ProbeStatusFailed:
		return "failing"
	case ProbeStatusDegraded:
		return "degraded"
	default:
		return "unknown"
	}
}

// buildEvidenceSummary builds the evidence summary.
func buildEvidenceSummary(http, icmp ProbeEvidence) ProbeEvidenceSummary {
	summary := ProbeEvidenceSummary{}

	if http.Seen {
		summary.HTTP = Seen{
			Timestamp: &http.Timestamp,
			Success:   http.Success && !http.Degraded,
			Degraded:  http.Degraded,
			Failed:    !http.Success,
		}
	}

	if icmp.Seen {
		summary.ICMP = Seen{
			Timestamp: &icmp.Timestamp,
			Success:   icmp.Success && !icmp.Degraded,
			Degraded:  icmp.Degraded,
			Failed:    !icmp.Success,
		}
	}

	return summary
}

// CanonicalStatusStrings returns all canonical reachability status strings.
// Used by verifiers to check that only canonical values are emitted.
func CanonicalStatusStrings() []string {
	return []string{
		string(ReachabilityTargetReachable),
		string(ReachabilityServiceReachable),
		string(ReachabilityPartiallyReachable),
		string(ReachabilityServiceUnreachable),
		string(ReachabilityNetworkUnreachable),
		string(ReachabilityProbeFailed),
		string(ReachabilityProbeDegraded),
		string(ReachabilityProbeRecovered),
		string(ReachabilityUnknown),
	}
}

// ForbiddenLabels returns label patterns that are forbidden.
// Plain "unreachable" without service/network qualifier.
func ForbiddenLabels() []string {
	return []string{
		"unreachable",
		"reachable",
	}
}

// IsCanonicalStatus returns true if the status is a canonical reachability status.
func IsCanonicalStatus(status string) bool {
	for _, canonical := range CanonicalStatusStrings() {
		if status == canonical {
			return true
		}
	}
	return false
}

// IsLabelForbidden returns true if the label contains forbidden wording.
func IsLabelForbidden(label string) bool {
	// Check for bare "unreachable" or "reachable" without qualifier
	for _, forbidden := range ForbiddenLabels() {
		// Simple substring check - in practice would use word boundaries
		if label == forbidden {
			return true
		}
	}
	return false
}
