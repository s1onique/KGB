// Package main provides a CLI wrapper for the UVB-76 capture netns polling library.
//
// This binary exposes the polling functions as a CLI tool for use by shell scripts
// that need to delegate to typed Go code. It is not intended to replace the full
// lab workflow but rather to provide a polling interface.
//
// Usage:
//   uvb76-capture-netns-polling [command] [flags]
//
// Commands:
//   probe-samples    Poll for probe samples readiness
//   spike-event      Poll for spike event with matching reasons
//   capture          Poll for capture status for a specific event
//
// Exit codes:
//   0   Success
//   1   Polling timeout or capture failure
//   2   Invalid arguments
//   3   API/client error
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-capture-netns-polling/internal/polling"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "probe-samples":
		os.Exit(runProbeSamples(os.Args[2:]))
	case "spike-event":
		os.Exit(runSpikeEvent(os.Args[2:]))
	case "capture":
		os.Exit(runCapture(os.Args[2:]))
	case "help", "-h", "--help":
		printUsage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `UVB-76 Capture Netns Polling CLI

Usage: uvb76-capture-netns-polling [command] [flags]

Commands:
  probe-samples    Poll for probe samples readiness
  spike-event      Poll for spike event with matching reasons
  capture          Poll for capture status for a specific event

Exit codes:
  0   Success
  1   Polling timeout or capture failure
  2   Invalid arguments
  3   API/client error

Examples:
  # Poll for probe samples
  uvb76-capture-netns-polling probe-samples \
    --base-url http://localhost:9999 \
    --target lab-tovarisch \
    --timeout 30s \
    --require-count 2 \
    --output /tmp/probe-ready.json

  # Poll for spike event
  uvb76-capture-netns-polling spike-event \
    --base-url http://localhost:9999 \
    --target lab-tovarisch \
    --cursor "2024-01-01T00:00:00Z" \
    --reasons "http_probe_timeout|http_probe_failure" \
    --timeout 30s \
    --output /tmp/spike-event.json

  # Poll for capture
  uvb76-capture-netns-polling capture \
    --base-url http://localhost:9999 \
    --target lab-tovarisch \
    --event-id evt123 \
    --require-captured \
    --timeout 30s \
    --output /tmp/capture.json
`)
}

// --- Probe Samples ---

func runProbeSamples(args []string) int {
	fs := flag.NewFlagSet("probe-samples", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `probe-samples: Poll for probe samples readiness

Usage: uvb76-capture-netns-polling probe-samples [flags]

Flags:
`)
		fs.PrintDefaults()
	}

	baseURL := fs.String("base-url", "http://localhost:9999", "UVB-76 API base URL")
	target := fs.String("target", "lab-tovarisch", "Target ID to poll")
	kind := fs.String("kind", "http", "Probe kind (http, icmp)")
	timeout := fs.Duration("timeout", 30*time.Second, "Poll timeout")
	requireCount := fs.Int("require-count", 2, "Minimum sample count for success")
	rangeSeconds := fs.Int("range-seconds", 120, "Time range for latency series")
	username := fs.String("username", "lab-admin", "API username")
	password := fs.String("password", "testpass123", "API password")
	cookieJar := fs.String("cookie-jar", "", "Path to cookie jar file for session persistence (optional)")
	output := fs.String("output", "", "Output file for artifact (optional)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Create API client
	client := polling.NewAPIClient(*baseURL, *username, *password)
	if *cookieJar != "" {
		client.Cookies = *cookieJar
	}

	// Create artifact writer if output specified
	var writer polling.ArtifactWriter
	if *output != "" {
		writer = &polling.FileArtifactWriter{ProbeReadyPath: *output}
	}

	// Create poller
	poller := polling.NewPoller(client, writer)

	// Configure polling
	cfg := polling.PollConfig{
		Interval:     2 * time.Second,
		Timeout:      *timeout,
		RequireCount: *requireCount,
	}

	// Poll
	ctx, cancel := context.WithTimeout(context.Background(), *timeout+5*time.Second)
	defer cancel()

	result := poller.PollProbeSamples(ctx, *target, *kind, *rangeSeconds, cfg)

	// Output result as JSON for shell consumption
	outputResult(result, *output)

	if result.Error != nil {
		log.Printf("ERROR: probe-samples: %v", result.Error)
		return 3
	}
	if result.Timeout {
		if result.LastError != nil {
			log.Printf("ERROR: probe-samples: timeout after %v (last API error: %v)", *timeout, result.LastError)
		} else {
			log.Printf("ERROR: probe-samples: timeout after %v", *timeout)
		}
		return 1
	}
	if !result.OK {
		log.Printf("ERROR: probe-samples: not ready (sample_count=%d, require=%d)", result.SampleCount, *requireCount)
		return 1
	}

	fmt.Printf("OK: probe samples ready (sample_count=%d, point_count=%d)\n", result.SampleCount, result.PointCount)
	return 0
}

// --- Spike Event ---

func runSpikeEvent(args []string) int {
	fs := flag.NewFlagSet("spike-event", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `spike-event: Poll for spike event with matching reasons

Usage: uvb76-capture-netns-polling spike-event [flags]

Flags:
`)
		fs.PrintDefaults()
	}

	baseURL := fs.String("base-url", "http://localhost:9999", "UVB-76 API base URL")
	target := fs.String("target", "lab-tovarisch", "Target ID to poll")
	cursor := fs.String("cursor", "", "Cursor timestamp (RFC3339) to filter spikes")
	reasons := fs.String("reasons", "http_probe_timeout|http_probe_failure|http_probe_connection_refused", "Regex pattern for spike reasons")
	timeout := fs.Duration("timeout", 30*time.Second, "Poll timeout")
	username := fs.String("username", "lab-admin", "API username")
	password := fs.String("password", "testpass123", "API password")
	cookieJar := fs.String("cookie-jar", "", "Path to cookie jar file for session persistence (optional)")
	output := fs.String("output", "", "Output file for artifact (optional)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Create API client
	client := polling.NewAPIClient(*baseURL, *username, *password)
	if *cookieJar != "" {
		client.Cookies = *cookieJar
	}

	// Create artifact writer if output specified
	var writer polling.ArtifactWriter
	if *output != "" {
		writer = &polling.FileArtifactWriter{SpikeEventPath: *output}
	}

	// Create poller
	poller := polling.NewPoller(client, writer)

	// Configure polling
	cfg := polling.SpikeEventConfig{
		Interval:    2 * time.Second,
		Timeout:     *timeout,
		ReasonRegex: *reasons,
	}

	// Poll
	ctx, cancel := context.WithTimeout(context.Background(), *timeout+5*time.Second)
	defer cancel()

	result := poller.PollSpikeEvent(ctx, *target, *cursor, *reasons, cfg)

	// Output result as JSON for shell consumption
	outputResult(result, *output)

	if result.Error != nil {
		log.Printf("ERROR: spike-event: %v", result.Error)
		return 3
	}
	if result.Timeout {
		if result.LastError != nil {
			log.Printf("ERROR: spike-event: timeout after %v (last API error: %v)", *timeout, result.LastError)
		} else {
			log.Printf("ERROR: spike-event: timeout after %v", *timeout)
		}
		return 1
	}
	if !result.OK {
		log.Printf("ERROR: spike-event: no matching spike event found")
		return 1
	}

	// Export matched event info for shell scripts
	fmt.Printf("OK: spike event found (event_id=%s, reasons=%s)\n", result.EventID, strings.Join(result.Reasons, "|"))
	fmt.Printf("MATCHED_EVENT_ID=%s\n", result.EventID)
	fmt.Printf("MATCHED_REASONS=%s\n", strings.Join(result.Reasons, "|"))
	return 0
}

// --- Capture ---

func runCapture(args []string) int {
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `capture: Poll for capture status for a specific event

Usage: uvb76-capture-netns-polling capture [flags]

Flags:
`)
		fs.PrintDefaults()
	}

	baseURL := fs.String("base-url", "http://localhost:9999", "UVB-76 API base URL")
	target := fs.String("target", "lab-tovarisch", "Target ID to poll")
	eventID := fs.String("event-id", "", "Event ID to poll capture for (required)")
	requireCaptured := fs.Bool("require-captured", true, "Require capture_status=captured")
	timeout := fs.Duration("timeout", 30*time.Second, "Poll timeout")
	username := fs.String("username", "lab-admin", "API username")
	password := fs.String("password", "testpass123", "API password")
	cookieJar := fs.String("cookie-jar", "", "Path to cookie jar file for session persistence (optional)")
	output := fs.String("output", "", "Output file for artifact (optional)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *eventID == "" {
		fmt.Fprintf(os.Stderr, "ERROR: --event-id is required\n")
		return 2
	}

	// Create API client
	client := polling.NewAPIClient(*baseURL, *username, *password)
	if *cookieJar != "" {
		client.Cookies = *cookieJar
	}

	// Create artifact writer if output specified
	var writer polling.ArtifactWriter
	if *output != "" {
		writer = &polling.FileArtifactWriter{CapturePath: *output}
	}

	// Create poller
	poller := polling.NewPoller(client, writer)

	// Configure polling
	cfg := polling.CapturePollConfig{
		Interval:       2 * time.Second,
		Timeout:        *timeout,
		RequireCapture: *requireCaptured,
	}

	// Poll
	ctx, cancel := context.WithTimeout(context.Background(), *timeout+5*time.Second)
	defer cancel()

	result := poller.PollCaptureForEvent(ctx, *target, *eventID, cfg)

	// Output result as JSON for shell consumption
	outputResult(result, *output)

	if result.Error != nil {
		log.Printf("ERROR: capture: %v", result.Error)
		return 3
	}
	if result.Timeout {
		if result.LastError != nil {
			log.Printf("ERROR: capture: timeout after %v (last API error: %v)", *timeout, result.LastError)
		} else {
			log.Printf("ERROR: capture: timeout after %v", *timeout)
		}
		return 1
	}
	if !result.OK {
		if result.FailureReason != "" {
			log.Printf("ERROR: capture: %s", result.FailureReason)
		} else {
			log.Printf("ERROR: capture: event_id=%s capture_status=%s (not captured)", *eventID, result.CaptureStatus)
		}
		return 1
	}

	fmt.Printf("OK: capture found (event_id=%s, capture_status=%s, raw_status=%s)\n", *eventID, result.CaptureStatus, result.RawStatus)
	fmt.Printf("MATCHED_CAPTURE_JSON=%s\n", marshalCaptureJSON(result))
	return 0
}

func marshalCaptureJSON(result polling.CaptureResult) string {
	capture := map[string]interface{}{
		"capture_status":    result.RawStatus,
		"status":            result.DiagStatus,
		"capture_started_at": result.CaptureStarted,
	}
	data, _ := json.Marshal(capture)
	return string(data)
}

func outputResult(result interface{}, artifactPath string) {
	// For debugging, print result summary
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return
	}
	// Only print to stderr for debugging, not stdout (which shell uses)
	log.Printf("DEBUG: result=%s", string(data))
}
