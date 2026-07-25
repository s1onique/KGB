// Package qualification owns the typed, independently verifiable authority
// chain for the two qualification-harness binaries.
package qualification

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// RecordSchemaVersion is the on-disk schema version for role-separation.json.
	RecordSchemaVersion = "qualification-role-separation/v1"
	// LiveHelperTest is the only test that can authorize the live-helper role.
	LiveHelperTest = "TestLiveDockerSmoke_QualifiedExecutionPath"
)

// BinaryRole identifies the authority a binary is allowed to exercise.
type BinaryRole string

const (
	BinaryRoleLiveHelper    BinaryRole = "live_helper"
	BinaryRoleProductionCLI BinaryRole = "production_cli"
)

// BinaryRecord contains facts reconstructed from one binary and its filesystem
// entry. None of these fields are claims: the verifier recomputes all of them.
type BinaryRecord struct {
	AbsolutePath string     `json:"absolute_path"`
	Device       uint64     `json:"device"`
	Inode        uint64     `json:"inode"`
	Size         int64      `json:"size"`
	SHA256       string     `json:"sha256"`
	VCS          string     `json:"vcs"`
	VCSRevision  string     `json:"vcs_revision"`
	VCSTime      string     `json:"vcs_time"`
	VCSModified  bool       `json:"vcs_modified"`
	Role         BinaryRole `json:"role"`
}

// QualificationRecord is the complete external authority record. Required
// fields intentionally have no omitempty tags; DecodeQualificationRecord also
// rejects absent and null fields rather than allowing Go zero values to hide
// malformed evidence.
type QualificationRecord struct {
	SchemaVersion          string       `json:"schema_version"`
	SourceRoot             string       `json:"source_root"`
	SourceCommit           string       `json:"source_commit"`
	SourceTree             string       `json:"source_tree"`
	Helper                 BinaryRecord `json:"helper"`
	Production             BinaryRecord `json:"production"`
	HelperLiveTest         string       `json:"helper_live_test"`
	ProductionHelpExitCode int          `json:"production_help_exit_code"`
}

var (
	ErrRecordUnknownField          = errors.New("qualification record contains an unknown field")
	ErrRecordMissingField          = errors.New("qualification record is missing a required field")
	ErrRecordNullField             = errors.New("qualification record contains a null required field")
	ErrRecordWrongType             = errors.New("qualification record contains a field with the wrong type")
	ErrRecordSecondJSON            = errors.New("qualification record contains a second JSON document")
	ErrRecordTrailingData          = errors.New("qualification record contains trailing non-whitespace data")
	ErrRecordDuplicateKey          = errors.New("qualification record contains a duplicate key")
	ErrRecordInvalid               = errors.New("qualification record contains invalid values")
	ErrSourceCommitMismatch        = errors.New("source commit does not match the checkout")
	ErrSourceTreeMismatch          = errors.New("source tree does not match the checkout")
	ErrSourceRootMismatch          = errors.New("source root does not match the checkout")
	ErrRelationshipSamePath        = errors.New("helper and production use the same path")
	ErrRelationshipSameDeviceInode = errors.New("helper and production use the same device and inode")
	ErrRelationshipSameHash        = errors.New("helper and production use the same SHA-256")
	ErrHelperRevisionMismatch      = errors.New("helper embedded VCS revision mismatch")
	ErrProductionRevisionMismatch  = errors.New("production embedded VCS revision mismatch")
	ErrHelperModified              = errors.New("helper binary is VCS-modified")
	ErrProductionModified          = errors.New("production binary is VCS-modified")
	ErrHelperTestMissing           = errors.New("helper does not expose the exact live test")
	ErrProductionHelpFailure       = errors.New("production --help did not exit successfully")
	ErrBuildVCSDisabled            = errors.New("GOFLAGS disables build VCS stamping")
	ErrDirtySource                 = errors.New("qualification source checkout is dirty")
	ErrBuildInfoRead               = errors.New("unable to read embedded Go build information")
	ErrMissingEmbeddedVCS          = errors.New("embedded vcs setting is missing")
	ErrMissingEmbeddedRevision     = errors.New("embedded vcs.revision setting is missing")
	ErrMissingEmbeddedTime         = errors.New("embedded vcs.time setting is missing")
	ErrMissingEmbeddedModified     = errors.New("embedded vcs.modified setting is missing")
	ErrEmptyEmbeddedModified       = errors.New("embedded vcs.modified setting is empty")
	ErrModifiedBinary              = errors.New("embedded vcs.modified is not false")
	ErrMalformedEmbeddedRevision   = errors.New("embedded vcs.revision is malformed")
	ErrEmbeddedRevisionMismatch    = errors.New("embedded vcs.revision does not match expected source commit")
)

var objectIDPattern = regexp.MustCompile(`^[0-9a-f]{40}(?:[0-9a-f]{24})?$`)
var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// MarshalQualificationRecord emits canonical, newline-terminated JSON.
func MarshalQualificationRecord(record QualificationRecord) ([]byte, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// DecodeQualificationRecord decodes exactly one strict JSON document. It uses
// raw-object presence checks before typed decoding so missing, null, and wrong
// type mutations remain distinguishable from ordinary zero values.
func DecodeQualificationRecord(data []byte) (QualificationRecord, error) {
	var top map[string]json.RawMessage
	if err := decodeExactlyOneObject(data, &top); err != nil {
		return QualificationRecord{}, err
	}
	if err := rejectUnknown(top, map[string]bool{
		"schema_version": true, "source_root": true, "source_commit": true,
		"source_tree": true, "helper": true, "production": true,
		"helper_live_test": true, "production_help_exit_code": true,
	}); err != nil {
		return QualificationRecord{}, err
	}

	var record QualificationRecord
	var err error
	if record.SchemaVersion, err = requiredString(top, "schema_version"); err != nil {
		return record, err
	}
	if record.SourceRoot, err = requiredString(top, "source_root"); err != nil {
		return record, err
	}
	if record.SourceCommit, err = requiredString(top, "source_commit"); err != nil {
		return record, err
	}
	if record.SourceTree, err = requiredString(top, "source_tree"); err != nil {
		return record, err
	}
	if record.Helper, err = requiredBinary(top, "helper"); err != nil {
		return record, err
	}
	if record.Production, err = requiredBinary(top, "production"); err != nil {
		return record, err
	}
	if record.HelperLiveTest, err = requiredString(top, "helper_live_test"); err != nil {
		return record, err
	}
	if record.ProductionHelpExitCode, err = requiredInt(top, "production_help_exit_code"); err != nil {
		return record, err
	}
	if err := record.Validate(); err != nil {
		return QualificationRecord{}, err
	}
	return record, nil
}

func decodeExactlyOneObject(data []byte, dst *map[string]json.RawMessage) error {
	if err := rejectDuplicateKeys(data); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if strings.Contains(err.Error(), "cannot unmarshal") || strings.Contains(err.Error(), "cannot decode") {
			return fmt.Errorf("%w: %v", ErrRecordWrongType, err)
		}
		return fmt.Errorf("%w: %v", ErrRecordWrongType, err)
	}
	var extra json.RawMessage
	err := dec.Decode(&extra)
	if err == nil {
		return ErrRecordSecondJSON
	}
	if !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: %v", ErrRecordTrailingData, err)
	}
	return nil
}

// rejectDuplicateKeys rejects records that declare the same object key more
// than once. The Go standard library silently coalesces duplicate map entries,
// so the independent verifier must defend against replayed records whose
// meaning is ambiguous.
func rejectDuplicateKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil
	}
	seen := map[string]int{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil
		}
		seen[key]++
		if seen[key] > 1 {
			return fmt.Errorf("%w: %s", ErrRecordDuplicateKey, key)
		}
		if err := rejectNestedDuplicateKeys(dec); err != nil {
			return err
		}
	}
	return nil
}

func rejectNestedDuplicateKeys(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); ok {
		if d == '[' {
			for dec.More() {
				if err := rejectNestedDuplicateKeys(dec); err != nil {
					return err
				}
			}
			closing, err := dec.Token()
			if err != nil {
				return err
			}
			_ = closing
			return nil
		}
		if d == '{' {
			seen := map[string]int{}
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return err
				}
				key, ok := keyTok.(string)
				if !ok {
					return err
				}
				seen[key]++
				if seen[key] > 1 {
					return fmt.Errorf("%w: nested %s", ErrRecordDuplicateKey, key)
				}
				if err := rejectNestedDuplicateKeys(dec); err != nil {
					return err
				}
			}
			closing, err := dec.Token()
			if err != nil {
				return err
			}
			_ = closing
			return nil
		}
	}
	return nil
}

func rejectUnknown(obj map[string]json.RawMessage, allowed map[string]bool) error {
	for key := range obj {
		if !allowed[key] {
			return fmt.Errorf("%w: %s", ErrRecordUnknownField, key)
		}
	}
	return nil
}

func requiredRaw(obj map[string]json.RawMessage, name string) (json.RawMessage, error) {
	raw, ok := obj[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRecordMissingField, name)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("%w: %s", ErrRecordNullField, name)
	}
	return raw, nil
}

func requiredString(obj map[string]json.RawMessage, name string) (string, error) {
	raw, err := requiredRaw(obj, name)
	if err != nil {
		return "", err
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrRecordWrongType, name, err)
	}
	return value, nil
}

func requiredInt(obj map[string]json.RawMessage, name string) (int, error) {
	raw, err := requiredRaw(obj, name)
	if err != nil {
		return 0, err
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("%w: %s: %v", ErrRecordWrongType, name, err)
	}
	return value, nil
}

func requiredBinary(obj map[string]json.RawMessage, name string) (BinaryRecord, error) {
	raw, err := requiredRaw(obj, name)
	if err != nil {
		return BinaryRecord{}, err
	}
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil || nested == nil {
		if err == nil {
			err = errors.New("object is null")
		}
		return BinaryRecord{}, fmt.Errorf("%w: %s: %v", ErrRecordWrongType, name, err)
	}
	if err := rejectUnknown(nested, map[string]bool{
		"absolute_path": true, "device": true, "inode": true, "size": true,
		"sha256": true, "vcs": true, "vcs_revision": true, "vcs_time": true,
		"vcs_modified": true, "role": true,
	}); err != nil {
		return BinaryRecord{}, err
	}
	var out BinaryRecord
	if out.AbsolutePath, err = requiredString(nested, "absolute_path"); err != nil {
		return out, err
	}
	if out.Device, err = requiredUint(nested, "device"); err != nil {
		return out, err
	}
	if out.Inode, err = requiredUint(nested, "inode"); err != nil {
		return out, err
	}
	if out.Size, err = requiredInt64(nested, "size"); err != nil {
		return out, err
	}
	if out.SHA256, err = requiredString(nested, "sha256"); err != nil {
		return out, err
	}
	if out.VCS, err = requiredString(nested, "vcs"); err != nil {
		return out, err
	}
	if out.VCSRevision, err = requiredString(nested, "vcs_revision"); err != nil {
		return out, err
	}
	if out.VCSTime, err = requiredString(nested, "vcs_time"); err != nil {
		return out, err
	}
	if out.VCSModified, err = requiredBool(nested, "vcs_modified"); err != nil {
		return out, err
	}
	var role string
	if role, err = requiredString(nested, "role"); err != nil {
		return out, err
	}
	out.Role = BinaryRole(role)
	return out, nil
}

func requiredUint(obj map[string]json.RawMessage, name string) (uint64, error) {
	raw, err := requiredRaw(obj, name)
	if err != nil {
		return 0, err
	}
	var value uint64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("%w: %s: %v", ErrRecordWrongType, name, err)
	}
	return value, nil
}
func requiredInt64(obj map[string]json.RawMessage, name string) (int64, error) {
	raw, err := requiredRaw(obj, name)
	if err != nil {
		return 0, err
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("%w: %s: %v", ErrRecordWrongType, name, err)
	}
	return value, nil
}
func requiredBool(obj map[string]json.RawMessage, name string) (bool, error) {
	raw, err := requiredRaw(obj, name)
	if err != nil {
		return false, err
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, fmt.Errorf("%w: %s: %v", ErrRecordWrongType, name, err)
	}
	return value, nil
}

// Validate checks semantic constraints which cannot be represented by JSON
// presence alone.
func (r QualificationRecord) Validate() error {
	if r.SchemaVersion != RecordSchemaVersion {
		return fmt.Errorf("%w: schema_version=%q", ErrRecordInvalid, r.SchemaVersion)
	}
	if !filepath.IsAbs(r.SourceRoot) || r.SourceRoot == "" {
		return fmt.Errorf("%w: source_root", ErrRecordInvalid)
	}
	if !objectIDPattern.MatchString(r.SourceCommit) {
		return fmt.Errorf("%w: source_commit", ErrRecordInvalid)
	}
	if !objectIDPattern.MatchString(r.SourceTree) {
		return fmt.Errorf("%w: source_tree", ErrRecordInvalid)
	}
	if err := r.Helper.validate(BinaryRoleLiveHelper); err != nil {
		return fmt.Errorf("helper: %w", err)
	}
	if err := r.Production.validate(BinaryRoleProductionCLI); err != nil {
		return fmt.Errorf("production: %w", err)
	}
	if r.HelperLiveTest == "" {
		return fmt.Errorf("%w: helper_live_test is empty", ErrRecordInvalid)
	}
	if r.ProductionHelpExitCode < 0 || r.ProductionHelpExitCode > 255 {
		return fmt.Errorf("%w: production_help_exit_code=%d", ErrRecordInvalid, r.ProductionHelpExitCode)
	}
	return nil
}

func (b BinaryRecord) validate(expectedRole BinaryRole) error {
	if !filepath.IsAbs(b.AbsolutePath) || b.AbsolutePath == "" {
		return fmt.Errorf("%w: absolute_path", ErrRecordInvalid)
	}
	if b.Size < 0 {
		return fmt.Errorf("%w: size", ErrRecordInvalid)
	}
	if !sha256Pattern.MatchString(b.SHA256) {
		return fmt.Errorf("%w: sha256", ErrRecordInvalid)
	}
	if b.VCS != "git" || b.VCSRevision == "" || !objectIDPattern.MatchString(b.VCSRevision) || b.VCSTime == "" {
		return fmt.Errorf("%w: embedded VCS fields", ErrRecordInvalid)
	}
	// A true value is parsed successfully so the independent verifier can
	// return the role-specific modified sentinel rather than hiding the
	// mutation behind a generic JSON/schema error.
	if b.Role != expectedRole {
		return fmt.Errorf("%w: role=%q", ErrRecordInvalid, b.Role)
	}
	return nil
}

// ValidateObjectID is shared by the builder and verifier.
func ValidateObjectID(value string) bool { return objectIDPattern.MatchString(value) }

// ValidateSHA256 is shared by the builder and verifier.
func ValidateSHA256(value string) bool { return sha256Pattern.MatchString(value) }
