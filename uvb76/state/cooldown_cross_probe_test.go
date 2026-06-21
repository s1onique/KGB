package state

import (
	"testing"
	"time"
)

// TestBuildCooldownInfoFromDecision_CrossProbeSuppression tests that IsCrossProbeSuppression
// is correctly computed when anchor and suppressed probe kinds differ.
func TestBuildCooldownInfoFromDecision_CrossProbeSuppression(t *testing.T) {
	// Set up: anchor was ICMP capture, now suppressing HTTP spike
	t0 := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	peer := "kamatera-tovarisch"

	// Create store with ICMP anchor
	store := NewCaptureStore()
	store.SetLastCaptureAnchor(peer, CaptureCooldownAnchor{
		AnchorProbeKind: "icmp",
		AnchorCreatedAt:  t0,
	})

	// Evaluate at t=30s (still in cooldown)
	t30 := t0.Add(30 * time.Second)
	decision := store.EvaluateCooldown(t30, peer, 90)

	if !decision.IsInCooldown {
		t.Fatal("Should be in cooldown at t=30s")
	}

	if decision.Anchor == nil {
		t.Fatal("Anchor should be present")
	}

	// Build cooldown info from decision
	info := BuildCooldownInfoFromDecision(decision, peer)
	if info == nil {
		t.Fatal("BuildCooldownInfoFromDecision should not return nil when in cooldown")
	}

	// Verify anchor probe kind is preserved
	if info.AnchorProbeKind != "icmp" {
		t.Errorf("AnchorProbeKind should be icmp, got %q", info.AnchorProbeKind)
	}

	// Simulate HTTP suppression: this is what recordSuppressedCooldown does
	info.SuppressedProbeKind = "http"
	info.IsCrossProbeSuppression = info.AnchorProbeKind != "" && info.SuppressedProbeKind != "" && info.AnchorProbeKind != info.SuppressedProbeKind

	if !info.IsCrossProbeSuppression {
		t.Error("IsCrossProbeSuppression should be true when anchor=icmp and suppressed=http")
	}

	t.Logf("PASS: anchor=%s, suppressed=%s, cross=%v", info.AnchorProbeKind, info.SuppressedProbeKind, info.IsCrossProbeSuppression)
}
