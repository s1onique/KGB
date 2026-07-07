package state

import (
	"time"
)

// =============================================================================
// DiagCapture — Diagnostic Capture Record
// =============================================================================

type DiagCapture struct {
	Source               string            `json:"source"`
	BaseURL              string            `json:"base_url"`
	CaptureStartedAt     time.Time         `json:"capture_started_at"`
	CaptureFinishedAt    *time.Time        `json:"capture_finished_at,omitempty"`
	DurationMs           *int64            `json:"duration_ms,omitempty"`
	Status               DiagCaptureStatus `json:"status"`
	Error                *string           `json:"error,omitempty"`
	NetworkDiag          *NetworkDiagData  `json:"network_diag,omitempty"`
	SuppressedByCooldown bool              `json:"suppressed_by_cooldown,omitempty"`
	ReferencedCaptureID  string            `json:"referenced_capture_id,omitempty"`
	// RequestedPath provides sanitized request evidence for error cases.
	RequestedPath *string `json:"requested_path,omitempty"`
	// EffectiveCaptureURL is the full URL that was requested (with query params).
	EffectiveCaptureURL string `json:"effective_capture_url,omitempty"`
	// HTTPStatusCode captures the HTTP status code from the diagnostic response.
	HTTPStatusCode *int `json:"http_status_code,omitempty"`
	// CaptureStatus is the derived capture status for UI display.
	CaptureStatus CaptureStatus `json:"capture_status"`
	// CooldownInfo provides auditable metadata when capture was skipped due to cooldown.
	CooldownInfo *CaptureCooldownInfo `json:"cooldown_info,omitempty"`
	// TcpAbsenceEvents contains TCP absence explanations when underlay_tcp is empty.
	// This field is populated by the capture service based on network_diag.events.
	TcpAbsenceEvents []TcpAbsenceEvent `json:"tcp_absence_events,omitempty"`
	// ProbeRoute contains route lookup evidence for the probe destination.
	// This provides evidence of which kernel route was selected for the exact
	// probe destination at capture time. Route lookup failures do not block capture.
	ProbeRoute *ProbeRoute `json:"probe_route,omitempty"`

	// TcpQuality contains TCP path quality evidence for the probe destination socket.
	// This provides evidence of network path health at the TCP layer during the spike,
	// including RTT, retransmits, congestion window, and queue depths.
	// TCP quality collection failures do not block diagnostic capture.
	// For ICMP probes, TCP quality is unavailable (not applicable).
	TcpQuality *TcpQuality `json:"tcp_quality,omitempty"`
}

// TcpAbsenceEvent explains why TCP diagnostics were absent from a successful capture.
// This provides machine-readable context for the UI when underlay_tcp is empty.
type TcpAbsenceEvent struct {
	// ReasonCode is a machine-readable reason code from the TcpAbsenceReason enum.
	// Values: no_matching_socket, socket_closed_before_capture, command_failed,
	// not_configured, permission_denied, target_not_tcp, target_mapping_missing,
	// parse_failed, unsupported_platform
	ReasonCode string `json:"reason_code"`
	// Source indicates the diagnostic component that generated this event.
	Source string `json:"source"`
	// ExpectedPeer is the peer/endpoint that was expected to match (if known).
	ExpectedPeer string `json:"expected_peer,omitempty"`
	// ExpectedPort is the port that was expected to match (if known).
	ExpectedPort *int `json:"expected_port,omitempty"`
	// ProbeKind indicates which probe triggered the capture (http/icmp).
	ProbeKind string `json:"probe_kind,omitempty"`
	// CommandTool is the tool/command that was attempted (e.g., "ss", "tcpdiag").
	CommandTool string `json:"command_tool,omitempty"`
	// RawMatchCount is the number of sockets that were found but did not match filters.
	RawMatchCount *int `json:"raw_match_count,omitempty"`
	// Namespace indicates the network namespace scope (if known).
	Namespace string `json:"namespace,omitempty"`
	// Detail provides additional context about the absence.
	Detail string `json:"detail,omitempty"`
}
