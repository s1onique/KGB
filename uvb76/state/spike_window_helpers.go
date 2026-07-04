package state

import (
	"strings"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// spike_window_helpers.go contains helper functions for spike detection that uses domain.SampleWindow.

// domainSpikeConfigForState converts state SpikeConfig to domain SpikeConfig for a given kind.
// Returns false if the kind is unknown or the config cannot be converted.
func domainSpikeConfigForState(kind string, cfg SpikeConfig) (domain.SpikeConfig, bool) {
	var warningMs, criticalMs float64

	switch kind {
	case "http":
		warningMs = cfg.HTTPWarningMs
		criticalMs = cfg.HTTPCriticalMs
	case "icmp":
		warningMs = cfg.ICMPWarningMs
		criticalMs = cfg.ICMPCriticalMs
	default:
		return domain.SpikeConfig{}, false
	}

	return domain.NewSpikeConfig(
		warningMs,
		criticalMs,
		cfg.RelativeMultiplier,
		cfg.MinSamplesForMedian,
	)
}

// currentDomainSample converts a state sample to a domain.Sample.
// Returns false if the kind is unknown.
func currentDomainSample(
	kind string,
	latencyMs float64,
	sampleTs time.Time,
	reached bool,
	probeError *string,
) (domain.Sample, bool) {
	var probeKind domain.ProbeKind
	switch kind {
	case "http":
		probeKind = domain.ProbeKindHTTP
	case "icmp":
		probeKind = domain.ProbeKindICMP
	default:
		return domain.Sample{}, false
	}

	if !reached {
		errText := ""
		if probeError != nil {
			errText = *probeError
		}
		return domain.Sample{
			At:   sampleTs,
			Kind: probeKind,
			OK:   false,
			Err:  errText,
		}, true
	}

	latency, ok := domain.NewLatencyMillis(latencyMs)
	if !ok {
		return domain.Sample{
			At:   sampleTs,
			Kind: probeKind,
			OK:   false,
			Err:  "invalid latency value",
		}, true
	}

	return domain.Sample{
		At:      sampleTs,
		Kind:    probeKind,
		Latency: latency,
		OK:      true,
	}, true
}

// severityFromDomainDecision maps domain spike decision kind to state severity string.
func severityFromDomainDecision(kind domain.SpikeDecisionKind) string {
	switch kind {
	case domain.SpikeDecisionCritical:
		return "critical"
	case domain.SpikeDecisionWarning:
		return "warning"
	default:
		return ""
	}
}

// stateReasonsFromDomainDecision maps domain decision reasons to state-specific reason strings.
// Maintains ordering: absolute threshold reasons first, relative threshold second.
func stateReasonsFromDomainDecision(kind string, decision domain.SpikeDecision) []string {
	if decision.Kind == domain.SpikeDecisionNone {
		return nil
	}

	reasons := strings.Split(decision.Reason, ",")
	out := make([]string, 0, len(reasons))

	// Map domain reasons to state reasons, preserving order
	for _, reason := range reasons {
		switch reason {
		case "critical_absolute":
			out = append(out, kind+"_critical_absolute_threshold")
		case "warning_absolute":
			out = append(out, kind+"_warning_absolute_threshold")
		case "relative_threshold":
			out = append(out, "relative_10x_median_threshold")
		}
	}

	return out
}

// spikeSamplesFromWindow converts domain.SampleWindow to []SpikeSample with bounded length.
func spikeSamplesFromWindow(previousWindow domain.SampleWindow, maxPrev int) []SpikeSample {
	domainSamples := previousWindow.Samples()
	if len(domainSamples) > maxPrev {
		domainSamples = domainSamples[len(domainSamples)-maxPrev:]
	}

	out := make([]SpikeSample, 0, len(domainSamples))
	for _, s := range domainSamples {
		out = append(out, SpikeSample{
			Ts:        s.At,
			LatencyMs: s.Latency.Float64(),
			OK:        s.OK,
		})
	}
	return out
}
