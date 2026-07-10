package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// startUVB76 starts UVB-76 as a child process with proper output capture.
func startUVB76(bin string, processState *ProcessState) (*exec.Cmd, error) {
	args := []string{
		"-dev",
		"-config", configFile,
	}

	cmd := exec.Command(bin, args...)

	// Create log file for stdout/stderr capture
	logOut, err := os.OpenFile(uvb76LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	// Capture output to file only (not teeing to console)
	cmd.Stdout = logOut
	cmd.Stderr = logOut

	if err := cmd.Start(); err != nil {
		logOut.Close()
		return nil, fmt.Errorf("start: %w", err)
	}
	uvb76PID = cmd.Process.Pid

	// Write PID file
	pidFile := filepath.Join(artifactDir, "uvb76.pid")
	os.WriteFile(pidFile, []byte(strconv.Itoa(uvb76PID)), 0644)

	// Initialize done channel
	processState.done = make(chan struct{})

	// Mark as running
	processState.mu.Lock()
	processState.running = true
	processState.exited = false
	processState.mu.Unlock()

	// Monitor process using the original exec.Cmd, not FindProcess
	// IMPORTANT: Only this goroutine calls cmd.Wait() - multiple calls are not allowed
	go func() {
		_ = cmd.Wait()

		// Close log file to flush and release FD
		_ = logOut.Close()

		processState.mu.Lock()
		defer processState.mu.Unlock()

		processState.running = false
		processState.exited = true // Always mark as exited after Wait()

		// Get exit info from cmd.ProcessState after Wait()
		if cmd.ProcessState != nil {
			if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
				if ws.Signaled() {
					processState.exitCode = -1
					processState.signal = ws.Signal()
				} else {
					processState.exitCode = ws.ExitStatus()
				}
			} else {
				// Fallback for non-syscall platforms
				processState.exitCode = cmd.ProcessState.ExitCode()
			}
		}

		// Signal completion to any waiters
		close(processState.done)
	}()

	log.Printf("[LAUNCH] Process started: PID=%d, log=%s", uvb76PID, uvb76LogFile)

	return cmd, nil
}

// gracefulShutdown sends SIGTERM and waits for exit via ProcessState.done channel.
// Does NOT call cmd.Wait() - that is owned by the monitor goroutine.
func gracefulShutdown(cmd *exec.Cmd, ps *ProcessState, timeout time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return fmt.Errorf("no process to shut down")
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return err
	}

	// Wait using the done channel (not cmd.Wait())
	select {
	case <-ps.done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("process did not exit after SIGTERM within %v", timeout)
	}
}

// forceKill sends SIGKILL to the process.
func forceKill(cmd *exec.Cmd, ps *ProcessState) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	cmd.Process.Kill()
	// Wait for the monitor goroutine to complete
	<-ps.done
}

// findUVB76Binary locates the UVB-76 binary.
func findUVB76Binary() string {
	// Check UVB76_BINARY env var first
	if bin := os.Getenv("UVB76_BINARY"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}

	// Check common paths
	paths := []string{
		"./uvb76",
		"../../uvb76",
		"/usr/local/bin/uvb76",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return "uvb76"
}

// waitForHTTPReady waits for an HTTP endpoint to be ready.
func waitForHTTPReady(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return true
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// waitForPPROFReady loops until pprof is reachable or timeout/proc exit.
func waitForPPROFReady(port string, timeout time.Duration, processState *ProcessState) (bool, error) {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://localhost:%s/debug/pprof/heap", port)
	client := &http.Client{Timeout: 2 * time.Second}

	for {
		// Check if process exited
		if processState.Exited() {
			exitCode, _ := processState.ExitInfo()
			return false, fmt.Errorf("process exited with code %d", exitCode)
		}

		// Check deadline
		if time.Now().After(deadline) {
			return false, fmt.Errorf("timeout after %v", timeout)
		}

		// Try pprof
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return true, nil
			}
		}

		time.Sleep(250 * time.Millisecond)
	}
}

// verifyEndpoints checks all required pprof endpoints are responding.
func verifyEndpoints(pprofPort string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	checks := []struct {
		name string
		url  string
	}{
		{"pprof index", fmt.Sprintf("http://localhost:%s/debug/pprof/", pprofPort)},
		{"pprof heap", fmt.Sprintf("http://localhost:%s/debug/pprof/heap", pprofPort)},
		{"pprof cmdline", fmt.Sprintf("http://localhost:%s/debug/pprof/cmdline", pprofPort)},
	}

	for _, check := range checks {
		resp, err := client.Get(check.url)
		if err != nil {
			log.Printf("[READINESS] %s unreachable: %v", check.name, err)
			return false
		}
		resp.Body.Close()
		if resp.StatusCode >= 500 {
			log.Printf("[READINESS] %s returned %d", check.name, resp.StatusCode)
			return false
		}
		log.Printf("[READINESS] %s OK", check.name)
	}

	return true
}

// cleanup stops child processes and fake server.
func cleanup(cmd *exec.Cmd, ps *ProcessState) {
	// Shutdown fake server gracefully first
	if fakeServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		fakeServer.Shutdown(ctx)
		fakeServer = nil
	}

	// Then kill uvb76 process
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Signal(syscall.SIGTERM)
		time.Sleep(2 * time.Second)
		cmd.Process.Kill()
		// Wait for monitor goroutine
		if ps != nil {
			<-ps.done
		}
	}
}
