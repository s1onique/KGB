package scriptdoctrine

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// BaselineProvenance contains required metadata from the baseline CSV header
type BaselineProvenance struct {
	CommitHash string
	Algorithm  string
}

// Supported loc algorithms
var supportedLocAlgorithms = map[string]bool{
	"logical-shell-v1": true,
}

// LoadBaseline loads the frozen bootstrap baseline from a CSV file.
// Returns a map of path to BaselineEntry.
// Validates required provenance metadata in header comments.
func LoadBaseline(path string) (map[string]*BaselineEntry, error) {
	// Read file as text first to parse provenance
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("opening baseline: %w", err)
	}

	// Parse and validate provenance
	provenance, err := parseProvenance(string(data))
	if err != nil {
		return nil, fmt.Errorf("baseline provenance: %w", err)
	}
	if err := validateProvenance(provenance); err != nil {
		return nil, fmt.Errorf("baseline provenance: %w", err)
	}

	// Now parse the CSV entries
	baseline := make(map[string]*BaselineEntry)
	r := csv.NewReader(strings.NewReader(string(data)))
	r.Comma = ','
	r.FieldsPerRecord = -1

	lineNum := 0
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading baseline at line %d: %w", lineNum+1, err)
		}

		lineNum++

		// Skip comment lines
		if len(record) > 0 && strings.HasPrefix(strings.TrimSpace(record[0]), "#") {
			continue
		}

		// Skip empty lines
		if len(record) == 0 || (len(record) == 1 && strings.TrimSpace(record[0]) == "") {
			continue
		}

		// Require exactly 3 columns: path, baseline_loc, python_invocation_count
		if len(record) != 3 {
			return nil, fmt.Errorf("baseline line %d: expected 3 columns, got %d", lineNum, len(record))
		}

		entryPath := strings.TrimSpace(record[0])
		locStr := strings.TrimSpace(record[1])
		pyCountStr := strings.TrimSpace(record[2])

		// Validate path
		if err := validatePath(entryPath); err != nil {
			return nil, fmt.Errorf("baseline line %d: invalid path: %w", lineNum, err)
		}

		// Check for duplicate paths
		if _, exists := baseline[entryPath]; exists {
			return nil, fmt.Errorf("baseline line %d: duplicate path: %s", lineNum, entryPath)
		}

		// Parse baseline LOC using strconv.Atoi (strict integer parsing)
		baselineLOC, err := strconv.Atoi(locStr)
		if err != nil {
			return nil, fmt.Errorf("baseline line %d: invalid baseline_loc %q: %w", lineNum, locStr, err)
		}
		if baselineLOC < 0 {
			return nil, fmt.Errorf("baseline line %d: baseline_loc must be non-negative, got %d", lineNum, baselineLOC)
		}

		// Parse python invocation count using strconv.Atoi
		pyCount, err := strconv.Atoi(pyCountStr)
		if err != nil {
			return nil, fmt.Errorf("baseline line %d: invalid python_invocation_count %q: %w", lineNum, pyCountStr, err)
		}
		if pyCount < 0 {
			return nil, fmt.Errorf("baseline line %d: python_invocation_count must be non-negative, got %d", lineNum, pyCount)
		}

		baseline[entryPath] = &BaselineEntry{
			Path:                  entryPath,
			BaselineLOC:           baselineLOC,
			PythonInvocationCount: pyCount,
		}
	}

	return baseline, nil
}

// parseProvenance extracts provenance metadata from baseline header comments
func parseProvenance(content string) (*BaselineProvenance, error) {
	prov := &BaselineProvenance{}

	// Match # baseline_commit=<hash> and # loc_algorithm=<name>
	commitRx := regexp.MustCompile(`(?m)^#\s*baseline_commit=(\S+)`)
	algoRx := regexp.MustCompile(`(?m)^#\s*loc_algorithm=(\S+)`)

	if m := commitRx.FindStringSubmatch(content); m != nil {
		prov.CommitHash = m[1]
	}
	if m := algoRx.FindStringSubmatch(content); m != nil {
		prov.Algorithm = m[1]
	}

	return prov, nil
}

// validateProvenance validates the provenance metadata
func validateProvenance(prov *BaselineProvenance) error {
	// baseline_commit is required
	if prov.CommitHash == "" {
		return errors.New("missing baseline_commit in header")
	}

	// Validate commit hash format (hex string, 7-40 chars)
	commitRx := regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	if !commitRx.MatchString(prov.CommitHash) {
		return fmt.Errorf("invalid baseline_commit %q (expected 7-40 hex chars)", prov.CommitHash)
	}

	// loc_algorithm is required
	if prov.Algorithm == "" {
		return errors.New("missing loc_algorithm in header")
	}

	// Validate algorithm is supported
	if !supportedLocAlgorithms[prov.Algorithm] {
		return fmt.Errorf("unsupported loc_algorithm %q (supported: %v)", prov.Algorithm, supportedLocAlgorithms)
	}

	return nil
}

// FindRepoRoot finds the repository root using git.
func FindRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting cwd: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}

	for {
		if _, err := os.Stat(filepath.Join(cwd, ".git")); err == nil {
			return cwd, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}

	return "", errors.New("repository root not found")
}
