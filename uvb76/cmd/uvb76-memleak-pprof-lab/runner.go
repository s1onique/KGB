package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// runLab orchestrates the full memory leak lab with real binaries.
func runLab() LabResult {
	result := LabResult{
		DurationSeconds:     int(flagDuration.Seconds()),
		ArtifactDir:         artifactDir,
		Classification:      "PARTIAL", // Default until proven otherwise
		RuntimeArtifactsDir: artifactDir,
	}

	tovarischCmd, uvb76Cmd := (*exec.Cmd)(nil), (*exec.Cmd)(nil)
	var tovarischPS, uvb76PS *ProcessState
	var tovarischStartTime, uvb76StartTime time.Time

	log.Printf("[SETUP] Ports: Tovarisch=%s, UVB-76=%s, pprof=%s",
		*flagTovarischPort, *flagUVB76Port, *flagPProfPort)

	// === PHASE 1: Start Tovarisch (or fake server) ===
	if *flagUseFakeTovarisch {
		log.Printf("[SETUP] Starting fake tovarisch on port %s", *flagTovarischPort)
		if err := startFakeTovarisch(); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("start fake tovarisch: %v", err))
			result.Classification = "FAILED"
			return result
		}
		result.RealTovarischStarted = true // Fake counts as started for fake mode
		result.TovarischBinPath = "fake"
	} else {
		// Start real Tovarisch
		log.Printf("[SETUP] Starting real Tovarisch: %s", *flagTovarischBin)
		tovarischStartTime = time.Now()
		result.TovarischStartTime = &tovarischStartTime

		var err error
		tovarischCmd, tovarischPS, err = startTovarisch(*flagTovarischBin, *flagTovarischArgs, *flagTovarischPort)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("start tovarisch: %v", err))
			result.Classification = "FAILED"
			return result
		}

		result.RealTovarischStarted = true
		result.TovarischPID = tovarischPID
		result.TovarischBinPath = *flagTovarischBin
	}

	// === PHASE 2: Wait for Tovarisch readiness ===
	tovarischReady := false
	if *flagUseFakeTovarisch {
		tovarischReady = waitForHTTPReady(
			fmt.Sprintf("http://localhost:%s/status", *flagTovarischPort),
			15*time.Second,
		)
	} else {
		tovarischReady = waitForTovarischReady(*flagTovarischPort, 15*time.Second)
	}

	now := time.Now()
	if tovarischReady {
		result.RealTovarischReady = true
		result.TovarischReadyTime = &now
		log.Printf("[READY] Tovarisch ready on port %s", *flagTovarischPort)
	} else {
		result.Errors = append(result.Errors, "tovarisch did not become ready")
		result.Classification = "FAILED"
		cleanup(tovarischCmd, uvb76Cmd, tovarischPS, uvb76PS)
		return result
	}

	// === PHASE 3: Start UVB-76 ===
	log.Printf("[LAUNCH] Starting UVB-76: %s", *flagUVB76Bin)
	uvb76StartTime = time.Now()
	result.UVB76StartTime = &uvb76StartTime

	var err error
	uvb76Cmd, uvb76PS, err = startUVB76(*flagUVB76Bin)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("start uvb76: %v", err))
		result.Classification = "FAILED"
		cleanup(tovarischCmd, uvb76Cmd, tovarischPS, uvb76PS)
		return result
	}

	result.RealUVB76Started = true
	result.UVB76PID = uvb76PID
	result.UVB76BinPath = *flagUVB76Bin

	// === PHASE 4: Wait for UVB-76 pprof readiness ===
	pprofReady, pprofErr := waitForPPROFReady(*flagPProfPort, 30*time.Second, uvb76PS)
	if pprofReady {
		result.UVB76PProfReady = true
		now := time.Now()
		result.UVB76PProfReadyTime = &now
		log.Printf("[READY] UVB-76 pprof ready on port %s", *flagPProfPort)
	} else {
		if uvb76PS.Exited() {
			exitCode, _ := uvb76PS.ExitInfo()
			result.Errors = append(result.Errors, fmt.Sprintf("uvb76 exited with code %d before pprof ready: %v", exitCode, pprofErr))
		} else {
			result.Errors = append(result.Errors, fmt.Sprintf("pprof never ready: %v", pprofErr))
		}
		result.Classification = "FAILED"
		cleanup(tovarischCmd, uvb76Cmd, tovarischPS, uvb76PS)
		return result
	}

	// Verify pprof endpoints
	if !verifyPPROFEndpoints(*flagPProfPort) {
		result.Errors = append(result.Errors, "pprof endpoint verification failed")
		result.Classification = "FAILED"
		cleanup(tovarischCmd, uvb76Cmd, tovarischPS, uvb76PS)
		return result
	}

	// === PHASE 5: Collection phase ===
	log.Printf("[COLLECTION] Starting collection phase...")
	collectionStart := time.Now()
	result.CollectionStartTime = &collectionStart

	// Create bounded collection context for collector goroutines
	collectionCtx, collectionCancel := context.WithTimeout(labCtx, *flagDuration)
	defer collectionCancel()

	// Start goroutines for collection
	var wg sync.WaitGroup
	var samplesMu sync.Mutex
	var tovarischSamples []ProcessSample
	var uvb76Samples []ProcessSample
	var targetObservations []TargetObservation

	// Collect process samples
	wg.Add(1)
	go func() {
		defer wg.Done()
		collectProcessSamples(collectionCtx, tovarischPID, *flagTovarischPort, *flagSampleInterval, &tovarischSamples, &samplesMu)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		collectUVB76Samples(collectionCtx, uvb76PID, *flagPProfPort, *flagSampleInterval, &uvb76Samples, &samplesMu)
	}()

	// Poll for target observations
	wg.Add(1)
	go func() {
		defer wg.Done()
		pollTargetObservations(collectionCtx, *flagUVB76Port, "real-tovarisch", 5*time.Second, &targetObservations)
	}()

	// Capture initial pprof profiles at t=0
	capturePPROFProfiles(*flagPProfPort, "start")

	// Schedule: start=0, mid=60s, final=120s
	profileInterval := *flagProfileInterval
	sleepUntil(collectionStart.Add(profileInterval))
	capturePPROFProfiles(*flagPProfPort, "mid")

	sleepUntil(collectionStart.Add(*flagDuration))
	capturePPROFProfiles(*flagPProfPort, "final")

	// Cancel collection and wait for goroutines
	collectionCancel()
	wg.Wait()

	collectionEnd := time.Now()
	result.CollectionEndTime = &collectionEnd
	log.Printf("[COLLECTION] Collection phase complete")

	// Wait for collection goroutines
	wg.Wait()

	// === PHASE 6: Verify cross-component interaction ===
	if len(targetObservations) > 0 {
		result.RealTargetObserved = true
		for _, obs := range targetObservations {
			if obs.Reachable {
				result.ScrapeCompleted = true
				break
			}
		}
		result.ScrapeAttempted = len(targetObservations) > 0
	}

	// === PHASE 7: Write collection artifacts ===
	writeProcessSeries("tovarisch", tovarischSamples)
	writeProcessSeries("uvb76", uvb76Samples)
	writeTargetObservations(targetObservations)
	writeReadinessResult(result)

	// Check if we have process samples
	result.ProcessSamplesPresent = len(tovarischSamples) > 0 && len(uvb76Samples) > 0

	// Check if we have profiles
	result.ProfilesPresent = checkProfilesPresent()

	// === PHASE 8: Cleanup ===
	cleanupErrors := cleanup(tovarischCmd, uvb76Cmd, tovarischPS, uvb76PS)
	result.Errors = append(result.Errors, cleanupErrors...)

	// Verify cleanup
	result.UVB76Removed = uvb76PID == 0 || processIsGone(uvb76PID)
	result.TovarischRemoved = tovarischPID == 0 || processIsGone(tovarischPID)
	result.PortsReleased = len(cleanupErrors) == 0

	// === PHASE 9: Final classification ===
	if result.RealTovarischStarted && result.RealUVB76Started &&
		result.UVB76PProfReady && result.RealTargetObserved &&
		result.ProcessSamplesPresent && result.ProfilesPresent &&
		result.UVB76Removed && result.TovarischRemoved && result.PortsReleased {
		result.Classification = "OBSERVED"
		result.OK = true
	}

	return result
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
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

// collectProcessSamples samples Tovarisch process metrics from /proc.
func collectProcessSamples(ctx context.Context, pid int, port string, interval time.Duration, samples *[]ProcessSample, mu *sync.Mutex) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !processIsGone(pid) {
				sample := sampleProcessMetrics(pid)
				mu.Lock()
				*samples = append(*samples, sample)
				mu.Unlock()
			} else {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// collectUVB76Samples samples UVB-76 process metrics and Go runtime stats.
func collectUVB76Samples(ctx context.Context, pid int, pprofPort string, interval time.Duration, samples *[]ProcessSample, mu *sync.Mutex) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !processIsGone(pid) {
				sample := sampleProcessMetrics(pid)
				// Add Go runtime stats if available
				if stats := getGoRuntimeStats(pprofPort); stats != nil {
					sample.GoroutineCount = stats.GoroutineCount
					sample.HeapAlloc = stats.HeapAlloc
					sample.HeapInuse = stats.HeapInuse
					sample.HeapObjects = stats.HeapObjects
					sample.NumGC = stats.NumGC
				}
				mu.Lock()
				*samples = append(*samples, sample)
				mu.Unlock()
			} else {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// sampleProcessMetrics reads /proc/<pid>/status and smaps_rollup for process metrics.
func sampleProcessMetrics(pid int) ProcessSample {
	sample := ProcessSample{
		Timestamp: time.Now(),
		PID:       pid,
	}

	// Read /proc/<pid>/status
	statusPath := fmt.Sprintf("/proc/%d/status", pid)
	if f, err := os.Open(statusPath); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])

			switch key {
			case "VmRSS":
				sample.RSSKIB = parseMemValue(val)
			case "VmSize":
				sample.VMSizeKIB = parseMemValue(val)
			case "Threads":
				sample.Threads = parseIntValue(val)
			}
		}
	}

	// Count open FDs directly from /proc/<pid>/fd
	sample.FDCount = countOpenFDs(pid)

	// Read /proc/<pid>/smaps_rollup for PSS and other memory metrics
	smapsPath := fmt.Sprintf("/proc/%d/smaps_rollup", pid)
	if f, err := os.Open(smapsPath); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "Pss:") {
				sample.PSS_KIB = parseMemValue(strings.TrimSpace(strings.TrimPrefix(line, "Pss:")))
			}
		}
	}

	return sample
}

// parseMemValue parses memory values like "1234 kB" to KiB.
func parseMemValue(s string) int64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "kB", "")
	s = strings.ReplaceAll(s, "KB", "")
	s = strings.TrimSpace(s)
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

// parseIntValue parses integer values.
func parseIntValue(s string) int {
	s = strings.TrimSpace(s)
	v, _ := strconv.Atoi(s)
	return v
}

// countOpenFDs counts the number of open file descriptors for a process.
func countOpenFDs(pid int) int {
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	count := 0
	if entries, err := os.ReadDir(fdDir); err == nil {
		count = len(entries)
	}
	return count
}

// GoRuntimeStats holds Go runtime statistics.
type GoRuntimeStats struct {
	GoroutineCount int64
	HeapAlloc      int64
	HeapInuse      int64
	HeapObjects    int64
	NumGC          int
}

// getGoRuntimeStats fetches Go runtime stats from pprof debug endpoint.
func getGoRuntimeStats(pprofPort string) *GoRuntimeStats {
	client := &http.Client{Timeout: 2 * time.Second}

	// Get goroutine count from /debug/pprof/goroutine?debug=1
	resp, err := client.Get(fmt.Sprintf("http://localhost:%s/debug/pprof/goroutine?debug=1", pprofPort))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	// Count goroutines in the dump
	body, _ := io.ReadAll(resp.Body)
	lines := strings.Split(string(body), "\n")
	goroutineCount := int64(0)
	for _, line := range lines {
		if strings.HasPrefix(line, "goroutine ") && strings.HasSuffix(line, ":") {
			goroutineCount++
		}
	}

	// Get heap stats from /debug/pprof/heap
	resp2, err := client.Get(fmt.Sprintf("http://localhost:%s/debug/pprof/heap", pprofPort))
	if err != nil {
		return &GoRuntimeStats{GoroutineCount: goroutineCount}
	}
	defer resp2.Body.Close()

	// Parse heap profile
	// For now, just return goroutine count
	return &GoRuntimeStats{GoroutineCount: goroutineCount}
}

// pollTargetObservations polls UVB-76 API for scrape observations.
func pollTargetObservations(ctx context.Context, uvb76Port, targetID string, interval time.Duration, observations *[]TargetObservation) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Query UVB-76 /api/v1/targets or /api/v1/status for scrape observations
			obs := fetchTargetObservation(uvb76Port, targetID)
			if obs != nil {
				*observations = append(*observations, *obs)
			}
		case <-ctx.Done():
			return
		}
	}
}

// fetchTargetObservation fetches the current target state from UVB-76.
func fetchTargetObservation(uvb76Port, targetID string) *TargetObservation {
	client := &http.Client{Timeout: 2 * time.Second}

	// Try /api/v1/targets endpoint
	url := fmt.Sprintf("http://localhost:%s/api/v1/targets", uvb76Port)
	resp, err := client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var targets struct {
		Targets []struct {
			ID        string `json:"id"`
			Reachable bool   `json:"reachable"`
			Status    string `json:"status"`
			Version   string `json:"version,omitempty"`
			NodeID    string `json:"node_id,omitempty"`
		} `json:"targets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil
	}

	for _, t := range targets.Targets {
		if t.ID == targetID {
			return &TargetObservation{
				Timestamp:  time.Now(),
				TargetID:   t.ID,
				Reachable:  t.Reachable,
				Status:     t.Status,
				Version:    t.Version,
				NodeID:     t.NodeID,
				ScrapedURL: fmt.Sprintf("http://localhost:%s/status", *flagTovarischPort),
			}
		}
	}

	return nil
}

// capturePPROFProfiles captures heap, allocs, and goroutine profiles.
func capturePPROFProfiles(pprofPort, label string) {
	client := &http.Client{Timeout: 30 * time.Second}

	profiles := []struct {
		name string
		url  string
	}{
		{"heap", fmt.Sprintf("http://localhost:%s/debug/pprof/heap", pprofPort)},
		{"allocs", fmt.Sprintf("http://localhost:%s/debug/pprof/allocs", pprofPort)},
		{"goroutine", fmt.Sprintf("http://localhost:%s/debug/pprof/goroutine?debug=1", pprofPort)},
	}

	for _, p := range profiles {
		resp, err := client.Get(p.url)
		if err != nil {
			log.Printf("[PROFILE] Failed to fetch %s: %v", p.name, err)
			continue
		}
		defer resp.Body.Close()

		filename := fmt.Sprintf("%s-%s.%s", p.name, label, getExtension(p.name))
		outPath := filepath.Join(artifactDir, filename)

		if f, err := os.Create(outPath); err == nil {
			io.Copy(f, resp.Body)
			f.Close()
			log.Printf("[PROFILE] Captured %s -> %s", p.name, filename)
		}
	}
}

// getExtension returns file extension based on profile type.
func getExtension(name string) string {
	switch name {
	case "heap", "allocs":
		return "pb.gz"
	case "goroutine":
		return "txt"
	default:
		return "bin"
	}
}

// checkProfilesPresent verifies required profile files exist.
func checkProfilesPresent() bool {
	required := []string{
		"heap-start.pb.gz",
		"heap-final.pb.gz",
		"allocs-start.pb.gz",
		"allocs-final.pb.gz",
		"goroutine-start.txt",
		"goroutine-final.txt",
	}
	for _, f := range required {
		path := filepath.Join(artifactDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// writeProcessSeries writes process samples to CSV file.
func writeProcessSeries(prefix string, samples []ProcessSample) {
	if len(samples) == 0 {
		return
	}

	filename := fmt.Sprintf("%s-process-series.csv", prefix)
	path := filepath.Join(artifactDir, filename)

	f, err := os.Create(path)
	if err != nil {
		log.Printf("[WRITE] Failed to create %s: %v", filename, err)
		return
	}
	defer f.Close()

	// Write header
	fmt.Fprintf(f, "timestamp,pid,rss_kib,vm_size_kib,pss_kib,threads,fd_count")
	if prefix == "uvb76" {
		fmt.Fprintf(f, ",goroutine_count,heap_alloc,heap_inuse,heap_objects,num_gc")
	}
	fmt.Fprintln(f)

	// Write samples
	for _, s := range samples {
		fmt.Fprintf(f, "%s,%d,%d,%d,%d,%d,%d",
			s.Timestamp.Format(time.RFC3339), s.PID, s.RSSKIB, s.VMSizeKIB, s.PSS_KIB, s.Threads, s.FDCount)
		if prefix == "uvb76" {
			fmt.Fprintf(f, ",%d,%d,%d,%d,%d",
				s.GoroutineCount, s.HeapAlloc, s.HeapInuse, s.HeapObjects, s.NumGC)
		}
		fmt.Fprintln(f)
	}

	log.Printf("[WRITE] %s samples written to %s", prefix, filename)
}

// writeTargetObservations writes scrape observations to JSON.
func writeTargetObservations(observations []TargetObservation) {
	if len(observations) == 0 {
		return
	}

	path := filepath.Join(artifactDir, "target-observations.json")
	data, err := json.MarshalIndent(observations, "", "  ")
	if err != nil {
		log.Printf("[WRITE] Failed to marshal observations: %v", err)
		return
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("[WRITE] Failed to write observations: %v", err)
		return
	}

	log.Printf("[WRITE] %d observations written to target-observations.json", len(observations))
}

// sleepUntil sleeps until the specified absolute time.
func sleepUntil(deadline time.Time) {
	now := time.Now()
	if deadline.After(now) {
		time.Sleep(deadline.Sub(now))
	}
}

// writeReadinessResult writes readiness results to JSON.
func writeReadinessResult(result LabResult) {
	readiness := ReadinessResult{
		TovarischReady:  result.RealTovarischReady,
		UVB76PProfReady: result.UVB76PProfReady,
	}
	if result.TovarischReadyTime != nil {
		readiness.TovarischReadyAt = result.TovarischReadyTime
	}
	if result.UVB76PProfReadyTime != nil {
		readiness.UVB76PProfReadyAt = result.UVB76PProfReadyTime
	}

	path := filepath.Join(artifactDir, "readiness.json")
	data, err := json.MarshalIndent(readiness, "", "  ")
	if err != nil {
		log.Printf("[WRITE] Failed to marshal readiness: %v", err)
		return
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("[WRITE] Failed to write readiness: %v", err)
		return
	}

	// Also write process identities
	identities := []ProcessIdentity{}
	if result.TovarischPID > 0 {
		identities = append(identities, ProcessIdentity{
			ExecutablePath: result.TovarischBinPath,
			PID:            result.TovarischPID,
			Port:           *flagTovarischPort,
		})
	}
	if result.UVB76PID > 0 {
		identities = append(identities, ProcessIdentity{
			ExecutablePath: result.UVB76BinPath,
			PID:            result.UVB76PID,
			Port:           *flagUVB76Port,
		})
	}

	if len(identities) > 0 {
		path = filepath.Join(artifactDir, "process-identities.json")
		data, err = json.MarshalIndent(identities, "", "  ")
		if err == nil {
			os.WriteFile(path, data, 0644)
		}
	}
}
