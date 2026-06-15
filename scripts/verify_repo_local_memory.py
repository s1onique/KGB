#!/usr/bin/env python3
"""
Verifier for AXIOM-1: Repo-Local Project Memory.

Validates structural memory artifacts:
- Required doctrine anchors exist
- AXIOM-1 covered in manifesto matrix
- Epic/WAL pairing exists
- Bootstrap discoverability maintained
- Close report structure present (advisory)
"""

import csv
import os
import re
import sys
import tempfile
from typing import List, Tuple

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

REQUIRED_ANCHORS = [
    "docs/doctrine/ai-native-code-discipline-axioms.md",
    "docs/doctrine/manifesto_axiom_coverage.csv",
    "AGENTS.md",
    ".clinerules/00-bootstrap.md",
]

OPTIONAL_DOCTRINE_INDEX = "docs/doctrine/README.md"

BOOTSTRAP_REFERENCES = {
    "AGENTS.md": ["docs/doctrine/ai-native-code-discipline-axioms.md", "ai-native-code-discipline-axioms.md"],
    ".clinerules/00-bootstrap.md": ["AGENTS.md", "docs/epics/"],
    "docs/doctrine/README.md": ["ai-native-code-discipline-axioms.md", "doctrine/ai-native-code-discipline-axioms.md"],
}

CLOSE_REPORT_MARKERS = [
    re.compile(r"^#.*files?\s*changed", re.IGNORECASE | re.MULTILINE),
    re.compile(r"^#.*verification", re.IGNORECASE | re.MULTILINE),
    re.compile(r"^#.*(caveats?|next\s*(exact\s*)?step|recommended\s*next)", re.IGNORECASE | re.MULTILINE),
]

WAL_SLICE_PATTERNS = [
    re.compile(r"^#.*ACT\s*\d+[a-z]?\s*:", re.IGNORECASE | re.MULTILINE),
    re.compile(r"^\|.*Status.*\|", re.MULTILINE),
    re.compile(r"^\*\*Status.*\*\*", re.MULTILINE),
]


def check_required_anchors(repo_root: str) -> List[str]:
    errors = []
    for anchor in REQUIRED_ANCHORS:
        path = os.path.join(repo_root, anchor)
        if not os.path.exists(path):
            errors.append(f"Required anchor missing: {anchor}")
        elif not os.path.getsize(path) > 0:
            errors.append(f"Required anchor empty: {anchor}")
    return errors


def check_doctrine_index(repo_root: str) -> List[str]:
    errors = []
    index_path = os.path.join(repo_root, OPTIONAL_DOCTRINE_INDEX)
    if os.path.exists(index_path) and os.path.getsize(index_path) > 0:
        with open(index_path, "r", encoding="utf-8") as f:
            if "axiom" not in f.read().lower():
                errors.append("docs/doctrine/README.md exists but does not reference axioms")
    return errors


def load_matrix(path: str) -> Tuple[List[dict], List[str]]:
    errors, rows = [], []
    if not os.path.exists(path):
        return [], [f"Matrix file not found: {path}"]
    try:
        with open(path, "r", newline="", encoding="utf-8") as f:
            for row in csv.DictReader(f):
                rows.append(row)
    except Exception as e:
        return [], [f"Error reading matrix: {e}"]
    return rows, errors


def check_axiom1_matrix_coverage(rows: List[dict]) -> List[str]:
    errors = []
    axiom1_rows = [r for r in rows if r.get("axiom_id", "").strip() == "AXIOM-1"]
    if not axiom1_rows:
        return ["AXIOM-1 (Repo-Local Project Memory) not found in matrix"]
    
    repo_areas = [r.get("repo_area", "").strip() for r in axiom1_rows]
    if not any(r.get("repo_area", "").strip() == "doctrine" for r in axiom1_rows):
        errors.append("AXIOM-1 missing 'doctrine' row in matrix")
    if not ("epics" in repo_areas or "wal" in repo_areas):
        errors.append("AXIOM-1 missing 'epics' or 'wal' row in matrix")
    if not any(r.get("repo_area", "").strip() == "gates" for r in axiom1_rows):
        errors.append("AXIOM-1 missing 'gates' row in matrix")
    return errors


def find_epics_and_wals(repo_root: str) -> Tuple[List[str], List[str], List[Tuple[str, str]], List[str]]:
    epics_dir = os.path.join(repo_root, "docs", "epics")
    epics, wals, paired, orphans = [], [], [], []
    
    if not os.path.isdir(epics_dir):
        return epics, wals, paired, orphans
    
    for fname in os.listdir(epics_dir):
        if not fname.endswith(".md"):
            continue
        fpath = os.path.join(epics_dir, fname)
        if "act-" in fname.lower() or "epic" in fname.lower():
            epics.append(fpath)
        elif "wal" in fname.lower():
            wals.append(fpath)
        else:
            epics.append(fpath)
    
    for epic_path in epics:
        with open(epic_path, "r", encoding="utf-8", errors="ignore") as f:
            content = f.read()
        if any(p.search(content) for p in WAL_SLICE_PATTERNS):
            paired.append((epic_path, "embedded-slices"))
    
    for wal_path in wals:
        wal_base = os.path.basename(wal_path).replace("-wal.md", "").replace("_wal.md", "")
        matched = False
        for epic_path in epics:
            if wal_base in os.path.basename(epic_path) or os.path.basename(epic_path).replace(".md", "") in wal_base:
                paired.append((epic_path, wal_path))
                matched = True
                break
        if not matched:
            orphans.append(wal_path)
    
    return epics, wals, paired, orphans


def check_epic_wal_pairing(repo_root: str) -> List[str]:
    errors = []
    epics, wals, paired, orphans = find_epics_and_wals(repo_root)
    
    print(f"\n  Epic/WAL Summary:")
    print(f"    Total epics: {len(epics)}")
    print(f"    Total WAL files: {len(wals)}")
    print(f"    Paired: {len(paired)}")
    print(f"    Orphan WALs: {len(orphans)}")
    
    if not epics and not wals:
        matrix_path = os.path.join(repo_root, "docs", "doctrine", "manifesto_axiom_coverage.csv")
        if os.path.exists(matrix_path):
            rows, _ = load_matrix(matrix_path)
            axiom1_rows = [r for r in rows if r.get("axiom_id", "").strip() == "AXIOM-1"]
            if axiom1_rows and "complete" in [r.get("status", "").strip() for r in axiom1_rows]:
                errors.append("docs/epics/ missing but AXIOM-1 claims complete")
    
    if orphans:
        errors.append(f"Orphan WAL files found: {[os.path.basename(o) for o in orphans]}")
    return errors


def check_bootstrap_discoverability(repo_root: str) -> List[str]:
    errors = []
    for file_path, expected_refs in BOOTSTRAP_REFERENCES.items():
        full_path = os.path.join(repo_root, file_path)
        if not os.path.exists(full_path):
            continue
        with open(full_path, "r", encoding="utf-8") as f:
            content = f.read()
        if not any(ref.lower() in content.lower() for ref in expected_refs):
            errors.append(f"{file_path} does not reference doctrine/bootstrap material")
    return errors


def check_close_report_structure(repo_root: str) -> Tuple[List[str], bool]:
    errors = []
    epics_dir = os.path.join(repo_root, "docs", "epics")
    
    if not os.path.isdir(epics_dir):
        return ["docs/epics/ does not exist"], False
    
    files_checked, files_with_markers = 0, 0
    for fname in os.listdir(epics_dir):
        if not fname.endswith(".md"):
            continue
        with open(os.path.join(epics_dir, fname), "r", encoding="utf-8", errors="ignore") as f:
            content = f.read()
        files_checked += 1
        if sum(1 for p in CLOSE_REPORT_MARKERS if p.search(content)) >= 2:
            files_with_markers += 1
    
    if files_checked > 0:
        pct = (files_with_markers / files_checked) * 100
        print(f"\n  Close Report Structure (advisory):")
        print(f"    Files checked: {files_checked}")
        print(f"    Files with markers: {files_with_markers} ({pct:.0f}%)")
        if pct < 50:
            print(f"    WARNING: Less than 50% of epics have close report structure")
    return errors, files_with_markers > 0


def run_verifier(repo_root: str) -> List[str]:
    all_errors = []
    print("=== AXIOM-1: Repo-Local Project Memory Verifier ===\n")
    
    print("A. Checking required memory anchors...")
    errors = check_required_anchors(repo_root)
    if errors:
        for e in errors:
            print(f"    ERROR: {e}")
        all_errors.extend(errors)
    else:
        print(f"    OK: All {len(REQUIRED_ANCHORS)} required anchors exist")
    
    errors = check_doctrine_index(repo_root)
    if not errors:
        print(f"    OK: {OPTIONAL_DOCTRINE_INDEX} references axiom doctrine")
    all_errors.extend(errors)
    
    print("\nB. Checking AXIOM-1 matrix coverage...")
    matrix_path = os.path.join(repo_root, "docs", "doctrine", "manifesto_axiom_coverage.csv")
    rows, errors = load_matrix(matrix_path)
    if errors:
        all_errors.extend(errors)
    else:
        errors = check_axiom1_matrix_coverage(rows)
        if errors:
            for e in errors:
                print(f"    ERROR: {e}")
            all_errors.extend(errors)
        else:
            print(f"    OK: AXIOM-1 has required matrix rows")
    
    print("\nC. Checking epic/WAL pairing...")
    errors = check_epic_wal_pairing(repo_root)
    if errors:
        for e in errors:
            print(f"    ERROR: {e}")
        all_errors.extend(errors)
    else:
        print(f"    OK: Epic/WAL pairing structure is valid")
    
    print("\nD. Checking close report structure...")
    errors, has_reports = check_close_report_structure(repo_root)
    all_errors.extend(errors)
    print(f"    {'OK' if has_reports else 'ADVISORY'}: Close report structure {'found' if has_reports else 'not a hard failure'}")
    
    print("\nE. Checking bootstrap discoverability...")
    errors = check_bootstrap_discoverability(repo_root)
    if errors:
        for e in errors:
            print(f"    ERROR: {e}")
        all_errors.extend(errors)
    else:
        print(f"    OK: Bootstrap discoverability verified")
    
    return all_errors


def run_self_tests() -> bool:
    print("\n=== Running Self-Tests ===\n")
    
    # Load fixtures directly from file
    fixtures_path = os.path.join(SCRIPT_DIR, "verify_repo_local_memory_fixtures.py")
    fixtures_mod = __import__("types").ModuleType("_fixtures")
    with open(fixtures_path) as f:
        exec(compile(f.read(), fixtures_path, "exec"), fixtures_mod.__dict__)
    create_test_fixture = fixtures_mod.create_test_fixture
    
    tests_passed, tests_failed = 0, 0
    
    def run_test(name: str, variant: str, should_pass: bool) -> bool:
        nonlocal tests_passed, tests_failed
        
        with tempfile.TemporaryDirectory() as tmpdir:
            create_test_fixture(tmpdir, variant)
            
            with open(__file__, "r") as f:
                source = f.read()
            
            patched = source.replace(
                "REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))",
                f"REPO_ROOT = {repr(tmpdir)}"
            )
            
            mod = __import__("types").ModuleType("test")
            mod.__file__ = __file__
            exec(compile(patched, __file__, "exec"), mod.__dict__)
            
            errors = mod.run_verifier(tmpdir)
            passed = len(errors) == 0
            
            if passed == should_pass:
                print(f"  PASS: {name}")
                tests_passed += 1
                return True
            else:
                print(f"  FAIL: {name}")
                print(f"    Expected: {'pass' if should_pass else 'fail'}, Got: {'pass' if passed else 'fail'}")
                tests_failed += 1
                return False
    
    run_test("valid fixture passes", "valid", True)
    run_test("missing required anchor fails", "missing_anchor", False)
    run_test("AXIOM-1 missing from matrix fails", "missing_axiom1", False)
    run_test("AXIOM-1 missing epics/wal coverage fails", "missing_epics_wal", False)
    run_test("orphan WAL fails", "orphan_wal", False)
    run_test("missing bootstrap reference fails", "missing_bootstrap_ref", False)
    
    print(f"\n=== Self-Test Results ===")
    print(f"  Passed: {tests_passed}, Failed: {tests_failed}")
    return tests_failed == 0


def main():
    if "--self-test" in sys.argv:
        sys.exit(0 if run_self_tests() else 1)
    
    errors = run_verifier(REPO_ROOT)
    print("\n" + "=" * 50)
    
    if errors:
        print("\nVERIFICATION FAILED:")
        for e in errors:
            print(f"  - {e}")
        sys.exit(1)
    else:
        print("\nVERIFICATION PASSED")
        print("AXIOM-1 repo-local memory structure is valid.")
        sys.exit(0)


if __name__ == "__main__":
    main()
