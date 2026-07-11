package scriptdoctrine

import (
	"strings"
	"testing"
)

// TestR11BashDashC exercises the bash -c / sh -c handling
// introduced in R11.2. The branch walks the R11.2 contract:
//
//   - `bash -c 'python3 x.py'`             -> 1
//   - `bash -ce 'python3 x.py'`            -> 1
//   - `bash -ec 'python3 x.py'`            -> 1
//   - `bash -euc 'python3 x.py'`           -> 1
//   - `bash -xec 'python3 x.py'`           -> 1
//   - `/bin/sh -xc 'python3 x.py'`         -> 1
//   - `bash "$SCRIPT" /tmp`                -> 0 (NOT a -c
//     invocation; no -c flag was seen)
//
// The dynamic command-string cases return errors from the
// internal helper; the public compatibility wrapper translates
// those to -1 so the verifier can surface an internal-error
// diagnostic.
func TestR11BashDashC(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		{"literal single-quoted", `bash -c 'python3 x.py'`, 1},
		{"literal double-quoted", `bash -c "python3 x.py"`, 1},
		{"combined -ce cluster", `bash -ce 'python3 x.py'`, 1},
		{"combined -ec cluster", `bash -ec 'python3 x.py'`, 1},
		{"combined -euc cluster", `bash -euc 'python3 x.py'`, 1},
		{"combined -xec cluster", `bash -xec 'python3 x.py'`, 1},
		{"abs path -xc", `/bin/sh -xc 'python3 x.py'`, 1},
		{"script-path not -c", `bash "$SCRIPT" /tmp`, 0},
		{"script-path with dynamic arg not -c", `bash /tmp/check.sh "$@"`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.data))
			if got != tc.want {
				t.Errorf("CountPythonInvocations(%q) = %d, want %d", tc.data, got, tc.want)
			}
		})
	}
}

// TestR11BashDashCFailClosed pins the dynamic-payload contract:
// a `bash -c "$VAR"` invocation must surface an internal error
// (translated to -1 by the public compatibility wrapper) rather
// than silently green-light the file.
func TestR11BashDashCFailClosed(t *testing.T) {
	cases := []string{
		`bash -c "$COMMAND"`,
		`bash -c "${COMMAND}"`,
		`bash -c "$(generate-command)"`,
		`bash -c "$prefix python3 x.py"`,
		`bash -c 'if true then python3 x.py'`, // malformed nested script
	}
	for _, data := range cases {
		t.Run(data, func(t *testing.T) {
			got := CountPythonInvocations([]byte(data))
			if got != -1 {
				t.Errorf("CountPythonInvocations(%q) = %d, want -1 (fail-closed)", data, got)
			}
		})
	}
}

// TestR11SudoWrappers exercises the sudo option tables added in
// R11.3. Each test asserts that a previously-undetected sudo
// option cluster is now classified as Python when the executable
// is python.
func TestR11SudoWrappers(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		{"sudo -S python3 x.py", `sudo -S python3 x.py`, 1},
		{"sudo -A python3 x.py", `sudo -A python3 x.py`, 1},
		{"sudo --stdin python3 x.py", `sudo --stdin python3 x.py`, 1},
		{"sudo --askpass python3 x.py", `sudo --askpass python3 x.py`, 1},
		{"sudo -u root python3 x.py", `sudo -u root python3 x.py`, 1},
		{"sudo --user=root python3 x.py", `sudo --user=root python3 x.py`, 1},
		{"sudo -- python3 x.py", `sudo -- python3 x.py`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.data))
			if got != tc.want {
				t.Errorf("CountPythonInvocations(%q) = %d, want %d", tc.data, got, tc.want)
			}
		})
	}
}

// TestR11CommandLookups pins the R11 command-prefix shape; the
// R10 baseline already covered `command -v` lookups, R11 confirms
// the contract at the boundary.
func TestR11CommandLookups(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		{"command -v python3", `command -v python3`, 0},
		{"command -V python3", `command -V python3`, 0},
		{"command -- python3 x.py", `command -- python3 x.py`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.data))
			if got != tc.want {
				t.Errorf("CountPythonInvocations(%q) = %d, want %d", tc.data, got, tc.want)
			}
		})
	}
}

// TestR11InvocationCountType pins the R11.1 contract that the
// internal classifier returns an InvocationCount value with a
// nil error on a clean script. The public compatibility helper
// translates that to a bare int.
func TestR11InvocationCountType(t *testing.T) {
	count, err := countPythonSitesInProgram(`python3 x.py`)
	if err != nil {
		t.Fatalf("countPythonSitesInProgram returned error: %v", err)
	}
	if count.Count != 1 {
		t.Errorf("count.Count = %d, want 1", count.Count)
	}
	if ZeroCount.Count != 0 {
		t.Errorf("ZeroCount.Count = %d, want 0", ZeroCount.Count)
	}
	if !count.HasSites() {
		t.Errorf("HasSites() = false, want true")
	}
	if ZeroCount.HasSites() {
		t.Errorf("ZeroCount.HasSites() = true, want false")
	}
}

// TestR11ClassificationError pins the R11.1 error type surface.
func TestR11ClassificationError(t *testing.T) {
	err := NewClassificationError("path/to/Makefile", 12, 5, "dynamic bash -c command string")
	if err == nil {
		t.Fatal("NewClassificationError returned nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "line 12") {
		t.Errorf("error msg %q missing line 12", msg)
	}
	if !strings.Contains(msg, "column 5") {
		t.Errorf("error msg %q missing column 5", msg)
	}
	if !strings.Contains(msg, "dynamic bash -c command string") {
		t.Errorf("error msg %q missing reason", msg)
	}
}

// TestR11WrappedBashDashC is the R12 closure matrix for the
// `sudo bash -c 'python3 x.py'` family. Each row exercises a
// wrapper around an otherwise-classified `bash -c` invocation
// and asserts the residual python count is exactly one.
func TestR11WrappedBashDashC(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		{"sudo bash -c 'python3 x.py'", `sudo bash -c 'python3 x.py'`, 1},
		{"env FOO=bar sh -c 'python3 x.py'", `env FOO=bar sh -c 'python3 x.py'`, 1},
		{"exec bash -c 'python3 x.py'", `exec bash -c 'python3 x.py'`, 1},
		{"command bash -c 'python3 x.py'", `command bash -c 'python3 x.py'`, 1},
		{"command -- bash -c 'python3 x.py'", `command -- bash -c 'python3 x.py'`, 1},
		{"env FOO=bar exec bash -c 'python3 x.py'", `env FOO=bar exec bash -c 'python3 x.py'`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.data))
			if got != tc.want {
				t.Errorf("CountPythonInvocations(%q) = %d, want %d", tc.data, got, tc.want)
			}
		})
	}
}

// TestR11WrappedBashDashCFailClosed documents the dynamic
// payload contract for the wrapped surface: `sudo bash -c
// "$COMMAND"` must surface an internal error rather than
// silently green-light the file.
func TestR11WrappedBashDashCFailClosed(t *testing.T) {
	cases := []string{
		`sudo bash -c "$COMMAND"`,
		`env FOO=bar sh -c "$COMMAND"`,
		`command bash -c "${COMMAND}"`,
	}
	for _, data := range cases {
		t.Run(data, func(t *testing.T) {
			got := CountPythonInvocations([]byte(data))
			if got != -1 {
				t.Errorf("CountPythonInvocations(%q) = %d, want -1", data, got)
			}
		})
	}
}

// TestR13BashValueOptions covers the bash -c value-taking option
// matrix: -O, +O, --rcfile, --init-file all consume the next
// argument as their value, so the value is never the -c payload.
func TestR13BashValueOptions(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		{"-O extglob -c 'python3 x.py'", `bash -O extglob -c 'python3 x.py'`, 1},
		{"+O extglob -c 'python3 x.py'", `bash +O extglob -c 'python3 x.py'`, 1},
		{"--rcfile file -c 'python3 x.py'", `bash --rcfile file -c 'python3 x.py'`, 1},
		{"--init-file file -c 'python3 x.py'", `bash --init-file file -c 'python3 x.py'`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.data))
			if got != tc.want {
				t.Errorf("CountPythonInvocations(%q) = %d, want %d", tc.data, got, tc.want)
			}
		})
	}
}
