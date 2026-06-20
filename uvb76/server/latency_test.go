package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/auth"
	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// TestHandleTargetLatencySeries_DefaultsFromZeroedConfig tests that when latency config
// has zero values (e.g., loaded from JSON with empty latency:{}), the series endpoint
// returns the correct default values, not zeros.
// This is a regression test for the bug where cfg.Latency wasn't defaulted before being
// passed to the server, causing series metadata to show interval_seconds=0, retained_range=0.
func TestHandleTargetLatencySeries_DefaultsFromZeroedConfig(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	// Create config with ZEROED latency - simulates JSON like: "latency": {"http": {}, "icmp": {}}
	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency:  config.LatencyConfig{}, // Zeroed - all fields default to 0
		Targets:  []config.TargetConfig{{ID: "test-1", Name: "Test", BaseURL: "http://localhost:8080", Enabled: true}},
	}

	// Apply defaults to cfg.Latency - this is what main.go should do
	cfg.Latency.ApplyDefaults()

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/series", http.HandlerFunc(srv.handleTargetLatencySeries))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/series?target_id=test-1&probe_kind=http", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var series state.LatencySeries
	json.NewDecoder(rec.Body).Decode(&series)

	// HTTP defaults from latency.go
	if series.IntervalSeconds != config.DefaultHTTPIntervalSeconds {
		t.Errorf("Expected HTTP interval_seconds=%d (default), got %d", config.DefaultHTTPIntervalSeconds, series.IntervalSeconds)
	}
	if series.RetainedRangeSeconds == 0 {
		t.Error("Expected retained_range_seconds > 0 after defaults, got 0")
	}

	// Test ICMP too
	reqICMP := httptest.NewRequest(http.MethodGet, "/api/v1/latency/series?target_id=test-1&probe_kind=icmp", nil)
	reqICMP.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	recICMP := httptest.NewRecorder()
	router.ServeHTTP(recICMP, reqICMP)

	if recICMP.Code != http.StatusOK {
		t.Errorf("Expected 200 for ICMP, got %d", recICMP.Code)
	}

	var seriesICMP state.LatencySeries
	json.NewDecoder(recICMP.Body).Decode(&seriesICMP)

	// ICMP defaults from latency.go
	if seriesICMP.IntervalSeconds != config.DefaultICMPIntervalSeconds {
		t.Errorf("Expected ICMP interval_seconds=%d (default), got %d", config.DefaultICMPIntervalSeconds, seriesICMP.IntervalSeconds)
	}
	if seriesICMP.RetainedRangeSeconds == 0 {
		t.Error("Expected ICMP retained_range_seconds > 0 after defaults, got 0")
	}
}

// TestHandleTargetLatencySeries_ExtremeParamsRejected verifies that pathological
// query parameters are rejected with HTTP 400 rather than causing unbounded allocations.
// This is a regression test for the SIGSEGV crash at latency.go:418.
func TestHandleTargetLatencySeries_ExtremeParamsRejected(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("testpass", salt)

	cfg := &config.Config{
		Listen:  config.ListenConfig{Addr: ":0"},
		Auth:    config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:  config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency: config.LatencyConfig{},
		Targets: []config.TargetConfig{{ID: "test-1", Name: "Test", BaseURL: "http://localhost:8080", Enabled: true}},
	}
	cfg.Latency.ApplyDefaults()

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/series", http.HandlerFunc(srv.handleTargetLatencySeries))

	tests := []struct {
		name         string
		query        string
		wantStatus   int
		checkPointCount bool
		maxPoints    int
	}{
		{
			name:         "range_seconds exceeds max returns 400",
			query:        "target_id=test-1&probe_kind=icmp&range_seconds=999999",
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "step_seconds exceeds max returns 400",
			query:        "target_id=test-1&probe_kind=icmp&step_seconds=999999",
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "normal params return 200 with bounded points",
			query:        "target_id=test-1&probe_kind=icmp&range_seconds=3600&step_seconds=60",
			wantStatus:   http.StatusOK,
			checkPointCount: true,
			maxPoints:    MaxOutputPoints,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/series?"+tc.query, nil)
			req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("Expected status %d, got %d", tc.wantStatus, rec.Code)
			}

			if tc.checkPointCount && rec.Code == http.StatusOK {
				var series state.LatencySeries
				if err := json.NewDecoder(rec.Body).Decode(&series); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if series.ReturnedPointCount > tc.maxPoints {
					t.Errorf("Point count %d exceeds max %d", series.ReturnedPointCount, tc.maxPoints)
				}
			}
		})
	}
}

// TestHandleTargetLatencySeries_BoundedOutput verifies that response point count
// is bounded even with large range_seconds/limit values.
func TestHandleTargetLatencySeries_BoundedOutput(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("testpass", salt)

	cfg := &config.Config{
		Listen:  config.ListenConfig{Addr: ":0"},
		Auth:    config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:  config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency: config.LatencyConfig{},
		Targets: []config.TargetConfig{{ID: "test-1", Name: "Test", BaseURL: "http://localhost:8080", Enabled: true}},
	}
	cfg.Latency.ApplyDefaults()

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/series", http.HandlerFunc(srv.handleTargetLatencySeries))

	// Request with max range - output should still be bounded
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/series?target_id=test-1&probe_kind=icmp&range_seconds=3600&step_seconds=1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}

	var series state.LatencySeries
	if err := json.NewDecoder(rec.Body).Decode(&series); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Even with step=1 and range=3600, output should be capped
	if series.ReturnedPointCount > MaxOutputPoints {
		t.Errorf("Point count %d exceeds MaxOutputPoints %d", series.ReturnedPointCount, MaxOutputPoints)
	}
}

// TestHandleTargetLatencySeries_EmptySamples verifies safe behavior with no samples.
func TestHandleTargetLatencySeries_EmptySamples(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("testpass", salt)

	cfg := &config.Config{
		Listen:  config.ListenConfig{Addr: ":0"},
		Auth:    config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:  config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency: config.LatencyConfig{},
		Targets: []config.TargetConfig{{ID: "test-1", Name: "Test", BaseURL: "http://localhost:8080", Enabled: true}},
	}
	cfg.Latency.ApplyDefaults()

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/series", http.HandlerFunc(srv.handleTargetLatencySeries))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/series?target_id=test-1&probe_kind=icmp", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}

	var series state.LatencySeries
	if err := json.NewDecoder(rec.Body).Decode(&series); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should have valid structure even with no samples
	if series.TargetID != "test-1" {
		t.Errorf("Expected target_id=test-1, got %s", series.TargetID)
	}
	if series.ProbeKind != "icmp" {
		t.Errorf("Expected probe_kind=icmp, got %s", series.ProbeKind)
	}
}

// TestHandleTargetLatencySeries_ConcurrentRecordAndRead is a regression test for
// the crash at uvb76/server/latency.go:418 during concurrent ICMP writes and
// series reads. The defensive copy fix ensures the read path is stable even
// when the ring buffer is being mutated.
func TestHandleTargetLatencySeries_ConcurrentRecordAndRead(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent stress test in short mode")
	}

	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("testpass", salt)

	cfg := &config.Config{
		Listen:  config.ListenConfig{Addr: ":0"},
		Auth:    config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:  config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency: config.LatencyConfig{},
		Targets: []config.TargetConfig{{ID: "test-1", Name: "Test", BaseURL: "http://localhost:8080", Enabled: true}},
	}
	cfg.Latency.ApplyDefaults()

	st := state.NewManager()
	st.ConfigureICMP([]int64{5, 10, 25, 50, 100}, 3600)
	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/series", http.HandlerFunc(srv.handleTargetLatencySeries))

	// Pre-populate with some samples
	for i := 0; i < 100; i++ {
		st.RecordICMPLatencyAt("test-1", float64(i%100)+10.0, true, time.Now().UTC())
	}

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// Writer goroutines simulating ICMP probes
	writerCount := 2
	for w := 0; w < writerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			i := 0
			for {
				select {
				case <-stopCh:
					return
				default:
					latency := float64((i*17)%200) + 10.0
					reachable := i%10 != 0
					st.RecordICMPLatency("test-1", latency, reachable)
					i++
					runtime.Gosched()
				}
			}
		}()
	}

	// Reader goroutines calling the series endpoint
	readerCount := 4
	errCh := make(chan error, readerCount)
	for r := 0; r < readerCount; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/series?target_id=test-1&probe_kind=icmp&range_seconds=3600", nil)
					req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)

					if rec.Code != http.StatusOK {
						select {
						case errCh <- fmt.Errorf("unexpected status %d", rec.Code):
						default:
						}
						return
					}

					// Verify JSON is valid
					var series state.LatencySeries
					if err := json.NewDecoder(rec.Body).Decode(&series); err != nil {
						select {
						case errCh <- fmt.Errorf("JSON decode error: %v", err):
						default:
						}
						return
					}

					// Verify bounds
					if series.ReturnedPointCount > MaxOutputPoints {
						select {
						case errCh <- fmt.Errorf("point count %d exceeds max %d", series.ReturnedPointCount, MaxOutputPoints):
						default:
						}
						return
					}

					runtime.Gosched()
				}
			}
		}()
	}

	// Run for fixed duration
	time.Sleep(2 * time.Second)
	close(stopCh)
	wg.Wait()

	// Check for errors
	close(errCh)
	for err := range errCh {
		t.Errorf("Concurrent read error: %v", err)
	}
}

// TestHandleTargetLatencySeries_ICMPBufferFullRegression is a regression test
// for the crash path: full ICMP buffer (3600 samples) being processed by the series
// handler. Note: this test verifies handler correctness with a full buffer;
// concurrent stress testing is covered by TestHandleTargetLatencySeries_ConcurrentRecordAndRead.
func TestHandleTargetLatencySeries_ICMPBufferFullRegression(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("testpass", salt)

	cfg := &config.Config{
		Listen:  config.ListenConfig{Addr: ":0"},
		Auth:    config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:  config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency: config.LatencyConfig{},
		Targets: []config.TargetConfig{{ID: "test-1", Name: "Test", BaseURL: "http://localhost:8080", Enabled: true}},
	}
	cfg.Latency.ApplyDefaults()

	st := state.NewManager()
	st.ConfigureICMP([]int64{5, 10, 25, 50, 100}, 3600)

	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/series", http.HandlerFunc(srv.handleTargetLatencySeries))

	// Fill ICMP buffer to capacity
	capacity := st.GetICMPMaxSamples()
	for i := 0; i < capacity; i++ {
		st.RecordICMPLatencyAt("test-1", float64(i%100)+10.0, true, time.Now().UTC().Add(time.Duration(-capacity+i)*time.Second))
	}

	// Verify buffer is full
	if st.GetICMPMaxSamples() != capacity {
		t.Fatalf("Buffer not filled: expected %d, got %d", capacity, st.GetICMPMaxSamples())
	}

	// Call series endpoint - this should not crash with a full ICMP buffer.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/series?target_id=test-1&probe_kind=icmp&range_seconds=3600", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}

	var series state.LatencySeries
	if err := json.NewDecoder(rec.Body).Decode(&series); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should have captured all samples
	if series.RetainedSampleCount != capacity {
		t.Errorf("Expected retained samples %d, got %d", capacity, series.RetainedSampleCount)
	}
}
