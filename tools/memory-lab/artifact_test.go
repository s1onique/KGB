// artifact_test.go — Tests for typed artifact writer and growth calculation

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewArtifact(t *testing.T) {
	service := ServiceInfo{Name: "test", Version: "1.0.0", Commit: "abc123"}
	workload := WorkloadInfo{Type: "idle-warmup", Operations: 0, Errors: 0, DurationMs: 60000}
	env := EnvironmentInfo{Arch: "linux/arm64", OS: "Linux", HasSmapsRollup: true}

	artifact := NewArtifact(service, workload, env)

	if artifact.SchemaVersion != "1.0" {
		t.Errorf("SchemaVersion = %q, want 1.0", artifact.SchemaVersion)
	}
	if artifact.EvidenceKind != "real_evidence" {
		t.Errorf("EvidenceKind = %q, want real_evidence", artifact.EvidenceKind)
	}
	if artifact.Service.Name != "test" {
		t.Errorf("Service.Name = %q, want test", artifact.Service.Name)
	}
}

func TestSetMemory(t *testing.T) {
	artifact := &Artifact{}
	first := MemorySnapshot{RSSKiB: 1000, PSSKiB: 900}
	last := MemorySnapshot{RSSKiB: 1100, PSSKiB: 980}

	artifact.SetMemory(first, last, 1150, 1000)

	if artifact.Memory.First != first {
		t.Errorf("Memory.First = %v, want %v", artifact.Memory.First, first)
	}
	if artifact.Memory.Last != last {
		t.Errorf("Memory.Last = %v, want %v", artifact.Memory.Last, last)
	}
	if artifact.Memory.Max.RSSKiB != 1150 {
		t.Errorf("Memory.Max.RSSKiB = %d, want 1150", artifact.Memory.Max.RSSKiB)
	}
	if artifact.Memory.Max.PSSKiB != 1000 {
		t.Errorf("Memory.Max.PSSKiB = %d, want 1000", artifact.Memory.Max.PSSKiB)
	}
	if artifact.Memory.Growth.RSSKiB != 100 {
		t.Errorf("Memory.Growth.RSSKiB = %d, want 100", artifact.Memory.Growth.RSSKiB)
	}
	if artifact.Memory.Growth.PSSKiB != 80 {
		t.Errorf("Memory.Growth.PSSKiB = %d, want 80", artifact.Memory.Growth.PSSKiB)
	}
}

func TestSetMemoryGrowthPercent(t *testing.T) {
	tests := []struct {
		name     string
		first    int64
		last     int64
		expected float64
	}{
		{"10% growth", 1000, 1100, 10.0},
		{"no growth", 1000, 1000, 0.0},
		{"50% growth", 1000, 1500, 50.0},
		{"1% growth", 10000, 10100, 1.0},
		{"zero first", 0, 100, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifact := &Artifact{}
			artifact.SetMemory(
				MemorySnapshot{RSSKiB: tt.first, PSSKiB: 0},
				MemorySnapshot{RSSKiB: tt.last, PSSKiB: 0},
				tt.last, 0,
			)
			if artifact.Memory.Growth.RSSPercent != tt.expected {
				t.Errorf("Growth.RSSPercent = %f, want %f", artifact.Memory.Growth.RSSPercent, tt.expected)
			}
		})
	}
}

func TestSetDecision(t *testing.T) {
	artifact := &Artifact{}

	artifact.SetDecision(true, "within budget")
	if !artifact.Decision.Pass {
		t.Errorf("Decision.Pass = false, want true")
	}
	if artifact.Decision.Reason != "within budget" {
		t.Errorf("Decision.Reason = %q, want within budget", artifact.Decision.Reason)
	}

	artifact.SetDecision(false, "exceeds budget")
	if artifact.Decision.Pass {
		t.Errorf("Decision.Pass = true, want false")
	}
	if artifact.Decision.Reason != "exceeds budget" {
		t.Errorf("Decision.Reason = %q, want exceeds budget", artifact.Decision.Reason)
	}
}

func TestArtifactWrite(t *testing.T) {
	artifact := &Artifact{
		SchemaVersion: "1.0",
		EvidenceKind:  "real_evidence",
		Service:       ServiceInfo{Name: "test", Version: "1.0.0", Commit: "abc123"},
		Environment:   EnvironmentInfo{Arch: "linux/arm64", OS: "Linux"},
		Workload:      WorkloadInfo{Type: "idle-warmup", Operations: 0, Errors: 0, DurationMs: 60000},
		Memory: MemoryInfo{
			First:  MemorySnapshot{RSSKiB: 1000, PSSKiB: 900},
			Max:    MemorySnapshot{RSSKiB: 1100, PSSKiB: 1000},
			Last:   MemorySnapshot{RSSKiB: 1050, PSSKiB: 950},
			Growth: MemoryGrowth{RSSKiB: 50, PSSKiB: 50, RSSPercent: 5.0},
		},
		Decision: Decision{Pass: true, Reason: "test"},
	}

	tmpDir := t.TempDir()
	path, err := artifact.Write(tmpDir, "tovarisch", "idle-warmup")
	if err != nil {
		t.Fatalf("artifact.Write: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("artifact file not created: %s", path)
	}

	// Verify filename format
	filename := filepath.Base(path)
	if !strings.HasPrefix(filename, "tovarisch-idle-warmup-") {
		t.Errorf("filename = %q, want prefix tovarisch-idle-warmup-", filename)
	}
	if !strings.HasSuffix(filename, ".json") {
		t.Errorf("filename = %q, want .json suffix", filename)
	}

	// Read and parse JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}

	var parsed Artifact
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse artifact JSON: %v", err)
	}

	if parsed.EvidenceKind != "real_evidence" {
		t.Errorf("Parsed EvidenceKind = %q, want real_evidence", parsed.EvidenceKind)
	}
	if parsed.Memory.Growth.RSSKiB != 50 {
		t.Errorf("Parsed Growth.RSSKiB = %d, want 50", parsed.Memory.Growth.RSSKiB)
	}
}

func TestArtifactJSONRoundTrip(t *testing.T) {
	original := &Artifact{
		SchemaVersion: "1.0",
		EvidenceKind:  "real_evidence",
		Service:       ServiceInfo{Name: "tovarisch", Version: "0.1.0", Commit: "f231216"},
		Environment:   EnvironmentInfo{Arch: "linux/arm64", Kernel: "5.15.0", OS: "Linux", HasSmapsRollup: true},
		Workload: WorkloadInfo{
			Type:          "status-json-network-diag",
			Operations:    100,
			Errors:        0,
			DurationMs:    10500,
			IntervalMs:    100,
			Endpoint:      "/status.json?include=network_diag",
			WarmupSeconds: 60,
			Description:   "Repeated status with network_diag after 60s warmup",
		},
		Memory: MemoryInfo{
			First:  MemorySnapshot{RSSKiB: 5120, PSSKiB: 4800},
			Max:    MemorySnapshot{RSSKiB: 5248, PSSKiB: 4900},
			Last:   MemorySnapshot{RSSKiB: 5184, PSSKiB: 4850},
			Growth: MemoryGrowth{RSSKiB: 64, PSSKiB: 50, RSSPercent: 1.25},
		},
		Runtime: RuntimeInfo{Allocator: "zig-default"},
		Decision: Decision{Pass: true, Reason: "baseline_required: RSS growth 64 KiB (1.25%) recorded, budget pending measurement"},
	}

	// Marshal
	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Unmarshal
	var parsed Artifact
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify all fields
	if parsed.SchemaVersion != original.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", parsed.SchemaVersion, original.SchemaVersion)
	}
	if parsed.EvidenceKind != original.EvidenceKind {
		t.Errorf("EvidenceKind = %q, want %q", parsed.EvidenceKind, original.EvidenceKind)
	}
	if parsed.Service.Name != original.Service.Name {
		t.Errorf("Service.Name = %q, want %q", parsed.Service.Name, original.Service.Name)
	}
	if parsed.Workload.Operations != original.Workload.Operations {
		t.Errorf("Workload.Operations = %d, want %d", parsed.Workload.Operations, original.Workload.Operations)
	}
	if parsed.Memory.Growth.RSSKiB != original.Memory.Growth.RSSKiB {
		t.Errorf("Growth.RSSKiB = %d, want %d", parsed.Memory.Growth.RSSKiB, original.Memory.Growth.RSSKiB)
	}
	if parsed.Memory.Growth.PSSKiB != original.Memory.Growth.PSSKiB {
		t.Errorf("Growth.PSSKiB = %d, want %d", parsed.Memory.Growth.PSSKiB, original.Memory.Growth.PSSKiB)
	}
	if !parsed.Decision.Pass {
		t.Errorf("Decision.Pass = false, want true")
	}
}

func TestCalculateGrowthPercent(t *testing.T) {
	tests := []struct {
		first    int64
		last     int64
		expected float64
	}{
		{1000, 1100, 10.0},
		{1000, 900, -10.0},
		{1000, 1000, 0.0},
		{0, 1000, 0.0},
		{10000, 10100, 1.0},
	}

	for _, tt := range tests {
		got := calculateGrowthPercent(tt.first, tt.last)
		if got != tt.expected {
			t.Errorf("calculateGrowthPercent(%d, %d) = %f, want %f", tt.first, tt.last, got, tt.expected)
		}
	}
}
