package server

import (
	"context"
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

// TestServer_ConcurrentLatencyAPIEndpoints_Race is a server-level integration test
// that simulates the exact crash scenario: concurrent HTTP/2 handlers hitting
// latency endpoints (both summary and series) while probes are recording samples.
//
// This test verifies that the server correctly handles concurrent latency API requests
// without crashing or returning corrupted data.
//
// The crash stack from production:
//   handleTargetLatencySeries -> GetRecentICMPLatencySamples
//     -> LatencyTracker.GetRecentSamples -> makeslice -> mallocgc -> memclrNoHeapPointers -> SIGSEGV
//
// Concurrent handlers seen in crash dump:
//   goroutine 13984: handleTargetLatencySeries -> GetRecentSamples
//   goroutine 13983: handleTargetLatencySeries running concurrently
//   goroutine 13935: handleTargetLatency -> GetICMPLatencySummary -> LatencyTracker.GetSummary
func TestServer_ConcurrentLatencyAPIEndpoints_Race(t *testing.T) {
	salt := []byte("test-salt-for-race-test")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency: config.LatencyConfig{
			HTTP: config.HTTPProbeConfig{
				Enabled:             boolPtr(true),
				IntervalSeconds:     30,
				WindowSeconds:       300,
				RetainedRangeSeconds: 3600,
			},
			ICMP: config.ICMPProbeConfig{
				Enabled:             boolPtr(true),
				IntervalSeconds:     1,
				WindowSeconds:       300,
				RetainedRangeSeconds: 3600,
			},
		},
		Targets: []config.TargetConfig{
			{ID: "router-1", Name: "ASUS RT-AX88U", BaseURL: "http://192.168.1.1", Enabled: true},
		},
	}
	st := state.NewManager()
	st.ConfigureICMP([]int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}, 3600)
	srv := NewServer(cfg, st, nil, true)

	// Pre-fill ICMP tracker with data
	targetID := "router-1"
	for i := 0; i < 3600; i++ {
		st.RecordICMPLatencyAt(targetID, float64(i%100)+10.0, true, time.Now().UTC())
	}
	for i := 0; i < 100; i++ {
		st.RecordLatency(targetID, float64(i%100)+10.0, true)
	}

	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/targets/{id}/latency", http.HandlerFunc(srv.handleTargetLatency)).Methods(http.MethodGet)
	protected.Handle("/targets/{id}/latency/samples", http.HandlerFunc(srv.handleTargetLatencySamples)).Methods(http.MethodGet)
	protected.Handle("/latency/series", http.HandlerFunc(srv.handleTargetLatencySeries)).Methods(http.MethodGet)
	protected.Handle("/latency", http.HandlerFunc(srv.handleAllLatency)).Methods(http.MethodGet)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Writer goroutine: simulate continuous ICMP probes
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Millisecond * 2)
		defer ticker.Stop()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				latency := float64((i*17)%200) + 10.0
				reachable := i%10 != 0
				st.RecordICMPLatency(targetID, latency, reachable)
				st.RecordLatency(targetID, latency, reachable)
			}
		}
	}()

	// Reader goroutines: simulate concurrent UI/API requests (HTTP/2 multiplexed)

	// /targets/{id}/latency (summary endpoint)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Millisecond * 2)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					req := httptest.NewRequest(http.MethodGet, "/api/v1/targets/"+targetID+"/latency", nil)
					req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Errorf("Summary endpoint returned %d instead of 200", rec.Code)
					}
					// Verify JSON is valid
					var resp map[string]interface{}
					if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
						t.Errorf("Summary endpoint returned invalid JSON: %v", err)
					}
				}
			}
		}()
	}

	// /targets/{id}/latency/samples
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Millisecond * 2)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					req := httptest.NewRequest(http.MethodGet, "/api/v1/targets/"+targetID+"/latency/samples?limit=100", nil)
					req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Errorf("Samples endpoint returned %d instead of 200", rec.Code)
					}
					// Verify JSON is valid
					var resp []interface{}
					if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
						t.Errorf("Samples endpoint returned invalid JSON: %v", err)
					}
				}
			}
		}()
	}

	// /latency/series (ICMP)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Millisecond * 2)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/series?target_id="+targetID+"&probe_kind=icmp&range_seconds=3600&step_seconds=60", nil)
					req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Errorf("Series endpoint (ICMP) returned %d instead of 200", rec.Code)
					}
					// Verify JSON is valid
					var resp map[string]interface{}
					if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
						t.Errorf("Series endpoint (ICMP) returned invalid JSON: %v", err)
					}
				}
			}
		}()
	}

	// /latency/series (HTTP)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Millisecond * 2)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/series?target_id="+targetID+"&probe_kind=http&range_seconds=3600&step_seconds=60", nil)
					req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Errorf("Series endpoint (HTTP) returned %d instead of 200", rec.Code)
					}
					// Verify JSON is valid
					var resp map[string]interface{}
					if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
						t.Errorf("Series endpoint (HTTP) returned invalid JSON: %v", err)
					}
				}
			}
		}()
	}

	// /latency (all targets summary)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Millisecond * 2)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					req := httptest.NewRequest(http.MethodGet, "/api/v1/latency", nil)
					req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Errorf("All latency endpoint returned %d instead of 200", rec.Code)
					}
					// Verify JSON is valid
					var resp map[string]interface{}
					if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
						t.Errorf("All latency endpoint returned invalid JSON: %v", err)
					}
				}
			}
		}()
	}

	t.Logf("Running concurrent latency API race test for 5s with 19 goroutines...")
	wg.Wait()
}
