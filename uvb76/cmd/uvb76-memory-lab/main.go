// uvb76-memory-lab collects memory profile artifacts from a running process.
//
// This tool is part of the KGB memory lab suite for analyzing memory growth
// patterns in Go processes without embedding TSDB or observability overhead.
//
// Usage:
//
//	uvb76-memory-lab --pprof-url http://127.0.0.1:6060 --pid 12345 --duration 60s --artifact-dir ./artifacts
//
// Artifacts produced (baseline always t000, final suffix derived from duration):
//
//	heap-t000.pb.gz         - baseline heap profile
//	heap-t<duration>.pb.gz  - final heap profile (e.g., t060 for 60s, t600 for 600s)
//	allocs-t000.pb.gz       - baseline allocs profile
//	allocs-t<duration>.pb.gz - final allocs profile
//	goroutine-t000.txt       - baseline goroutine dump
//	goroutine-t<duration>.txt - final goroutine dump
//	rss-series.csv          - RSS/VSZ/threads/fd count over time
//	goroutine-count-series.csv - goroutine count over time
//	manifest.json           - lab run metadata
//	verdict.json            - classification verdict
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-memory-lab/internal/collector"
)

var (
	pprofURL       = flag.String("pprof-url", "http://127.0.0.1:6060", "pprof server URL")
	pid            = flag.Int("pid", 0, "process ID to sample")
	duration       = flag.Duration("duration", 60*time.Second, "collection duration")
	sampleInterval = flag.Duration("sample-interval", 5*time.Second, "RSS sampling interval")
	profileInterval = flag.Duration("profile-interval", 30*time.Second, "profile capture interval")
	artifactDir    = flag.String("artifact-dir", "artifacts/uvb76-memory-lab", "artifact output directory")
	showVersion    = flag.Bool("version", false, "show version and exit")
)

const version = "0.1.0"

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Printf("uvb76-memory-lab %s\n", version)
		os.Exit(0)
	}

	if *pid <= 0 {
		fmt.Fprintf(os.Stderr, "Error: --pid is required and must be positive\n\n")
		flag.Usage()
		os.Exit(1)
	}

	if *pprofURL == "" {
		fmt.Fprintf(os.Stderr, "Error: --pprof-url is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	fmt.Printf("uvb76-memory-lab %s\n", version)
	fmt.Printf("Collecting from PID %d at %s\n", *pid, *pprofURL)
	fmt.Printf("Duration: %s, Sample interval: %s, Profile interval: %s\n",
		*duration, *sampleInterval, *profileInterval)
	fmt.Printf("Artifact directory: %s\n\n", *artifactDir)

	cfg := collector.Config{
		PProfURL:       *pprofURL,
		PID:            *pid,
		Duration:       *duration,
		SampleInterval: *sampleInterval,
		ProfileInterval: *profileInterval,
		ArtifactDir:    *artifactDir,
	}

	coll, err := collector.NewCollector(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating collector: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *duration+time.Minute)
	defer cancel()

	if err := coll.Run(ctx); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Println("Collection completed (deadline reached)")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error during collection: %v\n", err)
		os.Exit(1)
	}
}
