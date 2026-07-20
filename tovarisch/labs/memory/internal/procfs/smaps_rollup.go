// smaps_rollup.go — /proc/<pid>/smaps_rollup parsing
//
// Parses smaps_rollup for detailed memory accounting.
// Records RSS, PSS, Anonymous, Private_Clean/Dirty, Shared, Swap.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package procfs

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// SmapsRollup represents parsed /proc/<pid>/smaps_rollup data.
type SmapsRollup struct {
	RSSKiB          int64
	PSSKiB          int64
	PSSAnonKiB      int64
	PSSFileKiB      int64
	PSSShmemKiB     int64
	PrivateCleanKiB int64
	PrivateDirtyKiB int64
	SharedCleanKiB  int64
	SharedDirtyKiB  int64
	AnonymousKiB    int64
	SwapKiB         int64

	// Availability flags (true if field was present in the file)
	HasRSS          bool
	HasPSS          bool
	HasPSSAnon      bool
	HasPSSFile      bool
	HasPSSShmem     bool
	HasPrivateClean bool
	HasPrivateDirty bool
	HasSharedClean  bool
	HasSharedDirty  bool
	HasAnonymous    bool
	HasSwap         bool
}

// ReadSmapsRollup reads and parses /proc/<pid>/smaps_rollup.
func ReadSmapsRollup(pid int) (*SmapsRollup, error) {
	path := fmt.Sprintf("/proc/%d/smaps_rollup", pid)
	f, err := os.Open(path)
	if err != nil {
		return nil, &ProcError{PID: pid, Op: "open smaps_rollup", Err: err}
	}
	defer f.Close()

	return parseSmapsRollup(pid, f)
}

// parseSmapsRollup parses a smaps_rollup file.
func parseSmapsRollup(pid int, r *os.File) (*SmapsRollup, error) {
	scanner := bufio.NewScanner(r)
	smaps := &SmapsRollup{}

	for scanner.Scan() {
		line := scanner.Text()
		if err := parseSmapsLine(smaps, line); err != nil {
			return nil, &ProcError{PID: pid, Op: "parse smaps_rollup", Err: err}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, &ProcError{PID: pid, Op: "scan smaps_rollup", Err: err}
	}

	return smaps, nil
}

// parseSmapsLine parses a single line from smaps_rollup.
func parseSmapsLine(smaps *SmapsRollup, line string) error {
	// Format: "Metric:         12345 kB"
	colonIdx := strings.IndexByte(line, ':')
	if colonIdx < 0 {
		return nil // Skip malformed lines
	}

	key := strings.TrimSpace(line[:colonIdx])
	valueStr := strings.TrimSpace(line[colonIdx+1:])

	// Remove " kB" suffix
	valueStr = strings.TrimSuffix(valueStr, " kB")
	valueStr = strings.TrimSuffix(valueStr, "kB")

	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		return fmt.Errorf("parse value %q: %w", valueStr, err)
	}

	// Set availability flag and value
	switch key {
	case "Rss":
		smaps.RSSKiB = value
		smaps.HasRSS = true
	case "Pss":
		smaps.PSSKiB = value
		smaps.HasPSS = true
	case "Pss_Anon":
		smaps.PSSAnonKiB = value
		smaps.HasPSSAnon = true
	case "Pss_File":
		smaps.PSSFileKiB = value
		smaps.HasPSSFile = true
	case "Pss_Shmem":
		smaps.PSSShmemKiB = value
		smaps.HasPSSShmem = true
	case "Private_Clean":
		smaps.PrivateCleanKiB = value
		smaps.HasPrivateClean = true
	case "Private_Dirty":
		smaps.PrivateDirtyKiB = value
		smaps.HasPrivateDirty = true
	case "Shared_Clean":
		smaps.SharedCleanKiB = value
		smaps.HasSharedClean = true
	case "Shared_Dirty":
		smaps.SharedDirtyKiB = value
		smaps.HasSharedDirty = true
	case "Anonymous":
		smaps.AnonymousKiB = value
		smaps.HasAnonymous = true
	case "Swap":
		smaps.SwapKiB = value
		smaps.HasSwap = true
	}

	return nil
}

// MemorySample represents a complete memory sample from /proc.
type MemorySample struct {
	PID      int
	Sequence int
	Cgroup   *CgroupMemory
	Process  *SmapsRollup
	Resource *ResourceCounts
}

// ResourceCounts holds process resource counts.
type ResourceCounts struct {
	VMACount      int
	FDCount       int
	SocketFDCount int
	ThreadCount   int
}

// ReadResourceCounts reads resource counts for a PID.
func ReadResourceCounts(pid int) (*ResourceCounts, error) {
	r := &ResourceCounts{}

	vmaCount, err := ReadVMACount(pid)
	if err != nil {
		return nil, &ProcError{PID: pid, Op: "count VMAs", Err: err}
	}
	r.VMACount = vmaCount

	fdInfo, err := ReadFDCounts(pid)
	if err != nil {
		return nil, &ProcError{PID: pid, Op: "count FDs", Err: err}
	}
	r.FDCount = fdInfo.Total

	// Note: socket count derived from FD type analysis in ReadFDCounts
	r.SocketFDCount = fdInfo.Socket

	threadCount, err := ReadThreadCount(pid)
	if err != nil {
		return nil, &ProcError{PID: pid, Op: "count threads", Err: err}
	}
	r.ThreadCount = threadCount

	return r, nil
}
