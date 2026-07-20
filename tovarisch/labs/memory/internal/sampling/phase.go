// phase.go — Phase configuration and state machine
//
// Defines the memory lab phase lifecycle:
// startup → warmup → baseline → stimulus → settling → final
//
// Reference: kgb://doctrine/embedded-memory-frugality

package sampling

import "time"

// Phase represents the current sampling phase.
type Phase string

const (
	PhaseStartup  Phase = "startup"
	PhaseWarmup   Phase = "warmup"
	PhaseBaseline Phase = "baseline"
	PhaseStimulus Phase = "stimulus"
	PhaseSettling Phase = "settling"
	PhaseFinal    Phase = "final"
)

// String returns the string representation of a phase.
func (p Phase) String() string {
	return string(p)
}

// PhaseConfig defines durations for each phase.
type PhaseConfig struct {
	StartupDeadline time.Duration // Startup grace period
	Warmup          time.Duration // Warmup phase
	Baseline        time.Duration // Baseline measurement
	Stimulus        time.Duration // Stimulus/workload phase
	Settling        time.Duration // Settling phase
	Final           time.Duration // Final measurement
	Interval        time.Duration // Sample interval
}

// DefaultPhaseConfig returns the default phase configuration.
func DefaultPhaseConfig() PhaseConfig {
	return PhaseConfig{
		StartupDeadline: 120 * time.Second,
		Warmup:          120 * time.Second,
		Baseline:        180 * time.Second,
		Stimulus:        600 * time.Second,
		Settling:        120 * time.Second,
		Final:           180 * time.Second,
		Interval:        5 * time.Second,
	}
}

// SmokePhaseConfig returns a quick smoke test configuration.
func SmokePhaseConfig() PhaseConfig {
	return PhaseConfig{
		StartupDeadline: 10 * time.Second,
		Warmup:          10 * time.Second,
		Baseline:        15 * time.Second,
		Stimulus:        30 * time.Second,
		Settling:        10 * time.Second,
		Final:           15 * time.Second,
		Interval:        1 * time.Second,
	}
}

// TotalDuration returns the total duration of all phases.
func (c PhaseConfig) TotalDuration() time.Duration {
	return c.StartupDeadline + c.Warmup + c.Baseline +
		c.Stimulus + c.Settling + c.Final
}

// DurationFor returns the duration for a specific phase.
func (c *PhaseConfig) DurationFor(phase Phase) time.Duration {
	switch phase {
	case PhaseStartup:
		return c.StartupDeadline
	case PhaseWarmup:
		return c.Warmup
	case PhaseBaseline:
		return c.Baseline
	case PhaseStimulus:
		return c.Stimulus
	case PhaseSettling:
		return c.Settling
	case PhaseFinal:
		return c.Final
	default:
		return 0
	}
}

// PhaseState tracks the current phase and timing.
type PhaseState struct {
	Current     Phase
	PhaseStart  time.Time
	PhaseConfig PhaseConfig
}

// NewPhaseState creates a new phase state machine.
func NewPhaseState(cfg PhaseConfig) *PhaseState {
	return &PhaseState{
		Current:     PhaseStartup,
		PhaseStart:  time.Now(),
		PhaseConfig: cfg,
	}
}

// Advance moves to the next phase.
// Returns true if advanced, false if already in final phase.
func (s *PhaseState) Advance() bool {
	switch s.Current {
	case PhaseStartup:
		s.Current = PhaseWarmup
		s.PhaseStart = time.Now()
		return true
	case PhaseWarmup:
		s.Current = PhaseBaseline
		s.PhaseStart = time.Now()
		return true
	case PhaseBaseline:
		s.Current = PhaseStimulus
		s.PhaseStart = time.Now()
		return true
	case PhaseStimulus:
		s.Current = PhaseSettling
		s.PhaseStart = time.Now()
		return true
	case PhaseSettling:
		s.Current = PhaseFinal
		s.PhaseStart = time.Now()
		return true
	case PhaseFinal:
		return false // Already in final
	default:
		return false
	}
}

// IsFinal returns true if in the final phase.
func (s *PhaseState) IsFinal() bool {
	return s.Current == PhaseFinal
}

// Remaining returns the remaining time in the current phase.
func (s *PhaseState) Remaining() time.Duration {
	if s.Current == PhaseFinal {
		return 0
	}
	elapsed := time.Since(s.PhaseStart)
	phaseDuration := s.PhaseConfig.DurationFor(s.Current)
	remaining := phaseDuration - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Update checks phase transitions based on elapsed time.
func (s *PhaseState) Update() bool {
	if s.IsFinal() {
		return false
	}

	if s.Remaining() == 0 {
		return s.Advance()
	}
	return false
}

// AllPhases returns all phases in order.
func AllPhases() []Phase {
	return []Phase{
		PhaseStartup,
		PhaseWarmup,
		PhaseBaseline,
		PhaseStimulus,
		PhaseSettling,
		PhaseFinal,
	}
}
