#!/usr/bin/env python3
"""
Verifier for AXIOM-2: Cold-Resume Checkpoint.

Validates that ACT close reports have required structural markers for cold-resume:
- Status
- Summary
- Files changed
- Verification
- Caveats
- Next step

Supports embedded ACT slices inside epic documents.
Uses legacy allowlist for historical epics.
"""

import csv
import os
import re
import sys
import tempfile
from typing import Dict, List, Tuple

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# Checkpoint block start patterns (conservative matching)
CHECKPOINT_START_PATTERNS = [
    re.compile(r"^#{1,3}\s+ACT\s+\d", re.MULTILINE),
    re.compile(r"^#{1,3}\s+\[\s*(Closed|Open|Closed pending commit hygiene)\s*\]\s+ACT\s+\d", re.MULTILINE),
    re.compile(r"^#{1,3}\s+ACT\s+\d+\s+Scope", re.MULTILINE),
    re.compile(r"^#{1,3}\s+ACT\s+\d+\.\d+", re.MULTILINE),
    re.compile(r"^#{1,3}\s+ACT\s+\d.*?\n.*?Status:?\s*(Complete|done)", re.MULTILINE | re.DOTALL),
]

# Status marker patterns
STATUS_PATTERNS = [
    re.compile(r"\[\s*Closed\s*\]", re.MULTILINE),
    re.compile(r"\[\s*Closed\s+pending\s+commit\s+hygiene\s*\]", re.MULTILINE),
    re.compile(r"\[\s*Open\s*\]", re.MULTILINE),
    re.compile(r"\*\*Status\*\*:?\s*(Complete|done|closed)", re.MULTILINE),
    re.compile(r"\*\*Status\*\*:?\s*(open|in\s+progress)", re.MULTILINE),
    re.compile(r"Status:?\s*(Complete|done|closed)", re.MULTILINE),
    re.compile(r"Status:?\s*(open|in\s+progress|pending)", re.MULTILINE),
]

# Summary patterns
SUMMARY_PATTERNS = [
    re.compile(r"### Summary", re.MULTILINE),
    re.compile(r"## Summary", re.MULTILINE),
    re.compile(r"# Summary", re.MULTILINE),
    re.compile(r"### Completion summary", re.MULTILINE),
    re.compile(r"### Closure Summary", re.MULTILINE),
]

# Files changed patterns
FILES_CHANGED_PATTERNS = [
    re.compile(r"### Files Changed", re.MULTILINE),
    re.compile(r"## Files Changed", re.MULTILINE),
    re.compile(r"# Files Changed", re.MULTILINE),
    re.compile(r"### Files changed", re.MULTILINE),
]

# Verification patterns
VERIFICATION_PATTERNS = [
    re.compile(r"### Verification", re.MULTILINE),
    re.compile(r"## Verification", re.MULTILINE),
    re.compile(r"make gate", re.MULTILINE),
]

# Caveats patterns
CAVEATS_PATTERNS = [
    re.compile(r"### Caveats", re.MULTILINE),
    re.compile(r"## Caveats", re.MULTILINE),
    re.compile(r"### Known caveats", re.MULTILINE),
    re.compile(r"### Future Work", re.MULTILINE),
    re.compile(r"### Deferred", re.MULTILINE),
]

# Next step patterns
NEXT_STEP_PATTERNS = [
    re.compile(r"### Next Step", re.MULTILINE),
    re.compile(r"### Next exact step", re.MULTILINE),
    re.compile(r"## Next Step", re.MULTILINE),
    re.compile(r"### Next", re.MULTILINE),
]


def load_legacy_allowlist(repo_root: str) -> Dict[str, List[str]]:
    """Load legacy allowlist for incomplete historical checkpoints."""
    allowlist_path = os.path.join(repo_root, "docs", "reference_allowlists", 
                                   "cold_resume_checkpoint_legacy_allowlist.csv")
    allowlist = {}
    
    if not os.path.exists(allowlist_path):
        return allowlist
    
    try:
        with open(allowlist_path, "r", newline="", encoding="utf-8") as f:
            for row in csv.DictReader(f):
                path = row.get("path", "").strip()
                if path:
                    normalized = path
                    if path.startswith("docs/epics/"):
                        normalized = path[len("docs/epics/"):]
                    allowlist[normalized] = [
                        m.strip() for m in row.get("allowed_missing_markers", "").split(",")
                        if m.strip()
                    ]
    except Exception as e:
        print(f"    WARNING: Could not load allowlist: {e}")
    
    return allowlist


def find_checkpoint_blocks(content: str) -> List[Tuple[int, int, str]]:
    """Find all checkpoint blocks in content."""
    blocks = []
    lines = content.split("\n")
    
    for i, line in enumerate(lines):
        for pattern in CHECKPOINT_START_PATTERNS:
            if pattern.search(line):
                block_start = i
                block_end = len(lines)
                current_level = len(line) - len(line.lstrip("#"))
                
                for j in range(i + 1, len(lines)):
                    if re.match(r"^#+\s+", lines[j]):
                        heading_level = len(lines[j]) - len(lines[j].lstrip("#"))
                        if heading_level <= current_level:
                            block_end = j
                            break
                
                block_text = "\n".join(lines[block_start:block_end])
                blocks.append((block_start, block_end, block_text))
                break
    
    return blocks


def is_closed_checkpoint(block_text: str) -> bool:
    """Check if a checkpoint block represents a closed/completed ACT."""
    closed_patterns = [
        r"\[\s*Closed\s*\]",
        r"\[\s*Closed\s+pending\s+commit\s+hygiene\s*\]",
        r"Status:?\s*(Complete|done|closed)",
        r"\*\*Status\*\*:?.*?(Complete|done|closed)",
    ]
    
    for pattern in closed_patterns:
        if re.search(pattern, block_text, re.IGNORECASE):
            return True
    
    return False


def check_marker(block_text: str, patterns: List[re.Pattern]) -> bool:
    """Check if a marker exists in block text."""
    for pattern in patterns:
        if pattern.search(block_text):
            return True
    return False


def validate_checkpoint_block(block_text: str, is_closed: bool) -> Tuple[bool, List[str]]:
    """Validate a checkpoint block for required markers."""
    errors = []
    
    if not check_marker(block_text, STATUS_PATTERNS):
        errors.append("status")
    
    if is_closed:
        if not check_marker(block_text, SUMMARY_PATTERNS):
            errors.append("summary")
        if not check_marker(block_text, FILES_CHANGED_PATTERNS):
            errors.append("files_changed")
        if not check_marker(block_text, VERIFICATION_PATTERNS):
            errors.append("verification")
        if not check_marker(block_text, CAVEATS_PATTERNS):
            errors.append("caveats")
        if not check_marker(block_text, NEXT_STEP_PATTERNS):
            errors.append("next_step")
    else:
        if not check_marker(block_text, NEXT_STEP_PATTERNS):
            errors.append("next_step")
    
    return len(errors) == 0, errors


def scan_epic_file(filepath: str, allowlist: Dict[str, List[str]]) -> Dict:
    """Scan a single epic file for checkpoint blocks."""
    result = {
        "file": filepath,
        "files_checked": 1,
        "blocks_found": 0,
        "closed_checked": 0,
        "open_checked": 0,
        "valid": 0,
        "allowlisted_gaps": 0,
        "failures": [],
        "is_allowlisted": False,
        "allowlisted_missing": [],
    }
    
    filename = os.path.basename(filepath)
    if filename in allowlist:
        result["is_allowlisted"] = True
        result["allowlisted_missing"] = allowlist[filename]
    
    try:
        with open(filepath, "r", encoding="utf-8", errors="ignore") as f:
            content = f.read()
    except Exception:
        result["failures"].append("Could not read file")
        return result
    
    blocks = find_checkpoint_blocks(content)
    result["blocks_found"] = len(blocks)
    
    for start, end, block_text in blocks:
        is_closed = is_closed_checkpoint(block_text)
        
        if is_closed:
            result["closed_checked"] += 1
        else:
            result["open_checked"] += 1
        
        if result["is_allowlisted"]:
            valid, errors = validate_checkpoint_block(block_text, is_closed)
            filtered_errors = [e for e in errors if e not in result["allowlisted_missing"]]
            if filtered_errors:
                result["failures"].append(f"Block at line {start + 1}: missing {filtered_errors}")
            else:
                result["valid"] += 1
                if errors:
                    result["allowlisted_gaps"] += len(errors)
        else:
            valid, errors = validate_checkpoint_block(block_text, is_closed)
            if valid:
                result["valid"] += 1
            else:
                result["failures"].append(f"Block at line {start + 1}: missing {errors}")
    
    return result


def run_verifier(repo_root: str) -> List[str]:
    """Run the cold-resume checkpoint verifier."""
    all_errors = []
    epics_dir = os.path.join(repo_root, "docs", "epics")
    
    print("=== AXIOM-2: Cold-Resume Checkpoint Verifier ===\n")
    
    allowlist = load_legacy_allowlist(repo_root)
    print(f"Loaded allowlist with {len(allowlist)} entries")
    
    if not os.path.isdir(epics_dir):
        all_errors.append("docs/epics/ does not exist")
        return all_errors
    
    files_checked = 0
    total_blocks = 0
    total_closed = 0
    total_open = 0
    total_valid = 0
    total_allowlisted_gaps = 0
    files_with_failures = []
    
    for fname in sorted(os.listdir(epics_dir)):
        if not fname.endswith(".md"):
            continue
        
        filepath = os.path.join(epics_dir, fname)
        result = scan_epic_file(filepath, allowlist)
        
        files_checked += 1
        total_blocks += result["blocks_found"]
        total_closed += result["closed_checked"]
        total_open += result["open_checked"]
        total_valid += result["valid"]
        total_allowlisted_gaps += result["allowlisted_gaps"]
        
        if result["failures"]:
            files_with_failures.append(result)
        
        if result["blocks_found"] > 0:
            rel_path = os.path.relpath(filepath, repo_root)
            print(f"  {rel_path}:")
            print(f"    Blocks: {result['blocks_found']}, "
                  f"Closed: {result['closed_checked']}, "
                  f"Open: {result['open_checked']}, "
                  f"Valid: {result['valid']}")
            if result["is_allowlisted"]:
                print(f"    Allowlisted (missing: {result['allowlisted_missing']})")
    
    print(f"\n  Summary:")
    print(f"    Files checked: {files_checked}")
    print(f"    Checkpoint blocks found: {total_blocks}")
    print(f"    Closed checkpoints checked: {total_closed}")
    print(f"    Open checkpoints checked: {total_open}")
    print(f"    Fully valid checkpoints: {total_valid}")
    print(f"    Allowlisted legacy gaps: {total_allowlisted_gaps}")
    
    if files_with_failures:
        print(f"\n  Failures ({len(files_with_failures)} files):")
        for result in files_with_failures:
            print(f"    {os.path.relpath(result['file'], repo_root)}:")
            for failure in result["failures"]:
                print(f"      - {failure}")
            all_errors.append(f"{os.path.relpath(result['file'], repo_root)}: {len(result['failures'])} failure(s)")
    
    return all_errors


def run_self_tests() -> bool:
    """Run self-tests using temporary directories."""
    print("\n=== Running Self-Tests ===\n")
    
    # Load fixtures from separate module
    fixtures_path = os.path.join(SCRIPT_DIR, "verify_cold_resume_checkpoints_fixtures.py")
    fixtures_mod = __import__("types").ModuleType("_fixtures")
    with open(fixtures_path) as f:
        exec(compile(f.read(), fixtures_path, "exec"), fixtures_mod.__dict__)
    test_fixtures = fixtures_mod.TEST_FIXTURES
    
    tests_passed, tests_failed = 0, 0
    
    def run_test(name: str, epic_content: str, allowlist_content: str, should_pass: bool) -> bool:
        nonlocal tests_passed, tests_failed
        
        with tempfile.TemporaryDirectory() as tmpdir:
            epics_dir = os.path.join(tmpdir, "docs", "epics")
            allowlists_dir = os.path.join(tmpdir, "docs", "reference_allowlists")
            os.makedirs(epics_dir, exist_ok=True)
            os.makedirs(allowlists_dir, exist_ok=True)
            
            with open(os.path.join(epics_dir, "test-epic.md"), "w") as f:
                f.write(epic_content)
            
            if allowlist_content:
                with open(os.path.join(allowlists_dir, 
                                       "cold_resume_checkpoint_legacy_allowlist.csv"), "w") as f:
                    f.write(allowlist_content)
            
            errors = run_verifier(tmpdir)
            passed = len(errors) == 0
            
            if passed == should_pass:
                print(f"  PASS: {name}")
                tests_passed += 1
                return True
            else:
                print(f"  FAIL: {name}")
                print(f"    Expected: {'pass' if should_pass else 'fail'}, Got: {'pass' if passed else 'fail'}")
                if errors:
                    print(f"    Errors: {errors}")
                tests_failed += 1
                return False
    
    for fixture in test_fixtures:
        run_test(
            fixture["name"],
            fixture["epic_content"],
            fixture["allowlist_content"],
            fixture["should_pass"]
        )
    
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
        print("Cold-resume checkpoint structure is valid.")
        sys.exit(0)


if __name__ == "__main__":
    main()
