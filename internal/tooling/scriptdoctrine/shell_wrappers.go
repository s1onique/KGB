package scriptdoctrine

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Wrapper classification.
//
// The scriptdoctrine parser strips wrapper arguments (sudo, env,
// command) from the front of a CallExpr's Args before deciding
// whether the first remaining word is a Python interpreter. The
// option tables for each wrapper are kept here, separate from the
// AST walker in shell_command_parser.go, so that this policy file
// stays narrowly focused on per-wrapper option handling and does
// not bloat past the LLM-friendliness hard limit.
//
// Wrapper options come in two flavours:
//
//   - value-less flags (no argument follows): -i, -E, --help ...
//   - value flags (the next argument is consumed too): -u, --user,
//     -g, --group ...
//
// Some flags always carry their value as a separate argument
// (e.g. `sudo -u root`); others can be written as `sudo -uroot`
// (gnu-style = no special parse, just a different spelling of the
// same shape). The verifier must consume the value when present.
//
// R11.3 keeps the explicit wrapper option tables rather than
// inferring from spelling: --stdin is a no-value flag for sudo,
// --user takes a value. Spelling-based inference was a source of
// false negatives in earlier reviews.

// envValueFlag reports whether the literal flag text for the env
// wrapper takes a separate value argument. Flags NOT listed here
// are treated as value-less.
func envValueFlag(flag string) bool {
	switch flag {
	case "-u", "--unset",
		"-C", "--chdir",
		"-S", "--split-string",
		"-V", "--version",
		"--default-signal", "--ignore-signal",
		"--block-signal", "--sig-proxy",
		"-T", "--tmpdir",
		"--path":
		return true
	}
	return false
}

// sudoValueFlag reports whether the literal flag text for the sudo
// wrapper takes a separate value argument. The verifier treats
// `sudo -u root python3 x.py` and `sudo -uroot python3 x.py` as
// the same case: -u ALWAYS requires a value, otherwise the next
// token would be parsed as the executable.
func sudoValueFlag(flag string) bool {
	switch flag {
	case "-u", "--user",
		"-g", "--group",
		"-h", "--host",
		"-p", "--prompt",
		"-D", "--chdir",
		"--type":
		return true
	}
	return false
}

// sudoNoValueFlag records value-less flags that we MUST consume
// without treating the next token as their value. `-S` and `-A`
// (added in R11.3) live here, alongside `-E`, `-H`, and `-n`.
func sudoNoValueFlag(flag string) bool {
	switch flag {
	case "-S", "--stdin", "-A", "--askpass",
		"-E", "-H", "-n",
		"-i", "-s", "-b",
		"-k", "-K",
		"-l", "-v":
		return true
	}
	return false
}

// stripWrapperArgs removes an `env` or `sudo` wrapper from the
// start of args along with its options and (for env) any leading
// NAME=VALUE assignments. Returns the slice shifted past the
// wrapper.
//
// Recognised wrappers: env, /usr/bin/env, sudo. Anything else
// leaves args untouched (caller decides how to classify).
func stripWrapperArgs(args []*syntax.Word) []*syntax.Word {
	for {
		if len(args) == 0 {
			return args
		}
		first := args[0].Lit()
		switch first {
		case "env", "/usr/bin/env":
			args = args[1:]
			for len(args) > 0 {
				lit := args[0].Lit()
				if lit == "" {
					args = args[1:]
					continue
				}
				if lit == "--" {
					args = args[1:]
					break
				}
				if envValueFlag(lit) {
					args = args[1:]
					if len(args) > 0 && !strings.HasPrefix(args[0].Lit(), "-") {
						args = args[1:]
					}
					continue
				}
				if strings.HasPrefix(lit, "-") {
					args = args[1:]
					continue
				}
				if strings.Contains(lit, "=") {
					args = args[1:]
					continue
				}
				break
			}
		case "sudo":
			args = args[1:]
			for len(args) > 0 {
				lit := args[0].Lit()
				if lit == "" {
					args = args[1:]
					continue
				}
				if lit == "--" {
					args = args[1:]
					break
				}
				if sudoValueFlag(lit) {
					args = args[1:]
					if len(args) > 0 && !strings.HasPrefix(args[0].Lit(), "-") {
						args = args[1:]
					}
					continue
				}
				if sudoNoValueFlag(lit) || strings.HasPrefix(lit, "-") {
					args = args[1:]
					continue
				}
				break
			}
		default:
			return args
		}
	}
}

// stripCommandPrefixes strips recognised command prefixes
// (command, exec) from the start of args. The second return is
// true when the call should be classified as a lookup (e.g.
// `command -v python3`); false when the remainder is a real
// command.
//
// Preserved behaviours:
//   - `command -v` / `command -V` / `command --help` are lookups.
//   - `command --` is the end-of-options marker; both the marker
//     and the `command` prefix are dropped so the remainder is
//     treated as a real command.
func stripCommandPrefixes(args []*syntax.Word) ([]*syntax.Word, bool) {
	for len(args) > 0 {
		lit := args[0].Lit()
		if !isCommandPrefixWord(lit) {
			break
		}
		if lit == "command" && len(args) >= 2 {
			arg := args[1].Lit()
			if arg == "-v" || arg == "-V" || arg == "--help" {
				return nil, true
			}
			if arg == "--" {
				return args[2:], false
			}
			if strings.HasPrefix(arg, "-") {
				return nil, true
			}
		}
		args = args[1:]
	}
	return args, false
}
