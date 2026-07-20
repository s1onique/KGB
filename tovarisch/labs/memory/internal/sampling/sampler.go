// sampler.go — Memory sample orchestration
//
// Samples memory metrics from a process using procfs.
// Coordinates with phase state machine for timing.
// Guarantees immutability after Stop().
//
// Reference: kgb://doctrine/embedded-memory-frugality

package sampling

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs"
)

// Sample represents a single memory sample.
type Sample struct {
	Sequence         int       // Sample sequence number
	Timestamp        time.Time // Sample timestamp
	PID              int       // Process ID
	ProcessStartTime uint64    // Process start time (for identity)
	Phase            Phase     // Current phase
	Delayed          bool      // Sample was delayed

	// Primary memory signals (KiB)
	RSSKiB          int64
	PSSKiB          int64
	PSSAnonKiB      int64
	PrivateDirtyKiB int64
	AnonymousKiB    int64
	SwapKiB         int64

	// Resource signals
	VMACount      int
	FDCount       int
	SocketFDCount int
	ThreadCount   int
	PIDCount      int // cgroup pids.current

	// Cgroup memory
	CgroupAnonBytes    int64
	CgroupCurrentBytes int64

	// Semantic signals
	OOMEvents      int
	OOMKillEvents  int
	BGPState       string
	BGPFSMTicks    int64
	ReconnectCount int64

	// Availability flags
	HasRSS           bool
	HasPSS           bool
	HasPSSAnon       bool
	HasPrivateDirty  bool
	HasAnonymous     bool
	HasSwap          bool
	HasCgroup        bool
	HasThreadCount   bool
	HasPIDCount      bool
	HasFDCount       bool
	HasSocketFDCount bool
	HasVMACount      bool
}

// Event represents a sampling event.
type Event struct {
	Timestamp time.Time   `json:"timestamp"`
	Type      string      `json:"type"`
	Phase     Phase       `json:"phase,omitempty"`
	Message   string      `json:"message,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

// Sampler orchestrates memory sampling.
type Sampler struct {
	mu          sync.Mutex
	samples     []Sample
	events      []Event
	cfg         PhaseConfig
	phase       *PhaseState
	hostPIDFunc func() int // Function to get container's host PID
	docker      *dockerlab.Client
	cgroupPath  string
	stopCh      chan struct{}
	doneCh      chan struct{}
	running     bool
	stopped     bool // Once true, samples are immutable
}

// NewSampler creates a new sampler.
func NewSampler(hostPIDFunc func() int, cfg PhaseConfig) *Sampler {
	return &Sampler{
		cfg:         cfg,
		phase:       NewPhaseState(cfg),
		hostPIDFunc: hostPIDFunc,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}, 1),
	}
}

// NewSamplerWithDocker creates a new sampler with Docker client.
func NewSamplerWithDocker(hostPIDFunc func() int, docker *dockerlab.Client, cfg PhaseConfig) *Sampler {
	s := NewSampler(hostPIDFunc, cfg)
	s.docker = docker
	return s
}

// SetCgroupPath sets the cgroup path for container metrics.
func (s *Sampler) SetCgroupPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cgroupPath = path
}

// Start begins sampling.
func (s *Sampler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopped = false
	s.mu.Unlock()

	s.recordEvent("sampling_started", "Sampling loop started")

	go s.runLoop(ctx)
}

// Stop halts sampling and waits for completion.
// After Stop() returns, samples and events are immutable.
func (s *Sampler) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}

	// Signal stop
	close(s.stopCh)
	s.stopped = true
	s.running = false
	s.mu.Unlock()

	// Wait for goroutine to complete
	<-s.doneCh

	s.recordEvent("sampling_stopped", "Sampling loop stopped")
}

// Samples returns collected samples.
// Returns a copy to guarantee immutability.
func (s *Sampler) Samples() []Sample {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]Sample, len(s.samples))
	copy(result, s.samples)
	return result
}

// Events returns collected events.
// Returns a copy to guarantee immutability.
func (s *Sampler) Events() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]Event, len(s.events))
	copy(result, s.events)
	return result
}

// CurrentPhase returns the current phase.
func (s *Sampler) CurrentPhase() Phase {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.phase.Current
}

// IsStopped returns whether Stop() has been called.
func (s *Sampler) IsStopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

func (s *Sampler) recordEvent(eventType, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, Event{
		Timestamp: time.Now(),
		Type:      eventType,
		Phase:     s.phase.Current,
		Message:   message,
	})
}

func (s *Sampler) recordPhaseTransition(from, to Phase) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, Event{
		Timestamp: time.Now(),
		Type:      "phase_transition",
		Phase:     to,
		Message:   fmt.Sprintf("phase transition: %s -> %s", from, to),
		Data: map[string]string{
			"from": from.String(),
			"to":   to.String(),
		},
	})
}

func (s *Sampler) runLoop(ctx context.Context) {
	defer func() {
		s.doneCh <- struct{}{}
	}()

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	sequence := 0
	lastPhase := s.phase.Current

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			// Check phase transition
			s.phase.Update()
			currentPhase := s.phase.Current

			// Record phase transition
			if currentPhase != lastPhase {
				s.recordPhaseTransition(lastPhase, currentPhase)
				lastPhase = currentPhase
			}

			// Get host PID
			pid := s.hostPIDFunc()
			if pid <= 0 {
				continue
			}

			// Take sample
			sample := s.takeSample(ctx, pid, sequence, currentPhase)
			if sample != nil {
				s.mu.Lock()
				s.samples = append(s.samples, *sample)
				s.mu.Unlock()
				sequence++
			}
		}
	}
}

func (s *Sampler) takeSample(ctx context.Context, pid, seq int, phase Phase) *Sample {
	sample := &Sample{
		Sequence:  seq,
		Timestamp: time.Now(),
		PID:       pid,
		Phase:     phase,
	}

	// Read smaps_rollup
	smaps, err := procfs.ReadSmapsRollup(pid)
	if err != nil {
		// Check if zombie
		if procErr, ok := err.(*procfs.ProcError); ok && procErr.IsZombie() {
			return nil // Skip zombie
		}
		// Other errors: still record what we can
	} else {
		sample.RSSKiB = smaps.RSSKiB
		sample.PSSKiB = smaps.PSSKiB
		sample.PSSAnonKiB = smaps.PSSAnonKiB
		sample.PrivateDirtyKiB = smaps.PrivateDirtyKiB
		sample.AnonymousKiB = smaps.AnonymousKiB
		sample.SwapKiB = smaps.SwapKiB
		sample.HasRSS = smaps.HasRSS
		sample.HasPSS = smaps.HasPSS
		sample.HasPSSAnon = smaps.HasPSSAnon
		sample.HasPrivateDirty = smaps.HasPrivateDirty
		sample.HasAnonymous = smaps.HasAnonymous
		sample.HasSwap = smaps.HasSwap
	}

	// Read resource counts (includes thread count)
	resources, err := procfs.ReadResourceCounts(pid)
	if err == nil {
		sample.ThreadCount = resources.ThreadCount
		sample.HasThreadCount = true
	}

	// Read FD counts
	fds, err := procfs.ReadFDCounts(pid)
	if err == nil {
		sample.FDCount = fds.Total
		sample.SocketFDCount = fds.Socket
		sample.HasFDCount = true
		sample.HasSocketFDCount = true
	}

	// Read maps for VMA count
	vmas, err := procfs.ReadVMACount(pid)
	if err == nil {
		sample.VMACount = vmas
		sample.HasVMACount = true
	}

	// Read identity (for start time)
	identity, err := procfs.ReadIdentity(pid)
	if err == nil {
		sample.ProcessStartTime = identity.StartTime
	}

	// Read cgroup memory if path is set
	s.mu.Lock()
	cgroupPath := s.cgroupPath
	s.mu.Unlock()

	if cgroupPath != "" {
		cgroupMem, err := procfs.ReadCgroupMemory(cgroupPath)
		if err == nil && cgroupMem != nil {
			sample.CgroupAnonBytes = cgroupMem.MemoryStatAnonBytes
			sample.CgroupCurrentBytes = cgroupMem.MemoryCurrentBytes
			sample.HasCgroup = true
			sample.PIDCount = int(cgroupMem.PIDsCurrent)
			sample.HasPIDCount = true
			// Copy OOM counters from cgroup
			sample.OOMEvents = int(cgroupMem.MemoryEventsOOM)
			sample.OOMKillEvents = int(cgroupMem.MemoryEventsOOMKill)
		}
		// Note: If cgroup read fails, HasCgroup remains false
		// and this sample will be marked as unavailable in analysis
	}

	return sample
}

// CSVHeaders returns the CSV column headers.
func CSVHeaders() []string {
	return []string{
		"sequence",
		"timestamp",
		"process_pid",
		"process_start_time",
		"phase",
		"delayed",
		"rss_kib",
		"pss_kib",
		"pss_anon_kib",
		"private_dirty_kib",
		"anonymous_kib",
		"swap_kib",
		"vma_count",
		"fd_count",
		"socket_fd_count",
		"thread_count",
		"pid_count",
		"oom_events",
		"oom_kill_events",
		"bgp_state",
		"bgp_fsm_ticks",
		"reconnect_count",
		"cgroup_anon_bytes",
		"cgroup_current_bytes",
		"has_cgroup",
		"has_thread_count",
	}
}

// SampleForCSV converts a sample to CSV row.
func SampleForCSV(s *Sample) []string {
	return []string{
		fmt.Sprintf("%d", s.Sequence),
		s.Timestamp.Format(time.RFC3339Nano),
		fmt.Sprintf("%d", s.PID),
		fmt.Sprintf("%d", s.ProcessStartTime),
		s.Phase.String(),
		fmt.Sprintf("%t", s.Delayed),
		fmt.Sprintf("%d", s.RSSKiB),
		fmt.Sprintf("%d", s.PSSKiB),
		fmt.Sprintf("%d", s.PSSAnonKiB),
		fmt.Sprintf("%d", s.PrivateDirtyKiB),
		fmt.Sprintf("%d", s.AnonymousKiB),
		fmt.Sprintf("%d", s.SwapKiB),
		fmt.Sprintf("%d", s.VMACount),
		fmt.Sprintf("%d", s.FDCount),
		fmt.Sprintf("%d", s.SocketFDCount),
		fmt.Sprintf("%d", s.ThreadCount),
		fmt.Sprintf("%d", s.PIDCount),
		fmt.Sprintf("%d", s.OOMEvents),
		fmt.Sprintf("%d", s.OOMKillEvents),
		s.BGPState,
		fmt.Sprintf("%d", s.BGPFSMTicks),
		fmt.Sprintf("%d", s.ReconnectCount),
		fmt.Sprintf("%d", s.CgroupAnonBytes),
		fmt.Sprintf("%d", s.CgroupCurrentBytes),
		fmt.Sprintf("%t", s.HasCgroup),
		fmt.Sprintf("%t", s.HasThreadCount),
	}
}
