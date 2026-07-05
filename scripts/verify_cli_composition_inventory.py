#!/usr/bin/env python3
"""
CLI Composition Inventory Verifier

Scans codebase for process/CLI patterns and validates against inventory CSV.
This enforces the native-owned critical-paths doctrine: critical runtime paths
should use native code (Go, Zig) rather than shell-based CLI composition.

Usage:
    python verify_cli_composition_inventory.py [--self-test] [--verbose]
"""

import argparse
import csv
import re
import sys
from dataclasses import dataclass
from pathlib import Path


# ---------------------------------------------------------------------------
# Data structures
# ---------------------------------------------------------------------------

@dataclass
class InventoryEntry:
    """Represents a single row in the CLI composition inventory."""
    id: str
    path: str
    language: str
    pattern: str
    owner_area: str
    runtime_classification: str
    frequency: str
    allowed: str
    justification: str
    timeout_bounded: str
    output_bounded: str
    redaction_required: str
    replacement_candidate: str
    status: str
    notes: str


@dataclass
class DetectedSite:
    """A detected CLI usage site in the codebase."""
    file_path: str
    line_number: int
    pattern: str
    matched_text: str


# ---------------------------------------------------------------------------
# Pattern definitions
# ---------------------------------------------------------------------------

# Valid enum values per column
VALID_LANGUAGE = {"go", "python", "javascript", "typescript", "zig", "shell"}
VALID_RUNTIME_CLASSIFICATION = {
    "critical_runtime", "diagnostic_runtime", "build_test",
    "lab_infrastructure", "packaging", "ci_cd", "developer_tool"
}
VALID_FREQUENCY = {"once", "rare", "periodic", "frequent", "continuous"}
VALID_ALLOWED = {"yes", "no", "pending_review"}
VALID_TIMEOUT_BOUNDED = {"yes", "no", "n/a"}
VALID_OUTPUT_BOUNDED = {"yes", "no", "n/a"}
VALID_REDACTION_REQUIRED = {"yes", "no", "n/a"}
VALID_REPLACEMENT_CANDIDATE = {"yes", "no", "n/a"}
VALID_STATUS = {
    "verified", "missing", "migration_candidate", "temporary",
    "external_dependency", "unbounded"
}

# Pattern regexes for different languages
# Note: Go import patterns must match actual import statements, not comments
PATTERNS = {
    "go": [
        # Match actual import "os/exec" statements (not comments/prose)
        (r'^\s*import\s+"os/exec"', "os/exec import"),
        (r'^\s*"os/exec"', "os/exec import"),
        # Match actual exec calls (not comments)
        (r'exec\.Command\s*\(', "exec.Command()"),
        (r'exec\.CommandContext\s*\(', "exec.CommandContext()"),
        # Match syscall package usage (NETLINK_ROUTE, raw sockets)
        (r'\bsyscall\b', "syscall import"),
    ],
    "python": [
        (r'\bsubprocess\b', "subprocess module"),
        (r'subprocess\.(run|Popen|call|check_output|CalledProcessError)\s*\(', "subprocess call"),
        (r'\bre\b', "re module"),
        (r'\bos\.system\s*\(', "os.system()"),
        (r'\bos\.popen\s*\(', "os.popen()"),
    ],
    "javascript": [
        (r'\bchild_process\b', "child_process module"),
        (r'(exec|execSync|spawn|spawnSync|fork)\s*\(', "child_process call"),
    ],
    "typescript": [
        (r'\bchild_process\b', "child_process module"),
        (r'(exec|execSync|spawn|spawnSync|fork)\s*\(', "child_process call"),
    ],
    "zig": [
        (r'\bstd\.process\b', "std.process import"),
        (r'ChildProcess', "ChildProcess"),
        (r'execve', "execve syscall"),
    ],
    "shell": [
        (r'^#!/.*sh', "shebang"),
    ],
}

# Directories to exclude from scanning
EXCLUDE_DIRS = {
    "node_modules", "zig-cache", "zig-out", ".zig-cache", ".git",
    "coverage", "kcov-output", "vendor", "__pycache__", ".venv", "venv",
    "dist", ".dist"
}


def get_files_by_language(root_dir: Path, language: str) -> list[Path]:
    """Get all source files for a given language."""
    extensions = {
        "go": [".go"], "python": [".py"], "javascript": [".js", ".mjs"],
        "typescript": [".ts", ".tsx"], "zig": [".zig"], "shell": [".sh"],
    }
    exts = extensions.get(language, [])
    files = []
    for ext in exts:
        for f in root_dir.rglob(f"*{ext}"):
            if any(excl in f.parts for excl in EXCLUDE_DIRS):
                continue
            files.append(f)
    return files


def scan_file(path: Path, language: str) -> list[DetectedSite]:
    """Scan a single file for CLI composition patterns."""
    sites = []
    try:
        content = path.read_text()
    except (UnicodeDecodeError, PermissionError, OSError):
        return sites
    for idx, line in enumerate(content.splitlines(), 1):
        for regex, pattern_name in PATTERNS.get(language, []):
            if re.search(regex, line):
                sites.append(DetectedSite(
                    file_path=str(path), line_number=idx,
                    pattern=pattern_name, matched_text=line.strip()[:100],
                ))
    return sites


def scan_codebase(root_dir: Path) -> list[DetectedSite]:
    """Scan entire codebase for CLI patterns."""
    all_sites = []
    for language in PATTERNS:
        for filepath in get_files_by_language(root_dir, language):
            all_sites.extend(scan_file(filepath, language))
    return all_sites


# ---------------------------------------------------------------------------
# CSV validation
# ---------------------------------------------------------------------------

def validate_enum(value: str, valid_values: set, field_name: str,
                  entry_id: str, errors: list[str]) -> None:
    if value not in valid_values:
        errors.append(
            f"Invalid '{field_name}' value '{value}' for entry {entry_id}. "
            f"Valid values: {', '.join(sorted(valid_values))}"
        )


def load_inventory(csv_path: Path) -> tuple[list[InventoryEntry], list[str]]:
    """Load and validate the inventory CSV."""
    entries = []
    errors = []
    seen_ids = set()
    try:
        with open(csv_path, 'r', newline='', encoding='utf-8') as f:
            reader = csv.DictReader(f)
            fieldnames = reader.fieldnames or []
            required = ["id", "path", "language", "pattern", "owner_area",
                       "runtime_classification", "frequency", "allowed",
                       "justification", "timeout_bounded", "output_bounded",
                       "redaction_required", "replacement_candidate", "status", "notes"]
            missing = [h for h in required if h not in fieldnames]
            if missing:
                errors.append(f"Missing required headers: {', '.join(missing)}")
                return [], errors
            for row_num, row in enumerate(reader, 2):
                entry_id = row.get('id', '').strip()
                if not entry_id:
                    errors.append(f"Row {row_num}: Missing 'id' field")
                    continue
                if entry_id in seen_ids:
                    errors.append(f"Duplicate ID '{entry_id}' at row {row_num}")
                seen_ids.add(entry_id)
                entry = InventoryEntry(
                    id=entry_id, path=row.get('path', '').strip(),
                    language=row.get('language', '').strip().lower(),
                    pattern=row.get('pattern', '').strip(),
                    owner_area=row.get('owner_area', '').strip(),
                    runtime_classification=row.get('runtime_classification', '').strip(),
                    frequency=row.get('frequency', '').strip().lower(),
                    allowed=row.get('allowed', '').strip().lower(),
                    justification=row.get('justification', '').strip(),
                    timeout_bounded=row.get('timeout_bounded', '').strip().lower(),
                    output_bounded=row.get('output_bounded', '').strip().lower(),
                    redaction_required=row.get('redaction_required', '').strip().lower(),
                    replacement_candidate=row.get('replacement_candidate', '').strip().lower(),
                    status=row.get('status', '').strip(),
                    notes=row.get('notes', '').strip(),
                )
                validate_enum(entry.language, VALID_LANGUAGE, "language", entry_id, errors)
                validate_enum(entry.runtime_classification, VALID_RUNTIME_CLASSIFICATION,
                              "runtime_classification", entry_id, errors)
                validate_enum(entry.frequency, VALID_FREQUENCY, "frequency", entry_id, errors)
                validate_enum(entry.allowed, VALID_ALLOWED, "allowed", entry_id, errors)
                validate_enum(entry.timeout_bounded, VALID_TIMEOUT_BOUNDED,
                              "timeout_bounded", entry_id, errors)
                validate_enum(entry.output_bounded, VALID_OUTPUT_BOUNDED,
                              "output_bounded", entry_id, errors)
                validate_enum(entry.redaction_required, VALID_REDACTION_REQUIRED,
                              "redaction_required", entry_id, errors)
                validate_enum(entry.replacement_candidate, VALID_REPLACEMENT_CANDIDATE,
                              "replacement_candidate", entry_id, errors)
                validate_enum(entry.status, VALID_STATUS, "status", entry_id, errors)
                entries.append(entry)
    except FileNotFoundError:
        errors.append(f"CSV file not found: {csv_path}")
    except csv.Error as e:
        errors.append(f"CSV parsing error: {e}")
    except Exception as e:
        errors.append(f"Error loading CSV: {e}")
    return entries, errors


# ---------------------------------------------------------------------------
# Verification logic
# ---------------------------------------------------------------------------

def make_relative(path: str, root: Path) -> str:
    """Convert full path to relative path from root."""
    try:
        return str(Path(path).relative_to(root))
    except ValueError:
        return path


def path_matches_inventory(path: str, inventory_paths: set, inventory_dir_patterns: set) -> bool:
    """Check if a path matches an inventory entry or directory pattern."""
    if path in inventory_paths:
        return True
    for pattern in inventory_dir_patterns:
        if path.startswith(pattern):
            return True
    return False


def verify_inventory(entries: list[InventoryEntry], detected: list[DetectedSite],
                     repo_root: Path, verbose: bool = False) -> tuple[list[str], list[str], list[str]]:
    """
    Verify detected sites against inventory.
    Returns (errors, warnings, migration_backlog).
    """
    errors = []
    warnings = []
    backlog = []

    inventory_paths = {e.path for e in entries if not e.path.endswith('/')}
    inventory_dir_patterns = {e.path for e in entries if e.path.endswith('/')}
    
    # Build detected (path, pattern) set
    detected_relative = []
    for site in detected:
        rel_path = make_relative(site.file_path, repo_root)
        detected_relative.append((rel_path, site.pattern, site.line_number, site.matched_text))
    
    detected_path_patterns = {(p, pat) for p, pat, _, _ in detected_relative}
    detected_paths = {p for p, _, _, _ in detected_relative}

    # Check for missing inventory entries
    for rel_path, pattern, line_num, matched in detected_relative:
        if not path_matches_inventory(rel_path, inventory_paths, inventory_dir_patterns):
            errors.append(
                f"DETECTED CLI usage at {rel_path}:{line_num} pattern='{pattern}' "
                f"but no inventory entry exists"
            )
            if verbose:
                errors.append(f"  Context: {matched[:80]}")

    # Validate inventory rows
    for entry in entries:
        full_path = repo_root / entry.path if not entry.path.endswith('/') else None
        
        # Check missing path (unless status=removed or directory pattern)
        if entry.path.endswith('/'):
            # Directory aggregate - check if any files match
            matching = [p for p in detected_paths if p.startswith(entry.path)]
            if not matching:
                warnings.append(f"Entry '{entry.id}': directory pattern '{entry.path}' has no matching files")
        else:
            if not full_path.exists():
                if entry.status not in ("missing", "external_dependency", "migration_candidate"):
                    errors.append(
                        f"Entry '{entry.id}': path '{entry.path}' does not exist "
                        f"but status='{entry.status}' (expected status=missing/external_dependency/migration_candidate)"
                    )
            else:
                # Check (path, pattern) matches
                if (entry.path, entry.pattern) not in detected_path_patterns:
                    errors.append(
                        f"Entry '{entry.id}': path '{entry.path}' exists but "
                        f"pattern '{entry.pattern}' not detected"
                    )

        # Critical runtime validation
        if entry.runtime_classification == "critical_runtime":
            if entry.timeout_bounded == "no":
                errors.append(f"Entry '{entry.id}': critical_runtime requires timeout_bounded=yes")
            if entry.output_bounded == "no":
                errors.append(f"Entry '{entry.id}': critical_runtime requires output_bounded=yes")

        # allowed=no should not exist
        if entry.allowed == "no" and entry.path in detected_paths:
            errors.append(f"Entry '{entry.id}': allowed=no but CLI pattern exists at {entry.path}")

        # Warnings
        if entry.status == "temporary":
            warnings.append(f"Entry '{entry.id}': status=temporary, should be resolved")
        if entry.replacement_candidate == "yes":
            warnings.append(f"Entry '{entry.id}': marked as replacement_candidate")
            # Add to migration backlog
            score = calculate_migration_score(entry)
            backlog.append((score, entry.id, entry.path, entry.runtime_classification,
                           entry.frequency, entry.pattern))

    # Sort backlog by score (higher = more urgent)
    backlog.sort(key=lambda x: -x[0])
    
    return errors, warnings, backlog


def calculate_migration_score(entry: InventoryEntry) -> int:
    """Calculate migration urgency score."""
    score = 0
    # Higher urgency for critical paths
    if entry.runtime_classification == "critical_runtime":
        score += 50
    elif entry.runtime_classification == "diagnostic_runtime":
        score += 30
    # Higher urgency for frequent execution
    freq_scores = {"continuous": 20, "frequent": 15, "periodic": 10, "rare": 5, "once": 2}
    score += freq_scores.get(entry.frequency, 0)
    # Higher urgency if unbounded
    if entry.timeout_bounded == "no":
        score += 15
    if entry.output_bounded == "no":
        score += 15
    return score


# ---------------------------------------------------------------------------
# Self-test mode
# ---------------------------------------------------------------------------

def run_self_test() -> bool:
    """Run self-test mode with comprehensive test coverage."""
    import tempfile

    tests_passed = 0
    tests_failed = 0

    def assert_test(condition: bool, test_name: str, detail: str = ""):
        nonlocal tests_passed, tests_failed
        if condition:
            print(f"  ✓ {test_name}")
            tests_passed += 1
        else:
            print(f"  ✗ {test_name}")
            if detail:
                print(f"    Detail: {detail}")
            tests_failed += 1

    print("\n=== CLI Composition Inventory Verifier Self-Test ===\n")

    with tempfile.TemporaryDirectory() as tmpdir:
        tmppath = Path(tmpdir)
        inventory_file = tmppath / "inventory.csv"

        # Test 1: missing inventory row fails
        print("Test 1: Missing inventory row fails")
        scripts_dir = tmppath / "scripts"
        scripts_dir.mkdir()
        with open(inventory_file, 'w') as f:
            f.write("""id,path,language,pattern,owner_area,runtime_classification,frequency,allowed,justification,timeout_bounded,output_bounded,redaction_required,replacement_candidate,status,notes
CLI001,scripts/test.sh,shell,shebang,test,lab_infrastructure,once,yes,test only,yes,yes,no,no,temporary,test entry
""")
        test_go = tmppath / "test.go"
        test_go.write_text('package main\nimport "os/exec"\nfunc main() { exec.Command("echo") }\n')
        entries, _ = load_inventory(inventory_file)
        detected = scan_codebase(tmppath)
        errors, _, _ = verify_inventory(entries, detected, tmppath)
        assert_test(len(errors) > 0, "Detects missing inventory entry", f"Errors: {errors[:2]}")

        # Test 2: duplicate ID fails
        print("\nTest 2: Duplicate ID fails")
        with open(inventory_file, 'w') as f:
            f.write("""id,path,language,pattern,owner_area,runtime_classification,frequency,allowed,justification,timeout_bounded,output_bounded,redaction_required,replacement_candidate,status,notes
CLI001,scripts/test.sh,shell,shebang,test,lab_infrastructure,once,yes,test,yes,yes,no,no,verified,test
CLI001,scripts/other.sh,shell,shebang,test,lab_infrastructure,once,yes,dup,yes,yes,no,no,verified,dup
""")
        entries, errors = load_inventory(inventory_file)
        assert_test(len(errors) > 0, "Detects duplicate ID", f"Errors: {errors}")

        # Test 3: invalid enum fails
        print("\nTest 3: Invalid enum fails")
        with open(inventory_file, 'w') as f:
            f.write("""id,path,language,pattern,owner_area,runtime_classification,frequency,allowed,justification,timeout_bounded,output_bounded,redaction_required,replacement_candidate,status,notes
CLI001,scripts/test.sh,shell,shebang,test,lab_infrastructure,once,yes,test,yes,yes,no,no,invalid_status,test
""")
        entries, errors = load_inventory(inventory_file)
        assert_test(len(errors) > 0, "Detects invalid status enum", f"Errors: {errors}")

        # Test 4: missing required header fails
        print("\nTest 4: Missing required header fails")
        with open(inventory_file, 'w') as f:
            f.write("""id,path,language,pattern,owner_area
CLI001,scripts/test.sh,shell,shebang,test
""")
        entries, errors = load_inventory(inventory_file)
        assert_test(len(errors) > 0, "Detects missing required header", f"Errors: {errors}")

        # Test 5: allowed=no with current code fails
        print("\nTest 5: allowed=no with current code fails")
        with open(inventory_file, 'w') as f:
            f.write("""id,path,language,pattern,owner_area,runtime_classification,frequency,allowed,justification,timeout_bounded,output_bounded,redaction_required,replacement_candidate,status,notes
CLI001,test.go,go,os/exec import,test,lab_infrastructure,once,no,forbidden,yes,yes,no,no,verified,test
""")
        entries, _ = load_inventory(inventory_file)
        detected = scan_codebase(tmppath)
        errors, _, _ = verify_inventory(entries, detected, tmppath)
        assert_test(len(errors) > 0, "Detects allowed=no with existing code", f"Errors: {errors}")

        # Test 6: verified row with missing path fails
        print("\nTest 6: verified row with missing path fails")
        with open(inventory_file, 'w') as f:
            f.write("""id,path,language,pattern,owner_area,runtime_classification,frequency,allowed,justification,timeout_bounded,output_bounded,redaction_required,replacement_candidate,status,notes
CLI001,nonexistent.go,go,os/exec import,test,lab_infrastructure,once,yes,test,yes,yes,no,no,verified,missing file
""")
        entries, _ = load_inventory(inventory_file)
        detected = scan_codebase(tmppath)
        errors, _, _ = verify_inventory(entries, detected, tmppath)
        assert_test(len(errors) > 0, "Detects verified row with missing path", f"Errors: {errors}")

        # Test 7: verified row with wrong pattern fails
        print("\nTest 7: verified row with path but wrong pattern fails")
        scripts_dir = tmppath / "scripts"
        scripts_dir.mkdir(exist_ok=True)
        # Remove test.go to avoid false detections
        test_go = tmppath / "test.go"
        if test_go.exists():
            test_go.unlink()
        test_script = scripts_dir / "test.sh"
        test_script.write_text('#!/bin/sh\necho test\n')
        test_script.chmod(0o755)
        with open(inventory_file, 'w') as f:
            f.write("""id,path,language,pattern,owner_area,runtime_classification,frequency,allowed,justification,timeout_bounded,output_bounded,redaction_required,replacement_candidate,status,notes
CLI001,scripts/test.sh,shell,wrong_pattern,test,lab_infrastructure,once,yes,test,yes,yes,no,no,verified,pattern mismatch
""")
        entries, _ = load_inventory(inventory_file)
        detected = scan_codebase(tmppath)
        errors, _, _ = verify_inventory(entries, detected, tmppath)
        assert_test(len(errors) > 0, "Detects pattern mismatch", f"Errors: {errors}")

        # Test 8: directory aggregate row passes for matching subtree
        print("\nTest 8: directory aggregate row passes for matching subtree")
        with open(inventory_file, 'w') as f:
            f.write("""id,path,language,pattern,owner_area,runtime_classification,frequency,allowed,justification,timeout_bounded,output_bounded,redaction_required,replacement_candidate,status,notes
CLI001,scripts/,shell,shebang,test,lab_infrastructure,once,yes,test,yes,yes,no,no,verified,dir pattern
""")
        entries, _ = load_inventory(inventory_file)
        detected = scan_codebase(tmppath)
        errors, warnings, _ = verify_inventory(entries, detected, tmppath)
        assert_test(len(errors) == 0, "Directory pattern passes", f"Errors: {errors}")

        # Test 9: warning scenario does not fail
        print("\nTest 9: Warning scenarios produce warnings, not errors")
        with open(inventory_file, 'w') as f:
            f.write("""id,path,language,pattern,owner_area,runtime_classification,frequency,allowed,justification,timeout_bounded,output_bounded,redaction_required,replacement_candidate,status,notes
CLI001,scripts/test.sh,shell,shebang,test,lab_infrastructure,rare,yes,test,yes,yes,no,yes,temporary,warning test
""")
        entries, _ = load_inventory(inventory_file)
        detected = scan_codebase(tmppath)
        errors, warnings, _ = verify_inventory(entries, detected, tmppath)
        assert_test(len(errors) == 0, "No errors for warning scenarios")
        assert_test(len(warnings) > 0, "Produces warnings for warning scenarios", f"Warnings: {warnings}")

        # Test 10: valid minimal fixture passes
        print("\nTest 10: Valid minimal fixture passes")
        with open(inventory_file, 'w') as f:
            f.write("""id,path,language,pattern,owner_area,runtime_classification,frequency,allowed,justification,timeout_bounded,output_bounded,redaction_required,replacement_candidate,status,notes
CLI001,scripts/test.sh,shell,shebang,test,lab_infrastructure,once,yes,test only,yes,yes,no,no,verified,test entry
""")
        entries, load_errors = load_inventory(inventory_file)
        detected = scan_codebase(tmppath)
        errors, warnings, _ = verify_inventory(entries, detected, tmppath)
        assert_test(len(load_errors) == 0, "No load errors for valid fixture")
        assert_test(len(errors) == 0, "No verification errors for valid fixture", f"Errors: {errors}")

        # Test 11: Go comment containing os/exec does NOT count
        print("\nTest 11: Go comment with os/exec does not match")
        test_go_comment = tmppath / "comment_test.go"
        test_go_comment.write_text('// This file avoids os/exec for better performance\npackage main\nfunc main() {}\n')
        test_go_real = tmppath / "real_import.go"  # Initialize for later cleanup
        with open(inventory_file, 'w') as f:
            f.write("""id,path,language,pattern,owner_area,runtime_classification,frequency,allowed,justification,timeout_bounded,output_bounded,redaction_required,replacement_candidate,status,notes
CLI001,comment_test.go,go,os/exec import,test,lab_infrastructure,once,yes,test,yes,yes,no,no,verified,comment-only test
""")
        entries, load_errors = load_inventory(inventory_file)
        detected = scan_codebase(tmppath)
        errors, _, _ = verify_inventory(entries, detected, tmppath)
        assert_test(len(errors) > 0, "Comment-only os/exec does not match", f"Errors: {errors}")

        # Test 12: Go real import line matches
        print("\nTest 12: Go real import matches")
        # Clean up previous test files to avoid pollution
        for f in [test_go, test_go_comment]:
            if f.exists():
                f.unlink()
        test_go_real = tmppath / "real_import.go"
        test_go_real.write_text('package main\nimport "os/exec"\nfunc main() { exec.Command("echo") }\n')
        with open(inventory_file, 'w') as f:
            f.write("""id,path,language,pattern,owner_area,runtime_classification,frequency,allowed,justification,timeout_bounded,output_bounded,redaction_required,replacement_candidate,status,notes
CLI001,real_import.go,go,os/exec import,test,lab_infrastructure,once,yes,test,yes,yes,no,no,verified,real import test
CLI002,scripts/,shell,shebang,test,lab_infrastructure,once,yes,test,yes,yes,no,no,verified,dir pattern
""")
        entries, load_errors = load_inventory(inventory_file)
        detected = scan_codebase(tmppath)
        errors, _, _ = verify_inventory(entries, detected, tmppath)
        assert_test(len(errors) == 0, "Real import matches", f"Errors: {errors}")

        # Test 13: Native-owned iptables boundary - owned module passes
        print("\nTest 13: Native-owned iptables boundary - owned module passes")
        # Clean up previous test files
        if test_go_real.exists():
            test_go_real.unlink()
        # Clean up previous tovarisch directory if exists
        import shutil
        tovarisch_dir = tmppath / "tovarisch"
        if tovarisch_dir.exists():
            shutil.rmtree(tovarisch_dir)
        tovarisch_dir.mkdir()
        src_dir = tovarisch_dir / "src"
        src_dir.mkdir()
        net_dir = src_dir / "net"
        net_dir.mkdir()
        # Owned module with execve pattern (should pass)
        iptables_owned = net_dir / "iptables.zig"
        iptables_owned.write_text(
            '// Native-owned iptables boundary\n'
            'const std = @import("std");\n'
            'pub fn runIptablesReal(argv: []const []const u8) !c_int {\n'
            '    // execve syscall - owned boundary\n'
            '    return 0;\n'
            '}\n'
        )
        with open(inventory_file, 'w') as f:
            f.write("""id,path,language,pattern,owner_area,runtime_classification,frequency,allowed,justification,timeout_bounded,output_bounded,redaction_required,replacement_candidate,status,notes
CLI001,tovarisch/src/net/iptables.zig,zig,execve syscall,tovarisch,critical_runtime,rare,yes,NATIVE-OWNED: typed API boundary,yes,yes,no,yes,verified,NATIVE-OWNED iptables boundary
CLI002,scripts/,shell,shebang,tooling,ci_cd,periodic,yes,All shell scripts,yes,yes,no,no,verified,Shell scripts
""")
        entries, load_errors = load_inventory(inventory_file)
        detected = scan_codebase(tmppath)
        errors, _, _ = verify_inventory(entries, detected, tmppath)
        assert_test(len(errors) == 0, "Owned iptables module passes", f"Errors: {errors}")

        # Test 14: Native-owned boundary rejects ad-hoc iptables outside owned module
        print("\nTest 14: Native-owned boundary rejects ad-hoc iptables outside owned module")
        # Create a file with execve but NOT in the owned module
        other_file = src_dir / "other.zig"
        other_file.write_text(
            'const std = @import("std");\n'
            'pub fn someFunction() void {\n'
            '    // Direct execve outside owned boundary - should fail\n'
            '    _ = std.c.execve("/sbin/iptables", args, &.{});\n'
            '}\n'
        )
        # Inventory only has the owned module
        with open(inventory_file, 'w') as f:
            f.write("""id,path,language,pattern,owner_area,runtime_classification,frequency,allowed,justification,timeout_bounded,output_bounded,redaction_required,replacement_candidate,status,notes
CLI001,tovarisch/src/net/iptables.zig,zig,execve syscall,tovarisch,critical_runtime,rare,yes,NATIVE-OWNED: typed API boundary,yes,yes,no,yes,verified,NATIVE-OWNED iptables boundary
CLI002,scripts/,shell,shebang,tooling,ci_cd,periodic,yes,All shell scripts,yes,yes,no,no,verified,Shell scripts
""")
        entries, load_errors = load_inventory(inventory_file)
        detected = scan_codebase(tmppath)
        errors, _, _ = verify_inventory(entries, detected, tmppath)
        # Should fail because other.zig has execve but no inventory entry
        assert_test(len(errors) > 0, "Rejects ad-hoc iptables outside owned module", f"Errors: {errors}")

        # Test 15: Native-owned boundary allows docs references
        print("\nTest 15: Native-owned boundary allows docs references")
        # Clean up tovarisch directory first
        if tovarisch_dir.exists():
            shutil.rmtree(tovarisch_dir)
        # Re-create the tovarisch directory with the iptables.zig file
        # so the inventory entry with status=verified passes
        tovarisch_dir.mkdir()
        src_dir = tovarisch_dir / "src"
        src_dir.mkdir()
        net_dir = src_dir / "net"
        net_dir.mkdir()
        iptables_owned = net_dir / "iptables.zig"
        # Include 'execve' pattern so it's detected
        iptables_owned.write_text(
            '// Native-owned iptables boundary\n'
            'const std = @import("std");\n'
            'pub fn runIptablesReal(argv: []const []const u8) !c_int {\n'
            '    _ = std.c.execve("/sbin/iptables", args, &.{});\n'
            '    return 0;\n'
            '}\n'
        )
        docs_dir = tmppath / "docs"
        docs_dir.mkdir()
        inventory_md = docs_dir / "cli-composition-inventory.csv"
        inventory_md.write_text("""id,path,language,pattern,owner_area,runtime_classification,frequency,allowed,justification,timeout_bounded,output_bounded,redaction_required,replacement_candidate,status,notes
CLI001,tovarisch/src/net/iptables.zig,zig,execve syscall,tovarisch,critical_runtime,rare,yes,NATIVE-OWNED,yes,yes,no,yes,verified,NATIVE-OWNED
CLI002,scripts/,shell,shebang,tooling,ci_cd,periodic,yes,All shell scripts,yes,yes,no,no,verified,Shell scripts
""")
        with open(inventory_file, 'w') as f:
            f.write("""id,path,language,pattern,owner_area,runtime_classification,frequency,allowed,justification,timeout_bounded,output_bounded,redaction_required,replacement_candidate,status,notes
CLI001,tovarisch/src/net/iptables.zig,zig,execve syscall,tovarisch,critical_runtime,rare,yes,NATIVE-OWNED,yes,yes,no,yes,verified,NATIVE-OWNED
CLI002,scripts/,shell,shebang,tooling,ci_cd,periodic,yes,All shell scripts,yes,yes,no,no,verified,Shell scripts
""")
        entries, load_errors = load_inventory(inventory_file)
        detected = scan_codebase(tmppath)
        errors, _, _ = verify_inventory(entries, detected, tmppath)
        assert_test(len(errors) == 0, "Allows docs inventory references", f"Errors: {errors}")

    print(f"\n=== Self-Test Results ===")
    print(f"Passed: {tests_passed}")
    print(f"Failed: {tests_failed}")
    return tests_failed == 0


# ---------------------------------------------------------------------------
# Main entry point
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description="Verify CLI composition inventory compliance")
    parser.add_argument("--self-test", action="store_true", help="Run self-test mode")
    parser.add_argument("--verbose", "-v", action="store_true", help="Verbose output")
    args = parser.parse_args()

    if args.self_test:
        success = run_self_test()
        sys.exit(0 if success else 1)

    repo_root = Path(__file__).parent.parent
    inventory_path = repo_root / "docs" / "tooling" / "cli-composition-inventory.csv"

    if not inventory_path.exists():
        print(f"ERROR: Inventory CSV not found at {inventory_path}")
        sys.exit(1)

    print(f"Loading inventory from {inventory_path}")
    entries, errors = load_inventory(inventory_path)
    if errors:
        print("\nCSV LOAD ERRORS:")
        for e in errors:
            print(f"  - {e}")
        sys.exit(1)

    print(f"Loaded {len(entries)} inventory entries")

    print("Scanning codebase for CLI patterns...")
    detected = scan_codebase(repo_root)
    print(f"Detected {len(detected)} CLI usage sites")

    errors, warnings, backlog = verify_inventory(entries, detected, repo_root, args.verbose)

    if errors:
        print("\n=== VERIFICATION FAILED ===")
        print("\nErrors:")
        for e in errors:
            print(f"  - {e}")
        sys.exit(1)

    if warnings:
        print("\nWarnings:")
        for w in warnings:
            print(f"  - {w}")

    if backlog:
        print("\n=== CLI Composition Migration Backlog ===")
        for i, (score, eid, path, classification, freq, pattern) in enumerate(backlog[:10], 1):
            print(f"{i}. {eid} {path} score={score} ({classification}/{freq}/{pattern})")

    print("\n=== VERIFICATION PASSED ===")
    sys.exit(0)


if __name__ == "__main__":
    main()
