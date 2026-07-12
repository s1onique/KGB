package producer

// ACT-UVB76-HULK05R4R1: Bypass detection rewrite.
//
// The previous bypass detector mapped file -> one surface. This rewritten
// version:
//
//  1. Computes file -> set of producer bindings. A binding is the
//     (writer_symbol, surface_id, surface_path, persistence_policy,
//     required_api) tuple. A file shared by multiple surfaces preserves
//     all surface IDs.
//
//  2. Deduplicates files before parsing. The input list may contain
//     duplicates from various walks; we collapse them via a single
//     normalized key.
//
//  3. Detects destination paths via four mechanisms:
//     literal destination strings,
//     filepath.Join with literal root segments,
//     variables derived from registered output directories,
//     function parameters explicitly designated as destination paths.
//
//  4. Expands write-operation coverage to:
//     os.WriteFile, ioutil.WriteFile,
//     os.Create, os.CreateTemp,
//     os.OpenFile with any write-capable flag (incl. constants),
//     io.Copy where destination is a writable file,
//     json.NewEncoder(file).Encode,
//     fmt.Fprint/Fprintf/Fprintln to a writable file,
//     bufio.NewWriter(file) followed by writes,
//     os.Rename of an unvalidated temp file (publication of an
//     unvalidated tempfile).
//
// Symbol-scoped emission:
//   - During AST traversal the detector tracks the enclosing *ast.FuncDecl.
//   - For shared files, a finding is emitted only when the enclosing
//     symbol matches a binding's WriterSymbol, or when the contract
//     declares the whole file as dedicated.
//   - Symbols in the bypass_ast.go allowlist still suppress findings.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
)

// BypassFinding is a single bypassing persistence call detected in a file.
type BypassFinding struct {
	File              string
	Line              int
	Column            int
	CallName          string
	SurfaceIDs        []string // all applicable surface IDs (preserve all)
	WriterSymbol      string
	RequiredAPI       string
	Suggestion        string
	Destination       string // resolved destination, when statically known
	DestinationReason string // how the destination was resolved
}

// visitor is a tiny ast.Visitor adapter that lets enclosingFuncDecl
// receive every node during an ast.Walk traversal.
type visitor func(node ast.Node)

func (v visitor) Visit(node ast.Node) ast.Visitor {
	v(node)
	return v
}

// BypassConfig configures the bypass detector.
type BypassConfig struct {
	AllowlistedFiles []string
	// FileBindings returns the surface bindings for a file. A file may map to
	// multiple surfaces; the detector preserves all.
	FileBindings func(file string) []ProducerBinding
	RequiredAPI  string
	RepoRoot     string
}

// ProducerBinding is the closed descriptor returned by FileBindings.
//
// A binding identifies ONE writer symbol and ONE surface together with the
// persistence policy and the destination surface path. Files shared by
// multiple writers produce one binding per (file, surface, symbol) triple,
// not one binding that collapses all symbols.
type ProducerBinding struct {
	SurfaceID         string
	WriterSymbol      string
	SurfacePath       string
	PersistencePolicy PersistencePolicy
	RequiredAPI       string
}

// BypassDetector flags direct persistence calls that bypass the central boundary.
type BypassDetector struct {
	cfg BypassConfig
}

// NewBypassDetector constructs a detector.
func NewBypassDetector(cfg BypassConfig) *BypassDetector {
	if cfg.RequiredAPI == "" {
		cfg.RequiredAPI = "uvb76/internal/artifactio"
	}
	if cfg.FileBindings == nil {
		cfg.FileBindings = FileBindingsFromContracts(DefaultContracts)
	}
	return &BypassDetector{cfg: cfg}
}

// Scanner scans every file once. Files are deduplicated by normalized path.
func (d *BypassDetector) Scanner(files []string) ([]BypassFinding, error) {
	var findings []BypassFinding
	allowSet := make(map[string]bool)
	for _, f := range d.cfg.AllowlistedFiles {
		allowSet[NormalizedPath(f)] = true
	}
	seen := make(map[string]bool)
	dedup := make([]string, 0, len(files))
	for _, f := range files {
		rel := d.relativePath(f)
		if rel == "" || seen[rel] {
			continue
		}
		seen[rel] = true
		dedup = append(dedup, rel)
	}
	for _, rel := range dedup {
		if allowSet[rel] {
			continue
		}
		bindings := d.cfg.FileBindings(rel)
		if len(bindings) == 0 {
			continue
		}
		abs := filepath.Join(d.cfg.RepoRoot, filepath.FromSlash(rel))
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, abs, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", abs, err)
		}
		fileFindings := d.scanFile(fset, parsed, rel, bindings)
		findings = append(findings, fileFindings...)
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}

func (d *BypassDetector) relativePath(file string) string {
	clean := filepath.Clean(file)
	clean = strings.ReplaceAll(clean, "\\", "/")
	if !strings.HasPrefix(clean, "/") {
		return NormalizedPath(clean)
	}
	if d.cfg.RepoRoot == "" {
		return NormalizedPath(clean)
	}
	root := filepath.Clean(d.cfg.RepoRoot)
	root = strings.ReplaceAll(root, "\\", "/")
	if strings.HasPrefix(clean, root+"/") {
		return NormalizedPath(clean[len(root)+1:])
	}
	return NormalizedPath(clean)
}
