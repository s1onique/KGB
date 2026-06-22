package state

import "time"

// =============================================================================
// TcpQuality — TCP Path Quality Evidence for Diagnostic Packets
// =============================================================================

// TcpQualityErrorKind classifies TCP quality collection failures.
type TcpQualityErrorKind string

const (
	TcpQualityErrorCommandMissing     TcpQualityErrorKind = "command_missing"
	TcpQualityErrorNonZeroExit       TcpQualityErrorKind = "non_zero_exit"
	TcpQualityErrorTimeout           TcpQualityErrorKind = "timeout"
	TcpQualityErrorNoData            TcpQualityErrorKind = "no_data"
	TcpQualityErrorParseFailed       TcpQualityErrorKind = "parse_failed"
	TcpQualityErrorTargetUnresolved  TcpQualityErrorKind = "target_unresolved"
	TcpQualityErrorNoMatchingSocket  TcpQualityErrorKind = "no_matching_socket"
	TcpQualityErrorUnavailable       TcpQualityErrorKind = "unavailable"
)

// TcpQuality represents TCP path quality evidence for the probe destination socket.
// This provides evidence of network path health at the TCP layer during the spike,
// including RTT, retransmits, congestion window, and queue depths.
//
// Privacy: Local and remote addresses are redacted to "redacted" to prevent
// exposure of internal network topology. Ports may be retained if allowed by
// existing underlay_tcp diagnostics.
type TcpQuality struct {
	// Kind is the type of probe that triggered this TCP quality collection.
	// For HTTP probes, this is "http". For ICMP probes, TCP quality is unavailable.
	Kind string `json:"kind"`

	// LookupTarget is the host/IP that was matched (may be redacted).
	LookupTarget string `json:"lookup_target"`

	// MatchedSocket indicates whether a matching TCP socket was found.
	MatchedSocket bool `json:"matched_socket"`

	// Source is the evidence collection tool (e.g., "ss-tcp-info").
	Source string `json:"source"`

	// MatchCount is the number of sockets that matched the target when multiple
	// matches were found. The best match is selected deterministically.
	MatchCount *int `json:"match_count,omitempty"`

	// Socket state and addresses (only when MatchedSocket is true)
	// Note: Local and Remote are redacted to "redacted:port" for privacy.
	// LookupTarget may retain the probe destination because it is already present
	// in the diagnostic target identity.
	State          string `json:"state,omitempty"`
	Local          string `json:"local,omitempty"`
	Remote         string `json:"remote,omitempty"`
	// SendQueueBytes and RecvQueueBytes are pointers so that zero (observed) is
	// distinguishable from absent (field not present in ss output). nil means
	// queue data was not available; 0 means queues were observed and empty.
	SendQueueBytes *int64 `json:"send_queue_bytes,omitempty"`
	RecvQueueBytes *int64 `json:"recv_queue_bytes,omitempty"`

	// RTT and RTT variance in microseconds
	RTTUs     *int64 `json:"rtt_us,omitempty"`
	RTTVarUs  *int64 `json:"rttvar_us,omitempty"`

	// Retransmit counters
	RetransmitsCurrent *int64 `json:"retransmits_current,omitempty"`
	RetransmitsTotal   *int64 `json:"retransmits_total,omitempty"`

	// Loss and acknowledgment signals
	Unacked   *int64 `json:"unacked,omitempty"`
	Lost      *int64 `json:"lost,omitempty"`
	Sacked    *int64 `json:"sacked,omitempty"`
	Reordering *int64 `json:"reordering,omitempty"`

	// Congestion control
	SndCwnd              *int32 `json:"snd_cwnd,omitempty"`
	Ssthresh             *int32 `json:"ssthresh,omitempty"`
	DeliveryRateBps     *int64 `json:"delivery_rate_bps,omitempty"`
	CongestionAlgorithm  string `json:"congestion_algorithm,omitempty"`

	// Error fields (only when MatchedSocket is false)
	ErrorKind TcpQualityErrorKind `json:"error_kind,omitempty"`
	Error     string              `json:"error,omitempty"`

	// CollectedAt is when the TCP quality data was collected.
	CollectedAt string `json:"collected_at"`
}

// NewTcpQualityUnavailable creates a TcpQuality block indicating TCP quality is unavailable.
func NewTcpQualityUnavailable(kind string, target string, errorKind TcpQualityErrorKind, errMsg string) *TcpQuality {
	return &TcpQuality{
		Kind:           kind,
		LookupTarget:  target,
		MatchedSocket: false,
		Source:        "ss-tcp-info",
		ErrorKind:     errorKind,
		Error:         errMsg,
		CollectedAt:   time.Now().UTC().Format(time.RFC3339),
	}
}

// NewTcpQualitySuccess creates a TcpQuality block with successful socket evidence.
func NewTcpQualitySuccess(kind string, target string) *TcpQuality {
	return &TcpQuality{
		Kind:           kind,
		LookupTarget:  target,
		MatchedSocket: true,
		Source:        "ss-tcp-info",
		CollectedAt:   time.Now().UTC().Format(time.RFC3339),
	}
}
