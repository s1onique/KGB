// artifact.go — Typed memory lab artifact writer
//
// Produces real_evidence JSON artifacts with typed fields.
// No shell heredoc, no string concatenation.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Artifact represents a complete memory lab result artifact.
type Artifact struct {
	SchemaVersion string          `json:"schema_version"`
	EvidenceKind  string          `json:"evidence_kind"`
	Service       ServiceInfo     `json:"service"`
	Environment   EnvironmentInfo `json:"environment"`
	Workload      WorkloadInfo    `json:"workload"`
	Memory        MemoryInfo      `json:"memory"`
	Runtime       RuntimeInfo     `json:"runtime,omitempty"`
	Decision      Decision        `json:"decision"`
}

// ServiceInfo captures service identity.
type ServiceInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// EnvironmentInfo captures runtime environment.
type EnvironmentInfo struct {
	Arch            string `json:"arch"`
	Kernel          string `json:"kernel"`
	OS              string `json:"os"`
	HasSmapsRollup  bool   `json:"has_smaps_rollup"`
}

// WorkloadInfo describes the workload executed.
type WorkloadInfo struct {
	Type            string `json:"type"`
	Operations      int    `json:"operations"`
	Errors          int    `json:"errors"`
	DurationMs      int64  `json:"duration_ms"`
	IntervalMs      int    `json:"interval_ms,omitempty"`
	Endpoint        string `json:"endpoint,omitempty"`
	WarmupSeconds   int    `json:"warmup_seconds,omitempty"`
	Description     string `json:"description"`
}

// MemoryInfo contains memory snapshots and growth calculations.
type MemoryInfo struct {
	First MemorySnapshot `json:"first"`
	Max   MemorySnapshot `json:"max"`
	Last  MemorySnapshot `json:"last"`
	Growth MemoryGrowth  `json:"growth"`
}

// MemoryGrowth represents calculated memory growth.
type MemoryGrowth struct {
	RSSKiB     int64   `json:"rss_kib"`
	PSSKiB     int64   `json:"pss_kib"`
	RSSPercent float64 `json:"rss_percent"`
}

// RuntimeInfo captures runtime-specific stats (Go for uvb76).
type RuntimeInfo struct {
	Allocator string `json:"allocator,omitempty"`
	// Go runtime fields
	Goroutines      int64 `json:"goroutines,omitempty"`
	HeapAllocBytes  int64 `json:"heap_alloc_bytes,omitempty"`
	GCCount         int64 `json:"gc_count,omitempty"`
	GCPauseNs       int64 `json:"gc_pause_ns,omitempty"`
}

// Decision contains the pass/fail decision and reason.
type Decision struct {
	Pass   bool   `json:"pass"`
	Reason string `json:"reason"`
}

// NewArtifact creates a new artifact with common fields populated.
func NewArtifact(service ServiceInfo, workload WorkloadInfo, env EnvironmentInfo) *Artifact {
	return &Artifact{
		SchemaVersion: "1.0",
		EvidenceKind:  "real_evidence",
		Service:       service,
		Environment:   env,
		Workload:      workload,
	}
}

// SetMemory sets memory snapshots and calculates growth.
func (a *Artifact) SetMemory(first, last MemorySnapshot, maxRSS, maxPSS int64) {
	a.Memory = MemoryInfo{
		First: first,
		Max:   MemorySnapshot{RSSKiB: maxRSS, PSSKiB: maxPSS},
		Last:  last,
		Growth: MemoryGrowth{
			RSSKiB:     last.RSSKiB - first.RSSKiB,
			PSSKiB:     last.PSSKiB - first.PSSKiB,
			RSSPercent: calculateGrowthPercent(first.RSSKiB, last.RSSKiB),
		},
	}
}

// SetDecision sets the decision based on budget check.
func (a *Artifact) SetDecision(pass bool, reason string) {
	a.Decision = Decision{Pass: pass, Reason: reason}
}

// calculateGrowthPercent computes growth percentage.
func calculateGrowthPercent(first, last int64) float64 {
	if first == 0 {
		return 0.0
	}
	growth := last - first
	return float64(growth) * 100.0 / float64(first)
}

// Write writes the artifact to a JSON file.
func (a *Artifact) Write(dir, service, workloadType string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("artifact dir: %w", err)
	}

	timestamp := time.Now().Format("20060102T150405")
	filename := fmt.Sprintf("%s-%s-%s.json", service, workloadType, timestamp)
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return "", fmt.Errorf("json marshal: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return path, nil
}

// WriteStdout writes the artifact to stdout (for debugging).
func (a *Artifact) WriteStdout() error {
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}