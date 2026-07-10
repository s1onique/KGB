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

// Conservative thresholds for classification.
const (
	// MinRSSGrowthBytes is the minimum RSS growth to be considered material.
	// 1 MB = 1,048,576 bytes
	MinRSSGrowthBytes int64 = 1024 * 1024
	// MinHeapGrowthBytes is the minimum heap growth to be considered material.
	MinHeapGrowthBytes int64 = 512 * 1024
	// MinGoroutineGrowth is the minimum goroutine count increase to be material.
	// 10 goroutines is a conservative threshold for "leak-like" growth.
	MinGoroutineGrowth int = 10
)

// ClassifierInput contains the data needed for classification.
type ClassifierInput struct {
	// RSS and heap data (in bytes)
	RSSStartBytes      uint64
	RSSEndBytes        uint64
	HeapInuseStartBytes uint64
	HeapInuseEndBytes   uint64
	// Goroutine counts
	GoroutinesStart int
	GoroutinesEnd   int
	// Data availability flags
	HasHeapData bool
	HasRSSData  bool
}

// Classify returns the memory classification based on the input samples.
// This is a pure function with no side effects.
func Classify(in ClassifierInput) Classification {
	// Missing required data: inconclusive
	if !in.HasRSSData {
		return ClassificationInconclusive
	}

	// Compute deltas
	rssDelta := int64(in.RSSEndBytes) - int64(in.RSSStartBytes)
	heapDelta := int64(in.HeapInuseEndBytes) - int64(in.HeapInuseStartBytes)
	goroutineDelta := int64(in.GoroutinesEnd) - int64(in.GoroutinesStart)

	// Goroutine growth is independent of RSS/heap
	if in.GoroutinesStart > 0 && in.GoroutinesEnd > in.GoroutinesStart {
		if goroutineDelta >= int64(MinGoroutineGrowth) {
			return ClassificationGoroutineGrowth
		}
	}

	// RSS grows materially and heap grows materially => suspected_go_heap_retention
	if rssDelta >= MinRSSGrowthBytes && in.HasHeapData && heapDelta >= MinHeapGrowthBytes {
		return ClassificationGoHeapRetention
	}

	// RSS grows materially and heap is stable => rss_growth_heap_stable
	if rssDelta >= MinRSSGrowthBytes && in.HasHeapData && heapDelta < MinHeapGrowthBytes {
		return ClassificationRSSGrowthStable
	}

	// RSS grows materially but no heap data => inconclusive
	if rssDelta >= MinRSSGrowthBytes && !in.HasHeapData {
		return ClassificationInconclusive
	}

	// No material growth detected
	return ClassificationNoMaterialGrowth
}

// BuildVerdict creates a human-readable verdict from classification and samples.
func BuildVerdict(classification Classification, in ClassifierInput) Verdict {
	rssDelta := int64(in.RSSEndBytes) - int64(in.RSSStartBytes)
	heapDelta := int64(in.HeapInuseEndBytes) - int64(in.HeapInuseStartBytes)
	goroutineDelta := int64(in.GoroutinesEnd) - int64(in.GoroutinesStart)

	var summary string
	var reasons []string

	switch classification {
	case ClassificationGoroutineGrowth:
		summary = "goroutine count grew materially"
		reasons = []string{"goroutine_count_delta", formatCountDelta("goroutines", goroutineDelta)}
	case ClassificationGoHeapRetention:
		summary = "suspected Go heap retention"
		reasons = []string{
			"rss_grew_and_heap_grew",
			formatDelta("RSS", rssDelta),
			formatDelta("heap_inuse", heapDelta),
		}
	case ClassificationRSSGrowthStable:
		summary = "RSS grew but heap is stable"
		reasons = []string{
			"rss_grew_heap_stable",
			formatDelta("RSS", rssDelta),
			"heap_delta_below_threshold",
		}
	case ClassificationNoMaterialGrowth:
		summary = "no material memory growth detected"
		reasons = []string{
			formatDelta("RSS", rssDelta),
			formatDelta("heap_inuse", heapDelta),
			formatCountDelta("goroutines", goroutineDelta),
		}
	case ClassificationInconclusive:
		summary = "insufficient data for classification"
		if !in.HasRSSData {
			reasons = []string{"missing_rss_data"}
		} else if !in.HasHeapData {
			reasons = []string{"missing_heap_data", "rss_delta_may_be_material"}
		} else {
			reasons = []string{"unknown_insufficient_data"}
		}
	}

	return Verdict{
		Summary:         summary,
		RSSGrowthBytes:  rssDelta,
		HeapGrowthBytes: heapDelta,
		GoroutineDelta:  goroutineDelta,
		Reasons:         reasons,
	}
}

// formatDelta formats an int64 delta in bytes for logging.
func formatDelta(label string, delta int64) string {
	if delta >= 0 {
		return label + "_increase_" + formatBytes(delta)
	}
	return label + "_decrease_" + formatBytes(-delta)
}

// formatCountDelta formats an int64 delta as a count (no unit suffix).
// Goroutines are counts, not bytes, so they use this function.
func formatCountDelta(label string, delta int64) string {
	if delta >= 0 {
		return label + "_delta_" + formatInt(delta)
	}
	return label + "_delta_" + formatInt(delta) // negative delta already formatted in formatInt
}

// formatBytes formats bytes for human-readable output.
func formatBytes(n int64) string {
	if n < 1024 {
		return formatInt(n) + "B"
	}
	if n < 1024*1024 {
		return formatFloat(float64(n)/1024, 1) + "KB"
	}
	if n < 1024*1024*1024 {
		return formatFloat(float64(n)/(1024*1024), 1) + "MB"
	}
	return formatFloat(float64(n)/(1024*1024*1024), 2) + "GB"
}

// formatInt formats an int64 without importing strconv.
func formatInt(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// formatFloat formats a float64 without importing strconv.
func formatFloat(f float64, decimals int) string {
	if f < 0 {
		return "-" + formatFloat(-f, decimals)
	}
	// Simple integer part + decimal
	intPart := int64(f)
	fracPart := f - float64(intPart)
	// Build fractional digits
	var fracDigits []byte
	for i := 0; i < decimals; i++ {
		fracPart *= 10
		digit := byte('0' + int(fracPart)%10)
		fracDigits = append(fracDigits, digit)
	}
	return formatInt(intPart) + "." + string(fracDigits)
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
