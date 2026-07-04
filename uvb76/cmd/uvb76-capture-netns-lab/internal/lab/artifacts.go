package lab

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LabArtifacts holds all artifact paths for the lab.
type LabArtifacts struct {
	Root              string
	ConfigPath        string
	UVB76LogPath      string
	TovarischLogPath  string
	Phase1SpikePath   string
	Phase1CapturePath string
	Phase2SpikePath   string
	Phase3SpikePath   string
	Phase3CapturePath string
	SummaryPath       string
	CommandLogPath    string
	CleanupLogPath    string
	TopologyPath      string
	ResultPath        string
}

// ArtifactNames defines the artifact filename constants.
type ArtifactNames struct {
	Phase0Status            string
	Phase0ProbeReady        string
	Phase1SpikeEvent        string
	Phase1SpikeRow          string
	Phase1CapturePacket     string
	Phase1CaptureContract   string
	Phase2SpikeEvent        string
	Phase2SpikeRow          string
	Phase2CaptureContract   string
	Phase3SpikeEvent        string
	Phase3SpikeRow          string
	Phase3CapturePacket     string
	Phase3CaptureContract   string
	Phase3CooldownWait      string
	ContractVerifierOutput  string
	Topology                string
	UVB76Config             string
	UVB76Log                string
	TovarischConfig         string
	TovarischLog            string
	Result                  string
	CommandLog              string
	CleanupLog              string
}

// DefaultArtifactNames returns the standard artifact filenames.
func DefaultArtifactNames() ArtifactNames {
	return ArtifactNames{
		Phase0Status:           "phase0-status.json",
		Phase0ProbeReady:      "phase0-probe-ready.json",
		Phase1SpikeEvent:      "phase1-spike-event.json",
		Phase1SpikeRow:        "phase1-spike-row.json",
		Phase1CapturePacket:   "phase1-capture-packet.json",
		Phase1CaptureContract: "phase1-capture-contract.json",
		Phase2SpikeEvent:      "phase2-spike-event.json",
		Phase2SpikeRow:        "phase2-spike-row.json",
		Phase2CaptureContract: "phase2-capture-contract.json",
		Phase3SpikeEvent:      "phase3-spike-event.json",
		Phase3SpikeRow:        "phase3-spike-row.json",
		Phase3CapturePacket:   "phase3-capture-packet.json",
		Phase3CaptureContract: "phase3-capture-contract.json",
		Phase3CooldownWait:    "phase3-cooldown-wait-summary.json",
		ContractVerifierOutput: "contract-verifier-output.txt",
		Topology:              "topology.txt",
		UVB76Config:           "uvb76.json",
		UVB76Log:              "uvb76.log",
		TovarischConfig:       "tovarisch.conf",
		TovarischLog:          "tovarisch.log",
		Result:                "result.json",
		CommandLog:            "command-log.json",
		CleanupLog:            "cleanup-log.json",
	}
}

// NewLabArtifacts creates artifact paths under the given root directory.
func NewLabArtifacts(root string, names ArtifactNames) *LabArtifacts {
	return &LabArtifacts{
		Root:              root,
		ConfigPath:        filepath.Join(root, names.UVB76Config),
		UVB76LogPath:      filepath.Join(root, names.UVB76Log),
		TovarischLogPath:  filepath.Join(root, names.TovarischLog),
		Phase1SpikePath:   filepath.Join(root, names.Phase1SpikeRow),
		Phase1CapturePath: filepath.Join(root, names.Phase1CapturePacket),
		Phase2SpikePath:   filepath.Join(root, names.Phase2SpikeRow),
		Phase3SpikePath:   filepath.Join(root, names.Phase3SpikeRow),
		Phase3CapturePath: filepath.Join(root, names.Phase3CapturePacket),
		SummaryPath:       filepath.Join(root, "lab-summary.json"),
		CommandLogPath:   filepath.Join(root, names.CommandLog),
		CleanupLogPath:   filepath.Join(root, names.CleanupLog),
		TopologyPath:     filepath.Join(root, names.Topology),
		ResultPath:       filepath.Join(root, names.Result),
	}
}

// CreateArtifactDir creates a unique temp directory for lab artifacts.
func CreateArtifactDir(name string) (string, error) {
	dir, err := os.MkdirTemp("/tmp", name+"-*")
	if err != nil {
		return "", fmt.Errorf("mkdtemp: %w", err)
	}
	return dir, nil
}

// WriteJSON writes a JSON file to the artifact directory.
func WriteJSON(dir, filename string, data interface{}) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", filename, err)
	}

	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filename, err)
	}

	return nil
}

// LabSummary captures the complete lab outcome.
type LabSummary struct {
	SchemaVersion string        `json:"schema_version"`
	StartedAt     time.Time     `json:"started_at"`
	EndedAt       time.Time     `json:"ended_at"`
	Phases        []PhaseResult `json:"phases"`
	Artifacts     []string      `json:"artifacts"`
	Commands      []CommandLog  `json:"commands"`
	OK            bool          `json:"ok"`
}

// CommandLog records a command execution.
type CommandLog struct {
	Command  []string `json:"command"`
	ExitCode int      `json:"exit_code"`
	Duration string   `json:"duration"`
	Stdout   string   `json:"stdout,omitempty"`
	Stderr   string   `json:"stderr,omitempty"`
	Time     string   `json:"time"`
}

// LogCommand converts a CommandResult to a CommandLog entry.
func LogCommand(result CommandResult) CommandLog {
	return CommandLog{
		Command:  result.Command,
		ExitCode: result.ExitCode,
		Duration: result.Duration().String(),
		Stdout:   truncate(result.Stdout, 1024),
		Stderr:   truncate(result.Stderr, 1024),
		Time:     result.Started.Format(time.RFC3339Nano),
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

// WriteSummary writes the lab summary to the artifact directory.
func WriteSummary(artifacts *LabArtifacts, summary *LabSummary) error {
	return WriteJSON(artifacts.Root, "lab-summary.json", summary)
}

// LoadSummary loads a lab summary from an artifact directory.
func LoadSummary(dir string) (*LabSummary, error) {
	data, err := os.ReadFile(filepath.Join(dir, "lab-summary.json"))
	if err != nil {
		return nil, err
	}

	var summary LabSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, fmt.Errorf("malformed summary: %w", err)
	}

	return &summary, nil
}

