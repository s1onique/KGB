"""
Pattern definitions for effect boundary verifier.

Contains forbidden effect patterns and pattern matching helpers.
"""

import re
from typing import List, Tuple


# =============================================================================
# Forbidden patterns in PURE modules
# =============================================================================

# Patterns that indicate effect usage (case-sensitive, word-boundary aware)
# These are checked in context (not inside comments)
# Note: We use more specific patterns to avoid false positives from field names
FORBIDDEN_PATTERNS: List[Tuple[str, str]] = [
    # POSIX calls - very specific
    (r'\bstd\.c\.', "std.c namespace"),
    
    # Process execution - very specific
    (r'\bstd\.process\.', "std.process namespace"),
    
    # File system - cwd and open (specific namespace/function calls)
    (r'\bstd\.fs\.cwd\(\)', "std.fs.cwd()"),
    (r'\bstd\.fs\.open\b', "std.fs.open"),
    
    # File operations - specific function names
    (r'\bopenFile\b', "openFile"),
    (r'\bopenForRead\b', "openForRead"),
    (r'\bcreateFile\b', "createFile"),
    (r'\bdeleteFile\b', "deleteFile"),
    (r'\brenameFile\b', "renameFile"),
    
    # Network - specific function calls (not field names)
    (r'\bstd\.net\.', "std.net namespace"),
    (r'\bsocket\(', "socket("),
    (r'\bconnect\(', "connect("),
    (r'\blisten\(', "listen("),
    (r'\baccept\(', "accept("),
    (r'\bbind\(', "bind("),
    
    # Time operations - specific namespace/function calls
    (r'\bstd\.time\.', "std.time namespace"),
    (r'\bnanoTimestamp\(\)', "nanoTimestamp()"),
    (r'\bmilliTimestamp\(\)', "milliTimestamp()"),
    
    # Random - specific namespace
    (r'\bstd\.crypto\.random\b', "std.crypto.random"),
    
    # Global allocators - specific
    (r'\bstd\.heap\.page_allocator\b', "std.heap.page_allocator"),
    (r'\bstd\.heap\.c_allocator\b', "std.heap.c_allocator"),
    
    # Panic on external input - specific builtin
    (r'\@panic\(', "@panic("),
]


def strip_comments_and_tests(content: str) -> str:
    """
    Remove comments and test blocks from content to avoid false positives.
    
    Args:
        content: Source code content
        
    Returns:
        Content with comments and test blocks removed
    """
    lines = content.split('\n')
    result_lines = []
    
    in_test_block = False
    brace_depth = 0
    
    for line in lines:
        stripped = line.strip()
        
        # Track test block state
        if not in_test_block and (stripped.startswith('test "') or stripped.startswith("test '")):
            in_test_block = True
            brace_depth = 0
        
        # Skip content inside test blocks
        if in_test_block:
            # Count braces to know when test block ends
            for ch in stripped:
                if ch == '{':
                    brace_depth += 1
                elif ch == '}':
                    brace_depth -= 1
                    if brace_depth == 0:
                        in_test_block = False
                        break
            # Also check if test ends with single }
            if in_test_block and (stripped == '}' or (stripped.endswith('}') and not '{' in stripped and brace_depth == 0)):
                in_test_block = False
            continue
        
        # Remove doc comments first (///)
        if stripped.startswith('///'):
            result_lines.append('')
            continue
        
        # Remove line comments (//) - but not if inside string literals
        # Simple heuristic: find first // that is not inside quoted strings
        if '//' in line:
            # Count quotes to determine if we're inside a string
            # This is a simplified heuristic - handles common cases
            in_string = False
            escaped = False
            comment_start = -1
            for i, char in enumerate(line):
                if escaped:
                    escaped = False
                    continue
                if char == '\\':
                    escaped = True
                    continue
                if char == '"':
                    in_string = not in_string
                elif char == '/' and not in_string and i + 1 < len(line):
                    if line[i+1] == '/':
                        comment_start = i
                        break
            if comment_start >= 0:
                line = line[:comment_start]
        result_lines.append(line)
    
    return '\n'.join(result_lines)
