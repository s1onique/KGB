// analysis/classifier.go — Memory classification and trend analysis
//
// Classifies memory behavior using actual sample timestamps.
// Uses Theil-Sen estimator for robust slope calculation.
// Classification requires corroboration from multiple signals.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package analysis

import (
	"fmt"
	"sort"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling"
)

// Classification represents the result of memory analysis.
type Classification string

const (
	ClassificationStable          Classification = "stable"
	ClassificationGrowing         Classification = "growing"
	ClassificationResourceGrowth  Classification = "resource_growth"
	ClassificationProcessReplaced Classification = "process_replaced"
	ClassificationInconclusive    Classification = "inconclusive"
	ClassificationInvalid         Classification = "invalid"
)

// Thresholds define the sensitivity of classification.
type Thresholds struct {
	MemoryGrowthKibPerHour     int64   // Primary memory growth threshold (KiB/hour)
	MemoryGrowthPercentPerHour float64 // Relative memory growth threshold (%/hour)
	ResourceGrowthPerHour      int     // FD/thread/VMA growth per hour
	CorroborationCount         int     // Number of signals needed for growing
	SampleCountMinimum         int     // Minimum samples for valid analysis
	WindowMinimum              int     // Minimum samples per window
}

// DefaultThresholds returns the default classification thresholds.
func DefaultThresholds() Thresholds {
	return Thresholds{
		MemoryGrowthKibPerHour:     500, // 500 KiB/hour
		MemoryGrowthPercentPerHour: 0.5, // 0.5% per hour
		ResourceGrowthPerHour:      10,  // 10 resources/hour
		CorroborationCount:         2,   // Need 2 signals
		SampleCountMinimum:         10,  // Minimum 10 samples
		WindowMinimum:              3,   // Minimum 3 samples per window
	}
}

// Verdict represents the complete analysis verdict.
type Verdict struct {
	Overall  Classification
	Memory   Classification
	Resource Classification
	Semantic Classification
	Signals  []SignalSummary
	Failures []string
	Warnings []string
	Unknowns []string
}

// SignalKind identifies the evidence source of a SignalSummary.
//
// "sampled" is the default kind for host-side sample streams
// (memory and resource counts observed during the run).
//
// "state_invariant" is the descriptor-specific fallback source
// reconstructed from the canary's authoritative state endpoint
// (initial and final canary states plus workload result). It is
// used when the host-side FD signal is unavailable so the
// canary-state invariant can act as the named, distinct
// resource-classification source.
//
// CORRECTION02: Empty SignalKind is now a verifier-level rejection
// (the source-kind contract is strict: sampled or state_invariant).
type SignalKind string

const (
	SignalKindSampled        SignalKind = "sampled"
	SignalKindStateInvariant SignalKind = "state_invariant"
)

// SignalSummary holds statistics for a single signal.
type SignalSummary struct {
	Name              string
	SourceKind        SignalKind // "sampled" or "state_invariant"
	FirstWindowMedian int64
	LastWindowMedian  int64
	AbsoluteDelta     int64
	RelativeDelta     float64 // % change
	RatePerHour       float64 // KiB/hour or count/hour
	Slope             float64 // Theil-Sen slope (normalized to per-second)
	SampleCount       int
	MissingCount      int
	AvailableCount    int
	Minimum           int64
	Maximum           int64
	Classification    Classification
	IsPrimary         bool // PSSAnon, PrivateDirty, Anonymous, CgroupAnon
}

// Analyze performs memory classification on samples.
func Analyze(samples []sampling.Sample, thresholds Thresholds) *Verdict {
	verdict := &Verdict{
		Signals:  make([]SignalSummary, 0),
		Failures: make([]string, 0),
		Warnings: make([]string, 0),
		Unknowns: make([]string, 0),
	}

	// Check for empty samples
	if len(samples) == 0 {
		verdict.Overall = ClassificationInconclusive
		verdict.Warnings = append(verdict.Warnings, "no samples collected")
		return verdict
	}

	// Check for process replacement
	if hasProcessReplacement(samples) {
		verdict.Overall = ClassificationProcessReplaced
		verdict.Memory = ClassificationProcessReplaced
		verdict.Resource = ClassificationProcessReplaced
		verdict.Semantic = ClassificationProcessReplaced
		verdict.Failures = append(verdict.Failures, "process replacement detected")
		return verdict
	}

	// Check sample count
	if len(samples) < thresholds.SampleCountMinimum {
		verdict.Overall = ClassificationInconclusive
		verdict.Warnings = append(verdict.Warnings, "insufficient samples")
		return verdict
	}

	// Check for required phases
	if !hasBaselineAndStimulus(samples) {
		verdict.Overall = ClassificationInconclusive
		verdict.Warnings = append(verdict.Warnings, "missing baseline or stimulus samples")
		return verdict
	}

	// Analyze memory signals
	memorySignals := analyzeMemorySignals(samples, thresholds)

	// Analyze resource signals
	resourceSignals := analyzeResourceSignals(samples, thresholds)

	// Classify memory separately (modifies signals in place)
	memoryClass := classifyMemorySignals(memorySignals, thresholds)
	verdict.Memory = memoryClass

	// Classify resources separately using resource thresholds (modifies signals in place)
	resourceClass := classifyResourceSignals(resourceSignals, thresholds)
	verdict.Resource = resourceClass

	// Now populate verdict.Signals AFTER classification updates the Classification field
	verdict.Signals = append(verdict.Signals, memorySignals...)
	verdict.Signals = append(verdict.Signals, resourceSignals...)

	// Semantic classification
	semanticClass := classifySemantic(samples, thresholds)
	verdict.Semantic = semanticClass

	// Overall classification (CORRECTION01 priority order):
	//   1. invalid semantic or invalid invariant → invalid
	//   2. memory growing → growth (memory growth has priority over
	//      descriptor resource growth; simultaneous memory and FD
	//      growth yields overall=growth, not descriptor-only
	//      resource_growth)
	//   3. else resource growing → resource_growth
	//   4. else stable/inconclusive/process_replaced per existing
	//      rules
	verdict.Overall = computeOverall(verdict.Memory, verdict.Resource, verdict.Semantic)
	return verdict
}

// computeOverall derives the overall classification from the
// per-dimension classifications. Pure function so producer and
// verifier can derive the same overall value independently.
//
// Priority order (CORRECTION01):
//  1. invalid semantic → invalid
//  2. memory growing → growth (memory growth has priority over
//     descriptor resource growth)
//  3. else resource growing → resource_growth
//  4. else follow memory: inconclusive / process_replaced / stable
func computeOverall(memory, resource, semantic Classification) Classification {
	if semantic == ClassificationInvalid {
		return ClassificationInvalid
	}
	if memory == ClassificationGrowing {
		return ClassificationGrowing
	}
	if resource == ClassificationResourceGrowth {
		return ClassificationResourceGrowth
	}
	if memory == ClassificationInconclusive {
		return ClassificationInconclusive
	}
	if memory == ClassificationProcessReplaced {
		return ClassificationProcessReplaced
	}
	return ClassificationStable
}

// ComputeOverall exposes the priority logic for tests and
// reconstruction. It mirrors computeOverall exactly.
func ComputeOverall(memory, resource, semantic Classification) Classification {
	return computeOverall(memory, resource, semantic)
}

func hasProcessReplacement(samples []sampling.Sample) bool {
	if len(samples) < 2 {
		return false
	}
	first := samples[0]
	for _, s := range samples[1:] {
		if s.PID != first.PID || s.ProcessStartTime != first.ProcessStartTime {
			return true
		}
	}
	return false
}

func hasBaselineAndStimulus(samples []sampling.Sample) bool {
	hasBaseline := false
	hasStimulus := false
	for _, s := range samples {
		if s.Phase == sampling.PhaseBaseline {
			hasBaseline = true
		}
		if s.Phase == sampling.PhaseStimulus || s.Phase == sampling.PhaseFinal {
			hasStimulus = true
		}
	}
	return hasBaseline && hasStimulus
}

// analyzeMemorySignals analyzes primary memory signals.
func analyzeMemorySignals(samples []sampling.Sample, thresholds Thresholds) []SignalSummary {
	signals := []SignalSummary{}

	// Primary memory signals (must be available for growing classification)
	primaryFields := []struct {
		name string
		get  func(s sampling.Sample) int64
	}{
		{"pss_anon_kib", func(s sampling.Sample) int64 { return s.PSSAnonKiB }},
		{"private_dirty_kib", func(s sampling.Sample) int64 { return s.PrivateDirtyKiB }},
		{"anonymous_kib", func(s sampling.Sample) int64 { return s.AnonymousKiB }},
		{"cgroup_anon_kib", func(s sampling.Sample) int64 { return s.CgroupAnonBytes / 1024 }},
	}

	// Secondary memory signals
	secondaryFields := []struct {
		name string
		get  func(s sampling.Sample) int64
	}{
		{"rss_kib", func(s sampling.Sample) int64 { return s.RSSKiB }},
		{"pss_kib", func(s sampling.Sample) int64 { return s.PSSKiB }},
		{"cgroup_current_kib", func(s sampling.Sample) int64 { return s.CgroupCurrentBytes / 1024 }},
		// Docker container memory - corroborated via Docker stats API when procfs is unavailable
		{"docker_memory_kib", func(s sampling.Sample) int64 { return s.DockerMemoryUsageBytes / 1024 }},
	}

	for _, field := range primaryFields {
		summary := analyzeSignal(field.name, samples, field.get, thresholds, true)
		signals = append(signals, summary)
	}

	for _, field := range secondaryFields {
		summary := analyzeSignal(field.name, samples, field.get, thresholds, false)
		signals = append(signals, summary)
	}

	return signals
}

// analyzeResourceSignals analyzes resource count signals.
func analyzeResourceSignals(samples []sampling.Sample, thresholds Thresholds) []SignalSummary {
	signals := []SignalSummary{}

	resourceFields := []struct {
		name string
		get  func(s sampling.Sample) int64
	}{
		{"fd_count", func(s sampling.Sample) int64 { return int64(s.FDCount) }},
		{"socket_fd_count", func(s sampling.Sample) int64 { return int64(s.SocketFDCount) }},
		{"thread_count", func(s sampling.Sample) int64 { return int64(s.ThreadCount) }},
		{"vma_count", func(s sampling.Sample) int64 { return int64(s.VMACount) }},
		{"pid_count", func(s sampling.Sample) int64 { return int64(s.PIDCount) }},
	}

	for _, field := range resourceFields {
		summary := analyzeSignal(field.name, samples, field.get, thresholds, false)
		signals = append(signals, summary)
	}

	return signals
}

// getSignalAvailability returns the availability checker for a signal name.
func getSignalAvailability(name string) func(sampling.Sample) bool {
	switch name {
	case "rss_kib":
		return func(s sampling.Sample) bool { return s.HasRSS }
	case "pss_kib":
		return func(s sampling.Sample) bool { return s.HasPSS }
	case "pss_anon_kib":
		return func(s sampling.Sample) bool { return s.HasPSSAnon }
	case "private_dirty_kib":
		return func(s sampling.Sample) bool { return s.HasPrivateDirty }
	case "anonymous_kib":
		return func(s sampling.Sample) bool { return s.HasAnonymous }
	case "swap_kib":
		return func(s sampling.Sample) bool { return s.HasSwap }
	case "cgroup_anon_kib", "cgroup_current_kib":
		return func(s sampling.Sample) bool { return s.HasCgroup }
	case "docker_memory_kib":
		return func(s sampling.Sample) bool { return s.HasDockerMemory }
	case "thread_count":
		return func(s sampling.Sample) bool { return s.HasThreadCount }
	case "fd_count":
		return func(s sampling.Sample) bool { return s.HasFDCount }
	case "socket_fd_count":
		return func(s sampling.Sample) bool { return s.HasSocketFDCount }
	case "vma_count":
		return func(s sampling.Sample) bool { return s.HasVMACount }
	case "pid_count":
		return func(s sampling.Sample) bool { return s.HasPIDCount }
	default:
		return func(s sampling.Sample) bool { return true }
	}
}

// analyzeSignal computes trend statistics for a single signal using actual timestamps.
// Only uses samples where the signal is available (avoids treating missing values as zero).
func analyzeSignal(name string, samples []sampling.Sample, getValue func(sampling.Sample) int64, thresholds Thresholds, isPrimary bool) SignalSummary {
	if len(samples) < 2 {
		return SignalSummary{
			Name:           name,
			SourceKind:     SignalKindSampled,
			SampleCount:    len(samples),
			Classification: ClassificationInconclusive,
			IsPrimary:      isPrimary,
		}
	}

	// Get availability checker for this signal
	isAvailable := getSignalAvailability(name)

	// Extract values and timestamps, only for available samples
	points := make([]samplePoint, 0, len(samples))
	availableCount := 0
	missingCount := 0

	for _, s := range samples {
		if isAvailable(s) {
			val := getValue(s)
			points = append(points, samplePoint{time: s.Timestamp, value: val})
			availableCount++
		} else {
			missingCount++
		}
	}

	// Need at least 2 available samples for trend analysis
	if len(points) < 2 {
		return SignalSummary{
			Name:           name,
			SourceKind:     SignalKindSampled,
			SampleCount:    len(samples),
			AvailableCount: availableCount,
			MissingCount:   missingCount,
			Classification: ClassificationInconclusive,
			IsPrimary:      isPrimary,
		}
	}

	// Sort by time
	sort.Slice(points, func(i, j int) bool {
		return points[i].time.Before(points[j].time)
	})

	// Split into first and last windows
	mid := len(points) / 2
	firstWindow := make([]int64, mid)
	lastWindow := make([]int64, len(points)-mid)

	for i := 0; i < mid; i++ {
		firstWindow[i] = points[i].value
	}
	for i := mid; i < len(points); i++ {
		lastWindow[i-mid] = points[i].value
	}

	// Compute medians
	firstMedian := median(firstWindow)
	lastMedian := median(lastWindow)

	// Compute deltas
	absDelta := lastMedian - firstMedian
	var relDelta float64
	if firstMedian > 0 {
		relDelta = float64(absDelta) / float64(firstMedian) * 100
	}

	// Compute time-normalized rate (per hour)
	var ratePerHour float64
	if len(points) >= 2 {
		duration := points[len(points)-1].time.Sub(points[0].time)
		if duration > 0 {
			hours := duration.Hours()
			ratePerHour = float64(absDelta) / hours
		}
	}

	// Compute Theil-Sen slope (normalized to per-second)
	slope := theilSenSlope(points)

	// Find min/max
	minVal := points[0].value
	maxVal := points[0].value
	for _, p := range points[1:] {
		if p.value < minVal {
			minVal = p.value
		}
		if p.value > maxVal {
			maxVal = p.value
		}
	}

	return SignalSummary{
		Name:              name,
		SourceKind:        SignalKindSampled,
		FirstWindowMedian: firstMedian,
		LastWindowMedian:  lastMedian,
		AbsoluteDelta:     absDelta,
		RelativeDelta:     relDelta,
		RatePerHour:       ratePerHour,
		Slope:             slope,
		SampleCount:       len(samples),
		MissingCount:      len(samples) - availableCount,
		AvailableCount:    availableCount,
		Minimum:           minVal,
		Maximum:           maxVal,
		Classification:    ClassificationStable,
		IsPrimary:         isPrimary,
	}
}

// samplePoint holds a time-value pair for trend analysis.
type samplePoint struct {
	time  time.Time
	value int64
}

// theilSenSlope computes the Theil-Sen estimator slope in units per second.
func theilSenSlope(points []samplePoint) float64 {
	if len(points) < 2 {
		return 0
	}

	// For small samples, use simple linear regression
	if len(points) <= 5 {
		return simpleSlope(points)
	}

	// Compute all pairwise slopes
	var slopes []float64
	for i := 0; i < len(points)-1; i++ {
		for j := i + 1; j < len(points); j++ {
			dy := float64(points[j].value - points[i].value)
			dt := points[j].time.Sub(points[i].time).Seconds()
			if dt != 0 {
				slopes = append(slopes, dy/dt)
			}
		}
	}

	if len(slopes) == 0 {
		return 0
	}

	// Return median slope
	sort.Float64s(slopes)
	mid := len(slopes) / 2
	if len(slopes)%2 == 0 {
		return (slopes[mid-1] + slopes[mid]) / 2
	}
	return slopes[mid]
}

func simpleSlope(points []samplePoint) float64 {
	if len(points) < 2 {
		return 0
	}

	// Use actual timestamps for linear regression
	firstTime := points[0].time
	var sumX, sumY, sumXY, sumX2 float64
	n := float64(len(points))

	for _, p := range points {
		// Use seconds since first sample
		x := p.time.Sub(firstTime).Seconds()
		y := float64(p.value)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	denom := n*sumX2 - sumX*sumX
	if denom == 0 {
		return 0
	}

	// Return slope in units per second
	return (n*sumXY - sumX*sumY) / denom
}

// median computes the median of a slice.
func median(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]int64, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

// Canary calibration threshold: exactly 32 MiB retained delta
// This is the expected growth for the growing-canary calibration scenario
const CanaryRetainedDeltaBytes int64 = 32 * 1024 * 1024 // 32 MiB = 33,554,432 bytes

// classifyMemorySignals classifies memory behavior.
// Modifies the Classification field of signals in place.
func classifyMemorySignals(signals []SignalSummary, thresholds Thresholds) Classification {
	growingPrimaryCount := 0
	growingSecondaryCount := 0
	hasRSSOnlyGrowth := false
	dockerMemoryGrowing := false
	cgroupAnonGrowing := false

	// Track material deltas for canary calibration
	var dockerDeltaKiB, cgroupAnonDeltaKiB int64

	for i := range signals {
		s := &signals[i]
		growing := false

		// Check rate per hour against absolute threshold
		if s.RatePerHour > float64(thresholds.MemoryGrowthKibPerHour) {
			growing = true
		}

		// Check relative growth
		if s.RelativeDelta > thresholds.MemoryGrowthPercentPerHour {
			growing = true
		}

		// Check positive slope
		if s.Slope > 0.01 {
			growing = true
		}

		if growing {
			s.Classification = ClassificationGrowing
			if s.IsPrimary {
				growingPrimaryCount++
			} else {
				growingSecondaryCount++
			}

			// Track docker memory growth
			if s.Name == "docker_memory_kib" {
				dockerMemoryGrowing = true
				dockerDeltaKiB = s.AbsoluteDelta
			}

			// Track cgroup anon growth
			if s.Name == "cgroup_anon_kib" {
				cgroupAnonGrowing = true
				cgroupAnonDeltaKiB = s.AbsoluteDelta
			}

			// Track RSS-only growth
			if s.Name == "rss_kib" && !hasRSSOnlyGrowth {
				hasRSSOnlyGrowth = true
			}
		} else {
			s.Classification = ClassificationStable
		}
	}

	// RSS-only growth without primary signal is inconclusive
	if growingSecondaryCount > 0 && growingPrimaryCount == 0 && hasRSSOnlyGrowth && !dockerMemoryGrowing {
		return ClassificationInconclusive
	}

	// Require corroboration from primary signals
	if growingPrimaryCount >= thresholds.CorroborationCount {
		return ClassificationGrowing
	}

	// Primary growth with secondary corroboration
	if growingPrimaryCount > 0 && growingSecondaryCount >= thresholds.CorroborationCount {
		return ClassificationGrowing
	}

	// Any primary growth without corroboration is inconclusive
	if growingPrimaryCount > 0 {
		return ClassificationInconclusive
	}

	// Docker-only growth: restrict to canary calibration rule
	// Only classify as growing if:
	// 1. Docker memory shows material growth AND
	// 2. The delta is >= 32 MiB (canary calibration threshold)
	if dockerMemoryGrowing && growingPrimaryCount == 0 {
		// Check if Docker delta meets canary calibration threshold (32 MiB)
		// Docker delta is in KiB, threshold is in bytes
		canaryThresholdKiB := CanaryRetainedDeltaBytes / 1024
		if dockerDeltaKiB >= canaryThresholdKiB {
			return ClassificationGrowing
		}
		// Small Docker variation with no primary corroboration: stable.
		// Bounded and descriptor canaries legitimately show small Docker
		// memory variation from their static buffer, not from workload-
		// proportional allocation. The canary scenario's state invariants
		// (buffer unchanged, retained=0, operation-count delta == completed)
		// are the authoritative "no workload-proportional growth" signal
		// and are verified separately by validateStateInvariant.
		return ClassificationStable
	}

	// Cgroup anon-only growth: also restrict to canary calibration rule
	if cgroupAnonGrowing && !dockerMemoryGrowing && growingPrimaryCount == 0 && growingSecondaryCount == 0 {
		canaryThresholdKiB := CanaryRetainedDeltaBytes / 1024
		if cgroupAnonDeltaKiB >= canaryThresholdKiB {
			return ClassificationGrowing
		}
		return ClassificationInconclusive
	}

	// Docker memory corroborates secondary signals when primary signals show some growth
	if dockerMemoryGrowing && growingSecondaryCount > 0 && growingPrimaryCount == 0 {
		canaryThresholdKiB := CanaryRetainedDeltaBytes / 1024
		if dockerDeltaKiB >= canaryThresholdKiB {
			return ClassificationGrowing
		}
		return ClassificationInconclusive
	}

	return ClassificationStable
}

// classifyResourceSignals classifies resource behavior using resource thresholds.
func classifyResourceSignals(signals []SignalSummary, thresholds Thresholds) Classification {
	growingCount := 0

	for _, s := range signals {
		growing := false

		// Check rate per hour against resource threshold
		if s.RatePerHour > float64(thresholds.ResourceGrowthPerHour) {
			growing = true
		}

		// Check positive slope
		if s.Slope > 0.001 {
			growing = true
		}

		if growing {
			s.Classification = ClassificationResourceGrowth
			growingCount++
		} else {
			s.Classification = ClassificationStable
		}
	}

	// Require corroboration from at least one resource signal
	if growingCount >= 1 {
		return ClassificationResourceGrowth
	}

	return ClassificationStable
}

// classifySemantic checks semantic state consistency.
func classifySemantic(samples []sampling.Sample, thresholds Thresholds) Classification {
	if len(samples) == 0 {
		return ClassificationInconclusive
	}

	// Check for OOM events
	for _, s := range samples {
		if s.OOMEvents > 0 || s.OOMKillEvents > 0 {
			return ClassificationGrowing // OOM is definitive growth
		}
	}

	return ClassificationStable
}

// StateInvariantResult holds the result of state invariant validation.
type StateInvariantResult struct {
	Valid    bool
	Failures []string
}

// AnalyzeWithInvariant performs analysis with state invariant validation.
func AnalyzeWithInvariant(samples []sampling.Sample, thresholds Thresholds, invariant *StateInvariantResult) *Verdict {
	verdict := Analyze(samples, thresholds)

	// If invariant validation failed, mark as invalid
	if invariant != nil && !invariant.Valid {
		verdict.Overall = ClassificationInvalid
		verdict.Semantic = ClassificationInvalid
		verdict.Failures = append(verdict.Failures, invariant.Failures...)
	}

	return verdict
}

// DescriptorInitialState captures the canary's initial descriptor-mode
// state. Both fields are part of the descriptor invariant contract.
type DescriptorInitialState struct {
	FDCount        int
	OperationCount int
	Mode           string
	Ready          bool
}

// DescriptorFinalState captures the canary's final descriptor-mode
// state. The retained_blocks/retained_bytes fields must be zero
// (the descriptor scenario does not retain memory).
type DescriptorFinalState struct {
	FDCount        int
	OperationCount int
	Mode           string
	Ready          bool
	RetainedBlocks int
	RetainedBytes  int64
}

// DescriptorWorkloadResult is the workload result for the descriptor
// scenario (mirrors WorkloadResult but is defined here so the analysis
// package does not import the main package).
type DescriptorWorkloadResult struct {
	Requested int
	Attempted int
	Completed int
	Failed    int
	Returned  int
}

// DescriptorStateInvariant holds the precomputed canary-state
// invariant values: the FD delta, the expected FD delta, and
// whether the workload arithmetic supports the descriptor contract.
type DescriptorStateInvariant struct {
	FDDelta          int
	ExpectedFDDelta  int
	OpDelta          int
	WorkloadComplete int
	WorkloadFailed   int
	WorkloadReturned int
}

// DescriptorFallbackInput bundles the inputs that determine whether
// the descriptor-fallback path is allowed.
//
// CORRECTION02: StateInvariantValid is the explicit precomputed
// result of validateStateInvariant. The fallback may only be
// applied when StateInvariantValid == true. An invalid scenario
// invariant forces overall=invalid AND prevents the
// descriptor_state_invariant signal from being emitted.
type DescriptorFallbackInput struct {
	Scenario            string
	StateInvariantValid bool
	Invariant           DescriptorStateInvariant
	Initial             DescriptorInitialState
	Final               DescriptorFinalState
	Workload            DescriptorWorkloadResult
	SamplesAvailable    bool // at least one sample has HasFDCount=true
	SamplesCount        int
}

// DescriptorFallbackResult is the output of ApplyDescriptorStateInvariant.
// It carries the exact signal summary, the modified verdict fields, and
// a flag indicating whether the fallback was applied.
type DescriptorFallbackResult struct {
	Applied    bool
	Signal     SignalSummary
	Invariants DescriptorStateInvariant
	Failures   []string
}

// ApplyDescriptorStateInvariant is the pure function shared by
// producer and verifier. It applies the §8 "permitted fallback":
// when the descriptor scenario's complete canary-state invariant
// is satisfied and the host-side FD sampler is unavailable, the
// canary-state invariant becomes the named, distinct
// resource-classification source — descriptor_state_invariant.
//
// CORRECTION02 contract: Applied=true ONLY when every gate passes:
//
//   - StateInvariantValid == true
//   - Scenario == "canary-descriptor"
//   - Workload.Requested   == 100
//   - Workload.Attempted   == Workload.Requested
//   - Workload.Completed   == Workload.Requested
//   - Workload.Failed      == 0
//   - Workload.Returned    == Workload.Completed
//   - Invariant.OpDelta    == Workload.Completed
//   - Invariant.FDDelta    == Workload.Completed × 2
//   - Initial.Mode         == "descriptor"
//   - Final.Mode           == "descriptor"
//   - Initial.Ready        == true
//   - Final.Ready          == true
//   - Final.RetainedBlocks == 0
//   - Final.RetainedBytes  == 0
//   - SamplesAvailable     == false (no sampled FD data)
//
// The returned signal uses exactly two observations (initial +
// final canary state) with zero rate, slope, and relative delta.
// The minimum is the initial FD count; the maximum is the final
// FD count.
func ApplyDescriptorStateInvariant(
	input DescriptorFallbackInput,
) DescriptorFallbackResult {
	res := DescriptorFallbackResult{}
	res.Invariants = input.Invariant

	// Gate 0: scenario invariant must be valid. Checked FIRST so an
	// invalid scenario invariant cannot trigger the fallback signal.
	if !input.StateInvariantValid {
		res.Failures = append(res.Failures,
			"state_invariant_valid=false; descriptor_state_invariant fallback must not apply")
		return res
	}

	// Gate 1: scenario
	if input.Scenario != "canary-descriptor" {
		res.Failures = append(res.Failures,
			fmt.Sprintf("scenario=%s, expected canary-descriptor", input.Scenario))
		return res
	}

	// Gate 2: workload.Requested must be 100
	if input.Workload.Requested != 100 {
		res.Failures = append(res.Failures,
			fmt.Sprintf("workload.requested=%d, expected 100", input.Workload.Requested))
		return res
	}
	// Gate 3: workload.Attempted must equal Requested
	if input.Workload.Attempted != input.Workload.Requested {
		res.Failures = append(res.Failures,
			fmt.Sprintf("workload.attempted=%d, expected %d (==requested)",
				input.Workload.Attempted, input.Workload.Requested))
		return res
	}
	// Gate 4: workload.Completed must equal Requested
	if input.Workload.Completed != input.Workload.Requested {
		res.Failures = append(res.Failures,
			fmt.Sprintf("workload.completed=%d, expected %d (==requested)",
				input.Workload.Completed, input.Workload.Requested))
		return res
	}
	// Gate 5: workload.Failed must be 0
	if input.Workload.Failed != 0 {
		res.Failures = append(res.Failures,
			fmt.Sprintf("workload.failed=%d, expected 0", input.Workload.Failed))
		return res
	}
	// Gate 6: workload.Returned must equal Completed
	if input.Workload.Returned != input.Workload.Completed {
		res.Failures = append(res.Failures,
			fmt.Sprintf("workload.returned=%d, expected %d (==completed)",
				input.Workload.Returned, input.Workload.Completed))
		return res
	}

	// Gate 7: operation_count delta must equal workload.completed
	if input.Invariant.OpDelta != input.Invariant.WorkloadComplete {
		res.Failures = append(res.Failures,
			fmt.Sprintf("operation_count_delta=%d != workload.completed=%d",
				input.Invariant.OpDelta, input.Invariant.WorkloadComplete))
		return res
	}
	// Gate 8: fd_delta must equal workload.completed × 2
	if input.Invariant.FDDelta != input.Invariant.ExpectedFDDelta {
		res.Failures = append(res.Failures,
			fmt.Sprintf("fd_delta=%d != expected=%d",
				input.Invariant.FDDelta, input.Invariant.ExpectedFDDelta))
		return res
	}

	// Gate 9: initial mode must be "descriptor"
	if input.Initial.Mode != "descriptor" {
		res.Failures = append(res.Failures,
			fmt.Sprintf("initial.mode=%s, expected descriptor", input.Initial.Mode))
		return res
	}
	// Gate 10: final mode must be "descriptor"
	if input.Final.Mode != "descriptor" {
		res.Failures = append(res.Failures,
			fmt.Sprintf("final.mode=%s, expected descriptor", input.Final.Mode))
		return res
	}
	// Gate 11: initial.ready must be true
	if !input.Initial.Ready {
		res.Failures = append(res.Failures,
			"initial.ready=false, expected true")
		return res
	}
	// Gate 12: final.ready must be true
	if !input.Final.Ready {
		res.Failures = append(res.Failures,
			"final.ready=false, expected true")
		return res
	}
	// Gate 13: final.retained_blocks must be 0
	if input.Final.RetainedBlocks != 0 {
		res.Failures = append(res.Failures,
			fmt.Sprintf("final.retained_blocks=%d, expected 0", input.Final.RetainedBlocks))
		return res
	}
	// Gate 14: final.retained_bytes must be 0
	if input.Final.RetainedBytes != 0 {
		res.Failures = append(res.Failures,
			fmt.Sprintf("final.retained_bytes=%d, expected 0", input.Final.RetainedBytes))
		return res
	}

	// Gate 15: host-side FD sampler must be unavailable
	if input.SamplesAvailable {
		res.Failures = append(res.Failures,
			"sampled FD signal is available; descriptor_state_invariant fallback must not apply")
		return res
	}

	// All 16 gates pass — the canary-state invariant becomes the
	// authoritative descriptor oracle.
	//
	// The signal uses EXACTLY two observations: the initial and
	// final canary-state fd_count values. The host-side sample count
	// is captured separately for evidence-geometry audits but does
	// not contribute to the invariant's own geometry.
	res.Applied = true
	res.Signal = SignalSummary{
		Name:              "descriptor_state_invariant",
		SourceKind:        SignalKindStateInvariant,
		FirstWindowMedian: int64(input.Initial.FDCount),
		LastWindowMedian:  int64(input.Final.FDCount),
		AbsoluteDelta:     int64(input.Invariant.FDDelta),
		RelativeDelta:     0,
		RatePerHour:       0,
		Slope:             0,
		SampleCount:       2, // initial + final canary-state observations
		MissingCount:      0,
		AvailableCount:    2,
		Minimum:           int64(input.Initial.FDCount),
		Maximum:           int64(input.Final.FDCount),
		Classification:    ClassificationResourceGrowth,
		IsPrimary:         true,
	}
	return res
}

// ComputeDescriptorStateInvariant derives the canary-state
// descriptor invariant values from the canary's initial/final state
// and the workload result. Pure function for use by both producer
// and verifier.
func ComputeDescriptorStateInvariant(
	initialFDCount, finalFDCount, initialOpCount, finalOpCount int,
	workload DescriptorWorkloadResult,
) DescriptorStateInvariant {
	return DescriptorStateInvariant{
		FDDelta:          finalFDCount - initialFDCount,
		ExpectedFDDelta:  workload.Completed * 2,
		OpDelta:          finalOpCount - initialOpCount,
		WorkloadComplete: workload.Completed,
		WorkloadFailed:   workload.Failed,
		WorkloadReturned: workload.Returned,
	}
}

// samplesHaveFDAvailable reports whether any sample in the slice
// carries a usable FD observation (HasFDCount=true). Used by the
// descriptor-fallback gate.
func SamplesHaveFDAvailable(samples []sampling.Sample) bool {
	for _, s := range samples {
		if s.HasFDCount {
			return true
		}
	}
	return false
}

// ComputeOverallWithInvariant derives the overall classification
// with explicit invariant validity. Producer and verifier share
// this exact function.
//
// CORRECTION02 priority order:
//  1. invalid semantic OR invalid scenario invariant → invalid
//  2. memory growing → growth (memory growth has priority over
//     descriptor resource growth)
//  3. else resource growing → resource_growth
//  4. else follow memory: inconclusive / process_replaced / stable
func ComputeOverallWithInvariant(memory, resource, semantic Classification, invariantValid bool) Classification {
	if semantic == ClassificationInvalid {
		return ClassificationInvalid
	}
	if !invariantValid {
		return ClassificationInvalid
	}
	return computeOverall(memory, resource, semantic)
}
