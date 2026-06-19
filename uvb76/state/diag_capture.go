package state

import (
	"sync"
	"time"
)

type DiagCaptureStatus string

const (
	DiagCaptureStatusOK            DiagCaptureStatus = "ok"
	DiagCaptureStatusUnavailable   DiagCaptureStatus = "unavailable"
	DiagCaptureStatusTimeout       DiagCaptureStatus = "timeout"
	DiagCaptureStatusError         DiagCaptureStatus = "error"
	DiagCaptureStatusDisabled      DiagCaptureStatus = "disabled"
	DiagCaptureStatusNoPeerMapping DiagCaptureStatus = "no_peer_mapping"
)

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
	// Only includes path and query (no scheme/host/credentials).
	// Example: "/status.json?include=network_diag"
	// This helps operators debug 404 issues without exposing sensitive details.
	RequestedPath *string `json:"requested_path,omitempty"`
}

type NetworkDiagData struct {
	StartedAt   string              `json:"started_at"`
	Status      string              `json:"status"`
	Wireguard   *WireguardDiagData  `json:"wireguard,omitempty"`
	Interfaces  []InterfaceDiagData `json:"interfaces"`
	Routes      []RouteDiagData     `json:"routes"`
	UnderlayTCP []TcpSocketDiagData `json:"underlay_tcp"`
	Events      []DiagEventData     `json:"events"`
}

type WireguardDiagData struct {
	Status     string                `json:"status"`
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

type CaptureStore struct {
	mu          sync.RWMutex
	captures    map[string][]DiagCapture
	lastCapture map[string]time.Time
	maxCaptures int
	inFlight    map[string]bool
}

func NewCaptureStore() *CaptureStore {
	return &CaptureStore{
		captures:    make(map[string][]DiagCapture),
		lastCapture: make(map[string]time.Time),
		maxCaptures: 10,
		inFlight:    make(map[string]bool),
	}
}

func NewCaptureStoreWithMax(maxCaptures int) *CaptureStore {
	if maxCaptures <= 0 {
		maxCaptures = 10
	}
	return &CaptureStore{
		captures:    make(map[string][]DiagCapture),
		lastCapture: make(map[string]time.Time),
		maxCaptures: maxCaptures,
		inFlight:    make(map[string]bool),
	}
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

	// Only update cooldown for successful captures that were NOT suppressed
	if capture.Status == DiagCaptureStatusOK && capture.Source != "" && !capture.SuppressedByCooldown {
		cs.lastCapture[capture.Source] = time.Now().UTC()
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
	cs.inFlight = make(map[string]bool)
}

// SpikeRetentionStats holds spike retention metadata for UI display.
type SpikeRetentionStats struct {
	RetainedSpikeCount    int `json:"retained_spike_count"`
	VisibleSpikeCount     int `json:"visible_spike_count"`
	ProtectedCaptureCount int `json:"protected_capture_count"`
	PurgeEligibleCount    int `json:"purge_eligible_count"`
	MaxUncapturedSpikes   int `json:"max_uncaptured_spikes"`
}

// CaptureStatus represents the derived capture protection status for a spike.
type CaptureStatus string

const (
	CaptureStatusReady      CaptureStatus = "ready"                  // capture exists and can be accessed
	CaptureStatusInProgress CaptureStatus = "in_progress"            // capture creation in flight
	CaptureStatusTimeout    CaptureStatus = "timeout"                // timeout error
	CaptureStatusError      CaptureStatus = "error"                  // capture error
	CaptureStatusSuppressed CaptureStatus = "suppressed_by_cooldown" // suppressed by cooldown
	CaptureStatusMissing    CaptureStatus = "missing"                // metadata refers to missing artifact
	CaptureStatusNone       CaptureStatus = "none"                   // no capture attempted
)

// SpikeCaptureInfo holds capture-derived protection info for a spike.
type SpikeCaptureInfo struct {
	CaptureStatus CaptureStatus `json:"capture_status"`
	CaptureExists bool          `json:"capture_exists"` // true if artifact exists
	IsProtected   bool          `json:"is_protected"`   // true if spike must not be purged
}

// GetCaptureInfo returns derived capture protection info for a spike.
// A spike is protected if it has a capture artifact that exists or capture is in progress.
func (cs *CaptureStore) GetCaptureInfo(eventID string, isInFlight bool) SpikeCaptureInfo {
	// Check in-flight first - this takes precedence
	if isInFlight {
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusInProgress,
			CaptureExists: false,
			IsProtected:   true, // Don't race cleanup against in-flight captures
		}
	}

	captures := cs.GetCaptures(eventID)
	if len(captures) == 0 {
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusNone,
			CaptureExists: false,
			IsProtected:   false,
		}
	}

	// Take the most recent capture
	capture := captures[len(captures)-1]

	// Check suppressed by cooldown
	if capture.SuppressedByCooldown {
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusSuppressed,
			CaptureExists: false,
			IsProtected:   false, // Suppressed captures are purge-eligible
		}
	}

	// Check capture status
	switch capture.Status {
	case DiagCaptureStatusOK:
		// ok status with no artifact means partial/success without data
		// Still protected if status is ok (capture was attempted)
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusReady,
			CaptureExists: capture.NetworkDiag != nil,
			IsProtected:   true,
		}
	case DiagCaptureStatusTimeout:
		// Timeout with artifact is protected, without is purge-eligible
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusTimeout,
			CaptureExists: capture.NetworkDiag != nil,
			IsProtected:   capture.NetworkDiag != nil,
		}
	case DiagCaptureStatusError:
		// Error with artifact is protected, without is purge-eligible
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusError,
			CaptureExists: capture.NetworkDiag != nil,
			IsProtected:   capture.NetworkDiag != nil,
		}
	case DiagCaptureStatusDisabled:
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusNone,
			CaptureExists: false,
			IsProtected:   false,
		}
	case DiagCaptureStatusNoPeerMapping:
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusNone,
			CaptureExists: false,
			IsProtected:   false,
		}
	default:
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusNone,
			CaptureExists: false,
			IsProtected:   false,
		}
	}
}

// GetProtectionInfo returns protection info for a spike event.
// This method checks in-flight captures internally.
func (cs *CaptureStore) GetProtectionInfo(eventID string) (isProtected bool, hasArtifact bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// Check in-flight first - in-flight spikes are protected
	if cs.inFlight[eventID] {
		return true, false
	}

	captures := cs.captures[eventID]
	if len(captures) == 0 {
		return false, false
	}

	// Take the most recent capture
	capture := captures[len(captures)-1]

	// Check suppressed by cooldown - purge eligible
	// hasCapture=false because suppression means this capture didn't complete;
	// any artifact present was from a previous capture, not this suppressed attempt
	if capture.SuppressedByCooldown {
		return false, false
	}

	// Check capture status
	switch capture.Status {
	case DiagCaptureStatusOK:
		// ok status means capture was attempted - protected
		return true, capture.NetworkDiag != nil
	case DiagCaptureStatusTimeout:
		// Timeout with artifact is protected, without is purge-eligible
		return capture.NetworkDiag != nil, capture.NetworkDiag != nil
	case DiagCaptureStatusError:
		// Error with artifact is protected, without is purge-eligible
		return capture.NetworkDiag != nil, capture.NetworkDiag != nil
	case DiagCaptureStatusDisabled, DiagCaptureStatusNoPeerMapping:
		return false, capture.NetworkDiag != nil
	default:
		return false, capture.NetworkDiag != nil
	}
}

func (cs *CaptureStore) Count() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.captures)
}
