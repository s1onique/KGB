#!/usr/bin/env python3
"""
Verifier for ACT-UVB76-HULK03 latency series query boundary contracts.

This verifier checks:
1. Required test files exist
2. Fuzz targets exist with proper seed corpus
3. No unallowlisted t.Skip/t.Skipf in HULK03 tests
4. Makefile exposes hulk-uvb76-latency-gate
5. Gate commands include bounded fuzzing with -fuzztime
6. All HULK03 files are under 450 lines

Usage:
    python3 scripts/verify_uvb76_latency_series_contracts.py       # verify
    python3 scripts/verify_uvb76_latency_series_contracts.py --self-test  # self-test
"""

import argparse
import os
import re
import shutil
import sys
import tempfile

# Default paths (can be overridden for testing)
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def get_paths(repo_root):
    """Get all paths for a given repo root."""
    return {
        'repo_root': repo_root,
        'uvb76_server': os.path.join(repo_root, "uvb76", "server"),
        'makefile': os.path.join(repo_root, "Makefile"),
        'uvb76_makefile': os.path.join(repo_root, "uvb76", "Makefile"),
    }


# Required HULK03 files
REQUIRED_FILES = [
    "latency_series_query_contract_test.go",
    "latency_series_invariant_contract_test.go",
    "latency_series_fuzz_test.go",
    "latency_series_contract_helpers_test.go",
    "latency_series_query.go",
]

# Allowlisted patterns for t.Skip (e.g., platform-specific skips)
SKIP_ALLOWLIST = [
    r"runtime\.GOOS.*linux",  # Linux-only tests
    r"TestMain",  # Setup functions
    r"\.Skip\(`SKIP.*CI.*`\)",  # CI skip messages
]

# Max lines for HULK03 files
MAX_LINES = 450


def check_file_exists(path, name):
    """Check if a file exists."""
    if not os.path.exists(path):
        return False, f"missing required file: {name}"
    return True, None


def count_lines(path):
    """Count non-empty lines in a file. Returns (count, error)."""
    try:
        with open(path, 'r') as f:
            return sum(1 for line in f if line.strip()), None
    except Exception as e:
        return -1, str(e)


def check_file_line_limit(path, name):
    """Check if a file is under the line limit."""
    lines, err = count_lines(path)
    if err:
        return False, f"error reading {name}: {err}"
    if lines > MAX_LINES:
        return False, f"{name} has {lines} lines (max {MAX_LINES})"
    return True, None


def check_fuzz_targets(path):
    """Check fuzz test file has required targets and seed corpus."""
    errors = []
    
    with open(path, 'r') as f:
        content = f.read()
    
    # Check for FuzzLatencySeriesQueryParams
    if 'FuzzLatencySeriesQueryParams' not in content:
        errors.append("missing FuzzLatencySeriesQueryParams")
    
    # Check for FuzzLatencySeriesWindowStepRange
    if 'FuzzLatencySeriesWindowStepRange' not in content:
        errors.append("missing FuzzLatencySeriesWindowStepRange")
    
    # Check seed corpus has required cases
    required_seeds = [
        'range_seconds=0',
        'range_seconds=-1',
        'step_seconds=0',
        'window_seconds=0',
        '999999999',  # too-large value
        'abc',  # non-numeric
    ]
    
    for seed in required_seeds:
        if seed not in content:
            errors.append(f"missing seed corpus entry: {seed}")
    
    if errors:
        return False, "; ".join(errors)
    return True, None


def check_skip_allowlist(path):
    """Check for unallowlisted t.Skip calls."""
    with open(path, 'r') as f:
        lines = f.readlines()
    
    errors = []
    for i, line in enumerate(lines, 1):
        # Check for t.Skip or t.Skipf
        if re.search(r'\.Skip[f]?\(', line):
            # Check if allowlisted
            is_allowlisted = False
            for pattern in SKIP_ALLOWLIST:
                if re.search(pattern, line):
                    is_allowlisted = True
                    break
            
            if not is_allowlisted:
                errors.append(f"line {i}: {line.strip()}")
    
    if errors:
        return False, f"unallowlisted t.Skip found: {'; '.join(errors[:3])}"
    return True, None


def check_makefile_gate(makefile_path):
    """Check Makefile has proper hulk-uvb76-latency-gate target with bounded fuzzing."""
    with open(makefile_path, 'r') as f:
        content = f.read()
    
    errors = []
    
    # Check for hulk-uvb76-latency-gate target
    if 'hulk-uvb76-latency-gate:' not in content:
        return False, "missing hulk-uvb76-latency-gate target"
    
    # Extract the hulk-uvb76-latency-gate section
    gate_match = re.search(
        r'hulk-uvb76-latency-gate:(.*?)(?=\n\S|\Z)',
        content,
        re.DOTALL
    )
    if not gate_match:
        return False, "could not parse hulk-uvb76-latency-gate section"
    
    gate_section = gate_match.group(1)
    
    # Check for -race flag
    if '-race' not in gate_section:
        errors.append("missing -race flag in test commands")
    
    # Check for -fuzztime (REQUIRED - fuzzing must be bounded, not optional)
    if '-fuzztime=' not in gate_section:
        errors.append("missing -fuzztime= in fuzz commands (bounded fuzzing required)")
    
    # Check both fuzz targets are present
    if '-fuzz FuzzLatencySeriesQueryParams' not in gate_section:
        errors.append("missing -fuzz FuzzLatencySeriesQueryParams")
    if '-fuzz FuzzLatencySeriesWindowStepRange' not in gate_section:
        errors.append("missing -fuzz FuzzLatencySeriesWindowStepRange")
    
    # Check for soft-fail (not allowed - fuzz must gate)
    if '|| true' in gate_section:
        errors.append("soft-fail (|| true) not allowed - fuzz must gate")
    
    if errors:
        return False, "; ".join(errors)
    return True, None


def run_verification(repo_root=None):
    """Run full verification on a given repo root."""
    if repo_root is None:
        repo_root = REPO_ROOT
    
    paths = get_paths(repo_root)
    errors = []
    
    # Check required files exist
    for filename in REQUIRED_FILES:
        path = os.path.join(paths['uvb76_server'], filename)
        ok, err = check_file_exists(path, filename)
        if not ok:
            errors.append(err)
    
    # Check fuzz test has required targets
    fuzz_path = os.path.join(paths['uvb76_server'], "latency_series_fuzz_test.go")
    if os.path.exists(fuzz_path):
        ok, err = check_fuzz_targets(fuzz_path)
        if not ok:
            errors.append(f"fuzz test: {err}")
    
    # Check line limits
    for filename in REQUIRED_FILES:
        path = os.path.join(paths['uvb76_server'], filename)
        if os.path.exists(path):
            ok, err = check_file_line_limit(path, filename)
            if not ok:
                errors.append(err)
    
    # Check skip allowlist in contract tests
    contract_files = [
        "latency_series_query_contract_test.go",
        "latency_series_invariant_contract_test.go",
        "latency_series_fuzz_test.go",
    ]
    for filename in contract_files:
        path = os.path.join(paths['uvb76_server'], filename)
        if os.path.exists(path):
            ok, err = check_skip_allowlist(path)
            if not ok:
                errors.append(f"{filename}: {err}")
    
    # Check Makefile gate
    makefile_path = paths['makefile'] if os.path.exists(paths['makefile']) else paths['uvb76_makefile']
    if os.path.exists(makefile_path):
        ok, err = check_makefile_gate(makefile_path)
        if not ok:
            errors.append(f"Makefile gate: {err}")
    else:
        errors.append("Makefile not found")
    
    return errors


def run_self_test():
    """Run self-test mode using temp fixtures (no working tree mutation)."""
    errors = []
    
    with tempfile.TemporaryDirectory() as tmpdir:
        # Create minimal uvb76/server structure
        server_dir = os.path.join(tmpdir, "uvb76", "server")
        os.makedirs(server_dir, exist_ok=True)
        
        # === Test 1: Missing fuzz file should fail ===
        # Create valid Makefile
        makefile_path = os.path.join(tmpdir, "Makefile")
        with open(makefile_path, 'w') as f:
            f.write("""
hulk-uvb76-latency-gate:
	@echo "test"
""")
        
        # Run verification - should fail on missing fuzz file
        errs = run_verification(tmpdir)
        if not errs:
            errors.append("self-test failed: missing fuzz file should fail verification")
        elif not any("latency_series_fuzz_test.go" in e for e in errs):
            errors.append(f"self-test failed: missing fuzz file not detected, got: {errs}")
        
        # === Test 2: Missing FuzzLatencySeriesQueryParams should fail ===
        fuzz_path = os.path.join(server_dir, "latency_series_fuzz_test.go")
        with open(fuzz_path, 'w') as f:
            f.write("package server\n\n")
            f.write("func FuzzOther(f *testing.F) {}\n")
        
        errs = run_verification(tmpdir)
        if not errs:
            errors.append("self-test failed: missing FuzzLatencySeriesQueryParams should fail")
        elif not any("FuzzLatencySeriesQueryParams" in e for e in errs):
            errors.append(f"self-test failed: missing FuzzLatencySeriesQueryParams not detected, got: {errs}")
        
        # === Test 3: Missing -fuzztime should fail ===
        with open(fuzz_path, 'w') as f:
            f.write("""
package server

import "testing"

func FuzzLatencySeriesQueryParams(f *testing.F) {
    f.Add("test")
    f.Fuzz(func(t *testing.T, s string) {})
}

func FuzzLatencySeriesWindowStepRange(f *testing.F) {
    f.Add("test")
    f.Fuzz(func(t *testing.T, s string) {})
}
""")
        # Create Makefile without -fuzztime
        with open(makefile_path, 'w') as f:
            f.write("""
hulk-uvb76-latency-gate:
	@cd uvb76 && go test -fuzz FuzzLatencySeriesQueryParams
""")
        
        errs = run_verification(tmpdir)
        if not errs:
            errors.append("self-test failed: missing -fuzztime should fail")
        elif not any("-fuzztime=" in e for e in errs):
            errors.append(f"self-test failed: missing -fuzztime not detected, got: {errs}")
        
        # === Test 4: Soft-fail should fail ===
        with open(makefile_path, 'w') as f:
            f.write("""
hulk-uvb76-latency-gate:
	@cd uvb76 && go test -fuzz FuzzLatencySeriesQueryParams -fuzztime=10s || true
""")
        
        errs = run_verification(tmpdir)
        if not errs:
            errors.append("self-test failed: soft-fail should fail")
        elif not any("|| true" in e for e in errs):
            errors.append(f"self-test failed: soft-fail not detected, got: {errs}")
        
        # === Test 5: Valid fixture should pass ===
        # Create full valid structure
        for filename in REQUIRED_FILES:
            path = os.path.join(server_dir, filename)
            with open(path, 'w') as f:
                if filename.endswith('_test.go'):
                    f.write(f"package server\n\n// {filename}\n")
                    f.write("func TestPlaceholder(t *testing.T) {}\n")
                else:
                    f.write(f"package server\n\n// {filename}\n")
        
        # Write valid fuzz file
        with open(fuzz_path, 'w') as f:
            f.write("""
package server

import "testing"

func FuzzLatencySeriesQueryParams(f *testing.F) {
    f.Add("range_seconds=0")
    f.Add("range_seconds=-1")
    f.Add("step_seconds=0")
    f.Add("window_seconds=0")
    f.Add("999999999")
    f.Add("abc")
    f.Fuzz(func(t *testing.T, s string) {})
}

func FuzzLatencySeriesWindowStepRange(f *testing.F) {
    f.Add("range_seconds=0&step_seconds=0&window_seconds=0")
    f.Fuzz(func(t *testing.T, s string) {})
}
""")
        
        with open(makefile_path, 'w') as f:
            f.write("""
hulk-uvb76-latency-gate:
	@cd uvb76 && go test -race -v ./state/... ./server/... -run 'LatencySeries|FuzzLatencySeries'
	@cd uvb76 && go test ./server/... -run '^$$' -fuzz FuzzLatencySeriesQueryParams -fuzztime=10s
	@cd uvb76 && go test ./server/... -run '^$$' -fuzz FuzzLatencySeriesWindowStepRange -fuzztime=10s
	@python3 scripts/verify_uvb76_latency_series_contracts.py
	@python3 scripts/verify_uvb76_latency_series_contracts.py --self-test
""")
        
        errs = run_verification(tmpdir)
        if errs:
            errors.append(f"self-test failed: valid fixture should pass, got: {errs}")
    
    return errors


def main():
    parser = argparse.ArgumentParser(description="Verify UVB-76 latency series contracts")
    parser.add_argument("--self-test", action="store_true", help="Run self-test mode")
    args = parser.parse_args()
    
    if args.self_test:
        errors = run_self_test()
        if errors:
            print("SELF-TEST FAILED:")
            for err in errors:
                print(f"  - {err}")
            sys.exit(1)
        print("SELF-TEST PASSED")
        sys.exit(0)
    
    errors = run_verification()
    
    if errors:
        print("VERIFICATION FAILED:")
        for err in errors:
            print(f"  - {err}")
        sys.exit(1)
    
    print("VERIFICATION PASSED")
    sys.exit(0)


if __name__ == "__main__":
    main()
