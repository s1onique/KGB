package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"io/fs"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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

	// Diagnostics startup logger: uses slog text handler for structured key=value output.
	// We create a local logger rather than calling slog.SetDefault to avoid changing
	// the behavior of existing log.Printf/log.Println calls used elsewhere in main().
	diagLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

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

	// Apply memory profile rate as early as possible.
	// Go requires MemProfileRate to be constant across the program lifetime;
	// changing it early ensures all allocations are sampled consistently.
	if cfg.Diagnostics.Enabled && cfg.Diagnostics.PProf.Enabled {
		config.ApplyPProfRuntimeConfig(cfg.Diagnostics.PProf)
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
	icmpProbeClient, err := probe.NewICMPClient(&cfg.Latency.ICMP, stateManager, targets)
	if err != nil {
		log.Fatalf("Failed to initialize ICMP probe client: %v", err)
	}
	if icmpProbeClient.IsEnabled() {
		backendType := cfg.Latency.ICMP.BackendType()
		log.Printf("ICMP ping probe enabled (backend: %s, interval: %ds, timeout: %ds)",
			backendType, cfg.Latency.ICMP.IntervalSeconds, cfg.Latency.ICMP.TimeoutSeconds)
		// Initialize daemon-owned ICMP telemetry for HTTP status API exposure
		probe.InitGlobalICMPTelemetry(true, cfg.Latency.ICMP.MaxConcurrentOSPing)
		// Wire actual backend stats to global telemetry for /status API
		if stats := icmpProbeClient.GetNativeICMPStats(); stats != nil {
			probe.InitGlobalNativeICMPTelemetry(stats)
		}
	} else {
		log.Println("ICMP ping probe disabled")
	}
	icmpProbeClient.Start()

	// Initialize diagnostic capture service if enabled
	var diagCaptureService *diag.CaptureService
	if cfg.Diagnostics.Enabled {
		captureStore := stateManager.GetCaptureStore()
		diagCaptureService = diag.NewCaptureService(&cfg.Diagnostics, captureStore)
		
		// Wire anchor validator for ghost suppression prevention.
		// Production validator checks BOTH:
		// 1. Anchor spike is retained in timeline (wasn't evicted)
		// 2. Anchor capture has successful status (DiagCaptureStatusOK)
		diagCaptureService.SetAnchorValidatorWithCaptureStatus(
			func(targetID, probeKind string) []state.SpikeEvent {
				return stateManager.GetSpikes(targetID, probeKind, 0)
			},
			func(eventID string) []state.DiagCapture {
				return captureStore.GetCaptures(eventID)
			},
		)

		// Enable capture-aware spike retention with configured cap
		stateManager.EnableCaptureAwareSpikeRetentionWithCap(cfg.Diagnostics.MaxUncapturedSpikes)

		// Log diagnostics config with structured fields for grep/journal compatibility.
		// This proves UVB parsed its peer config correctly; it does NOT claim TCP diagnostics
		// are enabled on the peer side (that is controlled by tovarisch).
		logDiagnosticsStartup(diagLogger, &cfg.Diagnostics)
	}

	// Wire diagnostic capture service into HTTP and ICMP probe clients
	if diagCaptureService != nil {
		httpProbeClient.SetDiagCapture(diagCaptureService)
		icmpProbeClient.SetDiagCapture(diagCaptureService)
	}

	// Initialize server (HTTPS in production, HTTP in dev mode)
	srv := server.NewServer(cfg, stateManager, client, *devMode)

	// Initialize pprof server if enabled
	var pprofServer *http.Server
	if cfg.Diagnostics.Enabled && cfg.Diagnostics.PProf.Enabled {
		// Create the pprof server
		pprofServer = config.NewPProfServer(cfg.Diagnostics.PProf)

		// Bind synchronously - fail fast if address is in use
		listener, err := net.Listen("tcp", cfg.Diagnostics.PProf.Listen)
		if err != nil {
			log.Fatalf("bind pprof listener %q: %v", cfg.Diagnostics.PProf.Listen, err)
		}
		log.Printf("pprof server listening on %s", listener.Addr().String())

		// Start pprof server in background
		go func() {
			if err := pprofServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("pprof server error: %v", err)
			}
		}()
	}

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

	// Shutdown pprof server gracefully if it was started
	if pprofServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := pprofServer.Shutdown(ctx); err != nil {
			log.Printf("pprof server shutdown error: %v", err)
		} else {
			log.Println("pprof server stopped")
		}
	}

	log.Println("Shutdown complete.")
}

// sanitizeBaseURLForLog returns the base_url with scheme stripped for logging.
// The scheme is excluded because it adds no diagnostic value and keeping it
// consistent simplifies log parsing (always host:port/path format).
// base_url validation rejects userinfo, so no credentials are present.
func sanitizeBaseURLForLog(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	// Strip scheme:// prefix (validation ensures http:// or https://)
	withoutScheme := strings.TrimPrefix(baseURL, "http://")
	withoutScheme = strings.TrimPrefix(withoutScheme, "https://")
	return withoutScheme
}

// logDiagnosticsStartup logs the diagnostics configuration at startup with structured
// key=value fields for grep/journal compatibility. This proves UVB parsed its peer
// config correctly; it does NOT claim TCP diagnostics are enabled on the peer side
// (that is controlled by tovarisch).
func logDiagnosticsStartup(logger *slog.Logger, cfg *config.DiagnosticsConfig) {
	logger.Info("uvb76 diagnostics configured",
		"enabled", cfg.Enabled,
		"peer_count", len(cfg.Peers),
		"capture_on_spike", cfg.CaptureOnSpike,
		"timeout_ms", cfg.TimeoutMs,
		"cooldown_seconds", cfg.CooldownSeconds,
	)

	for _, peer := range cfg.Peers {
		logger.Info("uvb76 diagnostics peer configured",
			"name", peer.Name,
			"base_url", sanitizeBaseURLForLog(peer.BaseURL),
			"targets", strings.Join(peer.Targets, ","),
			"capture_on_spike", cfg.CaptureOnSpike,
			"timeout_ms", cfg.TimeoutMs,
			"cooldown_seconds", cfg.CooldownSeconds,
		)
	}
}
