// attribution.go — UVB-76 memory attribution lab support
//
// Provides checkpoint-based memory attribution for long-running UVB-76 memory
// attribution labs. Captures forced-GC memstats, pprof heap profiles, goroutine
// dumps, and RSS/PSS samples over time from the UVB-76 process via HTTP.
//
// CRITICAL: Checkpoint functions call UVB-76's diagnostic HTTP endpoints,
// not memory-lab's own Go runtime. This ensures evidence reflects the
// actual UVB-76 process, not the measurement tool.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// CheckpointPhase represents the phase of a checkpoint.
type CheckpointPhase string

const (
	CheckpointStart     CheckpointPhase = "start"
	CheckpointMidpoint CheckpointPhase = "midpoint"
	CheckpointEnd      CheckpointPhase = "end"
)

// MemStatsCheckpoint represents a memory stats snapshot at a checkpoint.
type MemStatsCheckpoint struct {
	Timestamp        string `json:"timestamp"`
	PID              int    `json:"pid"`
	Phase            string `json:"phase"`
	ForcedGC         bool   `json:"forced_gc"`
	SampleRSS        int64  `json:"sample_rss_kib"`
	SamplePSS        int64  `json:"sample_pss_kib"`
	Goroutines       uint64 `json:"goroutines"`
	HeapAlloc        uint64 `json:"heap_alloc_bytes"`
	HeapInuse        uint64 `json:"heap_inuse_bytes"`
	HeapObjects      uint64 `json:"heap_objects"`
	HeapSys          uint64 `json:"heap_sys_bytes"`
	Sys              uint64 `json:"sys_bytes"`
	HeapReleased     uint64 `json:"heap_released_bytes"`
	HeapIdle         uint64 `json:"heap_idle_bytes"`
	GCCount          uint32 `json:"gc_count"`
	GCPauseTotalNs   uint64 `json:"gc_pause_total_ns"`
	NextGCBytes      uint64 `json:"next_gc_bytes"`
	LastGCNs         uint64 `json:"last_gc_ns"`
	Mallocs          uint64 `json:"mallocs"`
	Frees            uint64 `json:"frees"`
}

// AttributionManifest represents the lab run manifest.
type AttributionManifest struct {
	SchemaVersion        string           `json:"schema_version"`
	RunTimestamp         string           `json:"run_timestamp"`
	GitCommit            string           `json:"git_commit"`
	UVB76Version         string           `json:"uvb76_version"`
	ConfiguredDurationS  int              `json:"configured_duration_seconds"`
	SampleIntervalMs     int              `json:"sample_interval_ms"`
	Checkpoints          []CheckpointInfo `json:"checkpoints"`
	PID                  int             `json:"pid"`
	PSSAvailable         bool             `json:"pss_available"`
	PSSFallbackUsed      bool             `json:"pss_fallback_used"`
	Service              string           `json:"service"`
	WorkloadType         string           `json:"workload_type"`
}

// CheckpointInfo records when each checkpoint was taken.
type CheckpointInfo struct {
	Phase     string `json:"phase"`
	Timestamp string `json:"timestamp"`
}

// RSSSample represents a single RSS/PSS sample.
type RSSSample struct {
	Timestamp string `json:"timestamp"`
	ElapsedMs int64  `json:"elapsed_ms"`
	RSSKiB    int64  `json:"rss_kib"`
	PSSKiB    int64  `json:"pss_kib"`
}

// uvB76MemStatsResponse represents the JSON response from UVB-76's memstats endpoint.
type uvB76MemStatsResponse struct {
	Timestamp   string `json:"timestamp"`
	PID         int    `json:"pid"`
	Goroutines  uint64 `json:"goroutines"`
	HeapAlloc   uint64 `json:"heap_alloc_bytes"`
	HeapInuse   uint64 `json:"heap_inuse_bytes"`
	HeapObjects uint64 `json:"heap_objects"`
	HeapSys     uint64 `json:"heap_sys_bytes"`
	Sys         uint64 `json:"sys_bytes"`
	HeapReleased uint64 `json:"heap_released_bytes"`
	HeapIdle    uint64 `json:"heap_idle_bytes"`
	GCCount     uint32 `json:"gc_count"`
	GCPauseNs   uint64 `json:"gc_pause_total_ns"`
	NextGC      uint64 `json:"next_gc_bytes"`
	Mallocs     uint64 `json:"mallocs"`
	Frees       uint64 `json:"frees"`
}

// CaptureMemStatsFromUVB76 fetches Go runtime stats from UVB-76 process via HTTP.
// CRITICAL: This captures from UVB-76, not from memory-lab.
// Verifies endpoint PID matches launched PID to prevent mixed-evidence bugs.
func CaptureMemStatsFromUVB76(client *http.Client, url string, pid int, phase CheckpointPhase, forceGC bool) (*MemStatsCheckpoint, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("UVB-76 memstats returned %d: %s", resp.StatusCode, string(body))
	}

	var uvbResp uvB76MemStatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&uvbResp); err != nil {
		return nil, fmt.Errorf("decode memstats response: %w", err)
	}

	// CRITICAL: Verify PID consistency - prevent mixed-evidence from port/config mix-up
	if uvbResp.PID != pid {
		return nil, fmt.Errorf("PID mismatch: endpoint returned %d, expected %d (possible port/config mix-up)", uvbResp.PID, pid)
	}

	// Read RSS/PSS from /proc for the UVB-76 PID
	sampleRSS, samplePSS := int64(0), int64(0)
	if snap, err := ReadMemorySnapshot(pid); err == nil {
		sampleRSS, samplePSS = snap.RSSKiB, snap.PSSKiB
	}

	return &MemStatsCheckpoint{
		Timestamp:        uvbResp.Timestamp,
		PID:              uvbResp.PID,
		Phase:            string(phase),
		ForcedGC:         forceGC,
		SampleRSS:        sampleRSS,
		SamplePSS:        samplePSS,
		Goroutines:       uvbResp.Goroutines,
		HeapAlloc:        uvbResp.HeapAlloc,
		HeapInuse:        uvbResp.HeapInuse,
		HeapObjects:      uvbResp.HeapObjects,
		HeapSys:          uvbResp.HeapSys,
		Sys:              uvbResp.Sys,
		HeapReleased:     uvbResp.HeapReleased,
		HeapIdle:         uvbResp.HeapIdle,
		GCCount:          uvbResp.GCCount,
		GCPauseTotalNs:   uvbResp.GCPauseNs,
		NextGCBytes:      uvbResp.NextGC,
		LastGCNs:         0, // Not exposed by endpoint
		Mallocs:          uvbResp.Mallocs,
		Frees:            uvbResp.Frees,
	}, nil
}

// DownloadHeapProfileFromUVB76 downloads pprof heap profile from UVB-76 via HTTP.
func DownloadHeapProfileFromUVB76(client *http.Client, url, dir, phase string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	filename := fmt.Sprintf("heap-%s.pprof", phase)
	path := filepath.Join(dir, filename)

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("UVB-76 heap-profile returned %d: %s", resp.StatusCode, string(body))
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}

	return nil
}

// DownloadGoroutineDumpFromUVB76 downloads goroutine stack dump from UVB-76 via HTTP.
func DownloadGoroutineDumpFromUVB76(client *http.Client, url, dir, phase string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	filename := fmt.Sprintf("goroutine-%s.txt", phase)
	path := filepath.Join(dir, filename)

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("UVB-76 goroutine-dump returned %d: %s", resp.StatusCode, string(body))
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}

	return nil
}

// WriteMemStatsCheckpoint writes a checkpoint to a JSON file.
func WriteMemStatsCheckpoint(cp *MemStatsCheckpoint, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	filename := fmt.Sprintf("memstats-%s.json", cp.Phase)
	path := filepath.Join(dir, filename)
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// WriteRSSSamples writes RSS/PSS samples to a TSV file.
func WriteRSSSamples(samples []RSSSample, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "rss-pss.tsv")

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write header
	fmt.Fprintf(f, "timestamp\telapsed_ms\trss_kib\tpss_kib\n")

	// Write samples
	for _, s := range samples {
		fmt.Fprintf(f, "%s\t%d\t%d\t%d\n", s.Timestamp, s.ElapsedMs, s.RSSKiB, s.PSSKiB)
	}

	return nil
}

// WriteManifest writes the attribution manifest to a YAML file.
func WriteManifest(manifest *AttributionManifest, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "manifest.yaml")

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Simple YAML serialization
	fmt.Fprintf(f, "# Memory Attribution Lab Manifest\n")
	fmt.Fprintf(f, "# Generated by memory-lab tool\n\n")
	fmt.Fprintf(f, "schema_version: %q\n", manifest.SchemaVersion)
	fmt.Fprintf(f, "run_timestamp: %q\n", manifest.RunTimestamp)
	fmt.Fprintf(f, "git_commit: %q\n", manifest.GitCommit)
	fmt.Fprintf(f, "uvb76_version: %q\n", manifest.UVB76Version)
	fmt.Fprintf(f, "configured_duration_seconds: %d\n", manifest.ConfiguredDurationS)
	fmt.Fprintf(f, "sample_interval_ms: %d\n", manifest.SampleIntervalMs)
	fmt.Fprintf(f, "pid: %d\n", manifest.PID)
	fmt.Fprintf(f, "pss_available: %v\n", manifest.PSSAvailable)
	fmt.Fprintf(f, "pss_fallback_used: %v\n", manifest.PSSFallbackUsed)
	fmt.Fprintf(f, "service: %q\n", manifest.Service)
	fmt.Fprintf(f, "workload_type: %q\n", manifest.WorkloadType)

	fmt.Fprintf(f, "\ncheckpoints:\n")
	for _, cp := range manifest.Checkpoints {
		fmt.Fprintf(f, "  - phase: %q\n", cp.Phase)
		fmt.Fprintf(f, "    timestamp: %q\n", cp.Timestamp)
	}

	return nil
}

// fetchStatus fetches the UVB-76 status API for attribution context.
func fetchStatus(client *http.Client, url string) (map[string]interface{}, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// AttributionConfig configures an attribution lab run.
type AttributionConfig struct {
	DurationSeconds   int
	SampleIntervalMs int
	CheckpointTimes  []int // seconds from start for each checkpoint (start, midpoint, end)
}

// DefaultAttributionConfig returns sensible defaults for attribution labs.
func DefaultAttributionConfig() AttributionConfig {
	return AttributionConfig{
		DurationSeconds:   600, // 10 minutes default for CI smoke
		SampleIntervalMs:  5000, // 5 second intervals
	}
}

// AttributionConfigForSoak returns config suitable for 30-60 minute soak.
func AttributionConfigForSoak(minutes int) AttributionConfig {
	return AttributionConfig{
		DurationSeconds:   minutes * 60,
		SampleIntervalMs:  10000, // 10 second intervals for longer runs
	}
}
