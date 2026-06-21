// Package probe implements independent latency probing for UVB-76.
package probe

import (
	"sync"
	"sync/atomic"
)

// MaxLastErrorLen is the maximum length for last_error to prevent unbounded memory growth.
const MaxLastErrorLen = 256

// ICMPPingTelemetry records ICMP OS ping telemetry in the daemon.
// This is the authoritative source for ICMP ping counters that can be
// exposed via the daemon's HTTP status API.
type ICMPPingTelemetry struct {
	enabled       atomic.Bool
	attempts      atomic.Uint64
	successes     atomic.Uint64
	failures      atomic.Uint64
	maxConcurrent int
	lastErrorMu   sync.Mutex
	lastError     string
}

// ICMPPingTelemetrySnapshot is a read-only snapshot of ICMP ping telemetry.
type ICMPPingTelemetrySnapshot struct {
	Enabled       bool   `json:"enabled"`
	Attempts      uint64 `json:"attempts"`
	Successes     uint64 `json:"successes"`
	Failures      uint64 `json:"failures"`
	LastError     string `json:"last_error,omitempty"`
	MaxConcurrent int    `json:"max_concurrent"`
}

// NewICMPPingTelemetry creates a new ICMP ping telemetry recorder.
func NewICMPPingTelemetry(enabled bool, maxConcurrent int) *ICMPPingTelemetry {
	t := &ICMPPingTelemetry{
		maxConcurrent: maxConcurrent,
	}
	t.enabled.Store(enabled)
	return t
}

// SetEnabled sets whether ICMP ping is enabled.
func (t *ICMPPingTelemetry) SetEnabled(enabled bool) {
	t.enabled.Store(enabled)
}

// IsEnabled returns whether ICMP ping is enabled.
func (t *ICMPPingTelemetry) IsEnabled() bool {
	return t.enabled.Load()
}

// RecordAttempt increments the attempt counter.
func (t *ICMPPingTelemetry) RecordAttempt() {
	t.attempts.Add(1)
}

// RecordSuccess increments the success counter.
func (t *ICMPPingTelemetry) RecordSuccess() {
	t.successes.Add(1)
}

// RecordFailure records a failed ping with the error message.
func (t *ICMPPingTelemetry) RecordFailure(errMsg string) {
	t.failures.Add(1)
	// Bounded error storage
	t.lastErrorMu.Lock()
	if len(errMsg) > MaxLastErrorLen {
		errMsg = errMsg[:MaxLastErrorLen]
	}
	t.lastError = errMsg
	t.lastErrorMu.Unlock()
}

// Snapshot returns a read-only copy of the telemetry.
func (t *ICMPPingTelemetry) Snapshot() ICMPPingTelemetrySnapshot {
	t.lastErrorMu.Lock()
	lastErr := t.lastError
	t.lastErrorMu.Unlock()

	return ICMPPingTelemetrySnapshot{
		Enabled:       t.enabled.Load(),
		Attempts:      t.attempts.Load(),
		Successes:     t.successes.Load(),
		Failures:      t.failures.Load(),
		LastError:     lastErr,
		MaxConcurrent: t.maxConcurrent,
	}
}

// Global telemetry instance for daemon-owned ICMP ping telemetry.
// This is the authoritative source exposed via HTTP status API.
var globalICMPTelemetry *ICMPPingTelemetry

// Global telemetry mutex for lazy initialization.
var globalTelemetryMu sync.Mutex

// InitGlobalICMPTelemetry initializes the global ICMP telemetry.
func InitGlobalICMPTelemetry(enabled bool, maxConcurrent int) {
	globalTelemetryMu.Lock()
	defer globalTelemetryMu.Unlock()
	if globalICMPTelemetry == nil {
		globalICMPTelemetry = NewICMPPingTelemetry(enabled, maxConcurrent)
	}
}

// GetGlobalICMPTelemetry returns the global ICMP telemetry instance.
func GetGlobalICMPTelemetry() *ICMPPingTelemetry {
	globalTelemetryMu.Lock()
	defer globalTelemetryMu.Unlock()
	return globalICMPTelemetry
}

// ResetGlobalICMPTelemetry resets the global telemetry (for testing).
func ResetGlobalICMPTelemetry() {
	globalTelemetryMu.Lock()
	tm := globalICMPTelemetry
	globalTelemetryMu.Unlock()

	if tm == nil {
		return
	}

	tm.attempts.Store(0)
	tm.successes.Store(0)
	tm.failures.Store(0)

	tm.lastErrorMu.Lock()
	tm.lastError = ""
	tm.lastErrorMu.Unlock()
}
