// Package producer enforces the executable producer contract catalog
// for UVB-76 artifact surfaces.
//
// Each inventory surface declared in scripts/uvb76_artifact_secret_hygiene
// has exactly one ProducerContract here. The validator rejects any
// inventory surface without a producer contract, any producer contract
// without an inventory surface, and any inconsistency between the two.
//
// ACT-UVB76-HULK05R4
package producer

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// ProducerStatus is the closed lifecycle classification of a producer.
type ProducerStatus string

const (
	StatusActive       ProducerStatus = "active"
	StatusStatic       ProducerStatus = "static"
	StatusProspective  ProducerStatus = "prospective"
	StatusExternal     ProducerStatus = "external"
	StatusDetectionOnly ProducerStatus = "detection_only"
)

// AllProducerStatuses is the closed set of legal status values.
var AllProducerStatuses = []ProducerStatus{
	StatusActive, StatusStatic, StatusProspective,
	StatusExternal, StatusDetectionOnly,
}

// IsValidStatus reports whether s is a recognized ProducerStatus.
func IsValidStatus(s ProducerStatus) bool {
	for _, v := range AllProducerStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// BinaryPolicy is the closed binary persistence policy.
type BinaryPolicy string

const (
	BinaryReject                BinaryPolicy = "reject"
	BinaryPublicCertOnly        BinaryPolicy = "public_certificate_only"
	BinaryPublicKeyOnly         BinaryPolicy = "public_key_only"
	BinaryExactHashFixture      BinaryPolicy = "exact_hash_fixture"
	BinaryArchiveMemberScan     BinaryPolicy = "archive_member_scan"
	BinaryTextOnlyWithinMixed   BinaryPolicy = "text_only_within_mixed_root"
	BinaryNotApplicable         BinaryPolicy = "not_applicable"
)

// AllBinaryPolicies is the closed set of legal binary policy values.
var AllBinaryPolicies = []BinaryPolicy{
	BinaryReject, BinaryPublicCertOnly, BinaryPublicKeyOnly,
	BinaryExactHashFixture, BinaryArchiveMemberScan,
	BinaryTextOnlyWithinMixed, BinaryNotApplicable,
}

// IsValidBinaryPolicy reports whether p is a recognized BinaryPolicy.
func IsValidBinaryPolicy(p BinaryPolicy) bool {
	for _, v := range AllBinaryPolicies {
		if v == p {
			return true
		}
	}
	return false
}

// PersistencePolicy is the closed persistence policy descriptor.
type PersistencePolicy string

const (
	PersistenceAtomicRedactedJSON   PersistencePolicy = "atomic_redacted_json"
	PersistenceAtomicRedactedText   PersistencePolicy = "atomic_redacted_text"
	PersistenceAtomicRedactedConfig PersistencePolicy = "atomic_redacted_config"
	PersistenceStaticScanned        PersistencePolicy = "static_scanned"
	PersistenceRejectBinary         PersistencePolicy = "reject_binary"
	PersistenceAllowPublicCert      PersistencePolicy = "allow_public_certificate"
	PersistenceAllowPublicKey       PersistencePolicy = "allow_public_key"
	PersistenceExactHashFixture     PersistencePolicy = "exact_hash_fixture"
	PersistenceExternalValidated    PersistencePolicy = "external_validated"
	PersistenceProspectiveNoWriter  PersistencePolicy = "prospective_no_writer"
)

// AllPersistencePolicies is the closed set of legal persistence policies.
var AllPersistencePolicies = []PersistencePolicy{
	PersistenceAtomicRedactedJSON, PersistenceAtomicRedactedText,
	PersistenceAtomicRedactedConfig, PersistenceStaticScanned,
	PersistenceRejectBinary, PersistenceAllowPublicCert,
	PersistenceAllowPublicKey, PersistenceExactHashFixture,
	PersistenceExternalValidated, PersistenceProspectiveNoWriter,
}

// IsValidPersistencePolicy reports whether p is a recognized policy.
func IsValidPersistencePolicy(p PersistencePolicy) bool {
	for _, v := range AllPersistencePolicies {
		if v == p {
			return true
		}
	}
	return false
}

// ExactHashException is the closed descriptor for EXACT_HASH_FIXTURE policy.
type ExactHashException struct {
	SHA256        string
	Owner         string
	Justification string
}

// ArchiveScanBounds is the closed descriptor for ARCHIVE_MEMBER_SCAN bounds.
type ArchiveScanBounds struct {
	MemberCount    int
	MemberSize     int
	TotalSize      int
	RecursionDepth int
}

// ProducerContract is the canonical contract for an inventory surface.
type ProducerContract struct {
	// SurfaceID matches the inventory surface ID 1:1.
	SurfaceID string

	// Status is the closed lifecycle classification.
	Status ProducerStatus

	// Producer names the owning producer or component.
	Producer string

	// WriterFiles is the set of Go source files that perform persistence
	// for this surface. Empty for STATIC/PROSPECTIVE/EXTERNAL/DETECTION_ONLY.
	WriterFiles []string

	// WriterSymbols is the set of Go function names that perform persistence
	// for this surface. Empty for STATIC/PROSPECTIVE/EXTERNAL/DETECTION_ONLY.
	WriterSymbols []string

	// Sanitizer is the typed sanitizer name from the redact/artifactio
	// boundary vocabulary.
	Sanitizer string

	// PersistencePolicy is the closed persistence policy.
	PersistencePolicy PersistencePolicy

	// BinaryPolicy is the closed binary policy.
	BinaryPolicy BinaryPolicy

	// TestFiles is the set of test files exercising the producer boundary.
	TestFiles []string

	// Justification is the human-readable reason this contract exists.
	Justification string

	// Owner is the human/team responsible for the contract.
	Owner string

	// CommittedAllowed reports whether tracked commit of the surface is allowed.
	CommittedAllowed bool

	// SurfacePath is the primary inventory path.
	SurfacePath string
}

// String renders the contract for human-readable diagnostics. Does not
// include writer files/symbols or test files.
func (c *ProducerContract) String() string {
	return fmt.Sprintf("contract(surface=%s, status=%s, producer=%s, sanitizer=%s, "+
		"persistence=%s, binary=%s, committed=%v)",
		c.SurfaceID, c.Status, c.Producer, c.Sanitizer,
		c.PersistencePolicy, c.BinaryPolicy, c.CommittedAllowed)
}

// Validate performs structural validation of the contract fields. It does
// not check parity with inventory; that is the registry's responsibility.
func (c *ProducerContract) Validate() error {
	if c.SurfaceID == "" {
		return errors.New("producer contract: empty surface_id")
	}
	if c.Producer == "" {
		return fmt.Errorf("producer contract %s: empty producer", c.SurfaceID)
	}
	if c.Owner == "" {
		return fmt.Errorf("producer contract %s: empty owner", c.SurfaceID)
	}
	if c.Justification == "" {
		return fmt.Errorf("producer contract %s: empty justification", c.SurfaceID)
	}
	if !IsValidStatus(c.Status) {
		return fmt.Errorf("producer contract %s: invalid status %q", c.SurfaceID, c.Status)
	}
	if !IsValidBinaryPolicy(c.BinaryPolicy) {
		return fmt.Errorf("producer contract %s: invalid binary policy %q", c.SurfaceID, c.BinaryPolicy)
	}
	if !IsValidPersistencePolicy(c.PersistencePolicy) {
		return fmt.Errorf("producer contract %s: invalid persistence policy %q",
			c.SurfaceID, c.PersistencePolicy)
	}
	switch c.Status {
	case StatusActive:
		if len(c.WriterFiles) == 0 {
			return fmt.Errorf("producer contract %s: ACTIVE requires at least one writer file", c.SurfaceID)
		}
		if len(c.WriterSymbols) == 0 {
			return fmt.Errorf("producer contract %s: ACTIVE requires at least one writer symbol", c.SurfaceID)
		}
	}
	switch c.PersistencePolicy {
	case PersistenceProspectiveNoWriter:
		if c.Status != StatusProspective && c.Status != StatusExternal {
			return fmt.Errorf(
				"producer contract %s: prospective_no_writer only valid with status=prospective|external, got %q",
				c.SurfaceID, c.Status,
			)
		}
	case PersistenceExternalValidated:
		if c.Status != StatusExternal && c.Status != StatusActive {
			return fmt.Errorf(
				"producer contract %s: external_validated only valid with status=external|active, got %q",
				c.SurfaceID, c.Status,
			)
		}
	}
	return nil
}

// HasWriterFile reports whether a path matches any declared writer file.
// Returns the matched pattern or empty.
func (c *ProducerContract) HasWriterFile(path string) string {
	for _, wf := range c.WriterFiles {
		if path == wf {
			return wf
		}
	}
	return ""
}

// IsTextOrJSONOrConfig returns true when the surface is text/JSON/config
// and the binary policy is not_applicable.
func (c *ProducerContract) IsTextOrJSONOrConfig() bool {
	return c.BinaryPolicy == BinaryNotApplicable
}

// NormalizedPath returns the surface path with forward slashes and no
// trailing slash.
func NormalizedPath(p string) string {
	s := strings.ReplaceAll(p, "\\", "/")
	for len(s) > 1 && strings.HasSuffix(s, "/") {
		s = s[:len(s)-1]
	}
	return s
}

// ValidateFileMode is a convenience validator helper.
func ValidateFileMode(m fs.FileMode) error {
	if m == 0 {
		return errors.New("file mode must be non-zero")
	}
	return nil
}
