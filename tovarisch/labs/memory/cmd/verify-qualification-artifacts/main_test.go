package main

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/qualification"
)

type fixture struct {
	root       string
	artifacts  string
	helper     string
	production string
	commit     string
	tree       string
}

var fixtureCache = struct {
	sync.Mutex
	values map[string]fixture
}{values: make(map[string]fixture)}

func TestMain(m *testing.M) {
	code := m.Run()
	fixtureCache.Lock()
	for _, value := range fixtureCache.values {
		_ = os.RemoveAll(value.root)
		_ = os.RemoveAll(value.artifacts)
	}
	fixtureCache.Unlock()
	os.Exit(code)
}

func cleanFixture(t *testing.T, liveTest string, helpExit int) fixture {
	t.Helper()
	key := liveTest + ":" + itoa(helpExit)
	fixtureCache.Lock()
	defer fixtureCache.Unlock()
	if value, ok := fixtureCache.values[key]; ok {
		return value
	}
	root, err := os.MkdirTemp("", "qualification-source-*")
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := os.MkdirTemp("", "qualification-artifacts-*")
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "go.mod"), "module fixture.example\n\ngo 1.25.0\n")
	pkg := filepath.Join(root, "cmd", "tovarisch-memory-lab")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(pkg, "live_test.go"), "package main\n\nimport \"testing\"\n\nfunc "+liveTest+"(t *testing.T) {}\n")
	mainBody := "package main\n\nimport (\"fmt\"; \"os\")\nfunc main() { if len(os.Args) > 1 && os.Args[1] == \"--help\" { fmt.Println(\"fixture help\"); os.Exit(" + itoa(helpExit) + ") }; os.Exit(0) }\n"
	mustWrite(t, filepath.Join(pkg, "main.go"), mainBody)
	git(t, root, "init", "-q")
	git(t, root, "config", "user.email", "fixture@example.invalid")
	git(t, root, "config", "user.name", "qualification fixture")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "fixture")
	commit := git(t, root, "rev-parse", "HEAD")
	tree := git(t, root, "rev-parse", "HEAD^{tree}")
	helper := filepath.Join(artifacts, "helper.test")
	production := filepath.Join(artifacts, "production")
	goRun(t, root, "test", "-buildvcs=true", "-c", "-o", helper, "./cmd/tovarisch-memory-lab")
	goRun(t, root, "build", "-buildvcs=true", "-o", production, "./cmd/tovarisch-memory-lab")
	value := fixture{root: root, artifacts: artifacts, helper: helper, production: production, commit: commit, tree: tree}
	fixtureCache.values[key] = value
	return value
}

func qualificationCase(t *testing.T, f fixture) (qualification.QualificationRecord, string) {
	t.Helper()
	dir := t.TempDir()
	helper := filepath.Join(dir, "helper.test")
	production := filepath.Join(dir, "production")
	linkOrCopy(t, f.helper, helper)
	linkOrCopy(t, f.production, production)
	helperRecord, err := qualification.BinaryRecordFromFile(helper, qualification.BinaryRoleLiveHelper)
	if err != nil {
		t.Fatal(err)
	}
	productionRecord, err := qualification.BinaryRecordFromFile(production, qualification.BinaryRoleProductionCLI)
	if err != nil {
		t.Fatal(err)
	}
	record := qualification.QualificationRecord{
		SchemaVersion: qualification.RecordSchemaVersion,
		SourceRoot:    f.root, SourceCommit: f.commit, SourceTree: f.tree,
		Helper: helperRecord, Production: productionRecord,
		HelperLiveTest: qualification.LiveHelperTest, ProductionHelpExitCode: 0,
	}
	return record, filepath.Join(dir, "role-separation.json")
}

func writeRecord(t *testing.T, path string, record qualification.QualificationRecord) {
	t.Helper()
	data, err := qualification.MarshalQualificationRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func verifyCase(t *testing.T, record qualification.QualificationRecord, path string) error {
	t.Helper()
	writeRecord(t, path, record)
	return qualification.VerifyQualificationArtifacts(qualification.VerifyOptions{SourceRoot: record.SourceRoot, RecordPath: path})
}

func TestQualificationArtifacts_SamePathRejectedAtRelationshipGuard(t *testing.T) {
	f := cleanFixture(t, qualification.LiveHelperTest, 0)
	record, path := qualificationCase(t, f)
	record.Production.AbsolutePath = record.Helper.AbsolutePath
	record.Production.Device = record.Helper.Device
	record.Production.Inode = record.Helper.Inode
	record.Production.Size = record.Helper.Size
	record.Production.SHA256 = record.Helper.SHA256
	record.Production.VCS = record.Helper.VCS
	record.Production.VCSRevision = record.Helper.VCSRevision
	record.Production.VCSTime = record.Helper.VCSTime
	record.Production.VCSModified = record.Helper.VCSModified
	if !errors.Is(verifyCase(t, record, path), qualification.ErrRelationshipSamePath) {
		t.Fatalf("expected same-path sentinel")
	}
}

func TestQualificationArtifacts_SameDeviceInodeRejectedAtRelationshipGuard(t *testing.T) {
	f := cleanFixture(t, qualification.LiveHelperTest, 0)
	record, path := qualificationCase(t, f)
	if err := os.Remove(record.Production.AbsolutePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(record.Helper.AbsolutePath, record.Production.AbsolutePath); err != nil {
		t.Fatal(err)
	}
	production, err := qualification.BinaryRecordFromFile(record.Production.AbsolutePath, qualification.BinaryRoleProductionCLI)
	if err != nil {
		t.Fatal(err)
	}
	record.Production = production
	if !errors.Is(verifyCase(t, record, path), qualification.ErrRelationshipSameDeviceInode) {
		t.Fatalf("expected same-device/inode sentinel")
	}
}

func TestQualificationArtifacts_SameHashRejectedAtRelationshipGuard(t *testing.T) {
	f := cleanFixture(t, qualification.LiveHelperTest, 0)
	record, path := qualificationCase(t, f)
	copyFile(t, record.Helper.AbsolutePath, record.Production.AbsolutePath)
	production, err := qualification.BinaryRecordFromFile(record.Production.AbsolutePath, qualification.BinaryRoleProductionCLI)
	if err != nil {
		t.Fatal(err)
	}
	record.Production = production
	if !errors.Is(verifyCase(t, record, path), qualification.ErrRelationshipSameHash) {
		t.Fatalf("expected same-hash sentinel")
	}
}

func TestQualificationArtifacts_HelperRevisionMismatchRejected(t *testing.T) {
	f := cleanFixture(t, qualification.LiveHelperTest, 0)
	record, path := qualificationCase(t, f)
	record.Helper.VCSRevision = strings.Repeat("b", 40)
	if !errors.Is(verifyCase(t, record, path), qualification.ErrHelperRevisionMismatch) {
		t.Fatalf("expected helper revision sentinel")
	}
}
func TestQualificationArtifacts_ProductionRevisionMismatchRejected(t *testing.T) {
	f := cleanFixture(t, qualification.LiveHelperTest, 0)
	record, path := qualificationCase(t, f)
	record.Production.VCSRevision = strings.Repeat("b", 40)
	if !errors.Is(verifyCase(t, record, path), qualification.ErrProductionRevisionMismatch) {
		t.Fatalf("expected production revision sentinel")
	}
}
func TestQualificationArtifacts_HelperModifiedRejected(t *testing.T) {
	f := cleanFixture(t, qualification.LiveHelperTest, 0)
	record, path := qualificationCase(t, f)
	record.Helper.VCSModified = true
	if !errors.Is(verifyCase(t, record, path), qualification.ErrHelperModified) {
		t.Fatalf("expected helper modified sentinel")
	}
}
func TestQualificationArtifacts_ProductionModifiedRejected(t *testing.T) {
	f := cleanFixture(t, qualification.LiveHelperTest, 0)
	record, path := qualificationCase(t, f)
	record.Production.VCSModified = true
	if !errors.Is(verifyCase(t, record, path), qualification.ErrProductionModified) {
		t.Fatalf("expected production modified sentinel")
	}
}
func TestQualificationArtifacts_HelperTestMissingRejected(t *testing.T) {
	f := cleanFixture(t, qualification.LiveHelperTest, 0)
	record, path := qualificationCase(t, f)
	record.HelperLiveTest = "TestLiveDockerSmoke_QualifiedExecutionPathSimilar"
	if !errors.Is(verifyCase(t, record, path), qualification.ErrHelperTestMissing) {
		t.Fatalf("expected helper-test sentinel")
	}
}
func TestQualificationArtifacts_ProductionHelpFailureRejected(t *testing.T) {
	f := cleanFixture(t, qualification.LiveHelperTest, 0)
	record, path := qualificationCase(t, f)
	record.ProductionHelpExitCode = 1
	if !errors.Is(verifyCase(t, record, path), qualification.ErrProductionHelpFailure) {
		t.Fatalf("expected production-help sentinel")
	}
}
func TestQualificationArtifacts_SourceCommitMismatchRejected(t *testing.T) {
	f := cleanFixture(t, qualification.LiveHelperTest, 0)
	record, path := qualificationCase(t, f)
	record.SourceCommit = strings.Repeat("b", 40)
	if !errors.Is(verifyCase(t, record, path), qualification.ErrSourceCommitMismatch) {
		t.Fatalf("expected source-commit sentinel")
	}
}
func TestQualificationArtifacts_SourceTreeMismatchRejected(t *testing.T) {
	f := cleanFixture(t, qualification.LiveHelperTest, 0)
	record, path := qualificationCase(t, f)
	record.SourceTree = strings.Repeat("b", 40)
	if !errors.Is(verifyCase(t, record, path), qualification.ErrSourceTreeMismatch) {
		t.Fatalf("expected source-tree sentinel")
	}
}

func validRecordJSON(t *testing.T) []byte {
	t.Helper()
	f := cleanFixture(t, qualification.LiveHelperTest, 0)
	record, _ := qualificationCase(t, f)
	data, err := qualification.MarshalQualificationRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func TestQualificationArtifacts_UnknownFieldRejected(t *testing.T) {
	data := string(validRecordJSON(t))
	data = strings.Replace(data, "{", "{\"unknown\":1,", 1)
	if _, err := qualification.DecodeQualificationRecord([]byte(data)); !errors.Is(err, qualification.ErrRecordUnknownField) {
		t.Fatalf("error=%v", err)
	}
}
func TestQualificationArtifacts_MissingFieldRejected(t *testing.T) {
	data := string(validRecordJSON(t))
	lines := strings.Split(data, "\n")
	var out []string
	for _, line := range lines {
		if strings.Contains(line, `"source_tree"`) {
			continue
		}
		out = append(out, line)
	}
	if _, err := qualification.DecodeQualificationRecord([]byte(strings.Join(out, "\n"))); !errors.Is(err, qualification.ErrRecordMissingField) {
		t.Fatalf("error=%v", err)
	}
}
func TestQualificationArtifacts_NullFieldRejected(t *testing.T) {
	data := string(validRecordJSON(t))
	data = replaceField(data, "source_tree", "null")
	if _, err := qualification.DecodeQualificationRecord([]byte(data)); !errors.Is(err, qualification.ErrRecordNullField) {
		t.Fatalf("error=%v", err)
	}
}
func TestQualificationArtifacts_WrongTypeRejected(t *testing.T) {
	data := string(validRecordJSON(t))
	data = replaceField(data, "source_tree", "7")
	if _, err := qualification.DecodeQualificationRecord([]byte(data)); !errors.Is(err, qualification.ErrRecordWrongType) {
		t.Fatalf("error=%v", err)
	}
}
func TestQualificationArtifacts_SecondJSONRejected(t *testing.T) {
	data := append(validRecordJSON(t), []byte("{}\n")...)
	if _, err := qualification.DecodeQualificationRecord(data); !errors.Is(err, qualification.ErrRecordSecondJSON) {
		t.Fatalf("error=%v", err)
	}
}
func TestQualificationArtifacts_DuplicateKeyRejected(t *testing.T) {
	data := strings.Replace(string(validRecordJSON(t)), "\"schema_version\"", "\"schema_version\":\""+qualification.RecordSchemaVersion+"\",\n  \"schema_version\"", 1)
	if _, err := qualification.DecodeQualificationRecord([]byte(data)); !errors.Is(err, qualification.ErrRecordDuplicateKey) {
		t.Fatalf("error=%v", err)
	}
}
func TestQualificationArtifacts_TrailingNonWhitespaceRejected(t *testing.T) {
	data := append(validRecordJSON(t), 'x')
	if _, err := qualification.DecodeQualificationRecord(data); !errors.Is(err, qualification.ErrRecordTrailingData) {
		t.Fatalf("error=%v", err)
	}
}

func TestHelperRole_ExactLiveTestPresent(t *testing.T) {
	f := cleanFixture(t, qualification.LiveHelperTest, 0)
	if _, err := qualification.RunExactHelperTestList(f.helper, qualification.LiveHelperTest); err != nil {
		t.Fatal(err)
	}
}
func TestHelperRole_MissingLiveTestRejected(t *testing.T) {
	f := cleanFixture(t, "TestOther", 0)
	if _, err := qualification.RunExactHelperTestList(f.helper, qualification.LiveHelperTest); !errors.Is(err, qualification.ErrHelperTestMissing) {
		t.Fatalf("error=%v", err)
	}
}
func TestHelperRole_AdditionalSimilarNameDoesNotSatisfy(t *testing.T) {
	f := cleanFixture(t, "TestLiveDockerSmoke_QualifiedExecutionPathSimilar", 0)
	if _, err := qualification.RunExactHelperTestList(f.helper, qualification.LiveHelperTest); !errors.Is(err, qualification.ErrHelperTestMissing) {
		t.Fatalf("error=%v", err)
	}
}
func TestHelperRole_TestListNonZeroRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "helper")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := qualification.RunExactHelperTestList(path, qualification.LiveHelperTest); !errors.Is(err, qualification.ErrHelperTestMissing) {
		t.Fatalf("error=%v", err)
	}
}

func TestMandatoryQualificationVerifierTestInventory(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"TestQualificationArtifacts_SamePathRejectedAtRelationshipGuard", "TestQualificationArtifacts_SameDeviceInodeRejectedAtRelationshipGuard", "TestQualificationArtifacts_SameHashRejectedAtRelationshipGuard", "TestQualificationArtifacts_HelperRevisionMismatchRejected", "TestQualificationArtifacts_ProductionRevisionMismatchRejected", "TestQualificationArtifacts_HelperModifiedRejected", "TestQualificationArtifacts_ProductionModifiedRejected", "TestQualificationArtifacts_HelperTestMissingRejected", "TestQualificationArtifacts_ProductionHelpFailureRejected", "TestQualificationArtifacts_SourceCommitMismatchRejected", "TestQualificationArtifacts_SourceTreeMismatchRejected", "TestQualificationArtifacts_UnknownFieldRejected", "TestQualificationArtifacts_MissingFieldRejected", "TestQualificationArtifacts_NullFieldRejected", "TestQualificationArtifacts_WrongTypeRejected", "TestQualificationArtifacts_SecondJSONRejected", "TestQualificationArtifacts_DuplicateKeyRejected", "TestQualificationArtifacts_TrailingNonWhitespaceRejected"}
	found := map[string]bool{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv == nil {
			found[fn.Name.Name] = true
		}
	}
	for _, name := range want {
		if !found[name] {
			t.Errorf("mandatory test missing: %s", name)
		}
	}
}

func replaceField(data, field, value string) string {
	for _, line := range strings.Split(data, "\n") {
		if strings.Contains(line, `"`+field+`"`) {
			old := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			if strings.HasSuffix(old, ",") {
				value += ","
			}
			return strings.Replace(data, old, value, 1)
		}
	}
	return data
}
func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	return "1"
}
func mustWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
func linkOrCopy(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.Link(src, dst); err == nil {
		return
	}
	copyFile(t, src, dst)
}
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(dst)
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatal(err)
	}
}
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
func goRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go %v: %v (%s)", args, err, out)
	}
}

