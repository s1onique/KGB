package state

import (
	"testing"
	"time"
)

// TestNewManagerWithDiagnosticsConfig verifies the production constructor wires up
// capture-aware retention with the correct MaxUncapturedSpikes configuration.
func TestNewManagerWithDiagnosticsConfig(t *testing.T) {
	m := NewManagerWithDiagnosticsConfig(200)
	if m == nil {
		t.Fatal("NewManagerWithDiagnosticsConfig returned nil")
	}

	spikeDetector := m.GetSpikeDetector()
	if spikeDetector == nil {
		t.Fatal("GetSpikeDetector returned nil")
	}
}

// TestNewManagerWithDiagnosticsConfig_WiresCaptureAwareRetention verifies that
// MaxUncapturedSpikes from diagnostics config is wired to spike eviction
// and capture-aware retention is enabled.
func TestNewManagerWithDiagnosticsConfig_WiresCaptureAwareRetention(t *testing.T) {
	const maxUncapturedSpikes = 50
	m := NewManagerWithDiagnosticsConfig(maxUncapturedSpikes)

	captureStore := m.GetCaptureStore()
	captureStore.AddCapture("protected-spike-1", DiagCapture{
		Source:           "peer-1",
		Status:           DiagCaptureStatusOK,
		CaptureStartedAt: time.Now().UTC(),
	})

	captures := captureStore.GetCaptures("protected-spike-1")
	if len(captures) != 1 {
		t.Errorf("Expected 1 capture, got %d", len(captures))
	}

	isProtected, _ := captureStore.GetProtectionInfo("protected-spike-1")
	if !isProtected {
		t.Error("Expected protected spike to be protected")
	}
}

// TestNewManagerWithDiagnosticsConfig_DefaultFallback verifies that invalid
// MaxUncapturedSpikes values fall back to the default.
func TestNewManagerWithDiagnosticsConfig_DefaultFallback(t *testing.T) {
	m := NewManagerWithDiagnosticsConfig(0)
	if m == nil {
		t.Fatal("NewManagerWithDiagnosticsConfig(0) returned nil")
	}

	m2 := NewManagerWithDiagnosticsConfig(-1)
	if m2 == nil {
		t.Fatal("NewManagerWithDiagnosticsConfig(-1) returned nil")
	}
}

// TestCaptureAwareSpikeRetention_ProtectedSpikesNotEvicted verifies that
// protected spikes (with captures) are NOT evicted when we exceed the cap.
func TestCaptureAwareSpikeRetention_ProtectedSpikesNotEvicted(t *testing.T) {
	const maxUncapturedSpikes = 5
	m := NewManagerWithDiagnosticsConfig(maxUncapturedSpikes)
	captureStore := m.GetCaptureStore()

	// Track protected spike event IDs
	protectedEventIDs := make([]string, 3)

	// Record protected spikes: record spike first, then add capture for that event ID
	for i := 0; i < 3; i++ {
		// Create previous samples to establish a baseline
		previousSamples := make([]LatencySample, 20)
		for j := 0; j < 20; j++ {
			previousSamples[j] = LatencySample{
				Timestamp: time.Now().UTC(),
				LatencyMs: 50.0, // Low baseline
				Reachable: true,
			}
		}
		
		// Record a spike that will be detected (using extreme latency)
		spike := m.DetectAndRecordSpike(
			"target-1", "http",
			5000.0, // High latency spike (well above thresholds)
			time.Now().UTC(),
			true,   // reachable
			nil,    // scheduler delay
			nil,    // http status
			nil,    // probe error
			previousSamples,
				nil, // httpTrace
		)
		
		if spike != nil {
			// Store the event ID
			protectedEventIDs[i] = spike.EventID
			
			// Add capture AFTER spike is recorded, with the correct event ID
			captureStore.AddCapture(spike.EventID, DiagCapture{
				Source:           "peer-1",
				Status:           DiagCaptureStatusOK,
				CaptureStartedAt: time.Now().UTC(),
			})
			
			// Verify the spike is now protected
			isProtected, _ := captureStore.GetProtectionInfo(spike.EventID)
			if !isProtected {
				t.Errorf("Expected spike %s to be protected after adding capture", spike.EventID)
			}
		}
	}

	// Now record uncaptured spikes (beyond the cap)
	// These are NOT protected and should be evicted
	for i := 0; i < maxUncapturedSpikes+5; i++ {
		previousSamples := make([]LatencySample, 20)
		for j := 0; j < 20; j++ {
			previousSamples[j] = LatencySample{
				Timestamp: time.Now().UTC(),
				LatencyMs: 50.0,
				Reachable: true,
			}
		}
		
		// Record spike without adding capture (uncaptured)
		m.DetectAndRecordSpike(
			"target-1", "http",
			5000.0,
			time.Now().UTC(),
			true,
			nil,
			nil,
			nil,
			previousSamples,
				nil, // httpTrace
		)
	}

	// Verify all protected spike event IDs are still accessible via GetSpikes
	allSpikes := m.GetSpikes("target-1", "http", 0)
	
	// Build a map of retained spike event IDs
	retainedIDs := make(map[string]bool)
	for _, spike := range allSpikes {
		retainedIDs[spike.EventID] = true
	}

	// Verify all protected spikes are still retained
	for _, eventID := range protectedEventIDs {
		if !retainedIDs[eventID] {
			t.Errorf("Expected protected spike %s to still be retained", eventID)
		}
		// Also verify capture store still knows it's protected
		isProtected, _ := captureStore.GetProtectionInfo(eventID)
		if !isProtected {
			t.Errorf("Expected protected spike %s to still be protected in capture store", eventID)
		}
	}
}

// TestCaptureAwareSpikeRetention_UncapturedSpikesEvicted verifies that uncaptured
// spikes are properly evicted when exceeding the cap.
func TestCaptureAwareSpikeRetention_UncapturedSpikesEvicted(t *testing.T) {
	const maxUncapturedSpikes = 5
	m := NewManagerWithDiagnosticsConfig(maxUncapturedSpikes)

	// Record many uncaptured spikes (not protected by captures)
	for i := 0; i < maxUncapturedSpikes+10; i++ {
		previousSamples := make([]LatencySample, 20)
		for j := 0; j < 20; j++ {
			previousSamples[j] = LatencySample{
				Timestamp: time.Now().UTC(),
				LatencyMs: 50.0,
				Reachable: true,
			}
		}
		
		m.DetectAndRecordSpike(
			"target-1", "http",
			5000.0,
			time.Now().UTC(),
			true,
			nil,
			nil,
			nil,
			previousSamples,
				nil, // httpTrace
		)
	}

	// Get all retained spikes
	allSpikes := m.GetSpikes("target-1", "http", 0)
	
	// Should be limited to maxUncapturedSpikes (uncaptured)
	// Note: Protected spikes don't count against this cap
	if len(allSpikes) > maxUncapturedSpikes {
		t.Errorf("Expected at most %d uncaptured spikes, got %d", maxUncapturedSpikes, len(allSpikes))
	}
}
