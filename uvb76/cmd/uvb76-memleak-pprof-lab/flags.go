package main

import (
	"context"
	"flag"
	"time"

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-memleak-pprof-lab/internal/fake"
)

const (
	// Default lab configuration
	defaultDuration        = 10 * time.Minute
	defaultSampleInterval  = 10 * time.Second
	defaultProfileInterval = 60 * time.Second

	// Ports
	uvb76Port     = "18444"
	pprofPort     = "16060"
	tovarischPort = "18317"
)

var (
	flagDuration         = flag.Duration("duration", defaultDuration, "lab duration")
	flagSampleInterval   = flag.Duration("sample-interval", defaultSampleInterval, "RSS sampling interval")
	flagProfileInterval  = flag.Duration("profile-interval", defaultProfileInterval, "profile capture interval")
	flagArtifactDir      = flag.String("artifact-dir", "", "artifact output directory (required)")
	flagSkipPprofDiff    = flag.Bool("skip-pprof-diff", false, "skip pprof diff execution")
	flagUseFakeTovarisch = flag.Bool("use-fake-tovarisch", true, "use fake tovarisch status server")
)

// Global state
var (
	artifactDir      string
	configFile       string
	uvb76LogFile     string
	tovarischLogFile string
	uvb76PID         int
	tovarischPID     int
	uvb76Done        chan struct{}
	tovarischDone    chan struct{}
	labCtx           context.Context
	labCancel        context.CancelFunc
	fakeServer       *fake.StatusServer
)
