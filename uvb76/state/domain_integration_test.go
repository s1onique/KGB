package state

import (
	"math"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// float64FromNaN returns a NaN float64 value for testing.
func float64FromNaN() float64 {
	return math.NaN()
}

func TestLatencySampleToDomainSample(t *testing.T) {
	now := time.Now().UTC()

	t.Run("successful sample", func(t *testing.T) {
		stateSample := LatencySample{
			Timestamp: now,
			LatencyMs: 100.5,
			Reachable: true,
		}

		domainSample := LatencySampleToDomainSample(stateSample)

		if !domainSample.OK {
			t.Error("expected OK=true for reachable sample")
		}
		if domainSample.At != now {
			t.Errorf("expected At=%v, got %v", now, domainSample.At)
		}
		if got := domainSample.Latency.Float64(); got != 100.5 {
			t.Errorf("expected Latency=100.5, got %v", got)
		}
		if domainSample.Err != "" {
			t.Errorf("expected empty Err, got %q", domainSample.Err)
		}
	})

	t.Run("failed sample", func(t *testing.T) {
		stateSample := LatencySample{
			Timestamp: now,
			LatencyMs: 0,
			Reachable: false,
			Error:     "connection refused",
		}

		domainSample := LatencySampleToDomainSample(stateSample)

		if domainSample.OK {
			t.Error("expected OK=false for unreachable sample")
		}
		if domainSample.Err != "connection refused" {
			t.Errorf("expected Err='connection refused', got %q", domainSample.Err)
		}
	})

	t.Run("NaN latency treated as failed", func(t *testing.T) {
		stateSample := LatencySample{
			Timestamp: now,
			LatencyMs: float64FromNaN(),
			Reachable: true,
		}

		domainSample := LatencySampleToDomainSample(stateSample)

		if domainSample.OK {
			t.Error("expected OK=false for NaN latency")
		}
	})
}

func TestLatencyTracker_GetSampleWindow(t *testing.T) {
	tracker := NewLatencyTracker([]int64{10, 50, 100, 500}, 100)

	t.Run("empty tracker returns empty window", func(t *testing.T) {
		window := tracker.GetSampleWindow(10)
		if window.Len() != 0 {
			t.Errorf("expected empty window, got len=%d", window.Len())
		}
	})

	t.Run("successful samples included", func(t *testing.T) {
		tracker.Record(100.0, true)
		tracker.Record(200.0, true)
		tracker.Record(300.0, true)

		window := tracker.GetSampleWindow(10)

		if window.Len() != 3 {
			t.Errorf("expected 3 samples, got %d", window.Len())
		}

		median, ok := window.Median()
		if !ok {
			t.Error("expected Median to succeed")
		}
		// Median of 100, 200, 300 is 200
		if got := median.Float64(); got != 200.0 {
			t.Errorf("expected median=200, got %v", got)
		}
	})

	t.Run("failed samples excluded from window", func(t *testing.T) {
		// Fresh tracker
		tracker2 := NewLatencyTracker([]int64{10, 50, 100, 500}, 100)
		tracker2.Record(100.0, true)
		tracker2.Record(0, false)   // Failed
		tracker2.Record(300.0, true)

		window := tracker2.GetSampleWindow(10)

		// Only 2 successful samples
		if window.Len() != 2 {
			t.Errorf("expected 2 successful samples, got %d", window.Len())
		}
	})
}

func TestManager_GetHTTPSampleWindow(t *testing.T) {
	m := NewManager()

	t.Run("no tracker returns empty window", func(t *testing.T) {
		window := m.GetHTTPSampleWindow("unknown-target", 10)
		if window.Len() != 0 {
			t.Errorf("expected empty window, got len=%d", window.Len())
		}
	})

	t.Run("samples converted correctly", func(t *testing.T) {
		m.RecordLatency("target1", 100.0, true)
		m.RecordLatency("target1", 200.0, true)

		window := m.GetHTTPSampleWindow("target1", 10)

		if window.Len() != 2 {
			t.Errorf("expected 2 samples, got %d", window.Len())
		}
	})
}

func TestManager_GetICMPSampleWindow(t *testing.T) {
	m := NewManager()
	// Configure ICMP buckets for the manager (required before recording ICMP samples)
	m.ConfigureICMP([]int64{5, 10, 25, 50, 100, 250, 500, 1000}, 100)

	t.Run("no tracker returns empty window", func(t *testing.T) {
		window := m.GetICMPSampleWindow("unknown-target", 10)
		if window.Len() != 0 {
			t.Errorf("expected empty window, got len=%d", window.Len())
		}
	})

	t.Run("samples converted correctly", func(t *testing.T) {
		m.RecordICMPLatency("target1", 50.0, true)
		m.RecordICMPLatency("target1", 75.0, true)

		window := m.GetICMPSampleWindow("target1", 10)

		if window.Len() != 2 {
			t.Errorf("expected 2 samples, got %d", window.Len())
		}
	})
}

func TestNewDiagnosticCaptureConfigFromDefaults(t *testing.T) {
	cfg := NewDiagnosticCaptureConfigFromDefaults(true, true, 5*time.Minute)

	if !cfg.Enabled {
		t.Error("expected Enabled=true")
	}
	if !cfg.Configured {
		t.Error("expected Configured=true")
	}
	if cfg.Cooldown != 5*time.Minute {
		t.Errorf("expected Cooldown=5m, got %v", cfg.Cooldown)
	}
}

func TestDecideCapture_WithDomainConfig(t *testing.T) {
	// Integration test: using domain.DecideCapture with state config
	cfg := NewDiagnosticCaptureConfigFromDefaults(true, true, 5*time.Minute)

	now := time.Now()
	lastCapture := now.Add(-10 * time.Minute) // 10 minutes ago

	decision := domain.DecideCapture(now, lastCapture, cfg)

	// 10 minutes > 5 minute cooldown, should return Run
	if decision.Kind != domain.CaptureDecisionRun {
		t.Errorf("expected CaptureDecisionRun, got %v", decision.Kind)
	}
}
