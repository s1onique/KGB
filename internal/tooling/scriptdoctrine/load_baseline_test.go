package scriptdoctrine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// LoadBaseline tests
// =============================================================================

func TestLoadBaselineRequiresProvenance(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("missing baseline_commit", func(t *testing.T) {
		content := `# Bootstrap baseline
# loc_algorithm=logical-shell-v1
scripts/test.sh,10,0
`
		f := filepath.Join(tmpDir, "missing_commit.csv")
		if err := os.WriteFile(f, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadBaseline(f)
		if err == nil || !strings.Contains(err.Error(), "baseline_commit") {
			t.Errorf("expected baseline_commit error, got: %v", err)
		}
	})

	t.Run("missing loc_algorithm", func(t *testing.T) {
		content := `# Bootstrap baseline
# baseline_commit=50e6975e2a599c99dd8825e8557edec677d60406
scripts/test.sh,10,0
`
		f := filepath.Join(tmpDir, "missing_algo.csv")
		if err := os.WriteFile(f, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadBaseline(f)
		if err == nil || !strings.Contains(err.Error(), "loc_algorithm") {
			t.Errorf("expected loc_algorithm error, got: %v", err)
		}
	})

	t.Run("invalid commit hash", func(t *testing.T) {
		content := `# Bootstrap baseline
# baseline_commit=NOT-A-HASH
# loc_algorithm=logical-shell-v1
scripts/test.sh,10,0
`
		f := filepath.Join(tmpDir, "bad_commit.csv")
		if err := os.WriteFile(f, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadBaseline(f)
		if err == nil || !strings.Contains(err.Error(), "invalid baseline_commit") {
			t.Errorf("expected invalid baseline_commit error, got: %v", err)
		}
	})

	t.Run("unsupported algorithm", func(t *testing.T) {
		content := `# Bootstrap baseline
# baseline_commit=50e6975e2a599c99dd8825e8557edec677d60406
# loc_algorithm=logical-shell-v99
scripts/test.sh,10,0
`
		f := filepath.Join(tmpDir, "bad_algo.csv")
		if err := os.WriteFile(f, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadBaseline(f)
		if err == nil || !strings.Contains(err.Error(), "unsupported loc_algorithm") {
			t.Errorf("expected unsupported loc_algorithm error, got: %v", err)
		}
	})

	t.Run("valid baseline", func(t *testing.T) {
		content := `# Bootstrap baseline
# baseline_commit=50e6975e2a599c99dd8825e8557edec677d60406
# loc_algorithm=logical-shell-v1
scripts/test.sh,10,2
`
		f := filepath.Join(tmpDir, "valid.csv")
		if err := os.WriteFile(f, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		entries, err := LoadBaseline(f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 entry, got %d", len(entries))
		}
	})
}

func TestLoadBaselineDetectsDuplicatePaths(t *testing.T) {
	tmpDir := t.TempDir()
	content := `# Bootstrap baseline
# baseline_commit=50e6975e2a599c99dd8825e8557edec677d60406
# loc_algorithm=logical-shell-v1
scripts/test.sh,10,0
scripts/test.sh,11,1
`
	f := filepath.Join(tmpDir, "dupe.csv")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadBaseline(f)
	if err == nil || !strings.Contains(err.Error(), "duplicate path") {
		t.Errorf("expected duplicate path error, got: %v", err)
	}
}

// =============================================================================
// LoadInventory tests
// =============================================================================

func TestLoadInventory(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid inventory", func(t *testing.T) {
		content := `id,path,language,logical_loc,role,public_interface,target_go_command,status,notes
S001,scripts/test.sh,shell,10,verifier,,cmd/test,migration-required,Test entry
`
		tmpFile := filepath.Join(tmpDir, "valid.csv")
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		entries, err := LoadInventory(tmpFile)
		if err != nil {
			t.Fatalf("LoadInventory() error = %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 entry, got %d", len(entries))
		}
	})

	t.Run("duplicate path", func(t *testing.T) {
		content := `id,path,language,logical_loc,role,public_interface,target_go_command,status,notes
S001,scripts/test.sh,shell,10,verifier,,cmd/test,migration-required,First
S002,scripts/test.sh,shell,10,verifier,,cmd/test,migration-required,Duplicate
`
		tmpFile := filepath.Join(tmpDir, "duplicate.csv")
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		_, err := LoadInventory(tmpFile)
		if err == nil {
			t.Error("expected error for duplicate path")
		}
	})

	t.Run("invalid language", func(t *testing.T) {
		content := `id,path,language,logical_loc,role,public_interface,target_go_command,status,notes
S001,scripts/test.sh,invalid_lang,10,verifier,,cmd/test,migration-required,Bad language
`
		tmpFile := filepath.Join(tmpDir, "badlang.csv")
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		_, err := LoadInventory(tmpFile)
		if err == nil {
			t.Error("expected error for invalid language")
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		content := `id,path,language,logical_loc,role,public_interface,target_go_command,status,notes
S001,scripts/test.sh,shell,10,verifier,,cmd/test,invalid_status,Bad status
`
		tmpFile := filepath.Join(tmpDir, "badstatus.csv")
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		_, err := LoadInventory(tmpFile)
		if err == nil {
			t.Error("expected error for invalid status")
		}
	})

	t.Run("absolute path rejected", func(t *testing.T) {
		content := `id,path,language,logical_loc,role,public_interface,target_go_command,status,notes
S001,/absolute/path.sh,shell,10,verifier,,cmd/test,migration-required,Absolute path
`
		tmpFile := filepath.Join(tmpDir, "absolutepath.csv")
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		_, err := LoadInventory(tmpFile)
		if err == nil {
			t.Error("expected error for absolute path")
		}
	})

	t.Run("nonexistent file returns error (fail-closed)", func(t *testing.T) {
		entries, err := LoadInventory(filepath.Join(tmpDir, "nonexistent.csv"))
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
		if entries != nil {
			t.Errorf("expected nil for nonexistent file, got %v", entries)
		}
	})
}
