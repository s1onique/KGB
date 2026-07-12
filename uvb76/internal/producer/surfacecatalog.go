package producer

// ACT-UVB76-HULK05R4R1: canonical machine-readable surface catalog loader.
//
// The previous contractcatalog.go maintained a handwritten Go mirror of the
// Python inventory. That mirror is replaced by this loader: it reads the same
// JSON file consumed by the Python toolchain, validates the closed shapes,
// and projects surfaces + contracts into the producer registry.
//
// There is now exactly one editable surface catalog
// (scripts/uvb76_artifact_secret_hygiene/surfaces.json). The Go projection
// is consumed exclusively by the validation/registry/test path.

// CanonicalCatalogPath is the repository-relative location of the canonical
// surface catalog used by both Python and Go.
const CanonicalCatalogPath = "scripts/uvb76_artifact_secret_hygiene/surfaces.json"

// LoadCanonicalCatalogBytes returns the canonical catalog JSON bytes by reading
// the file at CanonicalCatalogPath relative to repoRoot. There is no embedded
// asset because the catalog is project data, not code.
func LoadCanonicalCatalogBytes(repoRoot string) ([]byte, error) {
	if repoRoot == "" {
		return nil, &CatalogError{Reason: "no repoRoot provided"}
	}
	abs := joinRepoPath(repoRoot, CanonicalCatalogPath)
	return readFileBytes(abs)
}

// CatalogError is a loader error.
type CatalogError struct {
	Reason string
}

func (e *CatalogError) Error() string { return "canonical catalog: " + e.Reason }

// OwnershipScope declares the AST-walk scope the detector uses to
// attribute a finding to a writer symbol.
const (
	// OwnershipScopeSymbol: the detector matches a finding to a
	// binding whose WriterSymbol equals the enclosing *ast.FuncDecl's
	// name (or its receiver-qualified form).
	OwnershipScopeSymbol = "symbol"
	// OwnershipScopeDedicatedFile: the entire file is dedicated to the
	// surface; the detector emits a finding for any os.* write inside
	// the file regardless of the enclosing function.
	OwnershipScopeDedicatedFile = "dedicated_file"
)

// CanonicalSurface is the in-memory representation of one entry in surfaces.json.
//
// EnforcementState is one of:
//
//   - migrated: production serializer is wired through artifactio; the
//     gate must produce zero bypass findings.
//   - legacy_bypass: file-level raw os.* writes still exist and bypass the
//     artifactio boundary; the gate produces findings until the migration
//     ACT rewrites the writer.
//   - not_applicable: static / prospective / detection_only surface; the
//     bypass detector is not invoked.
type CanonicalSurface struct {
	ID                string
	Path              string
	Producer          string
	CommittedAllowed  bool
	Sensitivity       string
	Sanitizer         string
	Status            ProducerStatus
	PersistencePolicy PersistencePolicy
	BinaryPolicy      BinaryPolicy
	OutputFormat      string
	Owner             string
	Justification     string
	EnforcementState  string
	OwnershipScope    string
	WriterFiles       []string
	WriterSymbols     []string
	TestFiles         []string
}

// EnforcementState values. The gate uses these to split catalog validity
// from migration state.
const (
	EnforcementStateMigrated      = "migrated"
	EnforcementStateLegacyBypass  = "legacy_bypass"
	EnforcementStateNotApplicable = "not_applicable"
)

// CanonicalCatalog is the parsed canonical surface catalog.
type CanonicalCatalog struct {
	Surfaces []CanonicalSurface
}

// LoadCanonicalCatalog parses the canonical JSON catalog.
func LoadCanonicalCatalog(repoRoot string) (*CanonicalCatalog, error) {
	raw, err := LoadCanonicalCatalogBytes(repoRoot)
	if err != nil {
		return nil, err
	}
	return parseCanonicalCatalog(raw)
}

// LoadCanonicalCatalogFrom loads the canonical JSON catalog from an explicit
// filesystem path. Used by tests and by tooling that needs to point at a
// non-default catalog location.
func LoadCanonicalCatalogFrom(path string) (*CanonicalCatalog, error) {
	raw, err := readFileBytes(path)
	if err != nil {
		return nil, &CatalogError{Reason: "read " + path + ": " + err.Error()}
	}
	return parseCanonicalCatalog(raw)
}

// ProjectContracts projects the canonical catalog into the contract shape used
// by ValidateRegistry. Each canonical surface yields exactly one contract.
func (c *CanonicalCatalog) ProjectContracts() ([]*ProducerContract, []SurfaceRecord) {
	var contracts []*ProducerContract
	var surfaces []SurfaceRecord
	for _, s := range c.Surfaces {
		contract := &ProducerContract{
			SurfaceID:         s.ID,
			Status:            s.Status,
			Producer:          s.Producer,
			WriterFiles:       cloneStringSlice(s.WriterFiles),
			WriterSymbols:     cloneStringSlice(s.WriterSymbols),
			Sanitizer:         s.Sanitizer,
			PersistencePolicy: s.PersistencePolicy,
			BinaryPolicy:      s.BinaryPolicy,
			TestFiles:         cloneStringSlice(s.TestFiles),
			Justification:     s.Justification,
			Owner:             s.Owner,
			CommittedAllowed:  s.CommittedAllowed,
			SurfacePath:       s.Path,
		}
		contracts = append(contracts, contract)
		surfaces = append(surfaces, SurfaceRecord{
			ID:               s.ID,
			Path:             s.Path,
			Producer:         s.Producer,
			CommittedAllowed: s.CommittedAllowed,
			Sensitivity:      s.Sensitivity,
			Sanitizer:        s.Sanitizer,
		})
	}
	return contracts, surfaces
}

// CatalogMetrics is the executable metric summary used by the producer gate.
type CatalogMetrics struct {
	Total                   int
	ByStatus                map[ProducerStatus]int
	ActiveBySensitivity     map[string]int
	BinaryPolicyCounts      map[BinaryPolicy]int
	PersistencePolicyCounts map[PersistencePolicy]int
	ProducerCounts          map[string]int
}

// ComputeMetrics derives the metric summary from the catalog.
func (c *CanonicalCatalog) ComputeMetrics() CatalogMetrics {
	m := CatalogMetrics{
		Total:                   len(c.Surfaces),
		ByStatus:                map[ProducerStatus]int{},
		ActiveBySensitivity:     map[string]int{},
		BinaryPolicyCounts:      map[BinaryPolicy]int{},
		PersistencePolicyCounts: map[PersistencePolicy]int{},
		ProducerCounts:          map[string]int{},
	}
	for _, s := range c.Surfaces {
		m.ByStatus[s.Status]++
		if s.Status == StatusActive {
			m.ActiveBySensitivity[s.Sensitivity]++
		}
		m.BinaryPolicyCounts[s.BinaryPolicy]++
		m.PersistencePolicyCounts[s.PersistencePolicy]++
		m.ProducerCounts[s.Producer]++
	}
	return m
}

// ValidateCanonical performs closed-shape validation of the catalog.
func (c *CanonicalCatalog) ValidateCanonical() []string {
	var errors []string
	seen := map[string]bool{}

	validSensitivities := map[string]bool{"low": true, "medium": true, "high": true}
	validOutputFormats := map[string]bool{
		"json": true, "text": true, "csv": true, "config": true,
		"public_certificate": true, "public_key": true,
		"binary_profile": true, "other_binary": true, "mixed": true,
	}
	validEnforcementStates := map[string]bool{
		EnforcementStateMigrated:      true,
		EnforcementStateLegacyBypass:  true,
		EnforcementStateNotApplicable: true,
	}
	validOwnershipScopes := map[string]bool{
		"": true, OwnershipScopeSymbol: true, OwnershipScopeDedicatedFile: true,
	}

	for _, s := range c.Surfaces {
		if s.ID == "" {
			errors = append(errors, "missing surface: empty id")
		}
		if s.Path == "" {
			errors = append(errors, "missing surface: empty path ("+s.ID+")")
		}
		if s.Producer == "" {
			errors = append(errors, "missing surface: empty producer ("+s.ID+")")
		}
		if seen[s.ID] {
			errors = append(errors, "duplicate surface: "+s.ID)
		}
		seen[s.ID] = true

		if !IsValidStatus(s.Status) {
			errors = append(errors, "unknown surface status: "+s.ID+" status="+string(s.Status))
		}
		if !validSensitivities[s.Sensitivity] {
			errors = append(errors, "sensitivity mismatch: "+s.ID+" sensitivity="+s.Sensitivity)
		}
		if !IsValidBinaryPolicy(s.BinaryPolicy) {
			errors = append(errors, "policy mismatch: "+s.ID+" binary_policy="+string(s.BinaryPolicy))
		}
		if !IsValidPersistencePolicy(s.PersistencePolicy) {
			errors = append(errors, "policy mismatch: "+s.ID+" persistence_policy="+string(s.PersistencePolicy))
		}
		if !validOutputFormats[s.OutputFormat] {
			errors = append(errors, "policy mismatch: "+s.ID+" output_format="+s.OutputFormat)
		}
		if !validEnforcementStates[s.EnforcementState] {
			errors = append(errors, "enforcement state mismatch: "+s.ID+" enforcement_state="+s.EnforcementState)
		}
		if !validOwnershipScopes[s.OwnershipScope] {
			errors = append(errors, "ownership scope mismatch: "+s.ID+" ownership_scope="+s.OwnershipScope)
		}
		if s.EnforcementState == EnforcementStateMigrated {
			if s.Status != StatusActive {
				errors = append(errors, "status mismatch: "+s.ID+" migrated surface must be active")
			}
			if s.OwnershipScope != OwnershipScopeSymbol && s.OwnershipScope != OwnershipScopeDedicatedFile {
				errors = append(errors, "ownership scope mismatch: "+s.ID+" migrated surface requires scope")
			}
		} else if s.OwnershipScope != "" {
			errors = append(errors, "ownership scope mismatch: "+s.ID+" non-migrated surface declares scope")
		}
		if s.Status != StatusActive && s.EnforcementState != EnforcementStateNotApplicable {
			errors = append(errors, "enforcement state mismatch: "+s.ID+" non-active surface must be not_applicable")
		}
		if _, err := CompileInventoryPattern(s.Path); err != nil {
			errors = append(errors, "path mismatch: "+s.ID+" "+err.Error())
		}
		if s.Status == StatusActive && s.Sensitivity == "high" {
			if s.Sanitizer == "" || s.Sanitizer == "none" {
				errors = append(errors, "sanitizer mismatch: "+s.ID+" ACTIVE high sensitivity requires typed sanitizer")
			}
		}
		if s.Status == StatusProspective && s.PersistencePolicy != PersistenceProspectiveNoWriter {
			errors = append(errors, "policy mismatch: "+s.ID+" PROSPECTIVE requires prospective_no_writer")
		}
		if s.Status == StatusStatic && len(s.WriterFiles) > 0 {
			errors = append(errors, "unknown surface: "+s.ID+" STATIC cannot claim writer_files")
		}
		if s.Status == StatusActive && s.BinaryPolicy != BinaryExactHashFixture && len(s.WriterFiles) == 0 {
			errors = append(errors, "status mismatch: "+s.ID+" ACTIVE requires writer_files")
		}
		if s.Owner == "" {
			errors = append(errors, "producer mismatch: "+s.ID+" missing owner")
		}
		if s.Justification == "" {
			errors = append(errors, "missing surface justification: "+s.ID)
		}
	}
	return errors
}

// ValidationOptions controls ValidateCatalog behavior.
type ValidationOptions struct {
	RepoRoot                string
	CheckWriterFilesExist   bool
	CheckWriterSymbolsExist bool
	CheckTestFilesExist     bool
	EnforceCommittedAllowed bool
}

// DefaultValidationOptions returns the recommended validation options for
// the producer gate. All checks enabled.
func DefaultValidationOptions(repoRoot string) ValidationOptions {
	return ValidationOptions{
		RepoRoot:                repoRoot,
		CheckWriterFilesExist:   true,
		CheckWriterSymbolsExist: true,
		CheckTestFilesExist:     true,
		EnforceCommittedAllowed: true,
	}
}

// CloneContracts returns an independent deep copy of the given contracts.
func CloneContracts(contracts []*ProducerContract) []*ProducerContract {
	out := make([]*ProducerContract, 0, len(contracts))
	for _, c := range contracts {
		if c == nil {
			out = append(out, nil)
			continue
		}
		cc := *c
		cc.WriterFiles = cloneStringSlice(c.WriterFiles)
		cc.WriterSymbols = cloneStringSlice(c.WriterSymbols)
		cc.TestFiles = cloneStringSlice(c.TestFiles)
		out = append(out, &cc)
	}
	return out
}

func cloneStringSlice(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func joinRepoPath(repoRoot, rel string) string {
	if repoRoot == "" {
		return rel
	}
	if rel == "" {
		return repoRoot
	}
	return repoRoot + "/" + rel
}

func readFileBytes(path string) ([]byte, error) {
	return defaultReadFile(path)
}
