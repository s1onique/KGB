// bounded_workload_negative_test.go — Bounded workload arithmetic mutations.
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01-CORRECTION01
// §5.4: semantic mutations retain valid checksums; the workload
// arithmetic check is the one that fires.

package main

import (
	"path/filepath"
	"testing"
)

// TestWorkload_CompletedNotEqualRequested rejects a workload whose
// `completed` field does not equal `requested`.
func TestWorkload_CompletedNotEqualRequested(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "workload-result.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["completed"] = float64(99)
		})
	}, "workload counts: req=100 att=100 com=99 fail=0")
}

// TestWorkload_ReturnedNotEqualCompleted rejects a workload whose
// `returned` field does not equal `completed` (the bounded contract
// requires the producer to persist the observed completed count as
// the returned count).
func TestWorkload_ReturnedNotEqualCompleted(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "workload-result.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["returned"] = float64(99)
		})
	}, "workload returned=99 != completed=100")
}

// TestWorkload_AttemptedNotEqualRequested rejects a workload whose
// `attempted` field does not equal `requested`.
func TestWorkload_AttemptedNotEqualRequested(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "workload-result.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["attempted"] = float64(99)
		})
	}, "workload counts: req=100 att=99 com=100 fail=0")
}

// TestWorkload_FailedNonzero rejects a workload with failed != 0.
func TestWorkload_FailedNonzero(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		path := filepath.Join(boundDir, "workload-result.json")
		applyJSONMutation(t, path, func(m map[string]interface{}) {
			m["failed"] = float64(1)
			m["completed"] = float64(99)
		})
	}, "workload counts: req=100 att=100 com=99 fail=1")
}
