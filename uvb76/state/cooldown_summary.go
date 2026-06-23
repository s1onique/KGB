package state

import (
	"time"
)

// =============================================================================
// AnchorEventSummary — Embedded Anchor Proof for UI Display
// =============================================================================

// AnchorEventSummary provides sufficient anchor event context for the UI to display
// auditable suppression information even when the anchor spike row itself is not visible.
//
// This is used in response-time anchor expiry scenarios where:
// 1. The suppression decision was valid at decision time
// 2. The anchor spike was evicted from the visible timeline before response
// 3. We need to show why the suppression happened without the anchor row
//
// Fields are chosen to enable UI rendering of "Suppressed by ICMP capture at HH:MM:SS"
// without requiring the anchor row to be in the response.
type AnchorEventSummary struct {
	// EventID is the spike event ID of the anchor capture.
	EventID string `json:"event_id,omitempty"`
	// CaptureID is the capture record ID (may differ from EventID).
	CaptureID string `json:"capture_id,omitempty"`
	// ProbeKind is the probe kind (http/icmp) of the anchor capture.
	ProbeKind string `json:"probe_kind,omitempty"`
	// Severity is the spike severity (warning/critical) at capture time.
	Severity string `json:"severity,omitempty"`
	// LatencyMs is the latency value that triggered the anchor spike.
	LatencyMs float64 `json:"latency_ms,omitempty"`
	// SampleTs is the timestamp of the spike sample.
	SampleTs time.Time `json:"sample_ts,omitempty"`
	// CaptureStatus is the status of the anchor capture.
	CaptureStatus CaptureStatus `json:"capture_status,omitempty"`
	// Source is the diagnostic peer/source that performed the anchor capture.
	Source string `json:"source,omitempty"`
	// CapturedAt is when the anchor capture was started.
	CapturedAt time.Time `json:"captured_at,omitempty"`
}
