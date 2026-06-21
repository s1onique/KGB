// Package polling provides typed polling logic for UVB-76 capture netns lab.
//
// This package owns:
//   - API polling loop with context.Context deadline
//   - timeout/deadline handling
//   - capture status extraction
//   - terminal condition decisions
//   - JSON artifact reading/writing for polling results
//   - deterministic failure messages
//
// The shell script should not parse JSON or decide polling state after migration.
package polling

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// HTTPClient defines the interface for making HTTP requests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// DefaultHTTPClient wraps the standard http.Client.
type DefaultHTTPClient struct {
	Client *http.Client
}

// Do implements HTTPClient.
func (c *DefaultHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.Client.Do(req)
}

// APIClient makes authenticated HTTP requests to the UVB-76 API.
type APIClient struct {
	BaseURL    string
	Username   string
	Password   string
	HTTPClient HTTPClient
	Cookies    string // path to cookie file for session persistence
}

// NewAPIClient creates a new API client.
func NewAPIClient(baseURL, username, password string) *APIClient {
	return &APIClient{
		BaseURL:  strings.TrimSuffix(baseURL, "/"),
		Username: username,
		Password: password,
		HTTPClient: &DefaultHTTPClient{
			Client: &http.Client{Timeout: 10 * time.Second},
		},
	}
}

// FetchLatencySeries fetches latency series from the API.
func (c *APIClient) FetchLatencySeries(ctx context.Context, targetID, probeKind string, rangeSeconds int) (*LatencySeries, error) {
	url := fmt.Sprintf("%s/api/v1/latency/series?target_id=%s&probe_kind=%s&range_seconds=%d",
		c.BaseURL, targetID, probeKind, rangeSeconds)

	body, err := c.get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch latency series: %w", err)
	}

	var series LatencySeries
	if err := json.Unmarshal(body, &series); err != nil {
		return nil, fmt.Errorf("parse latency series: %w", err)
	}
	return &series, nil
}

// FetchSpikes fetches spike events from the API.
func (c *APIClient) FetchSpikes(ctx context.Context, targetID string, includeCaptures bool, limit int) (*SpikesResponse, error) {
	url := fmt.Sprintf("%s/api/v1/latency/spikes?target_id=%s&include_captures=%t&limit=%d",
		c.BaseURL, targetID, includeCaptures, limit)

	body, err := c.get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch spikes: %w", err)
	}

	var spikes SpikesResponse
	if err := json.Unmarshal(body, &spikes); err != nil {
		return nil, fmt.Errorf("parse spikes: %w", err)
	}
	return &spikes, nil
}

func (c *APIClient) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.SetBasicAuth(c.Username, c.Password)

	// Load cookies from file if available
	if c.Cookies != "" {
		c.loadCookies(req)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	// Save cookies to file
	if c.Cookies != "" {
		c.saveCookies(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *APIClient) loadCookies(req *http.Request) {
	if c.Cookies == "" {
		return
	}
	data, err := os.ReadFile(c.Cookies)
	if err != nil {
		return
	}
	// Parse Netscape cookie format
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 7 {
			continue
		}
		// parts[0]=domain, parts[1]=tailmatch, parts[2]=path, parts[3]=secure, parts[4]=expires, parts[5]=name, parts[6]=value
		cookie := &http.Cookie{
			Name:  parts[5],
			Value: parts[6],
		}
		req.AddCookie(cookie)
	}
}

func (c *APIClient) saveCookies(resp *http.Response) {
	if c.Cookies == "" || len(resp.Cookies()) == 0 {
		return
	}
	// Append cookies to existing file
	f, err := os.OpenFile(c.Cookies, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	for _, cookie := range resp.Cookies() {
		// Write in Netscape cookie format
		fmt.Fprintf(f, ".uvb76\tTRUE\t/\tFALSE\t%d\t%s\t%s\n",
			time.Now().Add(24*time.Hour).Unix(), cookie.Name, cookie.Value)
	}
}

// Poller provides the polling logic with injectable clock for testing.
type Poller struct {
	APIClient    *APIClient
	ArtifactWriter ArtifactWriter
	Clock        Clock
}

// Clock defines the interface for time operations.
type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

// RealClock uses real time.
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time { return time.Now() }

// Sleep pauses execution.
func (RealClock) Sleep(d time.Duration) { time.Sleep(d) }

// NewPoller creates a new Poller with real clock.
func NewPoller(client *APIClient, writer ArtifactWriter) *Poller {
	return &Poller{
		APIClient:      client,
		ArtifactWriter: writer,
		Clock:          RealClock{},
	}
}

// PollProbeSamples polls for probe samples until success or timeout.
func (p *Poller) PollProbeSamples(ctx context.Context, targetID, probeKind string, rangeSeconds int, cfg PollConfig) ProbePollResult {
	deadline := p.Clock.Now().Add(cfg.Timeout)
	interval := cfg.Interval
	var lastErr error

	for {
		select {
		case <-ctx.Done():
			return ProbePollResult{Timeout: true, LastError: lastErr, Error: ctx.Err()}
		default:
		}

		if p.Clock.Now().After(deadline) {
			return ProbePollResult{Timeout: true, LastError: lastErr}
		}

		series, err := p.APIClient.FetchLatencySeries(ctx, targetID, probeKind, rangeSeconds)
		if err != nil {
			lastErr = err
			fmt.Fprintf(os.Stderr, "[WARN] probe polling API error: %v\n", err)
			p.Clock.Sleep(interval)
			continue
		}

		// Check sample count
		sampleCount := series.RetainedSampleCount
		if sampleCount == 0 {
			sampleCount = series.SampleCount // fallback to deprecated field
		}

		if sampleCount >= cfg.RequireCount {
			// Success
			if p.ArtifactWriter != nil {
				if err := p.ArtifactWriter.WriteProbeReadyArtifact(series); err != nil {
					fmt.Fprintf(os.Stderr, "[WARN] failed to write probe ready artifact: %v\n", err)
				}
			}
			return ProbePollResult{
				OK:           true,
				SampleCount:  sampleCount,
				PointCount:   series.ReturnedPointCount,
				LastResponse: series,
			}
		}

		p.Clock.Sleep(interval)
	}
}

// PollSpikeEvent polls for spike event with matching reason pattern.
func (p *Poller) PollSpikeEvent(ctx context.Context, targetID, cursor, reasonRegex string, cfg SpikeEventConfig) SpikeEventResult {
	deadline := p.Clock.Now().Add(cfg.Timeout)
	interval := cfg.Interval
	var lastErr error

	reasonRe, err := regexp.Compile(reasonRegex)
	if err != nil {
		return SpikeEventResult{Error: fmt.Errorf("invalid reason regex: %w", err)}
	}

	for {
		select {
		case <-ctx.Done():
			return SpikeEventResult{Timeout: true, LastError: lastErr, Error: ctx.Err()}
		default:
		}

		if p.Clock.Now().After(deadline) {
			return SpikeEventResult{Timeout: true, LastError: lastErr}
		}

		spikes, err := p.APIClient.FetchSpikes(ctx, targetID, true, 20)
		if err != nil {
			lastErr = err
			fmt.Fprintf(os.Stderr, "[WARN] spike polling API error: %v\n", err)
			p.Clock.Sleep(interval)
			continue
		}

		// Search for matching spike event
		for _, spike := range spikes.Spikes {
			// Check if spike is after cursor
			if cursor != "" && !IsAfterCursor(spike.SampleTS, cursor) {
				continue
			}

			// Check if reasons match
			for _, reason := range spike.Reasons {
				if reasonRe.MatchString(reason) {
					// Found matching event
					if p.ArtifactWriter != nil {
						if err := p.ArtifactWriter.WriteSpikeEventArtifact(spikes); err != nil {
							fmt.Fprintf(os.Stderr, "[WARN] failed to write spike event artifact: %v\n", err)
						}
					}
					return SpikeEventResult{
						OK:           true,
						EventID:      spike.EventID,
						Reasons:      spike.Reasons,
						LastResponse: spikes,
					}
				}
			}
		}

		p.Clock.Sleep(interval)
	}
}

// PollCaptureForEvent polls for capture status for a specific event.
func (p *Poller) PollCaptureForEvent(ctx context.Context, targetID, eventID string, cfg CapturePollConfig) CaptureResult {
	deadline := p.Clock.Now().Add(cfg.Timeout)
	interval := cfg.Interval
	var lastErr error

	for {
		select {
		case <-ctx.Done():
			return CaptureResult{Timeout: true, LastError: lastErr, Error: ctx.Err()}
		default:
		}

		if p.Clock.Now().After(deadline) {
			return CaptureResult{Timeout: true, LastError: lastErr}
		}

		spikes, err := p.APIClient.FetchSpikes(ctx, targetID, true, 20)
		if err != nil {
			lastErr = err
			fmt.Fprintf(os.Stderr, "[WARN] capture polling API error: %v\n", err)
			p.Clock.Sleep(interval)
			continue
		}

		// Find the spike for this event
		var spike *Spike
		for i := range spikes.Spikes {
			if spikes.Spikes[i].EventID == eventID {
				spike = &spikes.Spikes[i]
				break
			}
		}

		if spike == nil {
			p.Clock.Sleep(interval)
			continue
		}

		// Check captures
		if len(spike.Captures) == 0 {
			p.Clock.Sleep(interval)
			continue
		}

		capture := &spike.Captures[0]
		lifecycleStatus := capture.ExtractLifecycleStatus()
		normalizedStatus := capture.ExtractNormalizedStatus()
		diagStatus := capture.Status

		// Write capture metadata artifact
		if p.ArtifactWriter != nil {
			if err := p.ArtifactWriter.WriteCaptureArtifact(spikes); err != nil {
				fmt.Fprintf(os.Stderr, "[WARN] failed to write capture artifact: %v\n", err)
			}
		}

		// Check if capture meets requirements
		if cfg.RequireCapture {
			if normalizedStatus != StatusCaptured {
				return CaptureResult{
					OK:             false,
					CaptureStatus:  normalizedStatus,
					RawStatus:      lifecycleStatus,
					DiagStatus:     diagStatus,
					CaptureStarted: capture.CaptureStartedAt,
					LastResponse:   spikes,
					FailureReason:  fmt.Sprintf("capture_status is '%s' (normalized: %s), expected 'captured'", lifecycleStatus, normalizedStatus),
				}
			}
		}

		// Success
		return CaptureResult{
			OK:             true,
			CaptureStatus:  normalizedStatus,
			RawStatus:      lifecycleStatus,
			DiagStatus:     diagStatus,
			CaptureStarted: capture.CaptureStartedAt,
			LastResponse:   spikes,
		}
	}
}
