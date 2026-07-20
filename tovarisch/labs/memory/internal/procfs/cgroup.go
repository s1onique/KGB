// cgroup.go — cgroup v2 memory and PID accounting via procfs
//
// Reads cgroup v2 memory statistics and PID counts for container isolation.
// Provides: CgroupMemory, ReadCgroupMemory, ResolveCgroupV2Path
//
// Reference: kgb://doctrine/embedded-memory-frugality

package procfs

import (
	"bufio"
	"os"
	"path/filepath"
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

// NamespaceInfo captures namespace identities for mismatch detection.
type NamespaceInfo struct {
	MountNamespace string
	CgroupNamespace string
}

// ReadNamespaceIDs reads namespace symlink targets for a PID.
// This enables comparing whether two PIDs share the same namespace.
func ReadNamespaceIDs(pid int) (*NamespaceInfo, error) {
	ns := &NamespaceInfo{}

	// Read mount namespace
	if target, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "ns", "mnt")); err == nil {
		ns.MountNamespace = target
	}

	// Read cgroup namespace
	if target, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "ns", "cgroup")); err == nil {
		ns.CgroupNamespace = target
	}

	if ns.MountNamespace == "" && ns.CgroupNamespace == "" {
		return nil, os.ErrNotExist
	}
	return ns, nil
}

// ResolveCgroupV2Path resolves the cgroup v2 path for a given PID.
// It reads /proc/<pid>/cgroup to find the unified hierarchy record,
// discovers the cgroup2 mount from /proc/self/mountinfo,
// and combines them to produce the absolute cgroup path.
func ResolveCgroupV2Path(pid int) (string, error) {
	// Read /proc/<pid>/cgroup
	cgroupFile := filepath.Join("/proc", strconv.Itoa(pid), "cgroup")
	data, err := os.ReadFile(cgroupFile)
	if err != nil {
		return "", err
	}

	// Find the unified hierarchy record (format: "0::/<relative-path>")
	var relPath string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		// Unified hierarchy format: hierarchy ID:controllers:control_group_path
		// For cgroup v2, controllers field may be empty (just colons)
		if strings.HasPrefix(line, "0::") || strings.HasPrefix(line, "0:") {
			parts := strings.SplitN(line, ":", 3)
			if len(parts) >= 3 {
				relPath = strings.TrimPrefix(parts[2], "/")
				break
			}
		}
	}

	if relPath == "" {
		return "", &ProcError{PID: pid, Op: "resolve cgroup", Err: ErrNoUnifiedCgroup}
	}

	// Find cgroup2 mount from /proc/self/mountinfo
	mountRoot, err := findCgroup2MountRoot()
	if err != nil {
		return "", err
	}

	// Combine mount root and relative cgroup path
	cgroupPath := filepath.Join(mountRoot, relPath)
	cgroupPath = filepath.Clean(cgroupPath)

	// Use filepath.Rel to verify containment: if relPath is valid, Rel will succeed
	// and the result will be clean (no ".." that escape mountRoot)
	rel, err := filepath.Rel(mountRoot, cgroupPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", &ProcError{PID: pid, Op: "resolve cgroup", Err: ErrPathTraversal}
	}

	// Verify the directory exists
	if _, err := os.Stat(cgroupPath); err != nil {
		return "", &ProcError{PID: pid, Op: "resolve cgroup", Err: err}
	}

	return cgroupPath, nil
}

// findCgroup2MountRoot reads /proc/self/mountinfo to find the cgroup2 mount root.
// Mountinfo format:
//   mount_id parent_id major:minor mount_point root_of_mount options [optional fields] - fs_type source super_options
//   field 5 = mount_point (index 4)
//   field after "-" separator = fs_type
//   field 4 = root_of_mount (index 3)
func findCgroup2MountRoot() (string, error) {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		mountRoot, ok := parseMountinfoLine(line)
		if ok {
			return mountRoot, nil
		}
	}

	return "", &ProcError{PID: 0, Op: "find cgroup2 mount", Err: ErrNoCgroup2Mount}
}

// parseMountinfoLine parses a single mountinfo line and returns the cgroup2 mount root.
// Returns (mountRoot, ok).
func parseMountinfoLine(line string) (string, bool) {
	// Find the "-" separator that separates mount info from filesystem info
	separatorIdx := strings.Index(line, " - ")
	if separatorIdx == -1 {
		return "", false
	}

	// Everything before "-" is the mount info (fields 1-6, plus optional fields)
	mountInfo := line[:separatorIdx]
	// Everything after "-" is filesystem info (fs_type source super_options)
	fsInfo := strings.TrimSpace(line[separatorIdx+3:])

	// Parse filesystem info: fs_type source [super_options]
	fsParts := strings.Fields(fsInfo)
	if len(fsParts) < 2 {
		return "", false
	}

	// Check if this is cgroup2fs
	if fsParts[0] != "cgroup2fs" {
		return "", false
	}

	// Parse mount info: mount_id parent_id major:minor mount_point root_of_mount options
	mountParts := strings.Fields(mountInfo)
	if len(mountParts) < 5 {
		return "", false
	}

	// field 5 = mount_point (index 4)
	// field 4 = root_of_mount (index 3)
	// Use root_of_mount as it's more precise for containment checks
	rootOfMount := decodeMountinfoPath(mountParts[3])
	mountPoint := decodeMountinfoPath(mountParts[4])

	// Return root_of_mount if non-empty, otherwise mount_point
	if rootOfMount != "" {
		return rootOfMount, true
	}
	return mountPoint, true
}

// decodeMountinfoPath decodes octal-escaped mountinfo paths.
// Mountinfo uses octal escapes for special characters:
//   \040 = space
//   \011 = tab
//   \012 = newline
//   \134 = backslash
func decodeMountinfoPath(path string) string {
	// Handle empty or root path
	if path == "" || path == "/" {
		return "/"
	}

	// Decode octal escapes
	var result strings.Builder
	i := 0
	for i < len(path) {
		if path[i] == '\\' && i+3 < len(path) {
			// Octal escape: \NNN
			oct := path[i+1 : i+4]
			val, err := strconv.ParseInt(oct, 8, 64)
			if err == nil {
				result.WriteByte(byte(val))
				i += 4
				continue
			}
		}
		result.WriteByte(path[i])
		i++
	}

	decoded := result.String()
	// Handle special case for root unified record "0::/"
	if decoded == "" {
		return "/"
	}
	return decoded
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
