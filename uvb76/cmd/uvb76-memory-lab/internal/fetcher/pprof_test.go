package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFetcherFetchSuccess(t *testing.T) {
	// Create a test server that returns fake profile data
	body := "fake pprof data"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}))
	defer server.Close()

	// Create temp dir for test
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test.pb.gz")

	fetcher := NewFetcher(Config{
		BaseURL: server.URL,
		Timeout: 10 * time.Second,
	})

	ctx := context.Background()
	err := fetcher.Fetch(ctx, "/debug/pprof/heap", destPath)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	// Verify file was created and content matches
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != body {
		t.Errorf("Expected content %q, got %q", body, string(data))
	}
}

func TestFetcherNon2xxResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test.pb.gz")

	fetcher := NewFetcher(Config{
		BaseURL: server.URL,
		Timeout: 10 * time.Second,
	})

	ctx := context.Background()
	err := fetcher.Fetch(ctx, "/debug/pprof/heap", destPath)
	if err == nil {
		t.Fatal("Expected error for non-2xx response")
	}
}

func TestFetcherTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test.pb.gz")

	fetcher := NewFetcher(Config{
		BaseURL: server.URL,
		Timeout: 100 * time.Millisecond,
	})

	ctx := context.Background()
	err := fetcher.Fetch(ctx, "/debug/pprof/heap", destPath)
	if err == nil {
		t.Fatal("Expected error for timeout")
	}
}

func TestFetcherContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test.pb.gz")

	fetcher := NewFetcher(Config{
		BaseURL: server.URL,
		Timeout: 10 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := fetcher.Fetch(ctx, "/debug/pprof/heap", destPath)
	if err == nil {
		t.Fatal("Expected error for cancelled context")
	}
}

func TestNewFetcher(t *testing.T) {
	cfg := Config{
		BaseURL: "http://localhost:6060",
		Timeout: 30 * time.Second,
	}

	f := NewFetcher(cfg)
	if f == nil {
		t.Fatal("NewFetcher returned nil")
	}
	if f.baseURL != cfg.BaseURL {
		t.Errorf("Expected baseURL %q, got %q", cfg.BaseURL, f.baseURL)
	}
	if f.userAgent != cfg.UserAgent && f.userAgent != "uvb76-memory-lab/1.0" {
		t.Errorf("Expected userAgent %q or fallback, got %q", cfg.UserAgent, f.userAgent)
	}
}

func TestNewFetcherEmptyUserAgentFallsBack(t *testing.T) {
	cfg := Config{
		BaseURL:   "http://localhost:6060",
		Timeout:   30 * time.Second,
		UserAgent: "",
	}

	f := NewFetcher(cfg)
	if f.userAgent != "uvb76-memory-lab/1.0" {
		t.Errorf("Expected fallback userAgent, got %q", f.userAgent)
	}
}

func TestFetcherCustomUserAgentSent(t *testing.T) {
	var receivedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test.pb.gz")

	fetcher := NewFetcher(Config{
		BaseURL:   server.URL,
		Timeout:   10 * time.Second,
		UserAgent: "custom-agent/1.0",
	})

	ctx := context.Background()
	err := fetcher.Fetch(ctx, "/debug/pprof/heap", destPath)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if receivedUA != "custom-agent/1.0" {
		t.Errorf("Expected User-Agent 'custom-agent/1.0', got %q", receivedUA)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("http://localhost:6060")
	if cfg.BaseURL != "http://localhost:6060" {
		t.Errorf("Expected BaseURL http://localhost:6060, got %s", cfg.BaseURL)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Expected Timeout 30s, got %v", cfg.Timeout)
	}
	if cfg.UserAgent != "uvb76-memory-lab/1.0" {
		t.Errorf("Expected UserAgent uvb76-memory-lab/1.0, got %s", cfg.UserAgent)
	}
}
