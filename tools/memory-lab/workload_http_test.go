// workload_http_test.go — Tests for HTTP workload execution

package main

import (
	"testing"
)

func TestIsLeakSlopeWorkload(t *testing.T) {
	tests := []struct {
		wt   WorkloadType
		want bool
	}{
		// Leak-slope workloads
		{WorkloadTovarischLeakSlope, true},
		{WorkloadTovarischLeakSlopeNetDiag, true},
		{WorkloadUVB76LeakSlope, true},
		{WorkloadUVB76LeakSlopeNetDiag, true},
		// Non-leak-slope workloads
		{WorkloadTovarischIdle, false},
		{WorkloadTovarischStatusJSON, false},
		{WorkloadTovarischStatusJSONNetDiag, false},
		{WorkloadUVB76Idle, false},
		{WorkloadUVB76StatusAPIPolling, false},
		{WorkloadUVB76DiagnosticCaptureLoop, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.wt), func(t *testing.T) {
			got := isLeakSlopeWorkload(tt.wt)
			if got != tt.want {
				t.Errorf("isLeakSlopeWorkload(%q) = %v, want %v", tt.wt, got, tt.want)
			}
		})
	}
}

func TestCalculateSlopeKiBPerMin(t *testing.T) {
	tests := []struct {
		name           string
		firstRSS       float64
		lastRSS        float64
		durationSecs   float64
		expected       float64
	}{
		{"10 KiB growth over 60s", 1000, 1010, 60.0, 10.0},
		{"100 KiB growth over 120s", 1000, 1100, 120.0, 50.0},
		{"no growth", 1000, 1000, 60.0, 0.0},
		{"negative growth (shrink)", 1000, 900, 60.0, -100.0},
		{"zero duration", 1000, 1100, 0.0, 0.0},
		{"negative duration", 1000, 1100, -60.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateSlopeKiBPerMin(tt.firstRSS, tt.lastRSS, tt.durationSecs)
			if got != tt.expected {
				t.Errorf("calculateSlopeKiBPerMin(%f, %f, %f) = %f, want %f",
					tt.firstRSS, tt.lastRSS, tt.durationSecs, got, tt.expected)
			}
		})
	}
}

func TestCalculateLeakSlopeMetrics(t *testing.T) {
	samples := []MemorySnapshot{
		{RSSKiB: 8000, PSSKiB: 6200},
		{RSSKiB: 8050, PSSKiB: 6250},
		{RSSKiB: 8100, PSSKiB: 6300},
		{RSSKiB: 8150, PSSKiB: 6350},
		{RSSKiB: 8200, PSSKiB: 6400},
	}
	first := MemorySnapshot{RSSKiB: 8000, PSSKiB: 6200}
	last := MemorySnapshot{RSSKiB: 8150, PSSKiB: 6350}
	maxRSS := int64(8200)
	maxPSS := int64(6400)
	workload := HTTPWorkloadResult{
		Operations: 100,
		Errors:     2,
		DurationMs: 10000, // 10 seconds
	}

	metrics := calculateLeakSlopeMetrics(samples, first, last, maxRSS, maxPSS, workload)

	// Verify sample count
	if metrics.SampledPoints != 5 {
		t.Errorf("SampledPoints = %d, want 5", metrics.SampledPoints)
	}

	// Verify duration
	if metrics.DurationSeconds != 10.0 {
		t.Errorf("DurationSeconds = %f, want 10.0", metrics.DurationSeconds)
	}

	// Verify RSS values
	if metrics.RSSFirstKiB != 8000 {
		t.Errorf("RSSFirstKiB = %d, want 8000", metrics.RSSFirstKiB)
	}
	if metrics.RSSLastKiB != 8150 {
		t.Errorf("RSSLastKiB = %d, want 8150", metrics.RSSLastKiB)
	}
	if metrics.RSSMaxKiB != 8200 {
		t.Errorf("RSSMaxKiB = %d, want 8200", metrics.RSSMaxKiB)
	}

	// Verify growth
	if metrics.RSSGrowthKiB != 150 {
		t.Errorf("RSSGrowthKiB = %d, want 150", metrics.RSSGrowthKiB)
	}

	// Verify slope: 150 KiB over 10s = 150 * 60 / 10 = 900 KiB/min
	if metrics.RSSSlopeKiBPerMin != 900.0 {
		t.Errorf("RSSSlopeKiBPerMin = %f, want 900.0", metrics.RSSSlopeKiBPerMin)
	}

	// Verify request count
	if metrics.RequestCount != 100 {
		t.Errorf("RequestCount = %d, want 100", metrics.RequestCount)
	}
	if metrics.RequestErrors != 2 {
		t.Errorf("RequestErrors = %d, want 2", metrics.RequestErrors)
	}
}

func TestTovarischWorkloadURLs(t *testing.T) {
	urls := TovarischWorkloadURLs(18080)

	// Verify leak-slope URLs exist
	if _, ok := urls[WorkloadTovarischLeakSlope]; !ok {
		t.Error("WorkloadTovarischLeakSlope not found in URLs")
	}
	if _, ok := urls[WorkloadTovarischLeakSlopeNetDiag]; !ok {
		t.Error("WorkloadTovarischLeakSlopeNetDiag not found in URLs")
	}

	// Verify URLs are correct
	if urls[WorkloadTovarischLeakSlope] != "http://127.0.0.1:18080/status" {
		t.Errorf("LeakSlope URL = %q, want http://127.0.0.1:18080/status", urls[WorkloadTovarischLeakSlope])
	}
	if urls[WorkloadTovarischLeakSlopeNetDiag] != "http://127.0.0.1:18080/status.json?include=network_diag" {
		t.Errorf("LeakSlopeNetDiag URL = %q, want http://127.0.0.1:18080/status.json?include=network_diag", urls[WorkloadTovarischLeakSlopeNetDiag])
	}
}

func TestUVB76WorkloadURLs(t *testing.T) {
	urls := UVB76WorkloadURLs(18081)

	// Verify leak-slope URLs exist
	if _, ok := urls[WorkloadUVB76LeakSlope]; !ok {
		t.Error("WorkloadUVB76LeakSlope not found in URLs")
	}
	if _, ok := urls[WorkloadUVB76LeakSlopeNetDiag]; !ok {
		t.Error("WorkloadUVB76LeakSlopeNetDiag not found in URLs")
	}

	// Verify URLs are HTTPS
	if urls[WorkloadUVB76LeakSlope] != "https://127.0.0.1:18081/api/v1/status" {
		t.Errorf("LeakSlope URL = %q, want https://127.0.0.1:18081/api/v1/status", urls[WorkloadUVB76LeakSlope])
	}
	if urls[WorkloadUVB76LeakSlopeNetDiag] != "https://127.0.0.1:18081/api/v1/status?include=network_diag" {
		t.Errorf("LeakSlopeNetDiag URL = %q, want https://127.0.0.1:18081/api/v1/status?include=network_diag", urls[WorkloadUVB76LeakSlopeNetDiag])
	}
}

func TestValidWorkload(t *testing.T) {
	tests := []struct {
		service string
		wt      WorkloadType
		want    bool
	}{
		// Tovarisch leak-slope
		{"tovarisch", WorkloadTovarischLeakSlope, true},
		{"tovarisch", WorkloadTovarischLeakSlopeNetDiag, true},
		// UVB-76 leak-slope
		{"uvb76", WorkloadUVB76LeakSlope, true},
		{"uvb76", WorkloadUVB76LeakSlopeNetDiag, true},
		// Invalid
		{"tovarisch", WorkloadUVB76LeakSlope, false},
		{"uvb76", WorkloadTovarischLeakSlope, false},
	}

	for _, tt := range tests {
		t.Run(tt.service+"-"+string(tt.wt), func(t *testing.T) {
			got := validWorkload(tt.service, tt.wt)
			if got != tt.want {
				t.Errorf("validWorkload(%q, %q) = %v, want %v", tt.service, tt.wt, got, tt.want)
			}
		})
	}
}
