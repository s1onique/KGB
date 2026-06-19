package state

import (
	"testing"
	"time"
)

// TestCaptureStore_GetCaptureInfo_NoCaptures tests that no captures returns none status.
func TestCaptureStore_GetCaptureInfo_NoCaptures(t *testing.T) {
	store := NewCaptureStore()

	info := store.GetCaptureInfo("nonexistent-event", false)
	if info.CaptureStatus != CaptureStatusNone {
		t.Errorf("expected CaptureStatusNone, got %s", info.CaptureStatus)
	}
	if info.CaptureExists {
		t.Error("expected CaptureExists=false")
	}
	if info.IsProtected {
		t.Error("expected IsProtected=false")
	}
}

// TestCaptureStore_GetCaptureInfo_OKWithArtifact tests that ok status with artifact is protected.
func TestCaptureStore_GetCaptureInfo_OKWithArtifact(t *testing.T) {
	store := NewCaptureStore()

	// Add a successful capture with network_diag
	store.AddCapture("event-1", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusOK,
		NetworkDiag:      &NetworkDiagData{Status: "ok"},
	})

	info := store.GetCaptureInfo("event-1", false)
	if info.CaptureStatus != CaptureStatusCaptured {
		t.Errorf("expected CaptureStatusCaptured, got %s", info.CaptureStatus)
	}
	if !info.CaptureExists {
		t.Error("expected CaptureExists=true")
	}
	if !info.IsProtected {
		t.Error("expected IsProtected=true")
	}
}

// TestCaptureStore_GetCaptureInfo_OKWithoutArtifact tests that ok status without artifact is still protected.
func TestCaptureStore_GetCaptureInfo_OKWithoutArtifact(t *testing.T) {
	store := NewCaptureStore()

	// Add a successful capture without network_diag
	store.AddCapture("event-1", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusOK,
		NetworkDiag:      nil,
	})

	info := store.GetCaptureInfo("event-1", false)
	if info.CaptureStatus != CaptureStatusCaptured {
		t.Errorf("expected CaptureStatusCaptured, got %s", info.CaptureStatus)
	}
	if info.CaptureExists {
		t.Error("expected CaptureExists=false")
	}
	// Still protected because capture was attempted
	if !info.IsProtected {
		t.Error("expected IsProtected=true for ok status")
	}
}

// TestCaptureStore_GetCaptureInfo_TimeoutWithArtifact tests that timeout with artifact is protected.
func TestCaptureStore_GetCaptureInfo_TimeoutWithArtifact(t *testing.T) {
	store := NewCaptureStore()

	store.AddCapture("event-1", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusTimeout,
		NetworkDiag:      &NetworkDiagData{Status: "ok"},
	})

	info := store.GetCaptureInfo("event-1", false)
	if info.CaptureStatus != CaptureStatusFailed {
		t.Errorf("expected CaptureStatusFailed, got %s", info.CaptureStatus)
	}
	if !info.CaptureExists {
		t.Error("expected CaptureExists=true")
	}
	if !info.IsProtected {
		t.Error("expected IsProtected=true")
	}
}

// TestCaptureStore_GetCaptureInfo_TimeoutWithoutArtifact tests that timeout without artifact is purge-eligible.
func TestCaptureStore_GetCaptureInfo_TimeoutWithoutArtifact(t *testing.T) {
	store := NewCaptureStore()

	store.AddCapture("event-1", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusTimeout,
		NetworkDiag:      nil,
	})

	info := store.GetCaptureInfo("event-1", false)
	if info.CaptureStatus != CaptureStatusFailed {
		t.Errorf("expected CaptureStatusFailed, got %s", info.CaptureStatus)
	}
	if info.CaptureExists {
		t.Error("expected CaptureExists=false")
	}
	if info.IsProtected {
		t.Error("expected IsProtected=false for timeout without artifact")
	}
}

// TestCaptureStore_GetCaptureInfo_ErrorWithArtifact tests that error with artifact is protected.
func TestCaptureStore_GetCaptureInfo_ErrorWithArtifact(t *testing.T) {
	store := NewCaptureStore()

	store.AddCapture("event-1", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusError,
		NetworkDiag:      &NetworkDiagData{Status: "ok"},
	})

	info := store.GetCaptureInfo("event-1", false)
	if info.CaptureStatus != CaptureStatusFailed {
		t.Errorf("expected CaptureStatusFailed, got %s", info.CaptureStatus)
	}
	if !info.CaptureExists {
		t.Error("expected CaptureExists=true")
	}
	if !info.IsProtected {
		t.Error("expected IsProtected=true")
	}
}

// TestCaptureStore_GetCaptureInfo_ErrorWithoutArtifact tests that error without artifact is purge-eligible.
func TestCaptureStore_GetCaptureInfo_ErrorWithoutArtifact(t *testing.T) {
	store := NewCaptureStore()

	store.AddCapture("event-1", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusError,
		NetworkDiag:      nil,
	})

	info := store.GetCaptureInfo("event-1", false)
	if info.CaptureStatus != CaptureStatusFailed {
		t.Errorf("expected CaptureStatusFailed, got %s", info.CaptureStatus)
	}
	if info.CaptureExists {
		t.Error("expected CaptureExists=false")
	}
	if info.IsProtected {
		t.Error("expected IsProtected=false for error without artifact")
	}
}

// TestCaptureStore_GetCaptureInfo_SuppressedCooldown tests that suppressed captures are purge-eligible.
func TestCaptureStore_GetCaptureInfo_SuppressedCooldown(t *testing.T) {
	store := NewCaptureStore()

	store.AddCapture("event-1", DiagCapture{
		Source:               "peer-1",
		CaptureStartedAt:     time.Now().UTC(),
		Status:               DiagCaptureStatusOK,
		SuppressedByCooldown: true,
		NetworkDiag:         &NetworkDiagData{Status: "ok"},
	})

	info := store.GetCaptureInfo("event-1", false)
	if info.CaptureStatus != CaptureStatusSkippedCooldown {
		t.Errorf("expected CaptureStatusSkippedCooldown, got %s", info.CaptureStatus)
	}
	if info.IsProtected {
		t.Error("expected IsProtected=false for suppressed capture")
	}
}

// TestCaptureStore_GetCaptureInfo_InFlight tests that in-flight captures are protected.
func TestCaptureStore_GetCaptureInfo_InFlight(t *testing.T) {
	store := NewCaptureStore()

	// No captures, but in-flight
	info := store.GetCaptureInfo("event-1", true)
	if info.CaptureStatus != CaptureStatusInProgress {
		t.Errorf("expected CaptureStatusInProgress, got %s", info.CaptureStatus)
	}
	if !info.IsProtected {
		t.Error("expected IsProtected=true for in-flight capture")
	}
}

// TestCaptureStore_GetCaptureInfo_Disabled tests that disabled captures are not protected.
func TestCaptureStore_GetCaptureInfo_Disabled(t *testing.T) {
	store := NewCaptureStore()

	store.AddCapture("event-1", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusDisabled,
	})

	info := store.GetCaptureInfo("event-1", false)
	if info.CaptureStatus != CaptureStatusDisabled {
		t.Errorf("expected CaptureStatusDisabled for disabled, got %s", info.CaptureStatus)
	}
	if info.IsProtected {
		t.Error("expected IsProtected=false for disabled capture")
	}
}

// TestCaptureStore_GetCaptureInfo_NoPeerMapping tests that no_peer_mapping captures are not protected.
func TestCaptureStore_GetCaptureInfo_NoPeerMapping(t *testing.T) {
	store := NewCaptureStore()

	store.AddCapture("event-1", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusNoPeerMapping,
	})

	info := store.GetCaptureInfo("event-1", false)
	if info.CaptureStatus != CaptureStatusNotConfigured {
		t.Errorf("expected CaptureStatusNotConfigured for no_peer_mapping, got %s", info.CaptureStatus)
	}
	if info.IsProtected {
		t.Error("expected IsProtected=false for no_peer_mapping capture")
	}
}

// TestSpikeRetentionStats_Calculation tests the retention stats calculation logic.
func TestSpikeRetentionStats_Calculation(t *testing.T) {
	store := NewCaptureStore()

	// Event 1: protected (ok with artifact)
	store.AddCapture("evt-1", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusOK,
		NetworkDiag:      &NetworkDiagData{Status: "ok"},
	})

	// Event 2: protected (timeout with artifact)
	store.AddCapture("evt-2", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusTimeout,
		NetworkDiag:      &NetworkDiagData{Status: "ok"},
	})

	// Event 3: purge-eligible (suppressed)
	store.AddCapture("evt-3", DiagCapture{
		Source:               "peer-1",
		CaptureStartedAt:     time.Now().UTC(),
		Status:               DiagCaptureStatusOK,
		SuppressedByCooldown: true,
	})

	// Event 4: purge-eligible (timeout without artifact)
	store.AddCapture("evt-4", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusTimeout,
		NetworkDiag:      nil,
	})

	// Event 5: no captures (purge-eligible)
	// ...

	tests := []struct {
		eventID             string
		isInFlight          bool
		expectedProtected   bool
	}{
		{"evt-1", false, true},  // ok with artifact
		{"evt-2", false, true},  // timeout with artifact
		{"evt-3", false, false}, // suppressed
		{"evt-4", false, false}, // timeout without artifact
		{"evt-5", false, false}, // no capture
		{"evt-1", true, true},   // in-flight overrides
	}

	for _, tt := range tests {
		info := store.GetCaptureInfo(tt.eventID, tt.isInFlight)
		if info.IsProtected != tt.expectedProtected {
			t.Errorf("event %s (inFlight=%v): expected IsProtected=%v, got %v",
				tt.eventID, tt.isInFlight, tt.expectedProtected, info.IsProtected)
		}
	}
}

// TestCaptureStore_MultipleCaptures tests that most recent capture is used.
func TestCaptureStore_MultipleCaptures(t *testing.T) {
	store := NewCaptureStore()

	// Add multiple captures
	store.AddCapture("event-1", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC().Add(-2 * time.Hour),
		Status:           DiagCaptureStatusError,
	})

	store.AddCapture("event-1", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC().Add(-1 * time.Hour),
		Status:           DiagCaptureStatusOK,
		NetworkDiag:      &NetworkDiagData{Status: "ok"},
	})

	// Most recent should be used
	info := store.GetCaptureInfo("event-1", false)
	if info.CaptureStatus != CaptureStatusCaptured {
		t.Errorf("expected CaptureStatusCaptured (most recent), got %s", info.CaptureStatus)
	}
}

// TestGetProtectionInfo_Basic tests the GetProtectionInfo method.
func TestGetProtectionInfo_Basic(t *testing.T) {
	store := NewCaptureStore()

	// Test no captures
	isProtected, hasCapture := store.GetProtectionInfo("evt-none")
	if isProtected || hasCapture {
		t.Error("expected unprotected, no capture for nonexistent event")
	}

	// Add a protected capture
	store.AddCapture("evt-protected", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusOK,
		NetworkDiag:      &NetworkDiagData{Status: "ok"},
	})

	isProtected, hasCapture = store.GetProtectionInfo("evt-protected")
	if !isProtected || !hasCapture {
		t.Error("expected protected with capture for ok with artifact")
	}

	// Add a purgeable capture
	store.AddCapture("evt-purgeable", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusTimeout,
		NetworkDiag:      nil,
	})

	isProtected, hasCapture = store.GetProtectionInfo("evt-purgeable")
	if isProtected || hasCapture {
		t.Error("expected purgeable, no capture for timeout without artifact")
	}
}

// TestGetProtectionInfo_OKWithoutArtifact is protected.
func TestGetProtectionInfo_OKWithoutArtifact(t *testing.T) {
	store := NewCaptureStore()

	store.AddCapture("evt-ok-no-artifact", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusOK,
		NetworkDiag:      nil,
	})

	// ok without artifact IS protected (capture was attempted)
	isProtected, hasCapture := store.GetProtectionInfo("evt-ok-no-artifact")
	if !isProtected {
		t.Error("expected protected for ok status without artifact")
	}
	if hasCapture {
		t.Error("expected no capture artifact for ok without NetworkDiag")
	}
}

// TestGetProtectionInfo_Suppressed is purgeable.
func TestGetProtectionInfo_Suppressed(t *testing.T) {
	store := NewCaptureStore()

	store.AddCapture("evt-suppressed", DiagCapture{
		Source:               "peer-1",
		CaptureStartedAt:     time.Now().UTC(),
		Status:               DiagCaptureStatusOK,
		SuppressedByCooldown: true,
		NetworkDiag:         &NetworkDiagData{Status: "ok"},
	})

	isProtected, hasCapture := store.GetProtectionInfo("evt-suppressed")
	// Suppressed captures are NOT protected (purgeable)
	if isProtected {
		t.Error("expected not protected for suppressed capture")
	}
	// hasCapture=false for GetProtectionInfo because it checks SuppressedByCooldown first
	// and returns without checking NetworkDiag
	if hasCapture {
		t.Error("expected hasCapture=false for suppressed (SuppressedByCooldown checked first)")
	}
}
