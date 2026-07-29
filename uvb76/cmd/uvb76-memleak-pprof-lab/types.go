package main

import "time"

// LabResult captures the lab outcome with full cross-component verification.
type LabResult struct {
	// Primary success indicators
	OK bool `json:"ok"`

	// Classification of the run result
	// Allowed values: OBSERVED, PARTIAL, FAILED
	// Forbidden: stable, growing, leak, bounded, resource_growth
	Classification string `json:"classification"`

	// Duration and artifact info
	DurationSeconds int    `json:"duration_seconds"`
	ArtifactDir     string `json:"artifact_dir"`

	// Real Tovarisch lifecycle
	RealTovarischStarted bool     `json:"real_tovarisch_started"`
	RealTovarischReady   bool     `json:"real_tovarisch_ready"`
	TovarischPID         int      `json:"tovarisch_pid,omitempty"`
	TovarischBinPath     string   `json:"tovarisch_bin_path,omitempty"`
	TovarischArgv        []string `json:"tovarisch_argv,omitempty"` // P0-3: Command arguments for identity

	// Real UVB-76 lifecycle
	RealUVB76Started bool     `json:"real_uvb76_started"`
	UVB76PProfReady  bool     `json:"uvb76_pprof_ready"`
	UVB76PID         int      `json:"uvb76_pid,omitempty"`
	UVB76BinPath     string   `json:"uvb76_bin_path,omitempty"`
	UVB76Argv        []string `json:"uvb76_argv,omitempty"` // P0-3: Command arguments for identity

	// Cross-component interaction
	RealTargetObserved bool `json:"real_tovarisch_target_observed"`
	ScrapeAttempted    bool `json:"scrape_attempted"`
	ScrapeCompleted    bool `json:"scrape_completed"`

	// Collection results
	ProcessSamplesPresent bool `json:"process_samples_present_for_both"`
	ProfilesPresent       bool `json:"baseline_and_final_profiles_present"`

	// Cleanup results
	UVB76Removed     bool `json:"uvb76_removed"`
	TovarischRemoved bool `json:"tovarisch_removed"`
	PortsReleased    bool `json:"ports_released"`

	// Timing information
	TovarischStartTime  *time.Time `json:"tovarisch_start_time,omitempty"`
	TovarischReadyTime  *time.Time `json:"tovarisch_ready_time,omitempty"`
	UVB76StartTime      *time.Time `json:"uvb76_start_time,omitempty"`
	UVB76PProfReadyTime *time.Time `json:"uvb76_pprof_ready_time,omitempty"`
	CollectionStartTime *time.Time `json:"collection_start_time,omitempty"`
	CollectionEndTime   *time.Time `json:"collection_end_time,omitempty"`

	// Error tracking
	Errors []string `json:"errors,omitempty"`

	// Runtime artifact info (not durable gate evidence)
	RuntimeArtifactsDir string `json:"runtime_artifacts_dir,omitempty"`
}

// ProcessIdentity records metadata about a started process.
type ProcessIdentity struct {
	ExecutablePath string    `json:"executable_path"`
	Argv           []string  `json:"argv"`
	PID            int       `json:"pid"`
	StartTime      time.Time `json:"start_time"`
	Port           string    `json:"port"`
}

// ReadinessResult captures readiness check outcomes.
type ReadinessResult struct {
	TovarischReady    bool       `json:"tovarisch_ready"`
	TovarischReadyAt  *time.Time `json:"tovarisch_ready_at,omitempty"`
	UVB76PProfReady   bool       `json:"uvb76_pprof_ready"`
	UVB76PProfReadyAt *time.Time `json:"uvb76_pprof_ready_at,omitempty"`
	Errors            []string   `json:"errors,omitempty"`
}

// ProcessSample records a single process measurement snapshot.
// P0-5: Removed zero placeholder Go metrics (HeapAlloc, HeapInuse, HeapObjects, NumGC)
// until a real parser exists. GoroutineCount is kept as it has a real authority.
type ProcessSample struct {
	Timestamp       time.Time `json:"timestamp"`
	PID             int       `json:"pid"`
	RSSKIB          int64     `json:"rss_kib"`
	VMSizeKIB       int64     `json:"vm_size_kib"`
	PSS_KIB         int64     `json:"pss_kib,omitempty"`
	PSSAnonKIB      int64     `json:"pss_anon_kib,omitempty"`
	PrivateDirtyKIB int64     `json:"private_dirty_kib,omitempty"`
	AnonymousKIB    int64     `json:"anonymous_kib,omitempty"`
	Threads         int       `json:"threads"`
	FDCount         int       `json:"fd_count"`
	// UVB-76 specific
	GoroutineCount int64 `json:"goroutine_count,omitempty"`
}

// TargetObservation records a single scrape observation from UVB-76.
type TargetObservation struct {
	Timestamp  time.Time `json:"timestamp"`
	TargetID   string    `json:"target_id"`
	Reachable  bool      `json:"reachable"`
	Status     string    `json:"status,omitempty"`
	Version    string    `json:"version,omitempty"`
	NodeID     string    `json:"node_id,omitempty"`
	ScrapedURL string    `json:"scraped_url"`
	Error      string    `json:"error,omitempty"`
}
