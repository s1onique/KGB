// cgroup_test.go — cgroup v2 parsing tests

package procfs

import (
	"testing"
)

func TestParseMountinfoLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantRoot string
		wantOK   bool
	}{
		{
			name:     "normal cgroup2 mount",
			line:     "22 21 0:20 / /sys/fs/cgroup/unified rw,nosuid,nodev,noexec,relatime shared:2 - cgroup2fs /sys/fs/cgroup/unified rw,seclabel",
			wantRoot: "/",
			wantOK:   true,
		},
		{
			name:     "Docker container cgroup2 mount",
			line:     "308 307 0:290 /docker/containers/abc123 /sys/fs/cgroup/unified rw,nosuid,nodev,noexec,relatime - cgroup2fs /sys/fs/cgroup/unified rw,seclabel",
			wantRoot: "/docker/containers/abc123",
			wantOK:   true,
		},
		{
			name:     "systemd Docker scope",
			line:     "456 455 0:390 /system.slice/docker-abc123.scope /sys/fs/cgroup/unified rw,nosuid,nodev,noexec,relatime - cgroup2fs /sys/fs/cgroup/unified rw,seclabel",
			wantRoot: "/system.slice/docker-abc123.scope",
			wantOK:   true,
		},
		{
			name:     "root cgroup",
			line:     "22 21 0:20 / /sys/fs/cgroup/unified rw,nosuid,nodev,noexec,relatime shared:2 - cgroup2fs /sys/fs/cgroup/unified rw,seclabel",
			wantRoot: "/",
			wantOK:   true,
		},
		{
			name:     "optional mountinfo fields with shared",
			line:     "22 21 0:20 / /sys/fs/cgroup/unified rw,nosuid,nodev,noexec,relatime shared:2 master:1 - cgroup2fs /sys/fs/cgroup/unified rw,seclabel",
			wantRoot: "/",
			wantOK:   true,
		},
		{
			name:     "optional mountinfo fields with propagation",
			line:     "22 21 0:20 / /sys/fs/cgroup/unified rw,nosuid,nodev,noexec,relatime shared:2 master:1 propagate_from:3 - cgroup2fs /sys/fs/cgroup/unified rw,seclabel",
			wantRoot: "/",
			wantOK:   true,
		},
		{
			name:     "non-cgroup2 filesystem",
			line:     "23 22 0:22 / /sys/fs/cgroup/memory rw,nosuid,nodev,noexec,relatime shared:3 - cgroup /sys/fs/cgroup/memory rw,seclabel",
			wantRoot: "",
			wantOK:   false,
		},
		{
			name:     "missing separator",
			line:     "22 21 0:20 / /sys/fs/cgroup/unified rw cgroup2fs /sys/fs/cgroup/unified rw",
			wantRoot: "",
			wantOK:   false,
		},
		{
			name:     "malformed line no fields",
			line:     "",
			wantRoot: "",
			wantOK:   false,
		},
		{
			name:     "malformed line missing mount point",
			line:     "22 21 0:20  - cgroup2fs /sys/fs/cgroup/unified rw",
			wantRoot: "",
			wantOK:   false,
		},
		{
			name:     "traversal attempt - not detected at mountinfo level",
			line:     "22 21 0:20 / /sys/fs/cgroup/unified rw,nosuid,nodev,noexec,relatime shared:2 - cgroup2fs /sys/fs/cgroup rw,seclabel",
			wantRoot: "/",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseMountinfoLine(tt.line)
			if ok != tt.wantOK {
				t.Errorf("parseMountinfoLine() ok = %v, want %v", ok, tt.wantOK)
				return
			}
			if ok && got != tt.wantRoot {
				t.Errorf("parseMountinfoLine() = %v, want %v", got, tt.wantRoot)
			}
		})
	}
}

func TestDecodeMountinfoPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "root path",
			path: "/",
			want: "/",
		},
		{
			name: "empty path",
			path: "",
			want: "/",
		},
		{
			name: "simple path",
			path: "/sys/fs/cgroup/unified",
			want: "/sys/fs/cgroup/unified",
		},
		{
			name: "docker container path",
			path: "/docker/containers/abc123",
			want: "/docker/containers/abc123",
		},
		{
			name: "systemd scope path",
			path: "/system.slice/docker-abc123.scope",
			want: "/system.slice/docker-abc123.scope",
		},
		{
			name: "escaped space",
			path: "/mount\\040point",
			want: "/mount point",
		},
		{
			name: "escaped tab",
			path: "/mount\\011point",
			want: "/mount\tpoint",
		},
		{
			name: "escaped backslash",
			path: "/path\\134with\\134backslashes",
			want: "/path\\with\\backslashes",
		},
		{
			name: "escaped newline",
			path: "/path\\012injected",
			want: "/path\ninjected",
		},
		{
			name: "mixed escapes",
			path: "/docker\\040containers/abc\\134123",
			want: "/docker containers/abc\\123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeMountinfoPath(tt.path)
			if got != tt.want {
				t.Errorf("decodeMountinfoPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseMemoryStatOrderIndependent(t *testing.T) {
	// Test that memory.stat keys are parsed correctly regardless of order
	tests := []struct {
		name   string
		input  string
		wantA  int64
		wantF  int64
		wantK  int64
		wantS  int64
		wantSk int64
	}{
		{
			name:   "normal order",
			input:  "anon 1000\nfile 2000\nkernel 3000\nslab 4000\nsock 5000",
			wantA:  1000,
			wantF:  2000,
			wantK:  3000,
			wantS:  4000,
			wantSk: 5000,
		},
		{
			name:   "reverse order",
			input:  "sock 5000\nslab 4000\nkernel 3000\nfile 2000\nanon 1000",
			wantA:  1000,
			wantF:  2000,
			wantK:  3000,
			wantS:  4000,
			wantSk: 5000,
		},
		{
			name:   "mixed order",
			input:  "file 2000\nanon 1000\nslab 4000\nsock 5000\nkernel 3000",
			wantA:  1000,
			wantF:  2000,
			wantK:  3000,
			wantS:  4000,
			wantSk: 5000,
		},
		{
			name:   "missing keys",
			input:  "anon 1000\nfile 2000",
			wantA:  1000,
			wantF:  2000,
			wantK:  0,
			wantS:  0,
			wantSk: 0,
		},
		{
			name:   "reordered memory stat keys from kernel",
			input:  "file 2000\nanon 1000\nkernel_stack 0\nslab 4000\nsock 5000\nfile_writeback 0\nfile_mlocked 0\nfile_fsync 0\nfile_pipe 0\nfile_shmem 0\nkernel_data 0\nkernel_code 0",
			wantA:  1000,
			wantF:  2000,
			wantK:  0,
			wantS:  4000,
			wantSk: 5000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &CgroupMemory{}
			parseMemoryStat(m, []byte(tt.input))
			if m.MemoryStatAnonBytes != tt.wantA {
				t.Errorf("MemoryStatAnonBytes = %d, want %d", m.MemoryStatAnonBytes, tt.wantA)
			}
			if m.MemoryStatFileBytes != tt.wantF {
				t.Errorf("MemoryStatFileBytes = %d, want %d", m.MemoryStatFileBytes, tt.wantF)
			}
			if m.MemoryStatKernelBytes != tt.wantK {
				t.Errorf("MemoryStatKernelBytes = %d, want %d", m.MemoryStatKernelBytes, tt.wantK)
			}
			if m.MemoryStatSlabBytes != tt.wantS {
				t.Errorf("MemoryStatSlabBytes = %d, want %d", m.MemoryStatSlabBytes, tt.wantS)
			}
			if m.MemoryStatSockBytes != tt.wantSk {
				t.Errorf("MemoryStatSockBytes = %d, want %d", m.MemoryStatSockBytes, tt.wantSk)
			}
		})
	}
}
