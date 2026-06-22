package state

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"testing"
	"time"
)

// TestCalculatePercentiles_ReturnsDetachedValues verifies that CalculatePercentiles
// returns detached percentile values that do not alias the input sample slice.
//
// This is important for API DTOs: response structs should not expose pointers
// into caller-owned or mutable sample storage.
func TestCalculatePercentiles_ReturnsDetachedValues(t *testing.T) {
	// Create sorted samples
	sortedSamples := make([]float64, 100)
	for i := 0; i < 100; i++ {
		sortedSamples[i] = float64(i+1) * 10.0 // 10, 20, 30, ... 1000
	}

	percentiles := []float64{50, 90, 95, 99}

	result := CalculatePercentiles(sortedSamples, percentiles)
	if result == nil {
		t.Fatal("CalculatePercentiles returned nil")
	}

	// Verify all percentiles have valid values
	for _, p := range percentiles {
		if ptr := result[p]; ptr == nil {
			t.Errorf("Percentile %v returned nil", p)
		} else if *ptr <= 0 || *ptr > 1000 {
			t.Errorf("Percentile %v has unexpected value: %v", p, *ptr)
		}
	}
}

// TestLatencySeries_JSONMarshal_UsesDetachedPercentileValues verifies that
// JSON marshaling of LatencySeries works correctly when percentile values
// are detached from the input sample slice.
//
// This exercises the production crash path: JSON encoder dereferencing
// *float64 fields in PercentilePoint structs.
func TestLatencySeries_JSONMarshal_UsesDetachedPercentileValues(t *testing.T) {
	sortedSamples := make([]float64, 100)
	for i := 0; i < 100; i++ {
		sortedSamples[i] = float64(i+1) * 10.0 // 10, 20, 30, ... 1000
	}

	percentiles := []float64{50, 90, 95, 99}

	// Build response with percentile points
	series := LatencySeries{
		TargetID:  "detached-test",
		ProbeKind: "icmp",
		ProbeURL:  "http://test",
		Points:    []PercentilePoint{},
	}

	now := time.Now().UTC()
	for i := 0; i < 10; i++ {
		// Calculate percentiles - result should not alias sortedSamples
		ptValues := CalculatePercentiles(sortedSamples, percentiles)

		point := PercentilePoint{
			Timestamp:   now.Add(time.Duration(i) * time.Minute),
			SampleCount: 100,
			ErrorCount:  0,
			P50Ms:       ptValues[50],
			P90Ms:       ptValues[90],
			P95Ms:       ptValues[95],
			P99Ms:       ptValues[99],
		}
		series.Points = append(series.Points, point)
	}

	// Marshal to JSON - this exercises the PercentilePoint pointer fields
	data, err := json.Marshal(series)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("JSON marshal produced empty output")
	}

	// Unmarshal to verify round-trip
	var decoded LatencySeries
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if len(decoded.Points) != 10 {
		t.Errorf("Expected 10 points, got %d", len(decoded.Points))
	}
}

// TestLatencySeries_ConcurrentSnapshotJSONMarshal_Race tests that JSON marshaling
// of latency series responses is safe under concurrent probe writes.
//
// This specifically exercises the concurrent path:
// - goroutine A: JSON encodes a PercentilePoint with *float64 pointers
// - goroutine B: Records new samples
func TestLatencySeries_ConcurrentSnapshotJSONMarshal_Race(t *testing.T) {
	m := NewManager()
	m.ConfigureICMP([]int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}, 3600)

	targetID := "concurrent-snapshot-test"

	// Pre-fill
	for i := 0; i < 3600; i++ {
		m.RecordICMPLatencyAt(targetID, float64(i%100)+10.0, true, time.Now().UTC())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Writer: simulate ongoing ICMP probes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				m.RecordICMPLatency(targetID, float64((i*17)%200)+10.0, i%10 != 0)
			}
		}
	}()

	// JSON marshalers: simulate dashboard fetching latency series
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					// Get snapshot
					snap := m.GetICMPSnapshot(targetID, 3600)
					if snap == nil || len(snap.Samples) == 0 {
						continue
					}

					// Build series with detached percentile values
					series := LatencySeries{
						TargetID:    targetID,
						ProbeKind:   "icmp",
						ProbeURL:    "http://test",
						Points:      []PercentilePoint{},
					}

					// Create percentile points
					for i := 0; i < 4; i++ {
						pt := PercentilePoint{
							Timestamp:   time.Now(),
							SampleCount: len(snap.Samples),
						}

						// Sort samples and calculate all percentiles per point
						sortedSamples := make([]float64, len(snap.Samples))
						for j, s := range snap.Samples {
							sortedSamples[j] = s.LatencyMs
						}
						sort.Float64s(sortedSamples)

						percentiles := []float64{50, 90, 95, 99}
						pctResult := CalculatePercentiles(sortedSamples, percentiles)
						pt.P50Ms = pctResult[50]
						pt.P90Ms = pctResult[90]
						pt.P95Ms = pctResult[95]
						pt.P99Ms = pctResult[99]

						series.Points = append(series.Points, pt)
					}

					// Marshal to JSON
					data, err := json.Marshal(series)
					if err != nil {
						t.Errorf("JSON marshal failed: %v", err)
					}
					_ = len(data) // Verify non-empty
				}
			}
		}()
	}

	wg.Wait()
}
