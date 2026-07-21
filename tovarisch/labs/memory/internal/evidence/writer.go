// evidence/writer.go — Evidence generation for memory lab artifacts
//
// Writes structured evidence to JSON and CSV formats with checksums.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package evidence

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling"
)

// Writer generates evidence artifacts.
type Writer struct {
	runID       string
	scenario    string
	artifactDir string
	startedAt   time.Time
}

// NewWriter creates a new evidence writer.
func NewWriter(runID, scenario, artifactDir string) *Writer {
	return &Writer{
		runID:       runID,
		scenario:    scenario,
		artifactDir: artifactDir,
		startedAt:   time.Now(),
	}
}

// Manifest represents the lab manifest.json structure.
type Manifest struct {
	SchemaVersion     string            `json:"schema_version"`
	RunID             string            `json:"run_id"`
	Scenario          string            `json:"scenario"`
	StartedAt         time.Time         `json:"started_at"`
	FinishedAt        time.Time         `json:"finished_at"`
	SubjectIdentity   *SubjectIdentity  `json:"subject_identity,omitempty"`
	ControllerID      string            `json:"controller_identity,omitempty"`
	HostID            *HostIdentity     `json:"host_identity,omitempty"`
	DockerID          *DockerIdentity   `json:"docker_identity,omitempty"`
	Configuration     *LabConfiguration `json:"configuration,omitempty"`
	ArtifactInventory []string          `json:"artifact_inventory"`
}

// SubjectIdentity captures the subject's identity for binding.
type SubjectIdentity struct {
	GitCommit                    string `json:"git_commit,omitempty"`
	GitTree                      string `json:"git_tree,omitempty"`
	Version                      string `json:"version,omitempty"`
	ImageID                      string `json:"image_id,omitempty"`
	ImageDigest                  string `json:"image_digest,omitempty"`
	ControllerExecutablePath     string `json:"controller_executable_path,omitempty"`
	ControllerExecutableSHA256  string `json:"controller_executable_sha256,omitempty"`
	ExecHash                    string `json:"exec_hash,omitempty"`
	ConfigHash                  string `json:"config_hash,omitempty"`
}

// HostIdentity captures the host environment.
type HostIdentity struct {
	KernelRelease    string `json:"kernel_release"`
	KernelVersion   string `json:"kernel_version,omitempty"`
	CgroupMode      string `json:"cgroup_mode"`
	CollectionStatus string `json:"collection_status,omitempty"`
}

// DockerIdentity captures the Docker Engine version.
type DockerIdentity struct {
	EngineVersion string `json:"engine_version"`
	APIVersion    string `json:"api_version"`
}

// LabConfiguration records the lab configuration used.
type LabConfiguration struct {
	ResourceLimits interface{} `json:"resource_limits,omitempty"`
	PhaseConfig    interface{} `json:"phase_config,omitempty"`
	Thresholds     interface{} `json:"thresholds,omitempty"`
}

// Verdict represents the verdict.json structure.
type Verdict struct {
	OverallClassification  analysis.Classification  `json:"overall_classification"`
	Scenario               string                   `json:"scenario"`
	ScenarioValid          bool                     `json:"scenario_valid"`
	CanariesValid          bool                     `json:"canaries_valid"`
	MemoryClassification   analysis.Classification  `json:"memory_classification"`
	ResourceClassification analysis.Classification  `json:"resource_classification"`
	SemanticClassification analysis.Classification  `json:"semantic_classification"`
	SignalSummaries        []analysis.SignalSummary `json:"signal_summaries"`
	Thresholds             *analysis.Thresholds     `json:"thresholds"`
	Failures               []string                 `json:"failures,omitempty"`
	Warnings               []string                 `json:"warnings,omitempty"`
	Unknowns               []string                 `json:"unknowns,omitempty"`
	ProvenanceValid        bool                     `json:"provenance_valid"`
	ProvenanceError        string                   `json:"provenance_error,omitempty"`
}

// WriteManifest writes the manifest.json file.
func (w *Writer) WriteManifest(manifest *Manifest) error {
	path := filepath.Join(w.artifactDir, "manifest.json")
	return writeJSON(path, manifest)
}

// WriteVerdict writes the verdict.json file.
func (w *Writer) WriteVerdict(verdict *Verdict) error {
	path := filepath.Join(w.artifactDir, "verdict.json")
	return writeJSON(path, verdict)
}

// WriteSamplesCSV writes samples.csv with LF endings.
func (w *Writer) WriteSamplesCSV(samples []sampling.Sample) error {
	path := filepath.Join(w.artifactDir, "samples.csv")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create samples.csv: %w", err)
	}
	defer f.Close()

	// Write headers
	headers := sampling.CSVHeaders()
	for i, h := range headers {
		if i > 0 {
			if _, err := f.WriteString(","); err != nil {
				return err
			}
		}
		if _, err := f.WriteString(h); err != nil {
			return err
		}
	}
	if _, err := f.WriteString("\n"); err != nil {
		return err
	}

	// Write samples
	for _, s := range samples {
		row := sampling.SampleForCSV(&s)
		for i, v := range row {
			if i > 0 {
				if _, err := f.WriteString(","); err != nil {
					return err
				}
			}
			if _, err := f.WriteString(v); err != nil {
				return err
			}
		}
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	return nil
}

// WriteEventsJSONL writes events.jsonl with LF endings.
func (w *Writer) WriteEventsJSONL(events []sampling.Event) error {
	path := filepath.Join(w.artifactDir, "events.jsonl")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create events.jsonl: %w", err)
	}
	defer f.Close()

	for _, e := range events {
		data, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal event: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return err
		}
	}

	return nil
}

// WriteContainerInspect writes container inspection JSON.
func (w *Writer) WriteContainerInspect(name string, data interface{}) error {
	filename := fmt.Sprintf("%s-inspect.json", name)
	path := filepath.Join(w.artifactDir, filename)
	return writeJSON(path, data)
}

// WriteContainerLogs writes container logs to a text file.
func (w *Writer) WriteContainerLogs(name string, logs []byte) error {
	filename := fmt.Sprintf("%s-logs.txt", name)
	path := filepath.Join(w.artifactDir, filename)
	return os.WriteFile(path, logs, 0644)
}

// WriteFile writes arbitrary data to a file in the artifact directory.
func (w *Writer) WriteFile(name string, data []byte, perm os.FileMode) error {
	path := filepath.Join(w.artifactDir, name)
	return os.WriteFile(path, data, perm)
}

// writeJSON writes data as formatted JSON with LF endings.
func writeJSON(path string, v interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}

	return nil
}

// Checksum holds a SHA256 checksum for an artifact.
type Checksum struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// GenerateChecksumsForInventory generates checksums for the exact declared inventory.
// Excludes checksums.txt itself and fails if any artifact cannot be read.
func (w *Writer) GenerateChecksumsForInventory(inventory []string) ([]Checksum, error) {
	// Sort for deterministic order
	sorted := make([]string, len(inventory))
	copy(sorted, inventory)
	sort.Strings(sorted)

	var checksums []Checksum
	for _, name := range sorted {
		// Skip checksums.txt itself
		if name == "checksums.txt" {
			continue
		}

		path := filepath.Join(w.artifactDir, name)
		sum, size, err := computeSHA256(path)
		if err != nil {
			return nil, fmt.Errorf("checksum %s: %w", name, err)
		}

		checksums = append(checksums, Checksum{
			Path:   name,
			SHA256: sum,
			Size:   size,
		})
	}

	return checksums, nil
}

// GenerateChecksums generates checksums for all artifacts in the directory.
// Excludes checksums.txt and skips unreadable files.
func (w *Writer) GenerateChecksums() ([]Checksum, error) {
	entries, err := os.ReadDir(w.artifactDir)
	if err != nil {
		return nil, fmt.Errorf("read artifact dir: %w", err)
	}

	var checksums []Checksum
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Skip checksums.txt
		if entry.Name() == "checksums.txt" {
			continue
		}

		path := filepath.Join(w.artifactDir, entry.Name())
		sum, size, err := computeSHA256(path)
		if err != nil {
			continue // Skip files we can't checksum
		}

		checksums = append(checksums, Checksum{
			Path:   entry.Name(),
			SHA256: sum,
			Size:   size,
		})
	}

	// Sort for deterministic order
	sort.Slice(checksums, func(i, j int) bool {
		return checksums[i].Path < checksums[j].Path
	})

	return checksums, nil
}

// WriteChecksums writes checksums.txt with artifact checksums.
func (w *Writer) WriteChecksums() error {
	checksums, err := w.GenerateChecksums()
	if err != nil {
		return fmt.Errorf("generate checksums: %w", err)
	}

	path := filepath.Join(w.artifactDir, "checksums.txt")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create checksums.txt: %w", err)
	}
	defer f.Close()

	for _, c := range checksums {
		fmt.Fprintf(f, "%s  %s\n", c.SHA256, c.Path)
	}

	return nil
}

// WriteChecksumsForInventory writes checksums.txt for the exact declared inventory.
func (w *Writer) WriteChecksumsForInventory(inventory []string) error {
	checksums, err := w.GenerateChecksumsForInventory(inventory)
	if err != nil {
		return fmt.Errorf("generate checksums: %w", err)
	}

	path := filepath.Join(w.artifactDir, "checksums.txt")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create checksums.txt: %w", err)
	}
	defer f.Close()

	for _, c := range checksums {
		fmt.Fprintf(f, "%s  %s\n", c.SHA256, c.Path)
	}

	return nil
}

// computeSHA256 computes SHA256 checksum and size of a file.
func computeSHA256(path string) (string, int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}

	h := sha256.New()
	h.Write(data)
	sum := fmt.Sprintf("%x", h.Sum(nil))

	return sum, int64(len(data)), nil
}

// WriteCanaryState writes canary state to initial-canary-state.json or final-canary-state.json.
func (w *Writer) WriteCanaryState(phase string, state interface{}) error {
	filename := fmt.Sprintf("%s-canary-state.json", phase)
	path := filepath.Join(w.artifactDir, filename)
	return writeJSON(path, state)
}

// WriteWorkloadResult writes workload-result.json.
func (w *Writer) WriteWorkloadResult(result interface{}) error {
	path := filepath.Join(w.artifactDir, "workload-result.json")
	return writeJSON(path, result)
}

// ParseChecksumLine parses a single line from checksums.txt.
// Returns (hash, path, error).
func ParseChecksumLine(line string) (hash, path string, err error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", fmt.Errorf("empty line")
	}

	// Format: "hash  filename" (two spaces between)
	parts := strings.SplitN(line, "  ", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("malformed line: %s", line)
	}

	hash = strings.TrimSpace(parts[0])
	path = strings.TrimSpace(parts[1])

	if hash == "" || path == "" {
		return "", "", fmt.Errorf("missing hash or path")
	}

	// Validate hash format (64 hex chars)
	if len(hash) != 64 {
		return "", "", fmt.Errorf("invalid hash length: %s", hash)
	}

	return hash, path, nil
}

// ParseChecksumsFile parses checksums.txt and returns a map of path -> hash.
func ParseChecksumsFile(data string) (map[string]string, error) {
	result := make(map[string]string)
	lines := splitLines(data)

	for _, line := range lines {
		hash, path, err := ParseChecksumLine(line)
		if err != nil {
			return nil, fmt.Errorf("parse line: %w", err)
		}

		if _, exists := result[path]; exists {
			return nil, fmt.Errorf("duplicate entry for: %s", path)
		}

		result[path] = hash
	}

	return result, nil
}

// splitLines splits a string on newlines, excluding empty lines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	return lines
}
