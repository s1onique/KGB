package lab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Orchestrator manages the complete lab execution lifecycle.
type Orchestrator struct {
	Runner        CommandRunner
	ProcessRunner ProcessRunner
	Netns         *NetnsHelper
	Config        NetnsLabConfig
	Options       OrchestratorConfig
	Artifacts     *LabArtifacts
	Names         ArtifactNames
	Cleanup       *CleanupStack
	Phases        []PhaseConfig
	Outcomes      map[PhaseName]PhaseOutcome
	Tracker       *PhaseTracker
	CommandLog    []CommandLog
	startedAt     time.Time
	tovarischHandle *ProcessHandle
	uvb76Handle    *ProcessHandle
	labDir         string
	uvb76APIBase   string
	tovarischURL   string
	probeReady     bool
	uvb76AuthCookie string
}

// OrchestratorConfig holds orchestrator options.
type OrchestratorConfig struct {
	ArtifactDir     string
	UVB76Bin       string
	TovarischBin   string
	Timeout        time.Duration
	PhaseTimeout   time.Duration
	KeepNamespaces bool
	SkipCleanup    bool
	Verbose        bool
	// API credentials
	APIUser string
	APIPass string
}

// DefaultOrchestratorConfig returns the default configuration.
func DefaultOrchestratorConfig() OrchestratorConfig {
	return OrchestratorConfig{
		Timeout:      10 * time.Minute,
		PhaseTimeout: 30 * time.Second,
		APIUser:     "lab-admin",
		APIPass:     "testpass123",
	}
}

// NewOrchestrator creates a new lab orchestrator.
func NewOrchestrator(runner CommandRunner, config OrchestratorConfig) (*Orchestrator, error) {
	// Create artifact directory if needed
	artifactDir := config.ArtifactDir
	if artifactDir == "" {
		dir, err := CreateArtifactDir("kgb-uvb76-capture-netns-lab")
		if err != nil {
			return nil, fmt.Errorf("create artifact dir: %w", err)
		}
		artifactDir = dir
	}

	// Ensure artifact directory exists
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return nil, fmt.Errorf("create artifact root: %w", err)
	}

	netnsConfig := DefaultNetnsLabConfig()
	names := DefaultArtifactNames()
	artifacts := NewLabArtifacts(artifactDir, names)
	phases := DefaultPhaseConfigs()
	outcomes := DefaultPhaseOutcomes()

	// Create process runner first
	processRunner := NewRealProcessRunner()

	// Create orchestrator with CommandLog field so LoggingRunner can share it
	o := &Orchestrator{
		ProcessRunner: processRunner,
		Config:        netnsConfig,
		Options:       config,
		Artifacts:     artifacts,
		Names:         names,
		Cleanup:       NewCleanupStack(),
		Phases:        phases,
		Outcomes:      outcomes,
		Tracker:       NewPhaseTracker(),
		CommandLog:    []CommandLog{},
		labDir:        artifactDir,
		uvb76APIBase:  fmt.Sprintf("http://localhost:%d", netnsConfig.UVB76Port),
		tovarischURL:  fmt.Sprintf("http://%s:%d",
			netnsConfig.TovarischNS.IPCIDR[:len(netnsConfig.TovarischNS.IPCIDR)-3],
			netnsConfig.TovarischPort),
	}

	// Wire LoggingRunner to share o.CommandLog via pointer
	loggingRunner := NewLoggingRunner(runner, &o.CommandLog)
	o.Runner = loggingRunner
	o.Netns = NewNetnsHelper(loggingRunner, netnsConfig)

	return o, nil
}

// Run executes the complete lab lifecycle.
func (o *Orchestrator) Run(ctx context.Context) error {
	o.startedAt = time.Now()
	log.Printf("=== UVB-76 Capture Netns Lab ===")
	log.Printf("Artifact dir: %s", o.Artifacts.Root)

	// Ensure cleanup runs on both success and failure
	cleanupRan := false
	defer func() {
		if cleanupRan || o.Options.SkipCleanup {
			return
		}
		o.Cleanup.Run(ctx)
	}()

	// Register cleanup steps (LIFO order)
	o.Cleanup.Add("cleanup-log", func(ctx context.Context) error {
		return o.writeCleanupLog()
	})
	o.Cleanup.Add("stop-tovarisch", func(ctx context.Context) error {
		return o.stopTovarisch(ctx)
	})
	o.Cleanup.Add("stop-uvb76", func(ctx context.Context) error {
		return o.stopUVB76(ctx)
	})
	o.Cleanup.Add("clear-defect", func(ctx context.Context) error {
		return o.clearDefect(ctx)
	})
	o.Cleanup.Add("delete-namespaces", func(ctx context.Context) error {
		if o.Options.KeepNamespaces {
			log.Printf("Keeping namespaces (--keep-namespaces)")
			return nil
		}
		return o.deleteNamespaces(ctx)
	})

	// Run setup
	if err := o.setup(ctx); err != nil {
		cleanupRan = true
		o.Cleanup.Run(ctx)
		return fmt.Errorf("setup failed: %w", err)
	}

	// Run phases
	var phaseResults []PhaseResult
	var phasesFailed bool
	for _, phase := range o.Phases {
		result := o.runPhase(ctx, phase)
		phaseResults = append(phaseResults, result)
		if result.Err != "" {
			log.Printf("Phase %s failed: %s", result.Name, result.Err)
			phasesFailed = true
		}
	}

	// Run contract verifier
	verifierOK, verifierOutput := o.runContractVerifier(ctx)
	log.Printf("Contract verifier: ok=%v", verifierOK)

	// Write summary
	summary := o.buildSummary(phaseResults)
	if err := WriteSummary(o.Artifacts, summary); err != nil {
		log.Printf("Warning: failed to write summary: %v", err)
	}

	// Write result.json for CI compatibility
	if err := o.writeResult(phaseResults, verifierOK, verifierOutput); err != nil {
		log.Printf("Warning: failed to write result: %v", err)
	}

	// Make artifacts readable
	if err := os.Chmod(o.Artifacts.Root, 0755); err != nil {
		log.Printf("Warning: failed to chmod artifacts: %v", err)
	}

	// Return error if phases failed or verifier failed
	// Cleanup runs via defer (cleanupRan stays false)
	if phasesFailed || !verifierOK {
		return fmt.Errorf("lab failed: phases=%v verifier=%v", phasesFailed, !verifierOK)
	}

	// Cleanup runs via defer on success
	return nil
}

func (o *Orchestrator) setup(ctx context.Context) error {
	// Create namespaces
	log.Printf("Creating network namespaces...")
	if err := o.Netns.CreateNamespaces(ctx); err != nil {
		return fmt.Errorf("create namespaces: %w", err)
	}

	// Configure interfaces
	log.Printf("Configuring interfaces...")
	if err := o.Netns.ConfigureInterfaces(ctx); err != nil {
		return fmt.Errorf("configure interfaces: %w", err)
	}

	// Generate configs
	if err := o.generateConfigs(ctx); err != nil {
		return fmt.Errorf("generate configs: %w", err)
	}

	// Write topology artifact
	topology := fmt.Sprintf(`UVB-76 Capture Netns Lab Topology
================================

Namespace: %s
  Interface: uvb76-veth
  IP: %s

Namespace: %s
  Interface: tovarisch-veth
  IP: %s

veth pair connects the namespaces

Diagnostic peer URL: %s
`, o.Config.UVB76NS.Name, o.Config.UVB76NS.IPCIDR,
		o.Config.TovarischNS.Name, o.Config.TovarischNS.IPCIDR,
		o.tovarischURL)

	if err := os.WriteFile(o.Artifacts.TopologyPath, []byte(topology), 0644); err != nil {
		log.Printf("Warning: failed to write topology: %v", err)
	}

	// Start tovarisch
	log.Printf("Starting tovarisch...")
	if err := o.startTovarisch(ctx); err != nil {
		return fmt.Errorf("start tovarisch: %w", err)
	}

	// Wait for tovarisch HTTP endpoint
	log.Printf("Waiting for tovarisch HTTP endpoint...")
	if err := o.waitForTovarischHTTP(ctx); err != nil {
		return fmt.Errorf("wait for tovarisch HTTP: %w", err)
	}

	// Start uvb76
	log.Printf("Starting uvb76...")
	if err := o.startUVB76(ctx); err != nil {
		return fmt.Errorf("start uvb76: %w", err)
	}

	// Authenticate to UVB-76 API
	log.Printf("Authenticating to UVB-76 API...")
	if err := o.uvb76Authenticate(ctx); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	// Phase 0: Baseline probe readiness
	log.Printf("=== PHASE 0: Baseline Probe Readiness ===")
	if err := o.phase0Readiness(ctx); err != nil {
		log.Printf("Warning: Phase 0 readiness check failed: %v", err)
		o.probeReady = false
	} else {
		o.probeReady = true
	}

	return nil
}

func (o *Orchestrator) buildSummary(phaseResults []PhaseResult) *LabSummary {
	allOK := true
	for _, r := range phaseResults {
		if r.Err != "" {
			allOK = false
			break
		}
		outcome, ok := o.Outcomes[r.Name]
		if ok && r.CaptureStatus != outcome.ExpectedCaptureStatus {
			allOK = false
			break
		}
	}

	// Write command log
	commandLogPath := filepath.Join(o.labDir, "command-log.json")
	logData, _ := json.MarshalIndent(o.CommandLog, "", "  ")
	os.WriteFile(commandLogPath, logData, 0644)

	return &LabSummary{
		SchemaVersion: "1.0",
		StartedAt:     o.startedAt,
		EndedAt:       time.Now(),
		Phases:        phaseResults,
		Artifacts:     o.collectArtifactPaths(),
		Commands:      o.CommandLog,
		OK:            allOK,
	}
}

func (o *Orchestrator) collectArtifactPaths() []string {
	var paths []string
	filepath.Walk(o.Artifacts.Root, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			paths = append(paths, info.Name())
		}
		return nil
	})
	return paths
}

func (o *Orchestrator) runContractVerifier(ctx context.Context) (bool, string) {
	verifierPath := "../../../scripts/verify_uvb76_diag_packet_contract.sh"
	verifierOutputPath := filepath.Join(o.labDir, "contract-verifier-output.txt")

	// Run the contract verifier on the artifact directory
	cmd := exec.CommandContext(ctx, "bash", verifierPath, "--dir", o.labDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	output := stdout.String() + stderr.String()

	// Write verifier output
	os.WriteFile(verifierOutputPath, []byte(output), 0644)

	// Determine exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	// Log verifier command with actual exit code
	o.CommandLog = append(o.CommandLog, CommandLog{
		Command:  []string{"bash", verifierPath, "--dir", o.labDir},
		ExitCode: exitCode,
		Duration: duration.String(),
		Stdout:   truncate(stdout.String(), 4096),
		Stderr:   truncate(stderr.String(), 4096),
		Time:     startTime.Format(time.RFC3339Nano),
	})

	return err == nil, output
}

func (o *Orchestrator) writeResult(phaseResults []PhaseResult, verifierOK bool, verifierOutput string) error {
	result := make(map[string]interface{})
	result["ok"] = verifierOK
	result["artifact_dir"] = o.Artifacts.Root

	// Write phase results with derived booleans
	derived := make(map[string]interface{})
	for _, r := range phaseResults {
		switch r.Name {
		case PhaseBaseline:
			derived["phase1_capture_contract_ok"] = r.CaptureStatus == CaptureStatusCaptured
			derived["phase1_packet_contract_ok"] = r.CaptureStatus == CaptureStatusCaptured && len(r.ArtifactPaths) > 0
		case PhaseDefect:
			derived["phase2_cooldown_contract_ok"] = r.CaptureStatus == CaptureStatusSkippedCooldown
		case PhaseRecovery:
			derived["phase3_capture_contract_ok"] = r.CaptureStatus == CaptureStatusCaptured
			derived["phase3_packet_contract_ok"] = r.CaptureStatus == CaptureStatusCaptured && len(r.ArtifactPaths) > 0
		}
	}
	derived["probe_ready"] = o.probeReady
	result["derived"] = derived

	if verifierOutput != "" {
		result["contract_verifier_output"] = "contract-verifier-output.txt"
		result["contract_verifier_exit_code"] = 0
		if !verifierOK {
			result["contract_verifier_exit_code"] = 1
		}
	}

	return WriteJSON(o.Artifacts.Root, "result.json", result)
}

// CheckLinux verifies the environment is suitable for netns labs.
func CheckLinux() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("this lab requires Linux (network namespaces); current GOOS=%s", runtime.GOOS)
	}
	return nil
}

// CheckDependencies verifies required tools are available.
func CheckDependencies(ctx context.Context, runner CommandRunner) error {
	required := []string{"ip", "tc", "curl"}
	for _, cmd := range required {
		res := runner.Run(ctx, "which", cmd)
		if !res.OK() {
			return fmt.Errorf("required command not found: %s", cmd)
		}
	}
	return nil
}

// runCommand is a helper to run a command and log it.
func (o *Orchestrator) runCommand(ctx context.Context, name string, args ...string) CommandResult {
	res := o.Runner.Run(ctx, name, args...)
	o.CommandLog = append(o.CommandLog, CommandLog{
		Command:  append([]string{name}, args...),
		ExitCode: res.ExitCode,
		Duration: res.Duration().String(),
		Stdout:   truncate(res.Stdout, 1024),
		Stderr:   truncate(res.Stderr, 1024),
		Time:     res.Started.Format(time.RFC3339Nano),
	})
	return res
}
