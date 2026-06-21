// Package probe implements independent latency probing for UVB-76.
package probe

import (
	"sync/atomic"
)

// Telemetry counters for ICMP exec churn monitoring.
// These are atomically updated to track ping execution health.
var (
	icmpPingStartedTotal   uint64
	icmpPingCompletedTotal uint64
	icmpPingTimeoutTotal   uint64
	icmpPingTruncatedTotal uint64
	icmpPingErrorTotal     uint64
	icmpPingInflight       int64
)

// ICMPPingCounters holds ICMP ping telemetry counters.
type ICMPPingCounters struct {
	Started   uint64
	Completed uint64
	Timeout   uint64
	Truncated uint64
	Error     uint64
	Inflight  int64
}

// GetICMPPingCounters returns current ICMP ping telemetry counters.
func GetICMPPingCounters() ICMPPingCounters {
	return ICMPPingCounters{
		Started:   atomic.LoadUint64(&icmpPingStartedTotal),
		Completed: atomic.LoadUint64(&icmpPingCompletedTotal),
		Timeout:   atomic.LoadUint64(&icmpPingTimeoutTotal),
		Truncated: atomic.LoadUint64(&icmpPingTruncatedTotal),
		Error:     atomic.LoadUint64(&icmpPingErrorTotal),
		Inflight:  atomic.LoadInt64(&icmpPingInflight),
	}
}

// ResetICMPPingCounters resets all ICMP ping counters to zero.
// Useful for testing.
func ResetICMPPingCounters() {
	atomic.StoreUint64(&icmpPingStartedTotal, 0)
	atomic.StoreUint64(&icmpPingCompletedTotal, 0)
	atomic.StoreUint64(&icmpPingTimeoutTotal, 0)
	atomic.StoreUint64(&icmpPingTruncatedTotal, 0)
	atomic.StoreUint64(&icmpPingErrorTotal, 0)
	atomic.StoreInt64(&icmpPingInflight, 0)
}

// IncPingStarted increments the started counter.
func IncPingStarted() {
	atomic.AddUint64(&icmpPingStartedTotal, 1)
	atomic.AddInt64(&icmpPingInflight, 1)
}

// IncPingCompleted increments the completed counter and decrements inflight.
func IncPingCompleted() {
	atomic.AddUint64(&icmpPingCompletedTotal, 1)
	atomic.AddInt64(&icmpPingInflight, -1)
}

// IncPingTimeout increments the timeout counter.
func IncPingTimeout() {
	atomic.AddUint64(&icmpPingTimeoutTotal, 1)
}

// IncPingTruncated increments the truncated counter.
func IncPingTruncated() {
	atomic.AddUint64(&icmpPingTruncatedTotal, 1)
}

// IncPingError increments the error counter.
func IncPingError() {
	atomic.AddUint64(&icmpPingErrorTotal, 1)
}
