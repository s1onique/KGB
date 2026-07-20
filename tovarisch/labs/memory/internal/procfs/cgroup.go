// cgroup.go — cgroup v2 memory and PID accounting via procfs
//
// Reads cgroup v2 memory statistics and PID counts for container isolation.
// Provides: CgroupMemory, ReadCgroupMemory
//
// Reference: kgb://doctrine/embedded-memory-frugality

package procfs

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// CgroupMemory represents cgroup v2 memory statistics.
type CgroupMemory struct {
	MemoryCurrentBytes    int64
	MemoryPeakBytes       int64
	MemoryEventsOOM       int64
	MemoryEventsOOMKill   int64
	MemoryStatAnonBytes   int64
	MemoryStatFileBytes   int64
	MemoryStatKernelBytes int64
	MemoryStatSockBytes   int64
	MemoryStatSlabBytes   int64
	PIDsCurrent           int64
}

// ReadCgroupMemory reads cgroup v2 memory stats for a container.
func ReadCgroupMemory(cgroupPath string) (*CgroupMemory, error) {
	m := &CgroupMemory{}

	// memory.current
	if val, err := readCgroupValue(cgroupPath, "memory.current"); err == nil {
		m.MemoryCurrentBytes = val
	}

	// memory.peak
	if val, err := readCgroupValue(cgroupPath, "memory.peak"); err == nil {
		m.MemoryPeakBytes = val
	}

	// memory.events
	if data, err := os.ReadFile(cgroupPath + "/memory.events"); err == nil {
		parseMemoryEvents(m, data)
	}

	// memory.stat
	if data, err := os.ReadFile(cgroupPath + "/memory.stat"); err == nil {
		parseMemoryStat(m, data)
	}

	// pids.current
	if val, err := readCgroupValue(cgroupPath, "pids.current"); err == nil {
		m.PIDsCurrent = val
	}

	return m, nil
}

// readCgroupValue reads a single cgroup value file.
func readCgroupValue(cgroupPath, name string) (int64, error) {
	data, err := os.ReadFile(cgroupPath + "/" + name)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
}

// parseMemoryEvents parses memory.events file.
func parseMemoryEvents(m *CgroupMemory, data []byte) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "oom":
			m.MemoryEventsOOM = val
		case "oom_kill":
			m.MemoryEventsOOMKill = val
		}
	}
}

// parseMemoryStat parses memory.stat file (key-based, order-independent).
func parseMemoryStat(m *CgroupMemory, data []byte) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "anon":
			m.MemoryStatAnonBytes = val
		case "file":
			m.MemoryStatFileBytes = val
		case "kernel":
			m.MemoryStatKernelBytes = val
		case "sock":
			m.MemoryStatSockBytes = val
		case "slab":
			m.MemoryStatSlabBytes = val
		}
	}
}
