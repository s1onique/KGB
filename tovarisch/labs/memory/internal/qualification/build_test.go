package qualification

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var buildSourceState struct {
	sync.Mutex
	root string
	err  error
}

func qualificationBuildSource(t *testing.T) string {
	t.Helper()
	buildSourceState.Lock()
	defer buildSourceState.Unlock()
	if buildSourceState.err != nil {
		t.Fatal(buildSourceState.err)
	}
	if buildSourceState.root != "" {
		return buildSourceState.root
	}
	root, err := os.MkdirTemp("", "qualification-builder-source-*")
	if err != nil {
		t.Fatal(err)
	}
	module := filepath.Join(root, "tovarisch", "labs", "memory")
	pkg := filepath.Join(module, "cmd", "tovarisch-memory-lab")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(module, "go.mod"):    "module builder.fixture\n\ngo 1.25.0\n",
		filepath.Join(pkg, "main.go"):      "package main\nimport (\"fmt\"; \"os\")\nfunc main(){ if len(os.Args)>1 && os.Args[1]==\"--help\" { fmt.Println(\"help\"); return } }\n",
		filepath.Join(pkg, "live_test.go"): "package main\nimport \"testing\"\nfunc TestLiveDockerSmoke_QualifiedExecutionPath(t *testing.T) {}\n",
		filepath.Join(root, "README"):      "fixture\n",
	}
	for path, data := range files {
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "fixture@example.invalid")
	run("config", "user.name", "builder fixture")
	run("add", ".")
	run("commit", "-q", "-m", "fixture")
	buildSourceState.root = root
	return root
}

func resetBuildSeams(t *testing.T) {
	t.Helper()
	oldRun, oldRecord := qualificationRunGo, qualificationRecordBinary
	t.Cleanup(func() { qualificationRunGo, qualificationRecordBinary = oldRun, oldRecord })
}

func fakeBuildOutput(moduleRoot string, args ...string) error {
	for i, arg := range args {
		if arg == "-o" && i+1 < len(args) {
			return os.WriteFile(args[i+1], []byte("temporary output"), 0o755)
		}
	}
	return errors.New("missing -o")
}

func TestQualificationBuild_GOFLAGSBuildVCSFalseRejected(t *testing.T) {
	t.Setenv("GOFLAGS", "-trimpath -buildvcs=false")
	if _, err := BuildQualificationArtifacts(BuildOptions{SourceRoot: "/unused", ArtifactRoot: t.TempDir()}); !errors.Is(err, ErrBuildVCSDisabled) {
		t.Fatalf("error=%v", err)
	}
}

func TestQualificationBuild_MissingEmbeddedRevisionFails(t *testing.T) {
	resetBuildSeams(t)
	qualificationRunGo = fakeBuildOutput
	qualificationRecordBinary = func(string, BinaryRole) (BinaryRecord, error) { return BinaryRecord{}, ErrMissingEmbeddedRevision }
	root, artifacts := qualificationBuildSource(t), t.TempDir()
	if _, err := BuildQualificationArtifacts(BuildOptions{SourceRoot: root, ArtifactRoot: artifacts}); !errors.Is(err, ErrMissingEmbeddedRevision) {
		t.Fatalf("error=%v", err)
	}
	assertBuildOutputsAbsent(t, artifacts)
}
func TestQualificationBuild_MissingEmbeddedModifiedFails(t *testing.T) {
	resetBuildSeams(t)
	qualificationRunGo = fakeBuildOutput
	qualificationRecordBinary = func(string, BinaryRole) (BinaryRecord, error) { return BinaryRecord{}, ErrMissingEmbeddedModified }
	root, artifacts := qualificationBuildSource(t), t.TempDir()
	if _, err := BuildQualificationArtifacts(BuildOptions{SourceRoot: root, ArtifactRoot: artifacts}); !errors.Is(err, ErrMissingEmbeddedModified) {
		t.Fatalf("error=%v", err)
	}
	assertBuildOutputsAbsent(t, artifacts)
}
func TestQualificationBuild_DirtySourceFailsBeforeBuild(t *testing.T) {
	resetBuildSeams(t)
	calls := 0
	qualificationRunGo = func(string, ...string) error { calls++; return nil }
	root := qualificationBuildSource(t)
	readme := filepath.Join(root, "README")
	if err := os.WriteFile(readme, []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd := exec.Command("git", "-C", root, "checkout", "--", "README"); _ = cmd.Run() })
	if _, err := BuildQualificationArtifacts(BuildOptions{SourceRoot: root, ArtifactRoot: t.TempDir()}); !errors.Is(err, ErrDirtySource) {
		t.Fatalf("error=%v", err)
	}
	if calls != 0 {
		t.Fatalf("build called %d times before dirty rejection", calls)
	}
}
func TestQualificationBuild_PartialOutputsRemovedOnFailure(t *testing.T) {
	resetBuildSeams(t)
	calls := 0
	qualificationRunGo = func(module string, args ...string) error {
		calls++
		if calls == 1 {
			return fakeBuildOutput(module, args...)
		}
		return errors.New("production build failed")
	}
	root, artifacts := qualificationBuildSource(t), t.TempDir()
	if _, err := BuildQualificationArtifacts(BuildOptions{SourceRoot: root, ArtifactRoot: artifacts}); err == nil {
		t.Fatal("expected build failure")
	}
	assertBuildOutputsAbsent(t, artifacts)
}
func TestQualificationBuild_RecordWrittenLast(t *testing.T) {
	resetBuildSeams(t)
	qualificationRunGo, qualificationRecordBinary = runGo, BinaryRecordFromFile
	root, artifacts := qualificationBuildSource(t), t.TempDir()
	path, err := BuildQualificationArtifacts(BuildOptions{SourceRoot: root, ArtifactRoot: artifacts})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := NewQualificationArtifactPathsForSource(artifacts, root)
	if err != nil {
		t.Fatal(err)
	}
	recordInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, binary := range []string{paths.HelperBinary, paths.ProductionBinary} {
		info, err := os.Stat(binary)
		if err != nil {
			t.Fatal(err)
		}
		if recordInfo.ModTime().Before(info.ModTime()) {
			t.Fatalf("record predates %s", binary)
		}
	}
	if err := VerifyQualificationArtifacts(VerifyOptions{SourceRoot: root, RecordPath: path}); err != nil {
		t.Fatalf("written record did not verify: %v", err)
	}
}

func assertBuildOutputsAbsent(t *testing.T, root string) {
	t.Helper()
	for _, path := range []string{filepath.Join(root, "bin", HelperBinaryName), filepath.Join(root, "bin", ProductionBinaryName), filepath.Join(root, "role-separation.json")} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("partial output remains: %s (err=%v)", path, err)
		}
	}
}

var _ = strings.Contains
