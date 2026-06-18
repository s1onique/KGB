// Package probe implements independent latency probing for UVB-76.
package probe

import (
	"context"
	"time"
)

// ICMPProbeBackend is the interface for ICMP ping backends.
// Raw ICMP requires elevated privileges, so we use OS ping command.
type ICMPProbeBackend interface {
	// Ping performs an ICMP ping to the given host with the specified timeout.
	// Returns the round-trip time and any error encountered.
	Ping(ctx context.Context, host string, timeout time.Duration) (time.Duration, error)
}

// OSPingBackend implements ICMPProbeBackend using the platform's ping command.
// This works on unprivileged systems (ASUSWRT/Entware-style routers).
type OSPingBackend struct{}

// NewOSPingBackend creates a new OS ping backend.
func NewOSPingBackend() *OSPingBackend {
	return &OSPingBackend{}
}

// Ping implements ICMPProbeBackend by running the platform ping command.
// It parses common ping output forms including:
//   - iputils ping: "64 bytes from 1.2.3.4: icmp_seq=1 ttl=64 time=1.23 ms"
//   - BusyBox ping: "64 bytes from 1.2.3.4: seq=1 ttl=64 time=1.23 ms"
//   - macOS ping: "64 bytes from 1.2.3.4: icmp_seq=1 ttl=64 time=1.234 ms"
func (b *OSPingBackend) Ping(ctx context.Context, host string, timeout time.Duration) (time.Duration, error) {
	return PingOS(ctx, host, timeout)
}
