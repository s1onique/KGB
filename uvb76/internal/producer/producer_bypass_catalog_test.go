package producer

import (
	"strings"
	"testing"
)

func TestBypassDetector_AllowlistIsRespected(t *testing.T) {
	cfg := BypassConfig{
		AllowlistedFiles: []string{"uvb76/internal/artifactio/atomic.go"},
		FileBindings:     FileBindingsFromContracts(DefaultContracts),
	}
	d := NewBypassDetector(cfg)
	findings, err := d.Scanner([]string{
		"uvb76/internal/artifactio/atomic.go",
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected allowlisted file to produce no findings, got %d", len(findings))
	}
}

// TestBypassDetector_IgnoresUntrackedFiles verifies files outside any contract
// are ignored (not flagged).
func TestBypassDetector_IgnoresUntrackedFiles(t *testing.T) {
	cfg := BypassConfig{
		AllowlistedFiles: DefaultAllowlistedWriterFiles,
		FileBindings:     FileBindingsFromContracts(DefaultContracts),
	}
	d := NewBypassDetector(cfg)
	findings, err := d.Scanner([]string{
		"some/unrelated/file.go",
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected no findings for unrelated file, got %d", len(findings))
	}
}

// TestCanonicalCatalog_LoadsFromRepo verifies the canonical catalog loads
// from the canonical path and the projection produces an 1:1 surface count.
func TestCanonicalCatalog_LoadsFromRepo(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	cat, err := LoadCanonicalCatalog(catalogRepoRoot)
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	m := cat.ComputeMetrics()
	if cat.Surfaces == nil {
		t.Fatal("nil surfaces")
	}
	if m.Total != len(cat.Surfaces) {
		t.Errorf("metric total %d != catalog total %d", m.Total, len(cat.Surfaces))
	}
}

// TestCanonicalCatalog_ClosesVocabularies exercises the closed-vocabulary
// validation routine.
func TestCanonicalCatalog_ClosesVocabularies(t *testing.T) {
	raw := []byte(`{
      "surfaces": [
        {
          "id": "x",
          "path": "foo/**/*.json",
          "producer": "p",
          "committed_allowed": true,
          "sensitivity": "high",
          "sanitizer": "redact_json",
          "status": "active",
          "persistence_policy": "atomic_redacted_json",
          "binary_policy": "not_applicable",
          "output_format": "json",
          "owner": "u",
          "justification": "j",
          "writer_files": ["foo/x.go"],
          "writer_symbols": ["writeFoo"]
        }
      ]
    }`)
	errs, _ := ValidateCanonicalCatalogBytes(raw)
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid catalog, got %v", errs)
	}
}

// TestCanonicalCatalog_DriftFailures verifies the required drift failures
// from the ACT closure rule are emitted for malformed catalogs.
func TestCanonicalCatalog_DriftFailures(t *testing.T) {
	raw := []byte(`{
      "surfaces": [
        {
          "id": "",
          "path": "",
          "producer": "",
          "committed_allowed": false,
          "sensitivity": "weird",
          "sanitizer": "none",
          "status": "weird",
          "persistence_policy": "weird",
          "binary_policy": "weird",
          "output_format": "weird",
          "owner": "",
          "justification": ""
        }
      ]
    }`)
	errs, _ := ValidateCanonicalCatalogBytes(raw)
	if len(errs) == 0 {
		t.Fatal("expected validation errors for malformed catalog")
	}
	joined := strings.Join(errs, "\n")
	for _, want := range []string{
		"missing surface",
		"unknown surface status",
		"sensitivity mismatch",
		"path mismatch",
		"policy mismatch",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing required drift failure containing %q in: %s", want, joined)
		}
	}
}
