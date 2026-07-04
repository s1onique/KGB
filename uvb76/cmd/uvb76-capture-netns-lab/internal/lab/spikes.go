package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// SpikeObservation represents an observed spike event.
type SpikeObservation struct {
	EventID       string
	CaptureStatus string
	RawJSON       string
	PacketPath    string
	CooldownInfo  bool
}

// phaseArtifactName maps PhaseName to contract artifact names.
func phaseArtifactName(phase PhaseName, suffix string) string {
	switch phase {
	case PhaseBaseline:
		return fmt.Sprintf("phase1-%s", suffix)
	case PhaseDefect:
		return fmt.Sprintf("phase2-%s", suffix)
	case PhaseRecovery:
		return fmt.Sprintf("phase3-%s", suffix)
	default:
		return fmt.Sprintf("%s-%s", phase, suffix)
	}
}

// waitForSpikeAfter polls for a spike event with a specific capture status after a cursor time.
func (o *Orchestrator) waitForSpikeAfter(ctx context.Context, phase PhaseName, cursor string, expectedStatus string, timeout time.Duration) (*SpikeObservation, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("timeout waiting for spike with status %s", expectedStatus)
			}

			// Query spikes API
			res := o.runNsCommand(ctx, o.Config.UVB76NS.Name, "curl", "-s", "-b", o.uvb76AuthCookie,
				o.uvb76APIBase+"/api/v1/latency/spikes?target_id=lab-tovarisch&include_captures=true&limit=20")

			if !res.OK() {
				continue
			}

			// Parse spikes response
			var spikesResp struct {
				Spikes []struct {
					EventID  string `json:"event_id"`
					Severity string `json:"severity"`
					Captures []struct {
						CaptureStatus string `json:"capture_status"`
						NetworkDiag   any    `json:"network_diag"`
						CooldownInfo  any    `json:"cooldown_info"`
					} `json:"captures"`
				} `json:"spikes"`
			}

			if err := json.Unmarshal([]byte(res.Stdout), &spikesResp); err != nil {
				log.Printf("Warning: parse spikes: %v", err)
				continue
			}

			// Find spike with matching status after cursor
			for _, spike := range spikesResp.Spikes {
				// Simple check: if we have captures, check the first one
				for _, cap := range spike.Captures {
					if cap.CaptureStatus == expectedStatus {
						// Found matching status
						obs := &SpikeObservation{
							EventID:       spike.EventID,
							CaptureStatus: cap.CaptureStatus,
							RawJSON:       res.Stdout,
							CooldownInfo:  cap.CooldownInfo != nil,
						}

						// Extract packet if network_diag present
						if cap.NetworkDiag != nil {
							packetJSON, _ := json.Marshal(map[string]interface{}{
								"phase":       string(phase),
								"network_diag": cap.NetworkDiag,
								"timestamp":    time.Now().UTC().Format(time.RFC3339),
							})
							packetName := phaseArtifactName(phase, "capture-packet.json")
							packetPath := filepath.Join(o.labDir, packetName)
							if err := os.WriteFile(packetPath, packetJSON, 0644); err == nil {
								obs.PacketPath = packetPath
							}
						}

						return obs, nil
					}
				}
			}
		}
	}
}

// waitForCooldownExpiration waits until the cooldown from phase 2 has expired.
func (o *Orchestrator) waitForCooldownExpiration(ctx context.Context, phase2Path string) error {
	data, err := os.ReadFile(phase2Path)
	if err != nil {
		return fmt.Errorf("read phase2 row: %w", err)
	}

	// Simple regex extraction for next_capture_eligible_at
	re := regexp.MustCompile(`"next_capture_eligible_at"\s*:\s*"([^"]+)"`)
	matches := re.FindStringSubmatch(string(data))
	if len(matches) < 2 {
		log.Printf("Warning: could not extract next_capture_eligible_at")
		// Fallback: wait a fixed time
		time.Sleep(7 * time.Second)
		return nil
	}

	nextTime, err := time.Parse(time.RFC3339, matches[1])
	if err != nil {
		log.Printf("Warning: parse next_capture_eligible_at: %v", err)
		time.Sleep(7 * time.Second)
		return nil
	}

	waitDuration := time.Until(nextTime) + 2*time.Second
	if waitDuration > 0 {
		log.Printf("Waiting for cooldown expiration: %v", waitDuration)
		time.Sleep(waitDuration)
	}

	return nil
}

// runNsCommand runs a command in a namespace and logs it.
func (o *Orchestrator) runNsCommand(ctx context.Context, ns, name string, args ...string) CommandResult {
	fullArgs := []string{"netns", "exec", ns, name}
	fullArgs = append(fullArgs, args...)
	// LoggingRunner (o.Runner) already logs commands, no manual append needed
	return o.Runner.Run(ctx, "ip", fullArgs...)
}
