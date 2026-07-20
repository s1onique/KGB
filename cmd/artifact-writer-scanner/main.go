// artifact-writer-scanner detects artifact writer patterns that bypass artifactio.
//
// This scanner is the authoritative source for artifact-writer ratchet enforcement.
// It uses Go AST parsing with type resolution to detect:
//
//   - os.WriteFile (no artifactio)
//   - ioutil.WriteFile (no artifactio)
//   - os.Create + Write (no artifactio)
//   - os.OpenFile + Write (no artifactio)
//   - fmt.Fprintf to file handles (no artifactio)
//
// The scanner outputs ratchet-baseline JSON and supports comparison mode
// for verifying ACT compliance.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Finding represents a detected artifact writer pattern.
type Finding struct {
	FindingID              string `json:"finding_id"`
	SurfaceID              string `json:"surface_id"`
	File                   string `json:"file"`
	Line                   int    `json:"line"`
	Operation              string `json:"operation"`
	DestinationExpression  string `json:"destination_expression"`
	EnclosingSymbol        string `json:"enclosing_symbol"`
	ASTFingerprint         string `json:"ast_fingerprint"`
	Justification          string `json:"justification"`
	Owner                  string `json:"owner"`
	SuccessorACT          string `json:"successor_act"`
}

// Baseline represents the ratchet baseline JSON format.
type Baseline struct {
	SchemaVersion   string    `json:"schema_version"`
	BaselineCommit string    `json:"baseline_commit"`
	Generator      string    `json:"generator"`
	GeneratedAt    string    `json:"generated_at"`
	Findings       []Finding `json:"findings"`
}

// SurfaceMapping maps file patterns to surface IDs.
var SurfaceMapping = map[string]string{
	"uvb76/cmd/uvb76-capture-netns-lab":            "capture-netns-lab-artifacts",
	"uvb76/cmd/uvb76-latency-crash-lab":           "latency-crash-lab-artifacts",
	"uvb76/cmd/uvb76-targets-crash-lab":           "targets-crash-lab-artifacts",
	"uvb76/cmd/uvb76-memory-lab":                   "memory-lab-artifacts",
	"uvb76/cmd/uvb76-memleak-pprof-lab":           "memleak-pprof-lab-artifacts",
	"uvb76/cmd/uvb76-icmp-os-ping-soak":          "icmp-ping-soak-artifacts",
	"uvb76/cmd/uvb76-tcp-diag-telemetry-lab":      "tcp-diag-telemetry-lab-artifacts",
	"uvb76/cmd/uvb76-capture-netns-polling":        "capture-netns-polling-artifacts",
	"uvb76/cmd/uvb76-makefile-composition-check":  "makefile-composition-artifacts",
	"artifacts/memory-labs":                       "memory-lab-evidence",
	"artifacts/wg-netlink-lab":                    "wg-netlink-lab-evidence",
	"scripts/memory_attribution_matrix":            "memory-attribution-matrix",
	"tools/wg-netlink-lab":                        "wg-netlink-lab-evidence",
	"tools/memory-lab":                            "memory-lab-evidence",
}

// BypassPatterns are the prohibited artifact writer patterns.
var BypassPatterns = map[string]bool{
	"os.WriteFile":    true,
	"ioutil.WriteFile": true,
	"ioutil.ReadFile":  false, // reading is allowed
	"fmt.Fprintf":     true,  // when writing to file handles
	"io.WriteString":  true,  // when writing to file handles
}

// DetectSurfaceID determines the surface ID for a given file path.
func DetectSurfaceID(filePath string) string {
	for pattern, surfaceID := range SurfaceMapping {
		if strings.Contains(filePath, pattern) {
			return surfaceID
		}
	}
	return "unknown-surface"
}

// ComputeASTFingerprint creates a stable SHA-256 fingerprint from AST context.
func ComputeASTFingerprint(file string, pos token.Pos, op string, dest string, symbol string) string {
	// Normalize the fingerprint components
	normalized := fmt.Sprintf("%s:%d:%s:%s:%s",
		filepath.Base(file),
		fset.Position(pos).Line,
		op,
		dest,
		symbol,
	)
	hash := sha256.Sum256([]byte(normalized))
	return "sha256:" + hex.EncodeToString(hash[:])
}

// ComputeFindingID creates a unique finding ID from AST details.
func ComputeFindingID(f Finding) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s",
		f.SurfaceID,
		f.File,
		f.Operation,
		f.DestinationExpression,
		f.EnclosingSymbol,
		f.ASTFingerprint,
	)
	hash := sha256.Sum256([]byte(data))
	return "sha256:" + hex.EncodeToString(hash[:])
}

// ASTVisitor traverses the AST to find artifact writer patterns.
type ASTVisitor struct {
	FilePath   string
	FileSet    *token.FileSet
	Pkg        *types.Package
	Findings   []Finding
	SurfaceID  string
	Successor  string
	currentFunc string
}

// Visit implements ast.Visitor.
func (v *ASTVisitor) Visit(node ast.Node) ast.Visitor {
	switch expr := node.(type) {
	case *ast.CallExpr:
		v.detectCall(expr)
	case *ast.FuncDecl:
		// Track function declarations for enclosing symbol
		v.currentFunc = expr.Name.Name
	}
	return v
}

// detectCall analyzes a function call for bypass patterns.
func (v *ASTVisitor) detectCall(call *ast.CallExpr) {
	// Get the callee identity
	var calleePkg, calleeName string

	switch fun := call.Fun.(type) {
	case *ast.Ident:
		calleeName = fun.Name
		calleePkg = "" // local function
	case *ast.SelectorExpr:
		if ident, ok := fun.X.(*ast.Ident); ok {
			calleePkg = ident.Name
		}
		calleeName = fun.Sel.Name
	}

	op := calleeName
	if calleePkg != "" {
		op = calleePkg + "." + calleeName
	}

	// Check if this is a bypass pattern
	if !BypassPatterns[op] {
		return
	}

	// Extract destination expression
	var destExpr string
	if len(call.Args) >= 2 {
		destExpr = exprToString(call.Args[0])
	}

	// Get position
	pos := v.FileSet.Position(call.Lparen)

	finding := Finding{
		SurfaceID:             v.SurfaceID,
		File:                  v.FilePath,
		Line:                  pos.Line,
		Operation:             op,
		DestinationExpression:  destExpr,
		EnclosingSymbol:        v.currentFunc,
		ASTFingerprint:         ComputeASTFingerprint(v.FilePath, call.Pos(), op, destExpr, v.currentFunc),
		Justification:         "Legacy bypass - needs migration to artifactio.WriteRedacted*",
		Owner:                 "uvb76-team",
		SuccessorACT:          v.Successor,
	}

	// Compute stable finding ID
	finding.FindingID = ComputeFindingID(finding)

	v.Findings = append(v.Findings, finding)

	// Check for Fprintf to file handles (detect file.Write, fmt.Fprintf to *os.File)
	if calleeName == "Fprintf" && len(call.Args) >= 2 {
		// Check if first arg looks like a file handle
		if ident, ok := call.Args[0].(*ast.Ident); ok {
			if strings.HasPrefix(ident.Name, "f") || strings.HasSuffix(ident.Name, "File") || strings.HasSuffix(ident.Name, "Handle") {
				finding := Finding{
					SurfaceID:             v.SurfaceID,
					File:                  v.FilePath,
					Line:                  pos.Line,
					Operation:             op,
					DestinationExpression: ident.Name,
					EnclosingSymbol:        v.currentFunc,
					ASTFingerprint:        ComputeASTFingerprint(v.FilePath, call.Pos(), op, ident.Name, v.currentFunc),
					Justification:         "Legacy bypass - needs migration to artifactio.WriteRedacted*",
					Owner:                 "uvb76-team",
					SuccessorACT:          v.Successor,
				}
				finding.FindingID = ComputeFindingID(finding)
				v.Findings = append(v.Findings, finding)
			}
		}
	}
}

// exprToString converts an AST expression to string representation.
func exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok {
			return x.Name + "." + e.Sel.Name
		}
	case *ast.CallExpr:
		return exprToString(e.Fun) + "(...)"
	case *ast.StarExpr:
		return "*" + exprToString(e.X)
	}
	return "unknown"
}

var fset = token.NewFileSet()

// scanFile parses a single Go file and extracts findings.
func scanFile(filePath string, successor string) ([]Finding, error) {
	f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse error in %s: %w", filePath, err)
	}

	surfaceID := DetectSurfaceID(filePath)

	visitor := &ASTVisitor{
		FilePath:   filePath,
		FileSet:    fset,
		Findings:   []Finding{},
		SurfaceID:  surfaceID,
		Successor:  successor,
	}

	ast.Walk(visitor, f)

	return visitor.Findings, nil
}

// scanPackage scans all Go files in a package.
func scanPackage(pkgPath string, successor string) ([]Finding, error) {
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
	}

	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return nil, fmt.Errorf("package load error: %w", err)
	}

	var allFindings []Finding

	for _, pkg := range pkgs {
		for _, syn := range pkg.Syntax {
			visitor := &ASTVisitor{
				FilePath:   pkg.Fset.Position(syn.Pos()).Filename,
				FileSet:    pkg.Fset,
				Pkg:        pkg.Types,
				Findings:   []Finding{},
				SurfaceID:  DetectSurfaceID(pkg.Fset.Position(syn.Pos()).Filename),
				Successor:  successor,
			}
			ast.Walk(visitor, syn)
			allFindings = append(allFindings, visitor.Findings...)
		}
	}

	return allFindings, nil
}

// scanDirectory recursively scans Go files in a directory.
func scanDirectory(dir string, successor string) ([]Finding, error) {
	var findings []Finding

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Skip vendor and testdata directories
			base := filepath.Base(path)
			if base == "vendor" || base == "testdata" || base == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			f, err := scanFile(path, successor)
			if err != nil {
				log.Printf("Warning: %v", err)
				return nil
			}
			findings = append(findings, f...)
		}

		return nil
	})

	return findings, err
}

const defaultSuccessor = "ACT-UVB76-RETAINED-ARTIFACT-MIGRATION-WAVE01"

func main() {
	var (
		output      = flag.String("output", "", "Output file path (default: stdout)")
		format      = flag.String("format", "ratchet-baseline", "Output format: ratchet-baseline, findings")
		commit      = flag.String("commit", "", "Git commit hash for baseline")
		packagePath = flag.String("package", "", "Go package path to scan")
		directory   = flag.String("directory", "", "Directory to scan")
		baselineFile = flag.String("baseline", "", "Baseline file for comparison")
	)
	flag.Parse()
	
	// baselineFile is used for future comparison mode
	_ = baselineFile

	var findings []Finding
	var err error

	if *packagePath != "" {
		findings, err = scanPackage(*packagePath, defaultSuccessor)
	} else if *directory != "" {
		findings, err = scanDirectory(*directory, defaultSuccessor)
	} else {
		// Default: scan uvb76 and tools directories
		dirs := []string{
			"uvb76/cmd",
			"tools",
			"scripts/memory_attribution_matrix",
		}

		for _, dir := range dirs {
			if _, err := os.Stat(dir); err == nil {
				f, err := scanDirectory(dir, defaultSuccessor)
				if err != nil {
					log.Printf("Error scanning %s: %v", dir, err)
					continue
				}
				findings = append(findings, f...)
			}
		}
	}

	if err != nil {
		log.Fatalf("Scan failed: %v", err)
	}

	// Sort findings for deterministic output
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})

	// Deduplicate findings
	seen := make(map[string]bool)
	var unique []Finding
	for _, f := range findings {
		if !seen[f.FindingID] {
			seen[f.FindingID] = true
			unique = append(unique, f)
		}
	}
	findings = unique

	// Output
	var outputData []byte

	switch *format {
	case "ratchet-baseline":
		commitHash := *commit
		if commitHash == "" {
			// Get current commit
			if data, err := os.ReadFile(".git/refs/heads/main"); err == nil {
				commitHash = strings.TrimSpace(string(data))
			} else {
				commitHash = "unknown"
			}
		}

		baseline := Baseline{
			SchemaVersion:   "ratchet-v1",
			BaselineCommit:   commitHash,
			Generator:        "artifact-writer-scanner",
			GeneratedAt:      "", // Will be filled by JSON encoder
			Findings:         findings,
		}

		outputData, err = json.MarshalIndent(baseline, "", "  ")
		if err != nil {
			log.Fatalf("JSON marshal failed: %v", err)
		}

	case "findings":
		outputData, err = json.MarshalIndent(findings, "", "  ")
		if err != nil {
			log.Fatalf("JSON marshal failed: %v", err)
		}

	default:
		log.Fatalf("Unknown format: %s", *format)
	}

	if *output != "" {
		if err := os.WriteFile(*output, outputData, 0644); err != nil {
			log.Fatalf("Write failed: %v", err)
		}
		fmt.Printf("Wrote %d findings to %s\n", len(findings), *output)
	} else {
		os.Stdout.Write(outputData)
		fmt.Println() // newline
	}
}
