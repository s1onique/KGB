#!/usr/bin/env python3
"""
verify_effect_boundaries.py — Tovarisch effect boundary verifier

ACT-TOVARISCH-ZIG-HULK20: Functional core / effect boundary register and gate

Verifies that PURE modules do not contain forbidden effect patterns.
"""

import sys
import os
import re
import tempfile
import shutil
from pathlib import Path
from dataclasses import dataclass
from typing import Set, List, Tuple


# =============================================================================
# Classification Table (hardcoded first version, can be extended to parse register)
# =============================================================================

# PURE modules: no effects allowed
PURE_MODULES: Set[str] = {
    "bgp/snapshot.zig",
    "bfd/snapshot.zig",
    "bgp/types.zig",
    "bgp/message.zig",
    "bgp/frame_decode.zig",
    "bgp/notification_decode.zig",
    "bgp/config_parse.zig",
    "bgp/validation.zig",
    "bgp/status.zig",
    "bgp/session_status.zig",
    "bfd/packet.zig",
    "bfd/config.zig",
    "status_query.zig",
    "status_bgp_diagnostics.zig",
    "net/linux_addr_parse.zig",
    "net/private_ip.zig",
    "net/rates.zig",
    "net/stat_formatter.zig",
    "net/ss_parser.zig",
    "net/wg_show_parser.zig",
    "net/interface_filter.zig",
    "net/network_diag_config.zig",
    "metrics_dto.zig",
    "config_parse_helpers.zig",
}

# BOUNDARY modules: effects allowed but must be documented
# These modules perform I/O but the effects are documented and bounded.
BOUNDARY_MODULES: Set[str] = {
    # Filesystem access - PURE-looking but actually touches sysfs
    "tunnel_check.zig",
    # Netlink socket operations - network I/O
    "net/linux_addr.zig",
    # Standard entry points
    "main.zig",
    "cli.zig",
    "http/routes.zig",
    "http/response.zig",
    "http/server.zig",
    "net/linux_read.zig",
    "net/safe_command.zig",
    "net/iptables.zig",
    "net/wg_status_boundary.zig",
    "net/wg_dump_collector.zig",
    "net/wg_show_collector.zig",
    "net/linux_interface_stats.zig",
    "net/linux_interfaces.zig",
    "net/linux_stats.zig",
    "net/inotify.zig",
    "status.zig",
    "status_network_diag.zig",
    "status_network_diag_tcp.zig",
    "logging.zig",
    "config.zig",
    "config_lab.zig",
    "metrics.zig",
    "metrics_state.zig",
    "build_info.zig",
}

# STATEFUL modules: own long-lived state
STATEFUL_MODULES: Set[str] = {
    "bgp/session.zig",
    "bgp/runtime.zig",
    "bgp/serve_integration.zig",
    "bgp/passive_listener.zig",
    "bgp/prefix_watch.zig",
    "bgp/prefix_file_loader.zig",
    "bfd/session.zig",
    "bfd/serve_integration.zig",
    "net/diag_event_ring.zig",
    "net/interface_sampler.zig",
    "runtime/uvb76_capture.zig",
}

# DEFERRED modules: classification unclear, report only
DEFERRED_MODULES: Set[str] = {
    "status_response.zig",
    "http/status_route_contract.zig",
}

# TEST module pattern
TEST_PATTERNS = [
    re.compile(r"_tests\.zig$"),
    re.compile(r"^test_"),
    re.compile(r"_test\.zig$"),
]


# =============================================================================
# Forbidden patterns in PURE modules
# =============================================================================

# Patterns that indicate effect usage (case-sensitive, word-boundary aware)
# These are checked in context (not inside comments)
# Note: We use more specific patterns to avoid false positives from field names
FORBIDDEN_PATTERNS = [
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


@dataclass
class Violation:
    """Represents a violation found in a module."""
    file: str
    pattern: str
    description: str
    line: int
    line_content: str


def is_test_file(path: Path) -> bool:
    """Check if a file is a test file based on naming patterns."""
    name = path.name
    for pattern in TEST_PATTERNS:
        if pattern.search(name):
            return True
    return False


def strip_comments_and_tests(content: str) -> str:
    """Remove comments and test blocks from content to avoid false positives."""
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


def check_for_violations(file_path: Path, content: str) -> List[Violation]:
    """Check a file for forbidden effect patterns."""
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
    """Check for production imports of test files."""
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


def classify_module(module_path: str, pure_set: Set[str], boundary_set: Set[str],
                   stateful_set: Set[str], deferred_set: Set[str]) -> str:
    """Classify a module based on the classification tables."""
    # Normalize path
    module_path = module_path.replace('tovarisch/src/', '')
    
    if module_path in pure_set:
        return "PURE"
    elif module_path in boundary_set:
        return "BOUNDARY"
    elif module_path in stateful_set:
        return "STATEFUL"
    elif module_path in deferred_set:
        return "DEFERRED"
    else:
        # Unknown modules are treated as DEFERRED for safety
        return "UNKNOWN"


def scan_directory(base_dir: Path, pure_set: Set[str], boundary_set: Set[str],
                   stateful_set: Set[str], deferred_set: Set[str],
                   force_modules: Set[str] = None) -> Tuple[List[Violation], List[Tuple[str, str, int]], List[Tuple[str, str]]]:
    """
    Scan directory for violations.
    
    Returns:
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
        
        classification = classify_module(module_path, pure_set, boundary_set, stateful_set, deferred_set)
        
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


def run_self_test() -> bool:
    """Run self-test to verify the verifier works correctly."""
    print("[self-test] Running effect boundary verifier self-test...")
    
    all_passed = True
    test_dir = None
    
    try:
        # Create temporary directory for test files
        test_dir = tempfile.mkdtemp(prefix="effect_boundary_test_")
        test_path = Path(test_dir)
        
        # =====================================================================
        # Test 1: Clean tree should pass
        # =====================================================================
        print("[self-test] Test 1: Clean PURE module should pass...")
        
        clean_module = test_path / "clean_module.zig"
        clean_module.write_text('''
const std = @import("std");

pub const MyEnum = enum { a, b, c };

pub fn parse(input: []const u8) MyEnum {
    if (std.mem.eql(u8, input, "a")) return .a;
    return .b;
}
''')
        
        violations, test_imports, _ = scan_directory(
            test_path, PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES
        )
        if violations or test_imports:
            print(f"[FAIL] Test 1: Clean module should not have violations")
            all_passed = False
        else:
            print("[PASS] Test 1: Clean module passed")
        
        # =====================================================================
        # Test 2: PURE module using std.fs.cwd() should fail
        # =====================================================================
        print("[self-test] Test 2: PURE module with std.fs.cwd() should fail...")
        
        cwd_module = test_path / "cwd_violation.zig"
        cwd_module.write_text('''
const std = @import("std");

pub fn readCurrentDir() !void {
    const cwd = std.fs.cwd();
    _ = cwd;
}
''')
        
        # Force this file to be treated as PURE
        violations, _, _ = scan_directory(
            test_path, PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES,
            force_modules={"cwd_violation.zig"}
        )
        
        cwd_violations = [v for v in violations if "cwd_violation.zig" in v.file and "std.fs.cwd" in v.description]
        if not cwd_violations:
            print(f"[FAIL] Test 2: std.fs.cwd() violation not detected")
            all_passed = False
        else:
            print(f"[PASS] Test 2: std.fs.cwd() violation detected at line {cwd_violations[0].line}")
        
        # =====================================================================
        # Test 3: PURE module using std.process should fail
        # =====================================================================
        print("[self-test] Test 3: PURE module with std.process should fail...")
        
        process_module = test_path / "process_violation.zig"
        process_module.write_text('''
const std = @import("std");

pub fn spawnProcess() !void {
    try std.process.spawn();
}
''')
        
        violations, _, _ = scan_directory(
            test_path, PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES,
            force_modules={"process_violation.zig"}
        )
        
        process_violations = [v for v in violations if "process_violation.zig" in v.file and "std.process" in v.description]
        if not process_violations:
            print(f"[FAIL] Test 3: std.process violation not detected")
            all_passed = False
        else:
            print(f"[PASS] Test 3: std.process violation detected at line {process_violations[0].line}")
        
        # =====================================================================
        # Test 4: BOUNDARY module using std.fs.cwd() should pass
        # =====================================================================
        print("[self-test] Test 4: BOUNDARY module with std.fs.cwd() should pass...")
        
        # First, add the test file to BOUNDARY modules
        BOUNDARY_MODULES.add("boundary_allowed.zig")
        
        boundary_module = test_path / "boundary_allowed.zig"
        boundary_module.write_text('''
const std = @import("std");

// This is a BOUNDARY module - effects are allowed
pub fn readCurrentDir() !void {
    const cwd = std.fs.cwd();
    _ = cwd;
}
''')
        
        violations, _, _ = scan_directory(
            test_path, PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES,
            force_modules=set()  # Empty - don't force to PURE
        )
        
        # Filter to only the BOUNDARY module's violations
        boundary_violations = [v for v in violations if "boundary_allowed.zig" in v.file]
        if boundary_violations:
            print(f"[FAIL] Test 4: BOUNDARY module should not have violations")
            all_passed = False
        else:
            print("[PASS] Test 4: BOUNDARY module passed")
        
        # Clean up
        BOUNDARY_MODULES.discard("boundary_allowed.zig")
        
        # =====================================================================
        # Test 5: Production import of test file should fail
        # =====================================================================
        print("[self-test] Test 5: Production import of *_tests.zig should fail...")
        
        test_file = test_path / "my_tests.zig"
        test_file.write_text('''
test "sample" {
    // This is a test file
}
''')
        
        prod_file = test_path / "producer.zig"
        prod_file.write_text('''
const std = @import("std");
const my_tests = @import("my_tests.zig");
''')
        
        violations, test_imports, _ = scan_directory(
            test_path, PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES,
            force_modules={"producer.zig"}
        )
        
        prod_imports = [(f, imp, line) for f, imp, line in test_imports if "producer.zig" in f]
        if not prod_imports:
            print(f"[FAIL] Test 5: Production import of test file not detected")
            all_passed = False
        else:
            print(f"[PASS] Test 5: Production import of test file detected at line {prod_imports[0][2]}")
        
        # =====================================================================
        # Test 6: Comment containing forbidden pattern should not trigger
        # =====================================================================
        print("[self-test] Test 6: Comments with forbidden pattern should not trigger...")
        
        comment_module = test_path / "comment_module.zig"
        comment_module.write_text('''
const std = @import("std");

// NOTE: std.fs.cwd() should not be used in pure functions.
// std.process is forbidden for PURE modules.
// @panic is not allowed on external input.
pub fn pureFunction() []const u8 {
    return "hello";
}
''')
        
        violations, _, _ = scan_directory(
            test_path, PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES,
            force_modules={"comment_module.zig"}
        )
        
        comment_violations = [v for v in violations if "comment_module.zig" in v.file]
        if comment_violations:
            print(f"[FAIL] Test 6: Comment exclusion failed - found violations")
            for v in comment_violations:
                print(f"      Line {v.line}: {v.description}")
            all_passed = False
        else:
            print("[PASS] Test 6: Comments correctly excluded")
        
    finally:
        # Cleanup
        if test_dir and os.path.exists(test_dir):
            shutil.rmtree(test_dir)
    
    return all_passed


def main():
    """Main entry point."""
    args = sys.argv[1:]
    
    if "--self-test" in args:
        success = run_self_test()
        if success:
            print("\n[self-test] All self-tests passed!")
            sys.exit(0)
        else:
            print("\n[self-test] Some self-tests FAILED!")
            sys.exit(1)
    
    # Normal scan mode
    base_dir = Path(__file__).parent.parent
    print(f"[verifier] Scanning {base_dir / 'tovarisch/src'}...")
    
    violations, test_imports, deferred = scan_directory(
        base_dir, PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES
    )
    
    # Report violations
    has_errors = False
    
    if violations:
        print("\n[ERROR] PURE module violations found:")
        for v in violations:
            print(f"  {v.file}:{v.line}: {v.description}")
            print(f"    -> {v.line_content}")
        has_errors = True
    
    if test_imports:
        print("\n[ERROR] Production modules importing test files:")
        for prod_file, test_file, line in test_imports:
            print(f"  {prod_file}:{line} imports {test_file}")
        has_errors = True
    
    if deferred:
        print("\n[INFO] DEFERRED/UNKNOWN modules (report only):")
        for module, reason in deferred:
            print(f"  {module} ({reason})")
    
    if has_errors:
        print("\n[FAIL] Effect boundary verification FAILED")
        sys.exit(1)
    else:
        print("\n[PASS] Effect boundary verification passed")
        sys.exit(0)


if __name__ == "__main__":
    main()
