// attribution_test.go — Tests for attribution functionality

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAttributionConfigDefaults(t *testing.T) {
	cfg := DefaultAttributionConfig()
	if cfg.DurationSeconds != 600 {
		t.Errorf("Default AttributionConfig.DurationSeconds = %d, want 600", cfg.DurationSeconds)
	}
	if cfg.SampleIntervalMs != 5000 {
		t.Errorf("Default AttributionConfig.SampleIntervalMs = %d, want 5000", cfg.SampleIntervalMs)
	}
}

func TestAttributionConfigForSoak(t *testing.T) {
	tests := []struct {
		minutes int
		want    int
	}{
		{30, 1800},
		{45, 2700},
		{60, 3600},
	}

	for _, tt := range tests {
		cfg := AttributionConfigForSoak(tt.minutes)
		if cfg.DurationSeconds != tt.want {
			t.Errorf("AttributionConfigForSoak(%d).DurationSeconds = %d, want %d", tt.minutes, cfg.DurationSeconds, tt.want)
		}
		if cfg.SampleIntervalMs != 10000 {
			t.Errorf("AttributionConfigForSoak(%d).SampleIntervalMs = %d, want 10000", tt.minutes, cfg.SampleIntervalMs)
		}
	}
}

func TestCheckpointPhaseConstants(t *testing.T) {
	if CheckpointStart != "start" {
		t.Errorf("CheckpointStart = %q, want \"start\"", CheckpointStart)
	}
	if CheckpointMidpoint != "midpoint" {
		t.Errorf("CheckpointMidpoint = %q, want \"midpoint\"", CheckpointMidpoint)
	}
	if CheckpointEnd != "end" {
		t.Errorf("CheckpointEnd = %q, want \"end\"", CheckpointEnd)
	}
}

func TestAttributionSamplerSamples(t *testing.T) {
	sampler := NewAttributionSampler(12345, 100)
	if sampler == nil {
		t.Fatal("NewAttributionSampler returned nil")
	}
	if sampler.pid != 12345 {
		t.Errorf("sampler.pid = %d, want 12345", sampler.pid)
	}
	if sampler.sampleInt.Milliseconds() != 100 {
		t.Errorf("sampler.sampleInt = %v, want 100ms", sampler.sampleInt)
	}

	// Samples should be empty initially
	samples := sampler.Samples()
	if len(samples) != 0 {
		t.Errorf("len(sampler.Samples()) = %d, want 0", len(samples))
	}
}

func TestAttributionArtifactDir(t *testing.T) {
	tests := []struct {
		name       string
		cfg        RunConfig
		wantPrefix string
	}{
		{
			name: "custom dir",
			cfg: RunConfig{
				Service:      "uvb76",
				ArtifactDir: "/custom/path",
			},
			wantPrefix: "/custom/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := attributionArtifactDir(tt.cfg)
			if len(got) < len(tt.wantPrefix) {
				t.Errorf("attributionArtifactDir() = %q, want prefix %q", got, tt.wantPrefix)
			}
		})
	}
}

func TestWriteMemStatsCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()

	cp := &MemStatsCheckpoint{
		Timestamp:  "2024-01-01T00:00:00Z",
		PID:        12345,
		Phase:      "start",
		ForcedGC:   true,
		SampleRSS:  10000,
		SamplePSS:  9000,
		Goroutines: 25,
		HeapAlloc:  4194304,
		HeapInuse:  4194304,
		HeapObjects: 100,
		HeapSys:    8388608,
		Sys:        10000000,
	}

	err := WriteMemStatsCheckpoint(cp, tmpDir)
	if err != nil {
		t.Fatalf("WriteMemStatsCheckpoint error: %v", err)
	}

	// Verify file was created
	path := filepath.Join(tmpDir, "memstats-start.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Expected memstats-start.json to exist at %s", path)
	}
}

func TestWriteRSSSamples(t *testing.T) {
	tmpDir := t.TempDir()

	samples := []RSSSample{
		{Timestamp: "2024-01-01T00:00:00Z", ElapsedMs: 0, RSSKiB: 10000, PSSKiB: 9000},
		{Timestamp: "2024-01-01T00:01:00Z", ElapsedMs: 60000, RSSKiB: 11000, PSSKiB: 10000},
		{Timestamp: "2024-01-01T00:02:00Z", ElapsedMs: 120000, RSSKiB: 12000, PSSKiB: 11000},
	}

	err := WriteRSSSamples(samples, tmpDir)
	if err != nil {
		t.Fatalf("WriteRSSSamples error: %v", err)
	}

	// Verify file was created
	path := filepath.Join(tmpDir, "rss-pss.tsv")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Expected rss-pss.tsv to exist at %s", path)
	}

	// Read and verify content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	content := string(data)

	// Check header
	if !contains(content, "timestamp\telapsed_ms\trss_kib\tpss_kib") {
		t.Errorf("TSV missing header")
	}
	// Check sample data
	if !contains(content, "2024-01-01T00:00:00Z\t0\t10000\t9000") {
		t.Errorf("TSV missing first sample")
	}
}

func TestWriteManifest(t *testing.T) {
	tmpDir := t.TempDir()

	manifest := &AttributionManifest{
		SchemaVersion:       "1.0",
		RunTimestamp:        "2024-01-01T00:00:00Z",
		GitCommit:           "abc1234",
		UVB76Version:        "1.0.0",
		ConfiguredDurationS: 600,
		SampleIntervalMs:    5000,
		Checkpoints: []CheckpointInfo{
			{Phase: "start", Timestamp: "2024-01-01T00:00:00Z"},
			{Phase: "midpoint", Timestamp: "2024-01-01T00:05:00Z"},
			{Phase: "end", Timestamp: "2024-01-01T00:10:00Z"},
		},
		PID:          12345,
		PSSAvailable: true,
		Service:      "uvb76",
		WorkloadType: "uvb76-attribution",
	}

	err := WriteManifest(manifest, tmpDir)
	if err != nil {
		t.Fatalf("WriteManifest error: %v", err)
	}

	// Verify file was created
	path := filepath.Join(tmpDir, "manifest.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Expected manifest.yaml to exist at %s", path)
	}

	// Read and verify content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	content := string(data)

	// Check key fields
	if !contains(content, "schema_version: \"1.0\"") {
		t.Errorf("Manifest missing schema_version")
	}
	if !contains(content, "pid: 12345") {
		t.Errorf("Manifest missing PID")
	}
	if !contains(content, "pss_available: true") {
		t.Errorf("Manifest missing pss_available")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
