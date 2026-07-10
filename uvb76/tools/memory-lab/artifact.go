// Package memlab provides artifact contracts and classification for memory leak labs.
//
// This package defines the data structures and pure classification logic for
// UVB-76/tovarisch memory leak analysis. It does not spawn processes or collect
// real profiles—those concerns belong in the lab runner.
package memlab

import (
	"encoding/json"
	"time"
)

// Allowed classification results from memory analysis.
const (
	ClassificationGoHeapRetention  = "suspected_go_heap_retention"
	ClassificationRSSGrowthStable  = "rss_growth_heap_stable"
	ClassificationGoroutineGrowth  = "goroutine_growth"
	ClassificationNoMaterialGrowth = "no_material_growth"
	ClassificationInconclusive     = "inconclusive"
)

// Classification represents the final verdict of memory analysis.
type Classification string

// IsValid returns true if the classification is a known allowed value.
func (c Classification) IsValid() bool {
	switch c {
	case ClassificationGoHeapRetention,
		ClassificationRSSGrowthStable,
		ClassificationGoroutineGrowth,
		ClassificationNoMaterialGrowth,
		ClassificationInconclusive:
		return true
	default:
		return false
	}
}

// SchemaVersion is the current artifact schema version.
const SchemaVersion = 1

// Manifest describes a single memory lab run artifact.
type Manifest struct {
	// SchemaVersion is the artifact schema version.
	SchemaVersion int `json:"schema_version"`
	// LabID identifies the lab run (e.g., timestamp-based).
	LabID string `json:"lab_id"`
	// StartedAt is when the lab run began.
	StartedAt time.Time `json:"started_at"`
	// EndedAt is when the lab run ended.
	EndedAt time.Time `json:"ended_at"`
	// TargetID is the tovarisch/node being profiled.
	TargetID string `json:"target_id"`
	// DurationSeconds is the elapsed time between start and end samples.
	DurationSeconds float64 `json:"duration_seconds"`
	// Classification is the final verdict.
	Classification Classification `json:"classification"`
	// Verdict is the human-readable verdict with details.
	Verdict Verdict `json:"verdict"`
	// Samples contains the raw measurement samples.
	Samples Samples `json:"samples"`
}

// Verdict provides human-readable analysis results.
type Verdict struct {
	// Summary is a one-line summary of the finding.
	Summary string `json:"summary"`
	// RSSGrowthBytes is the RSS delta in bytes.
	RSSGrowthBytes int64 `json:"rss_growth_bytes"`
	// HeapGrowthBytes is the Go heap in-use delta in bytes.
	HeapGrowthBytes int64 `json:"heap_growth_bytes"`
	// GoroutineDelta is the change in goroutine count.
	GoroutineDelta int64 `json:"goroutine_delta"`
	// Reasons contains the list of contributing factors.
	Reasons []string `json:"reasons"`
}

// Samples contains the measurements taken during the lab run.
type Samples struct {
	// StartRSS is the RSS at the beginning of the observation window.
	StartRSS RSSSample `json:"start_rss"`
	// EndRSS is the RSS at the end of the observation window.
	EndRSS RSSSample `json:"end_rss"`
	// StartMemStats is the Go runtime.MemStats at start.
	StartMemStats MemStatsSample `json:"start_mem_stats,omitempty"`
	// EndMemStats is the Go runtime.MemStats at end.
	EndMemStats MemStatsSample `json:"end_mem_stats,omitempty"`
	// GoroutineSamples contains goroutine counts over time.
	GoroutineSamples []GoroutineSample `json:"goroutine_samples"`
	// HeapProfiles contains paths to captured heap profiles.
	HeapProfiles []string `json:"heap_profiles,omitempty"`
}

// RSSSample captures process RSS (Resident Set Size) at a point in time.
type RSSSample struct {
	// Timestamp is when the sample was taken.
	Timestamp time.Time `json:"timestamp"`
	// Bytes is the RSS in bytes.
	Bytes uint64 `json:"bytes"`
}

// MemStatsSample captures Go runtime.MemStats.
type MemStatsSample struct {
	// Timestamp is when the sample was taken.
	Timestamp time.Time `json:"timestamp"`
	// Alloc is bytes of allocated heap objects.
	Alloc uint64 `json:"alloc"`
	// TotalAlloc is cumulative bytes allocated on heap.
	TotalAlloc uint64 `json:"total_alloc"`
	// Sys is bytes of memory obtained from the OS.
	Sys uint64 `json:"sys"`
	// Lookups is number of pointer lookups.
	Lookups uint64 `json:"lookups"`
	// Mallocs is number of heap objects allocated.
	Mallocs uint64 `json:"mallocs"`
	// Frees is number of heap objects freed.
	Frees uint64 `json:"frees"`
	// HeapAlloc is bytes of allocated heap objects.
	HeapAlloc uint64 `json:"heap_alloc"`
	// HeapSys is bytes of heap memory obtained from OS.
	HeapSys uint64 `json:"heap_sys"`
	// HeapIdle is bytes in idle spans.
	HeapIdle uint64 `json:"heap_idle"`
	// HeapInuse is bytes in in-use spans.
	HeapInuse uint64 `json:"heap_inuse"`
	// HeapReleased is bytes released to OS.
	HeapReleased uint64 `json:"heap_released"`
	// HeapObjects is number of allocated heap objects.
	HeapObjects uint64 `json:"heap_objects"`
	// StackInuse is bytes in stack spans.
	StackInuse uint64 `json:"stack_inuse"`
	// StackSys is bytes in stack spans from OS.
	StackSys uint64 `json:"stack_sys"`
	// MSpanInuse is bytes in mspan structures.
	MSpanInuse uint64 `json:"mspan_inuse"`
	// MSpanSys is bytes in mspan structures from OS.
	MSpanSys uint64 `json:"mspan_sys"`
	// MCacheInuse is bytes in mcache structures.
	MCacheInuse uint64 `json:"mcache_inuse"`
	// MCacheSys is bytes in mcache structures from OS.
	MCacheSys uint64 `json:"mcache_sys"`
	// BuckHashSys is bytes in profiling bucket hash tables.
	BuckHashSys uint64 `json:"buck_hash_sys"`
	// GCSys is bytes for GC metadata.
	GCSys uint64 `json:"gc_sys"`
	// OtherSys is bytes for other runtime allocations.
	OtherSys uint64 `json:"other_sys"`
	// NumForcedGC is number of forced GC cycles.
	NumForcedGC uint64 `json:"num_forced_gc"`
	// GCCPUFraction is fraction of CPU time used by GC.
	GCCPUFraction float64 `json:"gc_cpu_fraction"`
	// PauseTotalNs is GC pause accumulators (circular buffer).
	PauseTotalNs [256]uint64 `json:"pause_total_ns"`
	// NumGC is number of completed GC cycles.
	NumGC uint32 `json:"num_gc"`
}

// GoroutineSample captures goroutine count at a point in time.
type GoroutineSample struct {
	// Timestamp is when the sample was taken.
	Timestamp time.Time `json:"timestamp"`
	// Count is the number of goroutines.
	Count int `json:"count"`
}

// ToJSON serializes a Manifest to JSON with stable field names.
func (m *Manifest) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// FromJSON deserializes a Manifest from JSON.
func FromJSON(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
