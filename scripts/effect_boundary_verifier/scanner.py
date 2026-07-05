"""
Scanner module for effect boundary verifier.

Contains scanning logic for detecting effect violations in Zig modules.
"""

import os
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import List, Set, Tuple

from .classifications import classify_module, is_test_file
from .patterns import FORBIDDEN_PATTERNS, strip_comments_and_tests


@dataclass
class Violation:
    """Represents a violation found in a module."""
    file: str
    pattern: str
    description: str
    line: int
    line_content: str


def check_for_violations(file_path: Path, content: str) -> List[Violation]:
    """
    Check a file for forbidden effect patterns.
    
    Args:
        file_path: Path to the file being checked
        content: File content to check
        
    Returns:
        List of Violation objects found
    """
    violations = []
    
    # Strip comments and test blocks to avoid false positives
    content_no_comments = strip_comments_and_tests(content)
    
    for pattern, description in FORBIDDEN_PATTERNS:
        regex = re.compile(pattern)
        for match in regex.finditer(content_no_comments):
            # Find line number in the stripped content
            line_num = content_no_comments[:match.start()].count('\n') + 1
            
            # Get the line content from original content
            lines = content.split('\n')
            line_content = lines[line_num - 1] if line_num <= len(lines) else ""
            
            violations.append(Violation(
                file=str(file_path),
                pattern=pattern,
                description=description,
                line=line_num,
                line_content=line_content.strip()
            ))
    
    return violations


def check_imports(file_path: Path, content: str) -> List[Tuple[str, int]]:
    """
    Check for production imports of test files.
    
    Args:
        file_path: Path to the file being checked
        content: File content to check
        
    Returns:
        List of tuples (imported_module, line_number)
    """
    violations = []
    
    # Pattern to match @import statements
    import_pattern = re.compile(r'@import\("([^"]+)"\)')
    
    for match in import_pattern.finditer(content):
        imported = match.group(1)
        line_num = content[:match.start()].count('\n') + 1
        
        # Check if importing a test file
        imported_name = os.path.basename(imported)
        if is_test_file(Path(imported_name)):
            violations.append((imported, line_num))
    
    return violations


def scan_directory(
    base_dir: Path,
    pure_set: Set[str],
    boundary_set: Set[str],
    stateful_set: Set[str],
    deferred_set: Set[str],
    force_modules: Set[str] = None,
) -> Tuple[List[Violation], List[Tuple[str, str, int]], List[Tuple[str, str]]]:
    """
    Scan directory for violations.
    
    Args:
        base_dir: Base directory to scan
        pure_set: Set of PURE module paths
        boundary_set: Set of BOUNDARY module paths
        stateful_set: Set of STATEFUL module paths
        deferred_set: Set of DEFERRED module paths
        force_modules: Optional set of modules to force as PURE (for testing)
        
    Returns:
        Tuple containing:
        - List of PURE violations
        - List of (file, imported_test, line) tuples for test imports
        - List of (file, reason) for deferred/unknown modules
    """
    violations = []
    test_imports = []
    deferred_modules = []
    
    src_dir = base_dir / "tovarisch" / "src"
    
    if not src_dir.exists():
        # For self-test, scan base_dir directly for .zig files
        src_dir = base_dir
    
    if not src_dir.exists():
        print(f"[ERROR] Source directory not found: {src_dir}", file=sys.stderr)
        return violations, test_imports, deferred_modules
    
    # Find all .zig files
    for zig_file in src_dir.rglob("*.zig"):
        rel_path = zig_file.relative_to(base_dir)
        module_path = str(rel_path)
        
        # Skip test files for effect pattern checking
        if is_test_file(zig_file):
            continue
        
        try:
            content = zig_file.read_text()
        except Exception as e:
            print(f"[WARN] Could not read {zig_file}: {e}", file=sys.stderr)
            continue
        
        classification = classify_module(
            module_path, pure_set, boundary_set, stateful_set, deferred_set
        )
        
        # Check for forced modules (for self-test)
        if force_modules and module_path in force_modules:
            classification = "PURE"
        
        if classification == "PURE":
            # Check for effect violations in PURE modules
            file_violations = check_for_violations(zig_file, content)
            violations.extend(file_violations)
        
        # Check for test imports in production files
        if classification != "TEST":
            imports = check_imports(zig_file, content)
            for imported, line_num in imports:
                test_imports.append((str(rel_path), imported, line_num))
        
        # Track deferred/unknown modules
        if classification in ("DEFERRED", "UNKNOWN"):
            deferred_modules.append((str(rel_path), classification))
    
    return violations, test_imports, deferred_modules
