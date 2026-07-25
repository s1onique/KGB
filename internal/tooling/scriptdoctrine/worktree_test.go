package scriptdoctrine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScriptDoctrine_MainWorktree(t *testing.T) {
	main, _ := scriptDoctrineRepo(t)
	hooks, err := gitHooksPath(main)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(hooks); err != nil || !info.IsDir() {
		t.Fatalf("hooks authority=%q err=%v", hooks, err)
	}
	assertHookWalkHasNoInternalError(t, main)
}

func TestScriptDoctrine_LinkedWorktreeGitfile(t *testing.T) {
	main, parent := scriptDoctrineRepo(t)
	linked := filepath.Join(parent, "linked")
	runGitTest(t, main, "worktree", "add", "-q", "-b", "linked-test", linked, "HEAD")
	if info, err := os.Stat(filepath.Join(linked, ".git")); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("linked worktree must expose a gitfile: info=%v err=%v", info, err)
	}
	if _, err := gitHooksPath(linked); err != nil {
		t.Fatal(err)
	}
	assertHookWalkHasNoInternalError(t, linked)
}

func TestScriptDoctrine_DetachedLinkedWorktree(t *testing.T) {
	main, parent := scriptDoctrineRepo(t)
	linked := filepath.Join(parent, "detached")
	runGitTest(t, main, "worktree", "add", "-q", "--detach", linked, "HEAD")
	branch := runGitTest(t, linked, "symbolic-ref", "-q", "--short", "HEAD")
	if branch != "" {
		t.Fatalf("worktree unexpectedly attached to %q", branch)
	}
	if _, err := gitHooksPath(linked); err != nil {
		t.Fatal(err)
	}
	assertHookWalkHasNoInternalError(t, linked)
}

func scriptDoctrineRepo(t *testing.T) (main, parent string) {
	t.Helper()
	parent = t.TempDir()
	main = filepath.Join(parent, "main")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, main, "init", "-q")
	runGitTest(t, main, "config", "user.email", "fixture@example.invalid")
	runGitTest(t, main, "config", "user.name", "script doctrine fixture")
	if err := os.WriteFile(filepath.Join(main, "README"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, main, "add", "README")
	runGitTest(t, main, "commit", "-q", "-m", "fixture")
	return main, parent
}

func runGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	// symbolic-ref exits 1 for the expected detached state.
	if err != nil && !(len(args) > 0 && args[0] == "symbolic-ref") {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func assertHookWalkHasNoInternalError(t *testing.T, root string) {
	t.Helper()
	verifier := NewVerifier(root, false)
	var diags []Diagnostic
	verifier.walkGitHooks(root, &diags)
	for _, diag := range diags {
		if diag.Check == "internal-error" {
			t.Fatalf("hook walk internal error: %+v", diag)
		}
	}
}
