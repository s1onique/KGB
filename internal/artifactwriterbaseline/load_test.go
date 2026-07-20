// load_test.go — Tests for baseline loader.
package artifactwriterbaseline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleFindings returns test findings for testing.
func sampleFindings() []Finding {
	return []Finding{
		{FindingID: "sha256:aaa111", SurfaceID: "surface-a", File: "a.go", Line: 10},
		{FindingID: "sha256:bbb222", SurfaceID: "surface-a", File: "a.go", Line: 20},
		{FindingID: "sha256:ccc333", SurfaceID: "surface-b", File: "b.go", Line: 5},
	}
}

// writeTestManifest writes a manifest file to the directory.
func writeTestManifest(t *testing.T, dir string, m *Manifest) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// writeTestShard writes a JSONL shard to the directory.
func writeTestShard(t *testing.T, dir, name string, findings []Finding) {
	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create shard: %v", err)
	}
	defer file.Close()

	for _, f := range findings {
		data, err := json.Marshal(f)
		if err != nil {
			t.Fatalf("marshal finding: %v", err)
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}

func TestLoadManifest_Valid(t *testing.T) {
	dir := t.TempDir()
	manifest := &Manifest{
		SchemaVersion:  SchemaVersion,
		BaselineCommit: "abc123",
		Generator:      "test-scanner",
		GeneratedAt:    "2024-01-01T00:00:00Z",
		Shards:         []string{"surface-a.jsonl", "surface-b.jsonl"},
		SurfaceIDs:     []string{"surface-a", "surface-b"},
	}
	writeTestManifest(t, dir, manifest)

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if m.SchemaVersion != SchemaVersion {
		t.Errorf("expected schema_version %q, got %q", SchemaVersion, m.SchemaVersion)
	}
	if m.BaselineCommit != "abc123" {
		t.Errorf("expected baseline_commit %q, got %q", "abc123", m.BaselineCommit)
	}
	if len(m.Shards) != 2 {
		t.Errorf("expected 2 shards, got %d", len(m.Shards))
	}
}

func TestLoadManifest_Missing(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadManifest(dir)
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
	var loadErr *LoadError
	if !errorsAs(err, &loadErr) {
		t.Fatalf("expected LoadError, got %T", err)
	}
	if !strings.Contains(err.Error(), "manifest") {
		t.Errorf("error should mention manifest: %v", err)
	}
}

func TestLoadManifest_WrongSchema(t *testing.T) {
	dir := t.TempDir()
	manifest := &Manifest{
		SchemaVersion:  "ratchet-v2", // wrong schema
		BaselineCommit: "abc123",
		Generator:      "test",
		Shards:         []string{"a.jsonl"},
		SurfaceIDs:     []string{"surface-a"},
	}
	writeTestManifest(t, dir, manifest)

	_, err := LoadManifest(dir)
	if err == nil {
		t.Fatal("expected error for wrong schema")
	}
}

func TestLoadManifest_DuplicateShards(t *testing.T) {
	dir := t.TempDir()
	manifest := &Manifest{
		SchemaVersion:  SchemaVersion,
		BaselineCommit: "abc123",
		Generator:      "test",
		Shards:         []string{"a.jsonl", "a.jsonl"}, // duplicate
		SurfaceIDs:     []string{"surface-a"},
	}
	writeTestManifest(t, dir, manifest)

	_, err := LoadManifest(dir)
	if err == nil {
		t.Fatal("expected error for duplicate shards")
	}
}

func TestLoadShard_Valid(t *testing.T) {
	dir := t.TempDir()
	findings := []Finding{
		{FindingID: "sha256:aaa", SurfaceID: "test", File: "a.go", Line: 1},
		{FindingID: "sha256:bbb", SurfaceID: "test", File: "a.go", Line: 2},
	}
	writeTestShard(t, dir, "test.jsonl", findings)

	result, err := LoadShard(dir, "test.jsonl")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 findings, got %d", len(result))
	}
}

func TestLoadShard_Missing(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadShard(dir, "nonexistent.jsonl")
	if err == nil {
		t.Fatal("expected error for missing shard")
	}
}

func TestLoadShard_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jsonl")
	os.WriteFile(path, []byte(`{"valid": true}`+"\n"+`{ invalid }`+"\n"), 0644)

	_, err := LoadShard(dir, "bad.jsonl")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestLoadShard_InvalidPath(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadShard(dir, "../../../etc/passwd.jsonl")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestLoadAll_SortedAndDeduplicated(t *testing.T) {
	dir := t.TempDir()
	manifest := &Manifest{
		SchemaVersion:  SchemaVersion,
		BaselineCommit: "abc123",
		Generator:      "test",
		Shards:         []string{"surface-b.jsonl", "surface-a.jsonl"},
		SurfaceIDs:     []string{"surface-a", "surface-b"},
	}
	writeTestManifest(t, dir, manifest)

	// Write shards in non-sorted order
	writeTestShard(t, dir, "surface-b.jsonl", []Finding{
		{FindingID: "sha256:ccc", SurfaceID: "surface-b", File: "b.go", Line: 5},
	})
	writeTestShard(t, dir, "surface-a.jsonl", []Finding{
		{FindingID: "sha256:aaa", SurfaceID: "surface-a", File: "a.go", Line: 10},
		{FindingID: "sha256:bbb", SurfaceID: "surface-a", File: "a.go", Line: 20},
	})

	findings, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(findings) != 3 {
		t.Errorf("expected 3 findings, got %d", len(findings))
	}

	// Verify sorted order
	expectedOrder := []string{"sha256:aaa", "sha256:bbb", "sha256:ccc"}
	for i, expected := range expectedOrder {
		if findings[i].FindingID != expected {
			t.Errorf("position %d: expected %s, got %s", i, expected, findings[i].FindingID)
		}
	}
}

func TestLoadAll_DuplicateDetection(t *testing.T) {
	dir := t.TempDir()
	manifest := &Manifest{
		SchemaVersion:  SchemaVersion,
		BaselineCommit: "abc123",
		Generator:      "test",
		Shards:         []string{"a.jsonl", "b.jsonl"},
		SurfaceIDs:     []string{"surface-a"},
	}
	writeTestManifest(t, dir, manifest)

	// Same finding ID in two shards
	writeTestShard(t, dir, "a.jsonl", []Finding{
		{FindingID: "sha256:dup", SurfaceID: "surface-a", File: "a.go", Line: 1},
	})
	writeTestShard(t, dir, "b.jsonl", []Finding{
		{FindingID: "sha256:dup", SurfaceID: "surface-a", File: "b.go", Line: 1}, // duplicate!
	})

	_, err := LoadAll(dir)
	if err == nil {
		t.Fatal("expected error for duplicate findings")
	}
}

func TestLoadAll_SurfaceReconciliation(t *testing.T) {
	t.Run("declared_surface_with_no_findings", func(t *testing.T) {
		dir := t.TempDir()
		manifest := &Manifest{
			SchemaVersion:  SchemaVersion,
			BaselineCommit: "abc123",
			Generator:      "test",
			Shards:         []string{"surface-a.jsonl"},
			SurfaceIDs:     []string{"surface-a", "surface-b"}, // surface-b declared but no shard
		}
		writeTestManifest(t, dir, manifest)

		// Only write surface-a findings
		writeTestShard(t, dir, "surface-a.jsonl", []Finding{
			{FindingID: "sha256:aaa", SurfaceID: "surface-a", File: "a.go", Line: 1},
		})

		_, err := LoadAll(dir)
		if err == nil {
			t.Fatal("expected error for declared surface with no findings")
		}
		if !strings.Contains(err.Error(), "declared in manifest but no findings") {
			t.Errorf("error should mention missing surface: %v", err)
		}
	})

	t.Run("loaded_surface_not_in_manifest", func(t *testing.T) {
		dir := t.TempDir()
		manifest := &Manifest{
			SchemaVersion:  SchemaVersion,
			BaselineCommit: "abc123",
			Generator:      "test",
			Shards:         []string{"surface-a.jsonl"},
			SurfaceIDs:     []string{"surface-a"}, // surface-b not declared
		}
		writeTestManifest(t, dir, manifest)

		// Write findings for both declared surface and undeclared surface
		// surface-a is declared and has findings; surface-b is not declared
		writeTestShard(t, dir, "surface-a.jsonl", []Finding{
			{FindingID: "sha256:aaa", SurfaceID: "surface-a", File: "a.go", Line: 1},
			{FindingID: "sha256:bbb", SurfaceID: "surface-b", File: "b.go", Line: 1}, // undeclared!
		})

		_, err := LoadAll(dir)
		if err == nil {
			t.Fatal("expected error for undeclared surface")
		}
		if !strings.Contains(err.Error(), "not declared in manifest") {
			t.Errorf("error should mention undeclared surface: %v", err)
		}
	})

	t.Run("one_surface_multiple_shards", func(t *testing.T) {
		dir := t.TempDir()
		manifest := &Manifest{
			SchemaVersion:  SchemaVersion,
			BaselineCommit: "abc123",
			Generator:      "test",
			Shards:         []string{"surface-a-part1.jsonl", "surface-a-part2.jsonl"},
			SurfaceIDs:     []string{"surface-a"},
		}
		writeTestManifest(t, dir, manifest)

		// Split surface-a findings across two shards
		writeTestShard(t, dir, "surface-a-part1.jsonl", []Finding{
			{FindingID: "sha256:aaa", SurfaceID: "surface-a", File: "a.go", Line: 1},
			{FindingID: "sha256:bbb", SurfaceID: "surface-a", File: "a.go", Line: 2},
		})
		writeTestShard(t, dir, "surface-a-part2.jsonl", []Finding{
			{FindingID: "sha256:ccc", SurfaceID: "surface-a", File: "b.go", Line: 1},
		})

		findings, err := LoadAll(dir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(findings) != 3 {
			t.Errorf("expected 3 findings, got %d", len(findings))
		}
	})
}

func TestValidateShardName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"valid.jsonl", false},
		{"path/to/valid.jsonl", false},
		{"", true},
		{"noextension.txt", true},
		{"../../../etc/passwd", true},
		{"./valid.jsonl", true},  // Clean removes ./
		{"valid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateShardName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateShardName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

// errorsAs is a helper to check error type.
func errorsAs(err error, target **LoadError) bool {
	if le, ok := err.(*LoadError); ok {
		*target = le
		return true
	}
	return false
}
