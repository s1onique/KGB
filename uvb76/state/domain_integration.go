// Package state provides bounded in-memory state management for UVB-76.
package state

import (
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// GetSampleWindow returns an immutable analysis snapshot of the recent successful samples.
// This exposes the pure domain.SampleWindow API without returning mutable ring-buffer storage.
//
// Thread-safe: holds read lock, consistent with GetRecentSamples.
func (lt *LatencyTracker) GetSampleWindow(limit int) domain.SampleWindow {
	samples := lt.GetRecentSamples(limit)
	if len(samples) == 0 {
		return domain.SampleWindow{}
	}

	// Convert state.LatencySample to domain.Sample
	domainSamples := make([]domain.Sample, 0, len(samples))
	for _, s := range samples {
		domainSamples = append(domainSamples, LatencySampleToDomainSample(s))
	}

	return domain.NewSampleWindow(domainSamples)
}

// LatencySampleToDomainSample converts a state.LatencySample to a domain.Sample.
// Failed samples (Reachable=false) are marked OK=false with the error in Err.
func LatencySampleToDomainSample(s LatencySample) domain.Sample {
	if !s.Reachable {
		return domain.Sample{
			At:  s.Timestamp,
			OK:  false,
			Err: s.Error,
		}
	}

	latency, ok := domain.NewLatencyMillis(s.LatencyMs)
	if !ok {
		// Invalid latency (NaN/Inf) - treat as failed sample
		return domain.Sample{
			At:  s.Timestamp,
			OK:  false,
			Err: "invalid latency value",
		}
	}

	return domain.Sample{
		At:      s.Timestamp,
		Kind:    domain.ProbeKindHTTP, // Default; caller can override if needed
		Latency: latency,
		OK:      true,
	}
}

// LatencySampleToDomainSampleWithKind converts a state.LatencySample to a domain.Sample
// with a specific probe kind (http or icmp).
func LatencySampleToDomainSampleWithKind(s LatencySample, kind domain.ProbeKind) domain.Sample {
	sample := LatencySampleToDomainSample(s)
	sample.Kind = kind
	return sample
}

// GetICMPSampleWindow returns an immutable analysis snapshot of recent ICMP samples for a target.
func (m *Manager) GetICMPSampleWindow(targetID string, limit int) domain.SampleWindow {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tracker := m.icmpTrackers[targetID]
	if tracker == nil {
		return domain.SampleWindow{}
	}

	return tracker.GetSampleWindow(limit)
}

// GetHTTPSampleWindow returns an immutable analysis snapshot of recent HTTP samples for a target.
func (m *Manager) GetHTTPSampleWindow(targetID string, limit int) domain.SampleWindow {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tracker := m.httpTrackers[targetID]
	if tracker == nil {
		return domain.SampleWindow{}
	}

	return tracker.GetSampleWindow(limit)
}

// NewDiagnosticCaptureConfigFromDefaults creates a DiagnosticCaptureConfig
// from existing spike detector defaults. This enables gradual migration to pure domain functions.
func NewDiagnosticCaptureConfigFromDefaults(enabled, configured bool, minInterval time.Duration) domain.DiagnosticCaptureConfig {
	return domain.DiagnosticCaptureConfig{
		Enabled:    enabled,
		Configured: configured,
		Cooldown:   minInterval,
	}
}
