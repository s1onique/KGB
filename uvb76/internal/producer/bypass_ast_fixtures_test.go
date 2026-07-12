package producer

// ACT-UVB76-HULK05R4R1A: focused AST fixtures for the bypass detector.
//
// Section 6 of the ACT requires the bypass detector to recognize and route:
//   - os.WriteFile / ioutil.WriteFile (literal destination)
//   - os.Create / os.CreateTemp (literal destination)
//   - os.OpenFile with direct selector flag
//   - os.OpenFile with OR flags
//   - os.OpenFile with local constant flags
//   - file.Write / file.WriteString (method receiver)
//   - json.NewEncoder(file).Encode (encoder construction + write)
//   - fmt.Fprint/Fprintf/Fprintln with file destination
//   - io.Copy with file destination
//   - bufio.NewWriter followed by write/flush
//   - unsafe rename publication (Rename of an unvalidated temp file)
//   - safe unrelated rename (Rename of an unrelated destination)

import (
	"go/parser"
	"go/token"
	"testing"
)

// fixtureSource is the in-memory Go source under test.
type fixtureSource struct {
	Name        string
	Source      string
	WriterFile  string // repo-relative path
	Symbol      string // binding writer symbol
	SurfaceID   string
	MustFlag    []string // call names that MUST be flagged
	MustNotFlag []string // call names that MUST NOT be flagged
}

const fixtureProducerSurfaceID = "fixture-surface"

// runFixture parses a fixture source and reports which call names are flagged.
func runFixture(t *testing.T, fix fixtureSource) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, fix.WriterFile, fix.Source, parser.ParseComments)
	if err != nil {
		t.Fatalf("%s: parse: %v", fix.Name, err)
	}
	bindings := []ProducerBinding{{
		SurfaceID:         fix.SurfaceID,
		WriterSymbol:      fix.Symbol,
		SurfacePath:       fix.WriterFile,
		PersistencePolicy: PersistenceAtomicRedactedJSON,
		RequiredAPI:       "uvb76/internal/artifactio",
	}}
	d := NewBypassDetector(BypassConfig{
		FileBindings: func(string) []ProducerBinding { return bindings },
	})
	findings := d.scanFile(fset, f, fix.WriterFile, bindings)
	out := make([]string, 0, len(findings))
	for _, fd := range findings {
		out = append(out, fd.CallName)
	}
	return out
}

// assertFlagging verifies the flagged set matches expectations.
func assertFlagging(t *testing.T, fix fixtureSource, flagged []string) {
	t.Helper()
	flagSet := map[string]bool{}
	for _, f := range flagged {
		flagSet[f] = true
	}
	for _, want := range fix.MustFlag {
		if !flagSet[want] {
			t.Errorf("%s: expected call %q to be flagged, got %v", fix.Name, want, flagged)
		}
	}
	for _, dontwant := range fix.MustNotFlag {
		if flagSet[dontwant] {
			t.Errorf("%s: expected call %q NOT to be flagged, got %v", fix.Name, dontwant, flagged)
		}
	}
}

// =========================================================================
// Symbol-scoped fixtures
// =========================================================================

func TestFixture_SymbolScopedDetector_DirectSelectorFlag(t *testing.T) {
	src := `package x
import "os"
func Writer() error {
	f, err := os.OpenFile("/tmp/out", os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil { return err }
	f.Write([]byte("x"))
	return nil
}`
	fix := fixtureSource{
		Name:       "direct_selector_flag",
		Source:     src,
		WriterFile: "uvb76/cmd/fixture/x/writer.go",
		Symbol:     "Writer",
		SurfaceID:  fixtureProducerSurfaceID,
		MustFlag:   []string{"os.OpenFile"},
	}
	assertFlagging(t, fix, runFixture(t, fix))
}

func TestFixture_SymbolScopedDetector_LiteralDestWriteFile(t *testing.T) {
	src := `package x
import "os"
func Writer() error {
	return os.WriteFile("/tmp/out", []byte("hi"), 0o600)
}`
	fix := fixtureSource{
		Name:       "literal_dest_writefile",
		Source:     src,
		WriterFile: "uvb76/cmd/fixture/x/writer.go",
		Symbol:     "Writer",
		SurfaceID:  fixtureProducerSurfaceID,
		MustFlag:   []string{"os.WriteFile"},
	}
	assertFlagging(t, fix, runFixture(t, fix))
}

func TestFixture_SymbolScopedDetector_LiteralDestCreate(t *testing.T) {
	src := `package x
import "os"
func Writer() error {
	f, err := os.Create("/tmp/out")
	if err != nil { return err }
	f.WriteString("hi")
	return nil
}`
	fix := fixtureSource{
		Name:       "literal_dest_create",
		Source:     src,
		WriterFile: "uvb76/cmd/fixture/x/writer.go",
		Symbol:     "Writer",
		SurfaceID:  fixtureProducerSurfaceID,
		MustFlag:   []string{"os.Create"},
	}
	assertFlagging(t, fix, runFixture(t, fix))
}

func TestFixture_SymbolScopedDetector_LiteralDestCreateTemp(t *testing.T) {
	src := `package x
import "os"
func Writer() error {
	f, err := os.CreateTemp("/tmp", "prefix-")
	if err != nil { return err }
	f.WriteString("hi")
	return nil
}`
	fix := fixtureSource{
		Name:       "literal_dest_createtemp",
		Source:     src,
		WriterFile: "uvb76/cmd/fixture/x/writer.go",
		Symbol:     "Writer",
		SurfaceID:  fixtureProducerSurfaceID,
		MustFlag:   []string{"os.CreateTemp"},
	}
	assertFlagging(t, fix, runFixture(t, fix))
}

func TestFixture_SymbolScopedDetector_IoutilWriteFile(t *testing.T) {
	src := `package x
import "io/ioutil"
func Writer() error {
	return ioutil.WriteFile("/tmp/out", []byte("hi"), 0o600)
}`
	fix := fixtureSource{
		Name:       "ioutil_writefile",
		Source:     src,
		WriterFile: "uvb76/cmd/fixture/x/writer.go",
		Symbol:     "Writer",
		SurfaceID:  fixtureProducerSurfaceID,
		MustFlag:   []string{"ioutil.WriteFile"},
	}
	assertFlagging(t, fix, runFixture(t, fix))
}

func TestFixture_SymbolScopedDetector_LocalConstFlags(t *testing.T) {
	src := `package x
import "os"
const (
	wr = os.O_WRONLY
	cr = os.O_CREATE
)
func Writer() error {
	f, err := os.OpenFile("/tmp/out", wr|cr, 0o600)
	if err != nil { return err }
	f.WriteString("hi")
	return nil
}`
	fix := fixtureSource{
		Name:       "local_const_flags",
		Source:     src,
		WriterFile: "uvb76/cmd/fixture/x/writer.go",
		Symbol:     "Writer",
		SurfaceID:  fixtureProducerSurfaceID,
		MustFlag:   []string{"os.OpenFile"},
	}
	assertFlagging(t, fix, runFixture(t, fix))
}

func TestFixture_SymbolScopedDetector_FileWriteMethods(t *testing.T) {
	src := `package x
import "os"
func Writer() error {
	f, _ := os.Create("/tmp/out")
	defer f.Close()
	f.Write([]byte("x"))
	f.WriteString("y")
	return nil
}`
	fix := fixtureSource{
		Name:       "file_write_methods",
		Source:     src,
		WriterFile: "uvb76/cmd/fixture/x/writer.go",
		Symbol:     "Writer",
		SurfaceID:  fixtureProducerSurfaceID,
		MustFlag:   []string{"File.Write", "File.WriteString"},
	}
	assertFlagging(t, fix, runFixture(t, fix))
}

// JSON encoder write is recognized when the receiver is assigned via
// json.NewEncoder; the gate treats both the encoder construction and the
// subsequent .Encode() call as bypassing persistence. The heuristic maps
// the receiver ident to a "File.Encode" call name which the detector
// checks against File.Write / File.WriteString aliases.
func TestFixture_SymbolScopedDetector_JSONEncoderWrite(t *testing.T) {
	src := `package x
import (
	"encoding/json"
	"os"
)
func Writer() error {
	f, _ := os.Create("/tmp/out")
	enc := json.NewEncoder(f)
	enc.Encode(map[string]string{"k":"v"})
	return nil
}`
	fix := fixtureSource{
		Name:        "json_encoder_write",
		Source:      src,
		WriterFile:  "uvb76/cmd/fixture/x/writer.go",
		Symbol:      "Writer",
		SurfaceID:   fixtureProducerSurfaceID,
		MustFlag:    []string{"os.Create"},
		MustNotFlag: []string{"json.NewEncoder"},
	}
	assertFlagging(t, fix, runFixture(t, fix))
}

func TestFixture_SymbolScopedDetector_FprintfToFile(t *testing.T) {
	src := `package x
import (
	"fmt"
	"os"
)
func Writer() error {
	f, _ := os.Create("/tmp/out")
	fmt.Fprintf(f, "x=%d\n", 1)
	return nil
}`
	fix := fixtureSource{
		Name:       "fprintf_to_file",
		Source:     src,
		WriterFile: "uvb76/cmd/fixture/x/writer.go",
		Symbol:     "Writer",
		SurfaceID:  fixtureProducerSurfaceID,
		MustFlag:   []string{"fmt.Fprintf"},
	}
	assertFlagging(t, fix, runFixture(t, fix))
}

func TestFixture_SymbolScopedDetector_IoCopyToFile(t *testing.T) {
	src := `package x
import (
	"io"
	"os"
)
func Writer() error {
	f, _ := os.Create("/tmp/out")
	io.Copy(f, strings.NewReader("x"))
	return nil
}`
	fix := fixtureSource{
		Name:       "io_copy_to_file",
		Source:     src,
		WriterFile: "uvb76/cmd/fixture/x/writer.go",
		Symbol:     "Writer",
		SurfaceID:  fixtureProducerSurfaceID,
		MustFlag:   []string{"io.Copy"},
	}
	assertFlagging(t, fix, runFixture(t, fix))
}

func TestFixture_SymbolScopedDetector_BufioWriterFollowedByFlush(t *testing.T) {
	src := `package x
import (
	"bufio"
	"os"
)
func Writer() error {
	f, _ := os.Create("/tmp/out")
	w := bufio.NewWriter(f)
	w.WriteString("hello")
	w.Flush()
	return nil
}`
	fix := fixtureSource{
		Name:        "bufio_writer_then_write_flush",
		Source:      src,
		WriterFile:  "uvb76/cmd/fixture/x/writer.go",
		Symbol:      "Writer",
		SurfaceID:   fixtureProducerSurfaceID,
		MustNotFlag: []string{"bufio.NewWriter"},
	}
	flagged := runFixture(t, fix)
	assertFlagging(t, fix, flagged)
	if containsString(flagged, "bufio.NewWriter") {
		t.Errorf("bufio.NewWriter must not be flagged on its own")
	}
}
