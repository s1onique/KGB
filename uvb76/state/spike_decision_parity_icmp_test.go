package state

import (
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// spike_decision_parity_icmp_test.go contains ICMP-specific parity tests.
// Helper functions makeDomainSampleWindow and makeDomainSample are defined in spike_decision_parity_http_test.go.

// TestSpikeDecisionParity_ICMP_NormalLatency tests that ICMP normal latency produces no spike.
func TestSpikeDecisionParity_ICMP_NormalLatency(t *testing.T) {
	detector := NewSpikeDetector()
	window := makeDomainSampleWindow(30, 30.0, domain.ProbeKindICMP)
	currentLatency := 100.0

	domainDecision := domain.DecideSpike(
		makeDomainSample(currentLatency, domain.ProbeKindICMP, true),
		window, domain.DefaultSpikeConfigICMP(),
	)

	stateEvent := detector.DetectAndRecordWithWindow(
		"test-target", "icmp", currentLatency, time.Now().UTC(),
		true, nil, nil, nil, window, nil,
	)

	if domainDecision.Kind != domain.SpikeDecisionNone {
		t.Errorf("domain.DecideSpike expected none, got %s", domainDecision.Kind)
	}
	if stateEvent != nil {
		t.Errorf("state expected nil event, got %v", stateEvent.Severity)
	}
}

// TestSpikeDecisionParity_ICMP_WarningAbsolute tests ICMP warning absolute threshold.
func TestSpikeDecisionParity_ICMP_WarningAbsolute(t *testing.T) {
	detector := NewSpikeDetector()
	window := makeDomainSampleWindow(30, 30.0, domain.ProbeKindICMP)
	currentLatency := 600.0

	domainDecision := domain.DecideSpike(
		makeDomainSample(currentLatency, domain.ProbeKindICMP, true),
		window, domain.DefaultSpikeConfigICMP(),
	)

	stateEvent := detector.DetectAndRecordWithWindow(
		"test-target", "icmp", currentLatency, time.Now().UTC(),
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
}

// TestSpikeDecisionParity_ICMP_CriticalAbsolute tests ICMP critical absolute threshold.
func TestSpikeDecisionParity_ICMP_CriticalAbsolute(t *testing.T) {
	detector := NewSpikeDetector()
	window := makeDomainSampleWindow(30, 30.0, domain.ProbeKindICMP)
	currentLatency := 3000.0

	domainDecision := domain.DecideSpike(
		makeDomainSample(currentLatency, domain.ProbeKindICMP, true),
		window, domain.DefaultSpikeConfigICMP(),
	)

	stateEvent := detector.DetectAndRecordWithWindow(
		"test-target", "icmp", currentLatency, time.Now().UTC(),
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
}

// TestSpikeDecisionParity_ICMP_RelativeOnlySpike tests ICMP relative-only spike.
func TestSpikeDecisionParity_ICMP_RelativeOnlySpike(t *testing.T) {
	detector := NewSpikeDetector()
	window := makeDomainSampleWindow(30, 30.0, domain.ProbeKindICMP)
	// 400ms: below ICMP warning (500ms) but >= relative (300ms threshold: 30 * 10 = 300)
	currentLatency := 400.0

	domainDecision := domain.DecideSpike(
		makeDomainSample(currentLatency, domain.ProbeKindICMP, true),
		window, domain.DefaultSpikeConfigICMP(),
	)

	stateEvent := detector.DetectAndRecordWithWindow(
		"test-target", "icmp", currentLatency, time.Now().UTC(),
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
}
