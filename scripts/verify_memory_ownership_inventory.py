#!/usr/bin/env python3
# verify_memory_ownership_inventory.py — Verify memory ownership inventory CSV
#
# ACT-HULK29R-ZIG016-MEMOWN04-MEMORY-OWNERSHIP-INVENTORY
#
# This verifier validates the memory ownership inventory CSV for:
# 1. Schema and format correctness
# 2. Stable ID format (MEMOWN-\d{4})
# 3. Path existence
# 4. Enum values (kind, allocator_boundary, request_path, verified)
# 5. Source-backed ownership evidence:
#    - owned_type rows have pub fn deinit nearby
#    - producer rows have errdefer evidence
#    - consumer rows have .deinit or defer evidence
#    - test rows with cleanup=std.testing.allocator have std.testing.allocator in body
#    - request_path=yes rows have verified=yes
#
# Exit codes:
#   0 — all checks pass
#   1 — verification failed
#   2 — internal error

import csv
import re
import sys
from pathlib import Path
from typing import Dict, List, Optional, Tuple

# Repository root
REPO_ROOT = Path(__file__).parent.parent

# Required header fields
REQUIRED_HEADER = [
    "id", "path", "language", "symbol", "kind", "allocator_boundary",
    "owned_type", "owner", "cleanup", "coverage", "request_path", "verified", "notes"
]

# Valid enum values
VALID_KINDS = {"producer", "owned_type", "consumer", "test", "verifier"}
VALID_ALLOCATOR_BOUNDARIES = {"allocates", "returns_owned", "consumes_owned", "deinit", "verifies", "none"}
VALID_YES_NO = {"yes", "no"}


def _relative_path(path: Path) -> str:
    """Return repo-relative path for cleaner diagnostics."""
    try:
        return str(path.relative_to(REPO_ROOT))
    except ValueError:
        return str(path)


def find_symbol(content: str, symbol: str) -> bool:
    """Check if a symbol exists in the source content."""
    # Handle test names - look for test "symbol" pattern
    if re.search(r'test\s+"' + re.escape(symbol) + r'"', content):
        return True
    
    # Handle struct method names (e.g., "NetworkDiag.deinit")
    if '.' in symbol:
        parts = symbol.split('.')
        struct_name = parts[0]
        method_name = parts[1]
        
        # First find the struct definition
        struct_pattern = r'(const\s+' + re.escape(struct_name) + r'\s*=\s*struct\s*\{|struct\s+' + re.escape(struct_name) + r'\s*\{)'
        struct_match = re.search(struct_pattern, content)
        if struct_match:
            # Extract struct body (from opening brace to closing brace)
            start_pos = struct_match.end() - 1  # Position of opening brace
            brace_count = 1
            pos = start_pos + 1
            while pos < len(content) and brace_count > 0:
                if content[pos] == '{':
                    brace_count += 1
                elif content[pos] == '}':
                    brace_count -= 1
                pos += 1
            struct_body = content[start_pos:pos]
            # Look for method in struct body
            if re.search(r'(pub\s+)?fn\s+' + re.escape(method_name) + r'\s*\(', struct_body):
                return True
        
        # Also try standalone function patterns
        patterns = [
            r'fn\s+' + re.escape(symbol) + r'\s*\(',
            r'pub\s+fn\s+' + re.escape(parts[0]) + r'\s*\.\s*' + re.escape(parts[1]) + r'\s*\(',
            r'fn\s+' + re.escape(parts[0]) + r'\s*\.\s*' + re.escape(parts[1]) + r'\s*\(',
        ]
    else:
        patterns = []
    
    # Look for function/type/const declarations
    patterns.extend([
        r'pub\s+const\s+' + re.escape(symbol) + r'\s*=',
        r'const\s+' + re.escape(symbol) + r'\s*=',
        r'pub\s+fn\s+' + re.escape(symbol) + r'\s*\(',
        r'fn\s+' + re.escape(symbol) + r'\s*\(',
        r'pub\s+struct\s+' + re.escape(symbol) + r'\s*\{',
        r'struct\s+' + re.escape(symbol) + r'\s*\{',
    ])
    
    for pattern in patterns:
        if re.search(pattern, content):
            return True
    return False


def extract_nearby_window(content: str, symbol: str, window_lines: int = 200) -> str:
    """Extract a window of lines around the symbol definition."""
    lines = content.split('\n')
    
    # Find the line with the symbol - must match declaration pattern
    escaped = re.escape(symbol)
    for i, line in enumerate(lines):
        # Check if this line matches a function/type declaration for this symbol
        is_fn_match = re.search(r'\bpub\s+fn\s+' + escaped + r'\b', line)
        is_plain_fn_match = re.search(r'\bfn\s+' + escaped + r'\b', line)
        is_struct_match = re.search(r'\bstruct\s+' + escaped + r'\b', line)
        is_const_match = re.search(r'\bconst\s+' + escaped + r'\s*=', line)
        
        if is_fn_match or is_plain_fn_match or is_struct_match or is_const_match:
            start = max(0, i)
            end = min(len(lines), i + window_lines)
            return '\n'.join(lines[start:end])
    
    # Try broader search for test names
    for i, line in enumerate(lines):
        if f'test "{symbol}' in line or f'test "{symbol}"' in line:
            start = max(0, i)
            end = min(len(lines), i + window_lines)
            return '\n'.join(lines[start:end])
    
    return ""


def find_zig_deinit(content: str, owned_type: str) -> bool:
    """Check if an owned type has a deinit method nearby."""
    # Look for pub fn deinit or fn deinit within the struct definition
    # Handle both "struct Symbol {" and "pub const Symbol = struct {"
    
    # First try "const Symbol = struct {" pattern
    struct_pattern = r'const\s+' + re.escape(owned_type) + r'\s*=\s*struct\s*\{'
    match = re.search(struct_pattern, content)
    
    if not match:
        # Try "struct Symbol {" pattern
        struct_pattern = r'struct\s+' + re.escape(owned_type) + r'\s*\{'
        match = re.search(struct_pattern, content)
    
    if not match:
        return False
    
    # Find the closing brace (simple brace counting)
    start_pos = match.end()
    brace_count = 1
    pos = start_pos
    
    while pos < len(content) and brace_count > 0:
        if content[pos] == '{':
            brace_count += 1
        elif content[pos] == '}':
            brace_count -= 1
        pos += 1
    
    struct_body = content[start_pos:pos-1]
    
    # Look for deinit function (pub fn deinit or fn deinit)
    return bool(re.search(r'(pub\s+)?fn\s+deinit\s*\(', struct_body))


def find_errdefer(content: str, symbol: str, window_lines: int = 200) -> bool:
    """Check if a producer function has errdefer."""
    window = extract_nearby_window(content, symbol, window_lines)
    # Look for errdefer statements at the start of a line (after optional whitespace)
    # This avoids matching comments like "// Missing errdefer"
    return re.search(r'^\s*errdefer\s', window, re.MULTILINE) is not None


def find_deinit_or_defer(content: str, symbol: str, window_lines: int = 200) -> bool:
    """Check if a consumer function uses deinit, defer, or allocator.free for cleanup."""
    # Handle struct method names (e.g., "NetworkDiag.deinit")
    if '.' in symbol:
        parts = symbol.split('.')
        struct_name = parts[0]
        method_name = parts[1]
        
        # Find the struct definition
        struct_pattern = r'(const\s+' + re.escape(struct_name) + r'\s*=\s*struct\s*\{|struct\s+' + re.escape(struct_name) + r'\s*\{)'
        struct_match = re.search(struct_pattern, content)
        if struct_match:
            # Extract struct body
            start_pos = struct_match.end() - 1
            brace_count = 1
            pos = start_pos + 1
            while pos < len(content) and brace_count > 0:
                if content[pos] == '{':
                    brace_count += 1
                elif content[pos] == '}':
                    brace_count -= 1
                pos += 1
            struct_body = content[start_pos:pos]
            
            # Look for method in struct body
            method_pattern = r'(pub\s+)?fn\s+' + re.escape(method_name) + r'\s*\([^)]*\)\s*[^;]*\{'
            method_match = re.search(method_pattern, struct_body)
            if method_match:
                # Extract method body
                method_start = method_match.end() - 1  # position of opening brace
                brace_count = 1
                pos = method_start + 1
                while pos < len(struct_body) and brace_count > 0:
                    if struct_body[pos] == '{':
                        brace_count += 1
                    elif struct_body[pos] == '}':
                        brace_count -= 1
                    pos += 1
                method_body = struct_body[method_start:pos]
                
                # Look for cleanup patterns in method body
                has_allocator_free = re.search(r'allocator\.free\s*\(', method_body) is not None
                return has_allocator_free
    
    # Default: use window extraction for regular functions
    window = extract_nearby_window(content, symbol, window_lines)
    # Look for .deinit( calls or defer statements at start of line
    has_deinit = re.search(r'\.deinit\s*\(', window) is not None
    has_defer = re.search(r'^\s*defer\s', window, re.MULTILINE) is not None
    # Also accept allocator.free for raw slice cleanup
    has_allocator_free = re.search(r'allocator\.free\s*\(', window) is not None
    return has_deinit or has_defer or has_allocator_free


def find_test_body(content: str, test_name: str) -> Optional[str]:
    """Extract the body of a specific test."""
    # Look for test "test_name" {
    pattern = r'test\s+"' + re.escape(test_name) + r'"\s*\{'
    match = re.search(pattern, content)
    if not match:
        return None
    
    # Find the closing brace
    start_pos = match.end()
    brace_count = 1
    pos = start_pos
    
    while pos < len(content) and brace_count > 0:
        if content[pos] == '{':
            brace_count += 1
        elif content[pos] == '}':
            brace_count -= 1
        pos += 1
    
    return content[start_pos:pos-1]


def has_testing_allocator(content: str, test_name: str) -> bool:
    """Check if a test body uses std.testing.allocator."""
    test_body = find_test_body(content, test_name)
    if test_body is None:
        return False
    # Check each line - skip comment lines
    for line in test_body.split('\n'):
        stripped = line.strip()
        if stripped.startswith('//'):
            continue  # Skip comment lines
        if 'std.testing.allocator' in line:
            return True
    return False


def check_csv_schema(csv_path: Path) -> List[str]:
    """Validate CSV schema and basic format."""
    errors = []
    
    if not csv_path.exists():
        return [f"{_relative_path(csv_path)}: file not found"]
    
    try:
        with open(csv_path, 'r') as f:
            reader = csv.DictReader(f)
            rows = list(reader)
    except Exception as e:
        return [f"{_relative_path(csv_path)}: failed to read CSV: {e}"]
    
    if not rows:
        return [f"{_relative_path(csv_path)}: CSV is empty or has no data rows"]
    
    # Check header
    if list(rows[0].keys()) != REQUIRED_HEADER:
        errors.append(
            f"{_relative_path(csv_path)}: header mismatch\n"
            f"  expected: {REQUIRED_HEADER}\n"
            f"  actual: {list(rows[0].keys())}"
        )
        return errors  # Can't proceed with row validation if header is wrong
    
    # Check for duplicate IDs
    ids = [row['id'] for row in rows]
    seen = set()
    for id_val in ids:
        if id_val in seen:
            errors.append(f"{_relative_path(csv_path)}:{id_val}: duplicate ID")
        seen.add(id_val)
    
    # Validate each row
    for row in rows:
        id_val = row['id']
        row_prefix = f"{_relative_path(csv_path)}:{id_val}"
        
        # Check ID format
        if not re.match(r'^MEMOWN-\d{4}$', id_val):
            errors.append(f"{row_prefix}: malformed ID (expected MEMOWN-XXXX)")
        
        # Check path exists
        path = row['path']
        if path and not (REPO_ROOT / path).exists():
            errors.append(f"{row_prefix}: path does not exist: {path}")
        
        # Check required fields are non-empty
        if not row['language']:
            errors.append(f"{row_prefix}: language is empty")
        if not row['symbol']:
            errors.append(f"{row_prefix}: symbol is empty")
        
        # Check enum values
        kind = row['kind']
        if kind and kind not in VALID_KINDS:
            errors.append(
                f"{row_prefix}: invalid kind '{kind}'\n"
                f"  expected one of: {VALID_KINDS}"
            )
        
        allocator_boundary = row['allocator_boundary']
        if allocator_boundary and allocator_boundary not in VALID_ALLOCATOR_BOUNDARIES:
            errors.append(
                f"{row_prefix}: invalid allocator_boundary '{allocator_boundary}'\n"
                f"  expected one of: {VALID_ALLOCATOR_BOUNDARIES}"
            )
        
        request_path = row['request_path']
        if request_path and request_path not in VALID_YES_NO:
            errors.append(
                f"{row_prefix}: invalid request_path '{request_path}'\n"
                f"  expected: yes or no"
            )
        
        verified = row['verified']
        if verified and verified not in VALID_YES_NO:
            errors.append(
                f"{row_prefix}: invalid verified '{verified}'\n"
                f"  expected: yes or no"
            )
        
        # Check request_path=yes implies verified=yes
        if request_path == 'yes' and verified != 'yes':
            errors.append(
                f"{row_prefix}: request_path=yes but verified={verified}\n"
                f"  request_path=yes rows must have verified=yes"
            )
    
    return errors


def check_source_backed_ownership(csv_path: Path) -> List[str]:
    """Validate source-backed ownership evidence."""
    errors = []
    
    if not csv_path.exists():
        return []  # Already reported in schema check
    
    try:
        with open(csv_path, 'r') as f:
            reader = csv.DictReader(f)
            rows = list(reader)
    except Exception:
        return []  # Already reported in schema check
    
    # Skip if header is wrong (rows won't have expected keys)
    if rows and 'path' not in rows[0]:
        return []
    
    # Group rows by path for efficient file reading
    path_rows: Dict[str, List[Dict]] = {}
    for row in rows:
        path = row['path']
        if path not in path_rows:
            path_rows[path] = []
        path_rows[path].append(row)
    
    # Check each file
    for path, file_rows in path_rows.items():
        full_path = REPO_ROOT / path
        if not full_path.exists():
            continue  # Already reported in schema check
        
        try:
            content = full_path.read_text()
        except Exception:
            continue
        
        for row in file_rows:
            id_val = row['id']
            row_prefix = f"{_relative_path(csv_path)}:{id_val}"
            symbol = row['symbol']
            kind = row['kind']
            allocator_boundary = row['allocator_boundary']
            owned_type = row['owned_type']
            cleanup = row['cleanup']
            request_path = row['request_path']
            verified = row['verified']
            notes = row.get('notes', '')
            coverage = row.get('coverage', '')
            
            # owned_type rows: check deinit exists
            if kind == 'owned_type' and owned_type != 'n/a':
                if cleanup == 'deinit':
                    if not find_symbol(content, owned_type):
                        errors.append(
                            f"{row_prefix}: owned_type row `{symbol}` not found in {path}"
                        )
                    elif not find_zig_deinit(content, owned_type):
                        errors.append(
                            f"{row_prefix}: owned_type row `{symbol}` with cleanup=deinit "
                            f"lacks `fn deinit` in {path}"
                        )
            
            # producer rows: check errdefer exists
            if kind == 'producer' and allocator_boundary == 'returns_owned':
                if not find_symbol(content, symbol):
                    errors.append(
                        f"{row_prefix}: producer row `{symbol}` not found in {path}"
                    )
                elif owned_type != 'n/a' and not find_errdefer(content, symbol):
                    errors.append(
                        f"{row_prefix}: producer row `{symbol}` with allocator_boundary=returns_owned "
                        f"lacks errdefer in {path}"
                    )
            
            # consumer rows: check deinit/defer/allocator.free exists
            if kind == 'consumer' and allocator_boundary == 'consumes_owned':
                if not find_symbol(content, symbol):
                    errors.append(
                        f"{row_prefix}: consumer row `{symbol}` not found in {path}"
                    )
                elif not find_deinit_or_defer(content, symbol):
                    errors.append(
                        f"{row_prefix}: consumer row `{symbol}` lacks `.deinit(`, `defer`, or `allocator.free` near symbol "
                        f"in {path}"
                    )
            
            # test rows: check std.testing.allocator if cleanup requires it
            if kind == 'test' and cleanup == 'std.testing.allocator':
                if not find_symbol(content, symbol):
                    errors.append(
                        f"{row_prefix}: test row `{symbol}` not found in {path}"
                    )
                elif not has_testing_allocator(content, symbol):
                    errors.append(
                        f"{row_prefix}: test row `{symbol}` with cleanup=std.testing.allocator "
                        f"lacks std.testing.allocator in test body of {path}"
                    )
            
            # Allocation-free request_path rows must have explanatory notes (MEMOWN06)
            if request_path == 'yes' and allocator_boundary == 'none':
                # Accept if symbol exists OR notes contain one of the review phrases
                symbol_exists = find_symbol(content, symbol)
                notes_lower = notes.lower()
                has_review_note = any(phrase in notes_lower for phrase in [
                    'inventory reviewed',
                    'allocation-free',
                    'value-only',
                    'no owned',
                    'no request-scoped allocation',
                ])
                if not symbol_exists and not has_review_note:
                    errors.append(
                        f"{row_prefix}: request_path=yes with allocator_boundary=none requires "
                        f"symbol existence or explanatory notes (e.g., 'Inventory reviewed') in {path}"
                    )
            
            # verified=yes rows with non-n/a coverage must have coverage reference found
            if verified == 'yes' and coverage and coverage != 'n/a':
                # Check if coverage string exists in source file, Zig tests, or Python tests
                coverage_found = False
                
                # Check in the same source file
                if coverage in content:
                    coverage_found = True
                else:
                    # Check in Zig test files under tovarisch/src
                    for test_pattern in ['*_tests.zig', 'test_*.zig', '*_test.zig']:
                        for test_file in (REPO_ROOT / 'tovarisch/src').rglob(test_pattern):
                            try:
                                if coverage in test_file.read_text():
                                    coverage_found = True
                                    break
                            except Exception:
                                pass
                        if coverage_found:
                            break
                    
                    # Check in Python test files under tests/
                    if not coverage_found:
                        for test_file in (REPO_ROOT / 'tests').rglob('*.py'):
                            try:
                                if coverage in test_file.read_text():
                                    coverage_found = True
                                    break
                            except Exception:
                                pass
                
                if not coverage_found:
                    errors.append(
                        f"{row_prefix}: coverage '{coverage}' not found in {path}, "
                        f"tovarisch/src/*_tests.zig, or tests/*.py"
                    )
    
    return errors


def main() -> int:
    """Run all inventory verifications."""
    csv_path = REPO_ROOT / "docs/tooling/memory-ownership-inventory.csv"
    
    all_errors = []
    
    # Schema and consistency checks
    schema_errors = check_csv_schema(csv_path)
    all_errors.extend(schema_errors)
    
    # Source-backed ownership checks
    source_errors = check_source_backed_ownership(csv_path)
    all_errors.extend(source_errors)
    
    # Handle --self-test flag
    if "--self-test" in sys.argv:
        _run_self_test(all_errors)
        return 0
    
    # Output results
    if all_errors:
        print("MEMORY OWNERSHIP INVENTORY VERIFIER: FAIL")
        for error in sorted(all_errors):
            print(error)
        return 1
    
    print("MEMORY OWNERSHIP INVENTORY VERIFIER: PASS")
    return 0


def _run_self_test(initial_errors: List[str]) -> None:
    """Run self-test verification with temporary files."""
    import tempfile
    import os
    
    print("Running self-test...")
    
    test_cases = [
        # Case 1: Valid minimal inventory - should pass
        {
            "name": "valid minimal inventory should pass",
            "csv": """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,OwnedWgCommandResult,owned_type,consumes_owned,OwnedWgCommandResult,self,deinit,test_deinit,yes,yes,Test owned type
MEMOWN-0002,test.zig,zig,runWgShowDump,producer,returns_owned,OwnedWgCommandResult,caller,errdefer,test_deinit,yes,yes,Test producer
""",
            "zig_content": """pub const OwnedWgCommandResult = struct {
    pub fn deinit(self: *OwnedWgCommandResult, allocator: std.mem.Allocator) void {
        allocator.free(self.stdout_storage);
    }
};

fn runWgShowDump(allocator: std.mem.Allocator) !OwnedWgCommandResult {
    var buf = try allocator.alloc(u8, 1024);
    errdefer allocator.free(buf);
    return OwnedWgCommandResult{...};
}
""",
            "should_pass": True,
        },
        # Case 2: Missing file - should fail
        {
            "name": "missing file should fail",
            "csv": None,
            "should_pass": False,
            "error_contains": "file not found",
        },
        # Case 3: Wrong header - should fail
        {
            "name": "wrong header should fail",
            "csv": """id,name,description
MEMOWN-0001,test,Test entry
""",
            "should_pass": False,
            "error_contains": "header mismatch",
        },
        # Case 4: Duplicate IDs - should fail
        {
            "name": "duplicate IDs should fail",
            "csv": """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,Symbol,producer,allocates,n/a,n/a,none,n/a,no,yes,Test
MEMOWN-0001,test.zig,zig,Symbol2,producer,allocates,n/a,n/a,none,n/a,no,yes,Test
""",
            "should_pass": False,
            "error_contains": "duplicate ID",
        },
        # Case 5: Malformed ID - should fail
        {
            "name": "malformed ID should fail",
            "csv": """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-1,test.zig,zig,Symbol,producer,allocates,n/a,n/a,none,n/a,no,yes,Test
""",
            "should_pass": False,
            "error_contains": "malformed ID",
        },
        # Case 6: Nonexistent path - should fail
        {
            "name": "nonexistent path should fail",
            "csv": """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,nonexistent.zig,zig,Symbol,producer,allocates,n/a,n/a,none,n/a,no,yes,Test
""",
            "should_pass": False,
            "error_contains": "path does not exist",
        },
        # Case 7: Invalid kind - should fail
        {
            "name": "invalid kind should fail",
            "csv": """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,Symbol,invalid_kind,allocates,n/a,n/a,none,n/a,no,yes,Test
""",
            "should_pass": False,
            "error_contains": "invalid kind",
        },
        # Case 8: Invalid allocator_boundary - should fail
        {
            "name": "invalid allocator_boundary should fail",
            "csv": """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,Symbol,producer,invalid_boundary,n/a,n/a,none,n/a,no,yes,Test
""",
            "should_pass": False,
            "error_contains": "invalid allocator_boundary",
        },
        # Case 9: owned_type without deinit - should fail
        {
            "name": "owned_type without deinit should fail",
            "csv": """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,OwnedType,owned_type,consumes_owned,OwnedType,self,deinit,test,no,yes,Test
""",
            "zig_content": """pub const OwnedType = struct {
    // Missing deinit method
};
""",
            "should_pass": False,
            "error_contains": "lacks `fn deinit`",
        },
        # Case 10: producer without errdefer - should fail
        {
            "name": "producer without errdefer should fail",
            "csv": """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,createOwned,producer,returns_owned,OwnedType,caller,errdefer,test,yes,yes,Test
""",
            "zig_content": """const OwnedType = struct {};

fn createOwned(allocator: std.mem.Allocator) !OwnedType {
    var buf = try allocator.alloc(u8, 1024);
    // Missing errdefer
    return OwnedType{...};
}
""",
            "should_pass": False,
            "error_contains": "lacks errdefer",
        },
        # Case 11: consumer without deinit/defer - should fail
        {
            "name": "consumer without deinit/defer should fail",
            "csv": """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,consumeOwned,consumer,consumes_owned,OwnedType,self,defer,test,yes,yes,Test
""",
            "zig_content": """const OwnedType = struct {};

fn consumeOwned(allocator: std.mem.Allocator) !void {
    var result = try createOwned(allocator);
    // Missing defer or deinit
    _ = result;
}
""",
            "should_pass": False,
            "error_contains": "lacks `.deinit(` or `defer`",
        },
        # Case 12: test without std.testing.allocator - should fail
        {
            "name": "test without std.testing.allocator should fail",
            "csv": """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,memory leak test,test,verifies,OwnedType,test,std.testing.allocator,test,no,yes,Test
""",
            "zig_content": """const OwnedType = struct {};

test "memory leak test" {
    // Missing std.testing.allocator
    try std.testing.expect(true);
}
""",
            "should_pass": False,
            "error_contains": "lacks std.testing.allocator",
        },
        # Case 13: request_path=yes with verified=no - should fail
        {
            "name": "request_path=yes with verified=no should fail",
            "csv": """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,Symbol,producer,allocates,n/a,n/a,none,n/a,yes,no,Test
""",
            "should_pass": False,
            "error_contains": "request_path=yes but verified",
        },
        # Case 14: repo-relative diagnostics - should show relative path
        {
            "name": "diagnostics should be repo-relative",
            "csv": """id,path,language,symbol,kind,allocator_boundary,owned_type,owner,cleanup,coverage,request_path,verified,notes
MEMOWN-0001,test.zig,zig,Symbol,producer,invalid_boundary,n/a,n/a,none,n/a,no,yes,Test
""",
            "should_pass": False,
            "error_contains": "invalid allocator_boundary",
        },
    ]
    
    passed = 0
    failed = 0
    
    for tc in test_cases:
        if tc.get("csv") is None:
            # Test for missing file - create a temp dir without the CSV
            with tempfile.TemporaryDirectory() as tmpdir:
                csv_file = Path(tmpdir) / "docs/tooling/memory-ownership-inventory.csv"
                csv_file.parent.mkdir(parents=True, exist_ok=True)
                # Don't create the file
                
                import importlib
                import verify_memory_ownership_inventory
                importlib.reload(verify_memory_ownership_inventory)
                
                # Monkey-patch REPO_ROOT
                old_root = verify_memory_ownership_inventory.REPO_ROOT
                verify_memory_ownership_inventory.REPO_ROOT = Path(tmpdir)
                
                errors = verify_memory_ownership_inventory.check_csv_schema(csv_file)
                
                verify_memory_ownership_inventory.REPO_ROOT = old_root
                
                has_error = len(errors) > 0
                should_fail = tc["should_pass"] == False
                
                if has_error == should_fail:
                    print(f"  PASS: {tc['name']}")
                    passed += 1
                else:
                    print(f"  FAIL: {tc['name']}")
                    print(f"    expected: {'fail' if should_fail else 'pass'}")
                    print(f"    actual: {'fail' if has_error else 'pass'}")
                    if errors:
                        print(f"    errors: {errors}")
                    failed += 1
            continue
        
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            
            # Write CSV
            csv_file = tmpdir / "docs/tooling/memory-ownership-inventory.csv"
            csv_file.parent.mkdir(parents=True, exist_ok=True)
            csv_file.write_text(tc["csv"])
            
            # Write Zig file if specified
            if "zig_content" in tc:
                zig_file = tmpdir / "test.zig"
                zig_file.write_text(tc["zig_content"])
            
            # Run verification
            import importlib
            import verify_memory_ownership_inventory
            importlib.reload(verify_memory_ownership_inventory)
            
            # Monkey-patch REPO_ROOT
            old_root = verify_memory_ownership_inventory.REPO_ROOT
            verify_memory_ownership_inventory.REPO_ROOT = tmpdir
            
            schema_errors = verify_memory_ownership_inventory.check_csv_schema(csv_file)
            source_errors = verify_memory_ownership_inventory.check_source_backed_ownership(csv_file)
            all_errors = schema_errors + source_errors
            
            verify_memory_ownership_inventory.REPO_ROOT = old_root
            
            has_error = len(all_errors) > 0
            should_fail = tc["should_pass"] == False
            
            error_contains = tc.get("error_contains", "")
            
            if has_error == should_fail:
                if error_contains and has_error:
                    if not any(error_contains in e for e in all_errors):
                        print(f"  FAIL: {tc['name']}")
                        print(f"    expected error containing: {error_contains}")
                        print(f"    actual errors: {all_errors}")
                        failed += 1
                        continue
                print(f"  PASS: {tc['name']}")
                passed += 1
            else:
                print(f"  FAIL: {tc['name']}")
                print(f"    expected: {'fail' if should_fail else 'pass'}")
                print(f"    actual: {'fail' if has_error else 'pass'}")
                if all_errors:
                    print(f"    errors: {all_errors}")
                failed += 1
    
    print(f"\nSelf-test results: {passed} passed, {failed} failed")
    
    if failed > 0:
        print("SELF-TEST: FAIL")
        sys.exit(1)
    else:
        print("SELF-TEST: PASS")


if __name__ == "__main__":
    sys.exit(main())
