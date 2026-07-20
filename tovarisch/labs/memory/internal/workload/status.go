// workload/status.go — HTTP status workload for memory lab
//
// Provides HTTP workload generation for Tovarisch /status endpoint.
// Supports 1Hz polling and burst modes with bounded concurrency.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package workload

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// StatusWorkloadConfig configures the HTTP status workload.
type StatusWorkloadConfig struct {
	URL         string
	Operations  int
	IntervalMs  int
	Concurrency int
	TimeoutMs   int
	Client      *http.Client
	Name        string
}

// StatusWorkloadResult holds workload execution results.
type StatusWorkloadResult struct {
	Operations int
	Successful int
	Failed     int
	DurationMs int64
	FirstError error
}

// RunStatusWorkload executes HTTP status requests.
func RunStatusWorkload(ctx context.Context, cfg StatusWorkloadConfig) *StatusWorkloadResult {
	result := &StatusWorkloadResult{}
	if cfg.Operations <= 0 {
		return result
	}

	// Create HTTP client if not provided
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	if cfg.TimeoutMs > 0 {
		client.Timeout = time.Duration(cfg.TimeoutMs) * time.Millisecond
	}

	// Set concurrency
	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	// Set interval
	interval := time.Duration(cfg.IntervalMs) * time.Millisecond

	startTime := time.Now()
	var wg sync.WaitGroup
	var opCounter int64
	var successCounter int64
	var failCounter int64
	var firstErr error
	var firstErrMu sync.Mutex

	// Semaphore for bounded concurrency
	sem := make(chan struct{}, concurrency)

	for i := 0; i < cfg.Operations; i++ {
		select {
		case <-ctx.Done():
			break
		default:
		}

		wg.Add(1)
		go func(opNum int) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Check context
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Make request
			req, err := http.NewRequestWithContext(ctx, "GET", cfg.URL, nil)
			if err != nil {
				atomic.AddInt64(&failCounter, 1)
				setFirstError(&firstErr, &firstErrMu, err)
				return
			}

			resp, err := client.Do(req)
			if err != nil {
				atomic.AddInt64(&failCounter, 1)
				setFirstError(&firstErr, &firstErrMu, err)
				return
			}

			// Read and close body
			_, err = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()

			if err != nil {
				atomic.AddInt64(&failCounter, 1)
				setFirstError(&firstErr, &firstErrMu, err)
				return
			}

			// Check status code
			if resp.StatusCode != http.StatusOK {
				atomic.AddInt64(&failCounter, 1)
				setFirstError(&firstErr, &firstErrMu, fmt.Errorf("status %d", resp.StatusCode))
				return
			}

			atomic.AddInt64(&successCounter, 1)
		}(i)

		atomic.AddInt64(&opCounter, 1)

		// Wait for interval between requests
		if interval > 0 && i < cfg.Operations-1 {
			select {
			case <-ctx.Done():
				break
			case <-time.After(interval):
			}
		}
	}

	wg.Wait()

	result.Operations = int(atomic.LoadInt64(&opCounter))
	result.Successful = int(atomic.LoadInt64(&successCounter))
	result.Failed = int(atomic.LoadInt64(&failCounter))
	result.DurationMs = time.Since(startTime).Milliseconds()
	result.FirstError = firstErr

	return result
}

// setFirstError sets first error in a thread-safe manner.
func setFirstError(err *error, mu *sync.Mutex, newErr error) {
	mu.Lock()
	defer mu.Unlock()
	if *err == nil {
		*err = newErr
	}
}

// Runner orchestrates workload execution with phase awareness.
type Runner struct {
	config StatusWorkloadConfig
	result *StatusWorkloadResult
	ctx    context.Context
	cancel context.CancelFunc
}

// NewRunner creates a new workload runner.
func NewRunner(ctx context.Context, cfg StatusWorkloadConfig) *Runner {
	runCtx, cancel := context.WithCancel(ctx)
	return &Runner{
		config: cfg,
		ctx:    runCtx,
		cancel: cancel,
	}
}

// Start begins workload execution.
func (r *Runner) Start() {
	go func() {
		r.result = RunStatusWorkload(r.ctx, r.config)
	}()
}

// Stop halts workload execution.
func (r *Runner) Stop() {
	r.cancel()
}

// Result returns the workload result.
func (r *Runner) Result() *StatusWorkloadResult {
	return r.result
}

// StatusOneHz returns config for 1Hz status polling.
func StatusOneHz(url string) StatusWorkloadConfig {
	return StatusWorkloadConfig{
		URL:         url,
		Operations:  0, // Unlimited
		IntervalMs:  1000,
		Concurrency: 1,
		TimeoutMs:   5000,
		Name:        "status-1hz",
	}
}

// StatusBurst returns config for burst status requests.
func StatusBurst(url string, count int) StatusWorkloadConfig {
	return StatusWorkloadConfig{
		URL:         url,
		Operations:  count,
		IntervalMs:  0, // As fast as possible
		Concurrency: 10,
		TimeoutMs:   5000,
		Name:        "status-burst",
	}
}

// Passive returns config for passive (no HTTP) workload.
func Passive() StatusWorkloadConfig {
	return StatusWorkloadConfig{
		Operations: 0,
		Name:       "passive",
	}
}
