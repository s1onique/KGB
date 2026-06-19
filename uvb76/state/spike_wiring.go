package state

import "time"

// spikeWiring.go contains the state.Manager methods that delegate to SpikeDetector.
// This separation keeps state.go focused on latency tracking and snapshot management.

// GetSpikeDetector returns the spike detector for this manager.
func (m *Manager) GetSpikeDetector() *SpikeDetector {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.spikeDetector
}

// GetSpikes returns spike events for a target/kind combination.
func (m *Manager) GetSpikes(targetID, kind string, limit int) []SpikeEvent {
	return m.spikeDetector.GetSpikes(targetID, kind, limit)
}

// DetectAndRecordSpike checks a sample for spike conditions and records if detected.
func (m *Manager) DetectAndRecordSpike(
	targetID, kind string,
	latencyMs float64,
	sampleTs time.Time,
	reachable bool,
	schedulerDelayMs *float64,
	httpStatus *int,
	probeError *string,
	previousSamples []LatencySample,
) *SpikeEvent {
	return m.spikeDetector.DetectAndRecord(
		targetID, kind, latencyMs, sampleTs, reachable,
		schedulerDelayMs, httpStatus, probeError, previousSamples,
	)
}

// RecordRecoveryEvent explicitly records a recovery event when a target becomes
// reachable again after being unreachable.
func (m *Manager) RecordRecoveryEvent(
	targetID, kind string,
	latencyMs float64,
	sampleTs time.Time,
	httpStatus *int,
	previousSamples []LatencySample,
) *SpikeEvent {
	return m.spikeDetector.RecordRecoveryEvent(
		targetID, kind, latencyMs, sampleTs, httpStatus, previousSamples,
	)
}
