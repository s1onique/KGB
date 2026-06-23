// uvb76_tls.go — UVB-76 TLS setup for memory labs
//
// Handles ephemeral TLS certificate generation and derived config creation
// for UVB-76 memory lab runs. Runtime files are written to a temp directory
// created via os.MkdirTemp, not to the artifact directory.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package main

import (
	"fmt"
	"path/filepath"
)

// prepareUVB76TLS generates ephemeral TLS certificates and creates a derived config
// for UVB-76 memory lab runs. This allows UVB-76 (which requires TLS in production)
// to run in the memory lab without checking in test certificates.
//
// Runtime files (TLS cert/key, derived config) are written to a temp directory created
// via os.MkdirTemp, not to the artifact directory. This keeps the evidence tree clean.
func (r *Runner) prepareUVB76TLS() error {
	if r.cfg.Service != "uvb76" {
		return nil // Only applies to uvb76
	}

	// Use a temp dir for ephemeral runtime material — keeps artifacts/memory-labs/ clean.
	rtDir, err := r.runtimeDir()
	if err != nil {
		return fmt.Errorf("create runtime dir: %w", err)
	}

	// Generate ephemeral cert/key pair
	tlsFiles, err := GenerateEphemeralCert(rtDir, "uvb76")
	if err != nil {
		return fmt.Errorf("generate ephemeral cert: %w", err)
	}
	r.cfg.TLS = tlsFiles

	// Determine source config path
	sourceConfig := r.cfg.ConfigPath
	if sourceConfig == "" {
		sourceConfig = filepath.Join(findRepoRootOrCWD(), "uvb76", "uvb76.memory-lab.json")
	}

	// Write derived config with TLS paths populated to runtime dir (not artifact dir).
	derivedPath, err := WriteDerivedConfig(rtDir, "uvb76", sourceConfig, tlsFiles)
	if err != nil {
		return fmt.Errorf("write derived config: %w", err)
	}
	r.cfg.DerivedConfigPath = derivedPath

	fmt.Printf("Runtime dir: %s\n", rtDir)
	fmt.Printf("Generated TLS cert: %s\n", tlsFiles.CertFile)
	fmt.Printf("Generated TLS key: %s\n", tlsFiles.KeyFile)
	fmt.Printf("Derived config: %s\n", derivedPath)

	return nil
}
