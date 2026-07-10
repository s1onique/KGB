package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestPProfConfigDefaults(t *testing.T) {
	cfg := PProfConfig{}
	cfg.ApplyDefaults()

	if cfg.Enabled != DefaultPProfEnabled {
		t.Errorf("Expected Enabled=%v, got %v", DefaultPProfEnabled, cfg.Enabled)
	}
	if cfg.Listen != DefaultPProfListen {
		t.Errorf("Expected Listen=%q, got %q", DefaultPProfListen, cfg.Listen)
	}
	if cfg.MemProfileRate != DefaultMemProfileRate {
		t.Errorf("Expected MemProfileRate=%d, got %d", DefaultMemProfileRate, cfg.MemProfileRate)
	}
}

func TestPProfConfigParse(t *testing.T) {
	jsonData := `{
		"enabled": true,
		"listen": "0.0.0.0:9999",
		"mem_profile_rate": 8192
	}`

	var cfg PProfConfig
	if err := json.Unmarshal([]byte(jsonData), &cfg); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if !cfg.Enabled {
		t.Error("Expected Enabled=true")
	}
	if cfg.Listen != "0.0.0.0:9999" {
		t.Errorf("Expected Listen=%q, got %q", "0.0.0.0:9999", cfg.Listen)
	}
	if cfg.MemProfileRate != 8192 {
		t.Errorf("Expected MemProfileRate=8192, got %d", cfg.MemProfileRate)
	}
}

func TestPProfConfigValidate(t *testing.T) {
	// Disabled config is always valid
	cfg := PProfConfig{Enabled: false}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Expected no error for disabled config, got %v", err)
	}

	// Enabled with listen is valid
	cfg = PProfConfig{Enabled: true, Listen: "127.0.0.1:6060"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Expected no error for enabled config with listen, got %v", err)
	}

	// Enabled without listen is invalid
	cfg = PProfConfig{Enabled: true, Listen: ""}
	if err := cfg.Validate(); err == nil {
		t.Error("Expected error for enabled config without listen")
	}
}

func TestApplyPProfRuntimeConfigDisabled(t *testing.T) {
	// Save original rate and restore after test
	originalRate := runtime.MemProfileRate
	defer func() { runtime.MemProfileRate = originalRate }()

	// Set a known rate before test
	runtime.MemProfileRate = 16384

	// Disabled config should NOT modify MemProfileRate
	cfg := PProfConfig{Enabled: false, MemProfileRate: 32768}
	ApplyPProfRuntimeConfig(cfg)

	// Rate should remain unchanged (16384)
	if runtime.MemProfileRate != 16384 {
		t.Errorf("Expected MemProfileRate=16384 after disabled config, got %d", runtime.MemProfileRate)
	}
}

func TestApplyPProfRuntimeConfigEnabled(t *testing.T) {
	// Save original rate and restore after test
	originalRate := runtime.MemProfileRate
	defer func() { runtime.MemProfileRate = originalRate }()

	// Set a known rate before test
	runtime.MemProfileRate = 8192

	// Enabled config with positive rate should modify MemProfileRate
	cfg := PProfConfig{Enabled: true, MemProfileRate: 16384}
	ApplyPProfRuntimeConfig(cfg)

	// Rate should be updated to configured value
	if runtime.MemProfileRate != 16384 {
		t.Errorf("Expected MemProfileRate=16384 after enabled config, got %d", runtime.MemProfileRate)
	}
}

func TestApplyPProfRuntimeConfigZeroRate(t *testing.T) {
	// Save original rate and restore after test
	originalRate := runtime.MemProfileRate
	defer func() { runtime.MemProfileRate = originalRate }()

	// Set a known rate before test
	runtime.MemProfileRate = 8192

	// Zero rate should NOT modify MemProfileRate (guard against accidental changes)
	cfg := PProfConfig{Enabled: true, MemProfileRate: 0}
	ApplyPProfRuntimeConfig(cfg)

	// Rate should remain unchanged
	if runtime.MemProfileRate != 8192 {
		t.Errorf("Expected MemProfileRate=8192 after zero-rate config, got %d", runtime.MemProfileRate)
	}
}

func TestApplyPProfRuntimeConfigDisabledWithPositiveRate(t *testing.T) {
	// Regression test: disabled config with positive MemProfileRate should not change rate
	originalRate := runtime.MemProfileRate
	defer func() { runtime.MemProfileRate = originalRate }()

	runtime.MemProfileRate = 4096

	cfg := PProfConfig{Enabled: false, MemProfileRate: 16384}
	ApplyPProfRuntimeConfig(cfg)

	if runtime.MemProfileRate != 4096 {
		t.Errorf("Disabled config changed MemProfileRate from 4096 to %d", runtime.MemProfileRate)
	}
}

func TestPProfMuxEndpoints(t *testing.T) {
	mux := PProfMux()

	endpoints := []string{
		"/debug/pprof/",
		"/debug/pprof/heap",
		"/debug/pprof/goroutine",
		"/debug/pprof/allocs",
		"/debug/pprof/cmdline",
		"/debug/pprof/profile",
		"/debug/pprof/symbol",
		"/debug/pprof/trace",
		"/debug/pprof/block",
		"/debug/pprof/mutex",
	}

	for _, path := range endpoints {
		req := httptest.NewRequest("GET", path, nil)
		handler, _ := mux.Handler(req)
		if handler == nil {
			t.Errorf("No handler registered for %s", path)
		}
	}
}

func TestPProfMuxWorksWithoutDefaultServeMux(t *testing.T) {
	// Regression test: PProfMux should work even if DefaultServeMux is replaced
	// with a fresh empty mux for the duration of the test.

	// Save and replace DefaultServeMux
	origMux := http.DefaultServeMux
	defer func() { http.DefaultServeMux = origMux }()
	http.DefaultServeMux = http.NewServeMux()

	// Create pprof mux using the fresh DefaultServeMux
	mux := PProfMux()

	// Verify handlers still work
	req := httptest.NewRequest("GET", "/debug/pprof/cmdline", nil)
	handler, pattern := mux.Handler(req)
	if handler == nil {
		t.Fatal("No handler registered")
	}
	if pattern != "/debug/pprof/cmdline" {
		t.Errorf("Expected pattern /debug/pprof/cmdline, got %s", pattern)
	}

	// Verify handler can serve a response without panicking
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

func TestPProfMuxHeapHandler(t *testing.T) {
	mux := PProfMux()

	req := httptest.NewRequest("GET", "/debug/pprof/heap?gc=1", nil)
	handler, _ := mux.Handler(req)

	if handler == nil {
		t.Fatal("No handler for /debug/pprof/heap")
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusInternalServerError {
		t.Errorf("Handler returned 500: %s", rec.Body.String())
	}
}

func TestNewPProfServer(t *testing.T) {
	cfg := PProfConfig{
		Enabled: true,
		Listen:  "127.0.0.1:6060",
	}

	srv := NewPProfServer(cfg)

	if srv == nil {
		t.Fatal("Expected non-nil server")
	}
	if srv.Addr != "127.0.0.1:6060" {
		t.Errorf("Expected Addr 127.0.0.1:6060, got %s", srv.Addr)
	}
	if srv.Handler == nil {
		t.Error("Expected non-nil handler")
	}
}

func TestPProfConfigJSONRoundTrip(t *testing.T) {
	cfg := PProfConfig{
		Enabled:        true,
		Listen:         "127.0.0.1:6060",
		MemProfileRate: 65536,
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed PProfConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.Enabled != cfg.Enabled {
		t.Errorf("Enabled mismatch: got %v, want %v", parsed.Enabled, cfg.Enabled)
	}
	if parsed.Listen != cfg.Listen {
		t.Errorf("Listen mismatch: got %q, want %q", parsed.Listen, cfg.Listen)
	}
	if parsed.MemProfileRate != cfg.MemProfileRate {
		t.Errorf("MemProfileRate mismatch: got %d, want %d", parsed.MemProfileRate, cfg.MemProfileRate)
	}
}
