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

type authorityFixture struct {
	source, artifacts, valid, dirty, commit string
	settings                                map[string]string
}

var authorityOnce sync.Once
var authorityValue authorityFixture
var authorityErr error

func authorityTestFixture(t *testing.T) authorityFixture {
	t.Helper()
	authorityOnce.Do(func() {
		source, err := os.MkdirTemp("", "authority-source-*")
		if err != nil {
			authorityErr = err
			return
		}
		artifacts, err := os.MkdirTemp("", "authority-artifacts-*")
		if err != nil {
			authorityErr = err
			return
		}
		write := func(path, data string) {
			if authorityErr == nil {
				authorityErr = os.WriteFile(path, []byte(data), 0o644)
			}
		}
		write(filepath.Join(source, "go.mod"), "module authority.fixture\n\ngo 1.25.0\n")
		write(filepath.Join(source, "main.go"), "package main\nfunc main() {}\n")
		if authorityErr != nil {
			return
		}
		runFixtureCommand := func(name string, args ...string) string {
			if authorityErr != nil {
				return ""
			}
			cmd := exec.Command(name, args...)
			cmd.Dir = source
			out, err := cmd.CombinedOutput()
			if err != nil {
				authorityErr = errors.New(name + " failed: " + string(out))
				return ""
			}
			return strings.TrimSpace(string(out))
		}
		runFixtureCommand("git", "init", "-q")
		runFixtureCommand("git", "config", "user.email", "fixture@example.invalid")
		runFixtureCommand("git", "config", "user.name", "authority fixture")
		runFixtureCommand("git", "add", ".")
		runFixtureCommand("git", "commit", "-q", "-m", "fixture")
		commit := runFixtureCommand("git", "rev-parse", "HEAD")
		valid := filepath.Join(artifacts, "valid")
		runFixtureCommand("go", "build", "-buildvcs=true", "-o", valid, ".")
		settings, err := readEmbeddedBinarySettings(valid)
		if err != nil {
			authorityErr = err
			return
		}
		write(filepath.Join(source, "untracked.txt"), "dirty\n")
		dirty := filepath.Join(artifacts, "dirty")
		runFixtureCommand("go", "build", "-buildvcs=true", "-o", dirty, ".")
		_ = os.Remove(filepath.Join(source, "untracked.txt"))
		authorityValue = authorityFixture{source: source, artifacts: artifacts, valid: valid, dirty: dirty, commit: commit, settings: settings}
	})
	if authorityErr != nil {
		t.Fatal(authorityErr)
	}
	return authorityValue
}

func TestMain(m *testing.M) {
	code := m.Run()
	if authorityValue.source != "" {
		_ = os.RemoveAll(authorityValue.source)
	}
	if authorityValue.artifacts != "" {
		_ = os.RemoveAll(authorityValue.artifacts)
	}
	if buildSourceState.root != "" {
		_ = os.RemoveAll(buildSourceState.root)
	}
	os.Exit(code)
}

func copySettings(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func TestEmbeddedBinaryAuthority_MissingRevisionRejected(t *testing.T) {
	f := authorityTestFixture(t)
	settings := copySettings(f.settings)
	delete(settings, "vcs.revision")
	if _, err := validateEmbeddedBinarySettings(settings); !errors.Is(err, ErrMissingEmbeddedRevision) {
		t.Fatalf("error=%v", err)
	}
}
func TestEmbeddedBinaryAuthority_MissingModifiedRejected(t *testing.T) {
	f := authorityTestFixture(t)
	settings := copySettings(f.settings)
	delete(settings, "vcs.modified")
	if _, err := validateEmbeddedBinarySettings(settings); !errors.Is(err, ErrMissingEmbeddedModified) {
		t.Fatalf("error=%v", err)
	}
}
func TestEmbeddedBinaryAuthority_EmptyModifiedRejected(t *testing.T) {
	f := authorityTestFixture(t)
	settings := copySettings(f.settings)
	settings["vcs.modified"] = ""
	if _, err := validateEmbeddedBinarySettings(settings); !errors.Is(err, ErrEmptyEmbeddedModified) {
		t.Fatalf("error=%v", err)
	}
}
func TestEmbeddedBinaryAuthority_ModifiedTrueRejected(t *testing.T) {
	f := authorityTestFixture(t)
	if _, err := ReadEmbeddedBinaryAuthority(f.dirty); !errors.Is(err, ErrModifiedBinary) {
		t.Fatalf("error=%v", err)
	}
}
func TestEmbeddedBinaryAuthority_RevisionMismatchRejected(t *testing.T) {
	f := authorityTestFixture(t)
	if _, err := ReadEmbeddedBinaryAuthorityForCommit(f.valid, strings.Repeat("b", 40)); !errors.Is(err, ErrEmbeddedRevisionMismatch) {
		t.Fatalf("error=%v", err)
	}
}
func TestEmbeddedBinaryAuthority_MalformedRevisionRejected(t *testing.T) {
	f := authorityTestFixture(t)
	settings := copySettings(f.settings)
	settings["vcs.revision"] = "not-an-object-id"
	if _, err := validateEmbeddedBinarySettings(settings); !errors.Is(err, ErrMalformedEmbeddedRevision) {
		t.Fatalf("error=%v", err)
	}
}
func TestEmbeddedBinaryAuthority_ValidStampedBinaryAccepted(t *testing.T) {
	f := authorityTestFixture(t)
	authority, err := ReadEmbeddedBinaryAuthorityForCommit(f.valid, f.commit)
	if err != nil {
		t.Fatal(err)
	}
	if authority.VCS != "git" || authority.VCSRevision != f.commit || authority.VCSModified || authority.VCSTime == "" {
		t.Fatalf("authority=%+v", authority)
	}
}
