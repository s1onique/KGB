package evidence

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestEvidenceCapture_NoTrailingWhitespace(t *testing.T) {
	got, err := NormalizeCapturedEvidence([]byte("path\t\n\tmod example v1\t \n"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte("\t\n")) || bytes.Contains(got, []byte(" \n")) {
		t.Fatalf("trailing whitespace remains: %q", got)
	}
}
func TestEvidenceCapture_ExactlyOneFinalNewline(t *testing.T) {
	got, err := NormalizeCapturedEvidence([]byte("value\n\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("value\n")) {
		t.Fatalf("got=%q", got)
	}
}
func TestEvidenceCapture_PreservesBuildInfoSemantics(t *testing.T) {
	got, err := NormalizeCapturedEvidence([]byte("\tbuild\t-buildmode=exe\t\n\tbuild\tvcs.modified=false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("\tbuild\t-buildmode=exe")) || !bytes.Contains(got, []byte("vcs.modified=false")) {
		t.Fatalf("semantics changed: %q", got)
	}
}
func TestEvidenceCapture_DoesNotModifyBinaryHash(t *testing.T) {
	input := []byte{0, 1, 2, 3, 4}
	before := sha256.Sum256(input)
	_, _ = NormalizeCapturedEvidence(input)
	after := sha256.Sum256(input)
	if before != after {
		t.Fatal("input bytes were mutated")
	}
}
