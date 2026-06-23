package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/state"
)

// TargetLatencyResponse represents the latency response for a single target.
type TargetLatencyResponse struct {
	TargetID string               `json:"target_id"`
	HTTP    *state.LatencySummary `json:"http,omitempty"`
	ICMP    *state.LatencySummary `json:"icmp,omitempty"`
}

// handleTargetLatency returns the latency summary for a specific target (both HTTP and ICMP).
// Note: This handler uses GetSummary() which is already race-safe with proper RLock.
// The GetSummary method computes percentiles from a locked snapshot of the ring buffer,
// so it doesn't need the Snapshot primitive.
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
// Uses the Snapshot primitive for consistent atomic read of tracker state.
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

	// Use snapshot for atomic read - samples are already caller-owned copies
	snap := s.state.GetHTTPSnapshot(targetID, limit)
	if snap == nil {
		samples := []state.LatencySample{}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(samples)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snap.Samples)
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
	Spikes    []state.SpikeEventWithCaptures `json:"spikes"`
	Count     int                            `json:"count"`
	Retention state.SpikeRetentionStats     `json:"retention"`
	// PinnedAnchors contains anchor spike events that were pinned into the response
	// because they are required by suppressed rows but would otherwise be evicted.
	// These are rendered with "pinned_anchor" badge and do not count toward page totals.
	PinnedAnchors []state.SpikeEventWithCaptures `json:"pinned_anchors,omitempty"`
}

// anchorVisibleInProbeKind checks if an anchor timestamp is visible in the spike set
// for a specific probe kind. This is used for cross-probe suppression to verify
// the anchor spike exists in its own probe kind's spike set.
func anchorVisibleInProbeKind(st *state.Manager, targetID, probeKind string, anchorTime time.Time) bool {
	// Get all spikes for the anchor probe kind (no limit to check full retention)
	anchorSpikes := st.GetSpikes(targetID, probeKind, 0)

	// Check if any spike has a capture with the anchor timestamp
	captureStore := st.GetCaptureStore()
	for _, spike := range anchorSpikes {
		captures := captureStore.GetCaptures(spike.EventID)
		for _, capture := range captures {
			if capture.CaptureStatus == state.CaptureStatusCaptured && capture.CaptureStartedAt.Equal(anchorTime) {
				return true
			}
		}
	}
	return false
}

// handleTargetLatencySpikes returns recent spike events for a target.
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
	if limit > 100 {
		limit = 100
	}

	// Check if captures should be included
	includeCaptures := r.URL.Query().Get("include_captures") == "true"

	// Get spikes
	allRetainedSpikes := s.state.GetSpikes(targetID, kind, 0)
	displaySpikes := s.state.GetSpikes(targetID, kind, limit)
	captureStore := s.state.GetCaptureStore()

	// Calculate retention stats
	retainedCount := len(allRetainedSpikes)
	visibleCount := len(displaySpikes)
	protectedCount := 0
	purgeEligibleCount := 0
	maxUncaptured := s.cfg.Diagnostics.MaxUncapturedSpikes

	for _, spike := range allRetainedSpikes {
		isProtected, _ := captureStore.GetProtectionInfo(spike.EventID)
		if isProtected {
			protectedCount++
		} else {
			purgeEligibleCount++
		}
	}

	retention := state.SpikeRetentionStats{
		RetainedSpikeCount:    retainedCount,
		VisibleSpikeCount:     visibleCount,
		ProtectedCaptureCount:  protectedCount,
		PurgeEligibleCount:    purgeEligibleCount,
		MaxUncapturedSpikes:   maxUncaptured,
	}

	w.Header().Set("Content-Type", "application/json")
	if includeCaptures {
		// STEP 1: Build set of visible anchor timestamps from displaySpikes
		visibleAnchorTimestamps := make(map[time.Time]bool)
		visibleCapturedEventIDs := make(map[string]bool)
		for _, spike := range displaySpikes {
			captures := captureStore.GetCaptures(spike.EventID)
			for _, capture := range captures {
				if capture.CaptureStatus == state.CaptureStatusCaptured && capture.CaptureStartedAt.After(time.Time{}) {
					visibleAnchorTimestamps[capture.CaptureStartedAt] = true
					visibleCapturedEventIDs[spike.EventID] = true
				}
			}
		}

		// STEP 2: Collect anchor dependencies from suppressed spikes
		var suppressedDependencies []anchorDependency
		for _, spike := range displaySpikes {
			captures := captureStore.GetCaptures(spike.EventID)
			for _, capture := range captures {
				if capture.SuppressedByCooldown && capture.CooldownInfo != nil && capture.CooldownInfo.LastSuccessfulCaptureAt != nil {
					suppressedDependencies = append(suppressedDependencies, anchorDependency{
						suppressedEventID: spike.EventID,
						anchorCaptureID:   capture.CooldownInfo.AnchorCaptureID,
						anchorEventID:     capture.CooldownInfo.CooldownSourceSpikeID,
						anchorTargetID:    capture.CooldownInfo.AnchorTargetID,
						anchorProbeKind:   capture.CooldownInfo.AnchorProbeKind,
					})
				}
			}
		}

		// STEP 3: Determine which anchors need to be pinned
		anchorsToPin := make(map[string]*state.SpikeEventWithCaptures)
		allSpikesGetter := func(tid, k string, lim int) []state.SpikeEvent {
			return s.state.GetSpikes(tid, k, lim)
		}

		for _, dep := range suppressedDependencies {
			// Skip cross-probe suppression - those use embedded summaries
			if dep.anchorProbeKind != kind && dep.anchorProbeKind != "" {
				continue
			}
			if dep.anchorTargetID != targetID {
				continue
			}

			anchorTime := time.Time{}
			if dep.anchorCaptureID != "" {
				captures := captureStore.GetCaptures(dep.anchorCaptureID)
				for _, c := range captures {
					if c.CaptureStartedAt.After(time.Time{}) {
						anchorTime = c.CaptureStartedAt
						break
					}
				}
			}

			if visibleAnchorTimestamps[anchorTime] || visibleCapturedEventIDs[dep.anchorCaptureID] {
				continue
			}
			if _, alreadyPinned := anchorsToPin[dep.anchorCaptureID]; alreadyPinned {
				continue
			}

			anchorSpike := findAnchorSpike(dep.anchorCaptureID, dep.anchorTargetID, dep.anchorProbeKind, allSpikesGetter, captureStore)
			if anchorSpike != nil {
				anchorsToPin[dep.anchorCaptureID] = anchorSpike
			}
		}

		// STEP 4: Get visible cross-probe spikes for cross-probe visibility check
		// For HTTP query: get visible ICMP spikes; for ICMP query: get visible HTTP spikes
		crossProbeKind := "http"
		if kind == "http" {
			crossProbeKind = "icmp"
		}
		visibleCrossProbeSpikes := s.state.GetSpikes(targetID, crossProbeKind, limit)

		// STEP 5: Build pinned anchors list (sorted by timestamp for stable ordering)
		var pinnedAnchors []state.SpikeEventWithCaptures
		for _, anchor := range anchorsToPin {
			for i := range anchor.Captures {
				if anchor.Captures[i].CaptureStatus == state.CaptureStatusCaptured {
					break
				}
			}
			pinnedAnchors = append(pinnedAnchors, *anchor)
		}
		sortPinnedAnchorsByTimestamp(pinnedAnchors)

		// STEP 6: Process display spikes with anchor visibility logic
		spikesWithCaptures := make([]state.SpikeEventWithCaptures, len(displaySpikes))
		for i, spike := range displaySpikes {
			captures := captureStore.GetCaptures(spike.EventID)
			for j := range captures {
				capture := &captures[j]
				if capture.SuppressedByCooldown && capture.CooldownInfo != nil {
					capture.CooldownInfo = processSuppressedCaptureAnchorVisibility(
						capture.CooldownInfo, kind, targetID,
						visibleAnchorTimestamps, visibleCapturedEventIDs,
						anchorsToPin, allSpikesGetter, captureStore,
						visibleCrossProbeSpikes,
					)
				}
			}
			spikesWithCaptures[i] = state.SpikeEventWithCaptures{
				SpikeEvent: spike,
				Captures:   captures,
			}
		}

		json.NewEncoder(w).Encode(SpikeResponseWithCaptures{
			Spikes:       spikesWithCaptures,
			Count:        len(spikesWithCaptures),
			Retention:    retention,
			PinnedAnchors: pinnedAnchors,
		})
	} else {
		json.NewEncoder(w).Encode(SpikeResponse{
			Spikes: displaySpikes,
			Count:  len(displaySpikes),
		})
	}
}
