// Package collector provides the memory lab collector runner.
package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-memory-lab/internal/fetcher"
	"github.com/s1onique/KGB/uvb76/cmd/uvb76-memory-lab/internal/sampler"
	"github.com/s1onique/KGB/uvb76/tools/memory-lab"
)

// Config holds the collector configuration.
type Config struct {
	// PProfURL is the pprof server URL
	PProfURL string
	// PID is the process ID to sample
	PID int
	// Duration is how long to run the collection
	Duration time.Duration
	// SampleInterval is how often to sample RSS/proc metrics
	SampleInterval time.Duration
	// ProfileInterval is how often to capture heap profiles
	ProfileInterval time.Duration
	// ArtifactDir is where to write artifacts
	ArtifactDir string
}

// ArtifactTimeSuffix returns the artifact time suffix for a given duration in seconds.
// Example: 60 -> "t060", 600 -> "t600"
func ArtifactTimeSuffix(seconds int) string {
	return fmt.Sprintf("t%03d", seconds)
}

// Collector runs the memory lab collection.
type Collector struct {
	cfg     Config
	fetcher *fetcher.Fetcher
}

// NewCollector creates a new collector.
func NewCollector(cfg Config) (*Collector, error) {
	if err := os.MkdirAll(cfg.ArtifactDir, 0755); err != nil {
		return nil, fmt.Errorf("create artifact dir: %w", err)
	}

	return &Collector{
		cfg: cfg,
		fetcher: fetcher.NewFetcher(fetcher.Config{
			BaseURL:   cfg.PProfURL,
			Timeout:   30 * time.Second,
			UserAgent: "uvb76-memory-lab/1.0",
		}),
	}, nil
}

// Run executes the memory lab collection.
func (c *Collector) Run(ctx context.Context) error {
	startTime := time.Now()
	labID := fmt.Sprintf("memory-lab-%d", startTime.Unix())

	// Compute final artifact suffix from duration
	finalSuffix := ArtifactTimeSuffix(int(c.cfg.Duration.Seconds()))

	// Open CSV file for RSS series
	rssCSV, err := os.Create(filepath.Join(c.cfg.ArtifactDir, "rss-series.csv"))
	if err != nil {
		return fmt.Errorf("create rss csv: %w", err)
	}
	defer rssCSV.Close()

	// Write CSV header
	fmt.Fprintln(rssCSV, sampler.ProcSampleCSVHeader())

	// Open CSV file for goroutine count series
	grpcCSV, err := os.Create(filepath.Join(c.cfg.ArtifactDir, "goroutine-count-series.csv"))
	if err != nil {
		return fmt.Errorf("create goroutine csv: %w", err)
	}
	defer grpcCSV.Close()
	fmt.Fprintln(grpcCSV, "elapsed_seconds,goroutines")

	// Track goroutine counts
	var goroutineCounts []memlab.GoroutineSample

	// Initial samples
	fmt.Printf("[%s] Collecting baseline samples...\n", elapsed(startTime))

	// Initial heap profile
	heapBase := filepath.Join(c.cfg.ArtifactDir, "heap-t000.pb.gz")
	if err := c.fetcher.FetchHeap(ctx, heapBase, true); err != nil {
		fmt.Printf("Warning: failed to fetch baseline heap: %v\n", err)
	}

	// Initial allocs profile
	allocsBase := filepath.Join(c.cfg.ArtifactDir, "allocs-t000.pb.gz")
	if err := c.fetcher.FetchAllocs(ctx, allocsBase); err != nil {
		fmt.Printf("Warning: failed to fetch baseline allocs: %v\n", err)
	}

	// Initial goroutine dump
	goroutineBase := filepath.Join(c.cfg.ArtifactDir, "goroutine-t000.txt")
	if err := c.fetcher.FetchGoroutine(ctx, goroutineBase, 2); err != nil {
		fmt.Printf("Warning: failed to fetch baseline goroutines: %v\n", err)
	}

	// Count initial goroutines (best effort)
	initialGoroutines, _ := c.countGoroutines(goroutineBase)
	goroutineCounts = append(goroutineCounts, memlab.GoroutineSample{
		Timestamp: startTime,
		Count:     initialGoroutines,
	})
	fmt.Fprintf(grpcCSV, "%.1f,%d\n", 0.0, initialGoroutines)

	// Initial RSS sample
	initialRSS, err := sampler.SampleProc(c.cfg.PID)
	if err != nil {
		return fmt.Errorf("sample initial proc: %w", err)
	}
	fmt.Fprintln(rssCSV, sampler.ProcSampleToCSV(initialRSS, startTime))

	// Periodic sampling loop
	ticker := time.NewTicker(c.cfg.SampleInterval)
	defer ticker.Stop()

	profileTicker := time.NewTicker(c.cfg.ProfileInterval)
	defer profileTicker.Stop()

	var endRSS *sampler.ProcSample
	var finalGoroutines int

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Collection cancelled")
			return ctx.Err()

		case <-ticker.C:
			elapsedDur := time.Since(startTime)
			if elapsedDur >= c.cfg.Duration {
				goto final
			}

			// Sample RSS
			sample, err := sampler.SampleProc(c.cfg.PID)
			if err != nil {
				fmt.Printf("Warning: failed to sample proc: %v\n", err)
				continue
			}
			fmt.Fprintln(rssCSV, sampler.ProcSampleToCSV(sample, startTime))

			// Count goroutines from latest dump if available
			goroutinePath := filepath.Join(c.cfg.ArtifactDir,
				fmt.Sprintf("goroutine-t%03d.txt", int(elapsedDur.Seconds())))
			if _, err := os.Stat(goroutinePath); err == nil {
				count, _ := c.countGoroutines(goroutinePath)
				goroutineCounts = append(goroutineCounts, memlab.GoroutineSample{
					Timestamp: time.Now(),
					Count:     count,
				})
				fmt.Fprintf(grpcCSV, "%.1f,%d\n", elapsedDur.Seconds(), count)
			}

		case <-profileTicker.C:
			elapsedDur := time.Since(startTime)
			if elapsedDur >= c.cfg.Duration {
				goto final
			}

			tSeconds := int(elapsedDur.Seconds())
			fmt.Printf("[%s] Capturing profiles at t+%ds...\n", elapsed(startTime), tSeconds)

			// Capture heap
			heapPath := filepath.Join(c.cfg.ArtifactDir,
				fmt.Sprintf("heap-t%03d.pb.gz", tSeconds))
			if err := c.fetcher.FetchHeap(ctx, heapPath, true); err != nil {
				fmt.Printf("Warning: failed to fetch heap: %v\n", err)
			}

			// Capture allocs
			allocsPath := filepath.Join(c.cfg.ArtifactDir,
				fmt.Sprintf("allocs-t%03d.pb.gz", tSeconds))
			if err := c.fetcher.FetchAllocs(ctx, allocsPath); err != nil {
				fmt.Printf("Warning: failed to fetch allocs: %v\n", err)
			}

			// Capture goroutines
			goroutinePath := filepath.Join(c.cfg.ArtifactDir,
				fmt.Sprintf("goroutine-t%03d.txt", tSeconds))
			if err := c.fetcher.FetchGoroutine(ctx, goroutinePath, 2); err != nil {
				fmt.Printf("Warning: failed to fetch goroutines: %v\n", err)
			}
		}
	}

final:
	// Final samples
	fmt.Printf("[%s] Collecting final samples...\n", elapsed(startTime))

	endTime := time.Now()

	// Final heap profile (use duration-derived suffix)
	heapFinal := filepath.Join(c.cfg.ArtifactDir, fmt.Sprintf("heap-%s.pb.gz", finalSuffix))
	if err := c.fetcher.FetchHeap(ctx, heapFinal, true); err != nil {
		fmt.Printf("Warning: failed to fetch final heap: %v\n", err)
	}

	// Final allocs profile
	allocsFinal := filepath.Join(c.cfg.ArtifactDir, fmt.Sprintf("allocs-%s.pb.gz", finalSuffix))
	if err := c.fetcher.FetchAllocs(ctx, allocsFinal); err != nil {
		fmt.Printf("Warning: failed to fetch final allocs: %v\n", err)
	}

	// Final goroutine dump
	goroutineFinal := filepath.Join(c.cfg.ArtifactDir, fmt.Sprintf("goroutine-%s.txt", finalSuffix))
	if err := c.fetcher.FetchGoroutine(ctx, goroutineFinal, 2); err != nil {
		fmt.Printf("Warning: failed to fetch final goroutines: %v\n", err)
	}

	finalGoroutines, _ = c.countGoroutines(goroutineFinal)
	goroutineCounts = append(goroutineCounts, memlab.GoroutineSample{
		Timestamp: endTime,
		Count:     finalGoroutines,
	})
	fmt.Fprintf(grpcCSV, "%.1f,%d\n", c.cfg.Duration.Seconds(), finalGoroutines)

	// Final RSS sample
	endRSS, err = sampler.SampleProc(c.cfg.PID)
	if err != nil {
		return fmt.Errorf("sample final proc: %w", err)
	}
	fmt.Fprintln(rssCSV, sampler.ProcSampleToCSV(endRSS, startTime))

	// Build manifest
	manifest := c.buildManifest(labID, startTime, endTime, initialRSS, endRSS, goroutineCounts)
	manifestPath := filepath.Join(c.cfg.ArtifactDir, "manifest.json")
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	// Build and write verdict
	verdictPath := filepath.Join(c.cfg.ArtifactDir, "verdict.json")
	verdictData, err := json.MarshalIndent(manifest.Verdict, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal verdict: %w", err)
	}
	if err := os.WriteFile(verdictPath, verdictData, 0644); err != nil {
		return fmt.Errorf("write verdict: %w", err)
	}

	fmt.Printf("[%s] Collection complete. Artifacts in %s\n", elapsed(startTime), c.cfg.ArtifactDir)
	fmt.Printf("Classification: %s\n", manifest.Classification)

	return nil
}

func (c *Collector) buildManifest(labID string, start, end time.Time,
	initialRSS *sampler.ProcSample, finalRSS *sampler.ProcSample,
	goroutineCounts []memlab.GoroutineSample) *memlab.Manifest {

	// Extract goroutine start/end counts
	var goroutinesStart, goroutinesEnd int
	if len(goroutineCounts) > 0 {
		goroutinesStart = goroutineCounts[0].Count
	}
	if len(goroutineCounts) > 1 {
		goroutinesEnd = goroutineCounts[len(goroutineCounts)-1].Count
	}

	classification := memlab.Classify(memlab.ClassifierInput{
		RSSStartBytes:       initialRSS.RSSBytes,
		RSSEndBytes:         finalRSS.RSSBytes,
		HeapInuseStartBytes: 0, // Not extracted from pb.gz in this ACT
		HeapInuseEndBytes:   0,
		GoroutinesStart:     goroutinesStart,
		GoroutinesEnd:       goroutinesEnd,
		HasHeapData:         false, // Heap data extraction from pb.gz not implemented
		HasRSSData:          true,
	})

	verdict := memlab.BuildVerdict(classification, memlab.ClassifierInput{
		RSSStartBytes:       initialRSS.RSSBytes,
		RSSEndBytes:         finalRSS.RSSBytes,
		HeapInuseStartBytes: 0,
		HeapInuseEndBytes:   0,
		GoroutinesStart:     goroutinesStart,
		GoroutinesEnd:       goroutinesEnd,
		HasHeapData:         false,
		HasRSSData:          true,
	})

	// Collect heap profile paths using duration-derived suffix
	finalSuffix := ArtifactTimeSuffix(int(c.cfg.Duration.Seconds()))
	var heapProfiles []string
	for _, name := range []string{"heap-t000.pb.gz", fmt.Sprintf("heap-%s.pb.gz", finalSuffix)} {
		path := filepath.Join(c.cfg.ArtifactDir, name)
		if _, err := os.Stat(path); err == nil {
			heapProfiles = append(heapProfiles, name)
		}
	}

	return &memlab.Manifest{
		SchemaVersion:    memlab.SchemaVersion,
		LabID:            labID,
		StartedAt:        start,
		EndedAt:          end,
		TargetID:         fmt.Sprintf("pid-%d", c.cfg.PID),
		DurationSeconds:  c.cfg.Duration.Seconds(),
		Classification:   classification,
		Verdict:          verdict,
		Samples: memlab.Samples{
			StartRSS: memlab.RSSSample{
				Timestamp: initialRSS.Timestamp,
				Bytes:     initialRSS.RSSBytes,
			},
			EndRSS: memlab.RSSSample{
				Timestamp: finalRSS.Timestamp,
				Bytes:     finalRSS.RSSBytes,
			},
			GoroutineSamples: goroutineCounts,
			HeapProfiles:     heapProfiles,
		},
	}
}

// countGoroutines counts goroutines from a goroutine dump file.
// This is a best-effort count based on the number of "goroutine N" lines.
func (c *Collector) countGoroutines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	// Simple heuristic: count "goroutine " occurrences at start of lines
	// The pprof goroutine?debug=2 output has "goroutine <id>:" format
	count := 0
	content := string(data)
	for i := 0; i < len(content)-10; i++ {
		if content[i] == 'g' && content[i:i+10] == "goroutine " {
			// Check it's at start of line
			if i == 0 || content[i-1] == '\n' {
				count++
			}
		}
	}
	return count, nil
}

func elapsed(start time.Time) string {
	return time.Since(start).Round(time.Second).String()
}
