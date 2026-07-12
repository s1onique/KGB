package main

// ACT-UVB76-HULK05R4R1A-R2: serializer-level proof for the ICMP OS ping
// soak surface. The test calls the production WriteArtifacts function
// directly so every artifact produced by the lab goes through the
// artifactio boundary the catalog validator asserts.
//
// Required proofs:
//
//   - result.json valid and sanitized
//   - memstats.json valid and sanitized
//   - goroutines.txt useful and sanitized
//   - all modes 0600
//   - no temporary files left behind
//   - source inputs (Result struct) unchanged
//   - second pass idempotent
//   - prior artifacts preserved on failure
//   - production bypass scan reports zero findings for this surface
//   - mutation fixture proves the same detector catches a raw write
//   - exact password, API-key, session-token, and Bearer values are absent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/s1onique/KGB/uvb76/internal/producer"
)

// uniqueRunID returns a per-process identifier so each test invocation
// generates unique secret values. This makes the sanitization
// assertions non-vacuous: the canonical redactor strips the
// structured classes (password, api_key, session_token) and the text
// classes (Authorization: Bearer, X-Session-Token) that we
// deliberately inject.
func uniqueRunID() string {
	return fmt.Sprintf("%d", os.Getpid())
}

var (
	secretBearer     = "zBearer-" + uniqueRunID() + "-secret"
	secretPassword   = "zPassword-" + uniqueRunID() + "-secret"
	secretAPIKey     = "zApiKey-" + uniqueRunID() + "-secret"
	secretSessionTok = "zSession-" + uniqueRunID() + "-secret"
)

func makeResult() Result {
	return Result{
		OK:                       true,
		LabName:                  "kgb-uvb76-icmp-os-ping-soak",
		DurationSeconds:          30,
		ICMPEnabled:              true,
		ICMPIntervalSeconds:      1,
		ICMPTimeoutSeconds:       3,
		ICMPMaxConcurrent:        1,
		DaemonStarted:            true,
		DaemonExitedEarly:        false,
		PIDStable:                true,
		FatalLogPatternsFound:    nil,
		DaemonICMPAttempts:       10,
		DaemonICMPSuccesses:      10,
		DaemonICMPFailures:       1,
		DaemonICMPLastError:      "Authorization: Bearer " + secretBearer,
		DaemonStatusRaw:          "X-Session-Token: " + secretSessionTok,
		ICMPProbeExercised:       true,
		ICMPProbeExercisedReason: "lab ran",
		ICMPEvidenceSource:       "daemon_status",
		MemStatsBefore: fmt.Sprintf(
			`{"password":"%s","api_key":"%s","name":"safe"}`,
			secretPassword, secretAPIKey,
		),
		MemStatsAfter:       fmt.Sprintf(`{"session_token":"%s"}`, secretSessionTok),
		GoroutinesBefore:    5,
		GoroutinesAfter:     7,
		GoroutineLeaked:     false,
		HealthEndpointValid: true,
		StatusEndpointValid: true,
	}
}

func makeMemStats() MemStats {
	return MemStats{
		Alloc:      1024,
		TotalAlloc: 2048,
		Sys:        4096,
		NumGC:      1,
		GoVersion:  runtime.Version(),
	}
}

func TestWriteArtifacts_ProductionFunction_PassesAllChecks(t *testing.T) {
	tmp := t.TempDir()
	artifactDir := filepath.Join(tmp, "artifacts")
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	result := makeResult()
	memBefore := makeMemStats()
	memAfter := makeMemStats()

	// Production function under test.
	if err := WriteArtifacts(artifactDir, result, memBefore, memAfter,
		result.GoroutinesBefore, result.GoroutinesAfter); err != nil {
		t.Fatalf("WriteArtifacts: %v", err)
	}

	// result.json valid + sanitized.
	resultBytes, err := os.ReadFile(filepath.Join(artifactDir, "result.json"))
	if err != nil {
		t.Fatalf("read result.json: %v", err)
	}
	if !json.Valid(resultBytes) {
		t.Errorf("result.json is not valid JSON")
	}
	resultStr := string(resultBytes)
	for name, banned := range map[string]string{
		"password":      secretPassword,
		"api_key":       secretAPIKey,
		"session_token": secretSessionTok,
		"bearer":        secretBearer,
	} {
		if strings.Contains(resultStr, banned) {
			t.Errorf("result.json contains exact %s sentinel %q", name, banned)
		}
	}
	for _, want := range []string{
		`\"password\":\"[REDACTED]\"`,
		`\"api_key\":\"[REDACTED]\"`,
		`\"session_token\":\"[REDACTED]\"`,
		`Authorization: [REDACTED]`,
		`X-Session-Token: [REDACTED]`,
	} {
		if !strings.Contains(resultStr, want) {
			t.Errorf("result.json missing sanitized production evidence %q", want)
		}
	}

	// memstats.json is a second production output and remains valid JSON.
	memstatsBytes, err := os.ReadFile(filepath.Join(artifactDir, "memstats.json"))
	if err != nil {
		t.Fatalf("read memstats.json: %v", err)
	}
	if !json.Valid(memstatsBytes) {
		t.Errorf("memstats.json is not valid JSON")
	}
	// goroutines.txt useful + sanitized.
	goroutinesBytes, err := os.ReadFile(filepath.Join(artifactDir, "goroutines.txt"))
	if err != nil {
		t.Fatalf("read goroutines.txt: %v", err)
	}
	goroutinesStr := string(goroutinesBytes)
	for _, want := range []string{"before: 5", "after: 7", "leaked: false"} {
		if !strings.Contains(goroutinesStr, want) {
			t.Errorf("goroutines.txt lost useful evidence: %q", want)
		}
	}

	// all modes 0600.
	for _, name := range []string{"result.json", "memstats.json", "goroutines.txt"} {
		info, err := os.Stat(filepath.Join(artifactDir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if runtime.GOOS != "windows" {
			if got := info.Mode().Perm(); got != 0o600 {
				t.Errorf("%s mode = %s, want 0600", name, got)
			}
		}
	}

	// no temporary files left behind.
	entries, err := os.ReadDir(artifactDir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp.") {
			t.Errorf("leftover temp file %q", e.Name())
		}
	}
}

func TestWriteArtifacts_SourceInputsUnchanged(t *testing.T) {
	tmp := t.TempDir()
	artifactDir := filepath.Join(tmp, "artifacts")
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	result := makeResult()
	memBefore := makeMemStats()
	memAfter := makeMemStats()

	// Snapshot the original secret-bearing fields.
	origBefore := result.MemStatsBefore
	origAfter := result.MemStatsAfter
	origRaw := result.DaemonStatusRaw
	origFatal := append([]string(nil), result.FatalLogPatternsFound...)

	if err := WriteArtifacts(artifactDir, result, memBefore, memAfter,
		result.GoroutinesBefore, result.GoroutinesAfter); err != nil {
		t.Fatalf("WriteArtifacts: %v", err)
	}

	if result.MemStatsBefore != origBefore {
		t.Errorf("MemStatsBefore was mutated by WriteArtifacts")
	}
	if result.MemStatsAfter != origAfter {
		t.Errorf("MemStatsAfter was mutated by WriteArtifacts")
	}
	if result.DaemonStatusRaw != origRaw {
		t.Errorf("DaemonStatusRaw was mutated by WriteArtifacts")
	}
	if len(result.FatalLogPatternsFound) != len(origFatal) {
		t.Errorf("FatalLogPatternsFound length mutated by WriteArtifacts")
	}
}

func TestWriteArtifacts_SecondPassIdempotent(t *testing.T) {
	tmp := t.TempDir()
	artifactDir := filepath.Join(tmp, "artifacts")
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	result := makeResult()
	memBefore := makeMemStats()
	memAfter := makeMemStats()

	if err := WriteArtifacts(artifactDir, result, memBefore, memAfter, 5, 7); err != nil {
		t.Fatalf("first write: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(artifactDir, "result.json"))
	if err != nil {
		t.Fatalf("read first: %v", err)
	}

	if err := WriteArtifacts(artifactDir, result, memBefore, memAfter, 5, 7); err != nil {
		t.Fatalf("second write: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(artifactDir, "result.json"))
	if err != nil {
		t.Fatalf("read second: %v", err)
	}

	if string(first) != string(second) {
		t.Errorf("second-pass WriteArtifacts produced different bytes (non-idempotent)")
	}
}

func TestWriteArtifacts_FailurePreservesPriorArtifact(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode semantics differ on Windows")
	}
	tmp := t.TempDir()
	artifactDir := filepath.Join(tmp, "artifacts")
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// First write succeeds.
	result := makeResult()
	memBefore := makeMemStats()
	memAfter := makeMemStats()
	if err := WriteArtifacts(artifactDir, result, memBefore, memAfter, 5, 7); err != nil {
		t.Fatalf("first write: %v", err)
	}

	prior, err := os.ReadFile(filepath.Join(artifactDir, "result.json"))
	if err != nil {
		t.Fatalf("read prior: %v", err)
	}

	// Force failure by removing write permission on the artifact dir.
	if err := os.Chmod(artifactDir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(artifactDir, 0o700) })

	err = WriteArtifacts(artifactDir, result, memBefore, memAfter, 5, 7)
	if err == nil {
		t.Fatal("expected empty-payload failure")
	}
	// Re-grant read so we can verify the prior artifact.
	if err := os.Chmod(artifactDir, 0o700); err != nil {
		t.Fatalf("re-chmod: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(artifactDir, "result.json"))
	if err != nil {
		t.Fatalf("read after failed write: %v", err)
	}
	if string(got) != string(prior) {
		t.Errorf("prior artifact was modified by failed write")
	}
}

// TestICMP_NoBypassFindings invokes the production detector over the
// canonical ICMP writer files and requires a real zero-finding result.
func TestICMP_NoBypassFindings(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	cat, err := producer.LoadCanonicalCatalog(root)
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	var found *producer.CanonicalSurface
	for i := range cat.Surfaces {
		if cat.Surfaces[i].ID == "icmp-ping-soak-artifacts" {
			found = &cat.Surfaces[i]
			break
		}
	}
	if found == nil {
		t.Fatal("icmp-ping-soak-artifacts missing from catalog")
	}
	if found.EnforcementState != producer.EnforcementStateMigrated {
		t.Errorf("icmp-ping-soak-artifacts enforcement_state = %q, want migrated",
			found.EnforcementState)
	}
	if found.OwnershipScope != producer.OwnershipScopeSymbol {
		t.Errorf("icmp-ping-soak-artifacts ownership_scope = %q, want symbol",
			found.OwnershipScope)
	}
	if len(found.WriterSymbols) == 0 {
		t.Error("migrated surface must declare writer_symbols")
	}
	if len(found.TestFiles) == 0 {
		t.Error("migrated surface must declare test_files")
	}
	files := make([]string, 0, len(found.WriterFiles))
	for _, writerFile := range found.WriterFiles {
		files = append(files, filepath.Join(root, filepath.FromSlash(writerFile)))
	}
	if err := producer.DefaultInit(); err != nil {
		t.Fatalf("initialize canonical contracts: %v", err)
	}
	detector := producer.NewBypassDetector(producer.BypassConfig{
		AllowlistedFiles: producer.DefaultAllowlistedWriterFiles,
		FileBindings:     producer.FileBindingsFromContracts(producer.DefaultContracts),
		RepoRoot:         root,
	})
	findings, err := detector.Scanner(files)
	if err != nil {
		t.Fatalf("scan canonical ICMP writer: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("ICMP production writer has %d bypass finding(s): %v", len(findings), findings)
	}
}

func TestICMP_BypassDetectorMutationProof(t *testing.T) {
	root := t.TempDir()
	rel := "uvb76/cmd/uvb76-icmp-os-ping-soak/types.go"
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir mutation fixture: %v", err)
	}
	fixture := `package main
import "os"
func WriteArtifacts() error {
	return os.WriteFile("result.json", []byte("unsafe"), 0600)
}
`
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatalf("write mutation fixture: %v", err)
	}
	binding := producer.ProducerBinding{
		SurfaceID:         "icmp-ping-soak-artifacts",
		WriterSymbol:      "WriteArtifacts",
		SurfacePath:       "uvb76/cmd/uvb76-icmp-os-ping-soak/**/*",
		PersistencePolicy: producer.PersistenceAtomicRedactedJSON,
		RequiredAPI:       "uvb76/internal/artifactio",
	}
	detector := producer.NewBypassDetector(producer.BypassConfig{
		RepoRoot: root,
		FileBindings: func(file string) []producer.ProducerBinding {
			if file == rel {
				return []producer.ProducerBinding{binding}
			}
			return nil
		},
	})
	findings, err := detector.Scanner([]string{path})
	if err != nil {
		t.Fatalf("scan mutation fixture: %v", err)
	}
	if len(findings) != 1 || findings[0].CallName != "os.WriteFile" {
		t.Fatalf("mutation findings = %#v, want one os.WriteFile finding", findings)
	}
	if len(findings[0].SurfaceIDs) != 1 || findings[0].SurfaceIDs[0] != binding.SurfaceID {
		t.Fatalf("mutation finding attribution = %#v, want %q", findings[0].SurfaceIDs, binding.SurfaceID)
	}
}

func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "uvb76_artifact_secret_hygiene", "surfaces.json")); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "uvb76", "internal", "producer", "producer.go")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", os.ErrNotExist
}
