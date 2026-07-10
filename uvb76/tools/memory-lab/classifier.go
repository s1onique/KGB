// Package memlab provides artifact contracts and classification for memory leak labs.
package memlab

// Conservative thresholds for classification.
const (
	// MinRSSGrowthBytes is the minimum RSS growth to be considered material.
	// 1 MB = 1,048,576 bytes
	MinRSSGrowthBytes int64 = 1024 * 1024
	// MinHeapGrowthBytes is the minimum heap growth to be considered material.
	MinHeapGrowthBytes int64 = 512 * 1024
	// MinGoroutineGrowth is the minimum goroutine count increase to be material.
	// 10 goroutines is a conservative threshold for "leak-like" growth.
	MinGoroutineGrowth int = 10
)

// ClassifierInput contains the data needed for classification.
type ClassifierInput struct {
	// RSS and heap data (in bytes)
	RSSStartBytes      uint64
	RSSEndBytes        uint64
	HeapInuseStartBytes uint64
	HeapInuseEndBytes   uint64
	// Goroutine counts
	GoroutinesStart int
	GoroutinesEnd   int
	// Data availability flags
	HasHeapData bool
	HasRSSData  bool
}

// Classify returns the memory classification based on the input samples.
// This is a pure function with no side effects.
func Classify(in ClassifierInput) Classification {
	// Missing required data: inconclusive
	if !in.HasRSSData {
		return ClassificationInconclusive
	}

	// Compute deltas
	rssDelta := int64(in.RSSEndBytes) - int64(in.RSSStartBytes)
	heapDelta := int64(in.HeapInuseEndBytes) - int64(in.HeapInuseStartBytes)
	goroutineDelta := int64(in.GoroutinesEnd) - int64(in.GoroutinesStart)

	// Goroutine growth is independent of RSS/heap
	if in.GoroutinesStart > 0 && in.GoroutinesEnd > in.GoroutinesStart {
		if goroutineDelta >= int64(MinGoroutineGrowth) {
			return ClassificationGoroutineGrowth
		}
	}

	// RSS grows materially and heap grows materially => suspected_go_heap_retention
	if rssDelta >= MinRSSGrowthBytes && in.HasHeapData && heapDelta >= MinHeapGrowthBytes {
		return ClassificationGoHeapRetention
	}

	// RSS grows materially and heap is stable => rss_growth_heap_stable
	if rssDelta >= MinRSSGrowthBytes && in.HasHeapData && heapDelta < MinHeapGrowthBytes {
		return ClassificationRSSGrowthStable
	}

	// RSS grows materially but no heap data => inconclusive
	if rssDelta >= MinRSSGrowthBytes && !in.HasHeapData {
		return ClassificationInconclusive
	}

	// No material growth detected
	return ClassificationNoMaterialGrowth
}

// BuildVerdict creates a human-readable verdict from classification and samples.
func BuildVerdict(classification Classification, in ClassifierInput) Verdict {
	rssDelta := int64(in.RSSEndBytes) - int64(in.RSSStartBytes)
	heapDelta := int64(in.HeapInuseEndBytes) - int64(in.HeapInuseStartBytes)
	goroutineDelta := int64(in.GoroutinesEnd) - int64(in.GoroutinesStart)

	var summary string
	var reasons []string

	switch classification {
	case ClassificationGoroutineGrowth:
		summary = "goroutine count grew materially"
		reasons = []string{"goroutine_count_delta", formatCountDelta("goroutines", goroutineDelta)}
	case ClassificationGoHeapRetention:
		summary = "suspected Go heap retention"
		reasons = []string{
			"rss_grew_and_heap_grew",
			formatDelta("RSS", rssDelta),
			formatDelta("heap_inuse", heapDelta),
		}
	case ClassificationRSSGrowthStable:
		summary = "RSS grew but heap is stable"
		reasons = []string{
			"rss_grew_heap_stable",
			formatDelta("RSS", rssDelta),
			"heap_delta_below_threshold",
		}
	case ClassificationNoMaterialGrowth:
		summary = "no material memory growth detected"
		reasons = []string{
			formatDelta("RSS", rssDelta),
			formatDelta("heap_inuse", heapDelta),
			formatCountDelta("goroutines", goroutineDelta),
		}
	case ClassificationInconclusive:
		summary = "insufficient data for classification"
		if !in.HasRSSData {
			reasons = []string{"missing_rss_data"}
		} else if !in.HasHeapData {
			reasons = []string{"missing_heap_data", "rss_delta_may_be_material"}
		} else {
			reasons = []string{"unknown_insufficient_data"}
		}
	}

	return Verdict{
		Summary:         summary,
		RSSGrowthBytes:  rssDelta,
		HeapGrowthBytes: heapDelta,
		GoroutineDelta:  goroutineDelta,
		Reasons:         reasons,
	}
}
