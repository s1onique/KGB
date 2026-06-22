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
	cfg            *config.DiagnosticsConfig
	captures       *state.CaptureStore
	httpClient     *http.Client
	targetPeers    map[string]*config.DiagPeerConfig
	routeCollector *RouteCollector
	tcpCollector   *TcpQualityCollector
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

func NewCaptureService(cfg *config.DiagnosticsConfig, captureStore *state.CaptureStore) *CaptureService {
	timeout := cfg.TimeoutMs
	if timeout <= 0 {
		timeout = config.DefaultDiagTimeoutMs
	}

	return &CaptureService{
		cfg:            cfg,
		captures:       captureStore,
		httpClient:     &http.Client{Timeout: time.Duration(timeout) * time.Millisecond},
		targetPeers:    cfg.TargetToDiagPeers(),
		routeCollector: NewRouteCollector(),
		tcpCollector:   NewTcpQualityCollector(),
		stopCh:         make(chan struct{}),
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
	decision := cs.captures.EvaluateCooldown(now, peer.Name, cs.cfg.CooldownSeconds)
	if decision.IsInCooldown {
		cs.captures.ReleaseInFlight(peer.Name)
		cs.recordSuppressedCooldown(eventID, peer, targetID, now, decision, probeKind)
		return
	}

	// Trigger async capture
	targetIDForCapture := targetID
	probeKindForCapture := probeKind
	cs.wg.Add(1)
	go func() {
		defer cs.wg.Done()
		capture := cs.performCapture(peer, probeKindForCapture)
		cs.captures.AddCaptureWithProvenance(eventID, capture, targetIDForCapture, probeKindForCapture)
		cs.captures.ReleaseInFlight(peer.Name)
	}()
}

// probeKindToRouteKind converts a probe kind string to a ProbeRouteKind.
func probeKindToRouteKind(probeKind string) state.ProbeRouteKind {
	switch strings.ToLower(probeKind) {
	case "icmp":
		return state.ProbeRouteKindICMP
	default:
		return state.ProbeRouteKindHTTP
	}
}

func (cs *CaptureService) performCapture(peer *config.DiagPeerConfig, probeKind string) state.DiagCapture {
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

	// Collect route evidence early, before the HTTP request.
	// Route lookup failures do NOT block the diagnostic capture.
	// This is valuable even when the diagnostic endpoint is unreachable.
	routeKind := probeKindToRouteKind(probeKind)
	capture.ProbeRoute = cs.collectProbeRoute(ctx, peer, routeKind)

	// Collect TCP quality evidence for the probe destination.
	// TCP quality collection failures do NOT block the diagnostic capture.
	// For HTTP probes, we collect TCP socket metrics for the destination.
	// For ICMP probes, TCP quality is unavailable (TCP is HTTP/TCP-only).
	host := extractHostFromURL(peer.BaseURL)
	capture.TcpQuality = cs.tcpCollector.CollectTcpQuality(ctx, probeKind, host)

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

	if tovarischResp.NetworkDiag != nil {
		capture.NetworkDiag = tovarischResp.NetworkDiag
		capture.CaptureStatus = state.CaptureStatusCaptured

		if len(tovarischResp.NetworkDiag.UnderlayTCP) == 0 && len(tovarischResp.NetworkDiag.Events) > 0 {
			capture.TcpAbsenceEvents = buildTcpAbsenceEvents(tovarischResp.NetworkDiag.Events, peer)
		}
	}

	finishCapture(&capture)
	return capture
}

// collectProbeRoute performs a route lookup for the probe destination.
func (cs *CaptureService) collectProbeRoute(ctx context.Context, peer *config.DiagPeerConfig, routeKind state.ProbeRouteKind) *state.ProbeRoute {
	// Use the host from BaseURL as the probe destination for route lookup.
	// BaseURL contains the actual probe endpoint (e.g., http://10.0.0.5:8080).
	// The route lookup should answer: "Which path would this packet take from the router to 10.0.0.5?"
	host := extractHostFromURL(peer.BaseURL)
	if host == "" {
		return &state.ProbeRoute{
			Kind:        routeKind,
			Ok:          false,
			ErrorKind:   state.RouteLookupErrorUnavailable,
			Error:       "cannot extract host from peer base_url",
			CollectedAt: time.Now().UTC().Format(time.RFC3339),
		}
	}

	// RouteCollector is always wired in NewCaptureService
	return cs.routeCollector.CollectRouteLookup(ctx, routeKind, host, host)
}

// extractHostFromURL extracts the hostname from a URL string.
func extractHostFromURL(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
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

func (cs *CaptureService) recordSuppressedCooldown(eventID string, peer *config.DiagPeerConfig, targetID string, now time.Time, decision state.CaptureCooldownDecision, probeKind string) {
	cooldownInfo := state.BuildCooldownInfoFromDecision(decision, peer.Name)

	if cooldownInfo != nil {
		cooldownInfo.SuppressedProbeKind = probeKind
		if cooldownInfo.AnchorProbeKind != "" && probeKind != "" && cooldownInfo.AnchorProbeKind != probeKind {
			cooldownInfo.IsCrossProbeSuppression = true
		}
	}

	capture := state.DiagCapture{
		Source:               peer.Name,
		BaseURL:              peer.BaseURL,
		CaptureStartedAt:      now,
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
	safe = strings.ReplaceAll(safe, "\n", " ")
	safe = strings.ReplaceAll(safe, "\r", "")
	return &safe
}

type TovarischStatusResponse struct {
	NetworkDiag *state.NetworkDiagData `json:"network_diag,omitempty"`
}

type tcpAbsenceEventFields struct {
	Reason        string `json:"reason"`
	Detail        string `json:"detail,omitempty"`
	ExpectedPeer  string `json:"expected_peer,omitempty"`
	ExpectedPort  *int   `json:"expected_port,omitempty"`
	ProbeKind     string `json:"probe_kind,omitempty"`
	CommandTool   string `json:"command_tool,omitempty"`
	RawMatchCount *int   `json:"raw_match_count,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	ExitCode      *int   `json:"exit_code,omitempty"`
}

func buildTcpAbsenceEvents(events []state.DiagEventData, peer *config.DiagPeerConfig) []state.TcpAbsenceEvent {
	var absenceEvents []state.TcpAbsenceEvent

	for _, event := range events {
		if event.Source != "underlay_tcp" {
			continue
		}

		absenceEvent := parseEventFields(event, peer)
		absenceEvents = append(absenceEvents, absenceEvent)
	}

	return absenceEvents
}

func parseEventFields(event state.DiagEventData, peer *config.DiagPeerConfig) state.TcpAbsenceEvent {
	absenceEvent := state.TcpAbsenceEvent{
		Source: event.Source,
		Detail: event.Message,
	}

	if peer != nil {
		absenceEvent.ExpectedPeer = peer.Name
	}

	if event.Fields == nil || *event.Fields == "" {
		absenceEvent.ReasonCode = "no_matching_socket"
		return absenceEvent
	}

	var fields tcpAbsenceEventFields
	if err := json.Unmarshal([]byte(*event.Fields), &fields); err != nil {
		absenceEvent.ReasonCode = "parse_failed"
		if absenceEvent.Detail == "" {
			absenceEvent.Detail = "failed to parse underlay_tcp event fields"
		}
		return absenceEvent
	}

	if fields.Reason != "" {
		absenceEvent.ReasonCode = fields.Reason
	}
	if fields.Detail != "" {
		absenceEvent.Detail = fields.Detail
	}
	if fields.ExpectedPeer != "" {
		absenceEvent.ExpectedPeer = fields.ExpectedPeer
	}
	if fields.ExpectedPort != nil {
		absenceEvent.ExpectedPort = fields.ExpectedPort
	}
	if fields.ProbeKind != "" {
		absenceEvent.ProbeKind = fields.ProbeKind
	}
	if fields.Namespace != "" {
		absenceEvent.Namespace = fields.Namespace
	}
	if fields.RawMatchCount != nil {
		absenceEvent.RawMatchCount = fields.RawMatchCount
	}

	if fields.CommandTool != "" {
		absenceEvent.CommandTool = fields.CommandTool
	} else if fields.ExitCode != nil {
		absenceEvent.CommandTool = fmt.Sprintf("ss (exit=%d)", *fields.ExitCode)
	}

	return absenceEvent
}
