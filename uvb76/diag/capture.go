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

func (cs *CaptureService) TriggerCapture(eventID, targetID string) {
	if !cs.cfg.Enabled || !cs.cfg.CaptureOnSpike {
		cs.recordDisabledCapture(eventID, targetID)
		return
	}

	peer, exists := cs.targetPeers[targetID]
	if !exists {
		cs.recordNoPeerMappingCapture(eventID, targetID)
		return
	}

	// Check in-flight reservation
	if !cs.captures.ReserveInFlight(peer.Name) {
		cs.recordSuppressedCapture(eventID, peer, targetID)
		return
	}

	// Check cooldown
	if cs.captures.IsInCooldown(peer.Name, cs.cfg.CooldownSeconds) {
		cs.captures.ReleaseInFlight(peer.Name)
		cs.recordSuppressedCapture(eventID, peer, targetID)
		return
	}

	// Trigger async capture
	cs.wg.Add(1)
	go func() {
		defer cs.wg.Done()
		capture := cs.performCapture(peer)
		cs.captures.AddCapture(eventID, capture)
		cs.captures.ReleaseInFlight(peer.Name)
	}()
}

func (cs *CaptureService) performCapture(peer *config.DiagPeerConfig) state.DiagCapture {
	capture := state.DiagCapture{
		Source:           peer.Name,
		BaseURL:          peer.BaseURL,
		CaptureStartedAt: time.Now().UTC(),
		Status:           state.DiagCaptureStatusOK,
	}

	statusURL := config.DiagPeerStatusURL(peer.BaseURL)

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

	if resp.StatusCode != http.StatusOK {
		capture.Status = state.DiagCaptureStatusError
		if resp.StatusCode == http.StatusNotFound {
			capture.Error = SafeErrorMessage("Capture request returned HTTP 404")
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

	if tovarischResp.NetworkDiag != nil {
		capture.NetworkDiag = tovarischResp.NetworkDiag
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

func (cs *CaptureService) recordSuppressedCapture(eventID string, peer *config.DiagPeerConfig, targetID string) {
	capture := state.DiagCapture{
		Source:               peer.Name,
		BaseURL:              peer.BaseURL,
		CaptureStartedAt:     time.Now().UTC(),
		Status:               state.DiagCaptureStatusOK,
		SuppressedByCooldown: true,
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
