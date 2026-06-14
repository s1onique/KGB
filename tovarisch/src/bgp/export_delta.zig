// bgp/export_delta.zig — Prefix export delta computation
//
// ACT: Apply BGP export deltas after watched prefix reload (Phase 2)
//
// This module computes the delta between currently exported prefixes and
// newly loaded candidate prefixes, producing:
//   - added prefixes (in candidate but not in current)
//   - removed prefixes (in current but not in candidate)
//   - unchanged count
//
// Design:
//   - Deterministic sorted comparison for O(n log n) complexity
//   - Caller-provided allocator for all returned slices
//   - No global mutable state
//   - No stack-backed returned slices
//
// Key constraint: This module only computes deltas. Applying deltas to
// BGP sessions is handled by session.applyDelta().

const std = @import("std");
const types = @import("types.zig");

/// Result of computing prefix export delta.
pub const DeltaResult = struct {
    /// Prefixes that are new (in candidate but not in current).
    /// Caller must free this slice using the same allocator used to create it.
    added: []types.Ipv4Prefix,
    /// Prefixes that were removed (in current but not in candidate).
    /// Caller must free this slice using the same allocator used to create it.
    removed: []types.Ipv4Prefix,
    /// Number of prefixes that are unchanged between current and candidate.
    unchanged_count: usize,
};

/// Compare two prefixes for sorting and equality.
/// Returns negative if a < b, positive if a > b, zero if equal.
fn comparePrefixes(a: types.Ipv4Prefix, b: types.Ipv4Prefix) i32 {
    for (0..4) |i| {
        if (a.addr[i] < b.addr[i]) return -1;
        if (a.addr[i] > b.addr[i]) return 1;
    }
    if (a.len < b.len) return -1;
    if (a.len > b.len) return 1;
    return 0;
}

/// Check if two prefixes are equal.
pub fn prefixesEqual(a: types.Ipv4Prefix, b: types.Ipv4Prefix) bool {
    return comparePrefixes(a, b) == 0;
}

/// Sort a slice of prefixes in-place using std.mem.sort (O(n log n)).
fn sortPrefixes(prefixes: []types.Ipv4Prefix) void {
    std.mem.sort(types.Ipv4Prefix, prefixes, {}, struct {
        fn lessThan(_: void, a: types.Ipv4Prefix, b: types.Ipv4Prefix) bool {
            return comparePrefixes(a, b) < 0;
        }
    }.lessThan);
}

/// Compute the delta between current and candidate prefix sets.
///
/// Memory ownership:
///   - Caller provides allocator
///   - Caller must free returned added and removed slices
///
/// Time complexity: O(n log n) for sorting + O(n+m) for delta computation
/// Uses std.mem.sort which provides O(n log n) worst-case complexity.
pub fn computeDelta(
    allocator: std.mem.Allocator,
    current: []const types.Ipv4Prefix,
    candidate: []const types.Ipv4Prefix,
) !DeltaResult {
    // Handle empty inputs
    if (current.len == 0 and candidate.len == 0) {
        return DeltaResult{ .added = &.{}, .removed = &.{}, .unchanged_count = 0 };
    }
    if (current.len == 0) {
        const added = try allocator.alloc(types.Ipv4Prefix, candidate.len);
        @memcpy(added, candidate);
        return DeltaResult{ .added = added, .removed = &.{}, .unchanged_count = 0 };
    }
    if (candidate.len == 0) {
        const removed = try allocator.alloc(types.Ipv4Prefix, current.len);
        @memcpy(removed, current);
        return DeltaResult{ .added = &.{}, .removed = removed, .unchanged_count = 0 };
    }

    // Copy and sort both sets
    const current_sorted = try allocator.alloc(types.Ipv4Prefix, current.len);
    errdefer allocator.free(current_sorted);
    @memcpy(current_sorted, current);
    sortPrefixes(current_sorted);

    const candidate_sorted = try allocator.alloc(types.Ipv4Prefix, candidate.len);
    errdefer allocator.free(candidate_sorted);
    @memcpy(candidate_sorted, candidate);
    sortPrefixes(candidate_sorted);

    // Count deltas using two-pointer merge-like approach
    var added_count: usize = 0;
    var removed_count: usize = 0;
    var unchanged_count: usize = 0;
    var i: usize = 0;
    var j: usize = 0;

    while (i < current_sorted.len and j < candidate_sorted.len) {
        const cmp = comparePrefixes(current_sorted[i], candidate_sorted[j]);
        if (cmp == 0) {
            unchanged_count += 1;
            i += 1;
            j += 1;
        } else if (cmp < 0) {
            removed_count += 1;
            i += 1;
        } else {
            added_count += 1;
            j += 1;
        }
    }
    while (i < current_sorted.len) : (i += 1) removed_count += 1;
    while (j < candidate_sorted.len) : (j += 1) added_count += 1;

    // Allocate result slices
    var added: []types.Ipv4Prefix = if (added_count > 0)
        try allocator.alloc(types.Ipv4Prefix, added_count)
    else
        &.{};
    errdefer if (added_count > 0) allocator.free(added);
    var removed: []types.Ipv4Prefix = if (removed_count > 0)
        try allocator.alloc(types.Ipv4Prefix, removed_count)
    else
        &.{};
    errdefer if (removed_count > 0) allocator.free(removed);

    // Fill result slices
    var added_idx: usize = 0;
    var removed_idx: usize = 0;
    i = 0;
    j = 0;
    while (i < current_sorted.len and j < candidate_sorted.len) {
        const cmp = comparePrefixes(current_sorted[i], candidate_sorted[j]);
        if (cmp == 0) {
            i += 1;
            j += 1;
        } else if (cmp < 0) {
            removed[removed_idx] = current_sorted[i];
            removed_idx += 1;
            i += 1;
        } else {
            added[added_idx] = candidate_sorted[j];
            added_idx += 1;
            j += 1;
        }
    }
    while (i < current_sorted.len) : (i += 1) {
        removed[removed_idx] = current_sorted[i];
        removed_idx += 1;
    }
    while (j < candidate_sorted.len) : (j += 1) {
        added[added_idx] = candidate_sorted[j];
        added_idx += 1;
    }

    allocator.free(current_sorted);
    allocator.free(candidate_sorted);

    return DeltaResult{ .added = added, .removed = removed, .unchanged_count = unchanged_count };
}

// ============================================================================
// Unit Tests
// ============================================================================

test "delta with added prefix" {
    const allocator = std.testing.allocator;
    const current = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("192.168.0.0/16"),
    };
    const candidate = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("192.168.0.0/16"),
        types.Ipv4Prefix.init("172.16.0.0/12"),
    };
    const delta = try computeDelta(allocator, current, candidate);
    defer {
        allocator.free(delta.added);
        allocator.free(delta.removed);
    }
    try std.testing.expect(delta.added.len == 1);
    try std.testing.expect(delta.removed.len == 0);
    try std.testing.expect(delta.unchanged_count == 2);
    try std.testing.expect(prefixesEqual(delta.added[0], types.Ipv4Prefix.init("172.16.0.0/12")));
}

test "delta with removed prefix" {
    const allocator = std.testing.allocator;
    const current = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("192.168.0.0/16"),
        types.Ipv4Prefix.init("172.16.0.0/12"),
    };
    const candidate = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("192.168.0.0/16"),
    };
    const delta = try computeDelta(allocator, current, candidate);
    defer {
        allocator.free(delta.added);
        allocator.free(delta.removed);
    }
    try std.testing.expect(delta.added.len == 0);
    try std.testing.expect(delta.removed.len == 1);
    try std.testing.expect(delta.unchanged_count == 2);
    try std.testing.expect(prefixesEqual(delta.removed[0], types.Ipv4Prefix.init("172.16.0.0/12")));
}

test "delta with added and removed prefixes" {
    const allocator = std.testing.allocator;
    const current = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("192.168.0.0/16"),
    };
    const candidate = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("172.16.0.0/12"),
    };
    const delta = try computeDelta(allocator, current, candidate);
    defer {
        allocator.free(delta.added);
        allocator.free(delta.removed);
    }
    try std.testing.expect(delta.added.len == 1);
    try std.testing.expect(delta.removed.len == 1);
    try std.testing.expect(delta.unchanged_count == 1);
}

test "delta identical sets produces empty delta" {
    const allocator = std.testing.allocator;
    const current = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("172.16.0.0/12"),
        types.Ipv4Prefix.init("192.168.0.0/16"),
    };
    const candidate = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("172.16.0.0/12"),
        types.Ipv4Prefix.init("192.168.0.0/16"),
    };
    const delta = try computeDelta(allocator, current, candidate);
    defer {
        allocator.free(delta.added);
        allocator.free(delta.removed);
    }
    try std.testing.expect(delta.added.len == 0);
    try std.testing.expect(delta.removed.len == 0);
    try std.testing.expect(delta.unchanged_count == 3);
}

test "delta handles empty current" {
    const allocator = std.testing.allocator;
    const current: []const types.Ipv4Prefix = &.{};
    const candidate = &.{types.Ipv4Prefix.init("10.0.0.0/8")};
    const delta = try computeDelta(allocator, current, candidate);
    defer {
        allocator.free(delta.added);
        allocator.free(delta.removed);
    }
    try std.testing.expect(delta.added.len == 1);
    try std.testing.expect(delta.removed.len == 0);
}

test "delta handles empty candidate" {
    const allocator = std.testing.allocator;
    const current = &.{types.Ipv4Prefix.init("10.0.0.0/8")};
    const candidate: []const types.Ipv4Prefix = &.{};
    const delta = try computeDelta(allocator, current, candidate);
    defer {
        allocator.free(delta.added);
        allocator.free(delta.removed);
    }
    try std.testing.expect(delta.added.len == 0);
    try std.testing.expect(delta.removed.len == 1);
}

test "delta handles both empty" {
    const allocator = std.testing.allocator;
    const current: []const types.Ipv4Prefix = &.{};
    const candidate: []const types.Ipv4Prefix = &.{};
    const delta = try computeDelta(allocator, current, candidate);
    defer {
        allocator.free(delta.added);
        allocator.free(delta.removed);
    }
    try std.testing.expect(delta.added.len == 0);
    try std.testing.expect(delta.removed.len == 0);
    try std.testing.expect(delta.unchanged_count == 0);
}

test "delta handles unsorted input" {
    const allocator = std.testing.allocator;
    const current = &.{
        types.Ipv4Prefix.init("192.168.0.0/16"),
        types.Ipv4Prefix.init("10.0.0.0/8"),
    };
    const candidate = &.{
        types.Ipv4Prefix.init("172.16.0.0/12"),
        types.Ipv4Prefix.init("10.0.0.0/8"),
    };
    const delta = try computeDelta(allocator, current, candidate);
    defer {
        allocator.free(delta.added);
        allocator.free(delta.removed);
    }
    try std.testing.expect(delta.added.len == 1);
    try std.testing.expect(delta.removed.len == 1);
    try std.testing.expect(delta.unchanged_count == 1);
}
