package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/auth"
	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// Helper function to create a pointer to a bool
func boolPtr(b bool) *bool {
	return &b
}

func TestHealthzEndpoint(t *testing.T) {
	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: "sha256:aaaaaaaaaaaaaaaa:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)

	router := mux.NewRouter()
	router.Handle("/api/v1/healthz", http.HandlerFunc(srv.handleHealthz))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", resp["status"])
	}
}

func TestTargetsEndpoint_UnauthenticatedReturnsJSON401(t *testing.T) {
	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: "sha256:aaaaaaaaaaaaaaaa:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/targets", http.HandlerFunc(srv.handleTargets))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for unauthenticated request, got %d", rec.Code)
	}

	// Should not have WWW-Authenticate header
	if wwwAuth := rec.Header().Get("WWW-Authenticate"); wwwAuth != "" {
		t.Errorf("Should not have WWW-Authenticate header, got '%s'", wwwAuth)
	}

	// Should be JSON response
	if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

func TestTargetsEndpoint_Authenticated(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{{ID: "test-1", Name: "Test", BaseURL: "http://localhost", Enabled: true}},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)

	// Generate a valid token
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/targets", http.HandlerFunc(srv.handleTargets))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200 with valid session, got %d", rec.Code)
	}

	var targets []config.TargetConfig
	json.NewDecoder(rec.Body).Decode(&targets)
	if len(targets) != 1 {
		t.Errorf("Expected 1 target, got %d", len(targets))
	}
}

// Latency API Tests

func TestHandleTargetLatency_Returns404ForNonexistentTarget(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{{ID: "test-1", Name: "Test", BaseURL: "http://localhost", Enabled: true}},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)

	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/targets/{id}/latency", http.HandlerFunc(srv.handleTargetLatency))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets/nonexistent/latency", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for nonexistent target, got %d", rec.Code)
	}
}

func TestHandleTargetLatency_ReturnsSummary(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	// Enable HTTP latency in config
	httpCfg := config.HTTPProbeConfig{
		Enabled:             boolPtr(true),
		IntervalSeconds:     15,
		TimeoutMilliseconds: 10000,
		WindowSeconds:       300,
		RetainedRangeSeconds: 3000,
		RecentSamplesMax:    100,
		HistogramBucketsMS:  config.DefaultHistogramBuckets(),
	}
	icmpCfg := config.ICMPProbeConfig{
		Enabled:              boolPtr(true),
		IntervalSeconds:      1,
		TimeoutSeconds:       3,
		WindowSeconds:        300,
		RetainedRangeSeconds: 3000,
		RecentSamplesMax:     100,
		HistogramBucketsMS:   config.DefaultHistogramBuckets(),
	}

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency:  config.LatencyConfig{HTTP: httpCfg, ICMP: icmpCfg},
		Targets:  []config.TargetConfig{{ID: "test-1", Name: "Test", BaseURL: "http://localhost", Enabled: true}},
	}
	st := state.NewManager()
	
	// Add some latency data (HTTP samples)
	st.RecordLatency("test-1", 10.0, true)
	st.RecordLatency("test-1", 20.0, true)
	st.RecordLatency("test-1", 30.0, true)

	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/targets/{id}/latency", http.HandlerFunc(srv.handleTargetLatency))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets/test-1/latency", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	// Response is now TargetLatencyResponse with HTTP and ICMP fields
	var resp struct {
		TargetID string           `json:"target_id"`
		HTTP     *state.LatencySummary `json:"http,omitempty"`
		ICMP     *state.LatencySummary `json:"icmp,omitempty"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	
	if resp.TargetID != "test-1" {
		t.Errorf("Expected target_id 'test-1', got %q", resp.TargetID)
	}
	if resp.HTTP == nil {
		t.Fatal("Expected HTTP latency summary, got nil")
	}
	if resp.HTTP.SampleCount != 3 {
		t.Errorf("Expected 3 samples, got %d", resp.HTTP.SampleCount)
	}
	if resp.HTTP.MinLatencyMs != 10.0 {
		t.Errorf("Expected min 10.0, got %f", resp.HTTP.MinLatencyMs)
	}
	if resp.HTTP.MaxLatencyMs != 30.0 {
		t.Errorf("Expected max 30.0, got %f", resp.HTTP.MaxLatencyMs)
	}
}

func TestHandleTargetLatencySamples_Returns404ForNonexistentTarget(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{{ID: "test-1", Name: "Test", BaseURL: "http://localhost", Enabled: true}},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/targets/{id}/latency/samples", http.HandlerFunc(srv.handleTargetLatencySamples))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets/nonexistent/latency/samples", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for nonexistent target, got %d", rec.Code)
	}
}

func TestHandleTargetLatencySamples_ReturnsSamples(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{{ID: "test-1", Name: "Test", BaseURL: "http://localhost", Enabled: true}},
	}
	st := state.NewManager()
	
	// Add some latency data
	st.RecordLatency("test-1", 10.0, true)
	st.RecordLatency("test-1", 20.0, true)

	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/targets/{id}/latency/samples", http.HandlerFunc(srv.handleTargetLatencySamples))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets/test-1/latency/samples?limit=10", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var samples []state.LatencySample
	json.NewDecoder(rec.Body).Decode(&samples)
	if len(samples) != 2 {
		t.Errorf("Expected 2 samples, got %d", len(samples))
	}
}

func TestHandleTargetLatencySamples_RespectsLimit(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{{ID: "test-1", Name: "Test", BaseURL: "http://localhost", Enabled: true}},
	}
	st := state.NewManager()
	
	// Add some latency data
	st.RecordLatency("test-1", 10.0, true)
	st.RecordLatency("test-1", 20.0, true)
	st.RecordLatency("test-1", 30.0, true)
	st.RecordLatency("test-1", 40.0, true)

	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/targets/{id}/latency/samples", http.HandlerFunc(srv.handleTargetLatencySamples))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets/test-1/latency/samples?limit=2", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var samples []state.LatencySample
	json.NewDecoder(rec.Body).Decode(&samples)
	if len(samples) != 2 {
		t.Errorf("Expected 2 samples with limit=2, got %d", len(samples))
	}
}

func TestHandleAllLatency_ReturnsAllSummaries(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{{ID: "test-1", Name: "Test1", BaseURL: "http://localhost", Enabled: true}},
	}
	st := state.NewManager()
	
	// Add latency data for different targets
	st.RecordLatency("test-1", 10.0, true)
	st.RecordLatency("test-1", 20.0, true)

	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency", http.HandlerFunc(srv.handleAllLatency))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var summaries map[string]state.LatencySummary
	json.NewDecoder(rec.Body).Decode(&summaries)
	if len(summaries) != 1 {
		t.Errorf("Expected 1 summary for test-1, got %d", len(summaries))
	}
}

func TestLatencyEndpoint_UnauthenticatedReturnsJSON401(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{{ID: "test-1", Name: "Test", BaseURL: "http://localhost", Enabled: true}},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/targets/{id}/latency", http.HandlerFunc(srv.handleTargetLatency))

	// Request without auth
	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets/test-1/latency", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 without auth, got %d", rec.Code)
	}

	// Should NOT have WWW-Authenticate header
	if wwwAuth := rec.Header().Get("WWW-Authenticate"); wwwAuth != "" {
		t.Errorf("Should not have WWW-Authenticate header, got '%s'", wwwAuth)
	}
}
