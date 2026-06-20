// Package server provides concurrency tests for the /targets endpoint.
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/auth"
	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// TestTargetsEndpoint_ConcurrentRequests tests that handleTargets is safe under
// concurrent access from multiple goroutines while scraper/probe state updates run.
// This is a regression test for crashes under HTTPS/HTTP2 handler churn.
func TestTargetsEndpoint_ConcurrentRequests(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Diagnostics: config.DiagnosticsConfig{
			Enabled:        true,
			CaptureOnSpike: true,
			Peers: []config.DiagPeerConfig{
				{
					Name:    "peer-1",
					BaseURL: "http://10.88.76.2:8317",
					Targets: []string{"target-1", "target-2"},
				},
			},
		},
		Targets: []config.TargetConfig{
			{ID: "target-1", Name: "Target 1", BaseURL: "http://10.88.76.2:8317", Enabled: true},
			{ID: "target-2", Name: "Target 2", BaseURL: "http://10.88.76.3:8317", Enabled: true},
		},
	}

	// Validate to trigger PrecomputeCaptureURLs
	if err := cfg.Validate(config.ValidationOptions{AllowMissingTLS: true}); err != nil {
		t.Fatalf("Config validation failed: %v", err)
	}

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/targets", http.HandlerFunc(srv.handleTargets))

	var wg sync.WaitGroup
	stopCh := make(chan struct{})
	requestCount := 0
	errorCount := 0
	var mu sync.Mutex

	// Concurrent requesters
	numWorkers := 16
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					req := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
					req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)

					mu.Lock()
					requestCount++
					if rec.Code != http.StatusOK {
						errorCount++
					} else {
						var targets []TargetInfo
						if err := json.NewDecoder(rec.Body).Decode(&targets); err != nil {
							errorCount++
						} else if len(targets) != 2 {
							errorCount++
						}
					}
					mu.Unlock()
				}
			}
		}()
	}

	// Simulate scraper/probe state updates
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopCh:
				return
			default:
				st.UpdateSnapshot("target-1", &state.TargetSnapshot{
					TargetID:  "target-1",
					ScrapedAt: time.Now(),
					Reachable: true,
					Status:    "ok",
				})
				st.RecordLatency("target-1", 10.0+float64(time.Now().UnixNano()%100), true)
				st.RecordLatency("target-2", 20.0+float64(time.Now().UnixNano()%100), true)
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	// Run for 2 seconds
	time.Sleep(2 * time.Second)
	close(stopCh)
	wg.Wait()

	t.Logf("Concurrent test: %d requests, %d errors", requestCount, errorCount)
	if errorCount > 0 {
		t.Errorf("Expected no errors, got %d errors out of %d requests", errorCount, requestCount)
	}
}
