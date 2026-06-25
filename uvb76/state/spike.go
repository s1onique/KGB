// Package state provides bounded in-memory state management for UVB-76.
package state

import (
	"crypto/rand"
	"encoding/hex"
	"math"
	"sort"
	"sync"
	"time"
)

// SpikeEvent represents a detected latency spike event for evidence collection.
type SpikeEvent struct {
	EventID          string            `json:"event_id"`
	TargetID         string            `json:"target_id"`
	Kind             string            `json:"kind"`              // "http" or "icmp"
	Severity         string            `json:"severity"`          // "warning" or "critical"
	SampleTs         time.Time         `json:"sample_ts"`         // timestamp of the spike sample
	LatencyMs        float64           `json:"latency_ms"`        // the spike latency value
	RollingMedianMs  float64           `json:"rolling_median_ms"` // median before spike
	Reasons          []string          `json:"reasons"`           // why this was flagged as spike
	Thresholds       SpikeThresholds   `json:"thresholds"`        // thresholds that triggered
	PreviousSamples  []SpikeSample     `json:"previous_samples"`  // bounded window of samples before spike
	SchedulerDelayMs *float64          `json:"scheduler_delay_ms,omitempty"` // scheduler delay at spike time
	HTTPStatus       *int              `json:"http_status,omitempty"`        // HTTP status code if HTTP probe
	ProbeError       *string           `json:"probe_error,omitempty"`        // error string if probe failed
	HTTPTrace        *HTTPTrace        `json:"http_trace,omitempty"`         // per-phase HTTP timing (HTTP spikes only)
	// NativeTcpQuality holds TCP_INFO collected from the actual HTTP probe socket.
	// This provides native_tcp_info evidence with matched_socket=true.
	// Only populated for HTTP probes when TCP_INFO is successfully collected from the real connection.
	NativeTcpQuality *TcpQuality       `json:"native_tcp_quality,omitempty"` // native TCP_INFO from actual probe socket
	CollectedAt      time.Time         `json:"collected_at"`       // when spike was recorded
}

// SpikeSample represents a single latency sample captured around a spike event.
type SpikeSample struct {
	Ts       time.Time `json:"ts"`
	LatencyMs float64  `json:"latency_ms"`
	OK       bool      `json:"ok"` // reachable
}

// SpikeThresholds documents which thresholds were configured when spike was detected.
type SpikeThresholds struct {
	WarningMs         float64 `json:"warning_ms"`
	CriticalMs        float64 `json:"critical_ms"`
	RelativeMultiplier float64 `json:"relative_multiplier"`
}

// SpikeConfig holds spike detection configuration.
type SpikeConfig struct {
	// ICMP thresholds in milliseconds
	ICMPWarningMs    float64
	ICMPCriticalMs   float64
	// HTTP thresholds in milliseconds
	HTTPWarningMs    float64
	HTTPCriticalMs   float64
	// Relative threshold: latency >= RelativeMultiplier * rolling_median
	RelativeMultiplier float64
	// Minimum samples required to compute rolling median for relative threshold
	MinSamplesForMedian int
	// Maximum previous samples to capture around spike
	MaxPreviousSamples int
	// Maximum spike events to retain per target/probe kind
	MaxEventsPerTracker int
}

// DefaultSpikeConfig returns sensible default configuration for spike detection.
func DefaultSpikeConfig() SpikeConfig {
	return SpikeConfig{
		ICMPWarningMs:        500,   // ms
		ICMPCriticalMs:       2000,  // ms
		HTTPWarningMs:        1000,  // ms
		HTTPCriticalMs:       5000,  // ms
		RelativeMultiplier:   10.0,  // 10x median
		MinSamplesForMedian:  20,    // need at least 20 samples for reliable median
		MaxPreviousSamples:   30,    // capture 30 previous samples
		MaxEventsPerTracker:  100,   // retain 100 spike events per tracker
	}
}

// SpikeDetector handles spike detection and event recording.
type SpikeDetector struct {
	mu              sync.RWMutex
	config          SpikeConfig
	trackers        map[string]*spikeTracker // keyed by "targetID:kind"
	captureInfoFunc func(eventID string) (isProtected bool, hasCapture bool)
}

// NewSpikeDetector creates a new spike detector with default configuration.
func NewSpikeDetector() *SpikeDetector {
	return &SpikeDetector{
		config:          DefaultSpikeConfig(),
		trackers:       make(map[string]*spikeTracker),
		captureInfoFunc: nil, // No capture-aware eviction by default
	}
}

// NewSpikeDetectorWithConfig creates a new spike detector with custom configuration.
func NewSpikeDetectorWithConfig(config SpikeConfig) *SpikeDetector {
	return &SpikeDetector{
		config:          config,
		trackers:       make(map[string]*spikeTracker),
		captureInfoFunc: nil,
	}
}

// SetCaptureInfoFunc sets the function used to determine if a spike is protected by captures.
// This enables capture-aware eviction when the spike detector is used with a capture store.
func (sd *SpikeDetector) SetCaptureInfoFunc(f func(eventID string) (isProtected bool, hasCapture bool)) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.captureInfoFunc = f
}

// getTracker returns or creates a spike tracker for a target/kind combination.
func (sd *SpikeDetector) getTracker(targetID, kind string) *spikeTracker {
	key := targetID + ":" + kind

	sd.mu.RLock()
	tracker, exists := sd.trackers[key]
	sd.mu.RUnlock()

	if exists {
		return tracker
	}

	sd.mu.Lock()
	defer sd.mu.Unlock()

	// Double-check after acquiring write lock
	if tracker, exists = sd.trackers[key]; exists {
		return tracker
	}

	tracker = newSpikeTracker(kind, sd.config)
	sd.trackers[key] = tracker
	return tracker
}

// DetectAndRecord checks a sample for spike conditions and records if detected.
// Returns the spike event if detected, nil otherwise.
//
// HTTP probe failures (reachable=false) are treated as first-class failure events,
// not just latency anomalies. This ensures diagnostic captures are triggered
// for hard failures like timeouts, connection refused, HTTP 5xx errors, etc.
//
// The httpTrace parameter provides per-phase HTTP timing for HTTP spikes, enabling
// attribution of total latency to DNS, TCP connect, TLS handshake, etc.
//
// Note: DetectAndRecordWithTcpQuality is in spike_tcp_quality.go for file size management.
func (sd *SpikeDetector) DetectAndRecord(
	targetID, kind string,
	latencyMs float64,
	sampleTs time.Time,
	reached bool,
	schedulerDelayMs *float64,
	httpStatus *int,
	probeError *string,
	previousSamples []LatencySample,
	httpTrace *HTTPTrace,
) *SpikeEvent {
	// Delegate to DetectAndRecordWithTcpQuality with nil TcpQuality
	return sd.DetectAndRecordWithTcpQuality(
		targetID, kind, latencyMs, sampleTs, reached,
		schedulerDelayMs, httpStatus, probeError,
		previousSamples, httpTrace, nil)
}

// calculateMedian computes the median of successful samples from the previous samples.
//
// DEFENSIVE FUNCTION - NOT FULLY THREAD-SAFE:
// This function assumes the caller provides immutable input. It:
// - Validates slice header sanity (detects corruption from data races upstream)
// - Filters NaN/Inf values that could corrupt sort or cause panics
// - Creates a private copy of latency values before sorting
//
// The caller is responsible for ensuring `samples` is immutable for the duration
// of this call. In production, this is guaranteed by:
// - LatencyTracker.GetRecentSamples() returns a defensive copy under tracker locking.
//   (See TestLatencyTracker_ReturnedSliceIsDefensiveCopy for proof.)
// - The spike detector itself is protected by its own mutex.
func (sd *SpikeDetector) calculateMedian(samples []LatencySample) float64 {
	// Defensive: check for obviously corrupted slice header
	if cap(samples) == 0 && len(samples) > 0 {
		// Corrupted slice header: len > 0 but cap == 0 is invalid
		// This can happen from data races corrupting the slice header
		return 0
	}

	if len(samples) == 0 {
		return 0
	}

	// Defensive: ensure we don't read more elements than capacity allows
	safeLen := len(samples)
	if cap(samples) > 0 && safeLen > cap(samples) {
		// Corrupted slice header: len > cap is impossible from normal Go code
		// This strongly indicates a data race corrupting the slice header
		safeLen = cap(samples)
	}

	if safeLen == 0 {
		return 0
	}

	// Collect successful latencies with defensive copy and NaN/Inf filtering
	// Pre-allocate with expected capacity to avoid dynamic growth races
	latencies := make([]float64, 0, safeLen)
	for i := 0; i < safeLen; i++ {
		s := samples[i]
		if s.Reachable && !math.IsNaN(s.LatencyMs) && !math.IsInf(s.LatencyMs, 0) {
			latencies = append(latencies, s.LatencyMs)
		}
	}

	if len(latencies) == 0 {
		return 0
	}

	// Sort for median calculation (creates private copy for sort)
	sort.Float64s(latencies)

	mid := len(latencies) / 2
	if len(latencies)%2 == 0 {
		return (latencies[mid-1] + latencies[mid]) / 2
	}
	return latencies[mid]
}

// boundPreviousSamples limits the previous samples to the configured max.
func (sd *SpikeDetector) boundPreviousSamples(samples []LatencySample) []LatencySample {
	max := sd.config.MaxPreviousSamples
	if len(samples) <= max {
		return samples
	}
	// Return most recent samples
	return samples[len(samples)-max:]
}

// GetSpikes returns spike events for a target/kind combination.
func (sd *SpikeDetector) GetSpikes(targetID, kind string, limit int) []SpikeEvent {
	tracker := sd.getTracker(targetID, kind)
	return tracker.getEvents(limit)
}

// GetAllSpikeCounts returns the count of spike events for each target/kind.
func (sd *SpikeDetector) GetAllSpikeCounts() map[string]int {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	result := make(map[string]int)
	for key, tracker := range sd.trackers {
		result[key] = tracker.count()
	}
	return result
}

// generateEventID creates a unique event ID using timestamp + random bytes.
// Uses crypto/rand for randomness, avoiding external dependencies like google/uuid.
func generateEventID() string {
	// 8 bytes of random data (64 bits) is sufficient for uniqueness
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails (should not happen)
		return time.Now().UTC().Format(time.RFC3339Nano)
	}
	// Format: timestamp-randomhex
	return time.Now().UTC().Format(time.RFC3339Nano) + "-" + hex.EncodeToString(b)
}

// shouldIncludeRelativeReason checks if the relative 10x median threshold is exceeded
// and there are enough samples to compute a meaningful median.
func (sd *SpikeDetector) shouldIncludeRelativeReason(samples []LatencySample, medianMs, latencyMs float64) bool {
	// Need enough samples for reliable median
	if len(samples) < sd.config.MinSamplesForMedian || medianMs <= 0 {
		return false
	}
	// Check if latency exceeds 10x median
	threshold := medianMs * sd.config.RelativeMultiplier
	return latencyMs >= threshold
}
