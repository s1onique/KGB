package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
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
// It uses domain.SampleWindow for percentile math, preserving the existing API contract.
func (s *Server) handleTargetLatencySeries(w http.ResponseWriter, r *http.Request) {
	targetID := r.URL.Query().Get("target_id")
	if targetID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "target_id is required"})
		return
	}

	// Parse probe_kind (defaults to http)
	probeKindStr := r.URL.Query().Get("probe_kind")
	if probeKindStr == "" {
		probeKindStr = "http"
	}

	// Validate probe_kind
	if probeKindStr != "http" && probeKindStr != "icmp" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "probe_kind must be 'http' or 'icmp'"})
		return
	}

	// Convert to domain.ProbeKind
	var probeKind domain.ProbeKind
	if probeKindStr == "http" {
		probeKind = domain.ProbeKindHTTP
	} else {
		probeKind = domain.ProbeKindICMP
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

	// Get configuration values based on probe kind
	var maxSamples int
	var intervalSeconds int
	var windowSec int
	var retainedRange int
	var probeURL string

	if probeKind == domain.ProbeKindHTTP {
		maxSamples = s.state.GetMaxSamples()
		intervalSeconds = s.cfg.Latency.HTTP.IntervalSeconds
		windowSec = s.cfg.Latency.HTTP.WindowSeconds
		retainedRange = s.cfg.Latency.HTTP.RetainedRangeSeconds
		probeURL = config.TargetStatusURL(targetCfg.BaseURL)
	} else {
		maxSamples = s.state.GetICMPMaxSamples()
		intervalSeconds = s.cfg.Latency.ICMP.IntervalSeconds
		windowSec = s.cfg.Latency.ICMP.WindowSeconds
		retainedRange = s.cfg.Latency.ICMP.RetainedRangeSeconds
		probeURL = targetCfg.BaseURL
	}

	// Get series snapshot using domain.SampleWindow
	snap := s.state.GetSeriesSnapshot(targetID, probeKind, maxSamples)

	// Override window if provided in query
	if windowSeconds <= 0 {
		windowSeconds = windowSec
	}

	// Calculate time bounds
	now := time.Now().UTC()

	// RetainedRangeSeconds = min(rangeSeconds, max samples duration)
	maxRetained := retainedRange
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

	// Build time series response
	series := state.LatencySeries{
		TargetID:               targetID,
		ProbeKind:              probeKindStr,
		ProbeURL:               probeURL,
		IntervalSeconds:        intervalSeconds,
		QueryRangeSeconds:      rangeSeconds,
		RangeSeconds:           effectiveRange,
		StepSeconds:            stepSeconds,
		WindowSeconds:          windowSeconds,
		RetainedRangeSeconds:   maxRetained,
		SampleCount:            snap.RetainedSampleCount,
		RetainedSampleCount:    snap.RetainedSampleCount,
		RetainedSampleCapacity: snap.RetainedSampleCapacity,
		ReturnedPointCount:     0,
		Points:                 []state.PercentilePoint{},
	}

	// Handle timestamps - convert value to pointer if present
	if !snap.OldestSampleTs.IsZero() {
		ts := snap.OldestSampleTs
		series.OldestSampleTs = &ts
	}
	if !snap.NewestSampleTs.IsZero() {
		ts := snap.NewestSampleTs
		series.NewestSampleTs = &ts
	}

	// Build series points using domain.SampleWindow for percentile math
	numSteps := effectiveRange / stepSeconds
	if numSteps < 1 {
		numSteps = 1
	}
	// Hard cap on output points to prevent unbounded allocations
	if numSteps > MaxOutputPoints {
		numSteps = MaxOutputPoints
	}

	// Get domain samples for percentile math and metadata for exact error counting
	domainSamples := snap.Window.Samples()
	sampleMetas := snap.Samples

	for i := 0; i < numSteps; i++ {
		stepsFromNow := numSteps - 1 - i
		windowEnd := now.Add(-time.Duration(stepsFromNow*stepSeconds) * time.Second)
		windowStart := windowEnd.Add(-time.Duration(windowSeconds) * time.Second)

		// Collect samples in this window from domain samples and metadata
		var windowDomainSamples []domain.Sample
		var errorCount int

		// Filter domain samples by timestamp for percentile calculation
		for _, sample := range domainSamples {
			if sample.At.After(windowStart) && !sample.At.After(windowEnd) {
				if len(windowDomainSamples) < MaxSamplesPerWindow {
					windowDomainSamples = append(windowDomainSamples, sample)
				}
			}
		}

		// Count errors exactly using sample metadata
		// LatencySeriesSampleMeta provides timestamps and OK/failed without raw latency values,
		// preserving the domain boundary while enabling exact per-window error counts.
		for _, meta := range sampleMetas {
			if meta.At.After(windowStart) && !meta.At.After(windowEnd) {
				if !meta.OK {
					errorCount++
				}
			}
		}

		totalInWindow := len(windowDomainSamples)

		point := state.PercentilePoint{
			Timestamp:   windowEnd,
			SampleCount: totalInWindow + errorCount,
			ErrorCount:  errorCount,
		}

		// Use domain.SampleWindow for percentile calculation
		if len(windowDomainSamples) > 0 {
			bucketWindow := domain.NewSampleWindow(windowDomainSamples)

			// Use domain percentile methods
			if p50, ok := bucketWindow.P50(); ok {
				val := p50.Float64()
				point.P50Ms = &val
			}
			if p90, ok := bucketWindow.P90(); ok {
				val := p90.Float64()
				point.P90Ms = &val
			}
			if p95, ok := bucketWindow.P95(); ok {
				val := p95.Float64()
				point.P95Ms = &val
			}
			if p99, ok := bucketWindow.P99(); ok {
				val := p99.Float64()
				point.P99Ms = &val
			}
		}

		series.Points = append(series.Points, point)
	}

	series.ReturnedPointCount = len(series.Points)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(series)
}
