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

// TestServer_LatencySnapshotPrimitive_Race tests the new Snapshot() primitive
// under concurrent access patterns matching production.
func TestServer_LatencySnapshotPrimitive_Race(t *testing.T) {
	m := state.NewManager()
	m.ConfigureICMP([]int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}, 3600)

	targetID := "test-router"

	// Pre-fill
	for i := 0; i < 3600; i++ {
		m.RecordICMPLatencyAt(targetID, float64(i%100)+10.0, true, time.Now().UTC())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for i := 0; ; i++ {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					m.RecordICMPLatency(targetID, float64((i*17)%200)+10.0, i%10 != 0)
				}
			}
		}()
	}

	// Snapshot readers (new primitive)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					snap := m.GetICMPSnapshot(targetID, 3600)
					if snap == nil {
						t.Errorf("Snapshot returned nil")
						continue
					}
					// Verify snapshot integrity
					if snap.Count < 0 {
						t.Errorf("Snapshot count negative: %d", snap.Count)
					}
					if snap.Count > snap.Capacity {
						t.Errorf("Snapshot count %d exceeds capacity %d", snap.Count, snap.Capacity)
					}
					if len(snap.Samples) != snap.Count {
						t.Errorf("Sample slice length %d != count %d", len(snap.Samples), snap.Count)
					}
				}
			}
		}()
	}

	// GetRecentSamples readers (existing primitive)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					samples := m.GetRecentICMPLatencySamples(targetID, 3600)
					if len(samples) > 3600 {
						t.Errorf("Got %d samples, expected max 3600", len(samples))
					}
				}
			}
		}()
	}

	t.Logf("Running Snapshot primitive race test...")
	wg.Wait()
}

// TestServer_LatencySnapshotOwnership verifies that the snapshot's samples slice
// is owned by the caller and mutations don't affect the tracker.
func TestServer_LatencySnapshotOwnership(t *testing.T) {
	m := state.NewManager()
	m.ConfigureICMP([]int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}, 3600)

	targetID := "test-router"

	// Record samples
	for i := 0; i < 100; i++ {
		m.RecordICMPLatencyAt(targetID, float64(i+10), true, time.Now().UTC())
	}

	// Get snapshot
	snap := m.GetICMPSnapshot(targetID, 50)
	if snap == nil || len(snap.Samples) == 0 {
		t.Fatalf("Expected non-empty snapshot")
	}

	originalValue := snap.Samples[0].LatencyMs

	// Mutate the snapshot's samples
	snap.Samples[0].LatencyMs = 9999.0
	snap.Samples[0].Reachable = false

	// Get another snapshot - should NOT see the mutation
	snap2 := m.GetICMPSnapshot(targetID, 50)
	if snap2 == nil || len(snap2.Samples) == 0 {
		t.Fatalf("Expected non-empty second snapshot")
	}

	if snap2.Samples[0].LatencyMs != originalValue {
		t.Errorf("Mutation of snapshot affected tracker state: got %f, want %f",
			snap2.Samples[0].LatencyMs, originalValue)
	}
	if snap2.Samples[0].Reachable != true {
		t.Errorf("Mutation of snapshot affected Reachable field")
	}
}

// TestServer_LatencySnapshot_EmptyTarget verifies that GetICMPSnapshot returns
// a valid empty snapshot for a target that has no data.
func TestServer_LatencySnapshot_EmptyTarget(t *testing.T) {
	m := state.NewManager()
	m.ConfigureICMP([]int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}, 3600)

	targetID := "nonexistent-target"

	// Get snapshot for non-existent target
	snap := m.GetICMPSnapshot(targetID, 50)
	if snap == nil {
		t.Fatalf("Expected non-nil snapshot for non-existent target")
	}

	if snap.TargetID != targetID {
		t.Errorf("Expected TargetID %q, got %q", targetID, snap.TargetID)
	}
	if snap.Count != 0 {
		t.Errorf("Expected Count 0, got %d", snap.Count)
	}
	if len(snap.Samples) != 0 {
		t.Errorf("Expected 0 samples, got %d", len(snap.Samples))
	}
	if len(snap.Buckets) == 0 {
		t.Errorf("Expected non-empty buckets")
	}
}

// TestServer_LatencySnapshot_WithHTTP verifies GetHTTPSnapshot works correctly.
func TestServer_LatencySnapshot_WithHTTP(t *testing.T) {
	m := state.NewManager()

	targetID := "http-target"

	// Record HTTP samples
	for i := 0; i < 50; i++ {
		m.RecordLatency(targetID, float64(i+10), true)
	}

	snap := m.GetHTTPSnapshot(targetID, 30)
	if snap == nil {
		t.Fatalf("Expected non-nil snapshot")
	}

	if snap.Count != 30 {
		t.Errorf("Expected Count 30, got %d", snap.Count)
	}
	if len(snap.Samples) != 30 {
		t.Errorf("Expected 30 samples, got %d", len(snap.Samples))
	}
	if snap.Capacity != 100 {
		t.Errorf("Expected Capacity 100, got %d", snap.Capacity)
	}
}

// TestServer_LatencySeriesHandler_UsesSnapshot verifies that the series handler
// returns correct response with metadata derived from Snapshot primitive.
func TestServer_LatencySeriesHandler_UsesSnapshot(t *testing.T) {
	salt := []byte("test-salt-snapshot")
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
			{ID: "test-router", Name: "Test Router", BaseURL: "http://192.168.1.1", Enabled: true},
		},
	}
	st := state.NewManager()
	st.ConfigureICMP([]int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}, 3600)
	srv := NewServer(cfg, st, nil, true)

	// Pre-fill ICMP tracker
	targetID := "test-router"
	for i := 0; i < 100; i++ {
		st.RecordICMPLatencyAt(targetID, float64(i%50)+10.0, true, time.Now().UTC())
	}

	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/series", http.HandlerFunc(srv.handleTargetLatencySeries)).Methods(http.MethodGet)

	// Test ICMP series
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/series?target_id="+targetID+"&probe_kind=icmp&range_seconds=3600&step_seconds=60", nil)
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

	// Verify series metadata comes from snapshot
	if series.TargetID != targetID {
		t.Errorf("Expected TargetID %q, got %q", targetID, series.TargetID)
	}
	if series.ProbeKind != "icmp" {
		t.Errorf("Expected ProbeKind 'icmp', got %q", series.ProbeKind)
	}
	// RetainedSampleCount should reflect actual samples in snapshot
	if series.RetainedSampleCount != series.SampleCount {
		t.Errorf("RetainedSampleCount %d != SampleCount %d", series.RetainedSampleCount, series.SampleCount)
	}
	// Points should be populated
	if len(series.Points) == 0 {
		t.Errorf("Expected non-empty points")
	}
}
