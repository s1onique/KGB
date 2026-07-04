package state

import (
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// spike_window.go contains spike detection variants that accept domain.SampleWindow.
// These methods enforce the FP01 domain boundary by consuming immutable analysis
// snapshots instead of raw []LatencySample.

// DetectAndRecordSpikeWithWindow checks a sample for spike conditions using a
// domain.SampleWindow for previous samples. This is the preferred method for
// production code outside of uvb76/state.
func (m *Manager) DetectAndRecordSpikeWithWindow(
	targetID, kind string,
	latencyMs float64,
	sampleTs time.Time,
	reachable bool,
	schedulerDelayMs *float64,
	httpStatus *int,
	probeError *string,
	previousWindow domain.SampleWindow,
	httpTrace *HTTPTrace,
) *SpikeEvent {
	return m.spikeDetector.DetectAndRecordWithWindow(
		targetID, kind, latencyMs, sampleTs, reachable,
		schedulerDelayMs, httpStatus, probeError, previousWindow, httpTrace,
	)
}

// DetectAndRecordSpikeWithWindowAndTcpQuality checks a sample for spike conditions
// using a domain.SampleWindow for previous samples and includes native TCP_INFO.
func (m *Manager) DetectAndRecordSpikeWithWindowAndTcpQuality(
	targetID, kind string,
	latencyMs float64,
	sampleTs time.Time,
	reachable bool,
	schedulerDelayMs *float64,
	httpStatus *int,
	probeError *string,
	previousWindow domain.SampleWindow,
	httpTrace *HTTPTrace,
	nativeTcpQuality *TcpQuality,
) *SpikeEvent {
	return m.spikeDetector.DetectAndRecordWithWindowAndTcpQuality(
		targetID, kind, latencyMs, sampleTs, reachable,
		schedulerDelayMs, httpStatus, probeError, previousWindow, httpTrace,
		nativeTcpQuality,
	)
}

// RecordRecoveryEventWithWindow records a recovery event using domain.SampleWindow.
func (m *Manager) RecordRecoveryEventWithWindow(
	targetID, kind string,
	latencyMs float64,
	sampleTs time.Time,
	httpStatus *int,
	previousWindow domain.SampleWindow,
	httpTrace *HTTPTrace,
) *SpikeEvent {
	return m.spikeDetector.RecordRecoveryEventWithWindow(
		targetID, kind, latencyMs, sampleTs, httpStatus, previousWindow, httpTrace,
	)
}

// DetectAndRecordWithWindow is the SpikeDetector variant that accepts domain.SampleWindow.
func (sd *SpikeDetector) DetectAndRecordWithWindow(
	targetID, kind string,
	latencyMs float64,
	sampleTs time.Time,
	reached bool,
	schedulerDelayMs *float64,
	httpStatus *int,
	probeError *string,
	previousWindow domain.SampleWindow,
	httpTrace *HTTPTrace,
) *SpikeEvent {
	return sd.DetectAndRecordWithWindowAndTcpQuality(
		targetID, kind, latencyMs, sampleTs, reached,
		schedulerDelayMs, httpStatus, probeError, previousWindow, httpTrace, nil,
	)
}

// DetectAndRecordWithWindowAndTcpQuality is the SpikeDetector variant that accepts
// domain.SampleWindow and native TCP_INFO.
func (sd *SpikeDetector) DetectAndRecordWithWindowAndTcpQuality(
	targetID, kind string,
	latencyMs float64,
	sampleTs time.Time,
	reached bool,
	schedulerDelayMs *float64,
	httpStatus *int,
	probeError *string,
	previousWindow domain.SampleWindow,
	httpTrace *HTTPTrace,
	nativeTcpQuality *TcpQuality,
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

	// HTTP failure is a first-class diagnostic event - state-owned special case.
	// Do not route through domain.DecideSpike.
	if !reached && kind == "http" {
		return sd.recordHTTPFailureEvent(
			targetID, kind, latencyMs, sampleTs,
			warningMs, criticalMs,
			schedulerDelayMs, httpStatus, probeError,
			previousWindow, httpTrace, nativeTcpQuality,
		)
	}

	// Normal latency spike detection: route through domain.DecideSpike
	domainCfg, ok := domainSpikeConfigForState(kind, sd.config)
	if !ok {
		return nil
	}

	current, ok := currentDomainSample(kind, latencyMs, sampleTs, reached, probeError)
	if !ok {
		return nil
	}

	decision := domain.DecideSpike(current, previousWindow, domainCfg)
	if decision.Kind == domain.SpikeDecisionNone {
		return nil
	}

	// Build spike event using domain decision
	tracker := sd.getTracker(targetID, kind)

	// Get rolling median from domain decision baseline
	medianMs := 0.0
	if decision.PreviousCount > 0 {
		medianMs = decision.Baseline.Float64()
	}

	// Map domain reasons to state reasons
	reasons := stateReasonsFromDomainDecision(kind, decision)

	event := SpikeEvent{
		EventID:          generateEventID(),
		TargetID:         targetID,
		Kind:             kind,
		Severity:         severityFromDomainDecision(decision.Kind),
		SampleTs:         sampleTs,
		LatencyMs:        latencyMs,
		RollingMedianMs:  medianMs,
		Reasons:          reasons,
		Thresholds: SpikeThresholds{
			WarningMs:           warningMs,
			CriticalMs:          criticalMs,
			RelativeMultiplier: sd.config.RelativeMultiplier,
		},
		PreviousSamples:  spikeSamplesFromWindow(previousWindow, sd.config.MaxPreviousSamples),
		SchedulerDelayMs: schedulerDelayMs,
		HTTPStatus:       httpStatus,
		ProbeError:       probeError,
		HTTPTrace:        httpTrace,
		NativeTcpQuality: nativeTcpQuality,
		CollectedAt:      time.Now().UTC(),
	}

	// Use capture-aware eviction if configured
	sd.mu.RLock()
	captureFunc := sd.captureInfoFunc
	sd.mu.RUnlock()

	if captureFunc != nil {
		tracker.recordSpike(event, captureFunc)
	} else {
		tracker.recordSpike(event, func(eventID string) (isProtected bool, hasCapture bool) {
			return false, false
		})
	}
	return &event
}

// recordHTTPFailureEvent handles HTTP probe failures as first-class diagnostic events.
// This is intentionally state-owned and does NOT use domain.DecideSpike.
func (sd *SpikeDetector) recordHTTPFailureEvent(
	targetID, kind string,
	latencyMs float64,
	sampleTs time.Time,
	warningMs, criticalMs float64,
	schedulerDelayMs *float64,
	httpStatus *int,
	probeError *string,
	previousWindow domain.SampleWindow,
	httpTrace *HTTPTrace,
	nativeTcpQuality *TcpQuality,
) *SpikeEvent {
	severity := "critical"
	var reasons []string

	if probeError != nil {
		errStr := *probeError
		if len(errStr) > 0 {
			errLower := lower(errStr)

			if contains(errLower, "http_probe_503") {
				reasons = append(reasons, "http_probe_503")
			} else if contains(errLower, "http_probe_502") {
				reasons = append(reasons, "http_probe_502")
			} else if contains(errLower, "http_probe_504") {
				reasons = append(reasons, "http_probe_504")
			} else if contains(errLower, "http_probe_5xx") {
				reasons = append(reasons, "http_probe_5xx")
			} else if contains(errLower, "http_probe_timeout") {
				reasons = append(reasons, "http_probe_timeout")
			} else if contains(errLower, "timeout") || contains(errLower, "deadline") {
				reasons = append(reasons, "http_probe_timeout")
			} else if contains(errLower, "connection refused") {
				reasons = append(reasons, "http_probe_connection_refused")
			} else if contains(errLower, "no such host") || contains(errLower, "lookup") || contains(errLower, "dial") {
				reasons = append(reasons, "http_probe_dns_failure")
			} else if contains(errLower, "connection reset") {
				reasons = append(reasons, "http_probe_connection_reset")
			} else if contains(errLower, "http_probe_404") {
				reasons = append(reasons, "http_probe_404")
			} else if contains(errLower, "http_probe_4xx") {
				reasons = append(reasons, "http_probe_4xx")
			} else {
				reasons = append(reasons, "http_probe_failure")
			}
		} else {
			reasons = append(reasons, "http_probe_failure")
		}
	} else {
		reasons = append(reasons, "http_probe_failure")
	}

	// Check relative threshold even for HTTP failures
	baseline, baselineOK := previousWindow.Median()
	var medianMs float64
	if baselineOK {
		medianMs = baseline.Float64()
		if baselineOK && previousWindow.Len() >= sd.config.MinSamplesForMedian && sd.config.RelativeMultiplier > 0 {
			relativeThreshold := medianMs * sd.config.RelativeMultiplier
			if latencyMs >= relativeThreshold {
				reasons = append(reasons, "relative_10x_median_threshold")
			}
		}
	}

	// Build spike event
	tracker := sd.getTracker(targetID, kind)

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
			WarningMs:           warningMs,
			CriticalMs:          criticalMs,
			RelativeMultiplier: sd.config.RelativeMultiplier,
		},
		PreviousSamples:  spikeSamplesFromWindow(previousWindow, sd.config.MaxPreviousSamples),
		SchedulerDelayMs: schedulerDelayMs,
		HTTPStatus:       httpStatus,
		ProbeError:       probeError,
		HTTPTrace:        httpTrace,
		NativeTcpQuality: nativeTcpQuality,
		CollectedAt:      time.Now().UTC(),
	}

	// Use capture-aware eviction if configured
	sd.mu.RLock()
	captureFunc := sd.captureInfoFunc
	sd.mu.RUnlock()

	if captureFunc != nil {
		tracker.recordSpike(event, captureFunc)
	} else {
		tracker.recordSpike(event, func(eventID string) (isProtected bool, hasCapture bool) {
			return false, false
		})
	}
	return &event
}

// RecordRecoveryEventWithWindow records a recovery event using domain.SampleWindow.
func (sd *SpikeDetector) RecordRecoveryEventWithWindow(
	targetID, kind string,
	latencyMs float64,
	sampleTs time.Time,
	httpStatus *int,
	previousWindow domain.SampleWindow,
	httpTrace *HTTPTrace,
) *SpikeEvent {
	// Recovery events are similar to successful probes with a special reason
	// We create a "recovery" spike event with specific reason
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

	// Get baseline median
	baseline, baselineOK := previousWindow.Median()
	var medianMs float64
	if baselineOK {
		medianMs = baseline.Float64()
	}

	// Build spike event for recovery
	tracker := sd.getTracker(targetID, kind)

	// Recovery events use "http_probe_recovery" as the reason (consistent with other HTTP spike reasons)
	event := SpikeEvent{
		EventID:          generateEventID(),
		TargetID:         targetID,
		Kind:             kind,
		Severity:         "recovery", // Special severity for recovery events
		SampleTs:         sampleTs,
		LatencyMs:        latencyMs,
		RollingMedianMs:  medianMs,
		Reasons:          []string{"http_probe_recovery"},
		Thresholds: SpikeThresholds{
			WarningMs:           warningMs,
			CriticalMs:          criticalMs,
			RelativeMultiplier: sd.config.RelativeMultiplier,
		},
		PreviousSamples: spikeSamplesFromWindow(previousWindow, sd.config.MaxPreviousSamples),
		HTTPStatus:     httpStatus,
		HTTPTrace:      httpTrace,
		CollectedAt:    time.Now().UTC(),
	}

	// Use capture-aware eviction if configured
	sd.mu.RLock()
	captureFunc := sd.captureInfoFunc
	sd.mu.RUnlock()

	if captureFunc != nil {
		tracker.recordSpike(event, captureFunc)
	} else {
		tracker.recordSpike(event, func(eventID string) (isProtected bool, hasCapture bool) {
			return false, false
		})
	}
	return &event
}
