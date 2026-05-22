#!/usr/bin/env python3
"""coverage_gate.py — Real line coverage gate using kcov

Orchestrates kcov coverage collection with diagnostic retries.
Uses existing parsers from extract_kcov_line_coverage.py.

Behavior:
- Requires kcov tool (configurable via KCOV_BIN env var)
- Builds Zig test artifact
- Verifies test binary runs real tests
- Runs DWARF diagnostics
- Attempts kcov modes (standard, verify, include-pattern)
- Enforces coverage threshold (default 60%)
- Fails if no real tovarisch/src coverage found
"""

import sys
import os
import subprocess
import shutil
from pathlib import Path

# Timeout for each kcov attempt (prevents indefinite hangs)
KCOV_ATTEMPT_TIMEOUT_SECONDS = int(os.environ.get("KCOV_ATTEMPT_TIMEOUT_SECONDS", "90"))

# Whether to run the experimental --exit-first-process diagnostic
KCOV_ENABLE_EXIT_FIRST_PROCESS = os.environ.get("KCOV_ENABLE_EXIT_FIRST_PROCESS", "0") == "1"

# Configurable kcov binary (allows CI to use zig-kcov fork)
KCOV_BIN = os.environ.get("KCOV_BIN", "kcov")

# Resolve script directory for imports
SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parent
TOVARISCH_DIR = REPO_ROOT / "tovarisch"

# Import parsers and diagnostics
sys.path.insert(0, str(SCRIPT_DIR))
from extract_kcov_line_coverage import find_coverage_dir
from kcov_parsers import (
    parse_coverage_json,
    parse_cobertura_xml,
    parse_codecov_json,
)
from coverage_diagnostics import (
    print_dwarf_diagnostics,
    check_dwarf_has_paths,
    print_report_diagnostics,
)


COVERAGE_THRESHOLD = float(os.environ.get("COVERAGE_THRESHOLD", "60"))


def resolve_tool(tool: str) -> str:
    """Resolve a tool specification to an executable path.
    
    Handles both:
    - Bare command names (e.g., "kcov") -> uses shutil.which()
    - Explicit paths (e.g., "/usr/local/bin/kcov", "./tools/zig-kcov") -> validates directly
    
    Exits with clear error if tool is not found or not executable.
    """
    candidate = Path(tool)
    
    # Explicit path: has parent directory (not just a bare filename)
    if candidate.parent != Path("."):
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return str(candidate.resolve())
        print(f"[FAIL] coverage: KCOV_BIN={tool} is not a valid executable", file=sys.stderr)
        sys.exit(1)
    
    # Bare command name: use shutil.which()
    resolved = shutil.which(tool)
    if resolved:
        return resolved
    
    print(f"[FAIL] coverage: KCOV_BIN={tool} not found in PATH", file=sys.stderr)
    print("[INFO] coverage: install kcov or set KCOV_BIN=/path/to/zig-kcov", file=sys.stderr)
    sys.exit(1)


def run_command(cmd: list[str], cwd: Path | None = None, capture: bool = False,
                check: bool = True) -> subprocess.CompletedProcess:
    """Run a command, optionally capture output."""
    result = subprocess.run(
        cmd,
        cwd=cwd,
        capture_output=capture,
        text=True,
        check=False
    )
    if check and result.returncode != 0:
        print(f"[ERROR] Command failed: {' '.join(cmd)}", file=sys.stderr)
        if result.stderr:
            print(f"[ERROR] stderr: {result.stderr}", file=sys.stderr)
        sys.exit(1)
    return result


def require_tool(tool_path: str) -> str:
    """Check that a tool is available and print version."""
    result = subprocess.run([tool_path, "--version"], capture_output=True, text=True)
    version = result.stdout.split("\n")[0] if result.stdout else "unknown"
    print(f"[coverage] {Path(tool_path).name} version: {version}")
    return tool_path


def build_test_binary() -> Path:
    """Build the Zig test artifact for kcov."""
    print("[coverage] building test artifact for kcov")
    run_command(["zig", "build", "test-bin", "-Doptimize=Debug"], cwd=TOVARISCH_DIR)
    binary = TOVARISCH_DIR / "zig-out" / "bin" / "tovarisch-test"
    if not binary.exists():
        print(f"[FAIL] coverage: test binary not found at {binary}", file=sys.stderr)
        sys.exit(1)
    print(f"[coverage] test binary: {binary}")
    return binary


def verify_real_tests(binary: Path) -> None:
    """Run the test binary to verify it executes real tests."""
    print("[coverage] verifying test binary executes real tests...")
    result = subprocess.run([str(binary)], capture_output=True, text=True, check=False)
    if result.returncode != 0:
        print(f"[FAIL] coverage: test binary failed before kcov", file=sys.stderr)
        print(result.stderr, file=sys.stderr)
        sys.exit(1)
    if "All 0 tests passed" in result.stdout:
        print("[FAIL] coverage: test binary contains zero tests", file=sys.stderr)
        sys.exit(1)
    print("[coverage] test binary confirmed: real tests found")


def kcov_supports_include_pattern(kcov_cmd: str) -> bool:
    """Check if kcov supports --include-pattern."""
    result = subprocess.run(
        [kcov_cmd, "--help"],
        capture_output=True, text=True
    )
    return "--include-pattern" in result.stdout


def run_kcov_attempt(kcov_cmd: str, binary: Path, output_dir: Path, mode: str, 
                   extra_args: list[str] | None = None) -> tuple[bool, bool]:
    """Run kcov and return (success, timed_out).
    
    Returns:
        (True, False) if kcov succeeded
        (False, True) if kcov timed out
        (False, False) if kcov failed with non-timeout error
    """
    print(f"[coverage] running kcov ({mode}) [timeout={KCOV_ATTEMPT_TIMEOUT_SECONDS}s]")
    
    args = [kcov_cmd, "--exclude-path=zig-cache", "--exclude-path=.git"]
    if extra_args:
        args.extend(extra_args)
    args.extend([str(output_dir), str(binary)])
    
    # Run kcov with timeout - suppress test output to stderr, capture only errors
    try:
        result = subprocess.run(args, capture_output=True, text=True, 
                              timeout=KCOV_ATTEMPT_TIMEOUT_SECONDS)
    except subprocess.TimeoutExpired:
        print(f"[coverage] kcov ({mode}) timed out after {KCOV_ATTEMPT_TIMEOUT_SECONDS}s")
        return (False, True)
    
    if result.returncode != 0:
        print(f"[coverage] kcov ({mode}) exited with error")
        print(f"[coverage] stderr: {result.stderr[:500] if result.stderr else 'none'}")
        return (False, False)
    
    print(f"[coverage] kcov ({mode}) completed")
    return (True, False)


def parse_coverage(coverage_dir: Path) -> float | None:
    """Parse coverage from kcov output directory. Returns percentage or None."""
    kcov_dir = find_coverage_dir(str(coverage_dir))
    if kcov_dir is None:
        print(f"[coverage] kcov output directory not found in {coverage_dir}", file=sys.stderr)
        return None
    
    report_dir = Path(kcov_dir)
    
    parsers = [
        (report_dir / "coverage.json", parse_coverage_json),
        (report_dir / "cobertura.xml", parse_cobertura_xml),
        (report_dir / "cov.xml", parse_cobertura_xml),
        (report_dir / "codecov.json", parse_codecov_json),
    ]
    
    for path, parser in parsers:
        if path.exists():
            result = parser(path)
            if result is not None:
                print(f"[coverage] parsed coverage: {result:.2f}%")
                return result
    
    print(f"[coverage] no parser produced tovarisch/src coverage", file=sys.stderr)
    return None


def compare_threshold(coverage: float) -> bool:
    """Compare coverage against threshold. Returns True if above threshold."""
    return coverage >= COVERAGE_THRESHOLD


def main() -> int:
    """Main coverage gate logic."""
    print(f"[coverage] using kcov for real line coverage")
    print(f"[coverage] KCOV_BIN={KCOV_BIN} (requested)")
    
    # Check for kcov availability (before resolving)
    if os.environ.get("ALLOW_MISSING_KCOV") == "1":
        print("[INFO] coverage: kcov not found, skipping (ALLOW_MISSING_KCOV=1)")
        return 0
    
    # Resolve KCOV_BIN to actual executable path
    KCOV_CMD = resolve_tool(KCOV_BIN)
    print(f"[coverage] KCOV_CMD={KCOV_CMD} (resolved)")
    
    require_tool(KCOV_CMD)
    
    # Build and verify test binary
    binary = build_test_binary()
    verify_real_tests(binary)
    
    # Run DWARF diagnostics
    dwarf_had_paths = print_dwarf_diagnostics(binary)
    
    # KCOV RUN — With diagnostic retries
    attempts = [
        ("standard", []),
        ("verify", ["--verify"]),
    ]
    
    if kcov_supports_include_pattern(KCOV_CMD):
        attempts.append(("include-pattern", ["--include-pattern=tovarisch/src"]))
    
    # exit-first-process is opt-in only (experimental, can hang)
    if KCOV_ENABLE_EXIT_FIRST_PROCESS:
        attempts.append(("exit-first-process", ["--exit-first-process"]))
    
    final_coverage = None
    kcov_success = 0
    kcov_timeout_count = 0
    
    for i, (label, extra_args) in enumerate(attempts, 1):
        print("")
        print(f"[coverage] === Attempt {i}/{len(attempts)}: kcov with {label} ===")
        
        output_dir = TOVARISCH_DIR / "zig-out" / f"coverage-{label}"
        if output_dir.exists():
            shutil.rmtree(output_dir)
        output_dir.mkdir(parents=True, exist_ok=True)
        
        success, timed_out = run_kcov_attempt(KCOV_CMD, binary, output_dir, label, extra_args)
        
        if timed_out:
            kcov_timeout_count += 1
            print(f"[coverage] {label} timed out (counted as failure)")
            print_report_diagnostics(output_dir, find_coverage_dir)
        elif success:
            coverage = parse_coverage(output_dir)
            if coverage is not None:
                final_coverage = coverage
                kcov_success = i
                print(f"[coverage] SUCCESS: {label} produced coverage: {coverage:.2f}%")
                break
            else:
                print(f"[coverage] {label} did not produce real coverage")
                print_report_diagnostics(output_dir, find_coverage_dir)
        else:
            print(f"[coverage] {label} failed to run")
            print_report_diagnostics(output_dir, find_coverage_dir)
    
    print("")
    print(f"[coverage] kcov attempts: {len(attempts)} total, {kcov_success} succeeded, {kcov_timeout_count} timed out")
    
    if kcov_success == 0:
        print("")
        print("[FAIL] coverage: All kcov variants emitted empty reports.", file=sys.stderr)
        if dwarf_had_paths:
            print(f"[FAIL] coverage: DWARF has project paths, but {KCOV_CMD} emitted empty reports.", file=sys.stderr)
            print(f"[INFO] coverage: Tried kcov binary: {KCOV_CMD}", file=sys.stderr)
            print("[INFO] coverage: This may indicate a Zig DWARF + Linux kcov incompatibility.", file=sys.stderr)
            print("[INFO] coverage: Consider trying roc-lang/zig-kcov (github.com/roc-lang/zig-kcov)", file=sys.stderr)
        else:
            print("[INFO] coverage: DWARF also did not find project source paths.", file=sys.stderr)
        # Dynamic message: list only the modes that were actually attempted
        mode_labels = [label for label, _ in attempts]
        modes_str = ", ".join(mode_labels)
        print(f"[INFO] coverage: Tried configured modes: {modes_str}.", file=sys.stderr)
        if not KCOV_ENABLE_EXIT_FIRST_PROCESS:
            print("[INFO] coverage: Set KCOV_ENABLE_EXIT_FIRST_PROCESS=1 to try --exit-first-process.", file=sys.stderr)
        print("[INFO] coverage: Options:", file=sys.stderr)
        print("[INFO] coverage:   1. Pin kcov to a version known to work with Zig 0.16 DWARF", file=sys.stderr)
        print("[INFO] coverage:   2. Try roc-lang/zig-kcov fork: KCOV_BIN=/path/to/zig-kcov make coverage", file=sys.stderr)
        print("[INFO] coverage:   3. Verify Linux kernel has breakpoints enabled (ptrace_scope)", file=sys.stderr)
        print("[INFO] coverage:   4. Mark Linux kcov as accepted tooling risk until Zig-native coverage stabilizes", file=sys.stderr)
        return 1
    
    print("")
    print(f"[INFO] coverage: line coverage {final_coverage:.2f}%")
    print(f"[INFO] coverage: threshold {COVERAGE_THRESHOLD:.2f}%")
    
    if not compare_threshold(final_coverage):
        print(f"[FAIL] coverage: line coverage {final_coverage:.2f}% below threshold {COVERAGE_THRESHOLD:.2f}%", file=sys.stderr)
        return 1
    
    print("[PASS] coverage: real line coverage gate passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
