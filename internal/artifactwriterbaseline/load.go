// load.go — Production loader for sharded baseline.
package artifactwriterbaseline

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Common errors.
var (
	ErrMissingManifest  = errors.New("manifest not found")
	ErrMalformedJSON   = errors.New("malformed JSON")
	ErrDuplicateFinding = errors.New("duplicate finding")
	ErrInvalidPath     = errors.New("invalid shard path")
	ErrMissingShard    = errors.New("missing shard")
)

// LoadError wraps loading failures with context.
type LoadError struct {
	Shard  string // empty for manifest errors
	Line   int    // 0 for manifest errors
	Cause  error
}

func (e *LoadError) Error() string {
	if e.Shard == "" {
		return "manifest: " + e.Cause.Error()
	}
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d: %v", e.Shard, e.Line, e.Cause)
	}
	return e.Shard + ": " + e.Cause.Error()
}

func (e *LoadError) Unwrap() error {
	return e.Cause
}

// LoadManifest loads and validates the manifest from the given directory.
func LoadManifest(dir string) (*Manifest, error) {
	manifestPath := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &LoadError{Cause: ErrMissingManifest}
		}
		return nil, &LoadError{Cause: err}
	}

	var m Manifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return nil, &LoadError{Cause: fmt.Errorf("%w: %v", ErrMalformedJSON, err)}
	}

	// Validate manifest structure
	if errs := m.Validate(); len(errs) > 0 {
		return nil, &LoadError{Cause: fmt.Errorf("validation failed: %v", errs[0])}
	}

	return &m, nil
}

// LoadShard loads a single JSONL shard with strict validation.
func LoadShard(dir, shardName string) ([]Finding, error) {
	// Validate shard name is safe
	if err := validateShardName(shardName); err != nil {
		return nil, &LoadError{Shard: shardName, Cause: err}
	}

	shardPath := filepath.Join(dir, shardName)
	file, err := os.Open(shardPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &LoadError{Shard: shardName, Cause: ErrMissingShard}
		}
		return nil, &LoadError{Shard: shardName, Cause: err}
	}
	defer file.Close()

	var findings []Finding
	lineNum := 0
	scanner := bufio.NewScanner(file)
	// Set buffer to handle records up to 64KB, max 1MB
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var f Finding
		decoder := json.NewDecoder(strings.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&f); err != nil {
			return nil, &LoadError{
				Shard: shardName,
				Line:  lineNum,
				Cause: fmt.Errorf("%w: %v", ErrMalformedJSON, err),
			}
		}

		// Validate finding
		if errs := f.Validate(); len(errs) > 0 {
			return nil, &LoadError{
				Shard: shardName,
				Line:  lineNum,
				Cause: fmt.Errorf("validation failed: %v", errs[0]),
			}
		}

		findings = append(findings, f)
	}

	if err := scanner.Err(); err != nil {
		return nil, &LoadError{Shard: shardName, Cause: err}
	}

	return findings, nil
}

// LoadAll loads all shards from the manifest in the given directory.
// Findings are sorted by (surface_id, file, line, finding_id) for deterministic output.
// Returns all findings or an error.
func LoadAll(dir string) ([]Finding, error) {
	manifest, err := LoadManifest(dir)
	if err != nil {
		return nil, err
	}

	// Track findings by ID to detect duplicates
	seen := make(map[string]bool)
	var allFindings []Finding

	for _, shardName := range manifest.Shards {
		findings, err := LoadShard(dir, shardName)
		if err != nil {
			return nil, err
		}

		for _, f := range findings {
			if seen[f.FindingID] {
				return nil, &LoadError{
					Shard: shardName,
					Cause: fmt.Errorf("%w: %s", ErrDuplicateFinding, f.FindingID),
				}
			}
			seen[f.FindingID] = true
			allFindings = append(allFindings, f)
		}
	}

	// Reconcile manifest surface IDs with loaded findings
	loadedSurfaces := make(map[string]bool)
	for _, f := range allFindings {
		loadedSurfaces[f.SurfaceID] = true
	}
	for _, s := range manifest.SurfaceIDs {
		if !loadedSurfaces[s] {
			return nil, &LoadError{
				Cause: fmt.Errorf("surface_id %q declared in manifest but no findings loaded", s),
			}
		}
	}
	for s := range loadedSurfaces {
		found := false
		for _, ms := range manifest.SurfaceIDs {
			if ms == s {
				found = true
				break
			}
		}
		if !found {
			return nil, &LoadError{
				Cause: fmt.Errorf("surface_id %q has findings but not declared in manifest", s),
			}
		}
	}

	// Sort for deterministic output using stable sort on a key that includes finding_id
	sort.SliceStable(allFindings, func(i, j int) bool {
		a, b := allFindings[i], allFindings[j]
		if a.SurfaceID != b.SurfaceID {
			return a.SurfaceID < b.SurfaceID
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		// finding_id is guaranteed unique and provides final ordering
		return a.FindingID < b.FindingID
	})

	return allFindings, nil
}

// validateShardName ensures the shard name is safe to use in path operations.
func validateShardName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalidPath)
	}
	if !strings.HasSuffix(name, ".jsonl") {
		return fmt.Errorf("%w: must have .jsonl extension", ErrInvalidPath)
	}
	// filepath.IsLocal returns true for relative paths that don't escape the base
	// This prevents ../../../etc/passwd style attacks
	if !filepath.IsLocal(name) {
		return fmt.Errorf("%w: must be relative path", ErrInvalidPath)
	}
	// Check for path traversal attempts
	clean := filepath.Clean(name)
	if clean != name || clean == "." || clean == ".." {
		return fmt.Errorf("%w: path traversal not allowed", ErrInvalidPath)
	}
	return nil
}
