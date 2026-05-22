#!/usr/bin/env python3
"""coverage_gate.py — Real line coverage gate using kcov

Orchestrates kcov coverage collection with diagnostic retries.
Uses existing parsers from extract_kcov_line_coverage.py.

Behavior:
- Requires kcov tool
- Builds Zig test artifact
- Verifies test binary runs real tests
- Runs DWARF diagnostics
- Attempts up to 4 kcov modes if standard fails
- Enforces coverage threshold (default 60%)
- Fails if no real tovarisch/src coverage found
"""

import sys
import os
import subprocess
from pathlib import Path

# Timeout for each kcov attempt (prevents indefinite hangs)
KCOV_ATTEMPT_TIMEOUT_SECONDS = int(os.environ.get("KCOV_ATTEMPT_TIMEOUT_SECONDS", "90"))

# Whether to run the experimental --exit-first-process diagnostic
KCOV_ENABLE_EXIT_FIRST_PROCESS = os.environ.get("KCOV_ENABLE_EXIT_FIRST_PROCESS", "0") == "1"

# Resolve script directory for imports
SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parent
TOVARISCH_DIR = REPO_ROOT / "tovarisch"

# Import the coverage parser directly
sys.path.insert(0, str(SCRIPT_DIR))
from extract_kcov_line_coverage import find_coverage_dir
from kcov_parsers import (
    parse_coverage_json,
    parse_cobertura_xml,
    parse_codecov_json,
)


COVERAGE_THRESHOLD = float(os.environ.get("COVERAGE_THRESHOLD", "60"))


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


def require_tool(tool: str) -> str:
    """Check that a tool is available."""
    path = subprocess.run(["which", tool], capture_output=True, text=True)
    if path.returncode != 0:
        print(f"[FAIL] coverage: {tool} is required but not found", file=sys.stderr)
        print(f"[INFO] coverage: install {tool} or set ALLOW_MISSING_KCOV=1 for bypass", file=sys.stderr)
        sys.exit(1)
    result = subprocess.run([tool, "--version"], capture_output=True, text=True)
    version = result.stdout.split("\n")[0] if result.stdout else "unknown"
    print(f"[coverage] {tool} version: {version}")
    return tool


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


def print_dwarf_diagnostics(binary: Path) -> None:
    """Run DWARF diagnostics to check binary for project source paths."""
    print("")
    print("[DWARF-DIAGNOSTIC] === Binary analysis for source mapping ===")
    
    # File type
    result = subprocess.run(["file", str(binary)], capture_output=True, text=True)
    print(f"[DWARF-DIAGNOSTIC] file type:")
    print(f"[DWARF-DIAGNOSTIC] {result.stdout.strip()}")
    
    # Debug sections with readelf
    readelf = subprocess.run(["which", "readelf"], capture_output=True, text=True)
    if readelf.returncode == 0:
        result = subprocess.run(
            ["readelf", "-S", str(binary)],
            capture_output=True, text=True
        )
        debug_lines = [l for l in result.stdout.split("\n") 
                      if "debug" in l.lower() or "symtab" in l.lower()]
        if debug_lines:
            print(f"[DWARF-DIAGNOSTIC] debug sections:")
            for line in debug_lines[:10]:
                print(f"[DWARF-DIAGNOSTIC] {line.strip()}")
        else:
            print("[DWARF-DIAGNOSTIC] no debug/symtab sections found")
    else:
        print("[DWARF-DIAGNOSTIC] readelf not available — skipping section listing")
    
    # Check for project paths in DWARF line tables
    print("[DWARF-DIAGNOSTIC] checking for project source paths in DWARF line tables...")
    
    project_paths = []
    
    # Try readelf first
    readelf = subprocess.run(["which", "readelf"], capture_output=True, text=True)
    if readelf.returncode == 0:
        result = subprocess.run(
            ["readelf", "--debug-dump=decodedline", str(binary)],
            capture_output=True, text=True
        )
        if result.returncode == 0:
            import re
            patterns = [
                r'tovari?sch/src',
                r'src/(main|cli|status|http)',
                r'commands\.zig',
                r'status\.zig'
            ]
            combined = "|".join(patterns)
            matches = re.findall(combined, result.stdout, re.IGNORECASE)
            # Deduplicate while preserving order
            seen = set()
            for m in matches:
                if m.lower() not in seen:
                    seen.add(m.lower())
                    project_paths.append(m)
            project_paths = project_paths[:50]  # Cap at 50
    
    # Fallback to llvm-dwarfdump if readelf didn't find anything
    if not project_paths:
        llvm_dwarfdump = subprocess.run(["which", "llvm-dwarfdump"], capture_output=True, text=True)
        if llvm_dwarfdump.returncode == 0:
            result = subprocess.run(
                ["llvm-dwarfdump", "--debug-line", str(binary)],
                capture_output=True, text=True
            )
            if result.returncode == 0:
                import re
                patterns = [
                    r'tovari?sch/src',
                    r'src/(main|cli|status|http)',
                    r'commands\.zig',
                    r'status\.zig'
                ]
                combined = "|".join(patterns)
                matches = re.findall(combined, result.stdout, re.IGNORECASE)
                seen = set()
                for m in matches:
                    if m.lower() not in seen:
                        seen.add(m.lower())
                        project_paths.append(m)
                project_paths = project_paths[:50]
    
    if project_paths:
        print("[DWARF-DIAGNOSTIC] FOUND project paths in DWARF line tables:")
        for path in project_paths[:50]:
            print(f"[DWARF-DIAGNOSTIC] {path}")
        print(f"[DWARF-DIAGNOSTIC] project path match count: {len(project_paths)}")
    else:
        print("[DWARF-DIAGNOSTIC] WARNING: no project source paths found in DWARF line tables")
        print("[DWARF-DIAGNOSTIC] This suggests Zig compiled the tests without debug-line info")
        # Show sample of what is in the DWARF
        if readelf.returncode == 0:
            result = subprocess.run(
                ["readelf", "--debug-dump=decodedline", str(binary)],
                capture_output=True, text=True
            )
            if result.stdout:
                lines = result.stdout.split("\n")[:20]
                print("[DWARF-DIAGNOSTIC] Sample of actual DWARF paths (first 20 lines):")
                for line in lines:
                    print(f"[DWARF-DIAGNOSTIC] {line}")
        else:
            print("[DWARF-DIAGNOSTIC] readelf not available for sample")
    
    print("[DWARF-DIAGNOSTIC] === End binary analysis ===")
    print("")


def kcov_supports_include_pattern() -> bool:
    """Check if kcov supports --include-pattern."""
    result = subprocess.run(
        ["kcov", "--help"],
        capture_output=True, text=True
    )
    return "--include-pattern" in result.stdout


def run_kcov_attempt(binary: Path, output_dir: Path, mode: str, 
                   extra_args: list[str] | None = None) -> tuple[bool, bool]:
    """Run kcov and return (success, timed_out).
    
    Returns:
        (True, False) if kcov succeeded
        (False, True) if kcov timed out
        (False, False) if kcov failed with non-timeout error
    """
    print(f"[coverage] running kcov ({mode}) [timeout={KCOV_ATTEMPT_TIMEOUT_SECONDS}s]")
    
    args = ["kcov", "--exclude-path=zig-cache", "--exclude-path=.git"]
    if extra_args:
        args.extend(extra_args)
    args.extend([str(output_dir), str(binary)])
    
    # Run kcov with timeout - suppress test output to stderr, capture only errors
    try:
        result = subprocess.run(args, capture_output=True, text=True, 
                              timeout=KCOV_ATTEMPT_TIMEOUT_SECONDS)
    except subprocess.TimeoutExpired:
        print(f"[coverage] kcov ({mode}) timed out after {KCOV_ATTEMPT_TIMEOUT_SECONDS}s")
        # Capture whatever was written before timeout
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
    
    # Report files live under the kcov subdir, not the parent coverage_dir
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


def print_report_diagnostics(coverage_dir: Path) -> None:
    """Print diagnostic info for a failed kcov attempt."""
    print("")
    print("[coverage-debug] Report file sizes:")
    
    reports = ["coverage.json", "cobertura.xml", "cov.xml", "codecov.json"]
    for name in reports:
        path = coverage_dir / name
        if path.exists():
            size = path.stat().st_size
            print(f"  [coverage-debug]   {name}: {size} bytes")
        else:
            print(f"  [coverage-debug]   {name}: not found")
    
    # Print sample of coverage.json
    cj = coverage_dir / "coverage.json"
    if cj.exists():
        print("")
        print("[coverage-debug] coverage.json sample (first 20 lines):")
        lines = cj.read_text().split("\n")[:20]
        for line in lines:
            print(f"[coverage-debug] {line}")
    
    # Print sample of cobertura.xml
    cx = coverage_dir / "cobertura.xml"
    if cx.exists():
        print("")
        print("[coverage-debug] cobertura.xml sample (first 20 lines):")
        lines = cx.read_text().split("\n")[:20]
        for line in lines:
            print(f"[coverage-debug] {line}")
    
    # Cap file tree output
    print("")
    print("[coverage-debug] kcov output directory contents (capped at 100 files):")
    files = list(coverage_dir.rglob("*"))
    files = [f for f in files if f.is_file()]
    fcount = len(files)
    print(f"  [coverage-debug] total files: {fcount}")
    
    # Show capped list
    to_show = sorted(files, key=lambda f: str(f))[:100]
    for f in to_show:
        rel = f.relative_to(coverage_dir)
        print(f"  [coverage-debug] file: {rel}")
    
    if fcount > 100:
        print(f"  [coverage-debug] ... and {fcount - 100} more files (output capped)")


def compare_threshold(coverage: float) -> bool:
    """Compare coverage against threshold. Returns True if above threshold."""
    return coverage >= COVERAGE_THRESHOLD


def main() -> int:
    """Main coverage gate logic."""
    print("[coverage] using kcov for real line coverage")
    
    # Check for kcov
    if subprocess.run(["which", "kcov"], capture_output=True).returncode != 0:
        if os.environ.get("ALLOW_MISSING_KCOV") == "1":
            print("[INFO] coverage: kcov not found, skipping (ALLOW_MISSING_KCOV=1)")
            return 0
        print("[FAIL] coverage: kcov is required for real coverage gate", file=sys.stderr)
        print("[INFO] coverage: install kcov or set ALLOW_MISSING_KCOV=1 for bypass", file=sys.stderr)
        return 1
    
    require_tool("kcov")
    
    # Build and verify test binary
    binary = build_test_binary()
    verify_real_tests(binary)
    
    # Run DWARF diagnostics
    print_dwarf_diagnostics(binary)
    
    # KCOV RUN — With diagnostic retries
    attempts = [
        ("standard", []),
        ("verify", ["--verify"]),
    ]
    
    if kcov_supports_include_pattern():
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
            import shutil
            shutil.rmtree(output_dir)
        output_dir.mkdir(parents=True, exist_ok=True)
        
        success, timed_out = run_kcov_attempt(binary, output_dir, label, extra_args)
        
        if timed_out:
            kcov_timeout_count += 1
            print(f"[coverage] {label} timed out (counted as failure)")
            print_report_diagnostics(output_dir)
        elif success:
            coverage = parse_coverage(output_dir)
            if coverage is not None:
                final_coverage = coverage
                kcov_success = i
                print(f"[coverage] SUCCESS: {label} produced coverage: {coverage:.2f}%")
                break
            else:
                print(f"[coverage] {label} did not produce real coverage")
                print_report_diagnostics(output_dir)
        else:
            print(f"[coverage] {label} failed to run")
            print_report_diagnostics(output_dir)
    
    print("")
    print(f"[coverage] kcov attempts: {len(attempts)} total, {kcov_success} succeeded, {kcov_timeout_count} timed out")
    
    if kcov_success == 0:
        print("")
        print("[FAIL] coverage: All kcov variants emitted empty reports.", file=sys.stderr)
        print("[INFO] coverage: DWARF has project paths, but kcov failed to collect coverage.", file=sys.stderr)
        # Dynamic message: list only the modes that were actually attempted
        mode_labels = [label for label, _ in attempts]
        modes_str = ", ".join(mode_labels)
        print(f"[INFO] coverage: Tried configured modes: {modes_str}.", file=sys.stderr)
        if not KCOV_ENABLE_EXIT_FIRST_PROCESS:
            print("[INFO] coverage: Set KCOV_ENABLE_EXIT_FIRST_PROCESS=1 to try --exit-first-process.", file=sys.stderr)
        print("[INFO] coverage: Consider:", file=sys.stderr)
        print("[INFO] coverage:   1. Pin kcov to a version known to work with Zig 0.16 DWARF", file=sys.stderr)
        print("[INFO] coverage:   2. Try roc-lang/zig-kcov fork (github.com/roc-lang/zig-kcov)", file=sys.stderr)
        print("[INFO] coverage:   3. Verify Linux kernel has breakpoints enabled (ptrace_scope)", file=sys.stderr)
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
