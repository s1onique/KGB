// parser.go — Fail-closed CSV parser for memory lab samples
//
// Strict parsing with:
// - Authority from sampling.CSVHeaders()
// - Typed helpers that never discard errors
// - Field domain validation
// - Availability consistency enforcement
// - Sample progression validation
// - Maximum sample count enforcement
// - No bypass paths

package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling"
)

// Hard ceiling for maximum samples per CSV (derived from smoke phase config: 6 phases × ~60s × 1Hz + buffer)
const maxSamplesPerCSV = 500

// ParseError represents a CSV parsing error with row/column context.
type ParseError struct {
	Row    int
	Column string
	Err    error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("row %d, column %q: %v", e.Row, e.Column, e.Err)
}

func (e *ParseError) Unwrap() error {
	return e.Err
}

// columnIndex maps column names to their indices
type columnIndex map[string]int

// parser is the internal CSV parser state
type parser struct {
	reader      *csv.Reader
	colIdx      columnIndex
	rowNum      int
	lastTS      time.Time
	lastPID     int
	lastPST     uint64
	lastRank    int
	sampleCount int
}

// newParser creates a new CSV parser with the sampling schema as authority
func newParser(r io.Reader) (*parser, error) {
	csvReader := csv.NewReader(r)
	csvReader.TrimLeadingSpace = true

	// Read header
	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}

	// Build column index from sampling.CSVHeaders() as authority
	expectedHeaders := sampling.CSVHeaders()
	if len(header) != len(expectedHeaders) {
		return nil, fmt.Errorf("header column count mismatch: got %d, expected %d",
			len(header), len(expectedHeaders))
	}

	colIdx := make(columnIndex)
	seen := make(map[string]bool)

	for i, name := range header {
		col := strings.TrimSpace(name)
		if col == "" {
			return nil, fmt.Errorf("header: empty column name at index %d", i)
		}
		if seen[col] {
			return nil, fmt.Errorf("header: duplicate column %q", col)
		}
		seen[col] = true
		colIdx[col] = i
	}

	// Verify header matches expected exactly (order and names)
	for i, expected := range expectedHeaders {
		if i >= len(header) {
			return nil, fmt.Errorf("header: missing expected column %q at index %d", expected, i)
		}
		if header[i] != expected {
			return nil, fmt.Errorf("header column mismatch at index %d: got %q, expected %q",
				i, header[i], expected)
		}
	}

	csvReader.FieldsPerRecord = len(header)

	return &parser{
		reader: csvReader,
		colIdx: colIdx,
	}, nil
}

// phaseRank returns the ordinal rank of a phase (higher = later in lifecycle)
func phaseRank(phase sampling.Phase) int {
	switch phase {
	case sampling.PhaseStartup:
		return 1
	case sampling.PhaseWarmup:
		return 2
	case sampling.PhaseBaseline:
		return 3
	case sampling.PhaseStimulus:
		return 4
	case sampling.PhaseSettling:
		return 5
	case sampling.PhaseFinal:
		return 6
	default:
		return 0
	}
}

// requiredInt64 parses a required int64 field
func requiredInt64(rowNum int, record []string, colIdx columnIndex, col string) (int64, *ParseError) {
	idx, ok := colIdx[col]
	if !ok {
		return 0, &ParseError{Row: rowNum, Column: col, Err: fmt.Errorf("unknown column")}
	}
	if idx >= len(record) {
		return 0, &ParseError{Row: rowNum, Column: col, Err: fmt.Errorf("column index %d out of bounds", idx)}
	}
	val := strings.TrimSpace(record[idx])
	if val == "" {
		return 0, &ParseError{Row: rowNum, Column: col, Err: fmt.Errorf("empty")}
	}
	v, parseErr := strconv.ParseInt(val, 10, 64)
	if parseErr != nil {
		return 0, &ParseError{Row: rowNum, Column: col, Err: parseErr}
	}
	return v, nil
}

// requiredInt parses a required int field
func requiredInt(rowNum int, record []string, colIdx columnIndex, col string) (int, *ParseError) {
	idx, ok := colIdx[col]
	if !ok {
		return 0, &ParseError{Row: rowNum, Column: col, Err: fmt.Errorf("unknown column")}
	}
	if idx >= len(record) {
		return 0, &ParseError{Row: rowNum, Column: col, Err: fmt.Errorf("column index %d out of bounds", idx)}
	}
	val := strings.TrimSpace(record[idx])
	if val == "" {
		return 0, &ParseError{Row: rowNum, Column: col, Err: fmt.Errorf("empty")}
	}
	v, parseErr := strconv.Atoi(val)
	if parseErr != nil {
		return 0, &ParseError{Row: rowNum, Column: col, Err: parseErr}
	}
	return v, nil
}

// requiredUint64 parses a required uint64 field
func requiredUint64(rowNum int, record []string, colIdx columnIndex, col string) (uint64, *ParseError) {
	idx, ok := colIdx[col]
	if !ok {
		return 0, &ParseError{Row: rowNum, Column: col, Err: fmt.Errorf("unknown column")}
	}
	if idx >= len(record) {
		return 0, &ParseError{Row: rowNum, Column: col, Err: fmt.Errorf("column index %d out of bounds", idx)}
	}
	val := strings.TrimSpace(record[idx])
	if val == "" {
		return 0, &ParseError{Row: rowNum, Column: col, Err: fmt.Errorf("empty")}
	}
	v, parseErr := strconv.ParseUint(val, 10, 64)
	if parseErr != nil {
		return 0, &ParseError{Row: rowNum, Column: col, Err: parseErr}
	}
	return v, nil
}

// requiredBool parses a required boolean field
func requiredBool(rowNum int, record []string, colIdx columnIndex, col string) (bool, *ParseError) {
	idx, ok := colIdx[col]
	if !ok {
		return false, &ParseError{Row: rowNum, Column: col, Err: fmt.Errorf("unknown column")}
	}
	if idx >= len(record) {
		return false, &ParseError{Row: rowNum, Column: col, Err: fmt.Errorf("column index %d out of bounds", idx)}
	}
	val := strings.TrimSpace(record[idx])
	if val == "" {
		return false, &ParseError{Row: rowNum, Column: col, Err: fmt.Errorf("empty")}
	}
	v, parseErr := strconv.ParseBool(val)
	if parseErr != nil {
		return false, &ParseError{Row: rowNum, Column: col, Err: parseErr}
	}
	return v, nil
}

// requiredTimestamp parses a required RFC3339Nano timestamp
func requiredTimestamp(rowNum int, record []string, colIdx columnIndex, col string) (time.Time, *ParseError) {
	idx, ok := colIdx[col]
	if !ok {
		return time.Time{}, &ParseError{Row: rowNum, Column: col, Err: fmt.Errorf("unknown column")}
	}
	if idx >= len(record) {
		return time.Time{}, &ParseError{Row: rowNum, Column: col, Err: fmt.Errorf("column index %d out of bounds", idx)}
	}
	val := strings.TrimSpace(record[idx])
	if val == "" {
		return time.Time{}, &ParseError{Row: rowNum, Column: col, Err: fmt.Errorf("empty")}
	}
	v, parseErr := time.Parse(time.RFC3339Nano, val)
	if parseErr != nil {
		return time.Time{}, &ParseError{Row: rowNum, Column: col, Err: parseErr}
	}
	return v, nil
}

// optionalString parses an optional string field
func optionalString(rowNum int, record []string, colIdx columnIndex, col string) (string, *ParseError) {
	idx, ok := colIdx[col]
	if !ok {
		return "", nil
	}
	if idx >= len(record) {
		return "", nil
	}
	return strings.TrimSpace(record[idx]), nil
}

// ParseSamplesCSVStream parses samples CSV with strict fail-closed validation.
// Uses sampling.CSVHeaders() as the authoritative schema.
// Returns error on any malformed input.
func ParseSamplesCSVStream(r io.Reader) ([]sampling.Sample, error) {
	p, err := newParser(r)
	if err != nil {
		return nil, err
	}

	var samples []sampling.Sample

	for {
		record, err := p.reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("row %d: CSV read error: %w", p.rowNum+1, err)
		}

		p.rowNum++
		p.sampleCount++

		// Enforce maximum sample count
		if p.sampleCount > maxSamplesPerCSV {
			return nil, fmt.Errorf("exceeded maximum sample count: %d > %d", p.sampleCount, maxSamplesPerCSV)
		}

		// Parse required identity fields
		seq, parseErr := requiredInt(p.rowNum, record, p.colIdx, "sequence")
		if parseErr != nil {
			return nil, parseErr
		}

		timestamp, parseErr := requiredTimestamp(p.rowNum, record, p.colIdx, "timestamp")
		if parseErr != nil {
			return nil, parseErr
		}

		pid, parseErr := requiredInt(p.rowNum, record, p.colIdx, "process_pid")
		if parseErr != nil {
			return nil, parseErr
		}
		if pid <= 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "process_pid", Err: fmt.Errorf("must be > 0, got %d", pid)}
		}

		startTime, parseErr := requiredUint64(p.rowNum, record, p.colIdx, "process_start_time")
		if parseErr != nil {
			return nil, parseErr
		}
		if startTime == 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "process_start_time", Err: fmt.Errorf("must be > 0")}
		}

		phaseStr, parseErr := optionalString(p.rowNum, record, p.colIdx, "phase")
		if parseErr != nil {
			return nil, parseErr
		}
		if phaseStr == "" {
			return nil, &ParseError{Row: p.rowNum, Column: "phase", Err: fmt.Errorf("empty")}
		}
		phase := sampling.ParsePhase(phaseStr)
		if phase == "" {
			return nil, &ParseError{Row: p.rowNum, Column: "phase", Err: fmt.Errorf("invalid phase %q", phaseStr)}
		}
		// Reject 'complete' as a sample phase
		if phase == sampling.PhaseComplete {
			return nil, &ParseError{Row: p.rowNum, Column: "phase", Err: fmt.Errorf("invalid sample phase %q", phaseStr)}
		}

		// delayed is required (use requiredBool)
		delayed, parseErr := requiredBool(p.rowNum, record, p.colIdx, "delayed")
		if parseErr != nil {
			return nil, parseErr
		}

		// Validate sample progression for row > 0
		if p.rowNum > 1 {
			// Sequence must increment by exactly 1
			expectedSeq := samples[len(samples)-1].Sequence + 1
			if seq != expectedSeq {
				return nil, &ParseError{Row: p.rowNum, Column: "sequence",
					Err: fmt.Errorf("expected %d, got %d", expectedSeq, seq)}
			}

			// Timestamp must strictly increase
			if !timestamp.After(p.lastTS) {
				return nil, &ParseError{Row: p.rowNum, Column: "timestamp",
					Err: fmt.Errorf("not strictly increasing: row %d=%v, row %d=%v",
						p.rowNum, timestamp, p.rowNum-1, p.lastTS)}
			}

			// Phase rank must never decrease
			currentRank := phaseRank(phase)
			if currentRank < p.lastRank {
				return nil, &ParseError{Row: p.rowNum, Column: "phase",
					Err: fmt.Errorf("phase regression: rank %d < %d", currentRank, p.lastRank)}
			}

			// PID must remain constant
			if pid != p.lastPID {
				return nil, &ParseError{Row: p.rowNum, Column: "process_pid",
					Err: fmt.Errorf("PID changed from %d to %d", p.lastPID, pid)}
			}

			// Process start time must remain constant
			if startTime != p.lastPST {
				return nil, &ParseError{Row: p.rowNum, Column: "process_start_time",
					Err: fmt.Errorf("start time changed from %d to %d", p.lastPST, startTime)}
			}
		}

		// Update last values for next row validation
		p.lastTS = timestamp
		p.lastPID = pid
		p.lastPST = startTime
		p.lastRank = phaseRank(phase)

		// Memory fields - reject negatives unconditionally
		rssKiB, parseErr := requiredInt64(p.rowNum, record, p.colIdx, "rss_kib")
		if parseErr != nil {
			return nil, parseErr
		}
		if rssKiB < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "rss_kib", Err: fmt.Errorf("negative value: %d", rssKiB)}
		}

		pssKiB, parseErr := requiredInt64(p.rowNum, record, p.colIdx, "pss_kib")
		if parseErr != nil {
			return nil, parseErr
		}
		if pssKiB < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "pss_kib", Err: fmt.Errorf("negative value: %d", pssKiB)}
		}

		pssAnonKiB, parseErr := requiredInt64(p.rowNum, record, p.colIdx, "pss_anon_kib")
		if parseErr != nil {
			return nil, parseErr
		}
		if pssAnonKiB < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "pss_anon_kib", Err: fmt.Errorf("negative value: %d", pssAnonKiB)}
		}

		privateDirtyKiB, parseErr := requiredInt64(p.rowNum, record, p.colIdx, "private_dirty_kib")
		if parseErr != nil {
			return nil, parseErr
		}
		if privateDirtyKiB < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "private_dirty_kib", Err: fmt.Errorf("negative value: %d", privateDirtyKiB)}
		}

		anonymousKiB, parseErr := requiredInt64(p.rowNum, record, p.colIdx, "anonymous_kib")
		if parseErr != nil {
			return nil, parseErr
		}
		if anonymousKiB < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "anonymous_kib", Err: fmt.Errorf("negative value: %d", anonymousKiB)}
		}

		swapKiB, parseErr := requiredInt64(p.rowNum, record, p.colIdx, "swap_kib")
		if parseErr != nil {
			return nil, parseErr
		}
		if swapKiB < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "swap_kib", Err: fmt.Errorf("negative value: %d", swapKiB)}
		}

		// Docker container memory - required fields
		hasDocker, parseErr := requiredBool(p.rowNum, record, p.colIdx, "has_docker_memory")
		if parseErr != nil {
			return nil, parseErr
		}

		dockerUsage, parseErr := requiredInt64(p.rowNum, record, p.colIdx, "docker_memory_usage_bytes")
		if parseErr != nil {
			return nil, parseErr
		}
		if dockerUsage < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "docker_memory_usage_bytes", Err: fmt.Errorf("negative value: %d", dockerUsage)}
		}

		dockerLimit, parseErr := requiredInt64(p.rowNum, record, p.colIdx, "docker_memory_limit_bytes")
		if parseErr != nil {
			return nil, parseErr
		}
		if dockerLimit < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "docker_memory_limit_bytes", Err: fmt.Errorf("negative value: %d", dockerLimit)}
		}

		// Docker availability consistency
		if !hasDocker && dockerUsage != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "docker_memory_usage_bytes", Err: fmt.Errorf("has_docker_memory=false but value is %d", dockerUsage)}
		}
		if !hasDocker && dockerLimit != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "docker_memory_limit_bytes", Err: fmt.Errorf("has_docker_memory=false but value is %d", dockerLimit)}
		}

		// Resource counts - reject negatives unconditionally
		vmaCount, parseErr := requiredInt(p.rowNum, record, p.colIdx, "vma_count")
		if parseErr != nil {
			return nil, parseErr
		}
		if vmaCount < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "vma_count", Err: fmt.Errorf("negative value: %d", vmaCount)}
		}

		fdCount, parseErr := requiredInt(p.rowNum, record, p.colIdx, "fd_count")
		if parseErr != nil {
			return nil, parseErr
		}
		if fdCount < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "fd_count", Err: fmt.Errorf("negative value: %d", fdCount)}
		}

		socketFDCount, parseErr := requiredInt(p.rowNum, record, p.colIdx, "socket_fd_count")
		if parseErr != nil {
			return nil, parseErr
		}
		if socketFDCount < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "socket_fd_count", Err: fmt.Errorf("negative value: %d", socketFDCount)}
		}

		threadCount, parseErr := requiredInt(p.rowNum, record, p.colIdx, "thread_count")
		if parseErr != nil {
			return nil, parseErr
		}
		if threadCount < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "thread_count", Err: fmt.Errorf("negative value: %d", threadCount)}
		}

		pidCount, parseErr := requiredInt(p.rowNum, record, p.colIdx, "pid_count")
		if parseErr != nil {
			return nil, parseErr
		}
		if pidCount < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "pid_count", Err: fmt.Errorf("negative value: %d", pidCount)}
		}

		// Cgroup memory - required fields
		hasCgroup, parseErr := requiredBool(p.rowNum, record, p.colIdx, "has_cgroup")
		if parseErr != nil {
			return nil, parseErr
		}

		hasCgroupAnon, parseErr := requiredBool(p.rowNum, record, p.colIdx, "has_cgroup_anon")
		if parseErr != nil {
			return nil, parseErr
		}

		cgroupAnonBytes, parseErr := requiredInt64(p.rowNum, record, p.colIdx, "cgroup_anon_bytes")
		if parseErr != nil {
			return nil, parseErr
		}
		if cgroupAnonBytes < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "cgroup_anon_bytes", Err: fmt.Errorf("negative value: %d", cgroupAnonBytes)}
		}

		cgroupCurrentBytes, parseErr := requiredInt64(p.rowNum, record, p.colIdx, "cgroup_current_bytes")
		if parseErr != nil {
			return nil, parseErr
		}
		if cgroupCurrentBytes < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "cgroup_current_bytes", Err: fmt.Errorf("negative value: %d", cgroupCurrentBytes)}
		}

		cgroupStatAnon, parseErr := requiredInt64(p.rowNum, record, p.colIdx, "cgroup_memory_stat_anon")
		if parseErr != nil {
			return nil, parseErr
		}
		if cgroupStatAnon < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "cgroup_memory_stat_anon", Err: fmt.Errorf("negative value: %d", cgroupStatAnon)}
		}

		// Cgroup availability consistency
		if !hasCgroup && cgroupAnonBytes != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "cgroup_anon_bytes", Err: fmt.Errorf("has_cgroup=false but value is %d", cgroupAnonBytes)}
		}
		if !hasCgroup && cgroupCurrentBytes != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "cgroup_current_bytes", Err: fmt.Errorf("has_cgroup=false but value is %d", cgroupCurrentBytes)}
		}
		if !hasCgroupAnon && cgroupStatAnon != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "cgroup_memory_stat_anon", Err: fmt.Errorf("has_cgroup_anon=false but value is %d", cgroupStatAnon)}
		}

		// Semantic signals - reject negatives unconditionally
		oomEvents, parseErr := requiredInt(p.rowNum, record, p.colIdx, "oom_events")
		if parseErr != nil {
			return nil, parseErr
		}
		if oomEvents < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "oom_events", Err: fmt.Errorf("negative value: %d", oomEvents)}
		}

		oomKillEvents, parseErr := requiredInt(p.rowNum, record, p.colIdx, "oom_kill_events")
		if parseErr != nil {
			return nil, parseErr
		}
		if oomKillEvents < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "oom_kill_events", Err: fmt.Errorf("negative value: %d", oomKillEvents)}
		}

		// bgp_state is the only optional string field
		bgpState, parseErr := optionalString(p.rowNum, record, p.colIdx, "bgp_state")
		if parseErr != nil {
			return nil, parseErr
		}

		bgpFSMTicks, parseErr := requiredInt64(p.rowNum, record, p.colIdx, "bgp_fsm_ticks")
		if parseErr != nil {
			return nil, parseErr
		}
		if bgpFSMTicks < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "bgp_fsm_ticks", Err: fmt.Errorf("negative value: %d", bgpFSMTicks)}
		}

		reconnectCount, parseErr := requiredInt64(p.rowNum, record, p.colIdx, "reconnect_count")
		if parseErr != nil {
			return nil, parseErr
		}
		if reconnectCount < 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "reconnect_count", Err: fmt.Errorf("negative value: %d", reconnectCount)}
		}

		// Availability flags - all required
		hasRSS, parseErr := requiredBool(p.rowNum, record, p.colIdx, "has_rss")
		if parseErr != nil {
			return nil, parseErr
		}

		hasPSS, parseErr := requiredBool(p.rowNum, record, p.colIdx, "has_pss")
		if parseErr != nil {
			return nil, parseErr
		}

		hasPSSAnon, parseErr := requiredBool(p.rowNum, record, p.colIdx, "has_pss_anon")
		if parseErr != nil {
			return nil, parseErr
		}

		hasPrivateDirty, parseErr := requiredBool(p.rowNum, record, p.colIdx, "has_private_dirty")
		if parseErr != nil {
			return nil, parseErr
		}

		hasAnonymous, parseErr := requiredBool(p.rowNum, record, p.colIdx, "has_anonymous")
		if parseErr != nil {
			return nil, parseErr
		}

		hasSwap, parseErr := requiredBool(p.rowNum, record, p.colIdx, "has_swap")
		if parseErr != nil {
			return nil, parseErr
		}

		hasThreadCount, parseErr := requiredBool(p.rowNum, record, p.colIdx, "has_thread_count")
		if parseErr != nil {
			return nil, parseErr
		}

		hasPIDCount, parseErr := requiredBool(p.rowNum, record, p.colIdx, "has_pid_count")
		if parseErr != nil {
			return nil, parseErr
		}

		hasFDCount, parseErr := requiredBool(p.rowNum, record, p.colIdx, "has_fd_count")
		if parseErr != nil {
			return nil, parseErr
		}

		hasSocketFDCount, parseErr := requiredBool(p.rowNum, record, p.colIdx, "has_socket_fd_count")
		if parseErr != nil {
			return nil, parseErr
		}

		hasVMACount, parseErr := requiredBool(p.rowNum, record, p.colIdx, "has_vma_count")
		if parseErr != nil {
			return nil, parseErr
		}

		// Validate availability consistency
		if !hasRSS && rssKiB != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "rss_kib", Err: fmt.Errorf("has_rss=false but value is %d", rssKiB)}
		}
		if !hasPSS && pssKiB != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "pss_kib", Err: fmt.Errorf("has_pss=false but value is %d", pssKiB)}
		}
		if !hasPSSAnon && pssAnonKiB != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "pss_anon_kib", Err: fmt.Errorf("has_pss_anon=false but value is %d", pssAnonKiB)}
		}
		if !hasPrivateDirty && privateDirtyKiB != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "private_dirty_kib", Err: fmt.Errorf("has_private_dirty=false but value is %d", privateDirtyKiB)}
		}
		if !hasAnonymous && anonymousKiB != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "anonymous_kib", Err: fmt.Errorf("has_anonymous=false but value is %d", anonymousKiB)}
		}
		if !hasSwap && swapKiB != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "swap_kib", Err: fmt.Errorf("has_swap=false but value is %d", swapKiB)}
		}
		if !hasThreadCount && threadCount != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "thread_count", Err: fmt.Errorf("has_thread_count=false but value is %d", threadCount)}
		}
		if !hasPIDCount && pidCount != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "pid_count", Err: fmt.Errorf("has_pid_count=false but value is %d", pidCount)}
		}
		if !hasFDCount && fdCount != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "fd_count", Err: fmt.Errorf("has_fd_count=false but value is %d", fdCount)}
		}
		if !hasSocketFDCount && socketFDCount != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "socket_fd_count", Err: fmt.Errorf("has_socket_fd_count=false but value is %d", socketFDCount)}
		}
		if !hasVMACount && vmaCount != 0 {
			return nil, &ParseError{Row: p.rowNum, Column: "vma_count", Err: fmt.Errorf("has_vma_count=false but value is %d", vmaCount)}
		}

		samples = append(samples, sampling.Sample{
			Sequence:               seq,
			Timestamp:              timestamp,
			PID:                    pid,
			ProcessStartTime:       startTime,
			Phase:                  phase,
			Delayed:                delayed,
			RSSKiB:                 rssKiB,
			PSSKiB:                 pssKiB,
			PSSAnonKiB:             pssAnonKiB,
			PrivateDirtyKiB:        privateDirtyKiB,
			AnonymousKiB:           anonymousKiB,
			SwapKiB:                swapKiB,
			DockerMemoryUsageBytes: dockerUsage,
			DockerMemoryLimitBytes: dockerLimit,
			HasDockerMemory:        hasDocker,
			VMACount:               vmaCount,
			FDCount:                fdCount,
			SocketFDCount:          socketFDCount,
			ThreadCount:            threadCount,
			PIDCount:               pidCount,
			CgroupAnonBytes:        cgroupAnonBytes,
			CgroupCurrentBytes:     cgroupCurrentBytes,
			CgroupMemoryStatAnon:   cgroupStatAnon,
			HasCgroupAnon:          hasCgroupAnon,
			HasCgroup:              hasCgroup,
			OOMEvents:              oomEvents,
			OOMKillEvents:          oomKillEvents,
			BGPState:               bgpState,
			BGPFSMTicks:            bgpFSMTicks,
			ReconnectCount:         reconnectCount,
			HasRSS:                 hasRSS,
			HasPSS:                 hasPSS,
			HasPSSAnon:             hasPSSAnon,
			HasPrivateDirty:        hasPrivateDirty,
			HasAnonymous:           hasAnonymous,
			HasSwap:                hasSwap,
			HasThreadCount:         hasThreadCount,
			HasPIDCount:            hasPIDCount,
			HasFDCount:             hasFDCount,
			HasSocketFDCount:       hasSocketFDCount,
			HasVMACount:            hasVMACount,
		})
	}

	if len(samples) == 0 {
		return nil, fmt.Errorf("CSV has no data rows")
	}

	// Final validation: sequence must start at 0
	if samples[0].Sequence != 0 {
		return nil, fmt.Errorf("sequence must start at 0, got %d", samples[0].Sequence)
	}

	return samples, nil
}
