package state

import (
	"sync"
	"time"
)

// =============================================================================
// DiagCaptureStatus — Capture Status Constants
// =============================================================================

type DiagCaptureStatus string

const (
	DiagCaptureStatusOK            DiagCaptureStatus = "ok"
	DiagCaptureStatusUnavailable   DiagCaptureStatus = "unavailable"
	DiagCaptureStatusTimeout       DiagCaptureStatus = "timeout"
	DiagCaptureStatusError         DiagCaptureStatus = "error"
	DiagCaptureStatusDisabled      DiagCaptureStatus = "disabled"
	DiagCaptureStatusNoPeerMapping DiagCaptureStatus = "no_peer_mapping"
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

// =============================================================================
// NetworkDiagData — Network Diagnostic Payload
// =============================================================================

type NetworkDiagData struct {
	StartedAt   string              `json:"started_at"`
	Status      string              `json:"status"`
	Wireguard   *WireguardDiagData `json:"wireguard,omitempty"`
	Interfaces  []InterfaceDiagData `json:"interfaces"`
	Routes      []RouteDiagData    `json:"routes"`
	UnderlayTCP []TcpSocketDiagData `json:"underlay_tcp"`
	Events      []DiagEventData    `json:"events"`
}

type WireguardDiagData struct {
	Status     string                 `json:"status"`
	Interfaces []WgInterfaceDiagData `json:"interfaces"`
}

type WgInterfaceDiagData struct {
	Name   string           `json:"name"`
	Status string           `json:"status"`
	Peers  []WgPeerDiagData `json:"peers"`
}

type WgPeerDiagData struct {
	PublicKey             string  `json:"public_key"`
	Endpoint              string  `json:"endpoint"`
	AllowedIPs            string  `json:"allowed_ips"`
	LatestHandshakeAt     *string `json:"latest_handshake_at,omitempty"`
	LatestHandshakeAgeSec *int64  `json:"latest_handshake_age_seconds,omitempty"`
	TransferRxBytes       int64   `json:"transfer_rx_bytes"`
	TransferTxBytes       int64   `json:"transfer_tx_bytes"`
}

type InterfaceDiagData struct {
	Name      string `json:"name"`
	OperState string `json:"operstate"`
	Carrier   *bool  `json:"carrier,omitempty"`
	RxBytes   int64  `json:"rx_bytes"`
	TxBytes   int64  `json:"tx_bytes"`
	RxPackets int64  `json:"rx_packets"`
	TxPackets int64  `json:"tx_packets"`
	RxErrors  int64  `json:"rx_errors"`
	TxErrors  int64  `json:"tx_errors"`
	RxDropped int64  `json:"rx_dropped"`
	TxDropped int64  `json:"tx_dropped"`
}

type RouteDiagData struct {
	Target    string  `json:"target"`
	Interface string  `json:"interface"`
	Source    string  `json:"source"`
	Gateway   *string `json:"gateway,omitempty"`
	Status    string  `json:"status"`
}

type TcpSocketDiagData struct {
	Name           string   `json:"name"`
	State          string   `json:"state"`
	Local          string   `json:"local"`
	Remote         string   `json:"remote"`
	RTTMs          *float64 `json:"rtt_ms,omitempty"`
	RTTVarMs       *float64 `json:"rttvar_ms,omitempty"`
	RTOMs          *int64   `json:"rto_ms,omitempty"`
	Retransmits    *int64   `json:"retransmits,omitempty"`
	Unacked        *int64   `json:"unacked,omitempty"`
	Cwnd           *int32   `json:"cwnd,omitempty"`
	SendQueueBytes *int64   `json:"send_queue_bytes,omitempty"`
	RecvQueueBytes *int64   `json:"recv_queue_bytes,omitempty"`
	Status         string   `json:"status"`
}

type DiagEventData struct {
	Timestamp string  `json:"ts"`
	Severity  string  `json:"severity"`
	Source    string  `json:"source"`
	Message   string  `json:"message"`
	Fields    *string `json:"fields,omitempty"`
}

type SpikeEventWithCaptures struct {
	SpikeEvent
	Captures []DiagCapture `json:"captures,omitempty"`
}

// =============================================================================
// CaptureStore — In-Memory Capture Storage
// =============================================================================

type CaptureStore struct {
	mu               sync.RWMutex
	captures         map[string][]DiagCapture
	lastCapture      map[string]time.Time
	lastCaptureAnchor map[string]CaptureCooldownAnchor // Provenance for cooldown anchors
	maxCaptures      int
	inFlight         map[string]bool
}

func NewCaptureStore() *CaptureStore {
	return &CaptureStore{
		captures:         make(map[string][]DiagCapture),
		lastCapture:      make(map[string]time.Time),
		lastCaptureAnchor: make(map[string]CaptureCooldownAnchor),
		maxCaptures:      10,
		inFlight:         make(map[string]bool),
	}
}

func NewCaptureStoreWithMax(maxCaptures int) *CaptureStore {
	if maxCaptures <= 0 {
		maxCaptures = 10
	}
	return &CaptureStore{
		captures:         make(map[string][]DiagCapture),
		lastCapture:      make(map[string]time.Time),
		lastCaptureAnchor: make(map[string]CaptureCooldownAnchor),
		maxCaptures:      maxCaptures,
		inFlight:         make(map[string]bool),
	}
}

// isSuccessfulCooldownAnchorCapture returns true if this capture should update
// the cooldown anchor. Only real successful captures update anchors, not skipped,
// failed, or suppressed captures.
func isSuccessfulCooldownAnchorCapture(capture DiagCapture) bool {
	// Must have a source
	if capture.Source == "" {
		return false
	}
	// Must not be suppressed by cooldown
	if capture.SuppressedByCooldown {
		return false
	}
	// Must have successful status
	if capture.Status != DiagCaptureStatusOK {
		return false
	}
	// Must have captured status OR be legacy (empty capture_status with ok status)
	// Legacy handling: if CaptureStatus is empty but Status is OK, this is likely
	// a pre-provenance capture that should not update anchor
	if capture.CaptureStatus == "" {
		return false
	}
	// Only captured status updates anchor
	return capture.CaptureStatus == CaptureStatusCaptured
}

func (cs *CaptureStore) AddCapture(eventID string, capture DiagCapture) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	captures := cs.captures[eventID]
	if len(captures) >= cs.maxCaptures {
		captures = captures[1:]
	}
	captures = append(captures, capture)
	cs.captures[eventID] = captures

	// Only update cooldown anchor provenance for successful captures that were NOT suppressed
	// CRITICAL INVARIANT: Skipped cooldown records MUST NOT update anchor provenance
	if isSuccessfulCooldownAnchorCapture(capture) {
		// Use capture's start time for cooldown math consistency with provenance
		cs.lastCapture[capture.Source] = capture.CaptureStartedAt
		
		// Also update the provenance-bearing anchor record
		cs.lastCaptureAnchor[capture.Source] = CaptureCooldownAnchor{
			AnchorCaptureID:        eventID,
			AnchorSource:           capture.Source,
			AnchorCreatedAt:        capture.CaptureStartedAt,
			AnchorCompletedAt:      capture.CaptureFinishedAt,
			AnchorUpdatedByStatus: string(capture.CaptureStatus),
			CreatedFrom:            "diag_capture_success",
		}
	}
}

// AddCaptureWithProvenance adds a capture with explicit anchor provenance information.
// Use this method when the caller has additional context (target_id, probe_kind, etc.)
// that should be recorded in the cooldown anchor.
func (cs *CaptureStore) AddCaptureWithProvenance(eventID string, capture DiagCapture, anchorTargetID, anchorProbeKind string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	captures := cs.captures[eventID]
	if len(captures) >= cs.maxCaptures {
		captures = captures[1:]
	}
	captures = append(captures, capture)
	cs.captures[eventID] = captures

	// Only update cooldown anchor provenance for successful captures that were NOT suppressed
	if isSuccessfulCooldownAnchorCapture(capture) {
		cs.lastCapture[capture.Source] = capture.CaptureStartedAt
		
		// Update provenance-bearing anchor record with rich context
		cs.lastCaptureAnchor[capture.Source] = CaptureCooldownAnchor{
			AnchorCaptureID:        eventID,
			AnchorTargetID:        anchorTargetID,
			AnchorProbeKind:       anchorProbeKind,
			AnchorSource:          capture.Source,
			AnchorCreatedAt:       capture.CaptureStartedAt,
			AnchorCompletedAt:     capture.CaptureFinishedAt,
			AnchorUpdatedByStatus: string(capture.CaptureStatus),
			CreatedFrom:           "diag_capture_success",
		}
	}
}

func (cs *CaptureStore) GetCaptures(eventID string) []DiagCapture {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	captures := cs.captures[eventID]
	result := make([]DiagCapture, len(captures))
	copy(result, captures)
	return result
}

func (cs *CaptureStore) IsInCooldown(peerName string, cooldownSeconds int) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	lastTime, exists := cs.lastCapture[peerName]
	if !exists {
		return false
	}
	return time.Since(lastTime) < time.Duration(cooldownSeconds)*time.Second
}

func (cs *CaptureStore) IsInFlight(peerName string) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.inFlight[peerName]
}

func (cs *CaptureStore) ReserveInFlight(peerName string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.inFlight[peerName] {
		return false
	}
	cs.inFlight[peerName] = true
	return true
}

func (cs *CaptureStore) ReleaseInFlight(peerName string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.inFlight[peerName] = false
}

func (cs *CaptureStore) GetLastCaptureTime(peerName string) time.Time {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.lastCapture[peerName]
}

// GetLastCaptureAnchor returns the provenance-bearing anchor for a peer.
// Returns empty struct if no anchor exists.
func (cs *CaptureStore) GetLastCaptureAnchor(peerName string) CaptureCooldownAnchor {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.lastCaptureAnchor[peerName]
}

// GetAllLastCaptureAnchors returns all cooldown anchors for debugging.
func (cs *CaptureStore) GetAllLastCaptureAnchors() map[string]CaptureCooldownAnchor {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	result := make(map[string]CaptureCooldownAnchor, len(cs.lastCaptureAnchor))
	for k, v := range cs.lastCaptureAnchor {
		result[k] = v
	}
	return result
}

func (cs *CaptureStore) CooldownRemaining(peerName string, cooldownSeconds int) int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	lastTime, exists := cs.lastCapture[peerName]
	if !exists {
		return 0
	}
	remaining := time.Duration(cooldownSeconds)*time.Second - time.Since(lastTime)
	if remaining <= 0 {
		return 0
	}
	return int(remaining.Seconds())
}

func (cs *CaptureStore) Clear() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.captures = make(map[string][]DiagCapture)
	cs.lastCapture = make(map[string]time.Time)
	cs.lastCaptureAnchor = make(map[string]CaptureCooldownAnchor)
	cs.inFlight = make(map[string]bool)
}

func (cs *CaptureStore) Count() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.captures)
}

// =============================================================================
// SpikeRetentionStats — Retention Metadata
// =============================================================================

type SpikeRetentionStats struct {
	RetainedSpikeCount    int `json:"retained_spike_count"`
	VisibleSpikeCount     int `json:"visible_spike_count"`
	ProtectedCaptureCount int `json:"protected_capture_count"`
	PurgeEligibleCount    int `json:"purge_eligible_count"`
	MaxUncapturedSpikes   int `json:"max_uncaptured_spikes"`
}

// =============================================================================
// CaptureStatus — Derived Capture Protection Status
// =============================================================================

type CaptureStatus string

const (
	CaptureStatusCaptured         CaptureStatus = "captured"
	CaptureStatusSkippedCooldown  CaptureStatus = "skipped_cooldown"
	CaptureStatusFailed           CaptureStatus = "failed"
	CaptureStatusDisabled         CaptureStatus = "disabled"
	CaptureStatusNotConfigured    CaptureStatus = "not_configured"
	CaptureStatusNotAttempted     CaptureStatus = "not_attempted"
	CaptureStatusNone             CaptureStatus = "none"
	CaptureStatusInProgress       CaptureStatus = "in_progress"
	CaptureStatusMissing          CaptureStatus = "missing"
)

// CanonicalCaptureStatusFromDiagStatus maps a low-level DiagCaptureStatus
// to a canonical CaptureStatus for UI display and projection layers.
//
// This helper is used by:
// - CaptureService: to populate CaptureStatus on all service-created rows
// - API projection: for backward compatibility with legacy rows that lack CaptureStatus
//
// Mapping rules:
//   DiagCaptureStatusOK + hasNetworkDiag -> CaptureStatusCaptured
//   DiagCaptureStatusOK + no NetworkDiag  -> CaptureStatusFailed
//   DiagCaptureStatusError                -> CaptureStatusFailed
//   DiagCaptureStatusTimeout              -> CaptureStatusFailed
//   DiagCaptureStatusUnavailable         -> CaptureStatusNotAttempted
//   DiagCaptureStatusDisabled             -> CaptureStatusDisabled
//   DiagCaptureStatusNoPeerMapping       -> CaptureStatusNotConfigured
func CanonicalCaptureStatusFromDiagStatus(status DiagCaptureStatus, hasNetworkDiag bool) CaptureStatus {
	switch status {
	case DiagCaptureStatusOK:
		if hasNetworkDiag {
			return CaptureStatusCaptured
		}
		return CaptureStatusFailed
	case DiagCaptureStatusError:
		return CaptureStatusFailed
	case DiagCaptureStatusTimeout:
		return CaptureStatusFailed
	case DiagCaptureStatusUnavailable:
		return CaptureStatusNotAttempted
	case DiagCaptureStatusDisabled:
		return CaptureStatusDisabled
	case DiagCaptureStatusNoPeerMapping:
		return CaptureStatusNotConfigured
	default:
		return CaptureStatusFailed
	}
}
