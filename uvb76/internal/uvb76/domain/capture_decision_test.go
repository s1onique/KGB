package domain

import (
	"testing"
	"time"
)

func TestDecideCapture(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	makeConfig := func(enabled, configured bool, cooldown time.Duration) DiagnosticCaptureConfig {
		return DiagnosticCaptureConfig{
			Enabled:    enabled,
			Configured: configured,
			Cooldown:   cooldown,
		}
	}

	tests := []struct {
		name                   string
		now                    time.Time
		lastSuccessfulCapture  time.Time
		cfg                    DiagnosticCaptureConfig
		wantKind               CaptureDecisionKind
		description            string
	}{
		{
			name:                  "disabled config returns disabled",
			now:                   baseTime,
			lastSuccessfulCapture: time.Time{},
			cfg:                   makeConfig(false, true, 60*time.Second),
			wantKind:              CaptureDecisionDisabled,
			description:           "disabled capture returns disabled",
		},
		{
			name:                  "not configured returns not_configured",
			now:                   baseTime,
			lastSuccessfulCapture: time.Time{},
			cfg:                   makeConfig(true, false, 60*time.Second),
			wantKind:              CaptureDecisionNotConfigured,
			description:           "not configured returns not_configured",
		},
		{
			name:                  "zero last successful capture does not suppress cooldown",
			now:                   baseTime,
			lastSuccessfulCapture: time.Time{}, // zero time
			cfg:                   makeConfig(true, true, 60*time.Second),
			wantKind:              CaptureDecisionRun,
			description:           "zero lastSuccessfulCapture allows first capture",
		},
		{
			name:                  "cooldown active after successful capture returns skip_cooldown",
			now:                   baseTime,
			lastSuccessfulCapture: baseTime.Add(-30 * time.Second), // 30 seconds ago
			cfg:                   makeConfig(true, true, 60*time.Second), // 60 second cooldown
			wantKind:              CaptureDecisionSkipCooldown,
			description:           "within cooldown window returns skip_cooldown",
		},
		{
			name:                  "cooldown elapsed returns run",
			now:                   baseTime,
			lastSuccessfulCapture: baseTime.Add(-90 * time.Second), // 90 seconds ago
			cfg:                   makeConfig(true, true, 60*time.Second), // 60 second cooldown
			wantKind:              CaptureDecisionRun,
			description:           "after cooldown expires returns run",
		},
		{
			name:                  "zero cooldown does not suppress",
			now:                   baseTime,
			lastSuccessfulCapture: baseTime.Add(-1 * time.Second),
			cfg:                   makeConfig(true, true, 0),
			wantKind:              CaptureDecisionRun,
			description:           "zero cooldown means no suppression",
		},
		{
			name:                  "negative cooldown does not suppress",
			now:                   baseTime,
			lastSuccessfulCapture: baseTime.Add(-1 * time.Second),
			cfg:                   makeConfig(true, true, -1*time.Second),
			wantKind:              CaptureDecisionRun,
			description:           "negative cooldown means no suppression",
		},
		{
			name:                  "exactly at cooldown boundary returns run",
			now:                   baseTime,
			lastSuccessfulCapture: baseTime.Add(-60 * time.Second), // exactly at cooldown boundary
			cfg:                   makeConfig(true, true, 60*time.Second),
			wantKind:              CaptureDecisionRun,
			description:           "cooldown uses < not <=, so exactly at boundary allows capture",
		},
		{
			name:                  "just past cooldown boundary returns run",
			now:                   baseTime,
			lastSuccessfulCapture: baseTime.Add(-61 * time.Second), // just past cooldown
			cfg:                   makeConfig(true, true, 60*time.Second),
			wantKind:              CaptureDecisionRun,
			description:           "just past boundary allows capture",
		},
		{
			name:                  "disabled beats not_configured",
			now:                   baseTime,
			lastSuccessfulCapture: time.Time{},
			cfg:                   makeConfig(false, false, 60*time.Second),
			wantKind:              CaptureDecisionDisabled,
			description:           "disabled takes precedence over not_configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecideCapture(tt.now, tt.lastSuccessfulCapture, tt.cfg)
			if got.Kind != tt.wantKind {
				t.Errorf("DecideCapture() = %v, want %v (%s)", got.Kind, tt.wantKind, tt.description)
			}
		})
	}
}

func TestDecideCapture_CooldownInvariant(t *testing.T) {
	// Verify the critical invariant: cooldown only applies after prior successful capture
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Scenario: First ever capture should ALWAYS run, regardless of cooldown
	// This is the critical "zero lastSuccessfulCapture" invariant
	cfg := DiagnosticCaptureConfig{
		Enabled:    true,
		Configured: true,
		Cooldown:   60 * time.Second,
	}

	decision := DecideCapture(baseTime, time.Time{}, cfg)
	if decision.Kind != CaptureDecisionRun {
		t.Errorf("First capture should always run, got %v", decision.Kind)
	}
}
