package diag

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

type CaptureService struct {
	cfg         *config.DiagnosticsConfig
	captures    *state.CaptureStore
	httpClient  *http.Client
	targetPeers map[string]*config.DiagPeerConfig
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

func NewCaptureService(cfg *config.DiagnosticsConfig, captureStore *state.CaptureStore) *CaptureService {
	timeout := cfg.TimeoutMs
	if timeout <= 0 {
		timeout = config.DefaultDiagTimeoutMs
	}

	return &CaptureService{
		cfg:         cfg,
		captures:    captureStore,
		httpClient:  &http.Client{Timeout: time.Duration(timeout) * time.Millisecond},
		targetPeers: cfg.TargetToDiagPeers(),
		stopCh:      make(chan struct{}),
	}
}

// TriggerCapture triggers a diagnostic capture for the given spike event.
// probeKind should be "http" or "icmp" for provenance tracking.
func (cs *CaptureService) TriggerCapture(eventID, targetID, probeKind string) {
	if !cs.cfg.Enabled || !cs.cfg.CaptureOnSpike {
		cs.recordDisabledCapture(eventID, targetID)
		return
	}

	peer, exists := cs.targetPeers[targetID]
	if !exists {
		cs.recordNoPeerMappingCapture(eventID, targetID)
		return
	}

	now := time.Now().UTC()

	// Check in-flight reservation
	if !cs.captures.ReserveInFlight(peer.Name) {
		cs.recordSuppressedInFlight(eventID, peer, targetID, now)
		return
	}

	// Evaluate cooldown using the authoritative shared decision function.
	// This ensures the skip decision AND the exported cooldown_info are consistent.
	decision := cs.captures.EvaluateCooldown(now, peer.Name, cs.cfg.CooldownSeconds)
	if decision.IsInCooldown {
		cs.captures.ReleaseInFlight(peer.Name)
		cs.recordSuppressedCooldown(eventID, peer, targetID, now, decision)
		return
	}

	// Trigger async capture
	// Capture variables for the goroutine to avoid closure issues
	targetIDForCapture := targetID
	probeKindForCapture := probeKind
	cs.wg.Add(1)
	go func() {
		defer cs.wg.Done()
		capture := cs.performCapture(peer)
		// Use AddCaptureWithProvenance to include target/probe context for root-cause analysis
		cs.captures.AddCaptureWithProvenance(eventID, capture, targetIDForCapture, probeKindForCapture)
		cs.captures.ReleaseInFlight(peer.Name)
	}()
}

func (cs *CaptureService) performCapture(peer *config.DiagPeerConfig) state.DiagCapture {
	capture := state.DiagCapture{
		Source:               peer.Name,
		BaseURL:              peer.BaseURL,
		CaptureStartedAt:     time.Now().UTC(),
		Status:               state.DiagCaptureStatusOK,
		EffectiveCaptureURL: config.DiagPeerStatusURL(peer.BaseURL),
	}

	statusURL := capture.EffectiveCaptureURL

	// Extract sanitized path+query for error evidence (no credentials/host)
	u, _ := url.Parse(statusURL)
	var sanitizedPath string
	if u != nil {
		sanitizedPath = u.Path
		if u.RawQuery != "" {
			sanitizedPath += "?" + u.RawQuery
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cs.cfg.TimeoutMs)*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		capture.Status = state.DiagCaptureStatusError
		capture.Error = SafeErrorMessage(fmt.Sprintf("request creation failed: %v", err))
		capture.RequestedPath = &sanitizedPath
		finishCapture(&capture)
		return capture
	}

	resp, err := cs.httpClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			capture.Status = state.DiagCaptureStatusTimeout
			capture.Error = SafeErrorMessage("request timed out")
		} else {
			capture.Status = state.DiagCaptureStatusError
			capture.Error = SafeErrorMessage(fmt.Sprintf("request failed: %v", err))
		}
		capture.RequestedPath = &sanitizedPath
		finishCapture(&capture)
		return capture
	}
	defer resp.Body.Close()

	// Record HTTP status code for all responses (including errors)
	capture.HTTPStatusCode = &resp.StatusCode

	if resp.StatusCode != http.StatusOK {
		capture.Status = state.DiagCaptureStatusError
		if resp.StatusCode == http.StatusNotFound {
			capture.Error = SafeErrorMessage("Capture request returned HTTP 404 (check base_url is origin-only, not full path)")
		} else {
			capture.Error = SafeErrorMessage(fmt.Sprintf("Capture request returned HTTP %d", resp.StatusCode))
		}
		capture.RequestedPath = &sanitizedPath
		finishCapture(&capture)
		return capture
	}

	var tovarischResp TovarischStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&tovarischResp); err != nil {
		capture.Status = state.DiagCaptureStatusError
		capture.Error = SafeErrorMessage(fmt.Sprintf("parse failed: %v", err))
		capture.RequestedPath = &sanitizedPath
		finishCapture(&capture)
		return capture
	}

	// Only set CaptureStatusCaptured AFTER successful network diag attachment
	// This ensures we don't claim success prematurely
	if tovarischResp.NetworkDiag != nil {
		capture.NetworkDiag = tovarischResp.NetworkDiag
		capture.CaptureStatus = state.CaptureStatusCaptured
	}

	finishCapture(&capture)
	return capture
}

func finishCapture(capture *state.DiagCapture) {
	now := time.Now().UTC()
	capture.CaptureFinishedAt = &now
	duration := now.Sub(capture.CaptureStartedAt).Milliseconds()
	capture.DurationMs = &duration
}

func (cs *CaptureService) recordDisabledCapture(eventID, targetID string) {
	capture := state.DiagCapture{
		CaptureStartedAt: time.Now().UTC(),
		Status:           state.DiagCaptureStatusDisabled,
	}
	finishCapture(&capture)
	cs.captures.AddCapture(eventID, capture)
}

func (cs *CaptureService) recordNoPeerMappingCapture(eventID, targetID string) {
	capture := state.DiagCapture{
		CaptureStartedAt: time.Now().UTC(),
		Status:           state.DiagCaptureStatusNoPeerMapping,
	}
	finishCapture(&capture)
	cs.captures.AddCapture(eventID, capture)
}

// recordSuppressedInFlight records an in-flight suppression (another capture currently running).
// This is NOT a cooldown scenario, so it records not_attempted status.
func (cs *CaptureService) recordSuppressedInFlight(eventID string, peer *config.DiagPeerConfig, targetID string, now time.Time) {
	capture := state.DiagCapture{
		Source:           peer.Name,
		BaseURL:          peer.BaseURL,
		CaptureStartedAt: now,
		Status:           state.DiagCaptureStatusUnavailable,
		CaptureStatus:    state.CaptureStatusNotAttempted,
	}
	finishCapture(&capture)
	cs.captures.AddCapture(eventID, capture)
}

// recordSuppressedCooldown records a cooldown suppression using the authoritative decision.
// The cooldown_info is built from the same decision that determined the skip,
// ensuring metadata exactly matches the decision logic.
func (cs *CaptureService) recordSuppressedCooldown(eventID string, peer *config.DiagPeerConfig, targetID string, now time.Time, decision state.CaptureCooldownDecision) {
	// Build cooldown_info from the authoritative decision
	cooldownInfo := state.BuildCooldownInfoFromDecision(decision, peer.Name)
	
	capture := state.DiagCapture{
		Source:               peer.Name,
		BaseURL:              peer.BaseURL,
		CaptureStartedAt:     now,
		Status:               state.DiagCaptureStatusOK,
		SuppressedByCooldown: true,
		CaptureStatus:        state.CaptureStatusSkippedCooldown,
		CooldownInfo:         cooldownInfo,
	}
	finishCapture(&capture)
	cs.captures.AddCapture(eventID, capture)
}

func SafeErrorMessage(raw string) *string {
	safe := raw
	if len(safe) > 200 {
		safe = safe[:200]
	}
	// Remove any potential sensitive patterns
	safe = strings.ReplaceAll(safe, "\n", " ")
	safe = strings.ReplaceAll(safe, "\r", "")
	return &safe
}

type TovarischStatusResponse struct {
	NetworkDiag *state.NetworkDiagData `json:"network_diag,omitempty"`
}
