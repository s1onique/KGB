// status_query.zig — Closed query model for /status.json requests
//
// This module defines a closed type system for parsing status query parameters.
// Query parsing is represented as a closed enum, not raw strings.
//
// Design goals:
// - No ad hoc string checks leak across the codebase
// - Query parsing is deterministic and testable without sockets
// - Unknown/malformed queries are explicitly classified

const std = @import("std");

/// Closed enum representing parsed status query includes.
///
/// This is the canonical type for query parameter classification.
/// All query parsing happens here, and callers work with this enum.
pub const StatusInclude = enum {
    /// No includes requested - base status only
    none,

    /// Include network diagnostics section
    network_diag,

    /// Query value is recognized but unsupported
    unsupported,
};

/// Closed query model for status requests.
///
/// Represents the parsed query state of a /status.json request.
/// This is a pure data type with no allocator dependency.
pub const StatusQuery = struct {
    /// Parsed include directive.
    include: StatusInclude = .none,

    /// True if query contained duplicate include parameters.
    /// When true, behavior is deterministic (uses first occurrence).
    has_duplicate: bool = false,

    /// True if query contained values that are not recognized.
    has_unknown: bool = false,

    /// Parse a raw query string into a StatusQuery.
    ///
    /// Supports:
    /// - Empty query (no includes)
    /// - include=network_diag
    /// - include=network_diag&other=value (unknown params ignored)
    /// - other=value&include=network_diag
    ///
    /// Malformed query handling:
    /// - Malformed key=value pairs are skipped (has_unknown = true)
    /// - Duplicate include parameters are ignored after first (has_duplicate = true)
    /// - Empty query returns StatusInclude.none
    pub fn parse(query: []const u8) StatusQuery {
        if (query.len == 0) {
            return .{ .include = .none };
        }

        var result = StatusQuery{};
        var seen_include = false;
        var it = std.mem.splitScalar(u8, query, '&');

        while (it.next()) |part| {
            if (part.len == 0) continue; // Skip empty parts

            // Check for include= prefix
            const prefix = "include=";
            if (part.len > prefix.len and std.mem.eql(u8, part[0..prefix.len], prefix)) {
                const value = part[prefix.len..];

                if (std.mem.eql(u8, value, "network_diag")) {
                    if (seen_include) {
                        // Duplicate - mark but keep first occurrence
                        result.has_duplicate = true;
                    } else {
                        seen_include = true;
                        result.include = .network_diag;
                    }
                } else {
                    // Known include name but unsupported value
                    if (!seen_include) {
                        result.include = .unsupported;
                    }
                    result.has_unknown = true;
                }
            } else if (std.mem.indexOfScalar(u8, part, '=')) |_| {
                // Other key=value pairs - ignore
            } else {
                // Malformed (no '=') - treat as unknown
                result.has_unknown = true;
            }
        }

        return result;
    }

    /// Returns true if this query should include network diagnostics.
    pub fn wantsNetworkDiag(self: StatusQuery) bool {
        return self.include == .network_diag;
    }

    /// Returns true if query has any anomalies.
    /// Anomalies don't change behavior but may be logged.
    pub fn hasAnomaly(self: StatusQuery) bool {
        return self.has_duplicate or self.has_unknown;
    }
};

/// Select the response mode based on the parsed query.
///
/// This is a pure function with no allocator dependency.
/// It can be tested without sockets or network.
///
/// Returns a tag indicating what response variant to use.
pub fn selectResponseMode(query: StatusQuery) ResponseMode {
    _ = query; // Currently only one supported include, reserved for future expansion
    return .status_with_context;
}

/// Closed enum for response mode selection.
///
/// This enum determines which response variant to render.
/// Currently minimal but extensible for future response types.
pub const ResponseMode = enum {
    /// Full status response with runtime context (checks, BGP, BFD).
    /// This is the only current mode but is explicit for future expansion.
    status_with_context,

    // Reserved for future modes:
    // minimal,
    // diagnostic_only,
    // health_check,
};

test "StatusQuery.parse handles empty query" {
    const query = StatusQuery.parse("");
    try std.testing.expectEqual(StatusInclude.none, query.include);
    try std.testing.expect(!query.has_duplicate);
    try std.testing.expect(!query.has_unknown);
}

test "StatusQuery.parse handles include=network_diag" {
    const query = StatusQuery.parse("include=network_diag");
    try std.testing.expectEqual(StatusInclude.network_diag, query.include);
    try std.testing.expect(!query.has_duplicate);
    try std.testing.expect(!query.has_unknown);
}

test "StatusQuery.parse handles include=network_diag with other params" {
    const query = StatusQuery.parse("foo=bar&include=network_diag&baz=qux");
    try std.testing.expectEqual(StatusInclude.network_diag, query.include);
    try std.testing.expect(!query.has_duplicate);
    try std.testing.expect(!query.has_unknown);
}

test "StatusQuery.parse handles unknown include value" {
    const query = StatusQuery.parse("include=unknown_value");
    try std.testing.expectEqual(StatusInclude.unsupported, query.include);
    try std.testing.expect(query.has_unknown);
}

test "StatusQuery.parse handles empty include value" {
    // Empty value after "include=" is treated as no include (falls through to ignore)
    // because part.len > prefix.len check fails for exactly "include="
    const query = StatusQuery.parse("include=");
    try std.testing.expectEqual(StatusInclude.none, query.include);
    try std.testing.expect(!query.has_unknown);
}

test "StatusQuery.parse handles duplicate include" {
    const query = StatusQuery.parse("include=network_diag&include=network_diag");
    try std.testing.expectEqual(StatusInclude.network_diag, query.include);
    try std.testing.expect(query.has_duplicate);
    try std.testing.expect(!query.has_unknown);
}

test "StatusQuery.parse handles unknown params with known include" {
    const query = StatusQuery.parse("foo=bar&include=network_diag&baz=qux");
    try std.testing.expectEqual(StatusInclude.network_diag, query.include);
    try std.testing.expect(!query.has_duplicate);
    try std.testing.expect(!query.has_unknown);
}

test "StatusQuery.parse ignores params without equals" {
    const query = StatusQuery.parse("include=network_diag&nokey");
    try std.testing.expectEqual(StatusInclude.network_diag, query.include);
    try std.testing.expect(query.has_unknown);
}

test "StatusQuery.wantsNetworkDiag returns true for network_diag" {
    const query = StatusQuery.parse("include=network_diag");
    try std.testing.expect(query.wantsNetworkDiag());
}

test "StatusQuery.wantsNetworkDiag returns false for empty query" {
    const query = StatusQuery.parse("");
    try std.testing.expect(!query.wantsNetworkDiag());
}

test "StatusQuery.hasAnomaly returns false for clean query" {
    const query = StatusQuery.parse("include=network_diag");
    try std.testing.expect(!query.hasAnomaly());
}

test "StatusQuery.hasAnomaly returns true for duplicate" {
    const query = StatusQuery.parse("include=network_diag&include=network_diag");
    try std.testing.expect(query.hasAnomaly());
}

test "selectResponseMode is pure and deterministic" {
    const query = StatusQuery.parse("include=network_diag");
    const mode1 = selectResponseMode(query);
    const mode2 = selectResponseMode(query);
    try std.testing.expect(mode1 == mode2);
    try std.testing.expect(mode1 == .status_with_context);
}
