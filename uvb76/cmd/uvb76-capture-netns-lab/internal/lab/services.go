package lab

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// startTovarisch starts tovarisch inside its namespace.
func (o *Orchestrator) startTovarisch(ctx context.Context) error {
	binary := o.Options.TovarischBin
	if binary == "" {
		binary = "./tovarisch/zig-out/bin/tovarisch"
	}

	configPath := filepath.Join(o.labDir, "tovarisch.conf")
	stdoutLog := filepath.Join(o.labDir, "tovarisch.log")
	stderrLog := filepath.Join(o.labDir, "tovarisch.stderr.log")

	// Use ProcessRunner to start tovarisch inside its namespace
	// Run via ip netns exec to enter the namespace
	args := []string{"netns", "exec", o.Config.TovarischNS.Name, binary, "serve", "--config", configPath, "--listen-all-public-dangerous"}
	handle, res := o.ProcessRunner.StartWithLogs(ctx, "ip", args, stdoutLog, stderrLog)

	if res.Err != nil {
		return fmt.Errorf("start tovarisch: %w", res.Err)
	}

	o.tovarischHandle = handle

	// Wait a bit for startup
	time.Sleep(5 * time.Second)

	log.Printf("Tovarisch started with PID %d (logs: %s, %s)", handle.PID, stdoutLog, stderrLog)
	return nil
}

// startUVB76 starts uvb76 inside its namespace.
func (o *Orchestrator) startUVB76(ctx context.Context) error {
	binary := o.Options.UVB76Bin
	if binary == "" {
		binary = "./uvb76/uvb76"
	}

	configPath := filepath.Join(o.labDir, "uvb76.json")
	stdoutLog := filepath.Join(o.labDir, "uvb76.log")
	stderrLog := filepath.Join(o.labDir, "uvb76.stderr.log")

	// Use ProcessRunner to start uvb76 inside its namespace
	args := []string{"netns", "exec", o.Config.UVB76NS.Name, binary, "-dev", "-config", configPath}
	handle, res := o.ProcessRunner.StartWithLogs(ctx, "ip", args, stdoutLog, stderrLog)

	if res.Err != nil {
		return fmt.Errorf("start uvb76: %w", res.Err)
	}

	o.uvb76Handle = handle

	// Wait a bit for startup
	time.Sleep(5 * time.Second)

	log.Printf("UVB-76 started with PID %d (logs: %s, %s)", handle.PID, stdoutLog, stderrLog)
	return nil
}

// waitForTovarischHTTP waits for tovarisch HTTP endpoint to be ready.
func (o *Orchestrator) waitForTovarischHTTP(ctx context.Context) error {
	maxAttempts := 10
	attempt := 0

	for attempt < maxAttempts {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		res := o.runNsCommand(ctx, o.Config.TovarischNS.Name, "curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
			fmt.Sprintf("http://localhost:%d/status.json", o.Config.TovarischPort))

		if res.OK() && strings.Contains(res.Stdout, "200") {
			log.Printf("Tovarisch HTTP endpoint is ready")
			return nil
		}

		time.Sleep(1 * time.Second)
		attempt++
	}

	return fmt.Errorf("tovarisch HTTP endpoint did not become ready")
}

// uvb76Authenticate authenticates to the UVB-76 API.
func (o *Orchestrator) uvb76Authenticate(ctx context.Context) error {
	authData := fmt.Sprintf(`{"username":"%s","password":"%s"}`, o.Options.APIUser, o.Options.APIPass)

	res := o.runNsCommand(ctx, o.Config.UVB76NS.Name, "curl", "-s", "-c", "/tmp/uvb76-cookies.txt", "-b", "/tmp/uvb76-cookies.txt",
		"-X", "POST",
		"-H", "Content-Type: application/json",
		"-d", authData,
		o.uvb76APIBase+"/api/v1/auth/login")

	if !res.OK() {
		return fmt.Errorf("authenticate failed: %s", res.Stderr)
	}

	// Check if auth was successful
	if strings.Contains(res.Stdout, `"success":true`) {
		log.Printf("UVB-76 authentication successful")
		o.uvb76AuthCookie = "/tmp/uvb76-cookies.txt"
		return nil
	}

	return fmt.Errorf("authentication failed: %s", res.Stdout)
}

// phase0Readiness checks baseline probe readiness.
func (o *Orchestrator) phase0Readiness(ctx context.Context) error {
	// Save phase0 status
	res := o.runNsCommand(ctx, o.Config.UVB76NS.Name, "curl", "-s", "-b", o.uvb76AuthCookie,
		o.uvb76APIBase+"/api/v1/status")

	statusPath := filepath.Join(o.labDir, "phase0-status.json")
	if err := os.WriteFile(statusPath, []byte(res.Stdout), 0644); err != nil {
		log.Printf("Warning: failed to write phase0-status.json: %v", err)
	}

	// Poll for probe samples (wait for HTTP probe loop to be running)
	log.Printf("Waiting for probe samples...")

	pollCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-pollCtx.Done():
			log.Printf("Phase 0 readiness timeout")
			return nil
		case <-ticker.C:
			res := o.runNsCommand(ctx, o.Config.UVB76NS.Name, "curl", "-s", "-b", o.uvb76AuthCookie,
				o.uvb76APIBase+"/api/v1/latency?target_id=lab-tovarisch&kind=http&range_seconds=120")

			if res.OK() && strings.Contains(res.Stdout, `"samples"`) {
				log.Printf("Probe samples found")

				// Save probe ready artifact
				probeReadyPath := filepath.Join(o.labDir, "phase0-probe-ready.json")
				if err := os.WriteFile(probeReadyPath, []byte(res.Stdout), 0644); err != nil {
					log.Printf("Warning: failed to write phase0-probe-ready.json: %v", err)
				}
				return nil
			}
		}
	}
}

// stopUVB76 stops the uvb76 process.
func (o *Orchestrator) stopUVB76(ctx context.Context) error {
	if o.uvb76Handle != nil && o.uvb76Handle.StopFn != nil {
		if err := o.uvb76Handle.StopFn(ctx); err != nil {
			return fmt.Errorf("kill uvb76: %w", err)
		}
	}
	return nil
}

// stopTovarisch stops the tovarisch process.
func (o *Orchestrator) stopTovarisch(ctx context.Context) error {
	if o.tovarischHandle != nil && o.tovarischHandle.StopFn != nil {
		if err := o.tovarischHandle.StopFn(ctx); err != nil {
			return fmt.Errorf("kill tovarisch: %w", err)
		}
	}
	return nil
}
