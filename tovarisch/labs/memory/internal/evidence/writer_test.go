package evidence

import (
	"strings"
	"testing"
)

// TestParseChecksumLine_Valid covers the canonical happy path: a
// lowercase-hex 64-character hash plus a flat canonical artifact
// filename. Also covers every one of the nine canonical bounded
// ACT artifact names.
func TestParseChecksumLine_Valid(t *testing.T) {
	canonical := []string{
		"manifest.json",
		"verdict.json",
		"samples.csv",
		"events.jsonl",
		"container-inspect.json",
		"container-logs.txt",
		"initial-canary-state.json",
		"final-canary-state.json",
		"workload-result.json",
	}
	for _, name := range canonical {
		hash := strings.Repeat("a", 64)
		line := hash + "  " + name
		gotHash, gotPath, err := ParseChecksumLine(line)
		if err != nil {
			t.Errorf("canonical %q rejected: %v", name, err)
			continue
		}
		if gotHash != hash || gotPath != name {
			t.Errorf("canonical %q returned hash=%q path=%q", name, gotHash, gotPath)
		}
	}
}

func TestParseChecksumLine_HashLength63(t *testing.T) {
	line := strings.Repeat("a", 63) + "  manifest.json"
	_, _, err := ParseChecksumLine(line)
	if err == nil || !strings.Contains(err.Error(), "checksum hash length:") {
		t.Errorf("63 chars should fail with hash length diagnostic, got: %v", err)
	}
}

func TestParseChecksumLine_HashLength65(t *testing.T) {
	line := strings.Repeat("a", 65) + "  manifest.json"
	_, _, err := ParseChecksumLine(line)
	if err == nil || !strings.Contains(err.Error(), "checksum hash length:") {
		t.Errorf("65 chars should fail with hash length diagnostic, got: %v", err)
	}
}

func TestParseChecksumLine_HashNonHex(t *testing.T) {
	line := strings.Repeat("z", 64) + "  manifest.json"
	_, _, err := ParseChecksumLine(line)
	if err == nil || !strings.Contains(err.Error(), "invalid checksum hash encoding:") {
		t.Errorf("non-hex should fail with hash encoding diagnostic, got: %v", err)
	}
}

func TestParseChecksumLine_HashOneNonHexChar(t *testing.T) {
	// 63 valid hex chars + 1 non-hex char
	hash := strings.Repeat("a", 63) + "z"
	line := hash + "  manifest.json"
	_, _, err := ParseChecksumLine(line)
	if err == nil || !strings.Contains(err.Error(), "invalid checksum hash encoding:") {
		t.Errorf("63-hex + 1-nonhex (64 chars) should fail with hash encoding diagnostic, got: %v", err)
	}
}

func TestParseChecksumLine_HashUppercase(t *testing.T) {
	upper := strings.ToUpper(strings.Repeat("a", 64))
	line := upper + "  manifest.json"
	_, _, err := ParseChecksumLine(line)
	if err == nil || !strings.Contains(err.Error(), "non-canonical checksum hash:") {
		t.Errorf("uppercase should fail with case diagnostic, got: %v", err)
	}
}

func TestParseChecksumLine_HashMissing(t *testing.T) {
	line := "  manifest.json"
	_, _, err := ParseChecksumLine(line)
	if err == nil || !strings.Contains(err.Error(), "malformed delimiter") {
		t.Errorf("missing hash should fail with malformed-delimiter diagnostic, got: %v", err)
	}
}

func TestParseChecksumLine_MalformedDelimiter(t *testing.T) {
	// single space instead of double space
	line := strings.Repeat("a", 64) + " manifest.json"
	_, _, err := ParseChecksumLine(line)
	if err == nil || !strings.Contains(err.Error(), "malformed delimiter") {
		t.Errorf("single space delimiter should fail with malformed-delimiter diagnostic, got: %v", err)
	}
}

func TestValidateChecksumArtifactPath_DotEscape(t *testing.T) {
	err := ValidateChecksumArtifactPath("../escape.json")
	if err == nil || !strings.Contains(err.Error(), `invalid checksum artifact path: "../escape.json"`) {
		t.Errorf("../escape.json should fail with traversal diagnostic, got: %v", err)
	}
}

func TestValidateChecksumArtifactPath_NestedEscape(t *testing.T) {
	err := ValidateChecksumArtifactPath("a/../../escape.json")
	if err == nil || !strings.Contains(err.Error(), `invalid checksum artifact path:`) {
		t.Errorf("nested escape should fail, got: %v", err)
	}
}

func TestValidateChecksumArtifactPath_AbsolutePath(t *testing.T) {
	err := ValidateChecksumArtifactPath("/tmp/escape.json")
	if err == nil || !strings.Contains(err.Error(), `invalid checksum artifact path: "/tmp/escape.json"`) {
		t.Errorf("absolute path should fail, got: %v", err)
	}
}

func TestValidateChecksumArtifactPath_NestedPath(t *testing.T) {
	err := ValidateChecksumArtifactPath("subdir/manifest.json")
	if err == nil || !strings.Contains(err.Error(), `invalid checksum artifact path: "subdir/manifest.json"`) {
		t.Errorf("nested path should fail, got: %v", err)
	}
}

func TestValidateChecksumArtifactPath_WindowsSeparator(t *testing.T) {
	err := ValidateChecksumArtifactPath(`..\escape.json`)
	if err == nil || !strings.Contains(err.Error(), `invalid checksum artifact path: `) {
		t.Errorf("Windows separator should fail, got: %v", err)
	}
}

func TestValidateChecksumArtifactPath_BackslashNested(t *testing.T) {
	err := ValidateChecksumArtifactPath(`subdir\manifest.json`)
	if err == nil || !strings.Contains(err.Error(), `invalid checksum artifact path: `) {
		t.Errorf("backslash-nested should fail, got: %v", err)
	}
}

func TestValidateChecksumArtifactPath_Dot(t *testing.T) {
	err := ValidateChecksumArtifactPath(".")
	if err == nil || !strings.Contains(err.Error(), `invalid checksum artifact path: "."`) {
		t.Errorf("dot path should fail, got: %v", err)
	}
}

func TestValidateChecksumArtifactPath_Empty(t *testing.T) {
	err := ValidateChecksumArtifactPath("")
	if err == nil || !strings.Contains(err.Error(), `invalid checksum artifact path: ""`) {
		t.Errorf("empty path should fail, got: %v", err)
	}
}

func TestParseChecksumsFile_DuplicatePath(t *testing.T) {
	line1 := strings.Repeat("a", 64) + "  manifest.json"
	line2 := strings.Repeat("b", 64) + "  manifest.json"
	data := line1 + "\n" + line2 + "\n"
	_, err := ParseChecksumsFile(data)
	if err == nil || !strings.Contains(err.Error(), "duplicate entry for: manifest.json") {
		t.Errorf("duplicate path should fail with duplicate diagnostic, got: %v", err)
	}
}
