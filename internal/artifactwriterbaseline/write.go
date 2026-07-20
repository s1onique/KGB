// write.go — Writer for generating sharded baseline.
package artifactwriterbaseline

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WriteConfig controls how shards are generated.
type WriteConfig struct {
	BaselineCommit string
	Generator     string
	GeneratedAt   string // If empty, uses canonical empty string (deterministic baseline)
}

// Write writes findings as sharded JSONL files to the given directory.
// It creates a manifest and one shard per surface_id, further splitting
// memory-lab-evidence by source file to stay under line limits.
func Write(dir string, findings []Finding, config WriteConfig) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Group findings by shard key (surface_id or surface_id+source_file for memory-lab-evidence)
	shards := groupFindings(findings)

	// Sort shard names for deterministic ordering
	var shardNames []string
	for name := range shards {
		shardNames = append(shardNames, name)
	}
	sort.Strings(shardNames)

	// Collect unique surface IDs
	surfaceIDs := make(map[string]bool)
	for _, f := range findings {
		surfaceIDs[f.SurfaceID] = true
	}
	var sortedSurfaces []string
	for s := range surfaceIDs {
		sortedSurfaces = append(sortedSurfaces, s)
	}
	sort.Strings(sortedSurfaces)

	// Write each shard
	for _, shardName := range shardNames {
		shardFindings := shards[shardName]
		if err := writeShard(dir, shardName, shardFindings); err != nil {
			return fmt.Errorf("write shard %s: %w", shardName, err)
		}
	}

	// Write manifest - use canonical empty string for deterministic baseline identity
	// If GeneratedAt is explicitly provided, use it; otherwise use empty string
	generatedAt := config.GeneratedAt

	manifest := Manifest{
		SchemaVersion:  SchemaVersion,
		BaselineCommit: config.BaselineCommit,
		Generator:      config.Generator,
		GeneratedAt:    generatedAt,
		Shards:         shardNames,
		SurfaceIDs:     sortedSurfaces,
	}

	manifestPath := filepath.Join(dir, "manifest.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}

// groupFindings groups findings into shards.
// For memory-lab-evidence, it further splits by source file to stay under line limits.
// For all other surfaces, it groups by surface_id.
func groupFindings(findings []Finding) map[string][]Finding {
	shards := make(map[string][]Finding)

	// Sort findings for consistent ordering
	sorted := make([]Finding, len(findings))
	copy(sorted, findings)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.SurfaceID != b.SurfaceID {
			return a.SurfaceID < b.SurfaceID
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})

	for _, f := range sorted {
		shardName := f.SurfaceID + ".jsonl"

		// Further split memory-lab-evidence by source file
		if f.SurfaceID == "memory-lab-evidence" {
			base := filepath.Base(f.File)
			name := strings.TrimSuffix(base, filepath.Ext(base))
			shardName = fmt.Sprintf("memory-lab-evidence-%s.jsonl", name)
		}

		shards[shardName] = append(shards[shardName], f)
	}

	return shards
}

// writeShard writes a single JSONL shard.
func writeShard(dir, shardName string, findings []Finding) error {
	shardPath := filepath.Join(dir, shardName)
	file, err := os.Create(shardPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, f := range findings {
		data, err := json.Marshal(f)
		if err != nil {
			return fmt.Errorf("marshal finding: %w", err)
		}
		if _, err := writer.Write(data); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		if _, err := writer.WriteString("\n"); err != nil {
			return fmt.Errorf("write newline: %w", err)
		}
	}

	return writer.Flush()
}
