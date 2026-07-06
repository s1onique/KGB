# scanner_second_ring_tests.py — HULK23 second-ring parser module tests
"""Second-ring parser module scanner tests for total parser verification.

These tests prove that the second-ring parser modules (bfd/packet.zig,
net/ss_parser.zig, net/wg_show_parser.zig) are correctly classified and
that accepted patterns work as expected.
"""

import os
import tempfile
from typing import List, Tuple

from .scanner import scan_file, is_accepted_pattern


def run_second_ring_tests(
    src_root: str,
    passed: int,
    failed: int,
    errors: List[str],
    verbose: bool = False,
) -> Tuple[int, int, List[str]]:
    """Run second-ring parser module tests.
    
    Tests 6-8: Second-ring parser module classification and accepted patterns
    
    Args:
        src_root: Source root directory for test files
        passed: Current passed count
        failed: Current failed count
        errors: Current error messages
        verbose: Print verbose output
        
    Returns:
        Tuple of (passed, failed, errors)
    """
    from .classifications import Classification
    
    # Test 6: bfd/packet.zig is discovered and classified as TOTAL
    if verbose:
        print("\nScanner test 6: bfd/packet.zig classification")
    
    bfd_dir = os.path.join(src_root, "bfd")
    os.makedirs(bfd_dir)
    
    bfd_packet_path = os.path.join(bfd_dir, "packet.zig")
    # Use all 4 BFD states (RFC 5880) with proper bit-masked pattern
    bfd_packet_code = '''
const std = @import("std");

pub const State = enum(u2) {
    admin_down = 0,
    down = 1,
    init = 2,
    up = 3,
};

pub const Diagnostic = enum(u5) {
    no_diagnostic = 0,
    control_detection_time_expired = 1,
};

pub fn decode(buf: []const u8) !State {
    if (buf.len < 2) return error.InvalidPacket;
    // Bit-masked @truncate: RFC 5880 guarantees 2-bit state
    const diag_val = @as(u5, @truncate(buf[0]));
    const diag: Diagnostic = @enumFromInt(diag_val);
    const state_val = (buf[1] >> 6) & 0x03;
    const state: State = @enumFromInt(state_val);
    _ = diag;
    return state;
}
'''
    with open(bfd_packet_path, 'w') as f:
        f.write(bfd_packet_code)
    
    result = scan_file(bfd_packet_path, src_root)
    
    if result.module == "bfd/packet.zig":
        passed += 1
        if verbose:
            print("  PASS: module extracted as 'bfd/packet.zig'")
    else:
        failed += 1
        msg = f"Expected module 'bfd/packet.zig', got '{result.module}'"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    if result.classification == Classification.TOTAL:
        passed += 1
        if verbose:
            print("  PASS: classification is TOTAL")
    else:
        failed += 1
        msg = f"Expected TOTAL, got {result.classification}"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    # @enumFromInt(diag_val) and @enumFromInt(state_val) in bfd/packet.zig should be WARN (accepted pattern)
    if result.has_warnings and not result.has_failures:
        passed += 1
        if verbose:
            print("  PASS: @enumFromInt(diag_val/state_val) in bfd/packet.zig produces WARN, not FAIL")
    else:
        failed += 1
        msg = "@enumFromInt(diag_val/state_val) in bfd/packet.zig should be accepted (WARN only)"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    # Test 7: net/ss_parser.zig is discovered and classified as TOTAL
    if verbose:
        print("\nScanner test 7: net/ss_parser.zig classification")
    
    net_dir = os.path.dirname(bfd_packet_path).replace("bfd", "net")
    if not os.path.exists(net_dir):
        os.makedirs(net_dir)
    
    ss_parser_path = os.path.join(net_dir, "ss_parser.zig")
    ss_parser_code = '''
pub fn parseLine(line: []const u8) ?u32 {
    if (line.len == 0) return null;
    if (line.len > 0) |len| {
        return len.?;
    }
    return null;
}
'''
    with open(ss_parser_path, 'w') as f:
        f.write(ss_parser_code)
    
    result = scan_file(ss_parser_path, src_root)
    
    if result.module == "net/ss_parser.zig":
        passed += 1
        if verbose:
            print("  PASS: module extracted as 'net/ss_parser.zig'")
    else:
        failed += 1
        msg = f"Expected module 'net/ss_parser.zig', got '{result.module}'"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    if result.classification == Classification.TOTAL:
        passed += 1
        if verbose:
            print("  PASS: classification is TOTAL")
    else:
        failed += 1
        msg = f"Expected TOTAL, got {result.classification}"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    # .? in net/ss_parser.zig should be WARN (accepted pattern)
    if result.has_warnings and not result.has_failures:
        passed += 1
        if verbose:
            print("  PASS: .? in net/ss_parser.zig produces WARN, not FAIL")
    else:
        failed += 1
        msg = ".? in net/ss_parser.zig should be accepted (WARN only)"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    # Test 8: @panic in newly registered TOTAL module still fails
    if verbose:
        print("\nScanner test 8: @panic in TOTAL module fails")
    
    bgp_dir = os.path.join(src_root, "bgp")
    os.makedirs(bgp_dir, exist_ok=True)
    
    bgp_config_path = os.path.join(bgp_dir, "config_parse.zig")
    bgp_config_code = '''
pub fn parseConfig(input: []const u8) !void {
    if (input.len == 0) @panic("empty");
}
'''
    with open(bgp_config_path, 'w') as f:
        f.write(bgp_config_code)
    
    result = scan_file(bgp_config_path, src_root)
    
    if result.has_failures:
        passed += 1
        if verbose:
            print("  PASS: @panic in bgp/config_parse.zig produces FAIL")
    else:
        failed += 1
        msg = "@panic in bgp/config_parse.zig should produce FAIL"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    return passed, failed, errors
