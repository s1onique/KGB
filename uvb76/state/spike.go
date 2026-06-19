// Package state provides bounded in-memory state management for UVB-76.
package state

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"strings"
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
	CollectedAt      time.Time         `json:"collected_at"`      // when spike was recorded
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

// spikeTracker tracks spike events for a single target/probe kind combination.
type spikeTracker struct {
	mu          sync.Mutex
	events      []SpikeEvent // ring buffer of spike events
	maxEvents   int          // max events to retain
	config      SpikeConfig
	kind        string       // "http" or "icmp"
}

// newSpikeTracker creates a new spike tracker with the given configuration.
func newSpikeTracker(kind string, config SpikeConfig) *spikeTracker {
	return &spikeTracker{
		events:    make([]SpikeEvent, 0, config.MaxEventsPerTracker),
		maxEvents: config.MaxEventsPerTracker,
		config:    config,
		kind:      kind,
	}
}

// recordSpike records a spike event, evicting oldest if at capacity.
// EVICTION POLICY: Capture-aware spike retention
// - Protected spikes (with captures or in-flight) are NEVER evicted
// - Only purge-eligible spikes count against the uncaptured cap
// - maxEvents limits total storage, but protected spikes don't count toward it
func (st *spikeTracker) recordSpike(event SpikeEvent, getProtectionInfo func(eventID string) (isProtected bool, hasArtifact bool)) {
	st.mu.Lock()
	defer st.mu.Unlock()

	// Count purge-eligible (uncaptured) spikes
	purgeableCount := 0
	for _, ev := range st.events {
		isProtected, _ := getProtectionInfo(ev.EventID)
		if !isProtected {
			purgeableCount++
		}
	}

	// Evict oldest purge-eligible spike if we're at the uncaptured cap
	if purgeableCount >= st.config.MaxEventsPerTracker {
		// Find and remove oldest purge-eligible spike
		for i := 0; i < len(st.events); i++ {
			isProtected, _ := getProtectionInfo(st.events[i].EventID)
			if !isProtected {
				// Remove this purge-eligible spike
				st.events = append(st.events[:i], st.events[i+1:]...)
				break
			}
		}
	}

	st.events = append(st.events, event)
}

// getEvents returns all spike events (newest first for display).
func (st *spikeTracker) getEvents(limit int) []SpikeEvent {
	st.mu.Lock()
	defer st.mu.Unlock()

	if limit <= 0 || limit > len(st.events) {
		limit = len(st.events)
	}

	// Return newest first (reverse order)
	result := make([]SpikeEvent, limit)
	for i := 0; i < limit; i++ {
		result[i] = st.events[len(st.events)-1-i]
	}
	return result
}

// count returns the number of spike events.
func (st *spikeTracker) count() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.events)
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
// for hard failures like timeouts, connection refused, etc.
func (sd *SpikeDetector) DetectAndRecord(
	targetID, kind string,
	latencyMs float64,
	sampleTs time.Time,
	reached bool,
	schedulerDelayMs *float64,
	httpStatus *int,
	probeError *string,
	previousSamples []LatencySample,
) *SpikeEvent {
	// Determine thresholds based on probe kind
	var warningMs, criticalMs float64
	switch kind {
	case "icmp":
		warningMs = sd.config.ICMPWarningMs
		criticalMs = sd.config.ICMPCriticalMs
	case "http":
		warningMs = sd.config.HTTPWarningMs
		criticalMs = sd.config.HTTPCriticalMs
	default:
		return nil
	}

	// Calculate rolling median from previous samples
	medianMs := sd.calculateMedian(previousSamples)

	// Check spike conditions - use highest severity threshold only
	var severity string
	var reasons []string

	// Check for probe failure FIRST - HTTP failures are first-class diagnostic events
	// ICMP failures continue to use latency-based spike detection (different semantics)
	if !reached && kind == "http" {
		// HTTP probe failure is always significant, regardless of latency
		severity = "critical"
		
		// Determine failure type based on error message
		if probeError != nil {
			errStr := *probeError
			if len(errStr) > 0 {
				// Case-insensitive check for error type
				errLower := strings.ToLower(errStr)
				if strings.Contains(errLower, "timeout") || strings.Contains(errLower, "deadline") {
					reasons = append(reasons, "http_probe_timeout")
				} else if strings.Contains(errLower, "connection refused") {
					reasons = append(reasons, "http_probe_connection_refused")
				} else if strings.Contains(errLower, "no such host") || strings.Contains(errLower, "lookup") || strings.Contains(errLower, "dial") {
					reasons = append(reasons, "http_probe_dns_failure")
				} else if strings.Contains(errLower, "connection reset") {
					reasons = append(reasons, "http_probe_connection_reset")
				} else {
					reasons = append(reasons, "http_probe_failure")
				}
			} else {
				reasons = append(reasons, "http_probe_failure")
			}
		} else {
			reasons = append(reasons, "http_probe_failure")
		}
		
		// Still check if relative threshold also exceeded (for evidence)
		if sd.shouldIncludeRelativeReason(previousSamples, medianMs, latencyMs) {
			reasons = append(reasons, "relative_10x_median_threshold")
		}
	} else {
		// Normal latency spike detection for successful probes
		// Check absolute thresholds first (highest priority)
		if latencyMs >= criticalMs {
			severity = "critical"
			reasons = append(reasons, kind+"_critical_absolute_threshold")
			// Check if relative threshold also exceeded (for evidence)
			if sd.shouldIncludeRelativeReason(previousSamples, medianMs, latencyMs) {
				reasons = append(reasons, "relative_10x_median_threshold")
			}
		} else if latencyMs >= warningMs {
			severity = "warning"
			reasons = append(reasons, kind+"_warning_absolute_threshold")
			// Check if relative threshold also exceeded significantly (for evidence)
			if sd.shouldIncludeRelativeReason(previousSamples, medianMs, latencyMs) {
				reasons = append(reasons, "relative_10x_median_threshold")
			}
		} else if sd.shouldIncludeRelativeReason(previousSamples, medianMs, latencyMs) {
			// Only relative threshold triggered
			severity = "warning"
			reasons = append(reasons, "relative_10x_median_threshold")
		}
	}

	// No spike detected
	if len(reasons) == 0 {
		return nil
	}

	// Build spike event
	tracker := sd.getTracker(targetID, kind)

	// Convert previous samples to spike samples (bounded window)
	prevSamples := sd.boundPreviousSamples(previousSamples)
	spikePrevSamples := make([]SpikeSample, len(prevSamples))
	for i, s := range prevSamples {
		spikePrevSamples[i] = SpikeSample{
			Ts:        s.Timestamp,
			LatencyMs: s.LatencyMs,
			OK:        s.Reachable,
		}
	}

	event := SpikeEvent{
		EventID:          generateEventID(),
		TargetID:         targetID,
		Kind:             kind,
		Severity:         severity,
		SampleTs:         sampleTs,
		LatencyMs:        latencyMs,
		RollingMedianMs:  medianMs,
		Reasons:          reasons,
		Thresholds: SpikeThresholds{
			WarningMs:          warningMs,
			CriticalMs:         criticalMs,
			RelativeMultiplier: sd.config.RelativeMultiplier,
		},
		PreviousSamples:   spikePrevSamples,
		SchedulerDelayMs:  schedulerDelayMs,
		HTTPStatus:        httpStatus,
		ProbeError:        probeError,
		CollectedAt:       time.Now().UTC(),
	}

	// Use capture-aware eviction if configured
	sd.mu.RLock()
	captureFunc := sd.captureInfoFunc
	sd.mu.RUnlock()
	
	if captureFunc != nil {
		tracker.recordSpike(event, captureFunc)
	} else {
		// Fallback: use simple eviction function that always allows eviction
		tracker.recordSpike(event, func(eventID string) (isProtected bool, hasCapture bool) {
			return false, false
		})
	}
	return &event
}

// calculateMedian computes the median of successful samples from the previous samples.
func (sd *SpikeDetector) calculateMedian(samples []LatencySample) float64 {
	if len(samples) == 0 {
		return 0
	}

	// Collect successful latencies
	var latencies []float64
	for _, s := range samples {
		if s.Reachable {
			latencies = append(latencies, s.LatencyMs)
		}
	}

	if len(latencies) == 0 {
		return 0
	}

	// Sort for median calculation
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
