// Package server provides the HTTP server for UVB-76.
package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/s1onique/KGB/uvb76/probe"
)

// ServerStatus represents the runtime status of the UVB-76 server.
type ServerStatus struct {
	StartedAt  string                           `json:"started_at"`
	ICMPOSPing *probe.ICMPPingTelemetrySnapshot `json:"icmp_os_ping,omitempty"`
	ICMPNative *probe.NativeICMPStatsSnapshot   `json:"icmp_native,omitempty"`
}

// handleStatus returns server runtime status including start time and ICMP telemetry.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	status := ServerStatus{
		StartedAt: s.startedAt.Format(time.RFC3339),
	}

	// Include ICMP OS ping telemetry if available
	if tm := probe.GetGlobalICMPTelemetry(); tm != nil {
		snap := tm.Snapshot()
		status.ICMPOSPing = &snap
	}

	// Include native ICMP telemetry if available
	if tm := probe.GetGlobalNativeICMPTelemetry(); tm != nil {
		snap := tm.Snapshot()
		status.ICMPNative = &snap
	}

	json.NewEncoder(w).Encode(status)
}
