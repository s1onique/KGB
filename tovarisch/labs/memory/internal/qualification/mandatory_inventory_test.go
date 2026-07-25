package qualification

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMandatoryQualificationTestInventory(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	found := map[string]string{}
	err := filepath.Walk(moduleRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range parsed.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
				found[fn.Name.Name] = path
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	mandatory := []string{
		"TestEmbeddedBinaryAuthority_MissingRevisionRejected", "TestEmbeddedBinaryAuthority_MissingModifiedRejected", "TestEmbeddedBinaryAuthority_EmptyModifiedRejected", "TestEmbeddedBinaryAuthority_ModifiedTrueRejected", "TestEmbeddedBinaryAuthority_RevisionMismatchRejected", "TestEmbeddedBinaryAuthority_MalformedRevisionRejected", "TestEmbeddedBinaryAuthority_ValidStampedBinaryAccepted",
		"TestQualificationBuild_GOFLAGSBuildVCSFalseRejected", "TestQualificationBuild_MissingEmbeddedRevisionFails", "TestQualificationBuild_MissingEmbeddedModifiedFails", "TestQualificationBuild_DirtySourceFailsBeforeBuild", "TestQualificationBuild_PartialOutputsRemovedOnFailure", "TestQualificationBuild_RecordWrittenLast",
		"TestControllerProvenance_EmptyPolicyRejected", "TestControllerProvenance_UnknownPolicyRejected", "TestControllerProvenance_RequireCleanAcceptsClean", "TestControllerProvenance_RequireCleanRejectsTracked", "TestControllerProvenance_RequireCleanRejectsStaged", "TestControllerProvenance_RequireCleanRejectsUntracked", "TestControllerProvenance_RequireCleanRejectsVCSModified", "TestControllerProvenance_IgnoreWorktreeNonQualifying", "TestProductionCLI_UsesRequireClean", "TestLiveHelper_UsesRequireClean",
		"TestHelperRole_ExactLiveTestPresent", "TestHelperRole_MissingLiveTestRejected", "TestHelperRole_AdditionalSimilarNameDoesNotSatisfy", "TestHelperRole_TestListNonZeroRejected",
		"TestQualificationArtifacts_SamePathRejectedAtRelationshipGuard", "TestQualificationArtifacts_SameDeviceInodeRejectedAtRelationshipGuard", "TestQualificationArtifacts_SameHashRejectedAtRelationshipGuard", "TestQualificationArtifacts_HelperRevisionMismatchRejected", "TestQualificationArtifacts_ProductionRevisionMismatchRejected", "TestQualificationArtifacts_HelperModifiedRejected", "TestQualificationArtifacts_ProductionModifiedRejected", "TestQualificationArtifacts_HelperTestMissingRejected", "TestQualificationArtifacts_ProductionHelpFailureRejected", "TestQualificationArtifacts_SourceCommitMismatchRejected", "TestQualificationArtifacts_SourceTreeMismatchRejected", "TestQualificationArtifacts_UnknownFieldRejected", "TestQualificationArtifacts_MissingFieldRejected", "TestQualificationArtifacts_NullFieldRejected", "TestQualificationArtifacts_WrongTypeRejected", "TestQualificationArtifacts_SecondJSONRejected", "TestQualificationArtifacts_DuplicateKeyRejected", "TestQualificationArtifacts_TrailingNonWhitespaceRejected",
	}
	for _, name := range mandatory {
		if _, ok := found[name]; !ok {
			t.Errorf("mandatory test function is absent: %s", name)
		}
	}
}
