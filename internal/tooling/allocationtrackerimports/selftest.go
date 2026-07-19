package allocationtrackerimports

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const allZigPathspec = ":(glob)**/*.zig"

type fixtureRepo struct {
	root string
}

// SelfTest executes 15 fixture classes and two fail-closed I/O mutations.
func SelfTest() error {
	root, err := os.MkdirTemp("", "kgb-tracker-selftest-")
	if err != nil {
		return fmt.Errorf("creating fixture repository: %w", err)
	}
	defer os.RemoveAll(root)
	fixture := fixtureRepo{root: root}

	if err := fixture.initialize(); err != nil {
		return err
	}
	failures := make([]string, 0)

	trackedInternal, err := fixture.writeForbidden(
		"allocation_tracker_internal.zig", true, "", "",
	)
	if err != nil {
		return err
	}
	if err := fixture.git("-c", "commit.gpgsign=false", "commit", "-q", "-m", "tracked inventory control"); err != nil {
		return err
	}
	untrackedInternal, err := fixture.writeForbidden(
		"allocation_tracker_internal.zig", false, "", "",
	)
	if err != nil {
		return err
	}
	approvedFacade, err := fixture.write(
		"tovarisch/src/approved_facade.zig",
		"const tracker = @import(\"allocation_tracker.zig\");\n",
		true,
	)
	if err != nil {
		return err
	}

	runtimeControls := make([]string, 0, 2)
	for _, tracked := range []bool{true, false} {
		variant := "untracked"
		if tracked {
			variant = "tracked"
		}
		path, writeErr := fixture.write(
			"tovarisch/src/runtime/approved_sibling_"+variant+".zig",
			"const sibling = @import(\"allocation_tracker_internal.zig\");\n",
			tracked,
		)
		if writeErr != nil {
			return writeErr
		}
		runtimeControls = append(runtimeControls, path)
	}

	otherPrivate := make([]string, 0, 4)
	for _, basename := range privateSiblingBasenames[1:] {
		path, writeErr := fixture.writeForbidden(basename, true, "", "")
		if writeErr != nil {
			return writeErr
		}
		otherPrivate = append(otherPrivate, path)
	}
	normalized, err := fixture.writeForbidden(
		"allocation_tracker_internal.zig",
		true,
		"_normalized",
		"./runtime/allocation_tracker_internal.zig",
	)
	if err != nil {
		return err
	}
	multiline, err := fixture.write(
		"tovarisch/src/multiline_forbidden.zig",
		"const private_module = @import(\n    \"runtime/allocation_tracker_internal.zig\",\n);\n",
		true,
	)
	if err != nil {
		return err
	}
	decoyTarget, decoyImporter, err := fixture.writeDecoy()
	if err != nil {
		return err
	}
	computedConcat, computedIdentifier, err := fixture.writeComputedMutations()
	if err != nil {
		return err
	}
	lexicalControl, err := fixture.writeLexicalControl()
	if err != nil {
		return err
	}
	if err := compileDecoy(root, decoyImporter); err != nil {
		failures = append(failures, err.Error())
	}

	cached, err := gitList(root, "--cached", "--", allZigPathspec)
	if err != nil {
		return err
	}
	others, err := gitList(root, "--others", "--exclude-standard", "--", allZigPathspec)
	if err != nil {
		return err
	}
	cachedFiltered := filterTrustedRuntime(cached)
	othersFiltered := filterTrustedRuntime(others)

	record(
		contains(cachedFiltered, trackedInternal) && !contains(othersFiltered, trackedInternal),
		"tracked fixture is not exclusively in cached inventory",
		&failures,
	)
	record(
		contains(othersFiltered, untrackedInternal) && !contains(cachedFiltered, untrackedInternal),
		"untracked fixture is not exclusively in others inventory",
		&failures,
	)
	for _, control := range runtimeControls {
		record(
			!contains(cachedFiltered, control) && !contains(othersFiltered, control),
			"trusted runtime fixture survived filtering: "+control,
			&failures,
		)
	}

	scopedCached, err := gitList(root, "--cached", "--", SourceScopePathspec)
	if err != nil {
		return err
	}
	scopedOthers, err := gitList(
		root, "--others", "--exclude-standard", "--", SourceScopePathspec,
	)
	if err != nil {
		return err
	}
	record(
		contains(scopedCached, trackedInternal) && contains(scopedOthers, untrackedInternal),
		"source pathspec omitted root-level tovarisch/src Zig files",
		&failures,
	)

	inventory := append(cachedFiltered, othersFiltered...)
	findings, err := Scan(root, inventory)
	if err != nil {
		return err
	}
	hitPaths := make(map[string]struct{}, len(findings))
	for _, finding := range findings {
		hitPaths[finding.Path] = struct{}{}
	}
	required := []string{
		trackedInternal,
		untrackedInternal,
		normalized,
		multiline,
		computedConcat,
		computedIdentifier,
	}
	required = append(required, otherPrivate...)
	for _, path := range required {
		_, found := hitPaths[path]
		record(found, "required violation not detected: "+path, &failures)
		delete(hitPaths, path)
	}
	if len(hitPaths) != 0 {
		unexpected := make([]string, 0, len(hitPaths))
		for path := range hitPaths {
			unexpected = append(unexpected, path)
		}
		sort.Strings(unexpected)
		failures = append(failures, "unexpected paths flagged: "+strings.Join(unexpected, ", "))
	}
	for _, approved := range []string{
		approvedFacade, decoyTarget, decoyImporter, lexicalControl,
	} {
		for _, finding := range findings {
			record(finding.Path != approved, "approved fixture flagged: "+approved, &failures)
		}
	}

	// Fail closed when a cached file disappears after inventory generation.
	if err := os.Remove(fixture.absolute(trackedInternal)); err != nil {
		return fmt.Errorf("deleting mutation fixture: %w", err)
	}
	deletedInventory, err := gitList(root, "--cached", "--", allZigPathspec)
	if err != nil {
		return err
	}
	if _, scanErr := Scan(root, deletedInventory); scanErr == nil {
		failures = append(failures, "deleted-file mutation passed unexpectedly")
	}

	// Fail closed when an inventory entry is a directory rather than a file.
	directory := "tovarisch/src/directory_replacement.zig"
	if err := os.MkdirAll(fixture.absolute(directory), 0o755); err != nil {
		return fmt.Errorf("creating directory mutation: %w", err)
	}
	if _, scanErr := Scan(root, []string{directory}); scanErr == nil {
		failures = append(failures, "directory mutation passed unexpectedly")
	}

	if len(failures) != 0 {
		return fmt.Errorf("%d failure(s): %s", len(failures), strings.Join(failures, "; "))
	}
	return nil
}

func (f fixtureRepo) initialize() error {
	if err := f.git("init", "-q", "-b", "main"); err != nil {
		return err
	}
	if err := f.git("config", "user.email", "self-test@local"); err != nil {
		return err
	}
	if err := f.git("config", "user.name", "self-test"); err != nil {
		return err
	}
	for _, directory := range []string{
		"tovarisch/src/runtime", "tovarisch/src", "tools",
	} {
		if err := os.MkdirAll(f.absolute(directory), 0o755); err != nil {
			return fmt.Errorf("creating fixture directory %s: %w", directory, err)
		}
	}
	// Existing private targets make path-identity fixtures concrete.
	for _, basename := range privateSiblingBasenames {
		if _, err := f.write(
			"tovarisch/src/runtime/"+basename,
			"pub const marker: usize = 1;\n",
			true,
		); err != nil {
			return err
		}
	}
	return nil
}

func (f fixtureRepo) writeForbidden(basename string, tracked bool, suffix, target string) (string, error) {
	prefix := "untracked"
	if tracked {
		prefix = "tracked"
	}
	stem := strings.TrimSuffix(basename, ".zig")
	path := "tovarisch/src/" + prefix + "_forbidden_" + stem + suffix + ".zig"
	if target == "" {
		target = "runtime/" + basename
	}
	return f.write(path, "const private_module = @import(\""+target+"\");\n", tracked)
}

func (f fixtureRepo) writeDecoy() (string, string, error) {
	target, err := f.write(
		"tools/allocation_tracker_internal.zig",
		"pub const marker: usize = 1;\n",
		true,
	)
	if err != nil {
		return "", "", err
	}
	importer, err := f.write(
		"tools/decoy_importer.zig",
		"const std = @import(\"std\");\n"+
			"const decoy = @import(\"./allocation_tracker_internal.zig\");\n"+
			"test \"same-basename decoy resolves\" {\n"+
			"    try std.testing.expectEqual(@as(usize, 1), decoy.marker);\n"+
			"}\n",
		true,
	)
	return target, importer, err
}

func (f fixtureRepo) writeComputedMutations() (string, string, error) {
	concat, err := f.write(
		"tovarisch/src/computed_concat_import.zig",
		"const private_module = @import(\n"+
			"    \"runtime/\" ++ \"allocation_tracker_internal.zig\",\n"+
			");\n",
		true,
	)
	if err != nil {
		return "", "", err
	}
	identifier, err := f.write(
		"tovarisch/src/computed_identifier_import.zig",
		"const private_path = \"runtime/allocation_tracker_destroy.zig\";\n"+
			"const private_module = @import(private_path);\n",
		true,
	)
	return concat, identifier, err
}

func (f fixtureRepo) writeLexicalControl() (string, error) {
	return f.write(
		"tovarisch/src/lexical_mask_control.zig",
		"const std = @import(\"std\");\n"+
			"const ordinary = \"@import(private_path)\";\n"+
			"// @import(private_path)\n"+
			"const multiline =\n"+
			"    \\\\@import(private_path)\n"+
			";\n"+
			"comptime { _ = std; _ = ordinary; _ = multiline; }\n",
		true,
	)
}

func (f fixtureRepo) write(path, contents string, tracked bool) (string, error) {
	absolute := f.absolute(path)
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		return "", fmt.Errorf("creating parent for %s: %w", path, err)
	}
	if err := os.WriteFile(absolute, []byte(contents), 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	if tracked {
		if err := f.git("add", "--", path); err != nil {
			return "", err
		}
	}
	return path, nil
}

func (f fixtureRepo) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = f.root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, output)
	}
	return nil
}

func (f fixtureRepo) absolute(path string) string {
	return filepath.Join(f.root, filepath.FromSlash(path))
}

func compileDecoy(repoRoot, importer string) error {
	zig, err := exec.LookPath("zig")
	if err != nil {
		return fmt.Errorf("compilable decoy proof requires zig on PATH")
	}
	cache, err := os.MkdirTemp("", "kgb-decoy-zig-cache-")
	if err != nil {
		return fmt.Errorf("creating Zig cache: %w", err)
	}
	defer os.RemoveAll(cache)

	cmd := exec.Command(
		zig,
		"test",
		filepath.FromSlash(importer),
		"--cache-dir", filepath.Join(cache, "local"),
		"--global-cache-dir", filepath.Join(cache, "global"),
	)
	cmd.Dir = repoRoot
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return fmt.Errorf("same-basename decoy did not compile: %w: %s", runErr, output)
	}
	return nil
}

func contains(paths []string, wanted string) bool {
	for _, path := range paths {
		if path == wanted {
			return true
		}
	}
	return false
}

func record(condition bool, message string, failures *[]string) {
	if !condition {
		*failures = append(*failures, message)
	}
}
