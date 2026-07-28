package main

import (
	"context"
	"flag"
	"time"

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-memleak-pprof-lab/internal/fake"
)

const (
	// Default lab configuration
	defaultDuration        = 2 * time.Minute
	defaultSampleInterval  = 1 * time.Second
	defaultProfileInterval = 60 * time.Second

	// Ports
	defaultTovarischPort = "18317"
	defaultUVB76Port     = "18444"
	defaultPProfPort     = "16060"
)

var (
	flagDuration         = flag.Duration("duration", defaultDuration, "lab duration")
	flagSampleInterval   = flag.Duration("sample-interval", defaultSampleInterval, "RSS sampling interval")
	flagProfileInterval  = flag.Duration("profile-interval", defaultProfileInterval, "profile capture interval")
	flagArtifactDir      = flag.String("artifact-dir", "", "artifact output directory (required)")
	flagSkipPprofDiff    = flag.Bool("skip-pprof-diff", false, "skip pprof diff execution")
	flagUseFakeTovarisch = flag.Bool("use-fake-tovarisch", false, "use fake tovarisch status server (default false for real smoke)")
	flagTovarischBin     = flag.String("tovarisch-bin", "", "path to real Tovarisch binary (required in real mode)")
	flagTovarischArgs    = flag.String("tovarisch-args", "serve --listen-private", "arguments for Tovarisch serve command")
	flagUVB76Bin         = flag.String("uvb76-bin", "", "path to real UVB-76 binary (required in real mode)")
	flagTovarischPort    = flag.String("tovarisch-port", defaultTovarischPort, "port for Tovarisch HTTP status service")
	flagUVB76Port        = flag.String("uvb76-port", defaultUVB76Port, "port for UVB-76 HTTP server")
	flagPProfPort        = flag.String("pprof-port", defaultPProfPort, "port for UVB-76 pprof server")
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

	// Process state tracking
	tovarischProcessState *ProcessState
	uvb76ProcessState     *ProcessState
)

// Port accessors for use in config generation
func TovarischPort() string { return *flagTovarischPort }
func UVB76Port() string     { return *flagUVB76Port }
func PProfPort() string     { return *flagPProfPort }
