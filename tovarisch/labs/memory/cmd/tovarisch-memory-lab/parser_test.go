// parser_test.go — Unit tests for strict CSV parser
package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling"
)

// Full production CSV header matching sampling.CSVHeaders() (41 columns)
const fullProductionHeader = "sequence,timestamp,process_pid,process_start_time,phase,delayed,rss_kib,pss_kib,pss_anon_kib,private_dirty_kib,anonymous_kib,swap_kib,docker_memory_usage_bytes,docker_memory_limit_bytes,has_docker_memory,vma_count,fd_count,socket_fd_count,thread_count,pid_count,cgroup_anon_bytes,cgroup_current_bytes,cgroup_memory_stat_anon,has_cgroup,has_cgroup_anon,oom_events,oom_kill_events,bgp_state,bgp_fsm_ticks,reconnect_count,has_rss,has_pss,has_pss_anon,has_private_dirty,has_anonymous,has_swap,has_thread_count,has_pid_count,has_fd_count,has_socket_fd_count,has_vma_count"

// Full data row with 41 fields matching the header exactly.
// Fields: sequence, timestamp, process_pid, process_start_time, phase, delayed,
// rss_kib, pss_kib, pss_anon_kib, private_dirty_kib, anonymous_kib, swap_kib,
// docker_memory_usage_bytes, docker_memory_limit_bytes, has_docker_memory,
// vma_count, fd_count, socket_fd_count, thread_count, pid_count,
// cgroup_anon_bytes, cgroup_current_bytes, cgroup_memory_stat_anon,
// has_cgroup, has_cgroup_anon, oom_events, oom_kill_events, bgp_state,
// bgp_fsm_ticks, reconnect_count, has_rss, has_pss, has_pss_anon,
// has_private_dirty, has_anonymous, has_swap, has_thread_count, has_pid_count,
// has_fd_count, has_socket_fd_count, has_vma_count
// Note: has_swap=false at position 35, all memory/availability values consistent
const fullDataRow = `0,2024-01-01T00:00:00Z,1234,1000000,baseline,false,10240,8192,4096,2048,1024,0,10485760,134217728,true,50,10,2,5,1,10485760,10485760,5242880,true,true,0,0,"",0,0,true,true,true,true,true,false,true,true,true,true,true`

func makeInput(header, data string) string {
	return header + "\n" + data
}

// Helper to mutate a specific field by index
func mutateField(data string, fieldIndex int, newValue string) string {
	parts := strings.Split(data, ",")
	if fieldIndex < len(parts) {
		parts[fieldIndex] = newValue
	}
	return strings.Join(parts, ",")
}

func TestParseSamplesCSVStream_Valid(t *testing.T) {
	input := makeInput(fullProductionHeader, fullDataRow)

	samples, err := ParseSamplesCSVStream(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	if samples[0].Sequence != 0 {
		t.Errorf("expected sequence 0, got %d", samples[0].Sequence)
	}
	if samples[0].PID != 1234 {
		t.Errorf("expected PID 1234, got %d", samples[0].PID)
	}
	if samples[0].ProcessStartTime != 1000000 {
		t.Errorf("expected start time 1000000, got %d", samples[0].ProcessStartTime)
	}
	if samples[0].Phase != sampling.PhaseBaseline {
		t.Errorf("expected phase baseline, got %s", samples[0].Phase)
	}
	if samples[0].Delayed {
		t.Error("expected Delayed to be false")
	}
}

func TestParseSamplesCSVStream_Empty(t *testing.T) {
	_, err := ParseSamplesCSVStream(strings.NewReader(""))
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestParseSamplesCSVStream_HeaderOnly(t *testing.T) {
	_, err := ParseSamplesCSVStream(strings.NewReader(fullProductionHeader))
	if err == nil {
		t.Error("expected error for header-only CSV")
	}
}

func TestParseSamplesCSVStream_MissingRequiredColumn(t *testing.T) {
	// Create header missing cgroup_memory_stat_anon (column 22)
	header := "sequence,timestamp,process_pid,process_start_time,phase,delayed,rss_kib,pss_kib,pss_anon_kib,private_dirty_kib,anonymous_kib,swap_kib,docker_memory_usage_bytes,docker_memory_limit_bytes,has_docker_memory,vma_count,fd_count,socket_fd_count,thread_count,pid_count,cgroup_anon_bytes,cgroup_current_bytes,has_cgroup,has_cgroup_anon,oom_events,oom_kill_events,bgp_state,bgp_fsm_ticks,reconnect_count,has_rss,has_pss,has_pss_anon,has_private_dirty,has_anonymous,has_swap,has_thread_count,has_pid_count,has_fd_count,has_socket_fd_count,has_vma_count"
	// 41 fields in header (missing cgroup_memory_stat_anon)
	data := `0,2024-01-01T00:00:00Z,1234,1000000,baseline,false,10240,8192,4096,2048,1024,0,10485760,134217728,true,50,10,2,5,1,10485760,10485760,true,true,0,0,"",0,0,true,true,true,true,true,false,true,true,true,true,true`
	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(header, data)))
	if err == nil {
		t.Error("expected error for missing required column")
	}
}

func TestParseSamplesCSVStream_DuplicateColumn(t *testing.T) {
	// Header with duplicate 'sequence' column
	header := "sequence,timestamp,sequence,process_start_time,phase,delayed,rss_kib,pss_kib,pss_anon_kib,private_dirty_kib,anonymous_kib,swap_kib,docker_memory_usage_bytes,docker_memory_limit_bytes,has_docker_memory,vma_count,fd_count,socket_fd_count,thread_count,pid_count,cgroup_anon_bytes,cgroup_current_bytes,cgroup_memory_stat_anon,has_cgroup,has_cgroup_anon,oom_events,oom_kill_events,bgp_state,bgp_fsm_ticks,reconnect_count,has_rss,has_pss,has_pss_anon,has_private_dirty,has_anonymous,has_swap,has_thread_count,has_pid_count,has_fd_count,has_socket_fd_count,has_vma_count"
	data := fullDataRow
	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(header, data)))
	if err == nil {
		t.Error("expected error for duplicate column")
	}
}

func TestParseSamplesCSVStream_EmptyColumnName(t *testing.T) {
	header := "sequence,,process_start_time,phase,delayed,rss_kib,pss_kib,pss_anon_kib,private_dirty_kib,anonymous_kib,swap_kib,docker_memory_usage_bytes,docker_memory_limit_bytes,has_docker_memory,vma_count,fd_count,socket_fd_count,thread_count,pid_count,cgroup_anon_bytes,cgroup_current_bytes,cgroup_memory_stat_anon,has_cgroup,has_cgroup_anon,oom_events,oom_kill_events,bgp_state,bgp_fsm_ticks,reconnect_count,has_rss,has_pss,has_pss_anon,has_private_dirty,has_anonymous,has_swap,has_thread_count,has_pid_count,has_fd_count,has_socket_fd_count,has_vma_count"
	data := fullDataRow
	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(header, data)))
	if err == nil {
		t.Error("expected error for empty column name")
	}
}

func TestParseSamplesCSVStream_InvalidPID(t *testing.T) {
	// Replace PID with -1 in data row
	data := mutateField(fullDataRow, 2, "-1")
	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(fullProductionHeader, data)))
	if err == nil {
		t.Error("expected error for negative PID")
	}
}

func TestParseSamplesCSVStream_InvalidPhase(t *testing.T) {
	// Replace phase with invalid value
	data := mutateField(fullDataRow, 4, "invalid_phase")
	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(fullProductionHeader, data)))
	if err == nil {
		t.Error("expected error for invalid phase")
	}
}

func TestParseSamplesCSVStream_InvalidStartTime(t *testing.T) {
	// Replace start time with 0
	data := mutateField(fullDataRow, 3, "0")
	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(fullProductionHeader, data)))
	if err == nil {
		t.Error("expected error for zero start time")
	}
}

func TestParseSamplesCSVStream_SequenceGap(t *testing.T) {
	// First row sequence=0, second row sequence=2 (gap)
	data1 := fullDataRow
	// Modify the second data row to have sequence=2
	data2 := mutateField(mutateField(fullDataRow, 0, "2"), 1, "2024-01-02T00:00:00Z")
	input := fullProductionHeader + "\n" + data1 + "\n" + data2
	_, err := ParseSamplesCSVStream(strings.NewReader(input))
	if err == nil {
		t.Error("expected error for sequence gap")
	}
}

func TestParseSamplesCSVStream_TimestampNotIncreasing(t *testing.T) {
	// First row timestamp > second row timestamp
	data1 := mutateField(fullDataRow, 1, "2024-01-01T00:00:01Z")
	data2 := mutateField(fullDataRow, 1, "2024-01-01T00:00:00Z")
	input := fullProductionHeader + "\n" + data1 + "\n" + data2
	_, err := ParseSamplesCSVStream(strings.NewReader(input))
	if err == nil {
		t.Error("expected error for non-increasing timestamps")
	}
}

// TestParseSamplesCSVStream_DelayedAcceptedLiterals verifies that strconv.ParseBool's
// exact accepted literals work for the delayed field.
func TestParseSamplesCSVStream_DelayedAcceptedLiterals(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"1", true},
		{"t", true},
		{"T", true},
		{"TRUE", true},
		{"true", true},
		{"True", true},
		{"0", false},
		{"f", false},
		{"F", false},
		{"FALSE", false},
		{"false", false},
		{"False", false},
	}

	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			data := mutateField(fullDataRow, 5, tc.value)

			samples, err := ParseSamplesCSVStream(
				strings.NewReader(makeInput(fullProductionHeader, data)),
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if samples[0].Delayed != tc.want {
				t.Fatalf("Delayed=%v, want %v", samples[0].Delayed, tc.want)
			}
		})
	}
}

// TestParseSamplesCSVStream_DelayedRejectedLiterals verifies that everything outside
// strconv.ParseBool's accepted literals is rejected.
func TestParseSamplesCSVStream_DelayedRejectedLiterals(t *testing.T) {
	values := []string{
		"",
		"maybe",
		"yes",
		"no",
		"on",
		"off",
		"YES",
		"NO",
		"TrUe",
		"2",
	}

	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			data := mutateField(fullDataRow, 5, value)

			_, err := ParseSamplesCSVStream(
				strings.NewReader(makeInput(fullProductionHeader, data)),
			)
			if err == nil {
				t.Fatalf("expected error for delayed=%q", value)
			}
		})
	}
}

// Test malformed availability flags
func TestParseSamplesCSVStream_MalformedAvailabilityFlag(t *testing.T) {
	availCols := []int{13, 24, 25, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40} // has_* columns

	for _, col := range availCols {
		data := mutateField(fullDataRow, col, "yes")
		_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(fullProductionHeader, data)))
		if err == nil {
			t.Errorf("expected error for malformed availability flag at column %d", col)
		}
	}
}

// Test negative unavailable metrics
func TestParseSamplesCSVStream_NegativeUnavailableMetric(t *testing.T) {
	// Test that negative values are rejected even when has_*=false
	// This requires constructing data where has_*=false but value != 0

	// Use docker_memory_usage_bytes (index 12) with has_docker_memory=false (index 13)
	data := fullDataRow
	parts := strings.Split(data, ",")
	parts[12] = "-1"    // docker_memory_usage_bytes = -1
	parts[13] = "false" // has_docker_memory = false (inconsistent!)
	badData := strings.Join(parts, ",")

	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(fullProductionHeader, badData)))
	if err == nil {
		t.Error("expected error for negative docker_memory_usage_bytes")
	}
}

// Test PID change
func TestParseSamplesCSVStream_PIDChange(t *testing.T) {
	data1 := fullDataRow
	data2 := mutateField(fullDataRow, 2, "5678")
	input := fullProductionHeader + "\n" + data1 + "\n" + data2
	_, err := ParseSamplesCSVStream(strings.NewReader(input))
	if err == nil {
		t.Error("expected error for PID change")
	}
}

// Test process-start-time change
func TestParseSamplesCSVStream_ProcessStartTimeChange(t *testing.T) {
	data1 := fullDataRow
	data2 := mutateField(fullDataRow, 3, "9999999")
	input := fullProductionHeader + "\n" + data1 + "\n" + data2
	_, err := ParseSamplesCSVStream(strings.NewReader(input))
	if err == nil {
		t.Error("expected error for process_start_time change")
	}
}

// Test phase regression
func TestParseSamplesCSVStream_PhaseRegression(t *testing.T) {
	// baseline (rank 3) then startup (rank 1) is a regression
	data1 := mutateField(fullDataRow, 4, "baseline")
	data2 := mutateField(fullDataRow, 4, "startup")
	data2 = mutateField(data2, 1, "2024-01-01T00:00:01Z")
	input := fullProductionHeader + "\n" + data1 + "\n" + data2
	_, err := ParseSamplesCSVStream(strings.NewReader(input))
	if err == nil {
		t.Error("expected error for phase regression")
	}
}

// Test maximum sample count
func TestParseSamplesCSVStream_MaxSampleCount(t *testing.T) {
	// Generate more than maxSamplesPerCSV samples
	var sb strings.Builder
	sb.WriteString(fullProductionHeader)
	sb.WriteString("\n")

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i <= maxSamplesPerCSV; i++ {
		ts := baseTime.Add(time.Duration(i) * time.Second)
		data := mutateField(mutateField(fullDataRow, 0, strconv.Itoa(i)), 1, ts.Format(time.RFC3339))
		sb.WriteString(data)
		sb.WriteString("\n")
	}

	_, err := ParseSamplesCSVStream(strings.NewReader(sb.String()))
	if err == nil {
		t.Error("expected error for exceeding max sample count")
	}
}

// Test complete phase rejection
func TestParseSamplesCSVStream_CompletePhaseRejected(t *testing.T) {
	data := mutateField(fullDataRow, 4, "complete")
	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(fullProductionHeader, data)))
	if err == nil {
		t.Error("expected error for 'complete' phase")
	}
}

// Test boolean parsing
func TestParseSamplesCSVStream_BooleanParsing(t *testing.T) {
	data := fullDataRow
	samples, err := ParseSamplesCSVStream(strings.NewReader(makeInput(fullProductionHeader, data)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !samples[0].HasDockerMemory {
		t.Error("expected HasDockerMemory to be true")
	}
	if !samples[0].HasFDCount {
		t.Error("expected HasFDCount to be true")
	}
	if !samples[0].HasCgroup {
		t.Error("expected HasCgroup to be true")
	}
	if samples[0].HasSwap {
		t.Error("expected HasSwap to be false")
	}
}

func TestParseSamplesCSVStream_ShortRecord(t *testing.T) {
	// Short record (missing fields)
	header := "sequence,timestamp,process_pid,process_start_time,phase,delayed"
	data := `0,2024-01-01T00:00:00Z,1234,1000000`
	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(header, data)))
	if err == nil {
		t.Error("expected error for short record")
	}
}

func TestParseSamplesCSVStream_ValidSamplePhases(t *testing.T) {
	// Test all valid sample phases (not 'complete')
	validPhases := []string{"startup", "warmup", "baseline", "stimulus", "settling", "final"}

	for _, phase := range validPhases {
		data := mutateField(fullDataRow, 4, phase)
		_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(fullProductionHeader, data)))
		if err != nil {
			t.Errorf("phase %q: unexpected error: %v", phase, err)
		}
	}
}

func TestParseSamplesCSVStream_TimestampRFC3339Nano(t *testing.T) {
	// Test RFC3339Nano format with nanoseconds
	data := mutateField(fullDataRow, 1, "2024-01-01T00:00:00.123456789Z")
	samples, err := ParseSamplesCSVStream(strings.NewReader(makeInput(fullProductionHeader, data)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	// Verify the timestamp was parsed with nanoseconds
	expected := time.Date(2024, 1, 1, 0, 0, 0, 123456789, time.UTC)
	if !samples[0].Timestamp.Equal(expected) {
		t.Errorf("expected timestamp %v, got %v", expected, samples[0].Timestamp)
	}
}

func TestParseSamplesCSVStream_EmptyTimestamp(t *testing.T) {
	// Replace timestamp with empty
	data := mutateField(fullDataRow, 1, "")
	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(fullProductionHeader, data)))
	if err == nil {
		t.Error("expected error for empty timestamp")
	}
}

func TestParseSamplesCSVStream_EmptyPID(t *testing.T) {
	// Replace PID with empty
	data := mutateField(fullDataRow, 2, "")
	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(fullProductionHeader, data)))
	if err == nil {
		t.Error("expected error for empty PID")
	}
}

func TestParseSamplesCSVStream_HeaderMismatch(t *testing.T) {
	// Test that header column count must match production schema
	shortHeader := "sequence,timestamp,process_pid,process_start_time"
	data := `0,2024-01-01T00:00:00Z,1234,1000000`
	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(shortHeader, data)))
	if err == nil {
		t.Error("expected error for header column count mismatch")
	}
}

func TestParseSamplesCSVStream_MultipleRows(t *testing.T) {
	// Test parsing multiple rows
	data1 := fullDataRow
	data2 := mutateField(mutateField(fullDataRow, 0, "1"), 1, "2024-01-01T01:00:00Z")
	data3 := mutateField(mutateField(fullDataRow, 0, "2"), 1, "2024-01-01T02:00:00Z")
	input := fullProductionHeader + "\n" + data1 + "\n" + data2 + "\n" + data3
	samples, err := ParseSamplesCSVStream(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(samples))
	}
	if samples[0].Sequence != 0 || samples[1].Sequence != 1 || samples[2].Sequence != 2 {
		t.Error("unexpected sequence values")
	}
}

func TestParseSamplesCSVStream_InvalidTimestampFormat(t *testing.T) {
	// Replace timestamp with invalid format
	data := mutateField(fullDataRow, 1, "not-a-timestamp")
	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(fullProductionHeader, data)))
	if err == nil {
		t.Error("expected error for invalid timestamp format")
	}
}

func TestParseSamplesCSVStream_InvalidBooleanValue(t *testing.T) {
	// Replace first boolean field with invalid value
	data := mutateField(fullDataRow, 13, "yes")
	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(fullProductionHeader, data)))
	if err == nil {
		t.Error("expected error for invalid boolean value 'yes'")
	}
}

func TestParseSamplesCSVStream_HeaderOrderRequired(t *testing.T) {
	// Test that header column ORDER must match production schema exactly
	// Swap first two columns
	header := "timestamp,sequence,process_pid,process_start_time,phase,delayed,rss_kib,pss_kib,pss_anon_kib,private_dirty_kib,anonymous_kib,swap_kib,docker_memory_usage_bytes,docker_memory_limit_bytes,has_docker_memory,vma_count,fd_count,socket_fd_count,thread_count,pid_count,cgroup_anon_bytes,cgroup_current_bytes,cgroup_memory_stat_anon,has_cgroup,has_cgroup_anon,oom_events,oom_kill_events,bgp_state,bgp_fsm_ticks,reconnect_count,has_rss,has_pss,has_pss_anon,has_private_dirty,has_anonymous,has_swap,has_thread_count,has_pid_count,has_fd_count,has_socket_fd_count,has_vma_count"
	data := fullDataRow
	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(header, data)))
	if err == nil {
		t.Error("expected error for header column order mismatch")
	}
}

// Test availability consistency: has_*=false implies value must be zero
func TestParseSamplesCSVStream_AvailabilityConsistency(t *testing.T) {
	// Test that has_swap=false but swap_kib != 0 is rejected
	data := fullDataRow
	parts := strings.Split(data, ",")
	parts[11] = "100"   // swap_kib = 100
	parts[35] = "false" // has_swap = false (inconsistent!)
	badData := strings.Join(parts, ",")

	_, err := ParseSamplesCSVStream(strings.NewReader(makeInput(fullProductionHeader, badData)))
	if err == nil {
		t.Error("expected error for has_swap=false but swap_kib != 0")
	}
}

// Test round-trip: writer -> reader produces identical structs
func TestRoundTripWriteRead(t *testing.T) {
	// Verify that samples written via evidence.Writer and read via ParseSamplesCSVStream
	// produce identical Sample structs. This proves the CSV format is canonical.
	// Uses evidence.NewWriter with a temp directory.
	tmp := t.TempDir()
	runID := "roundtrip-test"
	scenario := "canary-growing"
	writer := evidence.NewWriter(runID, scenario, tmp)

	// Create known samples with all fields populated
	samples := []sampling.Sample{
		{
			Sequence:               0,
			Timestamp:              time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			PID:                    12345,
			ProcessStartTime:       9876543210,
			Phase:                  sampling.PhaseBaseline,
			Delayed:                false,
			RSSKiB:                 10240,
			PSSKiB:                 8192,
			PSSAnonKiB:             4096,
			PrivateDirtyKiB:        2048,
			AnonymousKiB:           1024,
			SwapKiB:                0,
			DockerMemoryUsageBytes: 10485760,
			DockerMemoryLimitBytes: 67108864,
			HasDockerMemory:        true,
			VMACount:               50,
			FDCount:                10,
			SocketFDCount:          2,
			ThreadCount:            5,
			PIDCount:               1,
			CgroupAnonBytes:        10485760,
			CgroupCurrentBytes:     10485760,
			CgroupMemoryStatAnon:   5242880,
			HasCgroupAnon:          true,
			HasCgroup:              true,
			OOMEvents:              0,
			OOMKillEvents:          0,
			BGPState:               "",
			BGPFSMTicks:            0,
			ReconnectCount:         0,
			HasRSS:                 true,
			HasPSS:                 true,
			HasPSSAnon:             true,
			HasPrivateDirty:        true,
			HasAnonymous:           true,
			HasSwap:                false,
			HasThreadCount:         true,
			HasPIDCount:            true,
			HasFDCount:             true,
			HasSocketFDCount:       true,
			HasVMACount:            true,
		},
		{
			Sequence:               1,
			Timestamp:              time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC),
			PID:                    12345,
			ProcessStartTime:       9876543210,
			Phase:                  sampling.PhaseFinal,
			Delayed:                false,
			RSSKiB:                 11264,
			PSSKiB:                 9216,
			PSSAnonKiB:             5120,
			PrivateDirtyKiB:        3072,
			AnonymousKiB:           2048,
			SwapKiB:                0,
			DockerMemoryUsageBytes: 11534336,
			DockerMemoryLimitBytes: 67108864,
			HasDockerMemory:        true,
			VMACount:               55,
			FDCount:                12,
			SocketFDCount:          3,
			ThreadCount:            6,
			PIDCount:               1,
			CgroupAnonBytes:        11534336,
			CgroupCurrentBytes:     11534336,
			CgroupMemoryStatAnon:   6291456,
			HasCgroupAnon:          true,
			HasCgroup:              true,
			OOMEvents:              0,
			OOMKillEvents:          0,
			BGPState:               "Established",
			BGPFSMTicks:            100,
			ReconnectCount:         0,
			HasRSS:                 true,
			HasPSS:                 true,
			HasPSSAnon:             true,
			HasPrivateDirty:        true,
			HasAnonymous:           true,
			HasSwap:                false,
			HasThreadCount:         true,
			HasPIDCount:            true,
			HasFDCount:             true,
			HasSocketFDCount:       true,
			HasVMACount:            true,
		},
	}

	// Write samples via evidence.Writer
	if err := writer.WriteSamplesCSV(samples); err != nil {
		t.Fatalf("WriteSamplesCSV: %v", err)
	}

	// Read back via ParseSamplesCSVStream
	samplesPath := tmp + "/samples.csv"
	data, err := os.ReadFile(samplesPath)
	if err != nil {
		t.Fatalf("read samples.csv: %v", err)
	}

	readBack, err := ParseSamplesCSVStream(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("ParseSamplesCSVStream: %v", err)
	}

	// Verify sample count
	if len(readBack) != len(samples) {
		t.Fatalf("sample count mismatch: wrote %d, read %d", len(samples), len(readBack))
	}

	// Verify each field in each sample (complete equality check)
	for i := range samples {
		s := samples[i]
		rb := readBack[i]

		// Identity fields
		if rb.Sequence != s.Sequence {
			t.Errorf("sample[%d].Sequence: got %d, want %d", i, rb.Sequence, s.Sequence)
		}
		if !rb.Timestamp.Equal(s.Timestamp) {
			t.Errorf("sample[%d].Timestamp: got %v, want %v", i, rb.Timestamp, s.Timestamp)
		}
		if rb.PID != s.PID {
			t.Errorf("sample[%d].PID: got %d, want %d", i, rb.PID, s.PID)
		}
		if rb.ProcessStartTime != s.ProcessStartTime {
			t.Errorf("sample[%d].ProcessStartTime: got %d, want %d", i, rb.ProcessStartTime, s.ProcessStartTime)
		}
		if rb.Phase != s.Phase {
			t.Errorf("sample[%d].Phase: got %s, want %s", i, rb.Phase, s.Phase)
		}
		if rb.Delayed != s.Delayed {
			t.Errorf("sample[%d].Delayed: got %v, want %v", i, rb.Delayed, s.Delayed)
		}

		// Memory fields
		if rb.RSSKiB != s.RSSKiB {
			t.Errorf("sample[%d].RSSKiB: got %d, want %d", i, rb.RSSKiB, s.RSSKiB)
		}
		if rb.PSSKiB != s.PSSKiB {
			t.Errorf("sample[%d].PSSKiB: got %d, want %d", i, rb.PSSKiB, s.PSSKiB)
		}
		if rb.PSSAnonKiB != s.PSSAnonKiB {
			t.Errorf("sample[%d].PSSAnonKiB: got %d, want %d", i, rb.PSSAnonKiB, s.PSSAnonKiB)
		}
		if rb.PrivateDirtyKiB != s.PrivateDirtyKiB {
			t.Errorf("sample[%d].PrivateDirtyKiB: got %d, want %d", i, rb.PrivateDirtyKiB, s.PrivateDirtyKiB)
		}
		if rb.AnonymousKiB != s.AnonymousKiB {
			t.Errorf("sample[%d].AnonymousKiB: got %d, want %d", i, rb.AnonymousKiB, s.AnonymousKiB)
		}
		if rb.SwapKiB != s.SwapKiB {
			t.Errorf("sample[%d].SwapKiB: got %d, want %d", i, rb.SwapKiB, s.SwapKiB)
		}

		// Docker memory
		if rb.DockerMemoryUsageBytes != s.DockerMemoryUsageBytes {
			t.Errorf("sample[%d].DockerMemoryUsageBytes: got %d, want %d", i, rb.DockerMemoryUsageBytes, s.DockerMemoryUsageBytes)
		}
		if rb.DockerMemoryLimitBytes != s.DockerMemoryLimitBytes {
			t.Errorf("sample[%d].DockerMemoryLimitBytes: got %d, want %d", i, rb.DockerMemoryLimitBytes, s.DockerMemoryLimitBytes)
		}
		if rb.HasDockerMemory != s.HasDockerMemory {
			t.Errorf("sample[%d].HasDockerMemory: got %v, want %v", i, rb.HasDockerMemory, s.HasDockerMemory)
		}

		// Resource counts
		if rb.VMACount != s.VMACount {
			t.Errorf("sample[%d].VMACount: got %d, want %d", i, rb.VMACount, s.VMACount)
		}
		if rb.FDCount != s.FDCount {
			t.Errorf("sample[%d].FDCount: got %d, want %d", i, rb.FDCount, s.FDCount)
		}
		if rb.SocketFDCount != s.SocketFDCount {
			t.Errorf("sample[%d].SocketFDCount: got %d, want %d", i, rb.SocketFDCount, s.SocketFDCount)
		}
		if rb.ThreadCount != s.ThreadCount {
			t.Errorf("sample[%d].ThreadCount: got %d, want %d", i, rb.ThreadCount, s.ThreadCount)
		}
		if rb.PIDCount != s.PIDCount {
			t.Errorf("sample[%d].PIDCount: got %d, want %d", i, rb.PIDCount, s.PIDCount)
		}

		// Cgroup
		if rb.CgroupAnonBytes != s.CgroupAnonBytes {
			t.Errorf("sample[%d].CgroupAnonBytes: got %d, want %d", i, rb.CgroupAnonBytes, s.CgroupAnonBytes)
		}
		if rb.CgroupCurrentBytes != s.CgroupCurrentBytes {
			t.Errorf("sample[%d].CgroupCurrentBytes: got %d, want %d", i, rb.CgroupCurrentBytes, s.CgroupCurrentBytes)
		}
		if rb.CgroupMemoryStatAnon != s.CgroupMemoryStatAnon {
			t.Errorf("sample[%d].CgroupMemoryStatAnon: got %d, want %d", i, rb.CgroupMemoryStatAnon, s.CgroupMemoryStatAnon)
		}
		if rb.HasCgroup != s.HasCgroup {
			t.Errorf("sample[%d].HasCgroup: got %v, want %v", i, rb.HasCgroup, s.HasCgroup)
		}
		if rb.HasCgroupAnon != s.HasCgroupAnon {
			t.Errorf("sample[%d].HasCgroupAnon: got %v, want %v", i, rb.HasCgroupAnon, s.HasCgroupAnon)
		}

		// Semantic
		if rb.OOMEvents != s.OOMEvents {
			t.Errorf("sample[%d].OOMEvents: got %d, want %d", i, rb.OOMEvents, s.OOMEvents)
		}
		if rb.OOMKillEvents != s.OOMKillEvents {
			t.Errorf("sample[%d].OOMKillEvents: got %d, want %d", i, rb.OOMKillEvents, s.OOMKillEvents)
		}
		if rb.BGPState != s.BGPState {
			t.Errorf("sample[%d].BGPState: got %s, want %s", i, rb.BGPState, s.BGPState)
		}
		if rb.BGPFSMTicks != s.BGPFSMTicks {
			t.Errorf("sample[%d].BGPFSMTicks: got %d, want %d", i, rb.BGPFSMTicks, s.BGPFSMTicks)
		}
		if rb.ReconnectCount != s.ReconnectCount {
			t.Errorf("sample[%d].ReconnectCount: got %d, want %d", i, rb.ReconnectCount, s.ReconnectCount)
		}

		// Availability flags
		if rb.HasRSS != s.HasRSS {
			t.Errorf("sample[%d].HasRSS: got %v, want %v", i, rb.HasRSS, s.HasRSS)
		}
		if rb.HasPSS != s.HasPSS {
			t.Errorf("sample[%d].HasPSS: got %v, want %v", i, rb.HasPSS, s.HasPSS)
		}
		if rb.HasPSSAnon != s.HasPSSAnon {
			t.Errorf("sample[%d].HasPSSAnon: got %v, want %v", i, rb.HasPSSAnon, s.HasPSSAnon)
		}
		if rb.HasPrivateDirty != s.HasPrivateDirty {
			t.Errorf("sample[%d].HasPrivateDirty: got %v, want %v", i, rb.HasPrivateDirty, s.HasPrivateDirty)
		}
		if rb.HasAnonymous != s.HasAnonymous {
			t.Errorf("sample[%d].HasAnonymous: got %v, want %v", i, rb.HasAnonymous, s.HasAnonymous)
		}
		if rb.HasSwap != s.HasSwap {
			t.Errorf("sample[%d].HasSwap: got %v, want %v", i, rb.HasSwap, s.HasSwap)
		}
		if rb.HasThreadCount != s.HasThreadCount {
			t.Errorf("sample[%d].HasThreadCount: got %v, want %v", i, rb.HasThreadCount, s.HasThreadCount)
		}
		if rb.HasPIDCount != s.HasPIDCount {
			t.Errorf("sample[%d].HasPIDCount: got %v, want %v", i, rb.HasPIDCount, s.HasPIDCount)
		}
		if rb.HasFDCount != s.HasFDCount {
			t.Errorf("sample[%d].HasFDCount: got %v, want %v", i, rb.HasFDCount, s.HasFDCount)
		}
		if rb.HasSocketFDCount != s.HasSocketFDCount {
			t.Errorf("sample[%d].HasSocketFDCount: got %v, want %v", i, rb.HasSocketFDCount, s.HasSocketFDCount)
		}
		if rb.HasVMACount != s.HasVMACount {
			t.Errorf("sample[%d].HasVMACount: got %v, want %v", i, rb.HasVMACount, s.HasVMACount)
		}
	}
}
