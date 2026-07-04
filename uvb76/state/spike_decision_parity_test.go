package state

import (
	"strings"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// spike_decision_parity_test.go contains remaining parity tests: failure handling,
// unknown kind, rolling median consistency, and both reasons tests.

// TestSpikeDecisionParity_FailedCurrentSample tests that failed current sample
// produces no domain latency spike in both.
func TestSpikeDecisionParity_FailedCurrentSample(t *testing.T) {
	detector := NewSpikeDetector()
	window := makeDomainSampleWindow(30, 50.0, domain.ProbeKindHTTP)

	// Domain: OK=false means no latency spike
	domainDecision := domain.DecideSpike(
		domain.Sample{
			At:   time.Now().UTC(),
			Kind: domain.ProbeKindHTTP,
			OK:   false,
			Err:  "connection refused",
		},
		window,
		domain.DefaultSpikeConfigHTTP(),
	)

	// State: HTTP failure is state-owned, so it WILL produce an event
	errStr := "http_probe_connection_refused"
	stateEvent := detector.DetectAndRecordWithWindow(
		"test-target", "http",
		0, time.Now().UTC(),
		false, nil, nil, &errStr, window, nil,
	)

	// Domain: no latency spike for failed sample
	if domainDecision.Kind != domain.SpikeDecisionNone {
		t.Errorf("domain.DecideSpike expected none for failed sample, got %s", domainDecision.Kind)
	}
	// State: HTTP failure produces critical event (state-owned behavior)
	if stateEvent == nil {
		t.Fatal("state expected HTTP failure event, got nil")
	}
	if stateEvent.Severity != "critical" {
		t.Errorf("state expected severity=critical for HTTP failure, got %s", stateEvent.Severity)
	}
}

// TestSpikeDecisionParity_UnknownKind tests that unknown probe kind produces no event in state.
func TestSpikeDecisionParity_UnknownKind(t *testing.T) {
	detector := NewSpikeDetector()

	stateEvent := detector.DetectAndRecordWithWindow(
		"test-target", "unknown",
		5000.0, time.Now().UTC(),
		true, nil, nil, nil, domain.SampleWindow{}, nil,
	)

	if stateEvent != nil {
		t.Errorf("state expected nil event for unknown kind, got %v", stateEvent)
	}
}

// TestSpikeDecisionParity_RollingMedianConsistency tests that RollingMedianMs in events
// matches domain decision baseline.
func TestSpikeDecisionParity_RollingMedianConsistency(t *testing.T) {
	detector := NewSpikeDetector()
	window := makeDomainSampleWindow(30, 50.0, domain.ProbeKindHTTP)
	currentLatency := 6000.0

	domainDecision := domain.DecideSpike(
		makeDomainSample(currentLatency, domain.ProbeKindHTTP, true),
		window,
		domain.DefaultSpikeConfigHTTP(),
	)

	stateEvent := detector.DetectAndRecordWithWindow(
		"test-target", "http",
		currentLatency, time.Now().UTC(),
		true, nil, nil, nil, window, nil,
	)

	if stateEvent == nil {
		t.Fatal("state expected event")
	}
	if stateEvent.RollingMedianMs != domainDecision.Baseline.Float64() {
		t.Errorf("state RollingMedianMs = %v, domain baseline = %v",
			stateEvent.RollingMedianMs, domainDecision.Baseline.Float64())
	}
}

// TestSpikeDecisionParity_BothAbsoluteAndRelativeReasons tests that when both
// absolute and relative thresholds trigger, both reasons appear in the event.
func TestSpikeDecisionParity_BothAbsoluteAndRelativeReasons(t *testing.T) {
	detector := NewSpikeDetector()
	window := makeDomainSampleWindow(30, 50.0, domain.ProbeKindHTTP)
	// 6000ms: exceeds both critical (5000ms) AND relative (500ms)
	currentLatency := 6000.0

	domainDecision := domain.DecideSpike(
		makeDomainSample(currentLatency, domain.ProbeKindHTTP, true),
		window,
		domain.DefaultSpikeConfigHTTP(),
	)

	stateEvent := detector.DetectAndRecordWithWindow(
		"test-target", "http",
		currentLatency, time.Now().UTC(),
		true, nil, nil, nil, window, nil,
	)

	if stateEvent == nil {
		t.Fatal("state expected event")
	}

	// Domain should have both reasons
	if !strings.Contains(domainDecision.Reason, "critical_absolute") {
		t.Errorf("domain expected critical_absolute in reason, got %s", domainDecision.Reason)
	}
	if !strings.Contains(domainDecision.Reason, "relative_threshold") {
		t.Errorf("domain expected relative_threshold in reason, got %s", domainDecision.Reason)
	}

	// State should have both reasons (mapped)
	hasCritical := false
	hasRelative := false
	for _, r := range stateEvent.Reasons {
		if strings.Contains(r, "critical_absolute_threshold") {
			hasCritical = true
		}
		if strings.Contains(r, "relative_10x_median_threshold") {
			hasRelative = true
		}
	}
	if !hasCritical {
		t.Errorf("state expected critical_absolute_threshold in reasons, got %v", stateEvent.Reasons)
	}
	if !hasRelative {
		t.Errorf("state expected relative_10x_median_threshold in reasons, got %v", stateEvent.Reasons)
	}
}
