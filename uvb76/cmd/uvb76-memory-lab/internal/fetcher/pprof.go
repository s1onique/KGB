// Package fetcher provides HTTP-based pprof profile fetching with atomic writes.
package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Config holds the fetcher configuration.
type Config struct {
	// BaseURL is the pprof server base URL (e.g., http://127.0.0.1:6060)
	BaseURL string
	// Timeout is the HTTP request timeout
	Timeout time.Duration
	// UserAgent identifies this client
	UserAgent string
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig(baseURL string) Config {
	return Config{
		BaseURL:   baseURL,
		Timeout:   30 * time.Second,
		UserAgent: "uvb76-memory-lab/1.0",
	}
}

// Fetcher fetches pprof profiles via HTTP.
type Fetcher struct {
	client    *http.Client
	baseURL   string
	userAgent string
}

// NewFetcher creates a new pprof fetcher.
func NewFetcher(cfg Config) *Fetcher {
	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = "uvb76-memory-lab/1.0"
	}
	return &Fetcher{
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		baseURL:   cfg.BaseURL,
		userAgent: userAgent,
	}
}

// Fetch fetches a pprof profile and writes it atomically to destPath.
// It first writes to a temp file, then renames to destPath on success.
func (f *Fetcher) Fetch(ctx context.Context, endpoint string, destPath string) error {
	url := f.baseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", f.userAgent)

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("non-2xx response from %s: status=%d", url, resp.StatusCode)
	}

	// Create temp file in same directory for atomic rename
	dir := filepath.Dir(destPath)
	tmp, err := os.CreateTemp(dir, filepath.Base(destPath)+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Write to temp file
	_, err = io.Copy(tmp, resp.Body)
	if err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}

	// Sync and close
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// FetchHeap fetches a heap profile with optional GC trigger.
func (f *Fetcher) FetchHeap(ctx context.Context, destPath string, gc bool) error {
	endpoint := "/debug/pprof/heap"
	if gc {
		endpoint += "?gc=1"
	}
	return f.Fetch(ctx, endpoint, destPath)
}

// FetchAllocs fetches an allocs profile.
func (f *Fetcher) FetchAllocs(ctx context.Context, destPath string) error {
	return f.Fetch(ctx, "/debug/pprof/allocs", destPath)
}

// FetchGoroutine fetches a goroutine dump.
func (f *Fetcher) FetchGoroutine(ctx context.Context, destPath string, debug int) error {
	endpoint := fmt.Sprintf("/debug/pprof/goroutine?debug=%d", debug)
	return f.Fetch(ctx, endpoint, destPath)
}
