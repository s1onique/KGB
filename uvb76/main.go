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

	// Apply latency config defaults and validate
	latencyCfg := cfg.Latency
	latencyCfg.ApplyDefaults()
	if err := config.ValidateLatencyConfig(latencyCfg); err != nil {
		log.Fatalf("Invalid latency config: %v", err)
	}

	// Initialize state manager with HTTP latency configuration
	stateManager := state.NewManagerWithConfig(latencyCfg.HTTP.HistogramBucketsMS, latencyCfg.HTTP.RecentSamplesMax)
	// Configure ICMP with its own histogram buckets and max samples
	stateManager.ConfigureICMP(latencyCfg.ICMP.HistogramBucketsMS, latencyCfg.ICMP.RecentSamplesMax)

	// Create target configs slice
	targets := make([]*config.TargetConfig, 0, len(cfg.Targets))
	for i := range cfg.Targets {
		targets = append(targets, &cfg.Targets[i])
	}

	// Initialize scraper client
	client := scraper.NewClient(&cfg.Scrape, stateManager, targets)
	client.Start()

	// Initialize HTTP probe client (independent latency probing)
	httpProbeClient := probe.NewClient(&latencyCfg.HTTP, stateManager, targets)
	if httpProbeClient.IsEnabled() {
		log.Printf("HTTP status probe enabled (interval: %ds, timeout: %dms)",
			latencyCfg.HTTP.IntervalSeconds, latencyCfg.HTTP.TimeoutMilliseconds)
	} else {
		log.Println("HTTP status probe disabled")
	}
	httpProbeClient.Start()

	// Initialize ICMP probe client (independent ICMP ping probing)
	icmpProbeClient := probe.NewICMPClient(&latencyCfg.ICMP, stateManager, targets)
	if icmpProbeClient.IsEnabled() {
		log.Printf("ICMP ping probe enabled (interval: %ds, timeout: %ds)",
			latencyCfg.ICMP.IntervalSeconds, latencyCfg.ICMP.TimeoutSeconds)
	} else {
		log.Println("ICMP ping probe disabled")
	}
	icmpProbeClient.Start()

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
