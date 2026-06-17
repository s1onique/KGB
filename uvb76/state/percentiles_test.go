package state

import "testing"

// Percentile Calculation Tests

func TestCalculatePercentiles_Empty(t *testing.T) {
	result := CalculatePercentiles([]float64{}, []float64{50, 90, 95, 99})
	
	if result[50] != nil {
		t.Error("Expected nil for p50 with empty input")
	}
	if result[99] != nil {
		t.Error("Expected nil for p99 with empty input")
	}
}

func TestCalculatePercentiles_SingleValue(t *testing.T) {
	result := CalculatePercentiles([]float64{100.0}, []float64{50, 90, 95, 99})
	
	if result[50] == nil || *result[50] != 100.0 {
		t.Errorf("Expected p50=100.0, got %v", result[50])
	}
	if result[90] == nil || *result[90] != 100.0 {
		t.Errorf("Expected p90=100.0, got %v", result[90])
	}
}

func TestCalculatePercentiles_TwoValues(t *testing.T) {
	// Sorted: [10, 100]
	result := CalculatePercentiles([]float64{10.0, 100.0}, []float64{50, 90, 99})
	
	// p50: rank = 0.5 * 1 + 1 = 1.5 -> k=1, d=0.5 -> 10 + 0.5*(100-10) = 55
	if result[50] == nil {
		t.Error("Expected p50 value, got nil")
	} else if *result[50] < 54.9 || *result[50] > 55.1 {
		t.Errorf("Expected p50 around 55.0, got %f", *result[50])
	}
	
	// p90: rank = 0.9 * 1 + 1 = 1.9 -> k=1, d=0.9 -> 10 + 0.9*(100-10) = 91
	if result[90] == nil {
		t.Error("Expected p90 value, got nil")
	} else if *result[90] < 90.9 || *result[90] > 91.1 {
		t.Errorf("Expected p90 around 91.0, got %f", *result[90])
	}
}

func TestCalculatePercentiles_10Values(t *testing.T) {
	// Sorted: [10, 20, 30, 40, 50, 60, 70, 80, 90, 100]
	samples := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	result := CalculatePercentiles(samples, []float64{50, 90, 95, 99})
	
	// p50: rank = 0.5 * 9 + 1 = 5.5 -> k=5, d=0.5 -> samples[4] + 0.5*(samples[5]-samples[4]) = 50 + 0.5*10 = 55
	if result[50] == nil {
		t.Error("Expected p50 value, got nil")
	} else if *result[50] < 54.9 || *result[50] > 55.1 {
		t.Errorf("Expected p50 around 55.0, got %f", *result[50])
	}
	
	// p90: rank = 0.9 * 9 + 1 = 9.1 -> k=9, d=0.1 -> samples[8] + 0.1*(samples[9]-samples[8]) = 90 + 0.1*10 = 91
	if result[90] == nil {
		t.Error("Expected p90 value, got nil")
	} else if *result[90] < 90.9 || *result[90] > 91.1 {
		t.Errorf("Expected p90 around 91.0, got %f", *result[90])
	}
	
	// p95: rank = 0.95 * 9 + 1 = 9.55 -> k=9, d=0.55 -> samples[8] + 0.55*(samples[9]-samples[8]) = 90 + 0.55*10 = 95.5
	if result[95] == nil {
		t.Error("Expected p95 value, got nil")
	} else if *result[95] < 95.4 || *result[95] > 95.6 {
		t.Errorf("Expected p95 around 95.5, got %f", *result[95])
	}
	
	// p99: rank = 0.99 * 9 + 1 = 9.91 -> k=9, d=0.91 -> samples[8] + 0.91*(samples[9]-samples[8]) = 90 + 0.91*10 = 99.1
	if result[99] == nil {
		t.Error("Expected p99 value, got nil")
	} else if *result[99] < 99.0 || *result[99] > 99.2 {
		t.Errorf("Expected p99 around 99.1, got %f", *result[99])
	}
}

func TestCalculatePercentiles_Deterministic(t *testing.T) {
	// Run calculation multiple times with same input, expect same output
	samples := []float64{5, 15, 25, 35, 45, 55, 65, 75, 85, 95}
	percentiles := []float64{50, 90, 95, 99}
	var prevP50, prevP90 float64
	
	for i := 0; i < 10; i++ {
		result := CalculatePercentiles(samples, percentiles)
		
		// Verify values are in expected ranges
		if result[50] == nil || *result[50] < 49 || *result[50] > 51 {
			t.Errorf("Run %d: Expected p50 around 50, got %v", i, result[50])
		}
		if result[90] == nil || *result[90] < 85 || *result[90] > 87 {
			t.Errorf("Run %d: Expected p90 around 86, got %v", i, result[90])
		}
		
		// Verify determinism - same input always produces same output
		if i > 0 {
			if *result[50] != prevP50 {
				t.Errorf("Run %d: p50 not deterministic (%f vs %f)", i, *result[50], prevP50)
			}
			if *result[90] != prevP90 {
				t.Errorf("Run %d: p90 not deterministic (%f vs %f)", i, *result[90], prevP90)
			}
		}
		prevP50 = *result[50]
		prevP90 = *result[90]
	}
}

// Latency Summary Percentile Tests

func TestLatencyTracker_GetSummary_WithPercentiles(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100, 250, 500}
	lt := NewLatencyTracker(buckets, 100)
	
	// Record 10 successful samples with known values
	lt.Record(10.0, true)
	lt.Record(20.0, true)
	lt.Record(30.0, true)
	lt.Record(40.0, true)
	lt.Record(50.0, true)
	lt.Record(60.0, true)
	lt.Record(70.0, true)
	lt.Record(80.0, true)
	lt.Record(90.0, true)
	lt.Record(100.0, true)
	
	summary := lt.GetSummary("percentile-test")
	
	if summary.SampleCount != 10 {
		t.Errorf("Expected sample count 10, got %d", summary.SampleCount)
	}
	if summary.ErrorCount != 0 {
		t.Errorf("Expected error count 0, got %d", summary.ErrorCount)
	}
	if summary.P50LatencyMs == nil {
		t.Error("Expected P50 value, got nil")
	} else if *summary.P50LatencyMs < 50.0 || *summary.P50LatencyMs > 60.0 {
		t.Errorf("Expected p50 around 55.0, got %f", *summary.P50LatencyMs)
	}
	if summary.P90LatencyMs == nil {
		t.Error("Expected P90 value, got nil")
	} else if *summary.P90LatencyMs < 90.0 || *summary.P90LatencyMs > 95.0 {
		t.Errorf("Expected p90 around 91.0, got %f", *summary.P90LatencyMs)
	}
}

func TestLatencyTracker_GetSummary_FailedProbesExcluded(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100, 250, 500}
	lt := NewLatencyTracker(buckets, 100)
	
	// Mix of successful and failed probes
	lt.Record(10.0, true)
	lt.Record(20.0, false)  // Failed
	lt.Record(30.0, true)
	lt.Record(40.0, false)  // Failed
	lt.Record(50.0, true)
	
	summary := lt.GetSummary("failed-test")
	
	// Total sample count includes failed
	if summary.SampleCount != 5 {
		t.Errorf("Expected sample count 5, got %d", summary.SampleCount)
	}
	// Error count tracks failed
	if summary.ErrorCount != 2 {
		t.Errorf("Expected error count 2, got %d", summary.ErrorCount)
	}
	// Percentiles should be from successful probes only (10, 30, 50)
	if summary.P50LatencyMs == nil {
		t.Error("Expected P50 value, got nil")
	} else {
		// For 3 samples, p50 = 30.0
		if *summary.P50LatencyMs != 30.0 {
			t.Errorf("Expected p50=30.0, got %f", *summary.P50LatencyMs)
		}
	}
}

func TestLatencyTracker_GetSummary_AllFailed(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)
	
	lt.Record(100.0, false)
	lt.Record(200.0, false)
	lt.Record(300.0, false)
	
	summary := lt.GetSummary("all-failed-test")
	
	if summary.SampleCount != 3 {
		t.Errorf("Expected sample count 3, got %d", summary.SampleCount)
	}
	if summary.ErrorCount != 3 {
		t.Errorf("Expected error count 3, got %d", summary.ErrorCount)
	}
	// Percentiles should be nil when all probes failed
	if summary.P50LatencyMs != nil {
		t.Errorf("Expected nil p50 for all-failed, got %f", *summary.P50LatencyMs)
	}
	if summary.P90LatencyMs != nil {
		t.Errorf("Expected nil p90 for all-failed, got %f", *summary.P90LatencyMs)
	}
}

func TestLatencyTracker_ErrorCountBounded(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 5) // Only 5 slots
	
	// Record more samples than buffer can hold, with alternating success/fail
	lt.Record(10.0, false) // error
	lt.Record(20.0, true)
	lt.Record(30.0, false) // error
	lt.Record(40.0, true)
	lt.Record(50.0, false) // error
	lt.Record(60.0, true)  // Overwrites first sample
	lt.Record(70.0, false) // Overwrites second sample (true)
	lt.Record(80.0, true)
	lt.Record(90.0, false)
	lt.Record(100.0, true)
	
	summary := lt.GetSummary("bounded-errors")
	
	// Only 5 samples in buffer
	if summary.SampleCount != 5 {
		t.Errorf("Expected sample count 5 (bounded), got %d", summary.SampleCount)
	}
}

// TestLatencyTracker_FailedSampleOverwriteRegression tests the fix for error count
// corruption when a failed probe overwrites another failed probe in the ring buffer.
// Before the fix: errorCount would go negative because we decremented when
// overwriting a failed sample with a successful one (since the old code always
// decremented errorCount when overwriting, but only incremented when the new
// sample was a failure).
func TestLatencyTracker_FailedSampleOverwriteRegression(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 3) // Only 3 slots
	
	// Record 3 failed samples (fills buffer)
	lt.Record(10.0, false) // error, count=1, errorCount=1
	lt.Record(20.0, false) // error, count=2, errorCount=2
	lt.Record(30.0, false) // error, count=3, errorCount=3
	
	summary := lt.GetSummary("all-failed")
	if summary.SampleCount != 3 {
		t.Errorf("Expected sample count 3, got %d", summary.SampleCount)
	}
	if summary.ErrorCount != 3 {
		t.Errorf("Expected error count 3, got %d", summary.ErrorCount)
	}
	
	// Now overwrite with another failed sample
	// Before fix: errorCount would decrement (old was failed) then increment (new is failed)
	// This would leave errorCount at 2 instead of staying at 3
	lt.Record(40.0, false) // Overwrites slot 0, should stay at errorCount=3
	
	summary = lt.GetSummary("failed-overwrite")
	if summary.SampleCount != 3 {
		t.Errorf("Expected sample count 3, got %d", summary.SampleCount)
	}
	if summary.ErrorCount != 3 {
		t.Errorf("Expected error count 3 (failed overwrite should not change count), got %d", summary.ErrorCount)
	}
	
	// Overwrite with successful sample (this should decrement)
	lt.Record(50.0, true) // Overwrites slot 1 (was error), errorCount should go to 2
	
	summary = lt.GetSummary("mixed")
	if summary.SampleCount != 3 {
		t.Errorf("Expected sample count 3, got %d", summary.SampleCount)
	}
	if summary.ErrorCount != 2 {
		t.Errorf("Expected error count 2 (one failed replaced with success), got %d", summary.ErrorCount)
	}
	
	// All percentiles should be nil since 1 success in the window
	// but we have mixed results now
	if summary.P50LatencyMs == nil {
		t.Error("Expected P50 value for mixed samples")
	}
}
