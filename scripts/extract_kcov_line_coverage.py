#!/usr/bin/env python3
"""extract_kcov_line_coverage.py — Robust kcov coverage parser

Parses kcov's coverage.json to extract line coverage percentage.
Handles multiple kcov output formats (list of files or aggregated format).
Handles missing files, parse errors, and malformed data gracefully.

Usage:
    extract_kcov_line_coverage.py <coverage_dir>

Exit codes:
    0 on success (prints percentage)
    1 on parse failure or missing data (prints error to stderr)
"""

import sys
import json
import os
from pathlib import Path


def extract_line_coverage(coverage_dir: str) -> float:
    """Extract line coverage percentage from kcov coverage.json"""
    
    # kcov may produce output in a subdirectory with the binary name
    # e.g., coverage/tovarisch-test.HASH/coverage.json
    # Or directly in coverage/kcov-merged/coverage.json
    
    # First, try to find any coverage.json in the coverage directory tree
    coverage_path = None
    
    # Try direct path
    direct = Path(coverage_dir) / "coverage.json"
    if direct.exists():
        coverage_path = direct
    
    # Try kcov-merged subdirectory
    if coverage_path is None:
        merged = Path(coverage_dir) / "kcov-merged" / "coverage.json"
        if merged.exists():
            coverage_path = merged
    
    # Try finding any coverage.json in subdirectories (binary-name.HASH pattern)
    if coverage_path is None:
        for subdir in Path(coverage_dir).iterdir():
            if subdir.is_dir() and subdir.name.startswith("tovarisch-test"):
                candidate = subdir / "coverage.json"
                if candidate.exists():
                    coverage_path = candidate
                    break
    
    if coverage_path is None:
        print(f"[ERROR] kcov coverage.json not found in {coverage_dir}", file=sys.stderr)
        print("[ERROR] kcov may have failed to run or produced no coverage data", file=sys.stderr)
        # List what we found for debugging
        try:
            contents = list(Path(coverage_dir).iterdir())
            print(f"[DEBUG] Contents of {coverage_dir}: {[c.name for c in contents]}", file=sys.stderr)
        except:
            pass
        sys.exit(1)
    
    try:
        with open(coverage_path, 'r') as f:
            data = json.load(f)
    except json.JSONDecodeError as e:
        print(f"[ERROR] failed to parse kcov coverage.json: {e}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"[ERROR] unexpected error reading coverage.json: {e}", file=sys.stderr)
        sys.exit(1)
    
    # kcov format variant 1: aggregated (newer kcov versions)
    # {"percent_covered": "0.00", "covered_lines": 0, "total_lines": 0, ...}
    if isinstance(data, dict):
        # Authoritative: use covered_lines and total_lines if available
        if 'total_lines' in data:
            total = int(data['total_lines'])
            if total <= 0:
                print("[ERROR] kcov found 0 lines to cover — no coverage data", file=sys.stderr)
                sys.exit(1)
            
            if 'covered_lines' in data:
                covered = int(data['covered_lines'])
                return (covered / total) * 100
        
        # Fallback: use percent_covered if no line totals
        # But only if we have total_lines to validate
        if 'percent_covered' in data:
            # If we have total_lines, we already handled it above
            # If not, percent_covered alone is not authoritative
            if 'total_lines' not in data:
                print("[ERROR] kcov data lacks total_lines field — ambiguous schema", file=sys.stderr)
                sys.exit(1)
            
            try:
                pct = float(data['percent_covered'])
                return pct
            except (ValueError, TypeError):
                print("[ERROR] invalid kcov percent_covered value", file=sys.stderr)
                sys.exit(1)
    
    # kcov format variant 2: per-file list (older kcov versions)
    # [{"filename": "...", "covered": [...], "found": [...]}]
    if isinstance(data, list):
        total_covered = 0
        total_found = 0
        
        for entry in data:
            if not isinstance(entry, dict):
                continue
            
            # kcov uses "covered" (hit lines) and "found" (all lines)
            covered = entry.get('covered', [])
            found = entry.get('found', [])
            
            # Handle list format
            if isinstance(covered, list) and isinstance(found, list):
                total_covered += len(covered)
                total_found += len(found)
            elif isinstance(covered, int) and isinstance(found, int):
                total_covered += covered
                total_found += found
        
        if total_found == 0:
            print("[ERROR] kcov found 0 lines to cover — no coverage data", file=sys.stderr)
            print("[ERROR] this may indicate the test binary did not run or produced no coverage", file=sys.stderr)
            sys.exit(1)
        
        coverage_pct = (total_covered / total_found) * 100
        return coverage_pct
    
    # Unknown format
    print(f"[ERROR] unknown kcov coverage format", file=sys.stderr)
    sys.exit(1)


def main():
    if len(sys.argv) < 2:
        print("Usage: extract_kcov_line_coverage.py <coverage_dir>", file=sys.stderr)
        sys.exit(1)
    
    coverage_dir = sys.argv[1]
    
    if not os.path.isdir(coverage_dir):
        print(f"[ERROR] coverage directory not found: {coverage_dir}", file=sys.stderr)
        sys.exit(1)
    
    try:
        coverage_pct = extract_line_coverage(coverage_dir)
        # Print raw number - shell adds % suffix
        print(f"{coverage_pct:.2f}")
    except SystemExit:
        raise
    except Exception as e:
        print(f"[ERROR] unexpected error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
