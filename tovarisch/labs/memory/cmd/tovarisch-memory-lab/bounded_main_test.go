// bounded_main_test.go — Fresh-verifier build for bounded ACT closure.
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01-CORRECTION01
// §5.5: "Fresh verifier per test execution. Do not reuse
// `.factory/bin/tovarisch-memory-lab`. Build once per test process
// into a temporary directory."
//
// TestMain builds the production controller binary (the same source
// the producer would use) into a per-process temp dir. The binary
// under test is therefore always derived from the same checkout that
// the test suite is running, never a stale `.factory` artefact.
//
// `TestMain` is the package-level entrypoint for `go test`. The
// per-test helpers in bounded_fixture_test.go read the cached
// `testVerifierPath` to invoke the verifier as a subprocess.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// testVerifierOnce ensures TestMain builds the verifier exactly once
// per test process even if the test binary is re-entered.
var (
	testVerifierOnce sync.Once
	testVerifierPath string
	testVerifierSHA  string
	testVerifierErr  error
)

// TestMain builds the production controller binary into a temp dir
// before any test runs, captures its path and SHA-256, and skips
// (with a clear error) if the build cannot proceed.
//
// All bounded negative tests depend on this. Missing the build is a
// hard failure: the ACT requires a verifier built from the same
// checkout, not a developer-machine binary.
func TestMain(m *testing.M) {
	buildOnce()
	if testVerifierErr != nil {
		fmt.Fprintf(os.Stderr, "FATAL: bounded ACT verifier build failed: %v\n", testVerifierErr)
		os.Exit(2)
	}
	os.Exit(m.Run())
}

// buildOnce compiles the production controller binary into a temp dir
// and hashes it. The temp dir is created in the OS temp dir and
// cleaned up implicitly by the OS on process exit.
func buildOnce() {
	testVerifierOnce.Do(func() {
		// Prefer explicit repo root from environment (closure mode).
		// Fall back to go list for development.
		var moduleDir string
		if repoRoot := os.Getenv("TOVARISCH_REPO_ROOT"); repoRoot != "" {
			moduleDir = repoRoot
		} else {
			moduleRoot, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}").Output()
			if err != nil {
				testVerifierErr = fmt.Errorf("go list module root: %w", err)
				return
			}
			moduleDir = strings.TrimSpace(string(moduleRoot))
		}

		// The verifier package is cmd/tovarisch-memory-lab relative to
		// the module root. Build from the package directory.
		srcDir := filepath.Join(moduleDir, "cmd", "tovarisch-memory-lab")

		binDir, err := os.MkdirTemp("", "bounded-verifier-*")
		if err != nil {
			testVerifierErr = fmt.Errorf("mktemp: %w", err)
			return
		}
		binPath := filepath.Join(binDir, "tovarisch-memory-lab")

		// Build with the canonical go-build invocation. The test
		// driver never relies on build tags, -trimpath, or any
		// embedded path semantics; the verifier produces a fresh
		// hash on every build, which the fixture rebinds to.
		cmd := exec.Command("go", "build", "-o", binPath, ".")
		cmd.Dir = srcDir
		cmd.Env = append(os.Environ(), "GOFLAGS=")
		if out, err := cmd.CombinedOutput(); err != nil {
			testVerifierErr = fmt.Errorf("go build: %w (%s)", err, out)
			return
		}
		data, err := os.ReadFile(binPath)
		if err != nil {
			testVerifierErr = fmt.Errorf("read built binary: %w", err)
			return
		}
		sum := sha256.Sum256(data)
		testVerifierPath = binPath
		testVerifierSHA = hex.EncodeToString(sum[:])
	})
}

// verifierPath returns the absolute path to the freshly built verifier.
// Tests use this to invoke the verifier as a subprocess.
func verifierPath() string {
	if testVerifierPath == "" {
		panic("verifier not built: TestMain did not run before test")
	}
	return testVerifierPath
}

// verifierSHA returns the SHA-256 of the freshly built verifier, in
// lowercase hex (64 chars). Tests use this to rebind the committed
// fixture's `controller_executable_sha256` so the live-inode binding
// verifies after each rebuild.
func verifierSHA() string {
	if testVerifierSHA == "" {
		panic("verifier hash not computed: TestMain did not run before test")
	}
	return testVerifierSHA
}
