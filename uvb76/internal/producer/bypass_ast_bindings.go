package producer

import (
	"os"
	"path/filepath"
)

// FileBindingsFromContracts builds a file -> []binding index from contracts.
//
// Each (file, writer_symbol, surface_id) triple becomes one binding entry.
// Files shared by multiple surfaces preserve all bindings instead of being
// collapsed into a single (file, surface) map entry.
//
// For legacy_bypass surfaces (no extracted writer symbols yet), the entire
// file is treated as dedicated to its surface so that any raw os.* write
// in the file produces a finding.
func FileBindingsFromContracts(contracts []*ProducerContract) func(string) []ProducerBinding {
	type key struct{ file, surface, symbol string }
	idx := map[key]ProducerBinding{}
	// For each writer_file of a legacy_bypass surface, add a single
	// file-scope binding that matches any enclosing function. The detector
	// uses it to attribute raw os.* calls to the legacy surface.
	legacyDedicated := map[string][]ProducerBinding{} // file -> bindings
	for _, c := range contracts {
		if c.Status != StatusActive {
			continue
		}
		for _, f := range c.WriterFiles {
			norm := NormalizedPath(f)
			if len(c.WriterSymbols) == 0 {
				// legacy_bypass: file is dedicated to this surface.
				legacyDedicated[norm] = append(legacyDedicated[norm], ProducerBinding{
					SurfaceID:         c.SurfaceID,
					WriterSymbol:      "", // no specific symbol
					SurfacePath:       c.SurfacePath,
					PersistencePolicy: c.PersistencePolicy,
					RequiredAPI:       "uvb76/internal/artifactio",
				})
				continue
			}
			for _, sym := range c.WriterSymbols {
				idx[key{norm, c.SurfaceID, sym}] = ProducerBinding{
					SurfaceID:         c.SurfaceID,
					WriterSymbol:      sym,
					SurfacePath:       c.SurfacePath,
					PersistencePolicy: c.PersistencePolicy,
					RequiredAPI:       "uvb76/internal/artifactio",
				}
			}
		}
	}
	return func(file string) []ProducerBinding {
		norm := NormalizedPath(file)
		var out []ProducerBinding
		for k, v := range idx {
			if k.file == norm {
				out = append(out, v)
			}
		}
		out = append(out, legacyDedicated[norm]...)
		return out
	}
}

// FileSurfaceMap returns a file -> surfaceID index built from contracts.
// Kept for backward compatibility with the previous detector.
func FileSurfaceMap(contracts []*ProducerContract) func(string) string {
	idx := make(map[string]string)
	for _, c := range contracts {
		if c.Status != StatusActive {
			continue
		}
		for _, f := range c.WriterFiles {
			norm := NormalizedPath(f)
			if _, ok := idx[norm]; !ok {
				idx[norm] = c.SurfaceID
			}
		}
	}
	return func(file string) string {
		return idx[NormalizedPath(file)]
	}
}

// FileExists reports whether rel exists under repoRoot. Public helper for
// the CLI to count verified writer files and test files.
func FileExists(repoRoot, rel string) bool {
	if repoRoot == "" || rel == "" {
		return false
	}
	abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
	info, err := osStatHelper(abs)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// DeclaredFunctionsInFile parses a Go file and returns the set of declared
// function and method names. The returned set contains both bare names and
// receiver-qualified forms (e.g. "Foo.Bar") so callers can look up either
// style.
func DeclaredFunctionsInFile(repoRoot, rel string) (map[string]bool, error) {
	return collectDeclaredFuncs(repoRoot, rel)
}

// osStatHelper is a thin shim around os.Stat used by FileExists.
func osStatHelper(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
