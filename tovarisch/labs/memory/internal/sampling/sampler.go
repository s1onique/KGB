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
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs"
)

// Sentinel errors for waiter termination
var (
	ErrSamplerStopped  = errors.New("sampler stopped")
	ErrContextCanceled = errors.New("context canceled")
	ErrSamplingFailed  = errors.New("sampling failed")
	ErrComplete        = errors.New("sampling complete")
)

// phaseRank returns the lifecycle rank of a phase.
// Used for ordering comparisons instead of lexical string comparison.
func phaseRank(p Phase) int {
	switch p {
	case PhaseStartup:
		return 0
	case PhaseWarmup:
		return 1
	case PhaseBaseline:
		return 2
	case PhaseStimulus:
		return 3
	case PhaseSettling:
		return 4
	case PhaseFinal:
		return 5
	case PhaseComplete:
		return 6
	default:
		return -1
	}
}

// Sample represents a single memory sample.
type Sample struct {
	Sequence         int       // Sample sequence number
	Timestamp        time.Time // Sample timestamp
	PID              int       // Process ID
	ProcessStartTime uint64    // Process start time (for identity)
	Phase            Phase     // Current phase
	Delayed          bool      // Sample was delayed

	// Primary memory signals (KiB) - from procfs
	RSSKiB          int64
	PSSKiB          int64
	PSSAnonKiB      int64
	PrivateDirtyKiB int64
	AnonymousKiB    int64
	SwapKiB         int64

	// Docker container memory (EXCLUSIVE - no projection to procfs fields)
	DockerMemoryUsageBytes int64
	DockerMemoryLimitBytes int64
	HasDockerMemory        bool

	// Resource signals
	VMACount      int
	FDCount       int
	SocketFDCount int
	ThreadCount   int
	PIDCount      int // cgroup pids.current

	// Cgroup memory (from real cgroup v2 files)
	CgroupAnonBytes      int64
	CgroupCurrentBytes   int64
	CgroupMemoryStatAnon int64
	HasCgroupAnon        bool

	// Semantic signals
	OOMEvents      int
	OOMKillEvents  int
	BGPState       string
	BGPFSMTicks    int64
	ReconnectCount int64

	// Availability flags - explicit, never false-positive
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

// waiter holds a registration for a phase waiter.
type waiter struct {
	target Phase
	result chan error
}

// terminalState represents the terminal state of the sampler.
// Explicitly separates "not terminated" from "successful termination".
type terminalState struct {
	set bool
	err error
}

// Sampler orchestrates memory sampling.
type Sampler struct {
	mu          sync.Mutex
	samples     []Sample
	events      []Event
	cfg         PhaseConfig
	phase       *PhaseState
	hostPIDFunc func() int // Function to get container's host PID
	containerID string     // Container ID for Docker stats API
	docker      *dockerlab.Client
	cgroupPath  string
	stopCh      chan struct{}
	doneCh      chan struct{}
	running     bool
	stopped     bool // Once true, samples are immutable

	// Centralized phase notification state
	stimulusCh       chan struct{} // Closed exactly once when stimulus begins
	stimulusNotified bool          // Guards against double-close
	waiters          []*waiter     // All registered waiters
	terminal         *terminalState // nil = not terminated, set = terminated (nil err = complete)
}

// NewSampler creates a new sampler.
func NewSampler(hostPIDFunc func() int, cfg PhaseConfig) *Sampler {
	return &Sampler{
		cfg:         cfg,
		phase:       NewPhaseState(cfg),
		hostPIDFunc: hostPIDFunc,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}, 1),
		waiters:     make([]*waiter, 0),
		stimulusCh:  make(chan struct{}),
	}
}

// NewSamplerWithDocker creates a new sampler with Docker container stats support.
func NewSamplerWithDocker(containerID string, hostPIDFunc func() int, docker *dockerlab.Client, cfg PhaseConfig) *Sampler {
	return &Sampler{
		cfg:         cfg,
		phase:       NewPhaseState(cfg),
		hostPIDFunc: hostPIDFunc,
		containerID: containerID,
		docker:      docker,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}, 1),
		waiters:     make([]*waiter, 0),
		stimulusCh:  make(chan struct{}),
	}
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

	s.recordEventLocked("sampling_started", "Sampling loop started")

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

// StimulusReady returns a channel that is closed exactly once when stimulus phase begins.
func (s *Sampler) StimulusReady() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	// If already notified or past stimulus, close immediately
	if s.stimulusNotified || phaseRank(s.phase.Current) >= phaseRank(PhaseStimulus) {
		if !s.stimulusNotified {
			s.stimulusNotified = true
			close(s.stimulusCh)
		}
	}

	return s.stimulusCh
}

// WaitForPhase blocks until the specified phase is reached or an error occurs.
// Returns nil on target reached, or error if stopped, context canceled, or sampling failed.
// Preserves context error types: returns ctx.Err() directly on cancellation/deadline.
func (s *Sampler) WaitForPhase(ctx context.Context, target Phase) error {
	// Fast path: already at or past target
	s.mu.Lock()
	if phaseRank(s.phase.Current) >= phaseRank(target) {
		s.mu.Unlock()
		return nil
	}
	// Check terminal result
	if s.terminal != nil {
		s.mu.Unlock()
		return s.terminal.err
	}
	s.mu.Unlock()

	// Register waiter
	w := &waiter{
		target: target,
		result: make(chan error, 1),
	}

	s.mu.Lock()
	// Double-check after acquiring lock
	if phaseRank(s.phase.Current) >= phaseRank(target) {
		s.mu.Unlock()
		return nil
	}
	if s.terminal != nil {
		s.mu.Unlock()
		return s.terminal.err
	}
	s.waiters = append(s.waiters, w)
	s.mu.Unlock()

	// Wait for result
	select {
	case <-ctx.Done():
		// Remove this waiter from the list
		s.mu.Lock()
		s.removeWaiterLocked(w)
		s.mu.Unlock()
		return ctx.Err() // Preserve exact context error type
	case err := <-w.result:
		return err
	}
}

// WaitForComplete blocks until the Complete phase is reached or an error occurs.
// Returns nil for both active and late callers after normal completion.
func (s *Sampler) WaitForComplete(ctx context.Context) error {
	// Check if we're already complete (including late callers after normal completion)
	s.mu.Lock()
	if s.phase.Current == PhaseComplete {
		s.mu.Unlock()
		return nil
	}
	// If terminal but not complete, return the error
	if s.terminal != nil {
		err := s.terminal.err
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	return s.WaitForPhase(ctx, PhaseComplete)
}

// advanceAndPublishLocked performs atomic phase transition under mutex.
// Must be called with s.mu held.
func (s *Sampler) advanceAndPublishLocked() {
	from := s.phase.Current

	// Check if we can advance
	if s.phase.Remaining() > 0 && !s.phase.ShouldAdvance() {
		return
	}

	if !s.phase.Advance() {
		return
	}

	to := s.phase.Current

	// Record transition event
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

	// Close stimulus channel exactly once when entering stimulus
	if to == PhaseStimulus && !s.stimulusNotified {
		s.stimulusNotified = true
		close(s.stimulusCh)
	}

	// Broadcast to all waiters whose target has been reached
	activeWaiters := make([]*waiter, 0, len(s.waiters))
	for _, w := range s.waiters {
		if phaseRank(w.target) <= phaseRank(to) {
			// Wake this waiter
			select {
			case w.result <- nil:
			default:
			}
		} else {
			// Keep this waiter
			activeWaiters = append(activeWaiters, w)
		}
	}
	s.waiters = activeWaiters
}

// removeWaiterLocked removes a specific waiter from the list.
// Must be called with s.mu held.
func (s *Sampler) removeWaiterLocked(w *waiter) {
	for i, uw := range s.waiters {
		if uw == w {
			// Remove this waiter by shifting
			s.waiters = append(s.waiters[:i], s.waiters[i+1:]...)
			return
		}
	}
}

// recordEventLocked records an event. Must be called with s.mu held.
func (s *Sampler) recordEventLocked(eventType, message string) {
	s.events = append(s.events, Event{
		Timestamp: time.Now(),
		Type:      eventType,
		Phase:     s.phase.Current,
		Message:   message,
	})
}

func (s *Sampler) recordEvent(eventType, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordEventLocked(eventType, message)
}

func (s *Sampler) runLoop(ctx context.Context) {
	defer func() {
		s.mu.Lock()

		// Broadcast to all remaining waiters
		// nil terminal means normal completion (PhaseComplete reached)
		// non-nil terminal.err means abnormal termination
		terminalErr := context.Canceled
		if s.terminal != nil && s.terminal.err != nil {
			terminalErr = s.terminal.err
		}
		for _, w := range s.waiters {
			select {
			case w.result <- terminalErr:
			default:
			}
		}
		s.waiters = nil

		s.mu.Unlock()
		s.doneCh <- struct{}{}
	}()

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	sequence := 0

	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.terminal = &terminalState{set: true, err: ctx.Err()}
			s.mu.Unlock()
			return
		case <-s.stopCh:
			s.mu.Lock()
			s.terminal = &terminalState{set: true, err: ErrSamplerStopped}
			s.mu.Unlock()
			return
		case <-ticker.C:
			// Check phase transition atomically
			s.mu.Lock()
			s.advanceAndPublishLocked()
			currentPhase := s.phase.Current
			s.mu.Unlock()

			// Skip sampling if we're in PhaseComplete (terminal state)
			if currentPhase == PhaseComplete {
				continue
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

	// Docker container stats - ONLY populate Docker-specific fields
	if s.docker != nil && s.containerID != "" {
		stats, err := s.docker.ContainerStats(ctx, s.containerID)
		if err == nil && stats != nil {
			// ONLY set Docker-specific fields
			sample.DockerMemoryUsageBytes = stats.MemoryUsageBytes
			sample.DockerMemoryLimitBytes = stats.MemoryLimitBytes
			sample.HasDockerMemory = true
		}
	}

	// Read smaps_rollup (procfs authority)
	smaps, err := procfs.ReadSmapsRollup(pid)
	if err != nil {
		// Check if zombie
		if procErr, ok := err.(*procfs.ProcError); ok && procErr.IsZombie() {
			return nil // Skip zombie
		}
		// Other errors: sample still valid, just missing procfs fields
	} else {
		// Only set procfs fields from authoritative source
		if smaps.HasRSS {
			sample.RSSKiB = smaps.RSSKiB
			sample.HasRSS = true
		}
		if smaps.HasPSS {
			sample.PSSKiB = smaps.PSSKiB
			sample.HasPSS = true
		}
		if smaps.HasPSSAnon {
			sample.PSSAnonKiB = smaps.PSSAnonKiB
			sample.HasPSSAnon = true
		}
		if smaps.HasPrivateDirty {
			sample.PrivateDirtyKiB = smaps.PrivateDirtyKiB
			sample.HasPrivateDirty = true
		}
		if smaps.HasAnonymous {
			sample.AnonymousKiB = smaps.AnonymousKiB
			sample.HasAnonymous = true
		}
		if smaps.HasSwap {
			sample.SwapKiB = smaps.SwapKiB
			sample.HasSwap = true
		}
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

	// Read cgroup memory from real cgroup v2 files
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
	}

	return sample
}

// ShouldAdvance returns true if the current phase has expired.
func (ps *PhaseState) ShouldAdvance() bool {
	return ps.Remaining() == 0 && ps.Current != PhaseComplete
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
		// Memory from procfs
		"rss_kib",
		"pss_kib",
		"pss_anon_kib",
		"private_dirty_kib",
		"anonymous_kib",
		"swap_kib",
		// Docker container memory (EXCLUSIVE)
		"docker_memory_usage_bytes",
		"docker_memory_limit_bytes",
		"has_docker_memory",
		// Resources
		"vma_count",
		"fd_count",
		"socket_fd_count",
		"thread_count",
		"pid_count",
		// Cgroup
		"cgroup_anon_bytes",
		"cgroup_current_bytes",
		"cgroup_memory_stat_anon",
		"has_cgroup",
		"has_cgroup_anon",
		// Semantic
		"oom_events",
		"oom_kill_events",
		"bgp_state",
		"bgp_fsm_ticks",
		"reconnect_count",
		// Availability flags
		"has_rss",
		"has_pss",
		"has_pss_anon",
		"has_private_dirty",
		"has_anonymous",
		"has_swap",
		"has_thread_count",
		"has_pid_count",
		"has_fd_count",
		"has_socket_fd_count",
		"has_vma_count",
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
		// Memory from procfs
		fmt.Sprintf("%d", s.RSSKiB),
		fmt.Sprintf("%d", s.PSSKiB),
		fmt.Sprintf("%d", s.PSSAnonKiB),
		fmt.Sprintf("%d", s.PrivateDirtyKiB),
		fmt.Sprintf("%d", s.AnonymousKiB),
		fmt.Sprintf("%d", s.SwapKiB),
		// Docker container memory (EXCLUSIVE)
		fmt.Sprintf("%d", s.DockerMemoryUsageBytes),
		fmt.Sprintf("%d", s.DockerMemoryLimitBytes),
		fmt.Sprintf("%t", s.HasDockerMemory),
		// Resources
		fmt.Sprintf("%d", s.VMACount),
		fmt.Sprintf("%d", s.FDCount),
		fmt.Sprintf("%d", s.SocketFDCount),
		fmt.Sprintf("%d", s.ThreadCount),
		fmt.Sprintf("%d", s.PIDCount),
		// Cgroup
		fmt.Sprintf("%d", s.CgroupAnonBytes),
		fmt.Sprintf("%d", s.CgroupCurrentBytes),
		fmt.Sprintf("%d", s.CgroupMemoryStatAnon),
		fmt.Sprintf("%t", s.HasCgroup),
		fmt.Sprintf("%t", s.HasCgroupAnon),
		// Semantic
		fmt.Sprintf("%d", s.OOMEvents),
		fmt.Sprintf("%d", s.OOMKillEvents),
		s.BGPState,
		fmt.Sprintf("%d", s.BGPFSMTicks),
		fmt.Sprintf("%d", s.ReconnectCount),
		// Availability flags
		fmt.Sprintf("%t", s.HasRSS),
		fmt.Sprintf("%t", s.HasPSS),
		fmt.Sprintf("%t", s.HasPSSAnon),
		fmt.Sprintf("%t", s.HasPrivateDirty),
		fmt.Sprintf("%t", s.HasAnonymous),
		fmt.Sprintf("%t", s.HasSwap),
		fmt.Sprintf("%t", s.HasThreadCount),
		fmt.Sprintf("%t", s.HasPIDCount),
		fmt.Sprintf("%t", s.HasFDCount),
		fmt.Sprintf("%t", s.HasSocketFDCount),
		fmt.Sprintf("%t", s.HasVMACount),
	}
}
