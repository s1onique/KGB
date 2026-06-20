// Package workload runs authenticated HTTPS requests against UVB-76 /api/v1/targets.
package workload

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ExpectedDiagnosticTarget holds the expected diagnostic fields.
type ExpectedDiagnosticTarget struct {
	ID                  string
	DiagnosticPeerName  string
	DiagnosticBaseURL   string
	EffectiveCaptureURL string
}

// TargetResponse represents a single target from /api/v1/targets.
type TargetResponse struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	BaseURL             string `json:"base_url"`
	ProbeURL            string `json:"probe_url,omitempty"`
	EffectiveProbeURL   string `json:"effective_probe_url"`
	Enabled             bool   `json:"enabled"`
	DiagnosticPeerName  string `json:"diagnostic_peer_name,omitempty"`
	DiagnosticBaseURL   string `json:"diagnostic_base_url,omitempty"`
	EffectiveCaptureURL string `json:"effective_capture_url,omitempty"`
}

// Result captures workload execution metrics.
type Result struct {
	SuccessCount  int           `json:"success_count"`
	ErrorCount    int           `json:"error_count"`
	RequestCount  int           `json:"request_count"`
	SampleValid   bool          `json:"sample_valid"`
	DiagFieldsOK  bool          `json:"diag_fields_present"`
	CaptureURLOK  bool          `json:"effective_capture_url_present"`
	WorkerStats   []WorkerStat  `json:"worker_stats"`
}

// WorkerStat tracks per-worker statistics.
type WorkerStat struct {
	WorkerID     int `json:"worker_id"`
	SuccessCount int `json:"success_count"`
	ErrorCount   int `json:"error_count"`
}

// Workload executes authenticated HTTPS requests to /api/v1/targets.
type Workload struct {
	port          string
	user          string
	pass          string
	certFile      string
	artifactDir   string
	workers       int
	requestCount  int32
	successCount  int32
	errorCount    int32
	client        *http.Client
	sessionCookie string
	workerStats   []WorkerStat
	workerMu      sync.Mutex
	responseJSON  []TargetResponse
	diagFieldsOK  atomic.Bool
	captureURLOK  atomic.Bool
}

// New creates a new workload runner for HTTPS requests.
func New(port, user, pass, certFile, artifactDir string, workers int, diag ExpectedDiagnosticTarget) *Workload {
	// For lab: use InsecureSkipVerify since we're using self-signed certs
	return &Workload{
		port:        port,
		user:        user,
		pass:        pass,
		certFile:    certFile,
		artifactDir: artifactDir,
		workers:     workers,
		workerStats: make([]WorkerStat, workers),
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					ServerName:         "localhost",
					InsecureSkipVerify: true, // Lab uses self-signed certs
				},
			},
		},
	}
}

// Run executes the workload for the specified duration.
func (w *Workload) Run(durationSeconds int) Result {
	result := Result{WorkerStats: make([]WorkerStat, w.workers)}
	for i := 0; i < w.workers; i++ {
		w.workerStats[i] = WorkerStat{WorkerID: i}
	}

	if err := w.authenticate(); err != nil {
		log.Printf("Authentication failed: %v", err)
		atomic.AddInt32(&w.errorCount, 1)
		return result
	}

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	log.Printf("Starting %d workers for %d seconds...", w.workers, durationSeconds)
	for i := 0; i < w.workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			w.runWorker(workerID, stopCh)
		}(i)
	}

	time.Sleep(time.Duration(durationSeconds) * time.Second)
	close(stopCh)
	wg.Wait()

	result.SuccessCount = int(atomic.LoadInt32(&w.successCount))
	result.ErrorCount = int(atomic.LoadInt32(&w.errorCount))
	result.RequestCount = int(atomic.LoadInt32(&w.requestCount))

	w.workerMu.Lock()
	for i := 0; i < w.workers; i++ {
		result.WorkerStats[i] = w.workerStats[i]
	}
	result.DiagFieldsOK = w.diagFieldsOK.Load()
	result.CaptureURLOK = w.captureURLOK.Load()
	w.workerMu.Unlock()

	w.writeArtifacts(result)
	return result
}

// runWorker executes requests in a loop until stopped.
func (w *Workload) runWorker(workerID int, stopCh <-chan struct{}) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			ok := w.queryTargets()
			w.recordResult(workerID, ok)
		}
	}
}

// recordResult records a single request result.
func (w *Workload) recordResult(workerID int, success bool) {
	atomic.AddInt32(&w.requestCount, 1)
	if success {
		atomic.AddInt32(&w.successCount, 1)
	} else {
		atomic.AddInt32(&w.errorCount, 1)
	}
	w.workerMu.Lock()
	if workerID < len(w.workerStats) {
		if success {
			w.workerStats[workerID].SuccessCount++
		} else {
			w.workerStats[workerID].ErrorCount++
		}
	}
	w.workerMu.Unlock()
}

// authenticate performs HTTPS session authentication.
func (w *Workload) authenticate() error {
	url := fmt.Sprintf("https://localhost:%s/api/v1/auth/login", w.port)
	body := fmt.Sprintf(`{"username":"%s","password":"%s"}`, w.user, w.pass)

	req, err := http.NewRequest("POST", url, strings.NewReader(body))
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

// queryTargets performs an authenticated GET to /api/v1/targets.
// Returns true only if the response is valid and diagnostic fields are present.
func (w *Workload) queryTargets() bool {
	url := fmt.Sprintf("https://localhost:%s/api/v1/targets", w.port)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Worker error: create request: %v", err)
		return false
	}
	if w.sessionCookie != "" {
		req.Header.Set("Cookie", w.sessionCookie)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		log.Printf("Worker error: request failed: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Worker error: status %d", resp.StatusCode)
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Worker error: read body: %v", err)
		return false
	}

	var targets []TargetResponse
	if err := json.Unmarshal(body, &targets); err != nil {
		log.Printf("Worker error: decode JSON: %v", err)
		return false
	}

	if len(targets) < 2 {
		log.Printf("Worker error: expected 2+ targets, got %d", len(targets))
		return false
	}

	targetIDs := make(map[string]bool)
	for _, t := range targets {
		targetIDs[t.ID] = true
	}
	if !targetIDs["target-with-diag"] || !targetIDs["target-plain"] {
		log.Printf("Worker error: missing expected target IDs")
		return false
	}

	var diagTarget *TargetResponse
	for i := range targets {
		if targets[i].ID == "target-with-diag" {
			diagTarget = &targets[i]
			break
		}
	}
	if diagTarget == nil {
		log.Printf("Worker error: diagnostic target not found")
		return false
	}

	// Validate diagnostic fields from actual response
	diagFieldsPresent := diagTarget.DiagnosticPeerName != "" && diagTarget.DiagnosticBaseURL != ""
	captureURLPresent := diagTarget.EffectiveCaptureURL != ""

	if diagFieldsPresent {
		w.diagFieldsOK.Store(true)
	}
	if captureURLPresent {
		w.captureURLOK.Store(true)
	}

	if !diagFieldsPresent {
		log.Printf("Worker error: DiagnosticPeerName or DiagnosticBaseURL is empty")
		return false
	}
	if !captureURLPresent {
		log.Printf("Worker error: EffectiveCaptureURL is empty")
		return false
	}

	w.workerMu.Lock()
	if w.responseJSON == nil {
		w.responseJSON = targets
	}
	w.workerMu.Unlock()

	return true
}

// writeArtifacts writes workload summary to disk.
func (w *Workload) writeArtifacts(result Result) {
	summary := map[string]any{
		"workers":              w.workers,
		"success_count":        result.SuccessCount,
		"error_count":          result.ErrorCount,
		"request_count":        result.RequestCount,
		"diag_fields_present":   result.DiagFieldsOK,
		"capture_url_present":   result.CaptureURLOK,
	}
	if data, err := json.MarshalIndent(summary, "", "  "); err == nil {
		os.WriteFile(filepath.Join(w.artifactDir, "workload-summary.json"), data, 0644)
	}

	w.workerMu.Lock()
	if len(w.responseJSON) > 0 {
		if sampleData, err := json.MarshalIndent(w.responseJSON, "", "  "); err == nil {
			os.WriteFile(filepath.Join(w.artifactDir, "targets-response-sample.json"), sampleData, 0644)
		}
	}
	w.workerMu.Unlock()

	if workerData, err := json.MarshalIndent(result.WorkerStats, "", "  "); err == nil {
		os.WriteFile(filepath.Join(w.artifactDir, "worker-stats.json"), workerData, 0644)
	}
}
