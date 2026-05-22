#!/usr/bin/env python3
"""extract_kcov_line_coverage.py — Robust kcov coverage parser

Parses kcov output to extract line coverage percentage.
Filters coverage to tovarisch/src/ only.

Report priority (most reliable first):
1. coverage.json — kcov generic JSON (counts executable lines only)
2. cobertura.xml — Cobertura XML format (counts executable lines; reliable fallback)
3. cov.xml — Cobertura XML format (alias)
4. codecov.json — codecov-compatible JSON (counts all lines including comments/blanks)

Handles multiple kcov output formats and provides diagnostics on failure.

Usage:
    extract_kcov_line_coverage.py <coverage_dir>

Exit codes:
    0 on success (prints percentage)
    1 on parse failure or missing data (prints error to stderr)
"""

import sys
import os
import json
import xml.etree.ElementTree as ET
from pathlib import Path

from kcov_parsers import (
    parse_codecov_json,
    parse_cobertura_xml,
    parse_coverage_json,
)


def find_coverage_dir(coverage_dir: str) -> Path | None:
    """Find the actual kcov output directory.
    
    kcov may produce output in:
    - coverage_dir/coverage.json (direct)
    - coverage_dir/kcov-merged/coverage.json (merged)
    - coverage_dir/tovarisch-test.HASH/coverage.json (per-binary)
    
    Returns the directory containing the kcov report files, or None.
    """
    base = Path(coverage_dir)
    
    # Direct path
    if base.is_file():
        return base.parent
    
    # Check for tovarisch-test.* subdirectory
    for subdir in base.iterdir():
        if subdir.is_dir() and subdir.name.startswith("tovarisch-test"):
            return subdir
    
    # Check for kcov-merged
    merged = base / "kcov-merged"
    if merged.exists() and merged.is_dir():
        return merged
    
    # Check if base itself has coverage files
    if (base / "coverage.json").exists() or (base / "cobertura.xml").exists():
        return base
    
    return None


def extract_line_coverage(coverage_dir: str) -> float:
    """Extract line coverage percentage from kcov output, filtered to tovarisch/src/ only."""
    
    kcov_dir = find_coverage_dir(coverage_dir)
    if kcov_dir is None:
        print(f"[ERROR] kcov output directory not found in {coverage_dir}", file=sys.stderr)
        print("[ERROR] kcov may have failed to run or produced no coverage data", file=sys.stderr)
        try:
            contents = list(Path(coverage_dir).iterdir())
            print(f"[DEBUG] Contents of {coverage_dir}: {[c.name for c in contents]}", file=sys.stderr)
        except:
            pass
        sys.exit(1)
    
    print(f"[DEBUG] Using kcov output directory: {kcov_dir}", file=sys.stderr)
    
    # List candidate report files (order matches parser priority)
    candidates = [
        ("coverage.json", kcov_dir / "coverage.json"),
        ("cobertura.xml", kcov_dir / "cobertura.xml"),
        ("cov.xml", kcov_dir / "cov.xml"),
        ("codecov.json", kcov_dir / "codecov.json"),
    ]
    
    print(f"[DEBUG] Candidate report files:", file=sys.stderr)
    for name, path in candidates:
        exists = "EXISTS" if path.exists() else "missing"
        print(f"  {name}: {exists}", file=sys.stderr)
    
    # Try each format in priority order
    # coverage.json is most accurate (counts executable lines only)
    # Cobertura XML is the reliable fallback
    # codecov.json counts all lines including blank/comments (overcounted)
    parsers = [
        ("coverage.json", parse_coverage_json),
        ("cobertura.xml", parse_cobertura_xml),
        ("cov.xml", parse_cobertura_xml),
        ("codecov.json", parse_codecov_json),
    ]
    
    for name, parser in parsers:
        path = kcov_dir / name
        if not path.exists():
            print(f"[DEBUG] Skipping {name} — not found", file=sys.stderr)
            continue
        
        print(f"[DEBUG] Trying {name}...", file=sys.stderr)
        result = parser(path)
        
        if result is not None:
            print(f"[DEBUG] Successfully parsed {name}", file=sys.stderr)
            return result
    
    # All parsers failed
    print("[ERROR] All kcov report formats failed to produce tovarisch/src coverage", file=sys.stderr)
    print("[ERROR] Tried: coverage.json, cobertura.xml, cov.xml, codecov.json", file=sys.stderr)
    
    # Provide more diagnostics
    for name, path in candidates:
        if path.exists():
            try:
                if name.endswith('.json'):
                    with open(path) as f:
                        data = json.load(f)
                    if isinstance(data, dict):
                        print(f"[DEBUG] {name} keys: {sorted(data.keys())}", file=sys.stderr)
                    elif isinstance(data, list):
                        print(f"[DEBUG] {name} is a list with {len(data)} entries", file=sys.stderr)
                elif name.endswith('.xml'):
                    tree = ET.parse(path)
                    classes = tree.getroot().findall('.//class')
                    tovarisch = [c.get('filename','') for c in classes if 'tovarisch' in c.get('filename','').lower()]
                    print(f"[DEBUG] {name}: {len(classes)} classes, {len(tovarisch)} tovarisch", file=sys.stderr)
            except Exception as e:
                print(f"[DEBUG] {name} diagnostic failed: {e}", file=sys.stderr)
    
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
