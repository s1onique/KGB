// status_response.zig — Allocator-owned status response rendering
//
// This module provides rendering functions for /status.json requests
// with explicit allocator ownership contracts.
//
// Key contracts:
// - All rendering functions accept an explicit allocator parameter
// - Caller owns any allocated buffers
// - Success-path and error-path cleanup are guaranteed
// - No global allocator is used
// - Response budgets enforce maximum response size bounds

const std = @import("std");
const status = @import("status.zig");
const status_network_diag = @import("status_network_diag.zig");
const status_query = @import("status_query.zig");
const status_route_contract = @import("http/status_route_contract.zig");

/// Owned response type for status JSON.
///
/// The caller owns the returned body slice and must free it
/// with the same allocator that was used to create it.
///
/// Uses capacity-aware design: stores the full allocation and exact length
/// separately. This allows precise memory accounting without requiring
/// allocator.shrink() or realloc() semantics.
///
/// Usage:
/// ```zig
/// var response = try OwnedResponse.init(allocator, inputs, query);
/// defer response.deinit(allocator);
/// // use response.slice() or response.body
/// ```
pub const OwnedResponse = struct {
    /// The owned allocation (may be larger than len).
    allocation: []u8,
    /// The exact length of the written data.
    len: usize,

    /// Render a status response into an owned buffer.
    ///
    /// **Allocator ownership:**
    /// - Caller owns returned OwnedResponse.allocation
    /// - Caller must call deinit() with the same allocator
    /// - Single allocation is released on both success and error paths
    ///
    /// **Arguments:**
    /// - `allocator`: The allocator used for response buffer allocation
    /// - `inputs`: Runtime status inputs (BFD, config, BGP state)
    /// - `query`: Parsed query parameters
    ///
    /// **Returns:** OwnedResponse containing the JSON body
    /// **Errors:** Allocator errors on buffer allocation failure
    pub fn init(
        allocator: std.mem.Allocator,
        inputs: status.RuntimeStatusInputs,
        query: status_query.StatusQuery,
    ) !OwnedResponse {
        // Estimate buffer size
        const estimated_size: usize = if (query.wantsNetworkDiag()) 8192 else 4096;

        // Pre-allocate buffer
        const buf = try allocator.alloc(u8, estimated_size);
        errdefer allocator.free(buf);

        // Create a writer that writes to our buffer
        var w = BufferedWriter{
            .buf = buf,
            .pos = 0,
        };

        // Render using the caller's allocator for all request-scoped allocations.
        // MemoryOwnership: renderPayloadWithContextAndDiag uses the allocator for
        // network_diag collection, but deinit is called via defer before return.
        try status.renderPayloadWithContextAndDiag(
            &w,
            inputs,
            allocator,
            query.wantsNetworkDiag(),
        );

        // Return capacity-aware owned response.
        // MemoryOwnership: Single allocation (buf) is owned. The len field tracks
        // the exact written bytes. No dupe operation needed.
        return OwnedResponse{
            .allocation = buf,
            .len = w.pos,
        };
    }

    /// Free the response body.
    ///
    /// **Ownership contract:** Must be called with the same allocator
    /// that was passed to init().
    pub fn deinit(self: OwnedResponse, allocator: std.mem.Allocator) void {
        allocator.free(self.allocation);
    }

    /// Returns the response body as a slice.
    pub fn slice(self: *const OwnedResponse) []const u8 {
        return self.allocation[0..self.len];
    }

    /// Returns the response body as a slice.
    pub fn body(self: *const OwnedResponse) []const u8 {
        return self.allocation[0..self.len];
    }
};

/// Buffered writer for status response rendering.
const BufferedWriter = struct {
    const Self = @This();
    const BufSize = 8192;
    buf: []u8,
    pos: usize = 0,

    pub fn writeAll(self: *Self, bytes: []const u8) !void {
        if (self.pos + bytes.len > self.buf.len) return error.BufferOverflow;
        for (bytes, 0..) |byte, i| {
            self.buf[self.pos + i] = byte;
        }
        self.pos += bytes.len;
    }

    pub fn print(self: *Self, comptime fmt: []const u8, args: anytype) !void {
        const result = std.fmt.bufPrint(self.buf[self.pos..], fmt, args) catch return error.BufferOverflow;
        self.pos += result.len;
    }

    pub fn writeByte(self: *Self, byte: u8) !void {
        if (self.pos >= self.buf.len) return error.BufferOverflow;
        self.buf[self.pos] = byte;
        self.pos += 1;
    }
};

/// Buffered writer for status response rendering with bounded size (runtime budget).
const BudgetedWriter = struct {
    const Self = @This();
    buf: []u8,
    pos: usize = 0,

    pub fn writeAll(self: *Self, bytes: []const u8) !void {
        if (self.pos + bytes.len > self.buf.len) return error.BufferOverflow;
        for (bytes, 0..) |byte, i| {
            self.buf[self.pos + i] = byte;
        }
        self.pos += bytes.len;
    }

    pub fn print(self: *Self, comptime fmt: []const u8, args: anytype) !void {
        if (self.pos >= self.buf.len) return error.BufferOverflow;
        const result = std.fmt.bufPrint(self.buf[self.pos..], fmt, args) catch return error.BufferOverflow;
        self.pos += result.len;
    }

    pub fn writeByte(self: *Self, byte: u8) !void {
        if (self.pos >= self.buf.len) return error.BufferOverflow;
        self.buf[self.pos] = byte;
        self.pos += 1;
    }
};

/// Render status response into an owned buffer with explicit budget enforcement.
///
/// **Allocator ownership:**
/// - Caller owns returned OwnedResponse.allocation
/// - Caller must call deinit() with the same allocator
/// - Single allocation is released on both success and error paths
///
/// **Budget policy:**
/// - Rendered output MUST NOT exceed `budget.max_body_bytes`
/// - Returns `error.BufferOverflow` if output exceeds budget
/// - No partial JSON is returned as success
///
/// **Arguments:**
/// - `allocator`: The allocator used for response buffer allocation
/// - `inputs`: Runtime status inputs (BFD, config, BGP state)
/// - `query`: Parsed query parameters
/// - `budget`: Maximum allowed response body size
///
/// **Returns:** OwnedResponse containing the JSON body
/// **Errors:**
/// - `error.OutOfMemory` on allocation failure
/// - `error.BufferOverflow` if rendered output exceeds budget
pub fn renderStatusOwnedWithBudget(
    allocator: std.mem.Allocator,
    inputs: status.RuntimeStatusInputs,
    query: status_query.StatusQuery,
    budget: status_route_contract.ResponseBudget,
) !OwnedResponse {
    // Allocate buffer matching the budget
    const buf = try allocator.alloc(u8, budget.max_body_bytes);
    errdefer allocator.free(buf);

    // Create budgeted writer that enforces the budget
    var w = BudgetedWriter{
        .buf = buf,
        .pos = 0,
    };

    // Render into budgeted buffer using caller's allocator.
    // MemoryOwnership: All network_diag allocations use the caller's allocator.
    // On error, errdefer frees buf. On success, we return the capacity-aware response.
    try status.renderPayloadWithContextAndDiag(
        &w,
        inputs,
        allocator,
        query.wantsNetworkDiag(),
    );

    // Return capacity-aware owned response.
    // MemoryOwnership: Single allocation (buf) is owned. The len field tracks
    // the exact written bytes. No dupe operation needed.
    return OwnedResponse{
        .allocation = buf,
        .len = w.pos,
    };
}

/// Render status response to a writer.
///
/// This is a simple helper that renders status JSON to any writer type.
/// The caller must provide an allocator for network_diag collection.
///
/// **Arguments:**
/// - `writer`: Writer type with writeAll method (e.g., *BufferedWriter, *TestWriter)
/// - `inputs`: Runtime status inputs
/// - `query`: Parsed query parameters
/// - `allocator`: Allocator for network_diag collection (used for request-scoped diagnostics)
pub fn renderStatusResponseToWriter(
    writer: anytype,
    inputs: status.RuntimeStatusInputs,
    query: status_query.StatusQuery,
    allocator: std.mem.Allocator,
) !void {
    try status.renderPayloadWithContextAndDiag(
        writer,
        inputs,
        allocator,
        query.wantsNetworkDiag(),
    );
}
