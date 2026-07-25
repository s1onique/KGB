package evidence

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestControllerProvenance_EmptyPolicyRejected(t *testing.T) {
	if err := ValidateProvenanceCleanPolicy(""); !errors.Is(err, ErrUnknownProvenanceCleanPolicy) {
		t.Fatalf("empty policy error=%v", err)
	}
}

func TestControllerProvenance_UnknownPolicyRejected(t *testing.T) {
	if err := ValidateProvenanceCleanPolicy(ProvenanceCleanPolicy("unknown")); !errors.Is(err, ErrUnknownProvenanceCleanPolicy) {
		t.Fatalf("unknown policy error=%v", err)
	}
}

func cleanSyntheticProvenance(policy ProvenanceCleanPolicy) ControllerProvenance {
	return ControllerProvenance{VCSRevision: strings.Repeat("a", 40), CleanPolicy: policy}
}

func TestControllerProvenance_RequireCleanAcceptsClean(t *testing.T) {
	cp := cleanSyntheticProvenance(ProvenanceRequireClean)
	if err := authorizeProvenance(&cp, cp.VCSRevision); err != nil {
		t.Fatalf("clean provenance rejected: %v", err)
	}
	if !cp.QualifyingObservation {
		t.Fatal("clean provenance was not qualifying")
	}
}

func TestControllerProvenance_RequireCleanRejectsTracked(t *testing.T) {
	cp := cleanSyntheticProvenance(ProvenanceRequireClean)
	cp.TrackedModified = true
	if err := authorizeProvenance(&cp, cp.VCSRevision); !errors.Is(err, ErrProvenanceDirty) {
		t.Fatalf("tracked error=%v", err)
	}
}
func TestControllerProvenance_RequireCleanRejectsStaged(t *testing.T) {
	cp := cleanSyntheticProvenance(ProvenanceRequireClean)
	cp.StagedModified = true
	if err := authorizeProvenance(&cp, cp.VCSRevision); !errors.Is(err, ErrProvenanceDirty) {
		t.Fatalf("staged error=%v", err)
	}
}
func TestControllerProvenance_RequireCleanRejectsUntracked(t *testing.T) {
	cp := cleanSyntheticProvenance(ProvenanceRequireClean)
	cp.UntrackedFiles = true
	if err := authorizeProvenance(&cp, cp.VCSRevision); !errors.Is(err, ErrProvenanceDirty) {
		t.Fatalf("untracked error=%v", err)
	}
}
func TestControllerProvenance_RequireCleanRejectsVCSModified(t *testing.T) {
	cp := cleanSyntheticProvenance(ProvenanceRequireClean)
	cp.VCSModified = true
	if err := authorizeProvenance(&cp, cp.VCSRevision); !errors.Is(err, ErrProvenanceDirty) {
		t.Fatalf("vcs.modified error=%v", err)
	}
}

func TestControllerProvenance_IgnoreWorktreeNonQualifying(t *testing.T) {
	cp := cleanSyntheticProvenance(ProvenanceIgnoreWorktree)
	cp.WorkingTreeDirty = true
	if err := authorizeProvenance(&cp, cp.VCSRevision); err != nil {
		t.Fatalf("ignore policy rejected: %v", err)
	}
	if cp.QualifyingObservation {
		t.Fatal("ignore_worktree authorized evidence")
	}
	final := validFinalProvenance()
	final.CleanPolicy = ProvenanceIgnoreWorktree
	final.QualifyingObservation = false
	if err := validateFinalControllerProvenance(final); !errors.Is(err, ErrFinalQualifiedProvenanceIncomplete) {
		t.Fatalf("ignore_worktree reached final evidence: %v", err)
	}
}

func TestProductionCLI_UsesRequireClean(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "cmd", "tovarisch-memory-lab", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "CleanPolicy:") || !strings.Contains(string(data), "evidence.ProvenanceRequireClean") {
		t.Fatal("production CLI lacks require_clean policy")
	}
}

func TestLiveHelper_UsesRequireClean(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "cmd", "tovarisch-memory-lab", "qualified_live_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "CleanPolicy:") || !strings.Contains(string(data), "evidence.ProvenanceRequireClean") {
		t.Fatal("live helper lacks require_clean policy")
	}
}

func TestProductionCallersConstructPolicyExplicitly(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "cmd", "tovarisch-memory-lab", "main.go"),
		filepath.Join("..", "..", "cmd", "tovarisch-memory-lab", "qualified_live_test.go"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for offset := 0; ; {
			i := strings.Index(text[offset:], "evidence.ProvenanceOptions{")
			if i < 0 {
				break
			}
			i += offset
			end := strings.Index(text[i:], "}")
			if end < 0 {
				t.Fatalf("unterminated options in %s", path)
			}
			block := text[i : i+end]
			if !strings.Contains(block, "CleanPolicy:") {
				t.Fatalf("caller at %s lacks CleanPolicy", path)
			}
			offset = i + end
		}
	}
}
