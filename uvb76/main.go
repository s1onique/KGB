package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/probe"
	"github.com/s1onique/KGB/uvb76/scraper"
	"github.com/s1onique/KGB/uvb76/server"
	"github.com/s1onique/KGB/uvb76/state"
)

var (
	configPath = flag.String("config", "uvb76.json", "Path to configuration file")
	devMode    = flag.Bool("dev", false, "Enable development mode (allows plain HTTP)")
)

func main() {
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(os.Stdout)

	// Load configuration - dev mode allows missing TLS
	opts := config.ValidationOptions{AllowMissingTLS: *devMode}
	cfg, err := config.LoadWithOptions(*configPath, opts)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if *devMode {
		log.Println("WARNING: Dev mode enabled - TLS not required")
	}

	// Apply latency config defaults and validate
	latencyCfg := cfg.Latency
	latencyCfg.ApplyDefaults()
	if err := config.ValidateLatencyConfig(latencyCfg); err != nil {
		log.Fatalf("Invalid latency config: %v", err)
	}

	// Initialize state manager with latency configuration
	stateManager := state.NewManagerWithConfig(latencyCfg.HistogramBucketsMS, latencyCfg.RecentSamplesMax)

	// Create target configs slice
	targets := make([]*config.TargetConfig, 0, len(cfg.Targets))
	for i := range cfg.Targets {
		targets = append(targets, &cfg.Targets[i])
	}

	// Initialize scraper client
	client := scraper.NewClient(&cfg.Scrape, stateManager, targets)
	client.Start()

	// Initialize probe client (independent latency probing)
	probeClient := probe.NewClient(&latencyCfg, stateManager, targets)
	if probeClient.IsEnabled() {
		log.Printf("Latency probing enabled (interval: %ds, timeout: %dms)",
			latencyCfg.IntervalSeconds, latencyCfg.TimeoutMilliseconds)
	} else {
		log.Println("Latency probing disabled")
	}
	probeClient.Start()

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
	probeClient.Stop()
	srv.Stop()
	log.Println("Shutdown complete.")
}
