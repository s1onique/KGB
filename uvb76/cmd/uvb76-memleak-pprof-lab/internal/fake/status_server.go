// Package fake provides a minimal fake tovarisch status server for the pprof lab.
//
// This fake server serves the status endpoint that UVB-76 expects.
// It is deterministic and only used by the localhost pprof lab.
package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

// StatusServer is a minimal fake tovarisch status server.
// This is an in-process HTTP server, not a subprocess.
type StatusServer struct {
	Port    string
	LogFile string

	// Internal state
	server *http.Server
	done   chan struct{}
	logF   *os.File
}

// StatusResponse is a minimal tovarisch-compatible status response.
type StatusResponse struct {
	Service   string         `json:"service"`
	Status    string         `json:"status"`
	Timestamp string         `json:"timestamp"`
	Uptime    int64          `json:"uptime_seconds"`
	Version   string         `json:"version"`
	Checks    []CheckEntry   `json:"checks"`
	Runtime   RuntimeMetrics `json:"runtime"`
}

// CheckEntry is a minimal health check entry.
type CheckEntry struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// RuntimeMetrics provides minimal runtime metrics.
type RuntimeMetrics struct {
	PID        int    `json:"pid"`
	RSSKib     uint64 `json:"rss_kib"`
	Goroutines int    `json:"goroutines"`
	NumGC      uint32 `json:"num_gc"`
}

var startTime = time.Now()

// Start starts the fake status server in-process.
func (s *StatusServer) Start() error {
	// Create log file
	var err error
	s.logF, err = os.OpenFile(s.LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}

	s.log("started on localhost:" + s.Port)

	// Bind port before returning - keep listener for Serve()
	ln, err := net.Listen("tcp", "localhost:"+s.Port)
	if err != nil {
		s.logF.Close()
		return fmt.Errorf("port %s not available: %w", s.Port, err)
	}

	// Initialize done channel
	s.done = make(chan struct{})

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/status", s.handleStatus)

	s.server = &http.Server{
		Addr:    "localhost:" + s.Port,
		Handler: mux,
	}

	// Start server with the already-bound listener
	go func() {
		defer close(s.done)
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.log(fmt.Sprintf("server error: %v", err))
		}
		s.log("server stopped")
	}()

	return nil
}

// Wait blocks until the server has stopped.
func (s *StatusServer) Wait() {
	if s.done != nil {
		<-s.done
	}
}

// Shutdown gracefully stops the server.
func (s *StatusServer) Shutdown(ctx context.Context) error {
	s.log("shutdown requested")
	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}
	if s.logF != nil {
		s.log("shutdown complete")
		s.logF.Close()
	}
	return nil
}

// log writes a timestamped line to the log file.
func (s *StatusServer) log(msg string) {
	if s.logF != nil {
		fmt.Fprintf(s.logF, "[%s] %s\n", time.Now().Format(time.RFC3339), msg)
	}
}

// handleStatus serves the /status endpoint.
func (s *StatusServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.log(fmt.Sprintf("request: %s %s", r.Method, r.URL.Path))

	// Only accept GET requests
	if r.Method != http.MethodGet {
		s.log("response: 405 method not allowed")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uptime := int64(time.Since(startTime).Seconds())

	resp := StatusResponse{
		Service:   "tovarisch",
		Status:    "ok",
		Timestamp: time.Now().Format(time.RFC3339),
		Uptime:    uptime,
		Version:   "fake-0.1.0",
		Checks: []CheckEntry{
			{Name: "process", Status: "ok", Detail: "running"},
			{Name: "binary", Status: "ok", Detail: "tovarisch"},
			{Name: "config", Status: "ok", Detail: "lab-mode"},
			{Name: "http", Status: "ok", Detail: "localhost"},
			{Name: "tunnel", Status: "warn", Detail: "no tunnel detected"},
			{Name: "wg_peers", Status: "warn", Detail: "no peers configured"},
			{Name: "bgp", Status: "warn", Detail: "BGP not configured"},
		},
		Runtime: RuntimeMetrics{
			PID:        os.Getpid(),
			RSSKib:     1024, // Fixed fake RSS
			Goroutines: 5,    // Fixed fake goroutine count
			NumGC:      0,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	s.log("response: 200 OK")
	json.NewEncoder(w).Encode(resp)
}

// GetURL returns the status endpoint URL.
func (s *StatusServer) GetURL() string {
	return fmt.Sprintf("http://localhost:%s/status", s.Port)
}
