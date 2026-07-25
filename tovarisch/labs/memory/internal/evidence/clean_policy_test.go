// clean_policy_test.go — ProvenanceCleanPolicy validation tests.
//
// These tests cover the typed-policy selector, the policy-aware
// defaults, and the relationship between the legacy RequireClean
// bool and the new CleanPolicy field. The integration tests that
// hit real `git status` / `git diff` live in controller_provenance_test.go
// (a companion file in this package) and they use a test-local
// git fixture under t.TempDir().

package evidence

import (
	"errors"
	"testing"
)

func TestControllerProvenance_UnknownPolicyRejected(t *testing.T) {
	cases := []struct {
		name string
		in   ProvenanceCleanPolicy
	}{
		{"require", ProvenanceCleanPolicy("require")},
		{"empty", ProvenanceCleanPolicy("")},
		{"unknown", ProvenanceCleanPolicy("anything-else")},
		{"misspelled", ProvenanceCleanPolicy("requir_clean")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateProvenanceCleanPolicy(tc.in); err == nil {
				t.Fatalf("expected %q to be rejected", tc.in)
			} else if !errors.Is(err, ErrUnknownProvenanceCleanPolicy) {
				t.Fatalf("expected ErrUnknownProvenanceCleanPolicy, got %v", err)
			}
		})
	}
	t.Run("require_clean", func(t *testing.T) {
		if err := ValidateProvenanceCleanPolicy(ProvenanceRequireClean); err != nil {
			t.Fatalf("require_clean should validate: %v", err)
		}
	})
	t.Run("ignore_worktree", func(t *testing.T) {
		if err := ValidateProvenanceCleanPolicy(ProvenanceIgnoreWorktree); err != nil {
			t.Fatalf("ignore_worktree should validate: %v", err)
		}
	})
}

func TestControllerProvenance_LegacyRequireCleanMapping(t *testing.T) {
	t.Run("RequireClean=true -> ProvenanceRequireClean", func(t *testing.T) {
		got, err := resolveCleanPolicy(ProvenanceOptions{RequireClean: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != ProvenanceRequireClean {
			t.Fatalf("expected ProvenanceRequireClean, got %q", got)
		}
	})
	t.Run("RequireClean=false -> ProvenanceIgnoreWorktree", func(t *testing.T) {
		got, err := resolveCleanPolicy(ProvenanceOptions{RequireClean: false})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != ProvenanceIgnoreWorktree {
			t.Fatalf("expected ProvenanceIgnoreWorktree, got %q", got)
		}
	})
}

func TestControllerProvenance_RequireCleanInconsistentLegacyFallback(t *testing.T) {
	_, err := resolveCleanPolicy(ProvenanceOptions{
		CleanPolicy:  ProvenanceRequireClean,
		RequireClean: false,
	})
	if !errors.Is(err, ErrInconsistentCleanPolicy) {
		t.Fatalf("expected ErrInconsistentCleanPolicy, got %v", err)
	}
	_, err = resolveCleanPolicy(ProvenanceOptions{
		CleanPolicy:  ProvenanceIgnoreWorktree,
		RequireClean: true,
	})
	if !errors.Is(err, ErrInconsistentCleanPolicy) {
		t.Fatalf("expected ErrInconsistentCleanPolicy on reverse disagreement, got %v", err)
	}
}

func TestControllerProvenance_ConsistentFieldsAccepted(t *testing.T) {
	t.Run("explicit require_clean + RequireClean=true", func(t *testing.T) {
		got, err := resolveCleanPolicy(ProvenanceOptions{CleanPolicy: ProvenanceRequireClean, RequireClean: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != ProvenanceRequireClean {
			t.Fatalf("expected ProvenanceRequireClean, got %q", got)
		}
	})
	t.Run("explicit ignore_worktree + RequireClean=false", func(t *testing.T) {
		got, err := resolveCleanPolicy(ProvenanceOptions{CleanPolicy: ProvenanceIgnoreWorktree, RequireClean: false})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != ProvenanceIgnoreWorktree {
			t.Fatalf("expected ProvenanceIgnoreWorktree, got %q", got)
		}
	})
}
