package scriptdoctrine

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LoadInventory loads and validates the script inventory from a CSV file.
// Returns strict errors for malformed data.
// Returns an error if the inventory file does not exist (fail-closed).
func LoadInventory(path string) (map[string]*InventoryEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		// Fail-closed: missing inventory is an error
		return nil, fmt.Errorf("opening inventory %q: %w", path, err)
	}
	defer f.Close()

	entries := make(map[string]*InventoryEntry)
	seenIDs := make(map[string]int) // Track line numbers for duplicate ID detection
	r := csv.NewReader(f)
	r.Comma = ','
	r.FieldsPerRecord = -1 // Allow variable field counts (comments have fewer fields)

	lineNum := 0
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading inventory at line %d: %w", lineNum+1, err)
		}

		lineNum++

		// Skip empty or comment lines before validation
		if len(record) == 0 || (len(record) == 1 && strings.TrimSpace(record[0]) == "") {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(record[0]), "#") {
			continue
		}

		// Validate header
		if lineNum == 1 {
			if err := validateHeader(record); err != nil {
				return nil, &InventoryLoadError{Line: 1, Message: err.Error()}
			}
			continue
		}

		entry, err := parseEntry(record, lineNum)
		if err != nil {
			return nil, err
		}

		if entry.Path == "" {
			continue
		}

		// Validate ID is not empty
		if entry.ID == "" {
			return nil, &InventoryLoadError{Line: lineNum, Message: "empty ID field"}
		}

		// Validate language is not empty
		if entry.Language == "" {
			return nil, &InventoryLoadError{Line: lineNum, Message: "empty language field"}
		}

		// Validate role is not empty
		if entry.Role == "" {
			return nil, &InventoryLoadError{Line: lineNum, Message: "empty role field"}
		}

		// Validate status is not empty
		if entry.Status == "" {
			return nil, &InventoryLoadError{Line: lineNum, Message: "empty status field"}
		}

		// Check for duplicate IDs
		if prevLine, exists := seenIDs[entry.ID]; exists {
			return nil, &InventoryLoadError{Line: lineNum, Message: fmt.Sprintf("duplicate ID %q (first seen on line %d)", entry.ID, prevLine)}
		}
		seenIDs[entry.ID] = lineNum

		// Validate path is clean relative
		if err := validatePath(entry.Path); err != nil {
			return nil, &InventoryLoadError{Line: lineNum, Message: err.Error()}
		}

		// Check for duplicate paths
		if _, exists := entries[entry.Path]; exists {
			return nil, &InventoryLoadError{Line: lineNum, Message: fmt.Sprintf("duplicate path: %s", entry.Path)}
		}

		entries[entry.Path] = entry
	}

	return entries, nil
}

func validateHeader(record []string) error {
	// Header must have exactly 9 columns
	if len(record) != len(requiredColumns) {
		return fmt.Errorf("header must have exactly %d columns, got %d", len(requiredColumns), len(record))
	}
	for i, col := range requiredColumns {
		if strings.TrimSpace(strings.ToLower(record[i])) != col {
			return fmt.Errorf("column %d must be '%s', got '%s'", i, col, record[i])
		}
	}
	return nil
}

// parseEntry parses a CSV record into an InventoryEntry.
// CSV columns: id,path,language,logical_loc,role,public_interface,target_go_command,status,notes
func parseEntry(record []string, lineNum int) (*InventoryEntry, error) {
	// Require exactly 9 columns
	if len(record) != len(requiredColumns) {
		return nil, &InventoryLoadError{Line: lineNum, Message: fmt.Sprintf("expected %d columns, got %d", len(requiredColumns), len(record))}
	}

	entry := &InventoryEntry{
		ID:       strings.TrimSpace(record[0]),
		Path:     strings.TrimSpace(record[1]),
		Language: strings.TrimSpace(strings.ToLower(record[2])),
		Role:     strings.TrimSpace(strings.ToLower(record[4])),
		Status:   strings.TrimSpace(strings.ToLower(record[7])),
		Notes:    "",
	}

	// Parse logical_loc using strconv.Atoi (strict integer parsing)
	if strings.TrimSpace(record[3]) != "" {
		loc, err := strconv.Atoi(strings.TrimSpace(record[3]))
		if err != nil {
			return nil, &InventoryLoadError{Line: lineNum, Message: fmt.Sprintf("invalid logical_loc: %s", record[3])}
		}
		if loc < 0 {
			return nil, &InventoryLoadError{Line: lineNum, Message: fmt.Sprintf("logical_loc must be non-negative, got %d", loc)}
		}
		entry.LogicalLOC = loc
	}

	// Validate language
	if entry.Language != "" && !validLanguages[entry.Language] {
		return nil, &InventoryLoadError{Line: lineNum, Message: fmt.Sprintf("invalid language: %s", entry.Language)}
	}

	// Validate status
	if entry.Status != "" && !validStatuses[entry.Status] {
		return nil, &InventoryLoadError{Line: lineNum, Message: fmt.Sprintf("invalid status: %s", entry.Status)}
	}

	// Validate role
	if entry.Role != "" && !validRoles[entry.Role] {
		return nil, &InventoryLoadError{Line: lineNum, Message: fmt.Sprintf("invalid role: %s", entry.Role)}
	}

	// Parse notes if present
	if len(record) > 8 {
		entry.Notes = strings.TrimSpace(record[8])
	}

	return entry, nil
}

func validatePath(path string) error {
	if path == "" {
		return errors.New("empty path")
	}
	// Must be relative
	if filepath.IsAbs(path) {
		return errors.New("path must be relative")
	}
	// Must not escape repository
	if strings.HasPrefix(path, "..") {
		return errors.New("path must not escape repository")
	}
	// Must be clean
	clean := filepath.Clean(path)
	if clean != path {
		return fmt.Errorf("path must be clean, got: %s", path)
	}
	return nil
}
