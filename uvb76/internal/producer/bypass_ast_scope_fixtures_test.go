package producer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestFixture_SymbolScopedDetector_UnsafeRenamePublication(t *testing.T) {
	src := `package x
import "os"
func UnsafePublish() error {
	return os.Rename("/tmp/unsafe-tmp", "/var/lib/out")
}`
	fix := fixtureSource{
		Name:       "unsafe_rename_publication",
		Source:     src,
		WriterFile: "uvb76/cmd/fixture/x/writer.go",
		Symbol:     "UnsafePublish",
		SurfaceID:  fixtureProducerSurfaceID,
		MustFlag:   []string{"os.Rename"},
	}
	assertFlagging(t, fix, runFixture(t, fix))
}

func TestFixture_SymbolScopedDetector_SafeUnrelatedRename(t *testing.T) {
	src := `package x
import "os"
func Renamer() error {
	return os.Rename("/var/lib/in", "/var/lib/processed")
}`
	fix := fixtureSource{
		Name:       "safe_unrelated_rename",
		Source:     src,
		WriterFile: "uvb76/cmd/fixture/x/writer.go",
		Symbol:     "Renamer",
		SurfaceID:  fixtureProducerSurfaceID,
		MustFlag:   []string{"os.Rename"},
	}
	assertFlagging(t, fix, runFixture(t, fix))
}

// =========================================================================
// Shared-file symbol scoping
// =========================================================================

// A shared file contains two writers; each bypassing call is tagged with
// the surface of its own writer symbol, not the file's union.
func TestFixture_SymbolScopedDetector_SharedFile_SymbolIsolated(t *testing.T) {
	src := `package x
import "os"
func Writer() error {
	return os.WriteFile("/tmp/primary", []byte("a"), 0o600)
}
func helper() error {
	return os.WriteFile("/tmp/aux", []byte("b"), 0o600)
}`
	bindings := []ProducerBinding{
		{
			SurfaceID:         "shared-primary",
			WriterSymbol:      "Writer",
			SurfacePath:       "uvb76/cmd/fixture/shared/writer.go",
			PersistencePolicy: PersistenceAtomicRedactedJSON,
			RequiredAPI:       "uvb76/internal/artifactio",
		},
		{
			SurfaceID:         "shared-aux",
			WriterSymbol:      "helper",
			SurfacePath:       "uvb76/cmd/fixture/shared/writer.go",
			PersistencePolicy: PersistenceAtomicRedactedJSON,
			RequiredAPI:       "uvb76/internal/artifactio",
		},
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "uvb76/cmd/fixture/shared/writer.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	d := NewBypassDetector(BypassConfig{
		FileBindings: func(string) []ProducerBinding { return bindings },
	})
	findings := d.scanFile(fset, f, "uvb76/cmd/fixture/shared/writer.go", bindings)
	t.Logf("DEBUG: got %d findings, bindings=%d", len(findings), len(bindings))
	if len(findings) != 2 {
		t.Fatalf("expected exactly 2 findings, got %d", len(findings))
	}
	// Each finding must be tagged with the matching writer symbol.
	syms := map[string]bool{}
	for _, fd := range findings {
		if len(fd.SurfaceIDs) != 1 {
			t.Errorf("expected 1 surface per finding, got %v", fd.SurfaceIDs)
		}
		syms[fd.WriterSymbol] = true
	}
	if !syms["Writer"] {
		t.Errorf("missing finding tagged with Writer")
	}
	if !syms["helper"] {
		t.Errorf("missing finding tagged with helper")
	}
}

// File-scope calls (no enclosing function) cannot be attributed to any
// binding, so the detector emits no surface-tagged findings. The detector
// does NOT pretend to suppress these calls; it just refrains from
// attributing them. This is the same behavior the gate relies on: a
// file-scope os.WriteFile produces a finding with zero surfaces, which the
// gate's downstream filter would drop.
func TestFixture_SymbolScopedDetector_FileScopeUnattributable(t *testing.T) {
	src := `package x
import "os"
var _ = os.WriteFile("/tmp/x", []byte("a"), 0o600)
func Writer() error { return nil }`
	bindings := []ProducerBinding{{
		SurfaceID:         "shared-scope",
		WriterSymbol:      "Writer",
		SurfacePath:       "uvb76/cmd/fixture/x/file_scope.go",
		PersistencePolicy: PersistenceAtomicRedactedJSON,
		RequiredAPI:       "uvb76/internal/artifactio",
	}}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "uvb76/cmd/fixture/x/file_scope.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	d := NewBypassDetector(BypassConfig{
		FileBindings: func(string) []ProducerBinding { return bindings },
	})
	findings := d.scanFile(fset, f, "uvb76/cmd/fixture/x/file_scope.go", bindings)
	// The file-scope call MUST NOT be misattributed. The detector
	// either drops the call (zero findings) or attributes it to a
	// binding whose surface is the same as the explicit one.
	// Anything else (e.g. attributing it to an unrelated surface)
	// is a regression.
	if len(findings) > 1 {
		t.Errorf("file-scope call produced %d findings; expected 0 or 1", len(findings))
	}
	for _, fd := range findings {
		for _, sid := range fd.SurfaceIDs {
			if sid != "shared-scope" {
				t.Errorf("file-scope call attributed to unexpected surface %q (surfaces=%v)", sid, fd.SurfaceIDs)
			}
		}
	}
}

// Constructor-only calls must be suppressed even when the enclosing
// function matches the writer symbol.
func TestFixture_SymbolScopedDetector_ConstructorOnlySuppressed(t *testing.T) {
	src := `package x
import (
	"bufio"
	"os"
)
func Writer() error {
	f, _ := os.Create("/tmp/x")
	w := bufio.NewWriter(f)
	w.WriteString("hello")
	return nil
}`
	bindings := []ProducerBinding{{
		SurfaceID:         "constructor-only",
		WriterSymbol:      "Writer",
		SurfacePath:       "uvb76/cmd/fixture/x/ctor.go",
		PersistencePolicy: PersistenceAtomicRedactedJSON,
		RequiredAPI:       "uvb76/internal/artifactio",
	}}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "uvb76/cmd/fixture/x/ctor.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	d := NewBypassDetector(BypassConfig{
		FileBindings: func(string) []ProducerBinding { return bindings },
	})
	findings := d.scanFile(fset, f, "uvb76/cmd/fixture/x/ctor.go", bindings)
	for _, fd := range findings {
		if fd.CallName == "bufio.NewWriter" {
			t.Errorf("constructor-only call must not be flagged")
		}
	}
}

// isFileFullyDedicated returns true when every binding for the file shares
// the same writer symbol.
func TestIsFileFullyDedicated(t *testing.T) {
	if !isFileFullyDedicated([]ProducerBinding{{WriterSymbol: "Writer"}, {WriterSymbol: "Writer"}}) {
		t.Error("expected fully dedicated")
	}
	if isFileFullyDedicated([]ProducerBinding{{WriterSymbol: "Writer"}, {WriterSymbol: "helper"}}) {
		t.Error("shared file must not be fully dedicated")
	}
	if isFileFullyDedicated(nil) {
		t.Error("empty bindings must not be fully dedicated")
	}
}

// funcDeclName handles receiver-qualified method names.
func TestFuncDeclName(t *testing.T) {
	src := `package x
func (s *Server) Write() {}
func Write() {}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	names := map[string]bool{}
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		names[funcDeclName(fd)] = true
	}
	if !names["Server.Write"] {
		t.Errorf("expected receiver-qualified method name 'Server.Write', got %v", names)
	}
	if !names["Write"] {
		t.Errorf("expected plain function name 'Write', got %v", names)
	}
}

// containsString is a tiny helper to avoid importing slices just for one call.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
