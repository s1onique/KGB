// Package config provides configuration types for UVB-76 diagnostics.
package config

import (
	"fmt"
	"net/http"
	"runtime"
	pprof "net/http/pprof"
)

// Default pprof settings.
const (
	DefaultPProfEnabled   = false
	DefaultPProfListen   = "127.0.0.1:6060"
	DefaultMemProfileRate = 65536
)

// PProfConfig holds pprof profiling settings.
type PProfConfig struct {
	// Enabled controls whether pprof server is started.
	// Default: false (disabled)
	Enabled bool `json:"enabled"`
	// Listen is the address for the debug HTTP server.
	// Default: "127.0.0.1:6060"
	Listen string `json:"listen"`
	// MemProfileRate sets runtime.MemProfileRate.
	// Values <= 0 mean "do not change the default".
	// Default: 65536
	MemProfileRate int `json:"mem_profile_rate"`
}

// ApplyDefaults sets default values for unset fields.
func (p *PProfConfig) ApplyDefaults() {
	if p.Listen == "" {
		p.Listen = DefaultPProfListen
	}
	if p.MemProfileRate == 0 {
		p.MemProfileRate = DefaultMemProfileRate
	}
}

// Validate checks that the config is valid.
func (p *PProfConfig) Validate() error {
	if !p.Enabled {
		return nil
	}
	if p.Listen == "" {
		return fmt.Errorf("pprof.listen is required when pprof is enabled")
	}
	return nil
}

// ApplyPProfRuntimeConfig applies runtime settings if pprof is enabled.
// This modifies runtime.MemProfileRate only when cfg.Enabled && cfg.MemProfileRate > 0.
func ApplyPProfRuntimeConfig(cfg PProfConfig) {
	if !cfg.Enabled || cfg.MemProfileRate <= 0 {
		return
	}
	runtime.MemProfileRate = cfg.MemProfileRate
}

// PProfMux returns a dedicated mux with pprof handlers registered.
// Uses direct pprof handler registration to avoid DefaultServeMux dependency.
func PProfMux() *http.ServeMux {
	mux := http.NewServeMux()
	// Register pprof handlers directly from net/http/pprof package
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	return mux
}

// NewPProfServer creates an http.Server configured for pprof debug endpoints.
func NewPProfServer(cfg PProfConfig) *http.Server {
	return &http.Server{
		Addr:    cfg.Listen,
		Handler: PProfMux(),
	}
}
