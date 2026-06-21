package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/diag"
	"github.com/s1onique/KGB/uvb76/probe"
	"github.com/s1onique/KGB/uvb76/scraper"
	"github.com/s1onique/KGB/uvb76/server"
	"github.com/s1onique/KGB/uvb76/state"
)

//go:embed web/dist
var webDist embed.FS

var (
	configPath = flag.String("config", "uvb76.json", "Path to configuration file")
	devMode    = flag.Bool("dev", false, "Enable development mode (allows plain HTTP)")
)

func main() {
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(os.Stdout)

	// Set the embedded web filesystem for the server
	webContent, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Fatalf("Failed to access embedded web content: %v", err)
	}
	server.SetWebFS(webContent)

	// Load configuration - dev mode allows missing TLS
	opts := config.ValidationOptions{AllowMissingTLS: *devMode}
	cfg, err := config.LoadWithOptions(*configPath, opts)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if *devMode {
		log.Println("WARNING: Dev mode enabled - TLS not required")
	}

	// Apply latency config defaults and validate.
	cfg.Latency.ApplyDefaults()
	if err := config.ValidateLatencyConfig(cfg.Latency); err != nil {
		log.Fatalf("Invalid latency config: %v", err)
	}

	// Initialize state manager with HTTP latency configuration
	stateManager := state.NewManagerWithConfig(cfg.Latency.HTTP.HistogramBucketsMS, cfg.Latency.HTTP.RecentSamplesMax)
	// Configure ICMP with its own histogram buckets and max samples
	stateManager.ConfigureICMP(cfg.Latency.ICMP.HistogramBucketsMS, cfg.Latency.ICMP.RecentSamplesMax)

	// Create target configs slice
	targets := make([]*config.TargetConfig, 0, len(cfg.Targets))
	for i := range cfg.Targets {
		targets = append(targets, &cfg.Targets[i])
	}

	// Initialize scraper client
	client := scraper.NewClient(&cfg.Scrape, stateManager, targets)
	client.Start()

	// Initialize HTTP probe client (independent latency probing)
	httpProbeClient := probe.NewClient(&cfg.Latency.HTTP, stateManager, targets)
	if httpProbeClient.IsEnabled() {
		log.Printf("HTTP status probe enabled (interval: %ds, timeout: %dms)",
			cfg.Latency.HTTP.IntervalSeconds, cfg.Latency.HTTP.TimeoutMilliseconds)
	} else {
		log.Println("HTTP status probe disabled")
	}
	httpProbeClient.Start()

	// Initialize ICMP probe client (independent ICMP ping probing)
	icmpProbeClient := probe.NewICMPClient(&cfg.Latency.ICMP, stateManager, targets)
	if icmpProbeClient.IsEnabled() {
		log.Printf("ICMP ping probe enabled (interval: %ds, timeout: %ds)",
			cfg.Latency.ICMP.IntervalSeconds, cfg.Latency.ICMP.TimeoutSeconds)
		// Initialize daemon-owned ICMP telemetry for HTTP status API exposure
		probe.InitGlobalICMPTelemetry(true, cfg.Latency.ICMP.MaxConcurrentOSPing)
	} else {
		log.Println("ICMP ping probe disabled")
	}
	icmpProbeClient.Start()

	// Initialize diagnostic capture service if enabled
	var diagCaptureService *diag.CaptureService
	if cfg.Diagnostics.Enabled {
		captureStore := stateManager.GetCaptureStore()
		diagCaptureService = diag.NewCaptureService(&cfg.Diagnostics, captureStore)
		// Enable capture-aware spike retention with configured cap
		stateManager.EnableCaptureAwareSpikeRetentionWithCap(cfg.Diagnostics.MaxUncapturedSpikes)
		log.Printf("Diagnostic capture enabled (timeout: %dms, cooldown: %ds, max_uncaptured_spikes: %d)",
			cfg.Diagnostics.TimeoutMs, cfg.Diagnostics.CooldownSeconds, cfg.Diagnostics.MaxUncapturedSpikes)
	}

	// Wire diagnostic capture service into HTTP and ICMP probe clients
	if diagCaptureService != nil {
		httpProbeClient.SetDiagCapture(diagCaptureService)
		icmpProbeClient.SetDiagCapture(diagCaptureService)
	}

	// Initialize server (HTTPS in production, HTTP in dev mode)
	srv := server.NewServer(cfg, stateManager, client, *devMode)

	// Set up graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil && err != os.ErrClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	log.Printf("UVB-76 running. Press Ctrl+C to stop.")

	<-sigCh
	log.Println("Shutting down...")
	client.Stop()
	httpProbeClient.Stop()
	icmpProbeClient.Stop()
	srv.Stop()
	log.Println("Shutdown complete.")
}
