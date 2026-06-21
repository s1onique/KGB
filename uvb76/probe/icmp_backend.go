// Package probe implements independent latency probing for UVB-76.
package probe

import (
	"context"
	"sync"
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
//
// Uses bounded command execution to prevent memory exhaustion on constrained routers.
// The bounded runner limits stdout/stderr capture and uses hard timeouts.
type OSPingBackend struct {
	runner       CommandRunner
	semaphore    *OSExecSemaphore
	mu           sync.Mutex // protects semaphore for lazy init
}

// NewOSPingBackend creates a new OS ping backend with bounded execution.
func NewOSPingBackend() *OSPingBackend {
	return &OSPingBackend{
		runner: NewBoundedCommandRunner(),
	}
}

// NewOSPingBackendWithLimit creates a new OS ping backend with explicit concurrency limit.
// maxConcurrent controls how many OS ping processes can run simultaneously.
func NewOSPingBackendWithLimit(maxConcurrent int) *OSPingBackend {
	return &OSPingBackend{
		runner:    NewBoundedCommandRunner(),
		semaphore: NewOSExecSemaphore(maxConcurrent),
	}
}

// getSemaphore returns the semaphore, initializing lazily if needed.
func (b *OSPingBackend) getSemaphore() *OSExecSemaphore {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.semaphore == nil {
		// Default to 1 for router safety (lazy initialization)
		b.semaphore = NewOSExecSemaphore(1)
	}
	return b.semaphore
}

// Ping implements ICMPProbeBackend by running the platform ping command.
// It parses common ping output forms including:
//   - iputils ping: "64 bytes from 1.2.3.4: icmp_seq=1 ttl=64 time=1.23 ms"
//   - BusyBox ping: "64 bytes from 1.2.3.4: seq=1 ttl=64 time=1.23 ms"
//   - macOS ping: "64 bytes from 1.2.3.4: icmp_seq=1 ttl=55 time=1.234 ms"
//
// Uses bounded command runner to prevent memory exhaustion.
func (b *OSPingBackend) Ping(ctx context.Context, host string, timeout time.Duration) (time.Duration, error) {
	// Acquire semaphore slot to limit concurrent OS pings
	sem := b.getSemaphore()
	guard := AcquireSemaphore(sem)
	defer guard.Release()

	// Use bounded runner instead of cmd.Output()
	return PingOSWithRunner(ctx, host, timeout, b.runner)
}
