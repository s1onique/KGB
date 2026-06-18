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

// TargetLatencyResponse represents the latency response for a single target.
type TargetLatencyResponse struct {
	TargetID string              `json:"target_id"`
	HTTP     *state.LatencySummary `json:"http,omitempty"`
	ICMP     *state.LatencySummary `json:"icmp,omitempty"`
}

// handleTargetLatency returns the latency summary for a specific target (both HTTP and ICMP).
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

	response := TargetLatencyResponse{
		TargetID: targetID,
	}

	// Get HTTP latency if enabled
	if s.cfg.Latency.HTTP.IsEnabled() {
		httpSummary := s.state.GetLatencySummary(targetID)
		response.HTTP = &httpSummary
	}

	// Get ICMP latency if enabled
	if s.cfg.Latency.ICMP.IsEnabled() {
		icmpSummary := s.state.GetICMPLatencySummary(targetID)
		response.ICMP = &icmpSummary
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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

// SpikeResponse represents the API response for spike events.
type SpikeResponse struct {
	Spikes []state.SpikeEvent `json:"spikes"`
	Count  int                `json:"count"`
}

// SpikeResponseWithCaptures represents the API response for spike events with diagnostic captures.
type SpikeResponseWithCaptures struct {
	Spikes []state.SpikeEventWithCaptures `json:"spikes"`
	Count  int                           `json:"count"`
}

// handleTargetLatencySpikes returns recent spike events for a target.
// Supports optional query parameter: include_captures=true to include diagnostic captures.
func (s *Server) handleTargetLatencySpikes(w http.ResponseWriter, r *http.Request) {
	targetID := r.URL.Query().Get("target_id")
	if targetID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "target_id is required"})
		return
	}

	// Validate target exists
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

	// Parse probe_kind (defaults to http)
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "http"
	}
	if kind != "http" && kind != "icmp" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "kind must be 'http' or 'icmp'"})
		return
	}

	// Parse limit (default 20, max 100)
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	// Clamp to safe maximum
	if limit > 100 {
		limit = 100
	}

	// Check if captures should be included
	includeCaptures := r.URL.Query().Get("include_captures") == "true"

	spikes := s.state.GetSpikes(targetID, kind, limit)

	w.Header().Set("Content-Type", "application/json")
	if includeCaptures {
		// Return spikes with diagnostic captures
		captures := s.state.GetCaptureStore()
		spikesWithCaptures := make([]state.SpikeEventWithCaptures, len(spikes))
		for i, spike := range spikes {
			spikesWithCaptures[i] = state.SpikeEventWithCaptures{
				SpikeEvent: spike,
				Captures:   captures.GetCaptures(spike.EventID),
			}
		}
		json.NewEncoder(w).Encode(SpikeResponseWithCaptures{
			Spikes: spikesWithCaptures,
			Count:  len(spikesWithCaptures),
		})
	} else {
		json.NewEncoder(w).Encode(SpikeResponse{
			Spikes: spikes,
			Count:  len(spikes),
		})
	}
}

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

	// Get samples based on probe_kind
	var samples []state.LatencySample
	var intervalSeconds int
	var windowSec int
	var retainedRange int
	var probeURL string
	var oldestTs, newestTs *time.Time
	var sampleCount int

	if probeKind == "http" {
		maxSamples := s.state.GetMaxSamples()
		samples = s.state.GetRecentLatencySamples(targetID, maxSamples)
		intervalSeconds = s.cfg.Latency.HTTP.IntervalSeconds
		windowSec = s.cfg.Latency.HTTP.WindowSeconds
		retainedRange = s.cfg.Latency.HTTP.RetainedRangeSeconds
		probeURL = config.TargetStatusURL(targetCfg.BaseURL)
		oldestTs, newestTs = s.state.GetLatencySampleTimestamps(targetID)
		// Use actual sample count from the samples returned
		sampleCount = len(samples)
	} else {
		maxSamples := s.state.GetICMPMaxSamples()
		samples = s.state.GetRecentICMPLatencySamples(targetID, maxSamples)
		intervalSeconds = s.cfg.Latency.ICMP.IntervalSeconds
		windowSec = s.cfg.Latency.ICMP.WindowSeconds
		retainedRange = s.cfg.Latency.ICMP.RetainedRangeSeconds
		// ICMP pings the hostname from base_url without port/path
		probeURL = targetCfg.BaseURL // for reference only
		oldestTs, newestTs = s.state.GetICMPLatencySampleTimestamps(targetID)
		// Use actual sample count from the samples returned
		sampleCount = len(samples)
	}

	// Override window if provided in query
	if windowSeconds <= 0 {
		windowSeconds = windowSec
	}

	// Calculate time bounds - use trailing windows anchored to now
	now := time.Now().UTC()

	// RetainedRangeSeconds = min(rangeSeconds, max samples duration)
	// We can only show data up to the age of our oldest sample
	maxRetained := retainedRange
	var maxSamples int
	if probeKind == "http" {
		maxSamples = s.state.GetMaxSamples()
	} else {
		maxSamples = s.state.GetICMPMaxSamples()
	}
	if maxSamples > 0 && intervalSeconds > 0 {
		// Each sample is at most IntervalSeconds old
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

	// Build time series with explicit retention metadata
	series := state.LatencySeries{
		TargetID:               targetID,
		ProbeKind:              probeKind,
		ProbeURL:               probeURL,
		IntervalSeconds:        intervalSeconds,
		QueryRangeSeconds:      rangeSeconds,           // what the client requested
		RangeSeconds:           effectiveRange,          // what we actually returned (clamped)
		StepSeconds:            stepSeconds,
		WindowSeconds:          windowSeconds,
		RetainedRangeSeconds:   maxRetained,
		SampleCount:            sampleCount,             // DEPRECATED: for backward compat
		RetainedSampleCount:    sampleCount,             // actual samples in buffer
		RetainedSampleCapacity: maxSamples,               // buffer capacity
		ReturnedPointCount:     0,                       // filled after building points
		OldestSampleTs:         oldestTs,
		NewestSampleTs:         newestTs,
		Points:                 []state.PercentilePoint{},
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

	// Update returned point count after building all points
	series.ReturnedPointCount = len(series.Points)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(series)
}
