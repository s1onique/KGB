package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/state"
)

// AnchorStatus represents the availability status of an anchor capture.
type AnchorStatus string

const (
	// AnchorStatusAvailable means both capture artifact and spike event are available.
	AnchorStatusAvailable AnchorStatus = "available"
	// AnchorStatusArtifactMissing means anchor metadata exists but capture artifact has been purged.
	AnchorStatusArtifactMissing AnchorStatus = "artifact_missing"
	// AnchorStatusMetadataOnly means only partial anchor metadata is available.
	AnchorStatusMetadataOnly AnchorStatus = "metadata_only"
	// AnchorStatusNotAnchor means the requested capture is not a cooldown anchor.
	AnchorStatusNotAnchor AnchorStatus = "not_an_anchor_capture"
	// AnchorStatusNotFound means the anchor could not be found.
	AnchorStatusNotFound AnchorStatus = "not_found"
)

// AnchorCaptureResponse represents a response containing anchor capture details.
type AnchorCaptureResponse struct {
	// Capture is the full capture record if available.
	Capture *state.DiagCapture `json:"capture,omitempty"`
	// SpikeEvent is the spike event associated with the capture if available.
	SpikeEvent *state.SpikeEvent `json:"spike_event,omitempty"`
	// Anchor is the cooldown anchor metadata that justified the suppression.
	Anchor *state.CaptureCooldownAnchor `json:"anchor,omitempty"`
	// Status indicates the availability state of the anchor.
	Status AnchorStatus `json:"status"`
	// Message provides human-readable context about availability.
	Message string `json:"message,omitempty"`
	// Degraded indicates the response has reduced information (artifact or spike missing).
	Degraded bool `json:"degraded"`
	// DegradationReason explains why degraded=true.
	// Values: "artifact_purged", "spike_event_evicted", "missing_provenance", "partial_metadata"
	DegradationReason string `json:"degradation_reason,omitempty"`
}

// handleGetAnchorCapture returns the anchor capture details for a skipped cooldown spike.
// This allows operators to inspect the prior successful capture that justified suppression,
// even when the anchor spike is outside the current visible window.
//
// GET /api/v1/captures/{capture_id}/anchor
//
// Response:
// - 200 OK with anchor capture details
// - 404 if capture_id not found
// - 400 if capture_id is not a valid anchor capture
func (s *Server) handleGetAnchorCapture(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	captureID := vars["capture_id"]

	if captureID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "capture_id is required"})
		return
	}

	captureStore := s.state.GetCaptureStore()

	// Get the capture record
	captures := captureStore.GetCaptures(captureID)
	if len(captures) == 0 {
		// Capture not found - try looking up by anchor capture ID in cooldown anchors
		// This handles the case where the anchor spike was evicted but anchor metadata exists
		anchors := captureStore.GetAllLastCaptureAnchors()
		for _, anchor := range anchors {
			if anchor.AnchorCaptureID == captureID {
				// Found anchor by its capture ID
				// Check if the actual capture record still exists
				anchorCaptures := captureStore.GetCaptures(anchor.AnchorCaptureID)
				if len(anchorCaptures) > 0 {
					// Capture artifact exists
					response := AnchorCaptureResponse{
						Capture:           &anchorCaptures[0],
						Anchor:            &anchor,
						Status:            AnchorStatusAvailable,
						Message:           "Anchor capture artifact is available",
						Degraded:          false,
						DegradationReason: "",
					}
					json.NewEncoder(w).Encode(response)
					return
				} else {
					// Anchor metadata exists but artifact is gone - degraded state
					response := AnchorCaptureResponse{
						Anchor:            &anchor,
						Status:            AnchorStatusArtifactMissing,
						Message:           "Anchor metadata retained but capture artifact has been purged",
						Degraded:          true,
						DegradationReason: "artifact_purged",
					}
					json.NewEncoder(w).Encode(response)
					return
				}
			}
		}

		// Capture not found anywhere
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "capture not found"})
		return
	}

	capture := captures[0]

	// Verify this is a cooldown anchor capture (successful capture that started cooldown)
	if !isCooldownAnchorCapture(capture) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "not_an_anchor_capture",
			"message": "This capture is not a cooldown anchor capture",
		})
		return
	}

	// Get the spike event if available
	// Use the probe kind from the anchor metadata to query the correct spike type
	probeKind := "http" // default
	if storedAnchor := captureStore.GetLastCaptureAnchor(capture.Source); storedAnchor.AnchorProbeKind != "" {
		probeKind = storedAnchor.AnchorProbeKind
	}
	var spikeEvent *state.SpikeEvent
	spikes := s.state.GetSpikes(capture.Source, probeKind, 0)
	for _, spike := range spikes {
		if spike.EventID == captureID {
			sp := spike
			spikeEvent = &sp
			break
		}
	}

	// Get anchor metadata
	var anchor *state.CaptureCooldownAnchor
	storedAnchor := captureStore.GetLastCaptureAnchor(capture.Source)
	if storedAnchor.AnchorCaptureID == captureID {
		anchorCopy := storedAnchor
		anchor = &anchorCopy
	}

	// Determine if degraded (has artifact but no spike event, or spike event outside retention)
	degraded := spikeEvent == nil || anchor == nil

	response := AnchorCaptureResponse{
		Capture:           &capture,
		SpikeEvent:        spikeEvent,
		Anchor:            anchor,
		Status:            AnchorStatusAvailable,
		Degraded:          degraded,
		DegradationReason: "",
	}

	if degraded {
		if anchor != nil && spikeEvent == nil {
			response.Message = "Anchor capture artifact is available but spike event is outside retention window"
			response.DegradationReason = "spike_event_evicted"
		} else if anchor == nil {
			response.Message = "Anchor capture is available but provenance metadata is incomplete"
			response.DegradationReason = "missing_provenance"
		}
	}

	json.NewEncoder(w).Encode(response)
}

// isCooldownAnchorCapture returns true if this capture is a successful capture
// that could have started a cooldown window.
func isCooldownAnchorCapture(capture state.DiagCapture) bool {
	// Must have successful status
	if capture.Status != state.DiagCaptureStatusOK {
		return false
	}
	// Must be captured (not skipped, failed, etc.)
	if capture.CaptureStatus != state.CaptureStatusCaptured {
		return false
	}
	// Must not be suppressed by cooldown
	if capture.SuppressedByCooldown {
		return false
	}
	return true
}

// handleGetCooldownAnchorForPeer returns the cooldown anchor for a specific peer.
// This allows operators to see what capture started the current cooldown for a peer.
//
// GET /api/v1/diagnostics/cooldown/anchors/{peer_name}
func (s *Server) handleGetCooldownAnchorForPeer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	peerName := vars["peer_name"]

	if peerName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "peer_name is required"})
		return
	}

	captureStore := s.state.GetCaptureStore()
	anchor := captureStore.GetLastCaptureAnchor(peerName)

	if anchor.AnchorCaptureID == "" {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "no_anchor",
			"message": "No cooldown anchor exists for this peer",
		})
		return
	}

	// Check if the capture artifact still exists
	captures := captureStore.GetCaptures(anchor.AnchorCaptureID)
	captureExists := len(captures) > 0

	// Build response with anchor details
	response := struct {
		Anchor        state.CaptureCooldownAnchor `json:"anchor"`
		CaptureExists bool                        `json:"capture_exists"`
		Degraded      bool                        `json:"degraded"`
		CheckedAt     string                      `json:"checked_at"`
	}{
		Anchor:        anchor,
		CaptureExists: captureExists,
		Degraded:      !captureExists,
		CheckedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	json.NewEncoder(w).Encode(response)
}
