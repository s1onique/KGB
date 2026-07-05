# scanner.py — Source code scanner for total parser verification
"""Scanner for detecting forbidden patterns in tovarisch parser modules."""

import os
import re
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import List, Optional, Set, Tuple

from .classifications import (
    Classification,
    get_module_classification,
    is_deferred,
    is_stateful_adapter,
    requires_strict_check,
    STATEFUL_ADAPTER_MODULES,
)
from .patterns import (
    FORBIDDEN_PATTERNS,
    MEDIUM_PATTERNS,
    is_forbidden_pattern,
    is_medium_pattern,
    strip_comments,
    strip_tests,
)


class FindingSeverity(Enum):
    """Severity levels for scanner findings."""
    FAIL = "FAIL"      # Must be fixed
    WARN = "WARN"      # Should review
    INFO = "INFO"      # Informational


@dataclass
class Finding:
    """A finding from scanning a source file."""
    severity: FindingSeverity
    module: str
    line_number: int
    line_content: str
    pattern: str
    description: str
    classification: Classification
    
    def __str__(self) -> str:
        """Format finding as a string."""
        location = f"{self.module}:{self.line_number}"
        if self.severity == FindingSeverity.FAIL:
            return f"[{location}] FAIL: {self.description}"
        elif self.severity == FindingSeverity.WARN:
            return f"[{location}] WARN: {self.description}"
        else:
            return f"[{location}] INFO: {self.description}"


@dataclass
class ScanResult:
    """Result of scanning a module."""
    module: str
    classification: Classification
    file_path: str
    findings: List[Finding] = field(default_factory=list)
    errors: List[str] = field(default_factory=list)
    scanned_lines: int = 0
    found_forbidden: int = 0
    found_medium: int = 0
    
    @property
    def has_failures(self) -> bool:
        """Check if scan has any FAIL findings."""
        return any(f.severity == FindingSeverity.FAIL for f in self.findings)
    
    @property
    def has_warnings(self) -> bool:
        """Check if scan has any WARN findings."""
        return any(f.severity == FindingSeverity.WARN for f in self.findings)


def extract_module_name(file_path: str, src_root: str = "tovarisch/src") -> Optional[str]:
    """Extract module name (relative path from src_root) from file path.
    
    Args:
        file_path: Full path to a .zig file
        src_root: Root directory containing source files
        
    Returns:
        Module name like "status_query.zig" or "net/linux_read.zig" or None
    """
    path = Path(file_path).resolve()
    root = Path(src_root).resolve()
    
    if path.suffix == '.zig':
        try:
            # Get relative path from src_root
            rel = path.relative_to(root)
            return str(rel).replace('\\', '/')  # Normalize for cross-platform
        except ValueError:
            # File is outside src_root, use just the filename
            return path.name
    return None


def should_ignore_line(line: str, module: str) -> bool:
    """Check if a line should be ignored (in comment or test).
    
    Args:
        line: Source line
        module: Module name for context
        
    Returns:
        True if line should be ignored
    """
    stripped = line.strip()
    
    # Skip empty lines
    if not stripped:
        return True
    
    # Skip line comments
    if stripped.startswith('//'):
        return True
    
    # Skip block comment start/end
    if stripped.startswith('/*') or stripped.startswith('*/'):
        return True
    
    # Skip test blocks in non-TOTAL modules
    # (tests in TOTAL modules are checked)
    if module in STATEFUL_ADAPTER_MODULES:
        if stripped.startswith('test '):
            return True
    
    return False


def check_line_for_patterns(
    line: str,
    line_number: int,
    module: str,
    classification: Classification,
    in_test_block: bool = False,
) -> List[Finding]:
    """Check a single line for forbidden and medium patterns.
    
    Args:
        line: Source line to check
        line_number: 1-based line number
        module: Module name
        classification: Classification of the module
        in_test_block: Whether line is in a test block
        
    Returns:
        List of findings (may be empty)
    """
    findings = []
    
    # Skip lines in test blocks for production modules
    # (test files like *_test.zig are excluded from production checks)
    if in_test_block:
        return findings
    
    # Check for forbidden patterns
    is_forbidden, desc = is_forbidden_pattern(line)
    if is_forbidden:
        # Determine severity based on classification
        if is_deferred(module):
            # DEFERRED modules report but don't fail
            severity = FindingSeverity.WARN
        elif is_stateful_adapter(module):
            # STATEFUL_ADAPTER has relaxed checking
            # Only @panic and catch unreachable are failures
            if '@panic' in line or 'catch unreachable' in line:
                severity = FindingSeverity.FAIL
            else:
                severity = FindingSeverity.WARN
        else:
            # TOTAL and BOUNDARY_TOTAL fail on forbidden patterns
            severity = FindingSeverity.FAIL
        
        # Get the matched pattern
        pattern = ""
        for pat, _, _ in FORBIDDEN_PATTERNS:
            if re.search(pat, line):
                pattern = pat
                break
        
        findings.append(Finding(
            severity=severity,
            module=module,
            line_number=line_number,
            line_content=line.strip()[:80],
            pattern=pattern,
            description=desc,
            classification=classification,
        ))
    
    # Check for medium patterns (always warnings)
    is_medium, desc = is_medium_pattern(line)
    if is_medium:
        pattern = ""
        for pat, _, _ in MEDIUM_PATTERNS:
            if re.search(pat, line):
                pattern = pat
                break
        
        findings.append(Finding(
            severity=FindingSeverity.WARN,
            module=module,
            line_number=line_number,
            line_content=line.strip()[:80],
            pattern=pattern,
            description=desc,
            classification=classification,
        ))
    
    return findings


def scan_file(file_path: str, src_root: str = "tovarisch/src") -> ScanResult:
    """Scan a single file for forbidden patterns.
    
    Args:
        file_path: Path to the .zig file
        src_root: Root directory containing source files
        
    Returns:
        ScanResult with findings
    """
    module = extract_module_name(file_path, src_root)
    if not module:
        return ScanResult(
            module="unknown",
            classification=Classification.TOTAL,
            file_path=file_path,
            errors=["Not a .zig file"],
        )
    
    try:
        classification = get_module_classification(module)
    except ValueError:
        return ScanResult(
            module=module,
            classification=Classification.TOTAL,
            file_path=file_path,
            errors=[f"Module not in register: {module}"],
        )
    
    result = ScanResult(
        module=module,
        classification=classification,
        file_path=file_path,
    )
    
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            content = f.read()
    except Exception as e:
        result.errors.append(f"Failed to read file: {e}")
        return result
    
    lines = content.split('\n')
    result.scanned_lines = len(lines)
    
    # Track test block depth
    in_test_block = False
    test_brace_depth = 0
    
    for line_number, line in enumerate(lines, start=1):
        # Track test block state
        stripped = line.strip()
        if stripped.startswith('test ') and '{' in line:
            in_test_block = True
            test_brace_depth = 0
        
        if in_test_block:
            test_brace_depth += stripped.count('{') - stripped.count('}')
            if test_brace_depth <= 0 and stripped.endswith('}'):
                in_test_block = False
        
        # Skip comments
        if should_ignore_line(line, module):
            continue
        
        # Check for patterns
        findings = check_line_for_patterns(
            line, line_number, module, classification, in_test_block
        )
        
        for finding in findings:
            if finding.severity == FindingSeverity.FAIL:
                result.found_forbidden += 1
            elif finding.severity == FindingSeverity.WARN:
                result.found_medium += 1
        result.findings.extend(findings)
    
    return result


def scan_modules(src_root: str = "tovarisch/src") -> Tuple[List[ScanResult], List[str]]:
    """Scan all registered parser modules.
    
    Args:
        src_root: Root directory containing source files
        
    Returns:
        Tuple of (list of scan results, list of errors)
    """
    from .classifications import get_all_registered_modules
    
    results = []
    errors = []
    
    for module in get_all_registered_modules():
        # Construct path - modules may be in subdirectories
        file_path = os.path.join(src_root, module)
        
        if not os.path.exists(file_path):
            # Try to find it
            found = False
            for root, _, files in os.walk(src_root):
                if module in files:
                    file_path = os.path.join(root, module)
                    found = True
                    break
            if not found:
                errors.append(f"Module not found: {module}")
                continue
        
        result = scan_file(file_path, src_root)
        results.append(result)
        
        if result.errors:
            errors.extend(result.errors)
    
    return results, errors


def format_results(results: List[ScanResult], verbose: bool = False) -> str:
    """Format scan results as a string.
    
    Args:
        results: List of ScanResult objects
        verbose: Include info-level findings
        
    Returns:
        Formatted string
    """
    lines = []
    
    # Group by classification
    by_class = {}
    for result in results:
        if result.classification not in by_class:
            by_class[result.classification] = []
        by_class[result.classification].append(result)
    
    for classification in sorted(by_class.keys(), key=lambda c: c.value):
        lines.append(f"\n=== {classification.value} modules ===")
        for result in by_class[classification]:
            if result.errors:
                for error in result.errors:
                    lines.append(f"  [ERROR] {result.module}: {error}")
                continue
            
            status = "OK" if not result.findings else f"{len(result.findings)} issues"
            lines.append(f"  {result.module}: {status}")
            
            if verbose:
                for finding in result.findings:
                    lines.append(f"    {finding}")
    
    return "\n".join(lines)


def check_production_imports(results: List[ScanResult]) -> List[str]:
    """Check for production imports of test files.
    
    Args:
        results: List of scan results
        
    Returns:
        List of warnings
    """
    warnings = []
    
    # This is a placeholder - would need to parse Zig imports
    # For now, just check file naming
    for result in results:
        if '_test' in result.module or '_tests' in result.module:
            warnings.append(f"Production module with test-like name: {result.module}")
    
    return warnings
