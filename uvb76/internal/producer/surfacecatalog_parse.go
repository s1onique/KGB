package producer

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// parseCanonicalCatalog decodes the canonical JSON catalog.
//
// The decoder is robust to whitespace and trailing newlines but strict on
// field types: any unknown types produce a parser error rather than silent
// defaulting. This keeps validator output trustworthy.
func parseCanonicalCatalog(raw []byte) (*CanonicalCatalog, error) {
	if len(raw) == 0 {
		return nil, &CatalogError{Reason: "empty catalog"}
	}
	var doc struct {
		Surfaces []json.RawMessage `json:"surfaces"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, &CatalogError{Reason: "decode: " + err.Error()}
	}
	out := &CanonicalCatalog{Surfaces: make([]CanonicalSurface, 0, len(doc.Surfaces))}
	for i, rs := range doc.Surfaces {
		s, err := parseSurface(rs)
		if err != nil {
			return nil, &CatalogError{Reason: "surface " + itoa(i) + ": " + err.Error()}
		}
		out.Surfaces = append(out.Surfaces, s)
	}
	return out, nil
}

func parseSurface(raw json.RawMessage) (CanonicalSurface, error) {
	var d struct {
		ID                string   `json:"id"`
		Path              string   `json:"path"`
		Producer          string   `json:"producer"`
		CommittedAllowed  bool     `json:"committed_allowed"`
		Sensitivity       string   `json:"sensitivity"`
		Sanitizer         string   `json:"sanitizer"`
		Status            string   `json:"status"`
		PersistencePolicy string   `json:"persistence_policy"`
		BinaryPolicy      string   `json:"binary_policy"`
		OutputFormat      string   `json:"output_format"`
		Owner             string   `json:"owner"`
		Justification     string   `json:"justification"`
		EnforcementState  string   `json:"enforcement_state"`
		OwnershipScope    string   `json:"ownership_scope"`
		WriterFiles       []string `json:"writer_files"`
		WriterSymbols     []string `json:"writer_symbols"`
		TestFiles         []string `json:"test_files"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return CanonicalSurface{}, err
	}
	cs := CanonicalSurface{
		ID:                d.ID,
		Path:              d.Path,
		Producer:          d.Producer,
		CommittedAllowed:  d.CommittedAllowed,
		Sensitivity:       d.Sensitivity,
		Sanitizer:         d.Sanitizer,
		Status:            ProducerStatus(d.Status),
		PersistencePolicy: PersistencePolicy(d.PersistencePolicy),
		BinaryPolicy:      BinaryPolicy(d.BinaryPolicy),
		OutputFormat:      d.OutputFormat,
		Owner:             d.Owner,
		Justification:     d.Justification,
		EnforcementState:  d.EnforcementState,
		OwnershipScope:    d.OwnershipScope,
		WriterFiles:       append([]string(nil), d.WriterFiles...),
		WriterSymbols:     append([]string(nil), d.WriterSymbols...),
		TestFiles:         append([]string(nil), d.TestFiles...),
	}
	// Default enforcement_state by status when not declared.
	if cs.EnforcementState == "" {
		switch cs.Status {
		case StatusActive:
			cs.EnforcementState = EnforcementStateLegacyBypass
		default:
			cs.EnforcementState = EnforcementStateNotApplicable
		}
	}
	if cs.OutputFormat == "" {
		cs.OutputFormat = "json"
	}
	if cs.Sanitizer == "" {
		cs.Sanitizer = "none"
	}
	return cs, nil
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// defaultReadFile is the default file reader used by the loader. Tests override
// this to point at temporary catalog fixtures.
var defaultReadFile = func(path string) ([]byte, error) {
	if path == "" {
		return nil, &CatalogError{Reason: "empty path"}
	}
	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, "..") {
		return nil, &CatalogError{Reason: "path escape"}
	}
	f, err := os.Open(cleaned)
	if err != nil {
		return nil, &CatalogError{Reason: "open " + cleaned + ": " + err.Error()}
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, &CatalogError{Reason: "read " + cleaned + ": " + err.Error()}
	}
	return data, nil
}

// os_Getwd is the Getwd override point used by the catalog root resolver.
// Tests substitute a fake working directory to avoid real filesystem walks.
var os_Getwd = func() (string, error) {
	return osGetwdReal()
}

func osGetwdReal() (string, error) {
	return os.Getwd()
}
