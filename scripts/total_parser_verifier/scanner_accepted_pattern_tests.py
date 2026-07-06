# scanner_accepted_pattern_tests.py — Accepted pattern negative tests
"""Negative tests for accepted patterns in total parser verification.

These tests prove that accepted patterns are:
1. Accepted when they match the exact variable names
2. NOT accepted when they use wrong/unknown variable names
3. Isolated to their specific modules
"""

from typing import List, Tuple


def run_accepted_pattern_tests(
    passed: int,
    failed: int,
    errors: List[str],
    verbose: bool = False,
) -> Tuple[int, int, List[str]]:
    """Run accepted pattern negative tests.
    
    Tests 9-14: Prove accepted patterns are narrow and isolated
    
    Args:
        passed: Current passed count
        failed: Current failed count
        errors: Current error messages
        verbose: Print verbose output
        
    Returns:
        Tuple of (passed, failed, errors)
    """
    from .scanner import is_accepted_pattern
    
    # Test 9: @enumFromInt with wrong variable fails in bfd/packet.zig
    if verbose:
        print("\nScanner test 9: @enumFromInt with wrong variable fails in bfd/packet.zig")
    
    # Verify the safe pattern IS accepted
    safe_line = "    const state: State = @enumFromInt(state_val);"
    accepted, _ = is_accepted_pattern("bfd/packet.zig", safe_line)
    if accepted:
        passed += 1
        if verbose:
            print("  PASS: @enumFromInt(state_val) IS accepted in bfd/packet.zig")
    else:
        failed += 1
        msg = "@enumFromInt(state_val) should be accepted in bfd/packet.zig"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    # Verify the unsafe pattern is NOT accepted (wrong variable name)
    unsafe_line = "    const state: State = @enumFromInt(raw_val);"
    accepted, _ = is_accepted_pattern("bfd/packet.zig", unsafe_line)
    if not accepted:
        passed += 1
        if verbose:
            print("  PASS: @enumFromInt(raw_val) NOT accepted (wrong variable)")
    else:
        failed += 1
        msg = "@enumFromInt(raw_val) should NOT be accepted in bfd/packet.zig"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    # Test 10: Known-safe .? accepted in net/ss_parser.zig
    if verbose:
        print("\nScanner test 10: Known-safe .? patterns in ss_parser.zig")
    
    # retransmits.? is accepted
    ss_safe_line = "    if (retransmits != null and retransmits.? > 0) {"
    accepted, _ = is_accepted_pattern("net/ss_parser.zig", ss_safe_line)
    if accepted:
        passed += 1
        if verbose:
            print("  PASS: retransmits.? IS accepted in ss_parser.zig")
    else:
        failed += 1
        msg = "retransmits.? should be accepted in ss_parser.zig"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    # colon_idx.? is accepted
    ss_safe_line2 = "    const port_str = addr[colon_idx.? + 1 ..];"
    accepted, _ = is_accepted_pattern("net/ss_parser.zig", ss_safe_line2)
    if accepted:
        passed += 1
        if verbose:
            print("  PASS: colon_idx.? IS accepted in ss_parser.zig")
    else:
        failed += 1
        msg = "colon_idx.? should be accepted in ss_parser.zig"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    # Test 11: Arbitrary .? fails in net/ss_parser.zig
    if verbose:
        print("\nScanner test 11: Arbitrary .? fails in ss_parser.zig")
    
    # Arbitrary maybe.? should NOT be accepted
    ss_unsafe_line = "    const val = maybe.?.value;"
    accepted, _ = is_accepted_pattern("net/ss_parser.zig", ss_unsafe_line)
    if not accepted:
        passed += 1
        if verbose:
            print("  PASS: maybe.? NOT accepted in ss_parser.zig (arbitrary .? blocked)")
    else:
        failed += 1
        msg = "maybe.? should NOT be accepted in ss_parser.zig"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    # Test 12: Known-safe .? accepted in net/wg_show_parser.zig
    if verbose:
        print("\nScanner test 12: Known-safe .? patterns in wg_show_parser.zig")
    
    # latest_handshake.? is accepted
    wg_safe_line = "        if (latest_handshake == null or age < latest_handshake.?) {"
    accepted, _ = is_accepted_pattern("net/wg_show_parser.zig", wg_safe_line)
    if accepted:
        passed += 1
        if verbose:
            print("  PASS: latest_handshake.? IS accepted in wg_show_parser.zig")
    else:
        failed += 1
        msg = "latest_handshake.? should be accepted in wg_show_parser.zig"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    # Test 13: Arbitrary .? fails in net/wg_show_parser.zig
    if verbose:
        print("\nScanner test 13: Arbitrary .? fails in wg_show_parser.zig")
    
    # Arbitrary maybe.? should NOT be accepted
    wg_unsafe_line = "    const val = maybe.?;"
    accepted, _ = is_accepted_pattern("net/wg_show_parser.zig", wg_unsafe_line)
    if not accepted:
        passed += 1
        if verbose:
            print("  PASS: maybe.? NOT accepted in wg_show_parser.zig (arbitrary .? blocked)")
    else:
        failed += 1
        msg = "maybe.? should NOT be accepted in wg_show_parser.zig"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    # Test 14: Pattern isolation - known-safe .? NOT accepted in other modules
    if verbose:
        print("\nScanner test 14: Pattern isolation for known-safe .? in other modules")
    
    # retransmits.? should NOT be accepted in bgp/config_parse.zig
    accepted, _ = is_accepted_pattern("bgp/config_parse.zig", ss_safe_line)
    if not accepted:
        passed += 1
        if verbose:
            print("  PASS: retransmits.? NOT accepted in bgp/config_parse.zig (isolated)")
    else:
        failed += 1
        msg = "retransmits.? should NOT be accepted in bgp/config_parse.zig"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    # latest_handshake.? should NOT be accepted in bgp/config_parse.zig
    accepted, _ = is_accepted_pattern("bgp/config_parse.zig", wg_safe_line)
    if not accepted:
        passed += 1
        if verbose:
            print("  PASS: latest_handshake.? NOT accepted in bgp/config_parse.zig (isolated)")
    else:
        failed += 1
        msg = "latest_handshake.? should NOT be accepted in bgp/config_parse.zig"
        errors.append(msg)
        if verbose:
            print(f"  FAIL: {msg}")
    
    return passed, failed, errors
