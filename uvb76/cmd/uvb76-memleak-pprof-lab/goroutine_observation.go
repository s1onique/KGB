// Package main provides the UVB-76 pprof memory leak lab.
//
// # Typed Goroutine Observation
//
// This file implements P0-14: Make goroutine observation typed and mandatory.
// The FetchGoroutineCount function returns typed success or typed failure.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	// MaxGoroutineBodySize is the maximum size for goroutine dump responses.
	MaxGoroutineBodySize = 10 * 1024 * 1024 // 10MB
)

// FetchGoroutineCount fetches goroutine count from pprof endpoint with typed return.
// P0-14: Returns (count, error) - never returns (0, nil) for missing authority.
func FetchGoroutineCount(ctx context.Context, client *http.Client, pprofBaseURL string) (int64, error) {
	if client == nil {
		return 0, fmt.Errorf("%w: nil HTTP client", ErrGoroutineTransport)
	}

	if pprofBaseURL == "" {
		return 0, fmt.Errorf("%w: empty pprof base URL", ErrGoroutineTransport)
	}

	// Build pprof URL
	pprofURL := strings.TrimSuffix(pprofBaseURL, "/") + "/debug/pprof/goroutine?debug=1"

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pprofURL, nil)
	if err != nil {
		return 0, fmt.Errorf("%w: create request: %v", ErrGoroutineTransport, err)
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrGoroutineTransport, err)
	}
	defer resp.Body.Close()

	// Check status
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%w: status %d", ErrGoroutineUnexpectedStatus, resp.StatusCode)
	}

	// Read with bounded limit
	body, err := io.ReadAll(http.MaxBytesReader(nil, resp.Body, MaxGoroutineBodySize))
	if err != nil {
		return 0, fmt.Errorf("%w: read body: %v", ErrGoroutineRead, err)
	}

	// Check if body is empty
	if len(body) == 0 {
		return 0, fmt.Errorf("%w: empty goroutine dump", ErrGoroutineObservationEmpty)
	}

	// Parse goroutine count
	count, parseErr := parseGoroutineDump(string(body))
	if parseErr != nil {
		return 0, fmt.Errorf("%w: %v", ErrGoroutineParse, parseErr)
	}

	// P0-14: Zero goroutines is a valid observation - process may have exited
	// or be in a quiescent state. Only empty body is an error.
	return count, nil
}

// parseGoroutineDump parses a goroutine dump and returns the count.
func parseGoroutineDump(dump string) (int64, error) {
	lines := strings.Split(dump, "\n")
	var count int64
	for _, line := range lines {
		if strings.HasPrefix(line, "goroutine ") && strings.HasSuffix(line, ":") {
			count++
		}
	}
	return count, nil
}

// GoroutineObservationError represents a goroutine observation failure.
type GoroutineObservationError struct {
	PID      int
	InnerErr error
}

func (e *GoroutineObservationError) Error() string {
	return fmt.Sprintf("goroutine observation error (PID %d): %v", e.PID, e.InnerErr)
}

func (e *GoroutineObservationError) Unwrap() error {
	return e.InnerErr
}

// ObserveGoroutineWithPID observes goroutines and associates with a PID for error reporting.
func ObserveGoroutineWithPID(ctx context.Context, client *http.Client, pprofBaseURL string, pid int) (int64, error) {
	count, err := FetchGoroutineCount(ctx, client, pprofBaseURL)
	if err != nil {
		return 0, &GoroutineObservationError{PID: pid, InnerErr: err}
	}
	return count, nil
}
