// descriptor_correction04_helpers_test.go — Shared helpers
// for the CORRECTION04 mutation tests.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// manifestPathFor returns the manifest.json path inside the
// bound fixture directory.
func manifestPathFor(boundDir string) string {
	return filepath.Join(boundDir, "manifest.json")
}

// readFile reads a JSON file and returns it as a generic map.
func readFile(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return m
}

// writeJSON writes a JSON file with the given data.
func writeJSON(t *testing.T, path string, data interface{}) {
	t.Helper()
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// readFixtureManifest reads a fixture's manifest.json as a
// generic map.
func readFixtureManifest(t *testing.T, fixtureDir string) map[string]interface{} {
	t.Helper()
	manifestPath := filepath.Join(fixtureDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read %s: %v", manifestPath, err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse %s: %v", manifestPath, err)
	}
	return m
}

// ensureFmt is a small helper to keep imports referenced if
// other tests strip fmt.
var _ = fmt.Sprintf
