package state

import (
	"testing"
)

// State Manager Latency Tests

func TestManager_RecordLatency(t *testing.T) {
	m := NewManager()

	m.RecordLatency("target-1", 10.0, true)
	m.RecordLatency("target-1", 20.0, true)
	m.RecordLatency("target-2", 30.0, true)

	summary1 := m.GetLatencySummary("target-1")
	summary2 := m.GetLatencySummary("target-2")

	if summary1.SampleCount != 2 {
		t.Errorf("Expected target-1 count 2, got %d", summary1.SampleCount)
	}
	if summary2.SampleCount != 1 {
		t.Errorf("Expected target-2 count 1, got %d", summary2.SampleCount)
	}
}

func TestManager_GetRecentLatencySamples(t *testing.T) {
	m := NewManager()

	m.RecordLatency("target-1", 10.0, true)
	m.RecordLatency("target-1", 20.0, true)
	m.RecordLatency("target-1", 30.0, true)

	samples := m.GetRecentLatencySamples("target-1", 2)
	if len(samples) != 2 {
		t.Errorf("Expected 2 samples, got %d", len(samples))
	}
}

func TestManager_GetRecentLatencySamples_Empty(t *testing.T) {
	m := NewManager()

	samples := m.GetRecentLatencySamples("nonexistent", 10)
	if len(samples) != 0 {
		t.Errorf("Expected 0 samples for nonexistent target, got %d", len(samples))
	}
}

func TestManager_GetAllLatencySummaries(t *testing.T) {
	m := NewManager()

	m.RecordLatency("target-1", 10.0, true)
	m.RecordLatency("target-2", 20.0, true)

	summaries := m.GetAllLatencySummaries()
	if len(summaries) != 2 {
		t.Errorf("Expected 2 summaries, got %d", len(summaries))
	}
}

func TestManager_GetLatencyBuckets(t *testing.T) {
	buckets := []int64{5, 10, 25, 50}
	m := NewManagerWithConfig(buckets, 50)

	retrieved := m.GetLatencyBuckets()
	if len(retrieved) != len(buckets) {
		t.Errorf("Expected %d buckets, got %d", len(buckets), len(retrieved))
	}
}

func TestManager_NewManagerWithConfig(t *testing.T) {
	buckets := []int64{1, 2, 3, 4, 5}
	maxSamples := 50
	m := NewManagerWithConfig(buckets, maxSamples)

	if m.maxSamples != maxSamples {
		t.Errorf("Expected maxSamples %d, got %d", maxSamples, m.maxSamples)
	}
	if len(m.buckets) != len(buckets) {
		t.Errorf("Expected %d buckets, got %d", len(buckets), len(m.buckets))
	}
}

func TestManager_GetMaxSamples(t *testing.T) {
	m := NewManager()
	if m.GetMaxSamples() != 100 {
		t.Errorf("Expected maxSamples 100, got %d", m.GetMaxSamples())
	}

	m2 := NewManagerWithConfig([]int64{1, 2, 3}, 50)
	if m2.GetMaxSamples() != 50 {
		t.Errorf("Expected maxSamples 50, got %d", m2.GetMaxSamples())
	}
}
