package state

import "time"

// spike_tcp_quality.go contains the spike detection variant that captures native TCP_INFO
// from the actual HTTP probe socket. This separation keeps spike.go focused on core
// spike detection logic.

// DetectAndRecordWithTcpQuality checks a sample for spike conditions and records if detected.
// Returns the spike event if detected, nil otherwise.
//
// This variant allows passing native TCP_INFO collected from the actual HTTP probe socket,
// which provides native_tcp_info evidence with matched_socket=true.
func (sd *SpikeDetector) DetectAndRecordWithTcpQuality(
	targetID, kind string,
	latencyMs float64,
	sampleTs time.Time,
	reached bool,
	schedulerDelayMs *float64,
	httpStatus *int,
	probeError *string,
	previousSamples []LatencySample,
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
				// First, check for explicit http_probe_* reasons embedded in the error string
				// This handles HTTP 5xx/4xx classification from probe.go
				errLower := lower(errStr)
				
				// Check 5xx specific codes (most specific first)
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
					// Generic timeout patterns (context deadline exceeded, etc.)
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
		HTTPTrace:         httpTrace,
		NativeTcpQuality:  nativeTcpQuality,
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

// lower returns s lowercased.
func lower(s string) string {
	// Use a simple byte loop for small strings to avoid import
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// contains reports whether substr is within s.
func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
