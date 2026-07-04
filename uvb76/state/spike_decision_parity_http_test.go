package state

import (
	"strings"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// spike_decision_parity_http_test.go contains HTTP-specific parity tests between domain.DecideSpike and state spike detection.

// makeDomainSampleWindow creates a domain.SampleWindow from a number of samples with the same latency and kind.
func makeDomainSampleWindow(samples int, latencyMs float64, kind domain.ProbeKind) domain.SampleWindow {
	now := time.Now().UTC()
	domainSamples := make([]domain.Sample, 0, samples)
	for i := 0; i < samples; i++ {
		lat, ok := domain.NewLatencyMillis(latencyMs)
		if !ok {
			panic("test latency must be valid")
		}
		domainSamples = append(domainSamples, domain.Sample{
			At:      now.Add(-time.Duration(samples-i) * time.Second),
			Kind:    kind,
			Latency: lat,
			OK:      true,
		})
	}
	return domain.NewSampleWindow(domainSamples)
}

// makeDomainSample creates a domain.Sample for the current sample.
func makeDomainSample(latencyMs float64, kind domain.ProbeKind, ok bool) domain.Sample {
	s := domain.Sample{At: time.Now().UTC(), Kind: kind, OK: ok}
	if ok {
		lat, _ := domain.NewLatencyMillis(latencyMs)
		s.Latency = lat
	}
	return s
}

// TestSpikeDecisionParity_HTTP_NormalLatency tests that HTTP normal latency produces no spike.
func TestSpikeDecisionParity_HTTP_NormalLatency(t *testing.T) {
	detector := NewSpikeDetector()
	window := makeDomainSampleWindow(30, 50.0, domain.ProbeKindHTTP)
	currentLatency := 100.0

	domainDecision := domain.DecideSpike(
		makeDomainSample(currentLatency, domain.ProbeKindHTTP, true),
		window, domain.DefaultSpikeConfigHTTP(),
	)

	stateEvent := detector.DetectAndRecordWithWindow(
		"test-target", "http", currentLatency, time.Now().UTC(),
		true, nil, nil, nil, window, nil,
	)

	if domainDecision.Kind != domain.SpikeDecisionNone {
		t.Errorf("domain.DecideSpike expected none, got %s", domainDecision.Kind)
	}
	if stateEvent != nil {
		t.Errorf("state expected nil event, got %v", stateEvent.Severity)
	}
}

// TestSpikeDecisionParity_HTTP_WarningAbsolute tests HTTP warning absolute threshold.
func TestSpikeDecisionParity_HTTP_WarningAbsolute(t *testing.T) {
	detector := NewSpikeDetector()
	window := makeDomainSampleWindow(30, 50.0, domain.ProbeKindHTTP)
	currentLatency := 2000.0

	domainDecision := domain.DecideSpike(
		makeDomainSample(currentLatency, domain.ProbeKindHTTP, true),
		window, domain.DefaultSpikeConfigHTTP(),
	)

	stateEvent := detector.DetectAndRecordWithWindow(
		"test-target", "http", currentLatency, time.Now().UTC(),
		true, nil, nil, nil, window, nil,
	)

	if domainDecision.Kind != domain.SpikeDecisionWarning {
		t.Errorf("domain.DecideSpike expected warning, got %s", domainDecision.Kind)
	}
	if stateEvent == nil {
		t.Fatal("state expected event, got nil")
	}
	if stateEvent.Severity != "warning" {
		t.Errorf("state expected severity=warning, got %s", stateEvent.Severity)
	}

	hasWarningReason := false
	for _, r := range stateEvent.Reasons {
		if strings.Contains(r, "warning_absolute_threshold") {
			hasWarningReason = true
			break
		}
	}
	if !hasWarningReason {
		t.Errorf("state expected warning_absolute_threshold reason, got %v", stateEvent.Reasons)
	}
}

// TestSpikeDecisionParity_HTTP_CriticalAbsolute tests HTTP critical absolute threshold.
func TestSpikeDecisionParity_HTTP_CriticalAbsolute(t *testing.T) {
	detector := NewSpikeDetector()
	window := makeDomainSampleWindow(30, 50.0, domain.ProbeKindHTTP)
	currentLatency := 6000.0

	domainDecision := domain.DecideSpike(
		makeDomainSample(currentLatency, domain.ProbeKindHTTP, true),
		window, domain.DefaultSpikeConfigHTTP(),
	)

	stateEvent := detector.DetectAndRecordWithWindow(
		"test-target", "http", currentLatency, time.Now().UTC(),
		true, nil, nil, nil, window, nil,
	)

	if domainDecision.Kind != domain.SpikeDecisionCritical {
		t.Errorf("domain.DecideSpike expected critical, got %s", domainDecision.Kind)
	}
	if stateEvent == nil {
		t.Fatal("state expected event, got nil")
	}
	if stateEvent.Severity != "critical" {
		t.Errorf("state expected severity=critical, got %s", stateEvent.Severity)
	}

	hasCriticalReason := false
	for _, r := range stateEvent.Reasons {
		if strings.Contains(r, "critical_absolute_threshold") {
			hasCriticalReason = true
			break
		}
	}
	if !hasCriticalReason {
		t.Errorf("state expected critical_absolute_threshold reason, got %v", stateEvent.Reasons)
	}
}

// TestSpikeDecisionParity_HTTP_RelativeOnlySpike tests HTTP relative-only spike.
func TestSpikeDecisionParity_HTTP_RelativeOnlySpike(t *testing.T) {
	detector := NewSpikeDetector()
	window := makeDomainSampleWindow(30, 50.0, domain.ProbeKindHTTP)
	// 600ms: below HTTP warning (1000ms) but >= relative (500ms threshold)
	currentLatency := 600.0

	domainDecision := domain.DecideSpike(
		makeDomainSample(currentLatency, domain.ProbeKindHTTP, true),
		window, domain.DefaultSpikeConfigHTTP(),
	)

	stateEvent := detector.DetectAndRecordWithWindow(
		"test-target", "http", currentLatency, time.Now().UTC(),
		true, nil, nil, nil, window, nil,
	)

	if domainDecision.Kind != domain.SpikeDecisionWarning {
		t.Errorf("domain.DecideSpike expected warning, got %s", domainDecision.Kind)
	}
	if stateEvent == nil {
		t.Fatal("state expected event, got nil")
	}
	if stateEvent.Severity != "warning" {
		t.Errorf("state expected severity=warning, got %s", stateEvent.Severity)
	}

	hasRelativeReason := false
	for _, r := range stateEvent.Reasons {
		if strings.Contains(r, "relative_10x_median_threshold") {
			hasRelativeReason = true
			break
		}
	}
	if !hasRelativeReason {
		t.Errorf("state expected relative_10x_median_threshold reason, got %v", stateEvent.Reasons)
	}
}

// TestSpikeDecisionParity_HTTP_InsufficientBaselineSamples tests HTTP with insufficient baseline samples.
func TestSpikeDecisionParity_HTTP_InsufficientBaselineSamples(t *testing.T) {
	detector := NewSpikeDetector()
	window := makeDomainSampleWindow(5, 50.0, domain.ProbeKindHTTP)
	currentLatency := 800.0

	domainDecision := domain.DecideSpike(
		makeDomainSample(currentLatency, domain.ProbeKindHTTP, true),
		window, domain.DefaultSpikeConfigHTTP(),
	)

	stateEvent := detector.DetectAndRecordWithWindow(
		"test-target", "http", currentLatency, time.Now().UTC(),
		true, nil, nil, nil, window, nil,
	)

	// 800ms < 1000ms warning, insufficient samples for relative
	if domainDecision.Kind != domain.SpikeDecisionNone {
		t.Errorf("domain.DecideSpike expected none, got %s", domainDecision.Kind)
	}
	if stateEvent != nil {
		t.Errorf("state expected nil event, got %v", stateEvent.Severity)
	}
}

// TestSpikeDecisionParity_HTTP_EmptyWindowCriticalLatency tests HTTP with empty window.
func TestSpikeDecisionParity_HTTP_EmptyWindowCriticalLatency(t *testing.T) {
	detector := NewSpikeDetector()
	emptyWindow := domain.SampleWindow{}
	currentLatency := 6000.0

	domainDecision := domain.DecideSpike(
		makeDomainSample(currentLatency, domain.ProbeKindHTTP, true),
		emptyWindow, domain.DefaultSpikeConfigHTTP(),
	)

	stateEvent := detector.DetectAndRecordWithWindow(
		"test-target", "http", currentLatency, time.Now().UTC(),
		true, nil, nil, nil, emptyWindow, nil,
	)

	if domainDecision.Kind != domain.SpikeDecisionCritical {
		t.Errorf("domain.DecideSpike expected critical, got %s", domainDecision.Kind)
	}
	if stateEvent == nil {
		t.Fatal("state expected event, got nil")
	}
	if stateEvent.Severity != "critical" {
		t.Errorf("state expected severity=critical, got %s", stateEvent.Severity)
	}
}
