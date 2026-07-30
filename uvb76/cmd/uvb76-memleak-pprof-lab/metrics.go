// Package main provides the UVB-76 pprof memory leak lab.
//
// # Process Metrics with Mandatory Field Presence
//
// This file implements P0-12: Mandatory procfs field presence authority.
// Every accepted process sample must have all mandatory fields observed and parsed.
// Missing fields must not become zero.
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

// mandatoryStatusFields lists all mandatory fields from /proc/<pid>/status.
var mandatoryStatusFields = []string{
	"VmRSS",
	"VmSize",
	"Threads",
}

// mandatorySmapsFields lists all mandatory fields from /proc/<pid>/smaps_rollup.
var mandatorySmapsFields = []string{
	"Pss",
	"Pss_Anon",
	"Private_Dirty",
	"Anonymous",
}

// fieldPresence tracks whether a mandatory field has been observed.
type fieldPresence struct {
	present   bool
	duplicate bool
	value     int64
}

// readStatusMetrics reads VmRSS, VmSize, and Threads from /proc/<pid>/status.
// P0-12: Tracks field presence explicitly.
func readStatusMetrics(pid int) (map[string]fieldPresence, *processMetrics, error) {
	statusPath := filepath.Join("/proc", strconv.Itoa(pid), "status")

	f, err := os.Open(statusPath)
	if err != nil {
		return nil, nil, SampleErrorf(pid, "open status", err)
	}
	defer f.Close()

	// Track field presence
	presence := make(map[string]fieldPresence)
	for _, field := range mandatoryStatusFields {
		presence[field] = fieldPresence{}
	}

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

		// Check if this is a mandatory field
		p, ok := presence[key]
		if !ok {
			continue
		}

		// P0-12: Detect duplicate fields
		if p.present {
			return nil, nil, SampleErrorf(pid, "%w: %s", ErrStatusFieldDuplicate, key)
		}

		switch key {
		case "VmRSS":
			v, err := parseMemValue(pid, "VmRSS", val)
			if err != nil {
				return nil, nil, SampleErrorf(pid, "%w: %s", ErrStatusFieldParse, key)
			}
			m.RSSKIB = v
			presence[key] = fieldPresence{present: true, value: v}
		case "VmSize":
			v, err := parseMemValue(pid, "VmSize", val)
			if err != nil {
				return nil, nil, SampleErrorf(pid, "%w: %s", ErrStatusFieldParse, key)
			}
			m.VMSizeKIB = v
			presence[key] = fieldPresence{present: true, value: v}
		case "Threads":
			v, err := parseIntValue(pid, "Threads", val)
			if err != nil {
				return nil, nil, SampleErrorf(pid, "%w: %s", ErrStatusFieldParse, key)
			}
			m.Threads = v
			presence[key] = fieldPresence{present: true, value: int64(v)}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, SampleErrorf(pid, "scan status", err)
	}

	return presence, m, nil
}

// readSmapsMetrics reads Pss, Pss_Anon, Private_Dirty, Anonymous from /proc/<pid>/smaps_rollup.
// P0-12: Tracks field presence explicitly.
func readSmapsMetrics(pid int) (map[string]fieldPresence, *processMetrics, error) {
	smapsPath := filepath.Join("/proc", strconv.Itoa(pid), "smaps_rollup")

	f, err := os.Open(smapsPath)
	if err != nil {
		// Fall back to smaps
		return readSmapsMetricsFallback(pid)
	}
	defer f.Close()

	// Track field presence
	presence := make(map[string]fieldPresence)
	for _, field := range mandatorySmapsFields {
		presence[field] = fieldPresence{}
	}

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

		// Check if this is a mandatory field
		p, ok := presence[key]
		if !ok {
			continue
		}

		// P0-12: Detect duplicate fields
		if p.present {
			return nil, nil, SampleErrorf(pid, "%w: %s", ErrSmapsFieldDuplicate, key)
		}

		switch key {
		case "Pss":
			v, err := parseMemValue(pid, "Pss", val)
			if err != nil {
				return nil, nil, SampleErrorf(pid, "%w: %s", ErrSmapsFieldParse, key)
			}
			m.PSS_KIB = v
			presence[key] = fieldPresence{present: true, value: v}
		case "Pss_Anon":
			v, err := parseMemValue(pid, "Pss_Anon", val)
			if err != nil {
				return nil, nil, SampleErrorf(pid, "%w: %s", ErrSmapsFieldParse, key)
			}
			m.PSSAnonKIB = v
			presence[key] = fieldPresence{present: true, value: v}
		case "Private_Dirty":
			v, err := parseMemValue(pid, "Private_Dirty", val)
			if err != nil {
				return nil, nil, SampleErrorf(pid, "%w: %s", ErrSmapsFieldParse, key)
			}
			m.PrivateDirtyKIB = v
			presence[key] = fieldPresence{present: true, value: v}
		case "Anonymous":
			v, err := parseMemValue(pid, "Anonymous", val)
			if err != nil {
				return nil, nil, SampleErrorf(pid, "%w: %s", ErrSmapsFieldParse, key)
			}
			m.AnonymousKIB = v
			presence[key] = fieldPresence{present: true, value: v}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, SampleErrorf(pid, "scan smaps_rollup", err)
	}

	return presence, m, nil
}

// readSmapsMetricsFallback reads Pss metrics from /proc/<pid>/smaps if smaps_rollup is not available.
func readSmapsMetricsFallback(pid int) (map[string]fieldPresence, *processMetrics, error) {
	smapsPath := filepath.Join("/proc", strconv.Itoa(pid), "smaps")

	f, err := os.Open(smapsPath)
	if err != nil {
		return nil, nil, SampleErrorf(pid, "open smaps", err)
	}
	defer f.Close()

	// Track field presence
	presence := make(map[string]fieldPresence)
	for _, field := range mandatorySmapsFields {
		presence[field] = fieldPresence{}
	}

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

		// Check if this is a mandatory field
		p, ok := presence[key]
		if !ok {
			continue
		}

		// P0-12: Detect duplicate fields
		if p.present {
			return nil, nil, SampleErrorf(pid, "%w: %s", ErrSmapsFieldDuplicate, key)
		}

		// Accumulate values for smaps (different from smaps_rollup)
		switch key {
		case "Pss":
			v, err := parseMemValue(pid, "Pss", val)
			if err != nil {
				return nil, nil, SampleErrorf(pid, "%w: %s", ErrSmapsFieldParse, key)
			}
			m.PSS_KIB += v
			presence[key] = fieldPresence{present: true, value: m.PSS_KIB}
		case "Pss_Anon":
			v, err := parseMemValue(pid, "Pss_Anon", val)
			if err != nil {
				return nil, nil, SampleErrorf(pid, "%w: %s", ErrSmapsFieldParse, key)
			}
			m.PSSAnonKIB += v
			presence[key] = fieldPresence{present: true, value: m.PSSAnonKIB}
		case "Private_Dirty":
			v, err := parseMemValue(pid, "Private_Dirty", val)
			if err != nil {
				return nil, nil, SampleErrorf(pid, "%w: %s", ErrSmapsFieldParse, key)
			}
			m.PrivateDirtyKIB += v
			presence[key] = fieldPresence{present: true, value: m.PrivateDirtyKIB}
		case "Anonymous":
			v, err := parseMemValue(pid, "Anonymous", val)
			if err != nil {
				return nil, nil, SampleErrorf(pid, "%w: %s", ErrSmapsFieldParse, key)
			}
			m.AnonymousKIB += v
			presence[key] = fieldPresence{present: true, value: m.AnonymousKIB}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, SampleErrorf(pid, "scan smaps", err)
	}

	return presence, m, nil
}

// countFDs counts the number of open file descriptors for a process.
func countFDs(pid int) (int, error) {
	fdDir := filepath.Join("/proc", strconv.Itoa(pid), "fd")

	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return 0, SampleErrorf(pid, "%w: read fd dir", ErrFDDirectoryRead, err)
	}

	return len(entries), nil
}

// sampleProcessMetricsWithPresence collects all process metrics with mandatory field presence.
// P0-12: Returns error if any mandatory field is absent.
func sampleProcessMetricsWithPresence(pid int) (*ProcessSample, error) {
	// Check if process still exists
	procPath := filepath.Join("/proc", strconv.Itoa(pid))
	if _, err := os.Stat(procPath); os.IsNotExist(err) {
		return nil, SampleErrorf(pid, "%w: process gone", ErrProcessDisappeared)
	}

	// Read status metrics with presence tracking
	statusPresence, status, err := readStatusMetrics(pid)
	if err != nil {
		return nil, err
	}

	// Verify all mandatory status fields present
	for _, field := range mandatoryStatusFields {
		p := statusPresence[field]
		if !p.present {
			return nil, SampleErrorf(pid, "%w: %s", ErrStatusFieldMissing, field)
		}
	}

	// Read smaps metrics with presence tracking
	smapsPresence, smaps, err := readSmapsMetrics(pid)
	if err != nil {
		return nil, err
	}

	// Verify all mandatory smaps fields present
	for _, field := range mandatorySmapsFields {
		p := smapsPresence[field]
		if !p.present {
			return nil, SampleErrorf(pid, "%w: %s", ErrSmapsFieldMissing, field)
		}
	}

	// Count open FDs
	fdCount, err := countFDs(pid)
	if err != nil {
		return nil, err
	}

	// Build ProcessSample
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

	// P0-12: Validate we got non-zero values for required fields
	// Note: Zero is valid, but all-zero indicates potential sampling failure
	if sample.RSSKIB == 0 && sample.VMSizeKIB == 0 && sample.Threads == 0 {
		return nil, SampleErrorf(pid, "all zero metrics - possible sampling failure")
	}

	return sample, nil
}

// sampleProcessMetricsFull collects all process metrics with strict error handling.
// P0-4: Returns error rather than silently appending all-zero sample.
func sampleProcessMetricsFull(pid int) (*ProcessSample, error) {
	return sampleProcessMetricsWithPresence(pid)
}

// parseMemValue parses memory values like "1234 kB" to KiB.
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
func parseIntValue(pid int, fieldName, s string) (int, error) {
	s = strings.TrimSpace(s)
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("parse %s value %q: %w", fieldName, s, err)
	}
	return v, nil
}
