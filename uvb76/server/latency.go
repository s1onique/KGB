package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// handleTargetLatency returns the latency summary for a specific target.
func (s *Server) handleTargetLatency(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetID := vars["id"]

	// Find target in config
	var found bool
	for _, t := range s.cfg.Targets {
		if t.ID == targetID {
			found = true
			break
		}
	}
	if !found {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "target_not_found"})
		return
	}

	summary := s.state.GetLatencySummary(targetID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// handleTargetLatencySamples returns recent latency samples for a specific target.
func (s *Server) handleTargetLatencySamples(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetID := vars["id"]

	// Find target in config
	var found bool
	for _, t := range s.cfg.Targets {
		if t.ID == targetID {
			found = true
			break
		}
	}
	if !found {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "target_not_found"})
		return
	}

	// Default limit to 100 samples if not specified
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		var parsedLimit int
		if err := json.Unmarshal([]byte(l), &parsedLimit); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	samples := s.state.GetRecentLatencySamples(targetID, limit)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(samples)
}

// handleAllLatency returns latency summaries for all configured targets.
func (s *Server) handleAllLatency(w http.ResponseWriter, r *http.Request) {
	targetIDs := make([]string, len(s.cfg.Targets))
	for i, t := range s.cfg.Targets {
		targetIDs[i] = t.ID
	}
	summaries := s.state.GetAllTargetSummaries(targetIDs)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summaries)
}

// handleTargetLatencySeries returns percentile time-series data for a target.
func (s *Server) handleTargetLatencySeries(w http.ResponseWriter, r *http.Request) {
	targetID := r.URL.Query().Get("target_id")
	if targetID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "target_id is required"})
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

	stepSeconds := 60
	if ss := r.URL.Query().Get("step_seconds"); ss != "" {
		if parsed, err := strconv.Atoi(ss); err == nil && parsed > 0 {
			stepSeconds = parsed
		}
	}

	windowSeconds := 300
	if ws := r.URL.Query().Get("window_seconds"); ws != "" {
		if parsed, err := strconv.Atoi(ws); err == nil && parsed > 0 {
			windowSeconds = parsed
		}
	}

	// Get samples
	samples := s.state.GetRecentLatencySamples(targetID, s.state.GetMaxSamples())

	// Calculate time bounds - use trailing windows anchored to now
	now := time.Now().UTC()
	
	// RetainedRangeSeconds = min(rangeSeconds, max samples duration)
	// We can only show data up to the age of our oldest sample
	retainedRange := rangeSeconds
	if s.state.GetMaxSamples() > 0 {
		// Each sample is at most IntervalSeconds old
		maxSampleAge := s.cfg.Latency.IntervalSeconds * s.state.GetMaxSamples()
		if maxSampleAge < retainedRange {
			retainedRange = maxSampleAge
		}
	}
	
	// Clamp effective range to retained capacity
	effectiveRange := rangeSeconds
	if retainedRange < effectiveRange {
		effectiveRange = retainedRange
	}

	// Build time series
	series := state.LatencySeries{
		TargetID:             targetID,
		ProbeKind:           "http_status",
		ProbeURL:            config.TargetStatusURL(targetCfg.BaseURL),
		IntervalSeconds:     s.cfg.Latency.IntervalSeconds,
		RangeSeconds:        effectiveRange, // clamped to retained capacity
		StepSeconds:         stepSeconds,
		WindowSeconds:       windowSeconds,
		RetainedRangeSeconds: retainedRange,
		Points:              []state.PercentilePoint{},
	}

	// Build series points: oldest first (left-to-right on graph)
	numSteps := effectiveRange / stepSeconds
	if numSteps < 1 {
		numSteps = 1
	}
	
	for i := 0; i < numSteps; i++ {
		// i=0 is oldest, i=numSteps-1 is newest
		// stepsFromNow: how many steps back from now
		stepsFromNow := numSteps - 1 - i
		windowEnd := now.Add(-time.Duration(stepsFromNow*stepSeconds) * time.Second)
		windowStart := windowEnd.Add(-time.Duration(windowSeconds) * time.Second)

		// Find samples in this window
		var windowSamples []float64
		var errorCount int
		for _, sample := range samples {
			if sample.Timestamp.After(windowStart) && !sample.Timestamp.After(windowEnd) {
				if sample.Reachable {
					windowSamples = append(windowSamples, sample.LatencyMs)
				} else {
					errorCount++
				}
			}
		}

		// Timestamp is the trailing-window end time
		point := state.PercentilePoint{
			Timestamp:   windowEnd,
			SampleCount: len(windowSamples) + errorCount,
			ErrorCount:  errorCount,
		}

		// Calculate percentiles if we have samples
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(series)
}
