// bounded_samples_negative_test.go — Bounded samples mutations.
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01-CORRECTION01
// §5.4: samples mutations test the production CSV parser and the
// sample progression / phase / availability checks. Checksum is
// recomputed so the sample-sequence validator fires (not the
// checksum validator).

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestSamples_AvailabilityValueContradiction rejects an evidence
// bundle where a sample's has_docker_memory is flipped from true to
// false while docker_memory_usage_bytes is non-zero (the parser
// rejects this inconsistency).
func TestSamples_AvailabilityValueContradiction(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		samplesPath := filepath.Join(boundDir, "samples.csv")
		// Flip has_docker_memory from true to false on the first
		// data row. The parser will reject this because
		// docker_memory_usage_bytes is non-zero and inconsistent
		// with has_docker_memory=false.
		data, err := os.ReadFile(samplesPath)
		if err != nil {
			t.Fatalf("read samples: %v", err)
		}
		rows := strings.Split(string(data), "\n")
		header := strings.Split(rows[0], ",")
		hasDockerMemIdx := -1
		for i, h := range header {
			if h == "has_docker_memory" {
				hasDockerMemIdx = i
				break
			}
		}
		if hasDockerMemIdx < 0 {
			t.Fatalf("has_docker_memory column not found")
		}
		row := strings.Split(rows[1], ",")
		row[hasDockerMemIdx] = "false"
		rows[1] = strings.Join(row, ",")
		if err := os.WriteFile(samplesPath, []byte(strings.Join(rows, "\n")), 0644); err != nil {
			t.Fatalf("write samples: %v", err)
		}
	}, "has_docker_memory=false")
}

// TestSamples_RepeatedSequence rejects an evidence bundle where
// sample sequence numbers are not strictly increasing.
func TestSamples_RepeatedSequence(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		samplesPath := filepath.Join(boundDir, "samples.csv")
		// Make the second data row have sequence=0 (same as first).
		data, err := os.ReadFile(samplesPath)
		if err != nil {
			t.Fatalf("read samples: %v", err)
		}
		rows := strings.Split(string(data), "\n")
		if len(rows) < 3 {
			t.Fatalf("expected >=3 lines, got %d", len(rows))
		}
		row := strings.Split(rows[2], ",")
		row[0] = "0"
		rows[2] = strings.Join(row, ",")
		if err := os.WriteFile(samplesPath, []byte(strings.Join(rows, "\n")), 0644); err != nil {
			t.Fatalf("write samples: %v", err)
		}
	}, "sequence")
}

// TestSamples_MissingBaselinePhase rejects an evidence bundle where
// no sample has phase "baseline". The strict production parser
// detects this as a phase regression (because the "startup"-only
// sequence violates the allowed phase ordering).
func TestSamples_MissingBaselinePhase(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		samplesPath := filepath.Join(boundDir, "samples.csv")
		// Replace every "baseline" phase with "startup" so that
		// no sample in the bundle has the baseline phase. The
		// strict production parser's phase-ordering check will
		// reject this.
		data, err := os.ReadFile(samplesPath)
		if err != nil {
			t.Fatalf("read samples: %v", err)
		}
		rows := strings.Split(string(data), "\n")
		header := strings.Split(rows[0], ",")
		phaseIdx := -1
		for i, h := range header {
			if h == "phase" {
				phaseIdx = i
				break
			}
		}
		if phaseIdx < 0 {
			t.Fatalf("phase column not found")
		}
		for i := 1; i < len(rows); i++ {
			if rows[i] == "" {
				continue
			}
			row := strings.Split(rows[i], ",")
			if row[phaseIdx] == "baseline" {
				row[phaseIdx] = "startup"
				rows[i] = strings.Join(row, ",")
			}
		}
		if err := os.WriteFile(samplesPath, []byte(strings.Join(rows, "\n")), 0644); err != nil {
			t.Fatalf("write samples: %v", err)
		}
	}, "phase regression")
}

// TestSamples_PIDInstability rejects an evidence bundle where the
// process PID changes mid-run. The strict production parser
// detects this on the first row that differs.
func TestSamples_PIDInstability(t *testing.T) {
	mutateAndVerify(t, func(boundDir string) {
		samplesPath := filepath.Join(boundDir, "samples.csv")
		// Change the PID on the second data row only.
		data, err := os.ReadFile(samplesPath)
		if err != nil {
			t.Fatalf("read samples: %v", err)
		}
		rows := strings.Split(string(data), "\n")
		if len(rows) < 3 {
			t.Fatalf("expected >=3 lines, got %d", len(rows))
		}
		header := strings.Split(rows[0], ",")
		pidIdx := -1
		for i, h := range header {
			if h == "process_pid" {
				pidIdx = i
				break
			}
		}
		if pidIdx < 0 {
			t.Fatalf("process_pid column not found")
		}
		row := strings.Split(rows[2], ",")
		// Increment the PID by 1 to simulate a process restart.
		oldPID, err := strconv.Atoi(row[pidIdx])
		if err != nil {
			t.Fatalf("parse PID: %v", err)
		}
		row[pidIdx] = strconv.Itoa(oldPID + 1)
		rows[2] = strings.Join(row, ",")
		if err := os.WriteFile(samplesPath, []byte(strings.Join(rows, "\n")), 0644); err != nil {
			t.Fatalf("write samples: %v", err)
		}
	}, "PID changed")
}
