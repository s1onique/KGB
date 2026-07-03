package domain

import (
	"testing"
)

func TestCaptureStatus_Parse(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOK    bool
		wantValue CaptureStatus
	}{
		{
			name:      "captured",
			input:     "captured",
			wantOK:    true,
			wantValue: CaptureStatusCaptured,
		},
		{
			name:      "skipped_cooldown",
			input:     "skipped_cooldown",
			wantOK:    true,
			wantValue: CaptureStatusSkippedCooldown,
		},
		{
			name:      "failed",
			input:     "failed",
			wantOK:    true,
			wantValue: CaptureStatusFailed,
		},
		{
			name:      "disabled",
			input:     "disabled",
			wantOK:    true,
			wantValue: CaptureStatusDisabled,
		},
		{
			name:      "not_configured",
			input:     "not_configured",
			wantOK:    true,
			wantValue: CaptureStatusNotConfigured,
		},
		{
			name:      "not_attempted",
			input:     "not_attempted",
			wantOK:    true,
			wantValue: CaptureStatusNotAttempted,
		},
		{
			name:      "in_progress",
			input:     "in_progress",
			wantOK:    true,
			wantValue: CaptureStatusInProgress,
		},
		{
			name:      "none",
			input:     "none",
			wantOK:    true,
			wantValue: CaptureStatusNone,
		},
		{
			name:   "missing",
			input:  "missing",
			wantOK: true,
			wantValue: CaptureStatusMissing,
		},
		{
			name:   "unknown",
			input:  "unknown",
			wantOK: false,
		},
		{
			name:   "UNKNOWN",
			input:  "UNKNOWN",
			wantOK: false,
		},
		{
			name:   "CAPTURED",
			input:  "CAPTURED",
			wantOK: false,
		},
		{
			name:   "empty",
			input:  "",
			wantOK: false,
		},
		{
			name:   "random_string",
			input:  "random_string",
			wantOK: false,
		},
		{
			name:   "captured_with_typo",
			input:  "capturedd",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseCaptureStatus(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ParseCaptureStatus(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
				return
			}
			if ok && got != tt.wantValue {
				t.Errorf("ParseCaptureStatus(%q) = %v, want %v", tt.input, got, tt.wantValue)
			}
		})
	}
}

func TestCaptureStatus_RoundTrip(t *testing.T) {
	// All canonical statuses should round-trip through string conversion
	statuses := []CaptureStatus{
		CaptureStatusCaptured,
		CaptureStatusSkippedCooldown,
		CaptureStatusFailed,
		CaptureStatusDisabled,
		CaptureStatusNotConfigured,
		CaptureStatusNotAttempted,
		CaptureStatusInProgress,
		CaptureStatusNone,
		CaptureStatusMissing,
	}

	for _, s := range statuses {
		raw := string(s)
		parsed, ok := ParseCaptureStatus(raw)
		if !ok {
			t.Errorf("ParseCaptureStatus(%q) failed for canonical status %v", raw, s)
			continue
		}
		if parsed != s {
			t.Errorf("ParseCaptureStatus(%q) = %v, want %v", raw, parsed, s)
		}
	}
}

func TestCaptureStatus_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		s     CaptureStatus
		want  bool
	}{
		{"captured", CaptureStatusCaptured, true},
		{"skipped_cooldown", CaptureStatusSkippedCooldown, true},
		{"failed", CaptureStatusFailed, true},
		{"disabled", CaptureStatusDisabled, true},
		{"not_configured", CaptureStatusNotConfigured, true},
		{"not_attempted", CaptureStatusNotAttempted, true},
		{"in_progress", CaptureStatusInProgress, true},
		{"none", CaptureStatusNone, true},
		{"missing", CaptureStatusMissing, true},
		{"unknown", CaptureStatus("unknown"), false},
		{"empty", CaptureStatus(""), false},
		{"random", CaptureStatus("random"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.IsValid(); got != tt.want {
				t.Errorf("CaptureStatus(%q).IsValid() = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}
