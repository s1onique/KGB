# patterns.py — Forbidden and medium-risk patterns for total parser verification
"""Pattern definitions for total parser verification.

Forbidden patterns cause verification FAILURE in TOTAL and BOUNDARY_TOTAL modules.
Medium patterns cause warnings but not failures (for @intCast with bounds checks).
"""

import re
from typing import List, Tuple


# Forbidden patterns: FAIL on these in TOTAL/BOUNDARY_TOTAL modules
# Format: (pattern, description, is_regex)
FORBIDDEN_PATTERNS: List[Tuple[str, str, bool]] = [
    # @panic calls - never in external input parsers
    (
        r'@panic\(',
        "@panic call - use error return instead",
        True,
    ),
    
    # unreachable - can be reached by malformed input
    (
        r'\bunreachable\b',
        "'unreachable' - external input should not reach unreachable",
        True,
    ),
    
    # catch unreachable - hiding parse errors
    (
        r'catch\s+unreachable\b',
        "'catch unreachable' - swallows parse errors",
        True,
    ),
    
    # Optional unwrap without explicit handling
    # Note: .? is valid after a null check, but we flag all .? for review
    (
        r'\.\?',
        "optional unwrap '.?' - use 'orelse' for explicit handling",
        True,
    ),
    
    # @enumFromInt without visible bounds validation
    # This is more nuanced - we flag it for review
    (
        r'@enumFromInt\(',
        "@enumFromInt - ensure bounds are validated before cast",
        True,
    ),
]


# Medium risk patterns: Report but don't fail
# These patterns require human review to determine if safe
MEDIUM_PATTERNS: List[Tuple[str, str, bool]] = [
    # @intCast - may be safe with prior bounds check
    (
        r'@intCast\(',
        "@intCast - verify bounds are checked before cast",
        True,
    ),
    
    # std.debug.assert on data that could be external
    (
        r'std\.debug\.assert\(',
        "std.debug.assert - consider if this could receive external input",
        True,
    ),
]


# Patterns that should be ignored (in comments, strings, tests)
IGNORE_CONTEXT_PATTERNS = [
    # Line comments
    (r'//.*', True),
    # Block comments
    (r'/\*[\s\S]*?\*/', True),
    # String literals (rough approximation)
    (r'"[^"\\]*(?:\\.[^"\\]*)*"', True),
]


def is_forbidden_pattern(line: str) -> Tuple[bool, str]:
    """Check if a line contains a forbidden pattern.
    
    Args:
        line: Source line to check
        
    Returns:
        Tuple of (is_forbidden, description)
    """
    for pattern, desc, is_regex in FORBIDDEN_PATTERNS:
        if is_regex:
            if re.search(pattern, line):
                return True, desc
        else:
            if pattern in line:
                return True, desc
    return False, ""


def is_medium_pattern(line: str) -> Tuple[bool, str]:
    """Check if a line contains a medium-risk pattern.
    
    Args:
        line: Source line to check
        
    Returns:
        Tuple of (is_medium, description)
    """
    for pattern, desc, is_regex in MEDIUM_PATTERNS:
        if is_regex:
            if re.search(pattern, line):
                return True, desc
        else:
            if pattern in line:
                return True, desc
    return False, ""


def strip_comments(code: str) -> str:
    """Remove comments from Zig code for pattern matching.
    
    This is a rough approximation - handles // and /* */ comments.
    
    Args:
        code: Full source code
        
    Returns:
        Code with comments removed
    """
    # Remove block comments first (/* ... */)
    code = re.sub(r'/\*[\s\S]*?\*/', '', code)
    # Remove line comments
    code = re.sub(r'//.*$', '', code, flags=re.MULTILINE)
    return code


def strip_tests(code: str) -> str:
    """Remove test blocks from Zig code.
    
    Args:
        code: Full source code
        
    Returns:
        Code with test blocks removed
    """
    # Remove test blocks
    # This is approximate - handles simple test blocks
    code = re.sub(
        r'^test\s*"[^"]*"\s*\{[\s\S]*?^\}\s*$',
        '',
        code,
        flags=re.MULTILINE
    )
    return code


def is_in_test_block(code: str, line_number: int) -> bool:
    """Check if a line is inside a test block.
    
    Args:
        code: Full source code
        line_number: 1-based line number
        
    Returns:
        True if line is inside a test block
    """
    lines = code.split('\n')
    if line_number < 1 or line_number > len(lines):
        return False
    
    # Find surrounding context (look back for test start)
    in_test = False
    brace_count = 0
    
    for i in range(line_number - 1):
        line = lines[i].strip()
        if line.startswith('test '):
            in_test = True
            brace_count = 0
        if in_test:
            brace_count += line.count('{') - line.count('}')
            if brace_count <= 0 and '}' in lines[i]:
                in_test = False
    
    return in_test


def get_pattern_summary() -> List[Tuple[str, str]]:
    """Get summary of all patterns with descriptions."""
    result = []
    for pattern, desc, _ in FORBIDDEN_PATTERNS:
        result.append((pattern, f"[FAIL] {desc}"))
    for pattern, desc, _ in MEDIUM_PATTERNS:
        result.append((pattern, f"[WARN] {desc}"))
    return result
