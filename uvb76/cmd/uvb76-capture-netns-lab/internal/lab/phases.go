package lab

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// PhaseName represents the name of a lab phase.
type PhaseName string

const (
	PhaseBaseline   PhaseName = "baseline"
	PhaseDefect     PhaseName = "defect"
	PhaseRecovery   PhaseName = "recovery"
)

// PhaseConfig holds phase configuration.
type PhaseConfig struct {
	Name        PhaseName
	Description string
	Timeout     time.Duration
}

// DefaultPhaseConfigs returns the default phase configurations.
func DefaultPhaseConfigs() []PhaseConfig {
	return []PhaseConfig{
		{Name: PhaseBaseline, Description: "Baseline probe readiness", Timeout: 60 * time.Second},
		{Name: PhaseDefect, Description: "Lab probe defect injection", Timeout: 30 * time.Second},
		{Name: PhaseRecovery, Description: "Recovery after defect", Timeout: 60 * time.Second},
	}
}

// PhaseOutcome represents expected outcome for a phase.
type PhaseOutcome struct {
	ExpectedCaptureStatus string
	MinSpikes             int
}

// DefaultPhaseOutcomes returns default phase outcomes.
func DefaultPhaseOutcomes() map[PhaseName]PhaseOutcome {
	return map[PhaseName]PhaseOutcome{
		PhaseBaseline: {ExpectedCaptureStatus: CaptureStatusCaptured, MinSpikes: 1},
		PhaseDefect:   {ExpectedCaptureStatus: CaptureStatusSkippedCooldown, MinSpikes: 1},
		PhaseRecovery: {ExpectedCaptureStatus: CaptureStatusCaptured, MinSpikes: 1},
	}
}

// PhaseResult holds the result of a phase execution.
type PhaseResult struct {
	Name            PhaseName
	Started         time.Time
	Ended          time.Time
	Err            string
	SpikeEventID   string
	CaptureStatus  string
	ArtifactPaths  []string
}

// Capture status constants.
const (
	CaptureStatusCaptured      = "captured"
	CaptureStatusSkippedCooldown = "skipped_cooldown"
)

// runPhase executes a single lab phase.
func (o *Orchestrator) runPhase(ctx context.Context, phase PhaseConfig) PhaseResult {
	result := PhaseResult{
		Name:    phase.Name,
		Started: time.Now(),
	}

	log.Printf("=== Phase: %s ===", phase.Name)

	// Set cursor for this phase
	o.Tracker.SetCursor(phase.Name)

	// Execute phase-specific logic
	switch phase.Name {
	case PhaseBaseline:
		o.runBaselinePhase(ctx, phase, &result)
	case PhaseDefect:
		o.runDefectPhase(ctx, phase, &result)
	case PhaseRecovery:
		o.runRecoveryPhase(ctx, phase, &result)
	}

	result.Ended = time.Now()
	return result
}

func (o *Orchestrator) runBaselinePhase(ctx context.Context, phase PhaseConfig, result *PhaseResult) {
	log.Printf("Running baseline phase...")

	// Set phase cursor
	cursor := time.Now().UTC().Format(time.RFC3339)
	o.Tracker.SetCursor(PhaseBaseline)

	// Wait for spike event with capture
	spike, err := o.waitForSpikeAfter(ctx, PhaseBaseline, cursor, "captured", 30*time.Second)
	if err != nil {
		result.Err = fmt.Sprintf("baseline spike: %v", err)
		return
	}

	result.SpikeEventID = spike.EventID
	result.CaptureStatus = spike.CaptureStatus

	// Save phase artifacts
	spikeRowPath := filepath.Join(o.labDir, "phase1-spike-row.json")
	if err := os.WriteFile(spikeRowPath, []byte(spike.RawJSON), 0644); err != nil {
		log.Printf("Warning: failed to write phase1-spike-row.json: %v", err)
	}

	// Save capture packet
	if spike.PacketPath != "" {
		result.ArtifactPaths = []string{spikeRowPath, spike.PacketPath}
	} else {
		result.ArtifactPaths = []string{spikeRowPath}
	}

	log.Printf("Baseline phase complete: event=%s status=%s", spike.EventID, spike.CaptureStatus)
}

func (o *Orchestrator) runDefectPhase(ctx context.Context, phase PhaseConfig, result *PhaseResult) {
	log.Printf("Running defect phase...")

	// Set phase cursor
	cursor := time.Now().UTC().Format(time.RFC3339)
	o.Tracker.SetCursor(PhaseDefect)

	// Inject tc netem 100% loss defect (lab contract defect for Phase 2)
	if err := o.injectDefect(ctx); err != nil {
		result.Err = fmt.Sprintf("inject defect: %v", err)
		return
	}

	// Wait for skipped_cooldown spike
	spike, err := o.waitForSpikeAfter(ctx, PhaseDefect, cursor, "skipped_cooldown", 15*time.Second)
	if err != nil {
		log.Printf("Warning: defect spike wait: %v", err)
		// Clear defect before returning
		o.clearDefect(ctx)
		result.Err = fmt.Sprintf("defect spike: %v", err)
		return
	}

	result.SpikeEventID = spike.EventID
	result.CaptureStatus = spike.CaptureStatus

	// Save phase artifacts - phase2-spike-row.json for contract verifier
	spikeRowPath := filepath.Join(o.labDir, "phase2-spike-row.json")
	if err := os.WriteFile(spikeRowPath, []byte(spike.RawJSON), 0644); err != nil {
		log.Printf("Warning: failed to write phase2-spike-row.json: %v", err)
	}

	// Save capture packet
	if spike.PacketPath != "" {
		result.ArtifactPaths = []string{spikeRowPath, spike.PacketPath}
	} else {
		result.ArtifactPaths = []string{spikeRowPath}
	}

	log.Printf("Defect phase complete: event=%s status=%s", spike.EventID, spike.CaptureStatus)
}

func (o *Orchestrator) runRecoveryPhase(ctx context.Context, phase PhaseConfig, result *PhaseResult) {
	log.Printf("Running recovery phase...")

	// Get cooldown info from phase 2
	phase2Path := filepath.Join(o.labDir, "phase2-spike-row.json")

	// Wait for cooldown expiration based on phase 2 cooldown_info
	if err := o.waitForCooldownExpiration(ctx, phase2Path); err != nil {
		log.Printf("Warning: cooldown wait: %v", err)
	}

	// Clear defect
	if err := o.clearDefect(ctx); err != nil {
		log.Printf("Warning: clear defect: %v", err)
	}

	// Wait a bit after clearing
	time.Sleep(2 * time.Second)

	// Set phase cursor
	cursor := time.Now().UTC().Format(time.RFC3339)
	o.Tracker.SetCursor(PhaseRecovery)

	// Wait for captured spike
	spike, err := o.waitForSpikeAfter(ctx, PhaseRecovery, cursor, "captured", 30*time.Second)
	if err != nil {
		result.Err = fmt.Sprintf("recovery spike: %v", err)
		return
	}

	result.SpikeEventID = spike.EventID
	result.CaptureStatus = spike.CaptureStatus

	// Save phase artifacts
	spikeRowPath := filepath.Join(o.labDir, "phase3-spike-row.json")
	if err := os.WriteFile(spikeRowPath, []byte(spike.RawJSON), 0644); err != nil {
		log.Printf("Warning: failed to write phase3-spike-row.json: %v", err)
	}

	// Save capture packet
	if spike.PacketPath != "" {
		result.ArtifactPaths = []string{spikeRowPath, spike.PacketPath}
	} else {
		result.ArtifactPaths = []string{spikeRowPath}
	}

	log.Printf("Recovery phase complete: event=%s status=%s", spike.EventID, spike.CaptureStatus)
}
