package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// Series endpoint constants for bounds enforcement.
// These prevent pathological request parameters from causing excessive allocations.
const (
	// MaxRangeSeconds caps the range_seconds query parameter.
	// With 1s ICMP probes, 3600 samples covers 1 hour.
	MaxRangeSeconds = 86400 // 24 hours max

	// MinStepSeconds prevents zero/negative step values.
	MinStepSeconds = 1

	// MaxStepSeconds prevents too-coarse-grained buckets that would create few points.
	MaxStepSeconds = 3600

	// MaxWindowSeconds caps the window size to prevent unbounded memory.
	MaxWindowSeconds = 3600

	// MaxOutputPoints caps the number of output points to prevent
	// excessive allocations. Range/step of 3600/60 = 60 points is typical.
	MaxOutputPoints = 1440 // 24 hours at 1-minute resolution

	// MaxSamplesPerWindow caps the per-window sample count for sorting.
	// This limits memory per window and prevents pathological input sizes.
	// Note: the nested loop scans all samples for each point; with bounded
	// MaxOutputPoints and max 3600 input samples, this is acceptable.
	MaxSamplesPerWindow = 36000
)

// handleTargetLatencySeries returns percentile time-series data for a target.
func (s *Server) handleTargetLatencySeries(w http.ResponseWriter, r *http.Request) {
	targetID := r.URL.Query().Get("target_id")
	if targetID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "target_id is required"})
		return
	}

	// Parse probe_kind (defaults to http)
	probeKind := r.URL.Query().Get("probe_kind")
	if probeKind == "" {
		probeKind = "http"
	}

	// Validate probe_kind
	if probeKind != "http" && probeKind != "icmp" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "probe_kind must be 'http' or 'icmp'"})
		return
	}

	// Find target in config
	var targetCfg *config.TargetConfig
	for _, t := range s.cfg.Targets {
		if t.ID == targetID {
			targetCfg = &t
			break
		}
	}
	if targetCfg == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "target_not_found"})
		return
	}

	// Parse parameters with defaults
	rangeSeconds := 3600
	if rs := r.URL.Query().Get("range_seconds"); rs != "" {
		if parsed, err := strconv.Atoi(rs); err == nil && parsed > 0 {
			rangeSeconds = parsed
		}
	}
	// Clamp range_seconds to maximum
	if rangeSeconds > MaxRangeSeconds {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":       "range_seconds exceeds maximum",
			"max_allowed": strconv.Itoa(MaxRangeSeconds),
		})
		return
	}

	stepSeconds := 60
	if ss := r.URL.Query().Get("step_seconds"); ss != "" {
		if parsed, err := strconv.Atoi(ss); err == nil && parsed > 0 {
			stepSeconds = parsed
		}
	}
	// Clamp step_seconds to valid range
	if stepSeconds < MinStepSeconds {
		stepSeconds = MinStepSeconds
	}
	if stepSeconds > MaxStepSeconds {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":       "step_seconds exceeds maximum",
			"max_allowed": strconv.Itoa(MaxStepSeconds),
		})
		return
	}

	windowSeconds := 300
	if ws := r.URL.Query().Get("window_seconds"); ws != "" {
		if parsed, err := strconv.Atoi(ws); err == nil && parsed > 0 {
			windowSeconds = parsed
		}
	}
	// Clamp window_seconds to maximum
	if windowSeconds > MaxWindowSeconds {
		windowSeconds = MaxWindowSeconds
	}
	if windowSeconds <= 0 {
		windowSeconds = 300 // default fallback
	}

	// Get samples based on probe_kind. This handler makes an explicit copy of the
	// samples slice before aggregation to harden against stale/shared slice ownership.
	// The crash at line 418 (series.Points = append(...)) during growslice suggested
	// potential aliasing with mutable ring buffer state. This defensive copy ensures
	// the handler owns its backing array independently of concurrent tracker writes.
	var samples []state.LatencySample
	var intervalSeconds int
	var windowSec int
	var retainedRange int
	var probeURL string
	var oldestTs, newestTs *time.Time
	var sampleCount int

	if probeKind == "http" {
		maxSamples := s.state.GetMaxSamples()
		rawSamples := s.state.GetRecentLatencySamples(targetID, maxSamples)
		// Defensive copy: ensures our backing array is owned and stable
		samples = append([]state.LatencySample(nil), rawSamples...)
		intervalSeconds = s.cfg.Latency.HTTP.IntervalSeconds
		windowSec = s.cfg.Latency.HTTP.WindowSeconds
		retainedRange = s.cfg.Latency.HTTP.RetainedRangeSeconds
		probeURL = config.TargetStatusURL(targetCfg.BaseURL)
		oldestTs, newestTs = s.state.GetLatencySampleTimestamps(targetID)
		sampleCount = len(samples)
	} else {
		maxSamples := s.state.GetICMPMaxSamples()
		rawSamples := s.state.GetRecentICMPLatencySamples(targetID, maxSamples)
		// Defensive copy: ensures our backing array is owned and stable
		samples = append([]state.LatencySample(nil), rawSamples...)
		intervalSeconds = s.cfg.Latency.ICMP.IntervalSeconds
		windowSec = s.cfg.Latency.ICMP.WindowSeconds
		retainedRange = s.cfg.Latency.ICMP.RetainedRangeSeconds
		probeURL = targetCfg.BaseURL
		oldestTs, newestTs = s.state.GetICMPLatencySampleTimestamps(targetID)
		sampleCount = len(samples)
	}

	// Override window if provided in query
	if windowSeconds <= 0 {
		windowSeconds = windowSec
	}

	// Calculate time bounds
	now := time.Now().UTC()

	// RetainedRangeSeconds = min(rangeSeconds, max samples duration)
	maxRetained := retainedRange
	var maxSamples int
	if probeKind == "http" {
		maxSamples = s.state.GetMaxSamples()
	} else {
		maxSamples = s.state.GetICMPMaxSamples()
	}
	if maxSamples > 0 && intervalSeconds > 0 {
		maxSampleAge := intervalSeconds * maxSamples
		if maxSampleAge < maxRetained {
			maxRetained = maxSampleAge
		}
	}

	// Clamp effective range to retained capacity
	effectiveRange := rangeSeconds
	if maxRetained < effectiveRange {
		effectiveRange = maxRetained
	}

	// Build time series
	series := state.LatencySeries{
		TargetID:               targetID,
		ProbeKind:              probeKind,
		ProbeURL:               probeURL,
		IntervalSeconds:        intervalSeconds,
		QueryRangeSeconds:      rangeSeconds,
		RangeSeconds:           effectiveRange,
		StepSeconds:            stepSeconds,
		WindowSeconds:          windowSeconds,
		RetainedRangeSeconds:   maxRetained,
		SampleCount:            sampleCount,
		RetainedSampleCount:    sampleCount,
		RetainedSampleCapacity: maxSamples,
		ReturnedPointCount:     0,
		OldestSampleTs:         oldestTs,
		NewestSampleTs:         newestTs,
		Points:                 []state.PercentilePoint{},
	}

	// Build series points
	numSteps := effectiveRange / stepSeconds
	if numSteps < 1 {
		numSteps = 1
	}
	// Hard cap on output points to prevent unbounded allocations
	if numSteps > MaxOutputPoints {
		numSteps = MaxOutputPoints
	}

	for i := 0; i < numSteps; i++ {
		stepsFromNow := numSteps - 1 - i
		windowEnd := now.Add(-time.Duration(stepsFromNow*stepSeconds) * time.Second)
		windowStart := windowEnd.Add(-time.Duration(windowSeconds) * time.Second)

		// Find samples in this window
		// Use preallocated slice with capacity hint to avoid repeated reallocations
		windowSamples := make([]float64, 0, 256)
		var errorCount int
		for _, sample := range samples {
			if sample.Timestamp.After(windowStart) && !sample.Timestamp.After(windowEnd) {
				if sample.Reachable {
					// Safety cap on window sample count
					if len(windowSamples) < MaxSamplesPerWindow {
						windowSamples = append(windowSamples, sample.LatencyMs)
					}
				} else {
					errorCount++
				}
			}
		}

		point := state.PercentilePoint{
			Timestamp:   windowEnd,
			SampleCount: len(windowSamples) + errorCount,
			ErrorCount:  errorCount,
		}

		if len(windowSamples) > 0 {
			sorted := make([]float64, len(windowSamples))
			copy(sorted, windowSamples)
			sort.Float64s(sorted)
			percentiles := state.CalculatePercentiles(sorted, []float64{50, 90, 95, 99})
			if p50, ok := percentiles[50]; ok {
				point.P50Ms = p50
			}
			if p90, ok := percentiles[90]; ok {
				point.P90Ms = p90
			}
			if p95, ok := percentiles[95]; ok {
				point.P95Ms = p95
			}
			if p99, ok := percentiles[99]; ok {
				point.P99Ms = p99
			}
		}

		series.Points = append(series.Points, point)
	}

	series.ReturnedPointCount = len(series.Points)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(series)
}
