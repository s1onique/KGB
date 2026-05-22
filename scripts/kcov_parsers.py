"""kcov_parsers.py — kcov report format parsers

Parsers for kcov output formats. Used by extract_kcov_line_coverage.py.

Formats supported:
- codecov.json: codecov-compatible JSON (counts all lines including comments/blanks)
- cobertura.xml / cov.xml: Cobertura XML format
- coverage.json: kcov generic JSON (counts executable lines only)

Each parser returns coverage percentage or None if no tovarisch files found.
"""

import sys
import json
import xml.etree.ElementTree as ET
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


def parse_codecov_json(coverage_path: Path) -> float | None:
    """Parse codecov.json format.
    
    Format: {"coverage": {"filepath": {"lineno": "hits/total", ...}, ...}}
    
    Returns coverage percentage or None if no tovarisch files found.
    """
    try:
        with open(coverage_path, 'r') as f:
            data = json.load(f)
    except json.JSONDecodeError as e:
        print(f"[DEBUG] codecov.json parse error: {e}", file=sys.stderr)
        return None
    except Exception as e:
        print(f"[DEBUG] codecov.json read error: {e}", file=sys.stderr)
        return None
    
    coverage = data.get('coverage', {})
    if not coverage:
        print("[DEBUG] codecov.json has no 'coverage' key", file=sys.stderr)
        return None
    
    total_covered = 0
    total_found = 0
    source_files_seen = []
    
    for filepath, lines in coverage.items():
        if not isinstance(lines, dict):
            continue
        
        # Filter to only tovarisch/src files
        if not is_tovarisch_src_file(filepath):
            continue
        
        source_files_seen.append(filepath)
        
        for lineno, hit_data in lines.items():
            # hit_data format: "0/1" or "1/3" etc
            if not isinstance(hit_data, str):
                continue
            parts = hit_data.split('/')
            if len(parts) == 2:
                try:
                    hits = int(parts[0])
                    total = int(parts[1])
                    total_covered += hits
                    total_found += total
                except ValueError:
                    pass
    
    if len(source_files_seen) == 0:
        print(f"[DEBUG] codecov.json has 0 tovarisch/src files", file=sys.stderr)
        return None
    
    if total_found == 0:
        print(f"[DEBUG] codecov.json found 0 lines in tovarisch/src", file=sys.stderr)
        return None
    
    coverage_pct = (total_covered / total_found) * 100
    print(f"[DEBUG] codecov.json: {len(source_files_seen)} files, {total_covered}/{total_found} lines, {coverage_pct:.2f}%", file=sys.stderr)
    return coverage_pct


def parse_cobertura_xml(coverage_path: Path) -> float | None:
    """Parse Cobertura XML format.
    
    Format: <class name="..." filename="..."> <lines> <line number="N" hits="H"/> ...
    
    Returns coverage percentage or None if no tovarisch files found.
    """
    try:
        tree = ET.parse(coverage_path)
    except ET.ParseError as e:
        print(f"[DEBUG] Cobertura XML parse error: {e}", file=sys.stderr)
        return None
    except Exception as e:
        print(f"[DEBUG] Cobertura XML read error: {e}", file=sys.stderr)
        return None
    
    root = tree.getroot()
    classes = root.findall('.//class')
    
    if len(classes) == 0:
        print("[DEBUG] Cobertura XML has 0 class elements", file=sys.stderr)
        return None
    
    total_covered = 0
    total_found = 0
    source_files_seen = []
    
    for cls in classes:
        filename = cls.get('filename', '')
        
        # Filter to only tovarisch/src files
        if not is_tovarisch_src_file(filename):
            continue
        
        source_files_seen.append(filename)
        
        lines = cls.findall('.//line')
        for line in lines:
            hits = line.get('hits', '0')
            try:
                # Line is covered if hits > 0 (don't add hit counts)
                if int(hits) > 0:
                    total_covered += 1
                total_found += 1
            except ValueError:
                pass
    
    if len(source_files_seen) == 0:
        print(f"[DEBUG] Cobertura XML has 0 tovarisch/src files (total classes: {len(classes)})", file=sys.stderr)
        return None
    
    if total_found == 0:
        print(f"[DEBUG] Cobertura XML found 0 lines in tovarisch/src", file=sys.stderr)
        return None
    
    coverage_pct = (total_covered / total_found) * 100
    print(f"[DEBUG] Cobertura XML: {len(source_files_seen)} files, {total_covered}/{total_found} lines, {coverage_pct:.2f}%", file=sys.stderr)
    return coverage_pct


def parse_coverage_json(coverage_path: Path) -> float | None:
    """Parse coverage.json format.
    
    Supports two variants:
    - dict with "files" array: {"files": [{"file": "...", "covered_lines": N, "total_lines": M}]}
    - list format: [{"file": "...", "covered_lines": N, "total_lines": M}]
    
    Returns coverage percentage or None if no tovarisch files found.
    """
    try:
        with open(coverage_path, 'r') as f:
            data = json.load(f)
    except json.JSONDecodeError as e:
        print(f"[DEBUG] coverage.json parse error: {e}", file=sys.stderr)
        return None
    except Exception as e:
        print(f"[DEBUG] coverage.json read error: {e}", file=sys.stderr)
        return None
    
    # Print top-level keys for diagnostics
    if isinstance(data, dict):
        keys = sorted(data.keys())
        print(f"[DEBUG] coverage.json dict keys: {keys}", file=sys.stderr)
    
    total_covered = 0
    total_found = 0
    source_files_seen = []
    files_list = None
    
    # Variant 1: dict with "files" array
    if isinstance(data, dict) and 'files' in data and isinstance(data['files'], list):
        files_list = data['files']
    
    # Variant 2: list format
    elif isinstance(data, list):
        files_list = data
    
    if files_list is None:
        print("[DEBUG] coverage.json: unknown format (no 'files' array or list)", file=sys.stderr)
        return None
    
    for entry in files_list:
        if not isinstance(entry, dict):
            continue
        
        filepath = entry.get('file', '') or entry.get('filename', '')
        
        # Filter to only tovarisch/src files
        if not is_tovarisch_src_file(filepath):
            continue
        
        source_files_seen.append(filepath)
        
        # Handle string or int values for covered/total
        covered = entry.get('covered_lines', 0)
        found = entry.get('total_lines', 0)
        
        if isinstance(covered, str):
            covered = int(covered) if covered.isdigit() else 0
        if isinstance(found, str):
            found = int(found) if found.isdigit() else 0
        
        total_covered += covered
        total_found += found
    
    if len(source_files_seen) == 0:
        print(f"[DEBUG] coverage.json has 0 tovarisch/src files (total entries: {len(files_list)})", file=sys.stderr)
        return None
    
    if total_found == 0:
        print(f"[DEBUG] coverage.json found 0 lines in tovarisch/src", file=sys.stderr)
        return None
    
    coverage_pct = (total_covered / total_found) * 100
    print(f"[DEBUG] coverage.json: {len(source_files_seen)} files, {total_covered}/{total_found} lines, {coverage_pct:.2f}%", file=sys.stderr)
    return coverage_pct

