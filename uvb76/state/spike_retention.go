package state

// DefaultSpikeUncapturedCap is the default cap for uncaptured spikes.
// This matches config.DefaultMaxUncapturedSpikes.
const DefaultSpikeUncapturedCap = 200

// NewManagerWithDiagnosticsConfig creates a new state manager with diagnostics config wired up.
// This is the production constructor that should be used when diagnostics are configured.
// It wires DiagnosticsConfig.MaxUncapturedSpikes to SpikeConfig.MaxEventsPerTracker
// and enables capture-aware spike retention.
func NewManagerWithDiagnosticsConfig(maxUncapturedSpikes int) *Manager {
	if maxUncapturedSpikes <= 0 {
		maxUncapturedSpikes = DefaultSpikeUncapturedCap
	}
	m := NewManager()
	// Wire MaxUncapturedSpikes from diagnostics config to spike detector
	spikeConfig := DefaultSpikeConfig()
	spikeConfig.MaxEventsPerTracker = maxUncapturedSpikes
	m.spikeDetector = NewSpikeDetectorWithConfig(spikeConfig)
	// Enable capture-aware retention
	m.EnableCaptureAwareSpikeRetention()
	return m
}

// EnableCaptureAwareSpikeRetentionWithCap wires up the spike detector to use capture-aware eviction
// with a custom cap for uncaptured spikes. This is the production method to call when diagnostics
// is configured, as it wires the DiagnosticsConfig.MaxUncapturedSpikes to SpikeConfig.MaxEventsPerTracker.
//
// WARNING: This method replaces the spike detector and must only be called during startup
// before probes run. Do not use for dynamic runtime reconfiguration without preserving
// existing spike event history, as replacement will discard any spikes recorded before
// the call.
func (m *Manager) EnableCaptureAwareSpikeRetentionWithCap(maxUncapturedSpikes int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Wire protection info function
	m.spikeDetector.SetCaptureInfoFunc(m.captureStore.GetProtectionInfo)

	// Update spike config cap to match diagnostics config
	if maxUncapturedSpikes > 0 {
		spikeConfig := DefaultSpikeConfig()
		spikeConfig.MaxEventsPerTracker = maxUncapturedSpikes
		m.spikeDetector = NewSpikeDetectorWithConfig(spikeConfig)
		// Re-wire after reconfiguring
		m.spikeDetector.SetCaptureInfoFunc(m.captureStore.GetProtectionInfo)
	}
}
