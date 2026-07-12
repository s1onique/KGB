package producer

// ACT-UVB76-HULK05R4R1: contractcatalog.go is now a thin delegate over the
// canonical JSON catalog. The previous handwritten mirror was eliminated:
//
//   scripts/uvb76_artifact_secret_hygiene/surfaces.json
//       => Python projection (inventory.py)
//       => Go projections (this file)
//
// There is exactly one editable source. The legacy DefaultContracts and
// DefaultInventorySurfaces variables are kept as exported symbols for
// compatibility with the bypass detector and existing tests, but they are
// produced by the canonical loader using the canonical JSON file located at
// CanonicalCatalogPath relative to the resolved repository root.
//
// The DefaultAllowlistedWriterFiles list remains a hand-maintained list of
// artifactio implementation files (file paths, not directory globs).

// catalogRepoRoot is the repository root used by the package-level
// delegate. It is initialized once at process start via loadDefaultCatalog.
//
// Tests that need a specific catalog location should use
// LoadCanonicalCatalogFrom or LoadCanonicalCatalog instead.
var (
	catalogRepoRoot string
	catalogInitOnce bool
)

// resolveCatalogRepoRoot returns the repository root the package-level
// delegate uses. It looks for the canonical catalog starting from the
// current working directory and walking up.
//
// The lookup is best-effort and falls back to "" when the canonical catalog
// cannot be located. Callers that need a strict gate must call
// LoadCanonicalCatalog(reproot) explicitly.
func resolveCatalogRepoRoot() string {
	if catalogRepoRoot != "" {
		return catalogRepoRoot
	}
	dir, err := osGetwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 6; i++ {
		candidate := joinRepoPath(dir, CanonicalCatalogPath)
		if _, err := defaultReadFile(candidate); err == nil {
			catalogRepoRoot = dir
			return dir
		}
		parent := dirParent(dir)
		if parent == "" || parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func osGetwd() (string, error) {
	return os_Getwd()
}

func dirParent(dir string) string {
	for i := len(dir) - 1; i >= 0; i-- {
		if dir[i] == '/' {
			return dir[:i]
		}
	}
	return ""
}

// DefaultInventorySurfaces is the package-level surface projection.
var DefaultInventorySurfaces []SurfaceRecord

// DefaultContracts is the package-level contract projection.
var DefaultContracts []*ProducerContract

// DefaultAllowlistedWriterFiles is the allowlist of writer files that ARE
// the artifactio implementation itself.
var DefaultAllowlistedWriterFiles = []string{
	"uvb76/internal/artifactio/atomic.go",
	"uvb76/internal/artifactio/json.go",
	"uvb76/internal/artifactio/text.go",
	"uvb76/internal/artifactio/headers_text.go",
	"uvb76/internal/artifactio/errors.go",
	"uvb76/internal/artifactio/policy.go",
	"uvb76/internal/producer/bypass_ast.go",
	"uvb76/internal/producer/catalog_validate.go",
	"uvb76/internal/producer/surfacecatalog.go",
	"uvb76/internal/producer/surfacecatalog_parse.go",
}

// DefaultInit loads the canonical catalog into the package-level variables.
// Called by uvb76-artifact-writer-verify on startup.
func DefaultInit() error {
	catalogRepoRoot = resolveCatalogRepoRoot()
	cat, err := LoadCanonicalCatalog(catalogRepoRoot)
	if err != nil {
		return err
	}
	contracts, surfaces := cat.ProjectContracts()
	DefaultContracts = contracts
	DefaultInventorySurfaces = surfaces
	catalogInitOnce = true
	return nil
}

func init() {
	// Best-effort bootstrap. Tests that need a working default catalog
	// call DefaultInit() explicitly.
	_ = DefaultInit()
}
