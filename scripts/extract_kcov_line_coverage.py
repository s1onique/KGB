#!/usr/bin/env python3
"""extract_kcov_line_coverage.py — Robust kcov coverage parser

Parses kcov's coverage.json to extract line coverage percentage.
Filters coverage to tovarisch/src/ only.
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


# Paths that are NOT part of tovarisch source
FORBIDDEN_PATTERNS = [
    'zig-cache',
    '.zig-cache',
    'zig-out',
    '.git',
    '/usr/',
    '/opt/homebrew/',
    '/nix/store/',
    '/.cache/',
    'kcov-',
    'compiler_rt/',
    'std/zig/',
]


def normalize_tovarisch_src_path(filepath: str) -> str | None:
    """Check if filepath is tovarisch source and return normalized repo-relative path.
    
    Accepts DWARF source paths in various forms:
    - src/main.zig
    - ./src/main.zig
    - tovarisch/src/main.zig
    - /path/to/KGB/tovarisch/src/main.zig
    - /home/runner/work/KGB/KGB/tovarisch/src/main.zig
    - /Volumes/.../tovarisch/src/main.zig
    
    Returns normalized path like "tovarisch/src/main.zig" or None if not project source.
    """
    if not filepath:
        return None
    
    # Normalize path separators
    path = filepath.replace("\\", "/")
    path_lower = path.lower()
    
    # Reject non-.zig files
    if not path_lower.endswith(".zig"):
        return None
    
    # Check for forbidden patterns first
    for pattern in FORBIDDEN_PATTERNS:
        if pattern.lower() in path_lower:
            return None
    
    # Method 1: suffix-based match — find /tovarisch/src/ at any position
    # This handles absolute paths like /home/runner/work/KGB/KGB/tovarisch/src/http/server.zig
    tovarisch_marker = "/tovarisch/src/"
    marker_pos = path_lower.rfind(tovarisch_marker)
    if marker_pos != -1:
        filename = path[marker_pos + len(tovarisch_marker):]
        # Accept nested paths (e.g., http/response.zig) and direct files (e.g., main.zig)
        # Must not start with / and must not contain parent-dir traversal
        if filename and not filename.startswith("/") and "/../" not in filename:
            return f"tovarisch/src/{filename}"
    
    # Method 2: relative paths starting with tovarisch/src/
    # Normalize ./ prefix for consistency
    if path_lower.startswith("./tovarisch/src/"):
        return path[2:]  # strip leading ./
    if path_lower.startswith("tovarisch/src/"):
        return path  # already repo-relative
    
    # Method 3: relative paths starting with src/ (within tovarisch directory)
    if path_lower.startswith("./src/"):
        return path[2:]  # strip leading ./
    if path_lower.startswith("src/"):
        return path  # already repo-relative
    
    # Reject — not under tovarisch/src/
    return None


def is_tovarisch_src_file(filepath: str) -> bool:
    """Check if filepath is under tovarisch/src/ directory.
    
    This is a wrapper for normalize_tovarisch_src_path for backward compatibility.
    """
    return normalize_tovarisch_src_path(filepath) is not None


def extract_line_coverage(coverage_dir: str) -> float:
    """Extract line coverage percentage from kcov coverage.json, filtered to tovarisch/src/ only."""
    
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
    
    # kcov format variant 1: dict with "files" list (newer kcov versions, e.g., kcov 43)
    # {"files": [{"file": "...", "covered_lines": N, "total_lines": M, ...}], "covered_lines": X, "total_lines": Y}
    if isinstance(data, dict):
        # Handle kcov's "files" list wrapper format
        # {"files": [{"file": "...", "covered_lines": N, "total_lines": M, ...}]}
        if 'files' in data and isinstance(data['files'], list):
            files_list = data['files']
            total_covered = 0
            total_found = 0
            source_files_seen = []
            
            for entry in files_list:
                if not isinstance(entry, dict):
                    continue
                
                filepath = entry.get('file', '')
                
                # Filter to only tovarisch/src files
                if not is_tovarisch_src_file(filepath):
                    continue
                
                source_files_seen.append(filepath)
                
                # Handle string values ("0", "5") or int values
                covered = entry.get('covered_lines', 0)
                found = entry.get('total_lines', 0)
                
                # Convert string to int if needed
                if isinstance(covered, str):
                    covered = int(covered) if covered.isdigit() else 0
                if isinstance(found, str):
                    found = int(found) if found.isdigit() else 0
                
                total_covered += covered
                total_found += found
            
            if len(source_files_seen) == 0:
                print("[ERROR] kcov found 0 tovarisch/src lines to cover — no source coverage data", file=sys.stderr)
                print(f"[ERROR] kcov emitted {len(files_list)} files but none matched tovarisch/src/", file=sys.stderr)
                # Show sample paths for debugging
                sample_paths = [e.get('file', '') for e in files_list[:10] if isinstance(e, dict)]
                if sample_paths:
                    print("[DEBUG] First observed source paths:", file=sys.stderr)
                    for p in sample_paths:
                        print(f"  {p}", file=sys.stderr)
                sys.exit(1)
            
            if total_found == 0:
                print("[ERROR] kcov found 0 lines to cover in tovarisch/src — no source coverage data", file=sys.stderr)
                sys.exit(1)
            
            coverage_pct = (total_covered / total_found) * 100
            return coverage_pct
        
        # Aggregate-only format without files list: REJECT as ambiguous
        # We cannot determine which lines are in tovarisch/src without per-file data
        if 'total_lines' in data and 'files' not in data:
            print("[ERROR] kcov data has aggregate totals but no per-file breakdown — ambiguous", file=sys.stderr)
            print("[ERROR] cannot determine tovarisch/src coverage without files list", file=sys.stderr)
            sys.exit(1)
        
        # No files list and no totals - unknown format
        print("[ERROR] unknown kcov coverage format", file=sys.stderr)
        sys.exit(1)
    
    # kcov format variant 2: per-file list (older kcov versions)
    # [{"filename": "...", "covered": [...], "found": [...]}]
    if isinstance(data, list):
        total_covered = 0
        total_found = 0
        source_files_seen = []
        
        for entry in data:
            if not isinstance(entry, dict):
                continue
            
            # Get filepath - kcov uses "file" or "filename"
            filepath = entry.get('file', '') or entry.get('filename', '')
            
            # Filter to only tovarisch/src files
            if not is_tovarisch_src_file(filepath):
                continue
            
            source_files_seen.append(filepath)
            
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
        
        if len(source_files_seen) == 0:
            print("[ERROR] kcov found 0 tovarisch/src lines to cover — no source coverage data", file=sys.stderr)
            print(f"[ERROR] kcov emitted {len(data)} files but none matched tovarisch/src/", file=sys.stderr)
            # Show sample paths for debugging
            sample_paths = []
            for e in data[:10]:
                if isinstance(e, dict):
                    p = e.get('file', '') or e.get('filename', '')
                    if p:
                        sample_paths.append(p)
            if sample_paths:
                print("[DEBUG] First observed source paths:", file=sys.stderr)
                for p in sample_paths:
                    print(f"  {p}", file=sys.stderr)
            sys.exit(1)
        
        if total_found == 0:
            print("[ERROR] kcov found 0 lines to cover in tovarisch/src — no source coverage data", file=sys.stderr)
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
