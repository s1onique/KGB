// procfs.go — Native /proc memory reading for Linux
//
// Provides typed RSS/PSS reading from /proc/<pid>/smaps_rollup or /proc/<pid>/status.
// No shell pipelines, no grep/awk/sed.
//
// Reference: kgb://doctrine/native-owned-critical-paths

package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// MemorySnapshot represents a single memory measurement.
type MemorySnapshot struct {
	RSSKiB int64 `json:"rss_kib"`
	PSSKiB int64 `json:"pss_kib"`
}

// HasSmapsRollup reports whether smaps_rollup is available on this system.
func HasSmapsRollup() bool {
	_, err := os.Stat("/proc/self/smaps_rollup")
	return err == nil
}

// ReadMemorySnapshot reads RSS and PSS from /proc/<pid>.
// It prefers smaps_rollup for accurate PSS, falls back to status for RSS only.
func ReadMemorySnapshot(pid int) (MemorySnapshot, error) {
	// Try smaps_rollup first (most accurate)
	if smapsRollup, err := os.Open(procPath(pid, "smaps_rollup")); err == nil {
		defer smapsRollup.Close()
		snapshot, err := parseSmapsRollup(smapsRollup)
		if err == nil {
			return snapshot, nil
		}
	}

	// Fallback to status for RSS only
	if statusFile, err := os.Open(procPath(pid, "status")); err == nil {
		defer statusFile.Close()
		return parseStatusRSS(statusFile)
	}

	return MemorySnapshot{}, &ProcError{PID: pid, Op: "read"}
}

// parseSmapsRollup parses /proc/<pid>/smaps_rollup for Rss and Pss.
// Format:
//   Rss:                5120 kB
//   Pss:                4800 kB
func parseSmapsRollup(r *os.File) (MemorySnapshot, error) {
	var rss, pss int64
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Rss:") {
			rss = parseMemValue(line)
		} else if strings.HasPrefix(line, "Pss:") {
			pss = parseMemValue(line)
		}
	}
	if err := scanner.Err(); err != nil {
		return MemorySnapshot{}, err
	}
	return MemorySnapshot{RSSKiB: rss, PSSKiB: pss}, nil
}

// parseStatusRSS parses /proc/<pid>/status for VmRSS only (fallback).
// Format: VmRSS:     5120 kB
func parseStatusRSS(r *os.File) (MemorySnapshot, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			return MemorySnapshot{RSSKiB: parseMemValue(line), PSSKiB: 0}, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return MemorySnapshot{}, err
	}
	return MemorySnapshot{}, &ProcError{PID: 0, Op: "parse"}
}

// parseMemValue extracts the numeric value from lines like "VmRSS:     5120 kB".
// The value is in KiB (kB in /proc).
func parseMemValue(line string) int64 {
	parts := strings.Fields(line)
	if len(parts) >= 2 {
		val, err := strconv.ParseInt(parts[1], 10, 64)
		if err == nil {
			return val
		}
	}
	return 0
}

// procPath returns the path to a /proc file for the given PID.
func procPath(pid int, file string) string {
	return "/proc/" + strconv.Itoa(pid) + "/" + file
}

// ProcError represents a /proc access error.
type ProcError struct {
	PID int
	Op  string
}

func (e *ProcError) Error() string {
	return "procfs: failed to " + e.Op + " for pid " + strconv.Itoa(e.PID)
}