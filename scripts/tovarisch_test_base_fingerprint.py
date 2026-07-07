#!/usr/bin/env python3
"""
Diagnostic fingerprint for `zig build test-base`.

Captures environment context to explain pass/skip/fail profile divergence
between CI and local runs.

Usage:
    python scripts/tovarisch_test_base_fingerprint.py --seed 0xa710199f
    python scripts/tovarisch_test_base_fingerprint.py --seed 0xa710199f --output-dir ./external-analysis
"""

import argparse
import json
import os
import re
import subprocess
import sys
from datetime import datetime
from pathlib import Path


def run_command(cmd: list[str], cwd: str | None = None) -> tuple[str, int]:
    """Run command, return (stdout, exit_code)."""
    try:
        result = subprocess.run(
            cmd,
            cwd=cwd,
            capture_output=True,
            text=True,
            timeout=300,
        )
        return result.stdout, result.returncode
    except subprocess.TimeoutExpired:
        return "", -1
    except Exception as e:
        return f"error: {e}", -1


def get_git_sha(cwd: str) -> str:
    """Get git commit SHA (full)."""
    stdout, code = run_command(["git", "rev-parse", "HEAD"], cwd=cwd)
    if code == 0:
        return stdout.strip()
    return "unknown"


def get_zig_version() -> str:
    """Get Zig version."""
    stdout, code = run_command(["zig", "version"])
    if code == 0:
        return stdout.strip()
    return "unknown"


def get_zig_target() -> str:
    """Get Zig target triple using zig env (preferred) or zig targets."""
    # Try zig env first - output is NOT JSON, it's Zig syntax
    stdout, code = run_command(["zig", "env"])
    if code == 0:
        # Parse .target = "aarch64-macos.14.7.4...14.7.4-none",
        match = re.search(r'\.target\s*=\s*"([^"]+)"', stdout)
        if match:
            return match.group(1)
    
    # Fallback: try zig targets and look for native
    stdout, code = run_command(["zig", "targets"])
    if code == 0:
        lines = stdout.strip().split("\n")
        for line in lines:
            if "native:" in line.lower() or "current:" in line.lower():
                # Extract the triple - handle both "native: .{ ... }" and "native: x86_64-linux-gnu"
                match = re.search(r'(\w+[\w-]*-[\w-]+-[\w]+)', line)
                if match:
                    return match.group(1)
                # Also try simpler pattern for single triples
                match = re.search(r'(\w+-\w+-\w+)', line)
                if match:
                    return match.group(1)
    return "unknown"


def get_uname() -> str:
    """Get OS/kernel info."""
    stdout, code = run_command(["uname", "-a"])
    if code == 0:
        return stdout.strip()
    return "unknown"


def parse_test_summary(output: str) -> dict:
    """Parse pass/skip/fail from zig build test output.
    
    Expected formats:
        - "742 pass, 7 skip, 1 fail"
        - "726/750 tests passed (24 skipped)"
        - "726 pass, 24 skip (750 total)"
        - "All X tests passed"
    """
    # Format 1: "726/750 tests passed (24 skipped)"
    pattern1 = r'(\d+)/(\d+)\s+tests?\s+passed\s+\((\d+)\s+skipped\)'
    match = re.search(pattern1, output)
    if match:
        passed = int(match.group(1))
        total = int(match.group(2))
        skipped = int(match.group(3))
        return {
            "pass": passed,
            "skip": skipped,
            "fail": total - passed - skipped,
            "total": total,
        }
    
    # Format 2: "726 pass, 24 skip (750 total)"
    pattern2 = r'(\d+)\s+pass,\s+(\d+)\s+skip\s+\((\d+)\s+total\)'
    match = re.search(pattern2, output)
    if match:
        passed = int(match.group(1))
        skipped = int(match.group(2))
        total = int(match.group(3))
        return {
            "pass": passed,
            "skip": skipped,
            "fail": total - passed - skipped,
            "total": total,
        }
    
    # Format 3: "742 pass, 7 skip, 1 fail"
    pattern3 = r'(\d+)\s+pass,\s+(\d+)\s+skip,\s+(\d+)\s+fail'
    match = re.search(pattern3, output)
    if match:
        return {
            "pass": int(match.group(1)),
            "skip": int(match.group(2)),
            "fail": int(match.group(3)),
            "total": int(match.group(1)) + int(match.group(2)) + int(match.group(3)),
        }
    
    # Format 4: "All X tests passed"
    all_passed = re.search(r'All\s+(\d+)\s+tests?\s+passed', output)
    if all_passed:
        total = int(all_passed.group(1))
        return {"pass": total, "skip": 0, "fail": 0, "total": total}
    
    return {"pass": 0, "skip": 0, "fail": 0, "total": 0}


def run_zig_test_base(seed: str | None, cwd: str) -> tuple[str, int, dict]:
    """Run zig build test-base with optional seed.
    
    Returns (raw_output, exit_code, parsed_summary).
    """
    cmd = ["zig", "build", "test-base", "--summary", "all"]
    if seed:
        cmd.extend(["--seed", seed])
    
    # Run from the tovarisch subdirectory
    tovarisch_dir = os.path.join(cwd, "tovarisch")
    
    # Run command and capture both stdout and stderr
    try:
        result = subprocess.run(
            cmd,
            cwd=tovarisch_dir,
            capture_output=True,
            text=True,
            timeout=300,
        )
        # Combine stdout and stderr since Zig may output to either
        combined_output = result.stdout + "\n" + result.stderr
        summary = parse_test_summary(combined_output)
        return combined_output, result.returncode, summary
    except subprocess.TimeoutExpired:
        return "", -1, {"pass": 0, "skip": 0, "fail": 0, "total": 0}
    except Exception as e:
        return f"error: {e}", -1, {"pass": 0, "skip": 0, "fail": 0, "total": 0}


def extract_raw_summary_line(output: str) -> str:
    """Extract the pass/skip/fail summary line from output."""
    # Format 1: "726/750 tests passed (24 skipped)"
    pattern1 = r'(\d+/\d+\s+tests?\s+passed\s+\(\d+\s+skipped\))'
    match = re.search(pattern1, output)
    if match:
        return match.group(1)
    
    # Format 2: "726 pass, 24 skip (750 total)"
    pattern2 = r'(\d+\s+pass,\s+\d+\s+skip\s+\(\d+\s+total\))'
    match = re.search(pattern2, output)
    if match:
        return match.group(1)
    
    # Format 3: "742 pass, 7 skip, 1 fail"
    pattern3 = r'(\d+\s+pass,\s+\d+\s+skip,\s+\d+\s+fail)'
    match = re.search(pattern3, output)
    if match:
        return match.group(1)
    
    # Fallback: look for any line with pass/skip/fail
    for line in output.split("\n"):
        if "pass" in line and "skip" in line and "fail" in line:
            return line.strip()
    
    return ""


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Diagnostic fingerprint for `zig build test-base`.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    python scripts/tovarisch_test_base_fingerprint.py
    python scripts/tovarisch_test_base_fingerprint.py --seed 0xa710199f
    python scripts/tovarisch_test_base_fingerprint.py --seed 0xa710199f --output-dir ./external-analysis
        """,
    )
    parser.add_argument(
        "--seed",
        type=str,
        default=None,
        help="Zig test seed (hex or decimal, e.g., 0xa710199f or 2808523935)",
    )
    parser.add_argument(
        "--output-dir",
        type=str,
        default="./external-analysis",
        help="Output directory for fingerprint artifact (default: ./external-analysis)",
    )
    parser.add_argument(
        "--cwd",
        type=str,
        default=None,
        help="Working directory (default: current directory)",
    )
    
    args = parser.parse_args()
    cwd = args.cwd or os.getcwd()
    
    # Determine seed - auto-generate if not provided
    seed = args.seed
    if seed is None:
        # Use timestamp-based seed with random component for uniqueness
        # This ensures fresh test runs even if caching is aggressive
        # Keep within 32-bit unsigned range (max 0xFFFFFFFF)
        import time
        import random
        base = int(time.time() * 1000) & 0x7FFFFFFF  # Use lower 31 bits for base
        random_bits = random.getrandbits(8)  # Add 8 random bits
        seed_int = (base << 8) | random_bits
        seed = f"0x{seed_int & 0xFFFFFFFF:x}"
    
    # Collect environment info
    print(f"Collecting fingerprint for: {cwd}")
    print(f"Seed: {seed}")
    
    git_sha = get_git_sha(cwd)
    print(f"Git SHA: {git_sha}")
    
    zig_version = get_zig_version()
    print(f"Zig version: {zig_version}")
    
    zig_target = get_zig_target()
    print(f"Zig target: {zig_target}")
    
    os_info = get_uname()
    print(f"OS: {os_info}")
    
    # Run the test
    print(f"Running: zig build test-base --summary all --seed {seed}")
    raw_output, exit_code, summary = run_zig_test_base(seed, cwd)
    
    raw_summary_line = extract_raw_summary_line(raw_output)
    
    print(f"Exit code: {exit_code}")
    print(f"Summary: {raw_summary_line or str(summary)}")
    
    # Build fingerprint artifact with bounded raw output tail for debugging
    lines = raw_output.split("\n")
    raw_output_tail = "\n".join(lines[-200:]) if len(lines) > 200 else raw_output
    
    fingerprint = {
        "git_sha": git_sha,
        "zig_version": zig_version,
        "zig_target": zig_target,
        "os_info": os_info,
        "seed": seed,
        "cwd": cwd,
        "exit_code": exit_code,
        "summary": summary,
        "raw_summary_line": raw_summary_line,
        "raw_output_tail": raw_output_tail,
        "timestamp": datetime.now().isoformat(),
        "command": f"zig build test-base --summary all --seed {seed}",
    }
    
    # Write artifact
    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    
    artifact_path = output_dir / "tovarisch-test-base-fingerprint.json"
    
    # Fail closed: if tests passed (exit_code 0) but no summary parsed, record parse error
    if exit_code == 0 and summary["total"] == 0:
        parse_error = "test summary not found in successful zig build output"
        fingerprint["parse_error"] = parse_error
        print(f"WARNING: {parse_error}")
        # Store the raw output for debugging
        fingerprint["raw_output_full"] = raw_output[:10000]  # First 10KB for diagnostics
    
    # Write artifact (always, to capture parse errors too)
    with open(artifact_path, "w") as f:
        json.dump(fingerprint, f, indent=2)
        f.write("\n")
    
    print(f"\nFingerprint written to: {artifact_path}")
    
    # Return 2 if we hit the parse error case
    if exit_code == 0 and summary["total"] == 0:
        return 2
    
    # Exit with test exit code if tests failed
    return exit_code


if __name__ == "__main__":
    sys.exit(main())
