// verify-script-doctrine checks repository scripts against the script doctrine.
//
// Inventory types and loading logic.
package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InventoryEntry represents a single script entry in the inventory CSV.
type InventoryEntry struct {
	Path       string
	Language   string
	LogicalLOC int
	Role       string
	Status     string
	Notes      string
}

// loadInventory loads the script inventory from a CSV file.
// It handles comment lines and variable field counts.
func loadInventory(path string) (map[string]*InventoryEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No inventory yet
		}
		return nil, err
	}
	defer f.Close()

	entries := make(map[string]*InventoryEntry)
	r := csv.NewReader(f)
	r.Comma = ','
	r.FieldsPerRecord = -1 // Allow variable field counts (for comment lines)

	// Read header
	header, err := r.Read()
	if err != nil {
		return entries, nil
	}

	// Find column indices
	pathIdx := findIndex(header, "path")
	langIdx := findIndex(header, "language")
	locIdx := findIndex(header, "logical_loc")
	roleIdx := findIndex(header, "role")
	statusIdx := findIndex(header, "status")
	notesIdx := findIndex(header, "notes")

	for {
		record, err := r.Read()
		if err != nil {
			break
		}

		// Skip comment lines
		if len(record) > 0 && strings.HasPrefix(record[0], "#") {
			continue
		}

		entry := &InventoryEntry{}
		if pathIdx >= 0 && pathIdx < len(record) {
			entry.Path = strings.TrimSpace(record[pathIdx])
		}
		if langIdx >= 0 && langIdx < len(record) {
			entry.Language = strings.TrimSpace(record[langIdx])
		}
		if locIdx >= 0 && locIdx < len(record) {
			fmt.Sscanf(strings.TrimSpace(record[locIdx]), "%d", &entry.LogicalLOC)
		}
		if roleIdx >= 0 && roleIdx < len(record) {
			entry.Role = strings.TrimSpace(record[roleIdx])
		}
		if statusIdx >= 0 && statusIdx < len(record) {
			entry.Status = strings.TrimSpace(record[statusIdx])
		}
		if notesIdx >= 0 && notesIdx < len(record) {
			entry.Notes = strings.TrimSpace(record[notesIdx])
		}

		if entry.Path != "" {
			entries[entry.Path] = entry
		}
	}

	return entries, nil
}

// findIndex finds the index of a column by name (case-insensitive).
func findIndex(header []string, name string) int {
	for i, h := range header {
		if strings.TrimSpace(strings.ToLower(h)) == name {
			return i
		}
	}
	return -1
}

// findRepoRoot walks up from cwd looking for .git directory.
func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(cwd, ".git")); err == nil {
			return cwd, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}

	return "", fmt.Errorf("repo root not found")
}
