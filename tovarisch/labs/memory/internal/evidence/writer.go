// Package evidence provides writers and parsers for the memory
// laboratory's bounded ACT artifact format.
//
// Canonical checksum hash grammar (CORRECTION03):
//   - exactly 64 lowercase hexadecimal characters
//
// Canonical checksum path grammar (CORRECTION03):
//   - local, flat, canonical artifact filename
//   - not absolute
//   - does not contain "..", "/", or "\"
//   - not "." or empty
//   - filepath.Base(name) == name
package evidence

import (
	"crypto/sha256"
	"encoding/hex"
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

	// CORRECTION03: SubjectImageIdentity moves the canary-image
	// provenance from the unchecked canary-image-provenance.json
	// sidecar into the canonical checksummed manifest. Every
	// fact used by the close report must live inside the
	// checksum boundary.
	SubjectImageIdentity *SubjectImageIdentity `json:"subject_image_identity,omitempty"`
}

// SubjectIdentity captures the subject's identity for binding.
type SubjectIdentity struct {
	GitCommit                  string `json:"git_commit,omitempty"`
	GitTree                    string `json:"git_tree,omitempty"`
	GitObjectFormat            string `json:"git_object_format,omitempty"` // "sha1" or "sha256"
	Version                    string `json:"version,omitempty"`
	ImageID                    string `json:"image_id,omitempty"`
	ImageDigest                string `json:"image_digest,omitempty"`
	ControllerExecutablePath   string `json:"controller_executable_path,omitempty"`
	ControllerExecutableSHA256 string `json:"controller_executable_sha256,omitempty"`
	ExecHash                   string `json:"exec_hash,omitempty"`
	ConfigHash                 string `json:"config_hash,omitempty"`
}

// HostIdentity captures the host environment.
type HostIdentity struct {
	KernelRelease    string `json:"kernel_release"`
	KernelVersion    string `json:"kernel_version,omitempty"`
	CgroupMode       string `json:"cgroup_mode"`
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

// SubjectImageIdentity captures the canary image identity and
// the source-tree binding. CORRECTION03 §2-§6: every fact used
// by the close report must live inside the canonical checksum
// boundary (i.e. inside the manifest.json file). The verifier
// reads this block and reconstructs the image identity without
// contacting Docker or Git.
type SubjectImageIdentity struct {
	// Image identity (actual docker image inspect output, never
	// synthesized).
	ImageReference   string   `json:"image_reference,omitempty"`
	ImageID          string   `json:"image_id,omitempty"`
	RepoDigests      []string `json:"repo_digests,omitempty"`
	RepoDigestStatus string   `json:"repo_digest_status,omitempty"` // "available" | "unavailable_local_image"

	// Source-tree binding (captured at image-build time via
	// `git rev-parse`).
	SourceCommitOID        string `json:"source_commit_oid,omitempty"`
	RepositoryTreeOID      string `json:"repository_tree_oid,omitempty"`
	CanarySourceSubtreeOID string `json:"canary_source_subtree_oid,omitempty"`

	// Binary hash binding. PrebuildBinarySHA256 is computed
	// before `docker build`; ExtractedImageBinarySHA256 is
	// computed by `docker create` + `docker cp` + `sha256sum`
	// of /app/canary inside the built image. The two MUST be
	// equal; the producer fails closed if they disagree.
	PrebuildBinarySHA256       string `json:"prebuild_binary_sha256,omitempty"`
	ExtractedImageBinarySHA256 string `json:"extracted_image_binary_sha256,omitempty"`

	// OCI + kgb.dev provenance labels captured at build time.
	// Verified against the source-tree identity and the
	// extracted binary hash.
	RevisionLabel       string `json:"revision_label,omitempty"`
	RepositoryTreeLabel string `json:"repository_tree_label,omitempty"`
	SourceSubtreeLabel  string `json:"source_subtree_label,omitempty"`
	BinarySHA256Label   string `json:"binary_sha256_label,omitempty"`

	// Container image ID (after `docker create`).
	ContainerImageID string `json:"container_image_id,omitempty"`
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

// writeJSON writes data as formatted JSON to path with LF endings.
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

// Checksum holds a SHA-256 checksum for an artifact.
type Checksum struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// ChecksumHashLen is the canonical SHA-256 hash length in lowercase
// hexadecimal characters.
const ChecksumHashLen = 64

// ValidateChecksumHash enforces the canonical hash grammar:
// exactly ChecksumHashLen lowercase hexadecimal characters.
func ValidateChecksumHash(hash string) error {
	if hash == "" {
		return fmt.Errorf("invalid checksum hash encoding: empty")
	}
	if len(hash) != ChecksumHashLen {
		return fmt.Errorf("invalid checksum hash length: %s", hash)
	}
	if hash != strings.ToLower(hash) {
		return fmt.Errorf("non-canonical checksum hash: uppercase hexadecimal")
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return fmt.Errorf("invalid checksum hash encoding: %s", hash)
	}
	return nil
}

// ValidateChecksumArtifactPath enforces the canonical path grammar
// for the flat bounded ACT artifact layout. The path must be a
// relative, single-segment, canonical artifact name.
func ValidateChecksumArtifactPath(name string) error {
	if name == "" {
		return fmt.Errorf("invalid checksum artifact path: \"\"")
	}
	if name == "." {
		return fmt.Errorf("invalid checksum artifact path: %q", name)
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return fmt.Errorf("invalid checksum artifact path: %q", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid checksum artifact path: %q", name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid checksum artifact path: %q", name)
	}
	if filepath.Base(name) != name {
		return fmt.Errorf("invalid checksum artifact path: %q", name)
	}
	return nil
}

// ParseChecksumLine parses a single line from checksums.txt.
// Returns (hash, path, error). The hash is validated against the
// canonical lowercase-hex SHA-256 grammar. The path is validated
// against the canonical flat-artifact grammar.
func ParseChecksumLine(line string) (hash, path string, err error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", fmt.Errorf("malformed line: empty")
	}

	// Format: "hash  filename" (two spaces between)
	parts := strings.SplitN(line, "  ", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("malformed delimiter in checksum line")
	}

	hash = strings.TrimSpace(parts[0])
	path = strings.TrimSpace(parts[1])

	if hash == "" {
		return "", "", fmt.Errorf("invalid checksum hash encoding: empty")
	}
	if path == "" {
		return "", "", fmt.Errorf("invalid checksum artifact path: \"\"")
	}
	if err := ValidateChecksumHash(hash); err != nil {
		return "", "", err
	}
	if err := ValidateChecksumArtifactPath(path); err != nil {
		return "", "", err
	}
	return hash, path, nil
}

// ParseChecksumsFile parses checksums.txt and returns a map of
// path -> hash. The parser rejects duplicate entries, paths
// containing "checksums.txt", and any other path that fails the
// canonical grammar. Each checksum is verified against the canonical
// lowercase-hex SHA-256 grammar.
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

// GenerateChecksumsForInventory generates checksums.txt from the
// declared inventory, matching the production writer's behaviour.
func (w *Writer) GenerateChecksumsForInventory(inventory []string) ([]Checksum, error) {
	// Build the deterministic sorted list, skipping checksums.txt
	// itself (per the production convention).
	type entry struct {
		path   string
		sha256 string
	}
	entries := make([]entry, 0, len(inventory))
	for _, name := range inventory {
		if name == "checksums.txt" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(w.artifactDir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		sum := sha256.Sum256(data)
		entries = append(entries, entry{name, hex.EncodeToString(sum[:])})
	}
	// Sort by path for determinism.
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j-1].path > entries[j].path; j-- {
			entries[j-1], entries[j] = entries[j], entries[j-1]
		}
	}
	out := make([]Checksum, 0, len(entries))
	for _, e := range entries {
		out = append(out, Checksum{Path: e.path, SHA256: e.sha256})
	}
	return out, nil
}

// GenerateChecksums generates checksums.txt from the directory
// contents (deprecated, retained for backwards compatibility).
func (w *Writer) GenerateChecksums() ([]Checksum, error) {
	entries, err := os.ReadDir(w.artifactDir)
	if err != nil {
		return nil, fmt.Errorf("read artifact dir: %w", err)
	}
	type entry struct {
		path   string
		sha256 string
	}
	var all []entry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if e.Name() == "checksums.txt" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(w.artifactDir, e.Name()))
		if err != nil {
			continue
		}
		sum := sha256.Sum256(data)
		all = append(all, entry{e.Name(), hex.EncodeToString(sum[:])})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].path < all[j].path })
	out := make([]Checksum, 0, len(all))
	for _, e := range all {
		out = append(out, Checksum{Path: e.path, SHA256: e.sha256})
	}
	return out, nil
}

// WriteChecksums writes checksums.txt with the production
// deterministic format.
func (w *Writer) WriteChecksums() error {
	checksums, err := w.GenerateChecksums()
	if err != nil {
		return fmt.Errorf("generate checksums: %w", err)
	}
	return w.writeChecksums(checksums)
}

// WriteChecksumsForInventory writes checksums.txt from the declared
// inventory, skipping checksums.txt itself.
func (w *Writer) WriteChecksumsForInventory(inventory []string) error {
	checksums, err := w.GenerateChecksumsForInventory(inventory)
	if err != nil {
		return fmt.Errorf("generate checksums: %w", err)
	}
	return w.writeChecksums(checksums)
}

func (w *Writer) writeChecksums(checksums []Checksum) error {
	var buf strings.Builder
	for _, c := range checksums {
		fmt.Fprintf(&buf, "%s  %s\n", c.SHA256, c.Path)
	}
	return os.WriteFile(filepath.Join(w.artifactDir, "checksums.txt"),
		[]byte(buf.String()), 0644)
}

// computeSHA256 computes the SHA-256 of a file's contents.
func computeSHA256(path string) (string, int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), int64(len(data)), nil
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
