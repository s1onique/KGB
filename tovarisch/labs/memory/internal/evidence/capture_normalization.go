package evidence

import (
	"bytes"
	"errors"
)

const MaxCapturedEvidenceBytes = 8 << 20

// NormalizeCapturedEvidence removes line-ending whitespace and emits exactly
// one final newline without mutating the supplied byte slice.
func NormalizeCapturedEvidence(input []byte) ([]byte, error) {
	if len(input) > MaxCapturedEvidenceBytes {
		return nil, errors.New("captured evidence exceeds 8 MiB bound")
	}
	normalized := bytes.ReplaceAll(input, []byte("\r\n"), []byte("\n"))
	normalized = bytes.TrimRight(normalized, "\n")
	lines := bytes.Split(normalized, []byte("\n"))
	var out bytes.Buffer
	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		out.Write(bytes.TrimRight(line, " \t\r"))
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}
