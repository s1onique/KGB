package labconfig

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/s1onique/KGB/uvb76/config"
)

func TestGenerateRealMode(t *testing.T) {
	result, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	cfg := result.Config

	// Verify target ID is real-tovarisch
	if len(cfg.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(cfg.Targets))
	}
	if cfg.Targets[0].ID != "real-tovarisch" {
		t.Errorf("expected target ID 'real-tovarisch', got %q", cfg.Targets[0].ID)
	}
	if cfg.Targets[0].Name != "Real Tovarisch Status Endpoint" {
		t.Errorf("expected target name 'Real Tovarisch Status Endpoint', got %q", cfg.Targets[0].Name)
	}

	// Verify diagnostics peer uses real-tovarisch
	if len(cfg.Diagnostics.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(cfg.Diagnostics.Peers))
	}
	if cfg.Diagnostics.Peers[0].Name != "real-tovarisch-peer" {
		t.Errorf("expected peer name 'real-tovarisch-peer', got %q", cfg.Diagnostics.Peers[0].Name)
	}
	if len(cfg.Diagnostics.Peers[0].Targets) != 1 || cfg.Diagnostics.Peers[0].Targets[0] != "real-tovarisch" {
		t.Errorf("expected peer targets ['real-tovarisch'], got %v", cfg.Diagnostics.Peers[0].Targets)
	}

	// Verify scrape interval is short for smoke
	if cfg.Scrape.IntervalSeconds != 1 {
		t.Errorf("expected scrape interval 1, got %d", cfg.Scrape.IntervalSeconds)
	}

	// Verify ephemeral password is present
	if len(result.EphemeralPassword) == 0 {
		t.Error("expected ephemeral password to be set")
	}
}

func TestGenerateFakeMode(t *testing.T) {
	result, err := Generate("18444", "16060", "18317", true)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	cfg := result.Config

	// Verify target ID is fake-tovarisch
	if cfg.Targets[0].ID != "fake-tovarisch" {
		t.Errorf("expected target ID 'fake-tovarisch', got %q", cfg.Targets[0].ID)
	}
	if cfg.Targets[0].Name != "Fake Tovarisch Status Endpoint" {
		t.Errorf("expected target name 'Fake Tovarisch Status Endpoint', got %q", cfg.Targets[0].Name)
	}

	// Verify diagnostics peer uses fake-tovarisch
	if cfg.Diagnostics.Peers[0].Name != "fake-tovarisch-peer" {
		t.Errorf("expected peer name 'fake-tovarisch-peer', got %q", cfg.Diagnostics.Peers[0].Name)
	}
}

func TestGeneratedConfigValidates(t *testing.T) {
	result, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	cfg := result.Config

	// Convert to config.Config for validation
	prodCfg := &config.Config{
		Listen:      cfg.Listen,
		Auth:        cfg.Auth,
		Scrape:      cfg.Scrape,
		Latency:     cfg.Latency,
		Diagnostics: cfg.Diagnostics,
		Targets:     cfg.Targets,
	}

	// Validate with dev mode (allow missing TLS)
	err = prodCfg.Validate(config.ValidationOptions{AllowMissingTLS: true})
	if err != nil {
		t.Errorf("generated config should be valid: %v", err)
	}
}

func TestPProfEnabled(t *testing.T) {
	result, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	cfg := result.Config

	if !cfg.Diagnostics.Enabled {
		t.Error("diagnostics should be enabled")
	}
	if !cfg.Diagnostics.PProf.Enabled {
		t.Error("pprof should be enabled")
	}
	if cfg.Diagnostics.PProf.Listen != "localhost:16060" {
		t.Errorf("expected pprof listen 'localhost:16060', got %q", cfg.Diagnostics.PProf.Listen)
	}
}

func TestDiagPeerHasValidBaseURL(t *testing.T) {
	result, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	cfg := result.Config

	peer := cfg.Diagnostics.Peers[0]
	if peer.BaseURL != "http://localhost:18317" {
		t.Errorf("expected peer base_url 'http://localhost:18317', got %q", peer.BaseURL)
	}
}

func TestTargetURLIsRealTovarisch(t *testing.T) {
	result, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	cfg := result.Config

	target := cfg.Targets[0]
	if target.BaseURL != "http://localhost:18317" {
		t.Errorf("expected target base_url 'http://localhost:18317', got %q", target.BaseURL)
	}
}

func TestScrapeIntervalForSmoke(t *testing.T) {
	result, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	cfg := result.Config

	// 1 second interval for active smoke test
	if cfg.Scrape.IntervalSeconds != 1 {
		t.Errorf("expected scrape interval 1 for smoke, got %d", cfg.Scrape.IntervalSeconds)
	}
}

func TestLatencyDisabled(t *testing.T) {
	result, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	cfg := result.Config

	// HTTP latency should be disabled
	if cfg.Latency.HTTP.Enabled != nil && *cfg.Latency.HTTP.Enabled != false {
		t.Error("HTTP latency should be disabled in lab")
	}

	// ICMP latency should be disabled
	if cfg.Latency.ICMP.Enabled != nil && *cfg.Latency.ICMP.Enabled != false {
		t.Error("ICMP latency should be disabled in lab")
	}
}

func TestFakeToarischAbsentInRealMode(t *testing.T) {
	result, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	cfg := result.Config

	// Verify no fake target ID exists
	for _, target := range cfg.Targets {
		if target.ID == "fake-tovarisch" {
			t.Error("fake-tovarisch should not exist in real mode")
		}
	}

	// Verify no fake peer name exists
	for _, peer := range cfg.Diagnostics.Peers {
		if peer.Name == "fake-tovarisch-peer" {
			t.Error("fake-tovarisch-peer should not exist in real mode")
		}
	}
}

func TestConfigJSONSerialize(t *testing.T) {
	result, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	cfg := result.Config

	// Marshal to JSON and back
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("should serialize to JSON: %v", err)
	}

	var cfg2 Config
	if err := json.Unmarshal(data, &cfg2); err != nil {
		t.Fatalf("should deserialize from JSON: %v", err)
	}

	// Verify key fields preserved
	if cfg2.Targets[0].ID != cfg.Targets[0].ID {
		t.Error("target ID should be preserved")
	}
	if cfg2.Diagnostics.PProf.Listen != cfg.Diagnostics.PProf.Listen {
		t.Error("pprof listen should be preserved")
	}
}

func TestRealConfigFileForUVB76(t *testing.T) {
	// Create a temp file to write the config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.json")

	result, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	cfg := result.Config

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("should marshal: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("should write file: %v", err)
	}

	// Load using production config loader
	prodCfg, err := config.LoadWithOptions(configPath, config.ValidationOptions{AllowMissingTLS: true})
	if err != nil {
		t.Fatalf("production config should load generated config: %v", err)
	}

	// Verify loaded config has correct values
	if prodCfg.Targets[0].ID != "real-tovarisch" {
		t.Errorf("loaded config should have real-tovarisch target, got %q", prodCfg.Targets[0].ID)
	}
}

// TestEphemeralPasswordNotInSerializedJSON proves the ephemeral password is never serialized.
// P0-7-fix: The password must not appear in the serialized config file.
func TestEphemeralPasswordNotInSerializedJSON(t *testing.T) {
	result, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	cfg := result.Config

	// Verify ephemeral password is set
	if len(result.EphemeralPassword) == 0 {
		t.Fatal("ephemeral password should be set")
	}

	// Marshal to JSON
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("should marshal: %v", err)
	}

	jsonStr := string(data)

	// Password should NOT appear in serialized JSON
	if strings.Contains(jsonStr, "lab-password") {
		t.Error("ephemeral password should not appear in serialized JSON")
	}

	// EphemeralPasswordS should NOT appear (it's been removed)
	if strings.Contains(jsonStr, "EphemeralPassword") {
		t.Error("EphemeralPassword field should not exist in Config struct")
	}

	// Verify password hash is present and username is present
	// The actual JSON field names come from config.AuthConfig
	if !strings.Contains(jsonStr, "lab-user") {
		t.Error("username should be present in serialized JSON")
	}

	// Password hash should be present (sha256:...)
	if !strings.Contains(jsonStr, "sha256:") {
		t.Error("password hash should be present in serialized JSON")
	}
}

// TestGeneratedConfigResultHasEphemeralPassword proves the result structure has ephemeral secret.
func TestGeneratedConfigResultHasEphemeralPassword(t *testing.T) {
	result, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Result should have Config
	if result.Config == nil {
		t.Error("Config should not be nil")
	}

	// Result should have EphemeralPassword
	if result.EphemeralPassword == nil {
		t.Error("EphemeralPassword should not be nil")
	}

	// P0-1: EphemeralPassword should be non-empty (randomly generated)
	if len(result.EphemeralPassword) == 0 {
		t.Error("EphemeralPassword should not be empty")
	}

	// P0-1: EphemeralPassword should have minimum entropy
	if len(result.EphemeralPassword) < 16 {
		t.Errorf("EphemeralPassword too short: got %d bytes, want >= 16", len(result.EphemeralPassword))
	}
}

// TestGeneratedConfigResultWrapperSerialization proves the full wrapper cannot serialize the password.
// P0-3: Even if someone marshals the wrapper, EphemeralPassword is excluded.
func TestGeneratedConfigResultWrapperSerialization(t *testing.T) {
	result, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Verify ephemeral password is set
	if len(result.EphemeralPassword) == 0 {
		t.Fatal("ephemeral password should be set")
	}

	// Marshal the full wrapper (not just Config)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("should marshal wrapper: %v", err)
	}

	jsonStr := string(data)

	// EphemeralPassword field should NOT appear in JSON (has json:"-" tag)
	if strings.Contains(jsonStr, "EphemeralPassword") {
		t.Error("EphemeralPassword field should not appear in serialized wrapper")
	}

	// Password literal should NOT appear
	if strings.Contains(jsonStr, "lab-password") {
		t.Error("password should not appear in serialized wrapper")
	}

	// The base64-encoded password should NOT appear
	// (would happen if json:"-" were missing and []byte were encoded)
	encoded := base64.StdEncoding.EncodeToString(result.EphemeralPassword)
	if strings.Contains(jsonStr, encoded) {
		t.Error("base64-encoded password should not appear in serialized wrapper")
	}
}

// TestCredentialGenerationFailure tests that credential generation failure is propagated.
// P0-1: Must return error when credential generation fails, not fall back to fixed password.
func TestCredentialGenerationFailure(t *testing.T) {
	// Create a failing generator
	failingGen := &failingCredentialGenerator{}

	_, err := GenerateWithGenerator("18444", "16060", "18317", false, failingGen)

	// Should return an error wrapping ErrCredentialGeneration
	if err == nil {
		t.Fatal("expected error when credential generation fails")
	}

	if !errors.Is(err, ErrCredentialGeneration) {
		t.Errorf("expected error to wrap ErrCredentialGeneration, got: %v", err)
	}
}

// failingCredentialGenerator always returns an error.
type failingCredentialGenerator struct{}

func (g *failingCredentialGenerator) Generate() ([]byte, error) {
	return nil, errors.New("simulated random source failure")
}

// TestCredentialGenerationEmptySlice tests that an empty credential slice is rejected.
func TestCredentialGenerationEmptySlice(t *testing.T) {
	emptyGen := &emptyCredentialGenerator{}

	_, err := GenerateWithGenerator("18444", "16060", "18317", false, emptyGen)

	if err == nil {
		t.Fatal("expected error when credential is empty")
	}

	if !errors.Is(err, ErrCredentialEmpty) {
		t.Errorf("expected error to wrap ErrCredentialEmpty, got: %v", err)
	}
}

// emptyCredentialGenerator returns an empty slice with nil error.
type emptyCredentialGenerator struct{}

func (g *emptyCredentialGenerator) Generate() ([]byte, error) {
	return []byte{}, nil
}

// errInjectedReader is used to test reader errors.
var errInjectedReader = errors.New("injected reader error")

// TestCredentialGenerationReaderErrorClearsRawBuffer tests that reader errors clear the raw buffer.
// P0-1C: Raw buffer must be cleared on reader error.
func TestCredentialGenerationReaderErrorClearsRawBuffer(t *testing.T) {
	var observed []byte

	gen := &CryptoCredentialGenerator{
		Read: func(b []byte) (int, error) {
			observed = b // Alias to the same backing array
			for i := range b {
				b[i] = 0xA5 // Fill with pattern
			}
			return len(b), errInjectedReader
		},
	}

	_, err := gen.Generate()
	if !errors.Is(err, errInjectedReader) {
		t.Fatalf("expected reader error, got %v", err)
	}

	// P0-1C: Verify the SAME backing array was cleared
	assertAllZero(t, observed)
}

// assertAllZero asserts that all bytes in the slice are zero.
func assertAllZero(t *testing.T, b []byte) {
	for i, v := range b {
		if v != 0 {
			t.Errorf("byte %d not cleared: got 0x%02x, want 0", i, v)
		}
	}
}

// TestCredentialGenerationShortReadClearsRawBuffer tests that a short read clears the raw buffer.
// P0-1C: Raw buffer must be cleared on short read.
func TestCredentialGenerationShortReadClearsRawBuffer(t *testing.T) {
	var observed []byte

	gen := &CryptoCredentialGenerator{
		Read: func(b []byte) (int, error) {
			observed = b // Alias to the same backing array
			for i := 0; i < 16; i++ {
				b[i] = byte(i + 1) // Fill with non-zero pattern
			}
			return 16, nil // Short read - only 16 bytes filled in 32-byte buffer
		},
	}

	_, err := GenerateWithGenerator("18444", "16060", "18317", false, gen)
	if !errors.Is(err, ErrCredentialGeneration) {
		t.Fatalf("expected ErrCredentialGeneration, got %v", err)
	}

	// P0-1C: Verify the SAME backing array was cleared
	assertAllZero(t, observed)
}

// TestGenerateWithNilGenerator tests that a nil generator returns an error without panicking.
func TestGenerateWithNilGenerator(t *testing.T) {
	_, err := GenerateWithGenerator("18444", "16060", "18317", false, nil)

	if err == nil {
		t.Fatal("expected error when generator is nil")
	}

	if !errors.Is(err, ErrCredentialGeneratorNil) {
		t.Errorf("expected error to wrap ErrCredentialGeneratorNil, got: %v", err)
	}
}

// TestGenerateWithGeneratorErrorPlusDataClearsData tests that error+data is handled and data is cleared.
// P0-1D: Partial output must be cleared when generator returns both data and error.
func TestGenerateWithGeneratorErrorPlusDataClearsData(t *testing.T) {
	// Generator that returns data AND error, with retained alias
	dataPlusErrorGen := &dataPlusErrorGenerator{}

	// This should NOT panic - error+data must be handled gracefully
	_, err := GenerateWithGenerator("18444", "16060", "18317", false, dataPlusErrorGen)

	if err == nil {
		t.Fatal("expected error when generator returns data+error")
	}

	if !errors.Is(err, ErrCredentialGeneration) {
		t.Fatalf("expected ErrCredentialGeneration, got %v", err)
	}

	// P0-1D: Verify the returned slice was cleared
	assertAllZero(t, dataPlusErrorGen.returned)
}

// dataPlusErrorGenerator returns data AND an error, retaining the slice.
type dataPlusErrorGenerator struct {
	returned []byte
}

func (g *dataPlusErrorGenerator) Generate() ([]byte, error) {
	g.returned = []byte("sensitive-data")
	return g.returned, errors.New("generator error")
}

// TestTwoProductionCredentialsDiffer proves two runs produce different credentials.
func TestTwoProductionCredentialsDiffer(t *testing.T) {
	result1, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("first Generate failed: %v", err)
	}

	result2, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("second Generate failed: %v", err)
	}

	if string(result1.EphemeralPassword) == string(result2.EphemeralPassword) {
		t.Error("two production runs should produce different credentials")
	}
}

// TestCredentialLengthAndEncoding verifies the credential format.
func TestCredentialLengthAndEncoding(t *testing.T) {
	result, err := Generate("18444", "16060", "18317", false)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// base64.RawURLEncoding of 32 bytes = 43 characters
	expectedLen := 43
	if len(result.EphemeralPassword) != expectedLen {
		t.Errorf("credential length wrong: got %d, want %d", len(result.EphemeralPassword), expectedLen)
	}

	// Should be valid base64 characters only
	for _, c := range string(result.EphemeralPassword) {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			t.Errorf("invalid base64 character in credential: %c", c)
		}
	}
}

// TestConfigHashMatchesReturnedPlaintext verifies the hash uses the returned plaintext.
func TestConfigHashMatchesReturnedPlaintext(t *testing.T) {
	// Use deterministic generator to predict the credential
	deterministicGen := &CryptoCredentialGenerator{
		Read: func(b []byte) (int, error) {
			// Fill with predictable bytes
			for i := range b {
				b[i] = byte(i + 1)
			}
			return len(b), nil
		},
	}

	result, err := GenerateWithGenerator("18444", "16060", "18317", false, deterministicGen)
	if err != nil {
		t.Fatalf("GenerateWithGenerator failed: %v", err)
	}

	// Verify the hash in the config matches what hashPassword produces
	expectedHash := hashPassword(result.EphemeralPassword)
	if result.Config.Auth.PasswordSHA256 != expectedHash {
		t.Errorf("config hash doesn't match returned plaintext: got %s, want %s",
			result.Config.Auth.PasswordSHA256, expectedHash)
	}
}

// TestFixedPasswordLiteralAbsent verifies the hardcoded fallback is gone across entire production surface.
// P0-1: Scans all non-test Go files under the memory-lab package.
func TestFixedPasswordLiteralAbsent(t *testing.T) {
	// Forbid these literals across all production files
	forbidden := []string{
		`"lab-password"`,
		`"lab-password-fallback"`,
	}

	// Scan entire production surface (non-test files only)
	root := filepath.Join("..", "..") // uvb76/cmd/uvb76-memleak-pprof-lab
	if err := scanForForbiddenLiterals(t, root, forbidden); err != nil {
		t.Fatal(err)
	}
}

// scanForForbiddenLiterals recursively scans all non-test .go files for forbidden literals.
func scanForForbiddenLiterals(t *testing.T, root string, forbidden []string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip test files and non-Go files
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		content := string(data)
		for _, lit := range forbidden {
			if strings.Contains(content, lit) {
				relPath, _ := filepath.Rel(root, path)
				t.Errorf("forbidden literal %s found in %s", lit, relPath)
			}
		}

		return nil
	})
}

// TestGenerateWithTypedNilGenerator tests that a typed-nil generator returns an error.
// P0-1E: Typed-nil interfaces must not panic.
func TestGenerateWithTypedNilGenerator(t *testing.T) {
	// Create a typed-nil generator
	var typedNilGen *CryptoCredentialGenerator
	// This interface is non-nil (has type info) but underlying pointer is nil
	var contract CredentialGenerator = typedNilGen

	_, err := GenerateWithGenerator("18444", "16060", "18317", false, contract)

	if err == nil {
		t.Fatal("expected error when generator is typed-nil")
	}

	// P0-1E: Should get both credential error AND nil-generator sentinel
	if !errors.Is(err, ErrCredentialGeneration) {
		t.Fatalf("expected ErrCredentialGeneration, got: %v", err)
	}
	if !errors.Is(err, ErrCredentialGeneratorNil) {
		t.Fatalf("expected ErrCredentialGeneratorNil, got: %v", err)
	}
}

// TestCryptoGeneratorNilReader tests that a nil Read function returns an error.
// P0-1E: Nil function fields must not cause panics.
func TestCryptoGeneratorNilReader(t *testing.T) {
	gen := &CryptoCredentialGenerator{
		Read: nil, // Nil function
	}

	_, err := gen.Generate()

	if err == nil {
		t.Fatal("expected error when Read function is nil")
	}

	// P0-1E: Error should expose the nil reader sentinel
	if !errors.Is(err, ErrCredentialGeneratorNil) {
		t.Errorf("expected ErrCredentialGeneratorNil, got: %v", err)
	}
}
