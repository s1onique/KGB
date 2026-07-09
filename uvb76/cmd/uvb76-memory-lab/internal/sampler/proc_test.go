package sampler

import (
	"strings"
	"testing"
	"time"
)

const fixtureProcStatus = `Name:	uvb76
State:	S (sleeping)
Tgid:	12345
Ngid:	0
Pid:	12345
PPid:	1
TracerPid:	0
Uid:	1000	1000	1000	1000
Gid:	1000	1000	1000	1000
FDSize:	64
Groups:	1000
VmPeak:	  524288 kB
VmSize:	  262144 kB
VmLck:	       0 kB
VmPin:	       0 kB
VmHWM:	   51200 kB
VmRSS:	   40960 kB
VmData:	  204800 kB
VmStk:	     136 kB
VmExe:	    8192 kB
VmLib:	   16384 kB
VmPTE:	     160 kB
VmSwap:	       0 kB
Threads:	12
SigQ:	0/127988
SigPnd:	0000000000000000
ShdPnd:	0000000000000000
SigBlk:	0000000000000000
SigIgn:	0000000000001000
SigCgt:	00000000000080cb
CapInh:	0000000000000000
CapPrm:	0000000000000000
CapEff:	0000000000000000
CapBnd:	0000000000000000
CapAmb:	0000000000000000
NoNewPrivs:	0
Seccomp:	0
Seccomp_filters:	0
Speculation_Store_Bypass:	unknown
Cpus_allowed:	ff
Cpus_allowed_list:	0-7
Mems_allowed:	00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000000,00000001
Mems_allowed_list:	0
voluntary_ctxt_switches:	100
nonvoluntary_ctxt_switches:	50
`

func TestParseKBytes(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
		wantErr  bool
	}{
		{"40960 kB", 40960, false},
		{"262144 kB", 262144, false},
		{"0 kB", 0, false},
		{"100", 100, false},
		{"", 0, true},
		{"invalid", 0, true},
		{"-100 kB", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseKBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseKBytes(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("parseKBytes(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseProcStatusReaderValid(t *testing.T) {
	r := strings.NewReader(fixtureProcStatus)
	rss, vsz, threads, err := ParseProcStatusReader(r)
	if err != nil {
		t.Fatalf("ParseProcStatusReader failed: %v", err)
	}

	// RSS should be 40960 kB = 41943040 bytes
	if rss != 41943040 {
		t.Errorf("Expected RSS=41943040, got %d", rss)
	}

	// VSZ should be 262144 kB = 268435456 bytes
	if vsz != 268435456 {
		t.Errorf("Expected VSZ=268435456, got %d", vsz)
	}

	// Threads should be 12
	if threads != 12 {
		t.Errorf("Expected Threads=12, got %d", threads)
	}
}

func TestParseProcStatusReaderMissingVmRSS(t *testing.T) {
	status := `Name:	test
VmSize:	  102400 kB
Threads:	5
`
	r := strings.NewReader(status)
	_, _, _, err := ParseProcStatusReader(r)
	if err == nil {
		t.Error("Expected error for missing VmRSS")
	}
}

func TestParseProcStatusReaderMissingVmSize(t *testing.T) {
	status := `Name:	test
VmRSS:	   40960 kB
Threads:	5
`
	r := strings.NewReader(status)
	_, _, _, err := ParseProcStatusReader(r)
	if err == nil {
		t.Error("Expected error for missing VmSize")
	}
}

func TestParseProcStatusReaderMissingThreads(t *testing.T) {
	status := `Name:	test
VmRSS:	   40960 kB
VmSize:	  102400 kB
`
	r := strings.NewReader(status)
	_, _, _, err := ParseProcStatusReader(r)
	if err == nil {
		t.Error("Expected error for missing Threads")
	}
}

func TestParseProcStatusReaderInvalidNumeric(t *testing.T) {
	status := `Name:	test
VmRSS:	   invalid kB
VmSize:	  102400 kB
Threads:	5
`
	r := strings.NewReader(status)
	_, _, _, err := ParseProcStatusReader(r)
	if err == nil {
		t.Error("Expected error for invalid VmRSS value")
	}
}

func TestParseProcStatusReaderInvalidThreads(t *testing.T) {
	status := `Name:	test
VmRSS:	   40960 kB
VmSize:	  102400 kB
Threads:	notanumber
`
	r := strings.NewReader(status)
	_, _, _, err := ParseProcStatusReader(r)
	if err == nil {
		t.Error("Expected error for invalid Threads value")
	}
}

func TestProcSampleToCSV(t *testing.T) {
	start := time.Now()
	sample := &ProcSample{
		Timestamp: start.Add(30 * time.Second),
		PID:       12345,
		RSSBytes:  50 * 1024 * 1024, // 50 MB
		VSZBytes:  200 * 1024 * 1024,
		Threads:   8,
		FDCount:   42,
	}

	csv := ProcSampleToCSV(sample, start)
	expected := "30.0,12345,52428800,209715200,8,42"
	if csv != expected {
		t.Errorf("Expected CSV=%q, got %q", expected, csv)
	}
}

func TestProcSampleCSVHeader(t *testing.T) {
	header := ProcSampleCSVHeader()
	expected := "elapsed_seconds,pid,rss_bytes,vsz_bytes,threads,fd_count"
	if header != expected {
		t.Errorf("Expected header=%q, got %q", expected, header)
	}
}
