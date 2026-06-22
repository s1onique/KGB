package diag

import (
	"context"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/state"
)

// slowCommandRunner is a CommandRunner that blocks until the context is cancelled.
// Used for testing timeout enforcement.
type slowCommandRunner struct{}

func (r *slowCommandRunner) RunCommand(ctx context.Context, name string, args ...string) ssCommandResult {
	// Block until context is cancelled or timeout fires
	<-ctx.Done()
	return ssCommandResult{}
}

// TestCollectTcpQuality_CollectorTimeoutEnforced verifies that the collector's
// timeout is applied even when the parent context has a longer timeout.
// This is a regression test for the production-timeout blocker.
func TestCollectTcpQuality_CollectorTimeoutEnforced(t *testing.T) {
	collector := &TcpQualityCollector{
		timeout:        50 * time.Millisecond, // Collector timeout: 50ms
		maxStdoutBytes: 4096,
		maxStderrBytes: 512,
		runner:         &slowCommandRunner{}, // Blocks indefinitely until cancelled
	}

	// Parent context has a MUCH longer timeout (10 seconds)
	parentCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	result := collector.CollectTcpQuality(parentCtx, "http", "10.0.0.5")
	elapsed := time.Since(start)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// CRITICAL: Result should be a timeout error (not a successful collection)
	// The collector timeout (50ms) should have fired, NOT the parent timeout (10s)
	if result.ErrorKind != state.TcpQualityErrorTimeout {
		t.Errorf("expected timeout error, got '%s'", result.ErrorKind)
	}
	if result.Error != "ss command timed out" {
		t.Errorf("expected 'ss command timed out', got '%s'", result.Error)
	}

	// Verify the operation completed within the collector timeout (with some margin)
	// It should NOT have taken anywhere near 10 seconds
	if elapsed >= 500*time.Millisecond {
		t.Errorf("collector took %v, expected < 500ms (collector timeout + margin)", elapsed)
	}
	if elapsed < 40*time.Millisecond {
		t.Errorf("collector took %v, expected >= 40ms (collector timeout)", elapsed)
	}
}
