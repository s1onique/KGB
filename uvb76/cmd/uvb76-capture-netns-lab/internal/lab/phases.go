package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// PhaseName represents the lab phase name.
type PhaseName string

const (
	PhaseBaseline  PhaseName = "baseline"
	PhaseDefect    PhaseName = "defect"
	PhaseRecovery  PhaseName = "recovery"
)

// PhaseResult captures the outcome of a phase execution.
type PhaseResult struct {
	Name          PhaseName  `json:"name"`
	Started       time.Time `json:"started"`
	Ended         time.Time `json:"ended"`
	SpikeEventID  string    `json:"spike_event_id,omitempty"`
	CaptureStatus string    `json:"capture_status,omitempty"`
	ArtifactPaths []string  `json:"artifact_paths,omitempty"`
	Err           string    `json:"error,omitempty"`
}

// Duration returns the phase duration.
func (r PhaseResult) Duration() time.Duration {
	return r.Ended.Sub(r.Started)
}

// PhaseConfig holds phase-specific configuration.
type PhaseConfig struct {
	Name            PhaseName
	SpikeTimeout    time.Duration
	CaptureTimeout  time.Duration
	CooldownSeconds int
}

// DefaultPhaseConfigs returns the standard phase configurations.
func DefaultPhaseConfigs() []PhaseConfig {
	return []PhaseConfig{
		{Name: PhaseBaseline, SpikeTimeout: 30 * time.Second, CaptureTimeout: 15 * time.Second, CooldownSeconds: 5},
		{Name: PhaseDefect, SpikeTimeout: 15 * time.Second, CaptureTimeout: 15 * time.Second, CooldownSeconds: 5},
		{Name: PhaseRecovery, SpikeTimeout: 30 * time.Second, CaptureTimeout: 15 * time.Second, CooldownSeconds: 5},
	}
}

// PhaseTracker tracks phase cursor for event isolation.
type PhaseTracker struct {
	cursors map[PhaseName]string
}

// NewPhaseTracker creates a new phase tracker.
func NewPhaseTracker() *PhaseTracker {
	return &PhaseTracker{
		cursors: make(map[PhaseName]string),
	}
}

// SetCursor records the cursor timestamp for a phase.
func (t *PhaseTracker) SetCursor(phase PhaseName) {
	t.cursors[phase] = time.Now().UTC().Format(time.RFC3339)
}

// GetCursor returns the cursor timestamp for a phase.
func (t *PhaseTracker) GetCursor(phase PhaseName) string {
	return t.cursors[phase]
}

// PhaseContext holds runtime context for phase execution.
type PhaseContext struct {
	Phase       PhaseConfig
	Tracker     *PhaseTracker
	ArtifactDir string
	Runner      CommandRunner
	Netns       *NetnsHelper
}

// PollFn is a polling function that returns (done, error).
type PollFn func(ctx context.Context) (done bool, err error)

// Poll executes a polling function with deadline and interval.
func Poll(ctx context.Context, interval time.Duration, fn PollFn) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			done, err := fn(ctx)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		}
	}
}

// PollWithTimeout polls until done or timeout, with explicit interval.
func PollWithTimeout(ctx context.Context, timeout, interval time.Duration, fn PollFn) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return Poll(ctx, interval, fn)
}

// PhaseOutcome represents the expected outcome for a phase.
type PhaseOutcome struct {
	ExpectedCaptureStatus string
	RequireCooldownInfo   bool
	RequirePacket        bool
}

// CaptureStatus constants matching production code.
const (
	CaptureStatusCaptured      = "captured"
	CaptureStatusSkippedCooldown = "skipped_cooldown"
	CaptureStatusNotAttempted  = "not_attempted"
	CaptureStatusFailed        = "failed"
)

// DefaultPhaseOutcomes returns the standard phase expectations.
func DefaultPhaseOutcomes() map[PhaseName]PhaseOutcome {
	return map[PhaseName]PhaseOutcome{
		PhaseBaseline:  {ExpectedCaptureStatus: CaptureStatusCaptured, RequireCooldownInfo: false, RequirePacket: true},
		PhaseDefect:    {ExpectedCaptureStatus: CaptureStatusSkippedCooldown, RequireCooldownInfo: true, RequirePacket: false},
		PhaseRecovery:  {ExpectedCaptureStatus: CaptureStatusCaptured, RequireCooldownInfo: false, RequirePacket: true},
	}
}

// ValidateOutcome checks if a phase result meets expectations.
func ValidateOutcome(result *PhaseResult, outcome PhaseOutcome) error {
	if result.CaptureStatus != outcome.ExpectedCaptureStatus {
		return fmt.Errorf("phase %s: expected capture_status=%s, got %s", result.Name, outcome.ExpectedCaptureStatus, result.CaptureStatus)
	}
	return nil
}

// MarshalJSON implements json.Marshaler for PhaseResult.
func (r PhaseResult) MarshalJSON() ([]byte, error) {
	type Alias PhaseResult
	return json.Marshal(&struct {
		Duration string `json:"duration"`
		Alias
	}{
		Duration: r.Duration().String(),
		Alias:    Alias(r),
	})
}

