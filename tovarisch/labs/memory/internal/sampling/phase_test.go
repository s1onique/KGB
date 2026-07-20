// phase_test.go — Unit tests for sampling package
//
// Tests for phase configuration and state machine.

package sampling

import (
	"testing"
	"time"
)

// TestPhaseConfigDuration tests phase duration calculations.
func TestPhaseConfigDuration(t *testing.T) {
	cfg := DefaultPhaseConfig()

	total := cfg.TotalDuration()
	expected := 120*time.Second + 120*time.Second + 180*time.Second +
		600*time.Second + 120*time.Second + 180*time.Second

	if total != expected {
		t.Errorf("TotalDuration: got %v, want %v", total, expected)
	}
}

// TestPhaseConfigDurationFor tests durationFor method.
func TestPhaseConfigDurationFor(t *testing.T) {
	cfg := DefaultPhaseConfig()

	tests := []struct {
		phase Phase
		want  time.Duration
	}{
		{PhaseStartup, 120 * time.Second},
		{PhaseWarmup, 120 * time.Second},
		{PhaseBaseline, 180 * time.Second},
		{PhaseStimulus, 600 * time.Second},
		{PhaseSettling, 120 * time.Second},
		{PhaseFinal, 180 * time.Second},
	}

	for _, tt := range tests {
		got := cfg.DurationFor(tt.phase)
		if got != tt.want {
			t.Errorf("DurationFor(%v): got %v, want %v", tt.phase, got, tt.want)
		}
	}
}

// TestPhaseStateAdvance tests phase advancement.
func TestPhaseStateAdvance(t *testing.T) {
	cfg := PhaseConfig{
		StartupDeadline: 1 * time.Millisecond,
		Warmup:          1 * time.Millisecond,
		Baseline:        1 * time.Millisecond,
		Stimulus:        1 * time.Millisecond,
		Settling:        1 * time.Millisecond,
		Final:           1 * time.Millisecond,
	}

	state := NewPhaseState(cfg)

	// Initial phase should be Startup
	if state.Current != PhaseStartup {
		t.Errorf("Initial phase: got %v, want Startup", state.Current)
	}

	// Wait and advance
	time.Sleep(2 * time.Millisecond)
	advanced := state.Advance()
	if !advanced {
		t.Error("Expected advance from Startup to Warmup")
	}
	if state.Current != PhaseWarmup {
		t.Errorf("After advance: got %v, want Warmup", state.Current)
	}

	// Continue advancing through all phases
	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Millisecond)
		state.Advance()
	}

	if !state.IsFinal() {
		t.Error("Should be in Final phase")
	}
}

// TestPhaseStateRemaining tests remaining time calculation.
func TestPhaseStateRemaining(t *testing.T) {
	cfg := PhaseConfig{
		StartupDeadline: 100 * time.Millisecond,
		Warmup:          100 * time.Millisecond,
	}

	state := NewPhaseState(cfg)

	// Should have time remaining initially
	remaining := state.Remaining()
	if remaining <= 0 {
		t.Error("Expected positive remaining time initially")
	}

	// Wait past the phase
	time.Sleep(150 * time.Millisecond)
	remaining = state.Remaining()
	if remaining != 0 {
		t.Errorf("After timeout: got %v, want 0", remaining)
	}
}

// TestSmokePhaseConfig tests smoke variant durations.
func TestSmokePhaseConfig(t *testing.T) {
	smokeCfg := SmokePhaseConfig()
	defaultCfg := DefaultPhaseConfig()

	// Smoke should be much shorter than default
	smokeTotal := smokeCfg.TotalDuration()
	defaultTotal := defaultCfg.TotalDuration()

	if smokeTotal >= defaultTotal {
		t.Errorf("Smoke total duration should be shorter than default: smoke=%v default=%v", smokeTotal, defaultTotal)
	}
}

// TestAllPhases tests phase ordering.
func TestAllPhases(t *testing.T) {
	phases := AllPhases()

	expected := []Phase{
		PhaseStartup, PhaseWarmup, PhaseBaseline,
		PhaseStimulus, PhaseSettling, PhaseFinal,
	}

	if len(phases) != len(expected) {
		t.Errorf("Phase count: got %d, want %d", len(phases), len(expected))
	}

	for i, phase := range phases {
		if phase != expected[i] {
			t.Errorf("Phase %d: got %v, want %v", i, phase, expected[i])
		}
	}
}

// TestCSVHeaders tests CSV header generation.
func TestCSVHeaders(t *testing.T) {
	headers := CSVHeaders()

	// Should have expected columns
	expectedColumns := []string{
		"sequence", "timestamp", "process_pid", "process_start_time",
		"phase", "delayed", "rss_kib", "pss_kib", "pss_anon_kib",
		"private_dirty_kib", "anonymous_kib", "swap_kib",
		"vma_count", "fd_count", "socket_fd_count", "thread_count",
	}

	for _, col := range expectedColumns {
		found := false
		for _, h := range headers {
			if h == col {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected column %q not found in headers", col)
		}
	}
}
