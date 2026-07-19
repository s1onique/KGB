// Command uvb76-artifact-writer-verify statically scans every active
// producer writer file and reports direct persistence calls that bypass
// the central artifactio boundary.
//
// ACT-UVB76-HULK05R4R1
//
// The CLI loads the canonical catalog from
// scripts/uvb76_artifact_secret_hygiene/surfaces.json, runs the production
// ValidateCatalog validator with DefaultValidationOptions, and prints the
// required canonical metrics. The self-test path runs the same validator;
// the bypass-scan path additionally reports per-file findings.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/s1onique/KGB/uvb76/internal/producer"
)

func main() {
	var (
		repoRoot = flag.String("repo", "", "repository root (default: auto-detect)")
		selfTest = flag.Bool("self-test", false, "run self-tests and exit")
		showHelp = flag.Bool("help", false, "show help")
		failFast = flag.Bool("fail-fast", false, "exit non-zero on first violation")
	)
	flag.Parse()
	if *showHelp {
		fmt.Fprintln(os.Stderr, "uvb76-artifact-writer-verify: scan writer files for bypassing direct persistence calls")
		flag.PrintDefaults()
		os.Exit(0)
	}

	root := *repoRoot
	if root == "" {
		root = autoDetectRepoRoot()
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "abs repo:", err)
		os.Exit(2)
	}

	// Always run the canonical ValidateCatalog validation against the real
	// catalog before any other operation. This is the fail-closed step that
	// proves the gate is exercising the canonical-source validator.
	metrics, err := runCanonicalValidation(abs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "canonical validation:", err)
		os.Exit(2)
	}
	printCanonicalMetrics(metrics)

	if *selfTest {
		runSelfTests(abs)
		os.Exit(0)
	}

	// Bypass-scan path: collect ACTIVE writer files and run the AST bypass
	// detector over them. Fail-closed if any ACTIVE writer file contains a
	// bypassing direct persistence call.
	files := collectActiveWriterFiles(abs, metrics.contracts)
	cfg := producer.BypassConfig{
		AllowlistedFiles: producer.DefaultAllowlistedWriterFiles,
		FileBindings:     producer.FileBindingsFromContracts(metrics.contracts),
		RepoRoot:         abs,
	}
	det := producer.NewBypassDetector(cfg)
	findings, err := det.Scanner(files)
	if err != nil {
		fmt.Fprintln(os.Stderr, "scan:", err)
		os.Exit(2)
	}

	if len(findings) == 0 {
		fmt.Println("PASS: no bypassing direct persistence calls detected")
		fmt.Printf("  scanned %d writer file(s)\n", len(files))
		os.Exit(0)
	}

	fmt.Printf("FAIL: %d bypassing direct persistence call(s) detected:\n", len(findings))
	for _, f := range findings {
		fmt.Printf("  surfaces=%v call=%s file=%s line=%d destination=%s reason=%s\n",
			f.SurfaceIDs, f.CallName, f.File, f.Line, f.Destination, f.DestinationReason)
		fmt.Printf("    %s\n", f.Suggestion)
		if *failFast {
			os.Exit(1)
		}
	}
	fmt.Println("validation summary:")
	fmt.Printf("  registry surfaces:  %d\n", len(metrics.surfaces))
	fmt.Printf("  registry contracts: %d\n", len(metrics.contracts))
	os.Exit(1)
}

// canonicalMetrics holds the data the CLI prints from the canonical catalog.
type canonicalMetrics struct {
	catalog                 *producer.CanonicalCatalog
	contracts               []*producer.ProducerContract
	surfaces                []producer.SurfaceRecord
	catalogSurfaces         int
	activeSurfaces          int
	verifiedWriterFiles     int
	verifiedWriterSymbols   int
	verifiedTestFiles       int
	catalogValidationErrors int
}

func runCanonicalValidation(repoRoot string) (canonicalMetrics, error) {
	var m canonicalMetrics

	cat, err := producer.LoadCanonicalCatalog(repoRoot)
	if err != nil {
		return m, err
	}
	m.catalog = cat

	contracts, surfaces := cat.ProjectContracts()
	m.contracts = contracts
	m.surfaces = surfaces

	options := producer.DefaultValidationOptions(repoRoot)
	issues := producer.ValidateCatalog(cat, options)

	m.catalogSurfaces = len(cat.Surfaces)
	m.catalogValidationErrors = countCatalogErrors(issues)

	// Compute per-surface verified counts by walking the catalog directly.
	for _, s := range cat.Surfaces {
		if s.Status != producer.StatusActive {
			continue
		}
		m.activeSurfaces++
		for _, wf := range s.WriterFiles {
			if producer.FileExists(repoRoot, wf) {
				m.verifiedWriterFiles++
			}
		}
		// Count verified writer symbols.
		for _, sym := range s.WriterSymbols {
			found := false
			for _, wf := range s.WriterFiles {
				syms, err := producer.DeclaredFunctionsInFile(repoRoot, wf)
				if err != nil {
					continue
				}
				if syms[sym] {
					found = true
					break
				}
				// Also accept Receiver.Method form (e.g. "Foo.Bar").
				for k := range syms {
					if k == sym {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if found {
				m.verifiedWriterSymbols++
			}
		}
		for _, tf := range s.TestFiles {
			if producer.FileExists(repoRoot, tf) {
				m.verifiedTestFiles++
			}
		}
	}

	return m, nil
}

func countCatalogErrors(issues []producer.CatalogValidationIssue) int {
	n := 0
	for _, i := range issues {
		if i.Severity == "error" {
			n++
		}
	}
	return n
}

func printCanonicalMetrics(m canonicalMetrics) {
	fmt.Println("=== Canonical catalog metrics ===")
	fmt.Printf("  catalog surfaces:          %d\n", m.catalogSurfaces)
	fmt.Printf("  active surfaces:           %d\n", m.activeSurfaces)
	fmt.Printf("  verified writer files:     %d\n", m.verifiedWriterFiles)
	fmt.Printf("  verified writer symbols:   %d\n", m.verifiedWriterSymbols)
	fmt.Printf("  verified test files:       %d\n", m.verifiedTestFiles)
	fmt.Printf("  catalog validation errors: %d\n", m.catalogValidationErrors)
	if m.catalogValidationErrors > 0 {
		// Fail closed: any severity=error fails the gate.
		os.Exit(2)
	}
}

// autoDetectRepoRoot walks up from the current working directory until it
// finds a known KGB anchor.
func autoDetectRepoRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	dir := cwd
	for i := 0; i < 6; i++ {
		if fileExists(filepath.Join(dir, "uvb76", "internal", "artifactio", "policy.go")) &&
			fileExists(filepath.Join(dir, "AGENTS.md")) {
			return dir
		}
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, "AGENTS.md")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return cwd
}

func fileExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}

func collectActiveWriterFiles(root string, contracts []*producer.ProducerContract) []string {
	var files []string
	for _, c := range contracts {
		if c.Status != producer.StatusActive {
			continue
		}
		for _, wf := range c.WriterFiles {
			if strings.HasPrefix(wf, "/") {
				continue
			}
			if strings.Contains(wf, "..") {
				continue
			}
			ext := filepath.Ext(wf)
			if ext != ".go" {
				continue
			}
			abs := filepath.Join(root, filepath.FromSlash(wf))
			if _, err := os.Stat(abs); err == nil {
				files = append(files, abs)
			}
		}
	}
	return files
}

func runSelfTests(root string) {
	// Self-test exercises the same production catalog validator used by
	// the gate, with mutation fixtures that fail closed.
	if err := producer.DefaultInit(); err != nil {
		fmt.Println("FAIL: default init:", err)
		os.Exit(1)
	}
	cat, err := producer.LoadCanonicalCatalog(root)
	if err != nil {
		fmt.Println("FAIL: load canonical:", err)
		os.Exit(1)
	}
	options := producer.DefaultValidationOptions(root)
	issues := producer.ValidateCatalog(cat, options)
	var errs []producer.CatalogValidationIssue
	for _, i := range issues {
		if i.Severity == "error" {
			errs = append(errs, i)
		}
	}
	if len(errs) != 0 {
		fmt.Println("FAIL: catalog validation produced errors")
		for _, i := range errs {
			fmt.Printf("  %s\n", i.String())
		}
		os.Exit(1)
	}
	// Self-test mutations prove the validator fails closed on malformed
	// catalogs.
	raw := []byte(`{
      "surfaces": [
        {
          "id": "x",
          "path": "",
          "producer": "",
          "committed_allowed": false,
          "sensitivity": "weird",
          "sanitizer": "none",
          "status": "weird",
          "persistence_policy": "weird",
          "binary_policy": "weird",
          "output_format": "weird",
          "owner": "",
          "justification": ""
        }
      ]
    }`)
	tmp, err := os.CreateTemp("", "self-test-*.json")
	if err != nil {
		fmt.Println("FAIL: temp:", err)
		os.Exit(1)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(raw); err != nil {
		fmt.Println("FAIL: write tmp:", err)
		os.Exit(1)
	}
	tmp.Close()
	mut, err := producer.LoadCanonicalCatalogFrom(tmpPath)
	if err != nil {
		fmt.Println("FAIL: load mutation:", err)
		os.Exit(1)
	}
	mutIssues := producer.ValidateCatalog(mut, options)
	var mutErrs int
	for _, i := range mutIssues {
		if i.Severity == "error" {
			mutErrs++
		}
	}
	if mutErrs == 0 {
		fmt.Println("FAIL: validator did not catch malformed mutation")
		os.Exit(1)
	}
	fmt.Println("self-test OK")
	_ = root
}
