// Package probe implements independent latency probing for UVB-76.
package probe

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestICMPPingTelemetryRecordAttempt(t *testing.T) {
	tm := NewICMPPingTelemetry(true, 1)
	
	if tm.IsEnabled() != true {
		t.Errorf("expected enabled=true, got %v", tm.IsEnabled())
	}
	
	initial := tm.attempts.Load()
	tm.RecordAttempt()
	if got := tm.attempts.Load(); got != initial+1 {
		t.Errorf("expected attempts=%d, got %d", initial+1, got)
	}
}

func TestICMPPingTelemetryRecordSuccess(t *testing.T) {
	tm := NewICMPPingTelemetry(true, 1)
	
	initial := tm.successes.Load()
	tm.RecordSuccess()
	if got := tm.successes.Load(); got != initial+1 {
		t.Errorf("expected successes=%d, got %d", initial+1, got)
	}
}

func TestICMPPingTelemetryRecordFailure(t *testing.T) {
	tm := NewICMPPingTelemetry(true, 1)
	
	initial := tm.failures.Load()
	testErr := "test error message"
	tm.RecordFailure(testErr)
	if got := tm.failures.Load(); got != initial+1 {
		t.Errorf("expected failures=%d, got %d", initial+1, got)
	}
	
	// Check last error is stored
	tm.lastErrorMu.Lock()
	if tm.lastError != testErr {
		t.Errorf("expected lastError=%q, got %q", testErr, tm.lastError)
	}
	tm.lastErrorMu.Unlock()
}

func TestICMPPingTelemetryRecordFailureBounded(t *testing.T) {
	tm := NewICMPPingTelemetry(true, 1)
	
	// Test that long error messages are truncated
	longErr := make([]byte, MaxLastErrorLen*2)
	for i := range longErr {
		longErr[i] = 'x'
	}
	longErrStr := string(longErr)
	
	tm.RecordFailure(longErrStr)
	
	tm.lastErrorMu.Lock()
	defer tm.lastErrorMu.Unlock()
	if len(tm.lastError) != MaxLastErrorLen {
		t.Errorf("expected lastError len=%d, got %d", MaxLastErrorLen, len(tm.lastError))
	}
}

func TestICMPPingTelemetrySnapshot(t *testing.T) {
	tm := NewICMPPingTelemetry(true, 2)
	
	// Record some telemetry
	tm.RecordAttempt()
	tm.RecordAttempt()
	tm.RecordSuccess()
	tm.RecordFailure("test error")
	
	snap := tm.Snapshot()
	
	if snap.Enabled != true {
		t.Errorf("expected Enabled=true, got %v", snap.Enabled)
	}
	if snap.Attempts != 2 {
		t.Errorf("expected Attempts=2, got %d", snap.Attempts)
	}
	if snap.Successes != 1 {
		t.Errorf("expected Successes=1, got %d", snap.Successes)
	}
	if snap.Failures != 1 {
		t.Errorf("expected Failures=1, got %d", snap.Failures)
	}
	if snap.LastError != "test error" {
		t.Errorf("expected LastError='test error', got %q", snap.LastError)
	}
	if snap.MaxConcurrent != 2 {
		t.Errorf("expected MaxConcurrent=2, got %d", snap.MaxConcurrent)
	}
}

func TestICMPPingTelemetrySnapshotImmutable(t *testing.T) {
	tm := NewICMPPingTelemetry(true, 1)
	tm.RecordAttempt()
	tm.RecordSuccess()
	
	// Get snapshot
	snap1 := tm.Snapshot()
	
	// Record more telemetry
	tm.RecordAttempt()
	tm.RecordFailure("new error")
	
	// Get another snapshot
	snap2 := tm.Snapshot()
	
	// Snapshots should be independent
	if snap1.Attempts != 1 || snap2.Attempts != 2 {
		t.Errorf("snapshots should be independent: snap1.Attempts=%d, snap2.Attempts=%d", snap1.Attempts, snap2.Attempts)
	}
	if snap1.Successes != 1 || snap2.Successes != 1 {
		t.Errorf("snapshots should be independent: snap1.Successes=%d, snap2.Successes=%d", snap1.Successes, snap2.Successes)
	}
	if snap1.Failures != 0 || snap2.Failures != 1 {
		t.Errorf("snapshots should be independent: snap1.Failures=%d, snap2.Failures=%d", snap1.Failures, snap2.Failures)
	}
}

func TestICMPPingTelemetryConcurrency(t *testing.T) {
	tm := NewICMPPingTelemetry(true, 1)
	
	const goroutines = 10
	const opsPerGoroutine = 100
	
	var wg sync.WaitGroup
	wg.Add(goroutines * 3) // 3 types of operations
	
	// Concurrent attempts
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				tm.RecordAttempt()
			}
		}()
	}
	
	// Concurrent successes
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				tm.RecordSuccess()
			}
		}()
	}
	
	// Concurrent failures
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				tm.RecordFailure("error")
			}
		}()
	}
	
	wg.Wait()
	
	// Verify final counts
	expected := uint64(goroutines * opsPerGoroutine)
	if got := tm.attempts.Load(); got != expected {
		t.Errorf("expected attempts=%d, got %d", expected, got)
	}
	if got := tm.successes.Load(); got != expected {
		t.Errorf("expected successes=%d, got %d", expected, got)
	}
	if got := tm.failures.Load(); got != expected {
		t.Errorf("expected failures=%d, got %d", expected, got)
	}
}

func TestICMPPingTelemetrySnapshotConcurrency(t *testing.T) {
	tm := NewICMPPingTelemetry(true, 1)
	
	// Start recording in background
	stop := make(chan struct{})
	var wg sync.WaitGroup
	
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
				tm.RecordAttempt()
				tm.RecordSuccess()
				tm.RecordFailure("error")
				i++
			}
		}
	}()
	
	// Take snapshots concurrently
	var snapshotWg sync.WaitGroup
	const snapshotGoroutines = 5
	const snapshotsPerGoroutine = 100
	
	for i := 0; i < snapshotGoroutines; i++ {
		snapshotWg.Add(1)
		go func() {
			defer snapshotWg.Done()
			for j := 0; j < snapshotsPerGoroutine; j++ {
				_ = tm.Snapshot()
			}
		}()
	}
	
	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
	snapshotWg.Wait()
	
	// Final snapshot should be consistent
	snap := tm.Snapshot()
	if snap.Attempts == 0 || snap.Successes == 0 || snap.Failures == 0 {
		t.Errorf("final snapshot should have non-zero counts: attempts=%d, successes=%d, failures=%d",
			snap.Attempts, snap.Successes, snap.Failures)
	}
}

func TestGlobalICMPTelemetry(t *testing.T) {
	// Reset global telemetry for clean test
	globalICMPTelemetry = nil
	
	// Initialize global telemetry
	InitGlobalICMPTelemetry(true, 3)
	
	tm := GetGlobalICMPTelemetry()
	if tm == nil {
		t.Fatal("expected non-nil global telemetry")
	}
	
	if !tm.IsEnabled() {
		t.Errorf("expected enabled=true")
	}
	
	// Record some telemetry
	tm.RecordAttempt()
	tm.RecordSuccess()
	
	snap := tm.Snapshot()
	if snap.Attempts != 1 {
		t.Errorf("expected attempts=1, got %d", snap.Attempts)
	}
	if snap.Successes != 1 {
		t.Errorf("expected successes=1, got %d", snap.Successes)
	}
	if snap.MaxConcurrent != 3 {
		t.Errorf("expected maxConcurrent=3, got %d", snap.MaxConcurrent)
	}
}

func TestGlobalICMPTelemetryIdempotent(t *testing.T) {
	// Reset global telemetry for clean test
	globalICMPTelemetry = nil
	
	InitGlobalICMPTelemetry(true, 5)
	tm1 := GetGlobalICMPTelemetry()
	
	// Calling again should return same instance
	InitGlobalICMPTelemetry(false, 10)
	tm2 := GetGlobalICMPTelemetry()
	
	if tm1 != tm2 {
		t.Error("expected same telemetry instance on re-initialization")
	}
}

func TestICMPPingTelemetrySetEnabled(t *testing.T) {
	tm := NewICMPPingTelemetry(false, 1)
	
	if tm.IsEnabled() != false {
		t.Errorf("expected enabled=false")
	}
	
	tm.SetEnabled(true)
	
	if tm.IsEnabled() != true {
		t.Errorf("expected enabled=true after SetEnabled")
	}
}

// Test that atomic operations are properly atomic
func TestICMPPingTelemetryAtomicPrecision(t *testing.T) {
	tm := NewICMPPingTelemetry(true, 1)
	
	var count atomic.Uint64
	const goroutines = 100
	const increments = 1000
	
	var wg sync.WaitGroup
	wg.Add(goroutines)
	
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < increments; j++ {
				tm.RecordAttempt()
				count.Add(1)
			}
		}()
	}
	
	wg.Wait()
	
	expected := uint64(goroutines * increments)
	if got := tm.attempts.Load(); got != expected {
		t.Errorf("expected attempts=%d, got %d (atomic precision test)", expected, got)
	}
	if got := count.Load(); got != expected {
		t.Errorf("sanity check failed: expected count=%d, got %d", expected, got)
	}
}

func TestResetGlobalICMPTelemetryNoDeadlock(t *testing.T) {
	// Reset global telemetry for clean test
	globalICMPTelemetry = nil
	
	// Initialize global telemetry
	InitGlobalICMPTelemetry(true, 2)
	
	// Record some telemetry
	tm := GetGlobalICMPTelemetry()
	tm.RecordAttempt()
	tm.RecordSuccess()
	tm.RecordFailure("test error")
	
	// Reset should not deadlock and should clear counters
	ResetGlobalICMPTelemetry()
	
	// Verify counters are reset
	snap := tm.Snapshot()
	if snap.Attempts != 0 {
		t.Errorf("expected attempts=0 after reset, got %d", snap.Attempts)
	}
	if snap.Successes != 0 {
		t.Errorf("expected successes=0 after reset, got %d", snap.Successes)
	}
	if snap.Failures != 0 {
		t.Errorf("expected failures=0 after reset, got %d", snap.Failures)
	}
	if snap.LastError != "" {
		t.Errorf("expected lastError='' after reset, got %q", snap.LastError)
	}
}

func TestResetGlobalICMPTelemetryNilSafe(t *testing.T) {
	// Reset global telemetry for clean test
	globalICMPTelemetry = nil
	
	// Reset with nil global telemetry should not panic
	ResetGlobalICMPTelemetry()
	
	// Should not have initialized anything
	if GetGlobalICMPTelemetry() != nil {
		t.Error("expected nil global telemetry after reset with nil")
	}
}
