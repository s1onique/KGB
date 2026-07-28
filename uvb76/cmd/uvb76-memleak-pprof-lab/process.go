package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-memleak-pprof-lab/internal/fake"
)

// startFakeTovarisch creates and starts the fake tovarisch status server.
func startFakeTovarisch() error {
	// Create log file
	logOut, err := os.OpenFile(tovarischLogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open tovarisch log: %w", err)
	}

	// Create fake server instance
	fakeServer = &fake.StatusServer{
		Port:    *flagTovarischPort,
		LogFile: tovarischLogFile,
	}

	// Start the server
	if err := fakeServer.Start(); err != nil {
		logOut.Close()
		return fmt.Errorf("start fake server: %w", err)
	}

	// Write PID file with fake PID
	fakePID := 99999
	tovarischPID = fakePID
	pidFile := filepath.Join(artifactDir, "tovarisch.pid")
	os.WriteFile(pidFile, []byte(strconv.Itoa(fakePID)), 0644)

	log.Printf("[LAUNCH] Fake Tovarisch started on port %s, log=%s", *flagTovarischPort, tovarischLogFile)

	return nil
}

// startTovarisch starts the real Tovarisch binary with serve command.
func startTovarisch(bin string, args string, port string) (*exec.Cmd, *ProcessState, error) {
	// Parse args string into slice (e.g., "serve --listen-private" -> ["serve", "--listen-private"])
	serveArgs := strings.Fields(args)
	if len(serveArgs) == 0 {
		serveArgs = []string{"--listen-private"}
	}

	// Build command: tovarisch <args>
	// The args already include the subcommand (e.g., "serve --listen-private")
	cmdArgs := serveArgs
	// Override port if specified
	cmdArgs = append(cmdArgs, "--listen", "127.0.0.1:"+port)

	cmd := exec.Command(bin, cmdArgs...)
	cmd.Args[0] = bin // Set binary name for ps output

	// Create log file
	logOut, err := os.OpenFile(tovarischLogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("open tovarisch log: %w", err)
	}

	cmd.Stdout = logOut
	cmd.Stderr = logOut

	if err := cmd.Start(); err != nil {
		logOut.Close()
		return nil, nil, fmt.Errorf("start tovarisch: %w", err)
	}
	tovarischPID = cmd.Process.Pid

	// Write PID file
	pidFile := filepath.Join(artifactDir, "tovarisch.pid")
	os.WriteFile(pidFile, []byte(strconv.Itoa(tovarischPID)), 0644)

	// Create process state
	ps := &ProcessState{}
	ps.done = make(chan struct{})

	ps.mu.Lock()
	ps.running = true
	ps.exited = false
	ps.mu.Unlock()

	// Monitor process
	go func() {
		_ = cmd.Wait()
		_ = logOut.Close()

		ps.mu.Lock()
		defer ps.mu.Unlock()

		ps.running = false
		ps.exited = true

		if cmd.ProcessState != nil {
			if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
				if ws.Signaled() {
					ps.exitCode = -1
					ps.signal = ws.Signal()
				} else {
					ps.exitCode = ws.ExitStatus()
				}
			} else {
				ps.exitCode = cmd.ProcessState.ExitCode()
			}
		}

		close(ps.done)
	}()

	log.Printf("[LAUNCH] Tovarisch started: PID=%d, port=%s, log=%s", tovarischPID, port, tovarischLogFile)

	return cmd, ps, nil
}

// startUVB76 starts UVB-76 as a child process with proper output capture.
func startUVB76(bin string) (*exec.Cmd, *ProcessState, error) {
	args := []string{
		"-dev",
		"-config", configFile,
	}

	cmd := exec.Command(bin, args...)

	// Create log file for stdout/stderr capture
	logOut, err := os.OpenFile(uvb76LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	cmd.Stdout = logOut
	cmd.Stderr = logOut

	if err := cmd.Start(); err != nil {
		logOut.Close()
		return nil, nil, fmt.Errorf("start: %w", err)
	}
	uvb76PID = cmd.Process.Pid

	// Write PID file
	pidFile := filepath.Join(artifactDir, "uvb76.pid")
	os.WriteFile(pidFile, []byte(strconv.Itoa(uvb76PID)), 0644)

	// Create process state
	ps := &ProcessState{}
	ps.done = make(chan struct{})

	ps.mu.Lock()
	ps.running = true
	ps.exited = false
	ps.mu.Unlock()

	// Monitor process using the original exec.Cmd
	go func() {
		_ = cmd.Wait()
		_ = logOut.Close()

		ps.mu.Lock()
		defer ps.mu.Unlock()

		ps.running = false
		ps.exited = true

		if cmd.ProcessState != nil {
			if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
				if ws.Signaled() {
					ps.exitCode = -1
					ps.signal = ws.Signal()
				} else {
					ps.exitCode = ws.ExitStatus()
				}
			} else {
				ps.exitCode = cmd.ProcessState.ExitCode()
			}
		}

		close(ps.done)
	}()

	log.Printf("[LAUNCH] UVB-76 started: PID=%d, log=%s", uvb76PID, uvb76LogFile)

	return cmd, ps, nil
}

// gracefulShutdown sends SIGTERM and waits for exit.
func gracefulShutdown(cmd *exec.Cmd, ps *ProcessState, timeout time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return fmt.Errorf("no process to shut down")
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return err
	}

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
	<-ps.done
}

// waitForTovarischReady waits for Tovarisch /status endpoint to be ready.
func waitForTovarischReady(port string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://localhost:%s/status", port)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

// waitForPPROFReady loops until pprof is reachable.
func waitForPPROFReady(port string, timeout time.Duration, ps *ProcessState) (bool, error) {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://localhost:%s/debug/pprof/heap", port)
	client := &http.Client{Timeout: 2 * time.Second}

	for {
		if ps.Exited() {
			exitCode, _ := ps.ExitInfo()
			return false, fmt.Errorf("process exited with code %d", exitCode)
		}

		if time.Now().After(deadline) {
			return false, fmt.Errorf("timeout after %v", timeout)
		}

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

// verifyPPROFEndpoints checks all required pprof endpoints.
func verifyPPROFEndpoints(port string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	checks := []struct {
		name string
		url  string
	}{
		{"pprof index", fmt.Sprintf("http://localhost:%s/debug/pprof/", port)},
		{"pprof heap", fmt.Sprintf("http://localhost:%s/debug/pprof/heap", port)},
		{"pprof goroutine", fmt.Sprintf("http://localhost:%s/debug/pprof/goroutine?debug=1", port)},
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

// cleanup stops all child processes and fake server.
func cleanup(tovarischCmd, uvb76Cmd *exec.Cmd, tovarischPS, uvb76PS *ProcessState) []string {
	var errors []string

	// Shutdown fake server first if running
	if fakeServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := fakeServer.Shutdown(ctx); err != nil {
			errors = append(errors, fmt.Sprintf("fake server shutdown: %v", err))
		}
		fakeServer = nil
	}

	// Stop UVB-76 gracefully
	if uvb76Cmd != nil && uvb76Cmd.Process != nil && uvb76PS != nil {
		if err := gracefulShutdown(uvb76Cmd, uvb76PS, 10*time.Second); err != nil {
			log.Printf("[CLEANUP] UVB-76 graceful shutdown failed, forcing: %v", err)
			forceKill(uvb76Cmd, uvb76PS)
			errors = append(errors, fmt.Sprintf("uvb76 forced kill: %v", err))
		}
		// Verify UVB-76 is gone
		if uvb76PS.Exited() {
			uvb76PID = 0 // Mark as cleaned
		}
	}

	// Stop Tovarisch gracefully
	if tovarischCmd != nil && tovarischCmd.Process != nil && tovarischPS != nil {
		if err := gracefulShutdown(tovarischCmd, tovarischPS, 10*time.Second); err != nil {
			log.Printf("[CLEANUP] Tovarisch graceful shutdown failed, forcing: %v", err)
			forceKill(tovarischCmd, tovarischPS)
			errors = append(errors, fmt.Sprintf("tovarisch forced kill: %v", err))
		}
		// Verify Tovarisch is gone
		if tovarischPS.Exited() {
			tovarischPID = 0 // Mark as cleaned
		}
	}

	// Verify ports are released
	if err := verifyPortsReleased(); err != nil {
		errors = append(errors, fmt.Sprintf("ports not released: %v", err))
	}

	// Clean up PID files
	pidFiles := []string{
		filepath.Join(artifactDir, "tovarisch.pid"),
		filepath.Join(artifactDir, "uvb76.pid"),
	}
	for _, f := range pidFiles {
		os.Remove(f)
	}

	return errors
}

// verifyPortsReleased checks that the selected ports are no longer in use.
func verifyPortsReleased() error {
	ports := []string{*flagTovarischPort, *flagUVB76Port, *flagPProfPort}
	for _, port := range ports {
		addr := fmt.Sprintf("localhost:%s", port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("port %s still in use", port)
		}
		ln.Close()
	}
	return nil
}

// processIsGone verifies a specific PID is no longer running.
func processIsGone(pid int) bool {
	if pid <= 0 {
		return true
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return true
	}
	// On Unix, FindProcess succeeds even for dead processes
	// Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	return err != nil
}
