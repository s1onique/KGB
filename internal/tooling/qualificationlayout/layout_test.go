package qualificationlayout

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestQualificationHarness_NoGeneratedExecutablesAtDeletedPaths(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{"build-qualification-artifacts", filepath.Join("cmd", "verify-qualification-artifacts", "verify-qualification-artifacts")} {
		path := filepath.Join(root, relative)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			t.Fatalf("generated executable remains at deleted path %s", relative)
		} else if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "-C", root, "ls-files", "--error-unmatch", "--", relative)
		if err := cmd.Run(); err == nil {
			t.Fatalf("deleted generated path is still tracked: %s", relative)
		}
	}
}

func TestQualificationHarness_NoTrackedELFBinaries(t *testing.T) {
	root := repositoryRoot(t)
	cmd := exec.Command("git", "-C", root, "ls-files", "-z")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Split(splitNUL)
	for scanner.Scan() {
		relative := scanner.Text()
		if relative == "" {
			continue
		}
		file, err := os.Open(filepath.Join(root, relative))
		if err != nil {
			continue
		}
		header := make([]byte, 4)
		n, _ := file.Read(header)
		_ = file.Close()
		if n == 4 && bytes.Equal(header, []byte{0x7f, 'E', 'L', 'F'}) {
			t.Errorf("tracked generated ELF binary: %s", relative)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestQualificationHarness_CommandsLiveInMemoryModule(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{filepath.Join("tovarisch", "labs", "memory", "cmd", "build-qualification-artifacts", "main.go"), filepath.Join("tovarisch", "labs", "memory", "cmd", "verify-qualification-artifacts", "main.go")} {
		if info, err := os.Stat(filepath.Join(root, relative)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("memory-module command missing: %s (err=%v)", relative, err)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
func splitNUL(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
