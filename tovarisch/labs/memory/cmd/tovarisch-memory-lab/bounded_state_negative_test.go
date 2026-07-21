// bounded_state_negative_test.go — Bounded state invariant mutations.
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01-CORRECTION01
// §5.4: semantic mutations retain valid checksums so the
// invariant validator — not the checksum validator — fires.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// applyJSONMutation reads a JSON file, applies fn to the parsed
// object, and writes it back atomically. Used by every semantic
// mutation test below.
func applyJSONMutation(t *testing.T, path string, fn func(map[string]interface{})) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	fn(obj)
	rewritten, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatalf("remarshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(rewritten, '\n'), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// mutateAndVerify binds a fresh fixture copy, applies the mutation,
// recomputes checksums, runs the verifier, and asserts the verifier
// exits non-zero with the expected diagnostic substring in its
// output. The expected substring is taken from the bounded-canary
// state invariant errors in the production verifier.
func mutateAndVerify(t *testing.T, mutate func(boundDir string), expectDiagnostic string) {
	t.Helper()
	dst := t.TempDir()
	boundDir := copyBoundedFixture(t, dst)
	rebindFixture(t, boundDir)
	mutate(boundDir)
	recomputeChecksumsFor(t, boundDir)

	out, err := runVerifier(t, dst)
	if err == nil {
		t.Fatalf("mutation did not cause verifier failure; output:\n%s", out)
	}
	if !strings.Contains(out, expectDiagnostic) {
		t.Errorf("mutation produced wrong diagnostic.\n"+
			"expected substring: %q\nfull output:\n%s", expectDiagnostic, out)
	}
}

// TestState_BufferCapacityChange rejects a change to the
// bounded canary's final buffer_capacity (the bounded contract
// requires buffer unchanged).
func TestState_BufferCapacityChange(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "final-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["buffer_capacity"] = float64(2 * 1048576)
		})
	}, "buffer_capacity changed from 1048576 to 2097152")
}

// TestState_RetainedBlocksNonzero rejects a final state with
// retained_blocks != 0 (the bounded contract requires zero).
func TestState_RetainedBlocksNonzero(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "final-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["retained_blocks"] = float64(1)
		})
	}, "bounded: retained should be 0, got blocks=1 bytes=0")
}

// TestState_RetainedBytesNonzero rejects a final state with
// retained_bytes != 0 (the bounded contract requires zero).
func TestState_RetainedBytesNonzero(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "final-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["retained_bytes"] = float64(1)
		})
	}, "bounded: retained should be 0, got blocks=0 bytes=1")
}

// TestState_OperationCountDeltaMismatch rejects a final state
// whose operation_count delta does not equal workload.completed.
func TestState_OperationCountDeltaMismatch(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "final-canary-state.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			// Final must still be > initial but != initial + 100.
			// Use 50 so delta=50 != completed=100.
			m["operation_count"] = float64(50)
		})
	}, "operation_count_delta=50 != completed=100")
}
