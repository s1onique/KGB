// diagnostics_handlers.go — UVB-76 diagnostic HTTP handlers
//
// Provides memory attribution diagnostic endpoints for the memory-lab tool.
// These handlers capture Go runtime evidence from the UVB-76 process.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package server

import (
	"encoding/json"
	"net/http"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"time"
)

// MemStatsSnapshot represents a Go runtime memory stats snapshot.
type MemStatsSnapshot struct {
	Timestamp    string `json:"timestamp"`
	PID          int    `json:"pid"`
	Goroutines   uint64 `json:"goroutines"`
	HeapAlloc    uint64 `json:"heap_alloc_bytes"`
	HeapInuse    uint64 `json:"heap_inuse_bytes"`
	HeapObjects  uint64 `json:"heap_objects"`
	HeapSys      uint64 `json:"heap_sys_bytes"`
	Sys          uint64 `json:"sys_bytes"`
	HeapReleased uint64 `json:"heap_released_bytes"`
	HeapIdle     uint64 `json:"heap_idle_bytes"`
	GCCount      uint32 `json:"gc_count"`
	GCPauseNs    uint64 `json:"gc_pause_total_ns"`
	NextGC       uint64 `json:"next_gc_bytes"`
	Mallocs      uint64 `json:"mallocs"`
	Frees        uint64 `json:"frees"`
}

// handleMemStatsSnapshot returns Go runtime memory stats from UVB-76 process.
// GET /api/v1/diagnostics/memstats
// CRITICAL: This endpoint is only available when devMode is enabled.
func (s *Server) handleMemStatsSnapshot(w http.ResponseWriter, r *http.Request) {
	// Reject if not in dev mode - these are sensitive diagnostic endpoints
	if !s.devMode {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "diagnostics disabled in production"})
		return
	}

	// Optional query param: ?force_gc=true to force GC before capturing
	forceGC := r.URL.Query().Get("force_gc") == "true"

	if forceGC {
		// Preserve and restore GC percent
		old := debug.SetGCPercent(-1)
		runtime.GC()
		runtime.GC()
		debug.SetGCPercent(old)
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	snapshot := MemStatsSnapshot{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		PID:         s.getPID(),
		Goroutines:  uint64(runtime.NumGoroutine()),
		HeapAlloc:   memStats.Alloc,
		HeapInuse:   memStats.HeapInuse,
		HeapObjects: memStats.HeapObjects,
		HeapSys:     memStats.HeapSys,
		Sys:         memStats.Sys,
		HeapReleased: memStats.HeapReleased,
		HeapIdle:    memStats.HeapIdle,
		GCCount:     memStats.NumGC,
		GCPauseNs:   memStats.PauseTotalNs,
		NextGC:      memStats.NextGC,
		Mallocs:     memStats.Mallocs,
		Frees:       memStats.Frees,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshot)
}

// handleHeapProfile returns pprof heap profile from UVB-76 process.
// GET /api/v1/diagnostics/heap-profile
// CRITICAL: This endpoint is only available when devMode is enabled.
func (s *Server) handleHeapProfile(w http.ResponseWriter, r *http.Request) {
	// Reject if not in dev mode - heap profiles expose sensitive runtime data
	if !s.devMode {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "diagnostics disabled in production"})
		return
	}

	// Force GC before capturing heap profile for accurate view
	runtime.GC()

	heap := pprof.Lookup("heap")
	if heap == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "heap profile not available"})
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=heap.pprof")

	if err := heap.WriteTo(w, 0); err != nil {
		// Headers already sent, can't change status now
		return
	}
}

// handleGoroutineDump returns goroutine stack dump from UVB-76 process.
// GET /api/v1/diagnostics/goroutine-dump
// CRITICAL: This endpoint is only available when devMode is enabled.
func (s *Server) handleGoroutineDump(w http.ResponseWriter, r *http.Request) {
	// Reject if not in dev mode - goroutine dumps expose sensitive runtime data
	if !s.devMode {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "diagnostics disabled in production"})
		return
	}

	// Write pprof goroutine profile
	prof := pprof.Lookup("goroutine")
	if prof == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "goroutine profile not available"})
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	if err := prof.WriteTo(w, 0); err != nil {
		return
	}
}
