// Package main provides the UVB-76 pprof memory leak lab.
//
// # Process Metrics
//
// This file implements P0-4: Make process metrics truthful.
// It parses and writes all required fields from /proc:
// - VmRSS, VmSize, Threads from /proc/<pid>/status
// - Pss, Pss_Anon, Private_Dirty, Anonymous from /proc/<pid>/smaps_rollup
// - Open FD count from /proc/<pid>/fd
//
// Returns a sampling error when required procfs files disappear or cannot be parsed.
// Does not silently append all-zero samples.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ProcessMetricsError represents a sampling error that should stop collection.
type ProcessMetricsError struct {
	PID  int
	What string
	Err  error
}

func (e *ProcessMetricsError) Error() string {
	return fmt.Sprintf("process metrics error for PID %d: %s: %v", e.PID, e.What, e.Err)
}

func (e *ProcessMetricsError) Unwrap() error {
	return e.Err
}

// SampleErrorf creates a ProcessMetricsError for a PID.
func SampleErrorf(pid int, what string, args ...interface{}) *ProcessMetricsError {
	var err error
	if len(args) > 0 {
		if e, ok := args[len(args)-1].(error); ok {
			err = e
			args = args[:len(args)-1]
		}
	}
	return &ProcessMetricsError{
		PID:  pid,
		What: fmt.Sprintf(what, args...),
		Err:  err,
	}
}

// processMetrics represents all collected process metrics.
type processMetrics struct {
	RSSKIB          int64
	VMSizeKIB       int64
	PSS_KIB         int64
	PSSAnonKIB      int64
	PrivateDirtyKIB int64
	AnonymousKIB    int64
	Threads         int
	FDCount         int
}

// readStatusMetrics reads VmRSS, VmSize, and Threads from /proc/<pid>/status.
// P0-3: Returns parse errors instead of silently ignoring failures.
func readStatusMetrics(pid int) (*processMetrics, error) {
	statusPath := filepath.Join("/proc", strconv.Itoa(pid), "status")

	f, err := os.Open(statusPath)
	if err != nil {
		return nil, SampleErrorf(pid, "open status", err)
	}
	defer f.Close()

	m := &processMetrics{}
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		// Parse key: value format
		// VmRSS:     1234 kB
		// Threads:   12
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		switch key {
		case "VmRSS":
			v, err := parseMemValue(pid, "VmRSS", val)
			if err != nil {
				return nil, SampleErrorf(pid, "parse VmRSS", err)
			}
			m.RSSKIB = v
		case "VmSize":
			v, err := parseMemValue(pid, "VmSize", val)
			if err != nil {
				return nil, SampleErrorf(pid, "parse VmSize", err)
			}
			m.VMSizeKIB = v
		case "Threads":
			v, err := parseIntValue(pid, "Threads", val)
			if err != nil {
				return nil, SampleErrorf(pid, "parse Threads", err)
			}
			m.Threads = v
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, SampleErrorf(pid, "scan status", err)
	}

	return m, nil
}

// readSmapsMetrics reads Pss, Pss_Anon, Private_Dirty, Anonymous from /proc/<pid>/smaps_rollup.
// P0-3: Returns parse errors instead of silently ignoring failures.
func readSmapsMetrics(pid int) (*processMetrics, error) {
	smapsPath := filepath.Join("/proc", strconv.Itoa(pid), "smaps_rollup")

	f, err := os.Open(smapsPath)
	if err != nil {
		// If smaps_rollup is not available, fall back to smaps
		return readSmapsMetricsFallback(pid)
	}
	defer f.Close()

	m := &processMetrics{}
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		// Parse key: value format
		// Pss:            1234 kB
		// Pss_Anon:       1000 kB
		// Private_Dirty:    500 kB
		// Anonymous:        300 kB
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		switch key {
		case "Pss":
			v, err := parseMemValue(pid, "Pss", val)
			if err != nil {
				return nil, SampleErrorf(pid, "parse Pss", err)
			}
			m.PSS_KIB = v
		case "Pss_Anon":
			v, err := parseMemValue(pid, "Pss_Anon", val)
			if err != nil {
				return nil, SampleErrorf(pid, "parse Pss_Anon", err)
			}
			m.PSSAnonKIB = v
		case "Private_Dirty":
			v, err := parseMemValue(pid, "Private_Dirty", val)
			if err != nil {
				return nil, SampleErrorf(pid, "parse Private_Dirty", err)
			}
			m.PrivateDirtyKIB = v
		case "Anonymous":
			v, err := parseMemValue(pid, "Anonymous", val)
			if err != nil {
				return nil, SampleErrorf(pid, "parse Anonymous", err)
			}
			m.AnonymousKIB = v
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, SampleErrorf(pid, "scan smaps_rollup", err)
	}

	return m, nil
}

// readSmapsMetricsFallback reads Pss metrics from /proc/<pid>/smaps if smaps_rollup is not available.
// P0-3: Returns parse errors instead of silently ignoring failures.
func readSmapsMetricsFallback(pid int) (*processMetrics, error) {
	smapsPath := filepath.Join("/proc", strconv.Itoa(pid), "smaps")

	f, err := os.Open(smapsPath)
	if err != nil {
		return nil, SampleErrorf(pid, "open smaps", err)
	}
	defer f.Close()

	m := &processMetrics{}
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		switch key {
		case "Pss":
			v, err := parseMemValue(pid, "Pss", val)
			if err != nil {
				return nil, SampleErrorf(pid, "parse Pss", err)
			}
			m.PSS_KIB += v
		case "Pss_Anon":
			v, err := parseMemValue(pid, "Pss_Anon", val)
			if err != nil {
				return nil, SampleErrorf(pid, "parse Pss_Anon", err)
			}
			m.PSSAnonKIB += v
		case "Private_Dirty":
			v, err := parseMemValue(pid, "Private_Dirty", val)
			if err != nil {
				return nil, SampleErrorf(pid, "parse Private_Dirty", err)
			}
			m.PrivateDirtyKIB += v
		case "Anonymous":
			v, err := parseMemValue(pid, "Anonymous", val)
			if err != nil {
				return nil, SampleErrorf(pid, "parse Anonymous", err)
			}
			m.AnonymousKIB += v
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, SampleErrorf(pid, "scan smaps", err)
	}

	return m, nil
}

// countFDs counts the number of open file descriptors for a process.
func countFDs(pid int) (int, error) {
	fdDir := filepath.Join("/proc", strconv.Itoa(pid), "fd")

	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return 0, SampleErrorf(pid, "read fd dir", err)
	}

	return len(entries), nil
}

// sampleProcessMetricsFull collects all process metrics with strict error handling.
// P0-4: Returns error rather than silently appending all-zero sample.
func sampleProcessMetricsFull(pid int) (*ProcessSample, error) {
	// Check if process still exists
	procPath := filepath.Join("/proc", strconv.Itoa(pid))
	if _, err := os.Stat(procPath); os.IsNotExist(err) {
		return nil, SampleErrorf(pid, "process gone")
	}

	// Read status metrics (VmRSS, VmSize, Threads)
	status, err := readStatusMetrics(pid)
	if err != nil {
		return nil, err
	}

	// Read smaps_rollup metrics (Pss, Pss_Anon, Private_Dirty, Anonymous)
	smaps, err := readSmapsMetrics(pid)
	if err != nil {
		return nil, err
	}

	// Count open FDs
	fdCount, err := countFDs(pid)
	if err != nil {
		return nil, err
	}

	// Build ProcessSample with PID and Timestamp
	now := time.Now()
	sample := &ProcessSample{
		PID:             pid,
		Timestamp:       now,
		RSSKIB:          status.RSSKIB,
		VMSizeKIB:       status.VMSizeKIB,
		PSS_KIB:         smaps.PSS_KIB,
		PSSAnonKIB:      smaps.PSSAnonKIB,
		PrivateDirtyKIB: smaps.PrivateDirtyKIB,
		AnonymousKIB:    smaps.AnonymousKIB,
		Threads:         status.Threads,
		FDCount:         fdCount,
	}

	// Validate we got non-zero values for required fields
	// If all are zero, the sampling may have failed silently
	if sample.RSSKIB == 0 && sample.VMSizeKIB == 0 && sample.Threads == 0 {
		return nil, SampleErrorf(pid, "all zero metrics - possible sampling failure")
	}

	return sample, nil
}

// parseMemValue parses memory values like "1234 kB" to KiB.
// P0-3: Returns error instead of silently discarding parse failures.
func parseMemValue(pid int, fieldName, s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "kB", "")
	s = strings.ReplaceAll(s, "KB", "")
	s = strings.TrimSpace(s)
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s value %q: %w", fieldName, s, err)
	}
	return v, nil
}

// parseIntValue parses integer values.
// P0-3: Returns error instead of silently discarding parse failures.
func parseIntValue(pid int, fieldName, s string) (int, error) {
	s = strings.TrimSpace(s)
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("parse %s value %q: %w", fieldName, s, err)
	}
	return v, nil
}
