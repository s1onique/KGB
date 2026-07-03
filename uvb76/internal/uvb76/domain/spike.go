package domain

// SpikeDecisionKind classifies the severity of a detected spike.
type SpikeDecisionKind string

const (
	SpikeDecisionNone     SpikeDecisionKind = "none"
	SpikeDecisionWarning  SpikeDecisionKind = "warning"
	SpikeDecisionCritical SpikeDecisionKind = "critical"
)

// SpikeDecision represents a pure spike detection decision.
type SpikeDecision struct {
	Kind          SpikeDecisionKind
	Reason        string
	Baseline      LatencyMillis
	Observed      LatencyMillis
	PreviousCount int
}

// SpikeConfig holds spike detection thresholds.
type SpikeConfig struct {
	WarningAbsoluteMillis  LatencyMillis
	CriticalAbsoluteMillis LatencyMillis
	RelativeMultiplier     float64
	MinSamplesForMedian    int
}

// NewSpikeConfig creates a SpikeConfig from raw float64 values.
// Returns false if any threshold value is invalid.
func NewSpikeConfig(warningMs, criticalMs, relativeMultiplier float64, minSamples int) (SpikeConfig, bool) {
	warn, ok := NewLatencyMillis(warningMs)
	if !ok {
		return SpikeConfig{}, false
	}
	crit, ok := NewLatencyMillis(criticalMs)
	if !ok {
		return SpikeConfig{}, false
	}
	return SpikeConfig{
		WarningAbsoluteMillis:  warn,
		CriticalAbsoluteMillis: crit,
		RelativeMultiplier:     relativeMultiplier,
		MinSamplesForMedian:    minSamples,
	}, true
}

// DefaultSpikeConfigHTTP returns sensible defaults for HTTP probes.
func DefaultSpikeConfigHTTP() SpikeConfig {
	return SpikeConfig{
		WarningAbsoluteMillis:  LatencyMillis{v: 1000},  // 1 second
		CriticalAbsoluteMillis: LatencyMillis{v: 5000},  // 5 seconds
		RelativeMultiplier:     10.0,
		MinSamplesForMedian:    20,
	}
}

// DefaultSpikeConfigICMP returns sensible defaults for ICMP probes.
func DefaultSpikeConfigICMP() SpikeConfig {
	return SpikeConfig{
		WarningAbsoluteMillis:  LatencyMillis{v: 500},  // 500ms
		CriticalAbsoluteMillis: LatencyMillis{v: 2000}, // 2 seconds
		RelativeMultiplier:     10.0,
		MinSamplesForMedian:    20,
	}
}

// DecideSpike is a pure function that determines if the current sample constitutes a spike.
// It does not perform any I/O, logging, or state mutation.
//
// Rules:
// - Failed samples (OK=false) do not produce latency-based spikes
// - No relative spike if there is no valid baseline median
// - Critical beats warning when both thresholds are exceeded
// - Relative threshold requires sane multiplier and sufficient samples
// - Empty or short windows do not panic
func DecideSpike(current Sample, previous SampleWindow, cfg SpikeConfig) SpikeDecision {
	// Failed samples do not produce latency-based spikes
	if !current.OK {
		return SpikeDecision{Kind: SpikeDecisionNone}
	}

	// Get baseline median from previous samples
	baseline, baselineOK := previous.Median()

	// Track reasons for the spike decision
	var reasons []string

	// Check absolute thresholds (independent of relative threshold)
	if current.Latency.Float64() >= cfg.CriticalAbsoluteMillis.Float64() {
		reasons = append(reasons, "critical_absolute")
	}
	if current.Latency.Float64() >= cfg.WarningAbsoluteMillis.Float64() {
		reasons = append(reasons, "warning_absolute")
	}

	// Check relative threshold (only if we have a valid baseline and enough samples)
	if baselineOK && previous.Len() >= cfg.MinSamplesForMedian && cfg.RelativeMultiplier > 0 {
		relativeThreshold := baseline.Float64() * cfg.RelativeMultiplier
		if current.Latency.Float64() >= relativeThreshold {
			reasons = append(reasons, "relative_threshold")
		}
	}

	// No spike if no reasons triggered
	if len(reasons) == 0 {
		return SpikeDecision{Kind: SpikeDecisionNone}
	}

	// Determine severity: critical takes precedence over warning
	kind := SpikeDecisionWarning
	hasCritical := false
	for _, r := range reasons {
		if r == "critical_absolute" {
			hasCritical = true
			break
		}
	}

	if hasCritical {
		kind = SpikeDecisionCritical
	}

	return SpikeDecision{
		Kind:          kind,
		Reason:        joinReasons(reasons),
		Baseline:      baseline,
		Observed:      current.Latency,
		PreviousCount: previous.Len(),
	}
}

// joinReasons concatenates spike reasons into a human-readable string.
func joinReasons(reasons []string) string {
	result := ""
	for i, r := range reasons {
		if i > 0 {
			result += ","
		}
		result += r
	}
	return result
}
