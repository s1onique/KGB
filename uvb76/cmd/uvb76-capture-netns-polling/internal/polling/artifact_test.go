// Package polling provides typed polling logic for UVB-76 capture netns lab.
package polling

import (
	"encoding/json"
	"os"
	"testing"
)

// --- Artifact Writer Tests ---

func TestFileArtifactWriter_WriteProbeReadyArtifact(t *testing.T) {
	tmpDir := t.TempDir()
	artifactPath := tmpDir + "/probe_ready.json"

	writer := &FileArtifactWriter{ProbeReadyPath: artifactPath}
	series := &LatencySeries{RetainedSampleCount: 5, ReturnedPointCount: 10, SampleCount: 0}

	err := writer.WriteProbeReadyArtifact(series)
	if err != nil {
		t.Fatalf("WriteProbeReadyArtifact() error = %v", err)
	}

	// Verify file exists and contains valid JSON (raw LatencySeries, backward compatible)
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("Failed to read artifact file: %v", err)
	}

	var result LatencySeries
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Artifact is not valid JSON: %v", err)
	}

	// Verify LatencySeries structure
	if result.RetainedSampleCount != 5 {
		t.Errorf("Expected RetainedSampleCount=5, got %d", result.RetainedSampleCount)
	}
	if result.ReturnedPointCount != 10 {
		t.Errorf("Expected ReturnedPointCount=10, got %d", result.ReturnedPointCount)
	}
}

func TestFileArtifactWriter_WriteSpikeEventArtifact(t *testing.T) {
	tmpDir := t.TempDir()
	artifactPath := tmpDir + "/spike_event.json"

	writer := &FileArtifactWriter{SpikeEventPath: artifactPath}
	resp := &SpikesResponse{
		Count: 1,
		Spikes: []Spike{
			{EventID: "evt123", SampleTS: "2024-01-01T00:01:00Z", Reasons: []string{"http_probe_timeout"}},
		},
	}

	err := writer.WriteSpikeEventArtifact(resp)
	if err != nil {
		t.Fatalf("WriteSpikeEventArtifact() error = %v", err)
	}

	// Verify file exists and contains valid JSON
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("Failed to read artifact file: %v", err)
	}

	var result SpikesResponse
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Artifact is not valid JSON: %v", err)
	}

	if result.Count != 1 {
		t.Errorf("Expected Count=1, got %d", result.Count)
	}
	if len(result.Spikes) != 1 {
		t.Errorf("Expected 1 spike, got %d", len(result.Spikes))
	}
	if result.Spikes[0].EventID != "evt123" {
		t.Errorf("Expected EventID=evt123, got %s", result.Spikes[0].EventID)
	}
}

func TestFileArtifactWriter_WriteCaptureArtifact(t *testing.T) {
	tmpDir := t.TempDir()
	artifactPath := tmpDir + "/capture.json"

	writer := &FileArtifactWriter{CapturePath: artifactPath}
	resp := &SpikesResponse{
		Count: 1,
		Spikes: []Spike{
			{
				EventID:  "evt123",
				SampleTS: "2024-01-01T00:01:00Z",
				Reasons:  []string{"http_probe_timeout"},
				Captures: []Capture{
					{CaptureStatus: "captured", Status: "ok", CaptureStartedAt: "2024-01-01T00:01:00Z"},
				},
			},
		},
	}

	err := writer.WriteCaptureArtifact(resp)
	if err != nil {
		t.Fatalf("WriteCaptureArtifact() error = %v", err)
	}

	// Verify file exists and contains valid JSON
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("Failed to read artifact file: %v", err)
	}

	var result SpikesResponse
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Artifact is not valid JSON: %v", err)
	}

	if len(result.Spikes) != 1 {
		t.Errorf("Expected 1 spike, got %d", len(result.Spikes))
	}
	if len(result.Spikes[0].Captures) != 1 {
		t.Errorf("Expected 1 capture, got %d", len(result.Spikes[0].Captures))
	}
	if result.Spikes[0].Captures[0].CaptureStatus != "captured" {
		t.Errorf("Expected CaptureStatus=captured, got %s", result.Spikes[0].Captures[0].CaptureStatus)
	}
}

func TestFileArtifactWriter_WriteError(t *testing.T) {
	// Test that write errors are propagated
	writer := &FileArtifactWriter{ProbeReadyPath: "/nonexistent/path/probe_ready.json"}
	series := &LatencySeries{RetainedSampleCount: 5, ReturnedPointCount: 10, SampleCount: 0}

	err := writer.WriteProbeReadyArtifact(series)
	if err == nil {
		t.Error("Expected error when writing to nonexistent path, got nil")
	}
}

func TestFileArtifactWriter_NilInput(t *testing.T) {
	tmpDir := t.TempDir()
	writer := &FileArtifactWriter{
		ProbeReadyPath: tmpDir + "/probe_ready.json",
		SpikeEventPath: tmpDir + "/spike.json",
		CapturePath:   tmpDir + "/capture.json",
	}

	// nil inputs should return nil (no-op)
	if err := writer.WriteProbeReadyArtifact(nil); err != nil {
		t.Errorf("WriteProbeReadyArtifact(nil) = %v, want nil", err)
	}
	if err := writer.WriteSpikeEventArtifact(nil); err != nil {
		t.Errorf("WriteSpikeEventArtifact(nil) = %v, want nil", err)
	}
	if err := writer.WriteCaptureArtifact(nil); err != nil {
		t.Errorf("WriteCaptureArtifact(nil) = %v, want nil", err)
	}

	// Empty paths should also be no-op
	writer.ProbeReadyPath = ""
	writer.SpikeEventPath = ""
	writer.CapturePath = ""

	if err := writer.WriteProbeReadyArtifact(&LatencySeries{}); err != nil {
		t.Errorf("WriteProbeReadyArtifact with empty path = %v, want nil", err)
	}
}
