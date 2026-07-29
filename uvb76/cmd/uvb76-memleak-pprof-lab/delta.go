// Package main provides the UVB-76 pprof memory leak lab.
//
// # Raw Delta Summarization
//
// This file implements P0-8: Add raw-delta summarization.
// For each process calculates without classifying:
// - first sample
// - last sample
// - minimum
// - maximum
// - absolute delta
//
// For: rss_kib, pss_kib, pss_anon_kib, private_dirty_kib, anonymous_kib,
// threads, fd_count, goroutine_count (UVB-76 only)
package main

import (
	"math"
	"sort"
	"time"
)

// DeltaSummary holds delta statistics for a single metric.
type DeltaSummary struct {
	Metric string `json:"metric"`
	First  int64  `json:"first"`
	Last   int64  `json:"last"`
	Min    int64  `json:"min"`
	Max    int64  `json:"max"`
	Delta  int64  `json:"delta"`
}

// ProcessDelta holds delta statistics with timestamps for a process.
type ProcessDelta struct {
	Metric            string    `json:"metric"`
	FirstTimestamp    time.Time `json:"first_timestamp"`
	LastTimestamp     time.Time `json:"last_timestamp"`
	FirstValue        int64     `json:"first_value"`
	LastValue         int64     `json:"last_value"`
	Delta             int64     `json:"delta"`
	Min               int64     `json:"min"`
	Max               int64     `json:"max"`
	SlopeKibPerMinute float64   `json:"slope_kib_per_minute"`
}

// ProcessDeltaSummary holds all delta summaries for a process.
type ProcessDeltaSummary struct {
	Process   string        `json:"process"`
	RowID     string        `json:"row_id,omitempty"`
	StartTime string        `json:"start_time,omitempty"`
	EndTime   string        `json:"end_time,omitempty"`
	Samples   int           `json:"samples"`
	RSS       *DeltaSummary `json:"rss_kib,omitempty"`
	PSS       *DeltaSummary `json:"pss_kib,omitempty"`
	PSSAnon   *DeltaSummary `json:"pss_anon_kib,omitempty"`
	PrivDirty *DeltaSummary `json:"private_dirty_kib,omitempty"`
	Anonymous *DeltaSummary `json:"anonymous_kib,omitempty"`
	Threads   *DeltaSummary `json:"threads,omitempty"`
	FDCount   *DeltaSummary `json:"fd_count,omitempty"`
	Goroutine *DeltaSummary `json:"goroutine_count,omitempty"`
}

// ComputeDeltaSummary computes delta statistics for a metric.
func ComputeDeltaSummary(samples []ProcessSample, field func(ProcessSample) int64) *DeltaSummary {
	if len(samples) == 0 {
		return nil
	}

	values := make([]int64, len(samples))
	for i, s := range samples {
		values[i] = field(s)
	}

	sorted := make([]int64, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	return &DeltaSummary{
		First: values[0],
		Last:  values[len(values)-1],
		Min:   sorted[0],
		Max:   sorted[len(sorted)-1],
		Delta: values[len(values)-1] - values[0],
	}
}

// ComputeProcessDeltas computes delta with timestamps and slope for RSS.
// P0-8: Simplified function for use by runner.
func ComputeProcessDeltas(samples []ProcessSample) *ProcessDelta {
	if len(samples) == 0 {
		return nil
	}

	if len(samples) == 1 {
		return &ProcessDelta{
			Metric:         "rss_kib",
			FirstTimestamp: samples[0].Timestamp,
			LastTimestamp:  samples[0].Timestamp,
			FirstValue:     samples[0].RSSKIB,
			LastValue:      samples[0].RSSKIB,
			Delta:          0,
			Min:            samples[0].RSSKIB,
			Max:            samples[0].RSSKIB,
		}
	}

	first := samples[0]
	last := samples[len(samples)-1]
	duration := last.Timestamp.Sub(first.Timestamp).Seconds()

	// Find min/max
	minVal := first.RSSKIB
	maxVal := first.RSSKIB
	for _, s := range samples {
		if s.RSSKIB < minVal {
			minVal = s.RSSKIB
		}
		if s.RSSKIB > maxVal {
			maxVal = s.RSSKIB
		}
	}

	return &ProcessDelta{
		Metric:            "rss_kib",
		FirstTimestamp:    first.Timestamp,
		LastTimestamp:     last.Timestamp,
		FirstValue:        first.RSSKIB,
		LastValue:         last.RSSKIB,
		Delta:             last.RSSKIB - first.RSSKIB,
		Min:               minVal,
		Max:               maxVal,
		SlopeKibPerMinute: ComputeSlopeKiBPerMinute(first.RSSKIB, last.RSSKIB, duration),
	}
}

// ComputeProcessDeltasFull computes delta summaries for a process.
// P0-8: Calculates without classifying - no leak verdict in this ACT.
func ComputeProcessDeltasFull(samples []ProcessSample, processName string, includeGoroutines bool) *ProcessDeltaSummary {
	if len(samples) == 0 {
		return nil
	}

	summary := &ProcessDeltaSummary{
		Process: processName,
		Samples: len(samples),
	}

	if len(samples) > 0 {
		summary.StartTime = samples[0].Timestamp.UTC().Format(time.RFC3339)
	}
	if len(samples) > 1 {
		summary.EndTime = samples[len(samples)-1].Timestamp.UTC().Format(time.RFC3339)
	}

	// Compute delta for each metric
	summary.RSS = ComputeDeltaSummary(samples, func(s ProcessSample) int64 { return s.RSSKIB })
	summary.PSS = ComputeDeltaSummary(samples, func(s ProcessSample) int64 { return s.PSS_KIB })
	summary.PSSAnon = ComputeDeltaSummary(samples, func(s ProcessSample) int64 { return s.PSSAnonKIB })
	summary.PrivDirty = ComputeDeltaSummary(samples, func(s ProcessSample) int64 { return s.PrivateDirtyKIB })
	summary.Anonymous = ComputeDeltaSummary(samples, func(s ProcessSample) int64 { return s.AnonymousKIB })
	summary.Threads = ComputeDeltaSummary(samples, func(s ProcessSample) int64 { return int64(s.Threads) })
	summary.FDCount = ComputeDeltaSummary(samples, func(s ProcessSample) int64 { return int64(s.FDCount) })

	// Only include goroutines for UVB-76, not Tovarisch
	if includeGoroutines {
		summary.Goroutine = ComputeDeltaSummary(samples, func(s ProcessSample) int64 { return s.GoroutineCount })
	}

	return summary
}

// MatrixSummary holds the comparison summary across multiple rows.
type MatrixSummary struct {
	SchemaVersion int          `json:"schema_version"`
	RunID         string       `json:"run_id"`
	SourceCommit  string       `json:"source_commit"`
	Rows          []RowSummary `json:"rows"`
}

// RowSummary holds the summary for a single row.
type RowSummary struct {
	RowID             string               `json:"row_id"`
	Classification    string               `json:"classification"`
	Tovarisch         *ProcessDeltaSummary `json:"tovarisch,omitempty"`
	UVB76             *ProcessDeltaSummary `json:"uvb76,omitempty"`
	ScrapeAttempts    int                  `json:"scrape_attempts"`
	ScrapeCompletions int                  `json:"scrape_completions"`
	ProfileCount      int                  `json:"profile_count"`
	ProfileFiles      []string             `json:"profile_files,omitempty"`
	Errors            []string             `json:"errors,omitempty"`
}

// BuildMatrixSummary builds a matrix summary from multiple row results.
// P0-12: Generates matrix-summary.json outside the repository.
func BuildMatrixSummary(runID string, sourceCommit string, rows []RowSummary) *MatrixSummary {
	return &MatrixSummary{
		SchemaVersion: 1,
		RunID:         runID,
		SourceCommit:  sourceCommit,
		Rows:          rows,
	}
}

// ComputeSlopeKiBPerMinute computes the memory growth slope in KiB/minute.
// P0-8: Renamed from IsLeakSlope; no leak classification - just raw calculation.
func ComputeSlopeKiBPerMinute(firstRSS, lastRSS int64, durationSeconds float64) float64 {
	if durationSeconds <= 0 {
		return 0
	}
	growth := float64(lastRSS - firstRSS)
	return growth / durationSeconds * 60.0 // KiB/minute
}

// round rounds a float64 to 2 decimal places.
func round(f float64) float64 {
	return math.Round(f*100) / 100
}
