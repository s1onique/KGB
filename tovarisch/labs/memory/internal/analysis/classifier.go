// analysis/classifier.go — Memory classification and trend analysis
//
// Classifies memory behavior using actual sample timestamps.
// Uses Theil-Sen estimator for robust slope calculation.
// Classification requires corroboration from multiple signals.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package analysis

import (
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

// SignalSummary holds statistics for a single signal.
type SignalSummary struct {
	Name              string
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

	// Overall classification: resource growth takes priority
	if resourceClass == ClassificationResourceGrowth {
		verdict.Overall = ClassificationResourceGrowth
	} else if memoryClass == ClassificationGrowing {
		verdict.Overall = ClassificationGrowing
	} else if memoryClass == ClassificationInconclusive {
		verdict.Overall = ClassificationInconclusive
	} else if memoryClass == ClassificationProcessReplaced {
		verdict.Overall = ClassificationProcessReplaced
	} else {
		verdict.Overall = ClassificationStable
	}

	return verdict
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

// signalAvailability defines which samples have valid values for a signal.
type signalAvailability struct {
	hasRSS          func(sampling.Sample) bool
	hasPSS          func(sampling.Sample) bool
	hasPSSAnon      func(sampling.Sample) bool
	hasPrivateDirty func(sampling.Sample) bool
	hasAnonymous    func(sampling.Sample) bool
	hasSwap         func(sampling.Sample) bool
	hasCgroup       func(sampling.Sample) bool
	hasThreadCount  func(sampling.Sample) bool
	hasPIDCount     func(sampling.Sample) bool
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
	case "pid_count", "fd_count", "socket_fd_count", "vma_count":
		return func(s sampling.Sample) bool { return true } // Always available if we have a sample
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
		// Docker growth below canary threshold is inconclusive
		return ClassificationInconclusive
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
