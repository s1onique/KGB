// output.go — Output formatting for artifact-writer-scanner.
package main

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/s1onique/KGB/internal/artifactwriterbaseline"
)

// outputSharded writes findings as sharded JSONL files.
func outputSharded(findings []Finding, outputDir, commitHash string) error {
	// Convert findings to internal package format
	var internalFindings []artifactwriterbaseline.Finding
	for _, f := range findings {
		internalFindings = append(internalFindings, artifactwriterbaseline.Finding{
			FindingID:              f.FindingID,
			SurfaceID:             f.SurfaceID,
			File:                  f.File,
			Line:                  f.Line,
			Operation:             f.Operation,
			DestinationExpression:  f.DestinationExpression,
			EnclosingSymbol:       f.EnclosingSymbol,
			ASTFingerprint:        f.ASTFingerprint,
			Justification:         f.Justification,
			Owner:                f.Owner,
			SuccessorACT:         f.SuccessorACT,
		})
	}

	return artifactwriterbaseline.Write(outputDir, internalFindings, artifactwriterbaseline.WriteConfig{
		BaselineCommit: commitHash,
		Generator:      "artifact-writer-scanner",
	})
}

// outputLegacy writes findings as legacy monolithic JSON.
func outputLegacy(findings []Finding, outputPath, commitHash string) ([]byte, error) {
	baseline := Baseline{
		SchemaVersion:   "ratchet-v1",
		BaselineCommit: commitHash,
		Generator:      "artifact-writer-scanner",
		GeneratedAt:    "",
		Findings:       findings,
	}

	return json.MarshalIndent(baseline, "", "  ")
}

// getCommitHash returns the current commit hash or "unknown".
func getCommitHash() string {
	if data, err := os.ReadFile(".git/refs/heads/main"); err == nil {
		return strings.TrimSpace(string(data))
	}
	return "unknown"
}

// getCommit returns the provided commit or auto-detects from git.
func getCommit(commitFlag string) string {
	if commitFlag != "" {
		return commitFlag
	}
	return getCommitHash()
}
