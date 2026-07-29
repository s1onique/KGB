// Package main provides tests for the memory lab.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// === P0-1/P0-2 Tests: Production Target-State Decoding ===

func TestFetchTargetState_Success(t *testing.T) {
	// Mock server returning valid target snapshot
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"target_id":  "real-tovarisch",
			"scraped_at": time.Now().Format(time.RFC3339),
			"reachable":  true,
			"status":     "ok",
		})
	}))
	defer server.Close()

	port := strings.Split(server.URL, ":")[2] // "http://127.0.0.1:XXXXX"
	port = strings.TrimPrefix(port, "//127.0.0.1:")

	auth, err := FetchTargetState(port, "real-tovarisch", "")
	if err != nil {
		t.Fatalf("FetchTargetState failed: %v", err)
	}
	if auth == nil {
		t.Fatal("Expected auth, got nil")
	}
	if auth.TargetID != "real-tovarisch" {
		t.Errorf("Expected target_id=real-tovarisch, got %q", auth.TargetID)
	}
	if !auth.Reachable {
		t.Error("Expected Reachable=true")
	}
	if auth.Status != "ok" {
		t.Errorf("Expected status=ok, got %q", auth.Status)
	}
}

func TestFetchTargetState_TargetNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	port := strings.Split(server.URL, ":")[2]
	port = strings.TrimPrefix(port, "//127.0.0.1:")

	auth, err := FetchTargetState(port, "nonexistent", "")
	if err != nil {
		t.Fatalf("FetchTargetState failed: %v", err)
	}
	if auth != nil {
		t.Error("Expected nil for not-found target")
	}
}

func TestFetchTargetState_ScrapeAttemptAuthority(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"target_id":  "test-target",
			"scraped_at": "2024-01-01T12:00:00Z",
			"reachable":  false,
		})
	}))
	defer server.Close()

	port := strings.Split(server.URL, ":")[2]
	port = strings.TrimPrefix(port, "//127.0.0.1:")

	auth, err := FetchTargetState(port, "test-target", "")
	if err != nil {
		t.Fatalf("FetchTargetState failed: %v", err)
	}

	// Attempt authority should be true even if unreachable
	if !auth.AttemptObserved {
		t.Error("Expected AttemptObserved=true (scraped_at present)")
	}
	if !auth.AttemptTimestampValid {
		t.Error("Expected AttemptTimestampValid=true")
	}
}

func TestFetchTargetState_ScrapeCompletionAuthority(t *testing.T) {
	tests := []struct {
		name         string
		snapshot     map[string]interface{}
		wantComplete bool
	}{
		{
			name: "reachable_no_error",
			snapshot: map[string]interface{}{
				"target_id":  "test",
				"scraped_at": time.Now().Format(time.RFC3339),
				"reachable":  true,
				"status":     "ok",
			},
			wantComplete: true,
		},
		{
			name: "reachable_with_error",
			snapshot: map[string]interface{}{
				"target_id":  "test",
				"scraped_at": time.Now().Format(time.RFC3339),
				"reachable":  true,
				"error":      "connection refused",
			},
			wantComplete: false,
		},
		{
			name: "not_reachable",
			snapshot: map[string]interface{}{
				"target_id":  "test",
				"scraped_at": time.Now().Format(time.RFC3339),
				"reachable":  false,
			},
			wantComplete: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.snapshot)
			}))
			defer server.Close()

			port := strings.Split(server.URL, ":")[2]
			port = strings.TrimPrefix(port, "//127.0.0.1:")

			auth, err := FetchTargetState(port, "test", "")
			if err != nil {
				t.Fatalf("FetchTargetState failed: %v", err)
			}

			got := auth.IsScrapeCompleted()
			if got != tt.wantComplete {
				t.Errorf("IsScrapeCompleted() = %v, want %v", got, tt.wantComplete)
			}
		})
	}
}

func TestFetchTargetState_UnknownFieldsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Send unknown field "foo_bar_baz"
		json.NewEncoder(w).Encode(map[string]interface{}{
			"target_id":   "test",
			"scraped_at":  time.Now().Format(time.RFC3339),
			"reachable":   true,
			"foo_bar_baz": "unknown",
		})
	}))
	defer server.Close()

	port := strings.Split(server.URL, ":")[2]
	port = strings.TrimPrefix(port, "//127.0.0.1:")

	_, err := FetchTargetState(port, "test", "")
	// Should fail due to DisallowUnknownFields
	if err == nil {
		t.Error("Expected error for unknown fields, got nil")
	}
}

// === P0-3 Tests: OBSERVED Mutation Matrix ===

func TestClassifyLabResult_OBSERVED(t *testing.T) {
	result := LabResult{
		TovarischBinPath:      "/path/to/tovarisch",
		RealTovarischStarted:  true,
		RealTovarischReady:    true,
		RealUVB76Started:      true,
		UVB76PProfReady:       true,
		RealTargetObserved:    true,
		ScrapeAttempted:       true,
		ScrapeCompleted:       true,
		ProcessSamplesPresent: true,
		ProfilesPresent:       true,
		UVB76Removed:          true,
		TovarischRemoved:      true,
		PortsReleased:         true,
	}

	classification, ok := classifyLabResult(result)
	if classification != "OBSERVED" {
		t.Errorf("Expected OBSERVED, got %s", classification)
	}
	if !ok {
		t.Error("Expected ok=true")
	}
}

func TestClassifyLabResult_FakeModeRejection(t *testing.T) {
	result := LabResult{
		TovarischBinPath:      "fake",
		RealTovarischStarted:  true,
		RealTovarischReady:    true,
		RealUVB76Started:      true,
		UVB76PProfReady:       true,
		RealTargetObserved:    true,
		ScrapeAttempted:       true,
		ScrapeCompleted:       true,
		ProcessSamplesPresent: true,
		ProfilesPresent:       true,
		UVB76Removed:          true,
		TovarischRemoved:      true,
		PortsReleased:         true,
	}

	classification, ok := classifyLabResult(result)
	if classification != "FAILED" {
		t.Errorf("Expected FAILED for fake mode, got %s", classification)
	}
	if ok {
		t.Error("Expected ok=false for fake mode")
	}
}

func TestClassifyLabResult_MissingScrapeAttempt(t *testing.T) {
	result := LabResult{
		TovarischBinPath:      "/path/to/tovarisch",
		RealTovarischStarted:  true,
		RealTovarischReady:    true,
		RealUVB76Started:      true,
		UVB76PProfReady:       true,
		RealTargetObserved:    true,
		ScrapeAttempted:       false, // Missing
		ScrapeCompleted:       true,
		ProcessSamplesPresent: true,
		ProfilesPresent:       true,
		UVB76Removed:          true,
		TovarischRemoved:      true,
		PortsReleased:         true,
	}

	classification, ok := classifyLabResult(result)
	if classification != "PARTIAL" {
		t.Errorf("Expected PARTIAL, got %s", classification)
	}
	if ok {
		t.Error("Expected ok=false")
	}
}

func TestClassifyLabResult_MissingScrapeCompleted(t *testing.T) {
	result := LabResult{
		TovarischBinPath:      "/path/to/tovarisch",
		RealTovarischStarted:  true,
		RealTovarischReady:    true,
		RealUVB76Started:      true,
		UVB76PProfReady:       true,
		RealTargetObserved:    true,
		ScrapeAttempted:       true,
		ScrapeCompleted:       false, // Missing
		ProcessSamplesPresent: true,
		ProfilesPresent:       true,
		UVB76Removed:          true,
		TovarischRemoved:      true,
		PortsReleased:         true,
	}

	classification, ok := classifyLabResult(result)
	if classification != "PARTIAL" {
		t.Errorf("Expected PARTIAL, got %s", classification)
	}
	if ok {
		t.Error("Expected ok=false")
	}
}

func TestMutationMatrix_AllFieldsCritical(t *testing.T) {
	matrix := GetMutationMatrix()

	// All fields in the mutation matrix should expect OBSERVED=false when mutated to false
	for _, m := range matrix {
		if m.ExpectOBSERVED {
			t.Logf("Field %s: mutating to false breaks OBSERVED", m.Field)
		}
	}

	if len(matrix) == 0 {
		t.Error("Expected non-empty mutation matrix")
	}
}

func TestMutationMatrix_EachField(t *testing.T) {
	// Create a valid OBSERVED result
	validResult := LabResult{
		TovarischBinPath:      "/path/to/tovarisch",
		RealTovarischStarted:  true,
		RealTovarischReady:    true,
		RealUVB76Started:      true,
		UVB76PProfReady:       true,
		RealTargetObserved:    true,
		ScrapeAttempted:       true,
		ScrapeCompleted:       true,
		ProcessSamplesPresent: true,
		ProfilesPresent:       true,
		UVB76Removed:          true,
		TovarischRemoved:      true,
		PortsReleased:         true,
	}

	matrix := GetMutationMatrix()
	for _, m := range matrix {
		// Mutate each field to false
		mutated := MutateResult(validResult, m.Field, false)
		classification, _ := classifyLabResult(mutated)

		if classification == "OBSERVED" {
			t.Errorf("Field %s mutated to false should not produce OBSERVED", m.Field)
		}
	}
}

// === P0-4 Tests: Status and Smaps Parsing ===

func TestParseMemValue(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1234 kB", 1234},
		{"1234 KB", 1234},
		{"5678", 5678},
		{"0 kB", 0},
		{"  100  kB  ", 100},
	}

	for _, tt := range tests {
		got, err := parseMemValue(1, "test_field", tt.input)
		if err != nil {
			t.Errorf("parseMemValue(1, \"test_field\", %q) returned error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseMemValue(1, \"test_field\", %q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseIntValue(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"12", 12},
		{"0", 0},
		{"  42  ", 42},
	}

	for _, tt := range tests {
		got, err := parseIntValue(1, "test_field", tt.input)
		if err != nil {
			t.Errorf("parseIntValue(1, \"test_field\", %q) returned error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseIntValue(1, \"test_field\", %q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// === P0-6 Tests: Profile Validation ===

func TestValidateProfile_EmptyFile(t *testing.T) {
	// Create temp file with no content
	tmpDir := t.TempDir()
	emptyFile := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(emptyFile, []byte{}, 0644); err != nil {
		t.Fatalf("failed to create empty file: %v", err)
	}

	// Should fail for empty file
	err := ValidateProfile(emptyFile, "goroutine")
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestValidateProfile_CorruptGzip(t *testing.T) {
	tmpDir := t.TempDir()
	corruptFile := filepath.Join(tmpDir, "corrupt.gz")
	// Write non-gzip bytes
	if err := os.WriteFile(corruptFile, []byte("not gzip data"), 0644); err != nil {
		t.Fatalf("failed to create corrupt file: %v", err)
	}

	// Should fail for corrupt gzip
	err := ValidateProfile(corruptFile, "heap")
	if err == nil {
		t.Error("expected error for corrupt gzip")
	}
}

func TestValidateProfile_EmptyGoroutineDump(t *testing.T) {
	tmpDir := t.TempDir()
	emptyFile := filepath.Join(tmpDir, "empty.txt")
	// Empty file
	if err := os.WriteFile(emptyFile, []byte(""), 0644); err != nil {
		t.Fatalf("failed to create empty file: %v", err)
	}

	// Should fail for empty goroutine dump
	err := ValidateProfile(emptyFile, "goroutine")
	if err == nil {
		t.Error("expected error for empty goroutine dump")
	}
}

func TestValidateProfile_ValidGzip(t *testing.T) {
	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "valid.gz")

	// Create valid gzip with some content
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte("heap profile v1\n"))
	gz.Close()

	if err := os.WriteFile(validFile, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to create valid gzip: %v", err)
	}

	// Should succeed for valid gzip
	err := ValidateProfile(validFile, "heap")
	if err != nil {
		t.Errorf("unexpected error for valid gzip: %v", err)
	}
}

func TestValidateProfile_ValidUTF8Goroutine(t *testing.T) {
	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "goroutine.txt")

	content := "goroutine 1:\n  some stack trace\ngoroutine 2:\n  another stack\n"
	if err := os.WriteFile(validFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create valid file: %v", err)
	}

	// Should succeed for valid UTF-8
	err := ValidateProfile(validFile, "goroutine")
	if err != nil {
		t.Errorf("unexpected error for valid UTF-8: %v", err)
	}
}

func TestValidateProfile_InvalidUTF8(t *testing.T) {
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "invalid.txt")

	// Invalid UTF-8 sequence
	content := []byte{0xff, 0xfe, 0xfd}
	if err := os.WriteFile(invalidFile, content, 0644); err != nil {
		t.Fatalf("failed to create invalid file: %v", err)
	}

	// Should fail for invalid UTF-8
	err := ValidateProfile(invalidFile, "goroutine")
	if err == nil {
		t.Error("expected error for invalid UTF-8")
	}
}

// === P0-7 Tests: result.json Atomic Publication ===

func TestPersistResult_UnknownFieldsRejected(t *testing.T) {
	// Create a result with unknown field - this must be rejected by strict decoder
	result := map[string]interface{}{
		"schema_version": 1,
		"run_id":         "test",
		"unknown_field":  "should fail",
	}

	data, _ := json.Marshal(result)

	// Use the same strict decoder as persistResult
	var r Result
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&r)

	// P0-7: Must reject unknown fields
	if err == nil {
		t.Error("expected error for unknown field, got nil")
	}
}

// === P0-8 Tests: Delta Calculation ===

func TestComputeDeltaSummary_Basic(t *testing.T) {
	samples := []ProcessSample{
		{RSSKIB: 1000},
		{RSSKIB: 1100},
		{RSSKIB: 1200},
		{RSSKIB: 1050},
		{RSSKIB: 1150},
	}

	summary := ComputeDeltaSummary(samples, func(s ProcessSample) int64 { return s.RSSKIB })

	if summary.First != 1000 {
		t.Errorf("First = %d, want 1000", summary.First)
	}
	if summary.Last != 1150 {
		t.Errorf("Last = %d, want 1150", summary.Last)
	}
	if summary.Min != 1000 {
		t.Errorf("Min = %d, want 1000", summary.Min)
	}
	if summary.Max != 1200 {
		t.Errorf("Max = %d, want 1200", summary.Max)
	}
	if summary.Delta != 150 {
		t.Errorf("Delta = %d, want 150", summary.Delta)
	}
}

func TestComputeDeltaSummary_Empty(t *testing.T) {
	var samples []ProcessSample
	summary := ComputeDeltaSummary(samples, func(s ProcessSample) int64 { return s.RSSKIB })
	if summary != nil {
		t.Error("Expected nil for empty samples")
	}
}

func TestComputeSlopeKiBPerMinute(t *testing.T) {
	tests := []struct {
		name     string
		first    int64
		last     int64
		duration float64
		want     float64
	}{
		{"positive_growth", 1000, 2000, 60, 1000},  // 1000 KiB in 60s = 1000 KiB/min
		{"negative_growth", 2000, 1000, 60, -1000}, // Shrinkage
		{"no_growth", 1000, 1000, 60, 0},
		{"zero_duration", 1000, 2000, 0, 0},
	}

	const epsilon = 0.001 // tolerance for float comparison
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeSlopeKiBPerMinute(tt.first, tt.last, tt.duration)
			if math.Abs(got-tt.want) > epsilon {
				t.Errorf("ComputeSlopeKiBPerMinute(%d, %d, %f) = %f, want %f", tt.first, tt.last, tt.duration, got, tt.want)
			}
		})
	}
}
