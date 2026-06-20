// Package workload runs the accelerated latency query workload against UVB-76.
package workload

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/s1onique/KGB/uvb76/state"
)

// Result captures workload execution metrics.
type Result struct {
	SampleValid     bool `json:"sample_valid"`
	SummaryValid   bool `json:"summary_valid"`
	SeriesValid     bool `json:"series_valid"`
	SampleCount     int  `json:"sample_count"`
	RequestsTotal   int  `json:"requests_total"`
	RequestsFailed  int  `json:"requests_failed"`
	MaxSampleCount  int  `json:"max_sample_count"`
}

// Workload executes the crash lab workload.
type Workload struct {
	port          string
	user          string
	pass          string
	targetID      string
	requestLimit  int
	artifactDir  string
	client        *http.Client
	sessionCookie string
}

// New creates a new workload runner.
func New(port, user, pass, targetID string, requestLimit int, artifactDir string) *Workload {
	return &Workload{
		port:         port,
		user:         user,
		pass:         pass,
		targetID:     targetID,
		requestLimit: requestLimit,
		artifactDir: artifactDir,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Run executes the workload continuously during the specified duration.
// Queries are made throughout the duration, not just at the end.
// This exercises the /latency/series endpoint specifically, which was the crash site
// for the SIGSEGV at uvb76/server/latency.go:418.
func (w *Workload) Run(durationSeconds int) Result {
	result := Result{}

	// Authenticate first
	if err := w.authenticate(); err != nil {
		log.Printf("Authentication failed: %v", err)
		result.RequestsFailed++
		return result
	}

	// Start concurrent workload goroutines
	var mu sync.Mutex
	var wg sync.WaitGroup
	
	// Track results across all goroutines
	maxSampleCount := 0
	sampleCount := 0
	sampleValid := true
	summaryValid := true
	seriesValid := true
	requestsTotal := 0
	requestsFailed := 0

	// Run requests continuously during the duration
	// Mix of /latency/samples and /latency/series endpoint hits
	interval := 500 * time.Millisecond // request every 500ms
	numGoroutines := 4                 // 4 concurrent requesters

	stopCh := make(chan struct{})
	
	// Start request goroutines - alternating between samples and series endpoints
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			requestNum := 0

			for {
				select {
				case <-stopCh:
					return
				case <-ticker.C:
					requestNum++
					// Alternate between endpoints to hit the crash path
					// The series endpoint is the crash site
					if requestNum%2 == 0 {
						// Query series endpoint - this is the crash path from latency.go:418
						seriesOK := w.querySeries()
						mu.Lock()
						requestsTotal++
						if !seriesOK {
							requestsFailed++
							seriesValid = false
						}
						mu.Unlock()
					} else {
						// Query samples endpoint
						count := w.querySamples(false)
						mu.Lock()
						requestsTotal++
						if count < 0 {
							requestsFailed++
						} else {
							sampleCount = count
							if count > maxSampleCount {
								maxSampleCount = count
							}
						}
						mu.Unlock()
					}
				}
			}
		}(i)
	}

	// Let workload run for duration
	log.Printf("Running continuous workload for %ds (%d concurrent requesters, %v interval)...",
		durationSeconds, numGoroutines, interval)
	
	time.Sleep(time.Duration(durationSeconds) * time.Second)

	// Stop goroutines
	close(stopCh)
	wg.Wait()

	// Make final queries to capture final state
	log.Printf("Capturing final state...")
	finalCount := w.querySamples(true)
	if finalCount < 0 {
		sampleValid = false
	} else {
		sampleCount = finalCount
		if finalCount > maxSampleCount {
			maxSampleCount = finalCount
		}
	}

	// Query summary
	if !w.querySummary() {
		summaryValid = false
	}

	// Query series - this is the specific crash path
	if !w.querySeries() {
		seriesValid = false
	}

	// Query spikes
	w.querySpikes()

	result = Result{
		SampleValid:    sampleValid,
		SummaryValid:   summaryValid,
		SeriesValid:    seriesValid,
		SampleCount:    sampleCount,
		RequestsTotal:  requestsTotal,
		RequestsFailed:  requestsFailed,
		MaxSampleCount:  maxSampleCount,
	}

	// Write final artifacts
	w.writeArtifacts(result)

	log.Printf("Workload complete: %d requests, %d failed, max %d samples, series_valid=%v",
		result.RequestsTotal, result.RequestsFailed, result.MaxSampleCount, seriesValid)

	return result
}

// authenticate performs session-based authentication.
func (w *Workload) authenticate() error {
	loginURL := fmt.Sprintf("http://localhost:%s/api/v1/auth/login", w.port)
	body := fmt.Sprintf(`{"username":"%s","password":"%s"}`, w.user, w.pass)

	req, err := http.NewRequest("POST", loginURL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	// Extract session cookie
	for _, c := range resp.Cookies() {
		if c.Name == "uvb76_session" {
			w.sessionCookie = c.String()
			break
		}
	}

	if w.sessionCookie == "" {
		return fmt.Errorf("no session cookie")
	}

	log.Printf("Authenticated successfully")
	return nil
}

// querySamples fetches latency samples from the API.
func (w *Workload) querySamples(saveFinal bool) int {
	url := fmt.Sprintf(
		"http://localhost:%s/api/v1/targets/%s/latency/samples?limit=%d",
		w.port, w.targetID, w.requestLimit,
	)

	body, statusCode, err := w.httpGet(url)
	if err != nil {
		return -1
	}

	if statusCode != http.StatusOK {
		return -1
	}

	// Parse as array of LatencySample
	var samples []state.LatencySample
	if err := json.Unmarshal(body, &samples); err != nil {
		return -1
	}

	count := len(samples)

	if saveFinal {
		samplesFile := filepath.Join(w.artifactDir, "final-latency-samples.json")
		if err := os.WriteFile(samplesFile, body, 0644); err != nil {
			log.Printf("Failed to write samples file: %v", err)
		}
	}

	return count
}

// querySummary fetches latency summary from the API.
func (w *Workload) querySummary() bool {
	url := fmt.Sprintf(
		"http://localhost:%s/api/v1/targets/%s/latency?probe_kind=icmp",
		w.port, w.targetID,
	)

	body, statusCode, err := w.httpGet(url)
	if err != nil {
		return false
	}

	if statusCode != http.StatusOK {
		return false
	}

	summaryFile := filepath.Join(w.artifactDir, "final-latency-summary.json")
	if err := os.WriteFile(summaryFile, body, 0644); err != nil {
		log.Printf("Failed to write summary file: %v", err)
		return false
	}

	return true
}

// querySeries fetches latency series from the API.
func (w *Workload) querySeries() bool {
	url := fmt.Sprintf(
		"http://localhost:%s/api/v1/latency/series?target_id=%s&probe_kind=icmp&range_seconds=3600",
		w.port, w.targetID,
	)

	body, statusCode, err := w.httpGet(url)
	if err != nil {
		return false
	}

	if statusCode != http.StatusOK {
		return false
	}

	seriesFile := filepath.Join(w.artifactDir, "final-latency-series.json")
	if err := os.WriteFile(seriesFile, body, 0644); err != nil {
		log.Printf("Failed to write series file: %v", err)
		return false
	}

	return true
}

// querySpikes fetches latency spikes from the API.
func (w *Workload) querySpikes() bool {
	url := fmt.Sprintf(
		"http://localhost:%s/api/v1/latency/spikes?target_id=%s&kind=icmp",
		w.port, w.targetID,
	)

	body, statusCode, err := w.httpGet(url)
	if err != nil {
		return false
	}

	if statusCode != http.StatusOK {
		return false
	}

	spikesFile := filepath.Join(w.artifactDir, "final-latency-spikes.json")
	if err := os.WriteFile(spikesFile, body, 0644); err != nil {
		log.Printf("Failed to write spikes file: %v", err)
		return false
	}

	return true
}

// httpGet performs an authenticated GET request.
func (w *Workload) httpGet(url string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	if w.sessionCookie != "" {
		req.Header.Set("Cookie", w.sessionCookie)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}

	return body, resp.StatusCode, nil
}

// writeArtifacts writes workload summary to disk.
func (w *Workload) writeArtifacts(result Result) {
	// Write workload summary
	summary := map[string]any{
		"requested_sample_limit": w.requestLimit,
		"requests_total":        result.RequestsTotal,
		"requests_failed":       result.RequestsFailed,
		"max_observed_samples":  result.MaxSampleCount,
		"sample_endpoint_valid": result.SampleValid,
		"summary_endpoint_valid": result.SummaryValid,
	}

	data, _ := json.MarshalIndent(summary, "", "  ")
	summaryFile := filepath.Join(w.artifactDir, "workload-summary.json")
	if err := os.WriteFile(summaryFile, data, 0644); err != nil {
		log.Printf("Failed to write workload summary: %v", err)
	}
}
