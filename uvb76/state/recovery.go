// Package state provides bounded in-memory state management for UVB-76.
package state

import (
	"time"
)

// ReachabilityState represents the previous reachability state for a target.
// Uses tri-state to avoid false-positive recovery detection.
type ReachabilityState int

const (
	ReachabilityUnknown     ReachabilityState = iota
	ReachabilityReachable
	ReachabilityUnreachable
)

// RecordRecoveryEvent explicitly records a recovery event when a target becomes
// reachable again after being unreachable. This is separate from latency spike
// detection because recovery is about state transition, not latency magnitude.
func (sd *SpikeDetector) RecordRecoveryEvent(
	targetID, kind string,
	latencyMs float64,
	sampleTs time.Time,
	httpStatus *int,
	previousSamples []LatencySample,
) *SpikeEvent {
	if kind != "http" {
		return nil
	}

	medianMs := sd.calculateMedian(previousSamples)
	tracker := sd.getTracker(targetID, kind)
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
		EventID:         generateEventID(),
		TargetID:        targetID,
		Kind:            kind,
		Severity:        "warning",
		SampleTs:        sampleTs,
		LatencyMs:      latencyMs,
		RollingMedianMs: medianMs,
		Reasons:         []string{"http_probe_recovery"},
		Thresholds: SpikeThresholds{
			WarningMs:          sd.config.HTTPWarningMs,
			CriticalMs:         sd.config.HTTPCriticalMs,
			RelativeMultiplier:  sd.config.RelativeMultiplier,
		},
		PreviousSamples: spikePrevSamples,
		HTTPStatus:     httpStatus,
		CollectedAt:    time.Now().UTC(),
	}

	sd.mu.RLock()
	captureFunc := sd.captureInfoFunc
	sd.mu.RUnlock()

	if captureFunc != nil {
		tracker.recordSpike(event, captureFunc)
	} else {
		tracker.recordSpike(event, func(eventID string) (isProtected bool, hasArtifact bool) {
			return false, false
		})
	}
	return &event
}
