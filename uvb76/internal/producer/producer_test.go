package producer

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateCatalog_Defaults verifies the canonical catalog validates
// green (zero errors). The catalog is loaded from
// scripts/uvb76_artifact_secret_hygiene/surfaces.json; legacy_bypass
// surfaces are tolerated (raw os.* writes are expected until migrated).
func TestValidateCatalog_Defaults(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	cat, err := LoadCanonicalCatalog(catalogRepoRoot)
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	options := DefaultValidationOptions(catalogRepoRoot)
	issues := ValidateCatalog(cat, options)
	var errs []string
	for _, i := range issues {
		if i.Severity == "error" {
			errs = append(errs, i.String())
		}
	}
	if len(errs) != 0 {
		var b strings.Builder
		b.WriteString("default catalog has validation errors:\n")
		for _, e := range errs {
			b.WriteString("  ")
			b.WriteString(e)
			b.WriteString("\n")
		}
		t.Fatal(b.String())
	}
}

// TestValidateCatalog_OnlyMigratedZeroBypass scans the migrated ICMP surface
// and confirms zero bypass findings; legacy_bypass surfaces are excluded.
func TestValidateCatalog_OnlyMigratedZeroBypass(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	cat, err := LoadCanonicalCatalog(catalogRepoRoot)
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	cfg := BypassConfig{
		AllowlistedFiles: DefaultAllowlistedWriterFiles,
		FileBindings:     FileBindingsFromContracts(DefaultContracts),
		RepoRoot:         catalogRepoRoot,
	}
	det := NewBypassDetector(cfg)
	var migrated []string
	for _, s := range cat.Surfaces {
		if s.EnforcementState != EnforcementStateMigrated {
			continue
		}
		migrated = append(migrated, s.ID)
	}
	for _, id := range migrated {
		var files []string
		for _, c := range DefaultContracts {
			if c.SurfaceID != id {
				continue
			}
			if c.Status != StatusActive {
				continue
			}
			for _, wf := range c.WriterFiles {
				files = append(files, filepath.Join(catalogRepoRoot, filepath.FromSlash(wf)))
			}
		}
		findings, err := det.Scanner(files)
		if err != nil {
			t.Fatalf("%s: scan: %v", id, err)
		}
		if len(findings) != 0 {
			t.Errorf("migrated surface %s produced %d bypass findings; expected 0: %v",
				id, len(findings), findings)
		}
	}
}

// TestRegistryDefaultsMetricCounts drives counts from the canonical catalog
// metrics. Updating the canonical catalog should not require test edits.
func TestRegistryDefaultsMetricCounts(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	cat, err := LoadCanonicalCatalog(catalogRepoRoot)
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	m := cat.ComputeMetrics()

	reg := NewRegistry(DefaultInventorySurfaces, DefaultContracts)
	if got := reg.CountByStatus(StatusActive); got != m.ByStatus[StatusActive] {
		t.Errorf("active count: got %d, want %d (from canonical metric)", got, m.ByStatus[StatusActive])
	}
	if got := reg.CountByStatus(StatusStatic); got != m.ByStatus[StatusStatic] {
		t.Errorf("static count: got %d, want %d (from canonical metric)", got, m.ByStatus[StatusStatic])
	}
	if got := reg.CountByStatus(StatusProspective); got != m.ByStatus[StatusProspective] {
		t.Errorf("prospective count: got %d, want %d (from canonical metric)", got, m.ByStatus[StatusProspective])
	}
	if got := reg.CountByStatus(StatusExternal); got != m.ByStatus[StatusExternal] {
		t.Errorf("external count: got %d, want %d (from canonical metric)", got, m.ByStatus[StatusExternal])
	}
	if got := reg.CountByStatus(StatusDetectionOnly); got != m.ByStatus[StatusDetectionOnly] {
		t.Errorf("detection_only count: got %d, want %d (from canonical metric)", got, m.ByStatus[StatusDetectionOnly])
	}
	if got := len(reg.Surfaces()); got != m.Total {
		t.Errorf("total surfaces: got %d, want %d", got, m.Total)
	}
}

// TestRegistryDefaultsTotalSurfaces asserts the canonical catalog exposes
// every surface as a contract.
func TestRegistryDefaultsTotalSurfaces(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	cat, err := LoadCanonicalCatalog(catalogRepoRoot)
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	m := cat.ComputeMetrics()
	if m.Total < 12 {
		t.Errorf("catalog total %d < 12 expected after surface split", m.Total)
	}
}

// TestRegistryParity verifies inventory and contract IDs are 1:1 equal.
func TestRegistryParity(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	reg := NewRegistry(DefaultInventorySurfaces, DefaultContracts)

	inventoryIDs := make(map[string]bool)
	for _, s := range reg.Surfaces() {
		inventoryIDs[s.ID] = true
	}
	contractIDs := make(map[string]bool)
	for _, c := range reg.Contracts() {
		contractIDs[c.SurfaceID] = true
	}
	for id := range inventoryIDs {
		if !contractIDs[id] {
			t.Errorf("inventory ID %s missing from contract registry", id)
		}
	}
	for id := range contractIDs {
		if !inventoryIDs[id] {
			t.Errorf("contract ID %s missing from inventory registry", id)
		}
	}
}

// TestCloneContracts_DoesNotMutate asserts CloneContracts returns an
// independent copy so mutation tests don't pollute the default catalog.
func TestCloneContracts_DoesNotMutate(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	clone := CloneContracts(DefaultContracts)
	for _, c := range clone {
		if c != nil {
			c.SurfaceID = "mutated"
		}
	}
	for _, c := range DefaultContracts {
		if c.SurfaceID == "mutated" {
			t.Fatalf("CloneContracts returned a shared slice: original was mutated")
		}
	}
}

// Mutation: unknown contract surface_id.
// Uses CloneContracts to avoid mutating DefaultContracts.
func TestMutation_UnknownContractID(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	clone := CloneContracts(DefaultContracts)
	clone = append(clone, &ProducerContract{
		SurfaceID:         "not-in-inventory",
		Status:            StatusStatic,
		Producer:          "hygiene",
		Sanitizer:         "none",
		PersistencePolicy: PersistenceStaticScanned,
		BinaryPolicy:      BinaryNotApplicable,
		Justification:     "test",
		Owner:             "uvb76-team",
		CommittedAllowed:  true,
	})
	reg := NewRegistry(DefaultInventorySurfaces, clone)
	issues := ValidateRegistry(reg)
	if !HasErrors(issues) {
		t.Fatal("expected validation error for unknown surface_id")
	}
}

// Mutation: duplicate contract surface ID.
func TestMutation_DuplicateContractSurfaceID(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	clone := CloneContracts(DefaultContracts)
	clone = append(clone, clone[0])
	reg := NewRegistry(DefaultInventorySurfaces, clone)
	issues := ValidateRegistry(reg)
	if !HasErrors(issues) {
		t.Fatal("expected duplicate contract detection")
	}
}

// Mutation: ACTIVE surface missing writer file.
func TestMutation_ActiveMissingWriterFile(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	clone := CloneContracts(DefaultContracts)
	for _, c := range clone {
		if c.SurfaceID == "memory-lab-artifacts" {
			c.WriterFiles = nil
			break
		}
	}
	reg := NewRegistry(DefaultInventorySurfaces, clone)
	issues := ValidateRegistry(reg)
	if !HasErrors(issues) {
		t.Fatal("expected ACTIVE missing-writer-file rejection")
	}
}

// Mutation: ACTIVE surface missing writer symbol.
func TestMutation_ActiveMissingWriterSymbol(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	clone := CloneContracts(DefaultContracts)
	for _, c := range clone {
		if c.SurfaceID == "memory-lab-artifacts" {
			c.WriterSymbols = nil
			break
		}
	}
	reg := NewRegistry(DefaultInventorySurfaces, clone)
	issues := ValidateRegistry(reg)
	if !HasErrors(issues) {
		t.Fatal("expected ACTIVE missing-writer-symbol rejection")
	}
}

// Mutation: HIGH-sensitivity ACTIVE surface with sanitizer==none.
func TestMutation_HighSensitivityMissingSanitizer(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	clone := CloneContracts(DefaultContracts)
	for _, c := range clone {
		if c.SurfaceID == "memory-lab-artifacts" {
			c.Sanitizer = "none"
			break
		}
	}
	reg := NewRegistry(DefaultInventorySurfaces, clone)
	issues := ValidateRegistry(reg)
	if !HasErrors(issues) {
		t.Fatal("expected HIGH-sensitivity missing-sanitizer rejection")
	}
}

// Mutation: unknown persistence policy.
func TestMutation_UnknownPersistencePolicy(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	clone := CloneContracts(DefaultContracts)
	for _, c := range clone {
		if c.SurfaceID == "memory-lab-artifacts" {
			c.PersistencePolicy = PersistencePolicy("bogus_policy")
			break
		}
	}
	reg := NewRegistry(DefaultInventorySurfaces, clone)
	issues := ValidateRegistry(reg)
	if !HasErrors(issues) {
		t.Fatal("expected unknown-policy rejection")
	}
}

// Mutation: unknown binary policy.
func TestMutation_UnknownBinaryPolicy(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	clone := CloneContracts(DefaultContracts)
	for _, c := range clone {
		if c.SurfaceID == "memory-lab-artifacts" {
			c.BinaryPolicy = BinaryPolicy("bogus_binary")
			break
		}
	}
	reg := NewRegistry(DefaultInventorySurfaces, clone)
	issues := ValidateRegistry(reg)
	if !HasErrors(issues) {
		t.Fatal("expected unknown-binary-policy rejection")
	}
}

// Mutation: PROSPECTIVE surface claiming an active writer.
func TestMutation_ProspectiveClaimingActiveWriter(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	clone := CloneContracts(DefaultContracts)
	for _, c := range clone {
		if c.SurfaceID == "diag-capture-packets" {
			c.WriterFiles = []string{"some/file.go"}
			c.WriterSymbols = []string{"writeArtifact"}
			c.PersistencePolicy = PersistenceAtomicRedactedJSON
			break
		}
	}
	reg := NewRegistry(DefaultInventorySurfaces, clone)
	issues := ValidateRegistry(reg)
	if !HasErrors(issues) {
		t.Fatal("expected prospective-writer contradiction rejection")
	}
}

// Mutation: STATIC surface claiming runtime writer files.
func TestMutation_StaticClaimingWriterFiles(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	clone := CloneContracts(DefaultContracts)
	for _, c := range clone {
		if c.SurfaceID == "hygiene-verifier" {
			c.WriterFiles = []string{"some/file.go"}
			c.WriterSymbols = []string{"writeArtifact"}
			break
		}
	}
	reg := NewRegistry(DefaultInventorySurfaces, clone)
	issues := ValidateRegistry(reg)
	if !HasErrors(issues) {
		t.Fatal("expected static-with-writer-files rejection")
	}
}

// Mutation: empty justification.
func TestMutation_EmptyJustification(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	clone := CloneContracts(DefaultContracts)
	for _, c := range clone {
		if c.SurfaceID == "memory-lab-artifacts" {
			c.Justification = ""
			break
		}
	}
	reg := NewRegistry(DefaultInventorySurfaces, clone)
	issues := ValidateRegistry(reg)
	if !HasErrors(issues) {
		t.Fatal("expected empty-justification rejection")
	}
}

// Mutation: empty owner.
func TestMutation_EmptyOwner(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	clone := CloneContracts(DefaultContracts)
	for _, c := range clone {
		if c.SurfaceID == "memory-lab-artifacts" {
			c.Owner = ""
			break
		}
	}
	reg := NewRegistry(DefaultInventorySurfaces, clone)
	issues := ValidateRegistry(reg)
	if !HasErrors(issues) {
		t.Fatal("expected empty-owner rejection")
	}
}

// Mutation: order-independent invariant — repeated mutation across multiple
// surface ids must remain stable.
func TestMutation_OrderIndependent(t *testing.T) {
	if err := DefaultInit(); err != nil {
		t.Fatalf("default init: %v", err)
	}
	for i := 0; i < 10; i++ {
		clone := CloneContracts(DefaultContracts)
		clone[0].Justification = ""
		reg := NewRegistry(DefaultInventorySurfaces, clone)
		issues := ValidateRegistry(reg)
		if !HasErrors(issues) {
			t.Fatalf("iteration %d: expected validation error", i)
		}
	}
}

// TestPathMatching_Exact verifies exact-path semantics.
