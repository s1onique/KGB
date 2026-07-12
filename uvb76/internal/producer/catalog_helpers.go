package producer

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ValidateCanonicalCatalogBytes parses raw bytes and validates them.
func ValidateCanonicalCatalogBytes(raw []byte) ([]string, *CanonicalCatalog) {
	cat, err := parseCanonicalCatalog(raw)
	if err != nil {
		return []string{"decode: " + err.Error()}, nil
	}
	return cat.ValidateCanonical(), cat
}

// EncodeCanonicalJSON serializes a CanonicalCatalog back to JSON. Used by tests.
func EncodeCanonicalJSON(c *CanonicalCatalog) ([]byte, error) {
	if c == nil {
		return nil, errors.New("nil catalog")
	}
	return json.MarshalIndent(c.Surfaces, "", "  ")
}

// CatalogSummary returns a small printable summary of the catalog.
func CatalogSummary(c *CanonicalCatalog) string {
	if c == nil {
		return "nil catalog"
	}
	m := c.ComputeMetrics()
	var summary strings.Builder
	fmt.Fprintf(&summary, "total=%d ", m.Total)
	for status, count := range m.ByStatus {
		fmt.Fprintf(&summary, "%s=%d ", string(status), count)
	}
	return strings.TrimSpace(summary.String())
}
