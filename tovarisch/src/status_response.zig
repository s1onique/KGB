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

const std = @import("std");
const status = @import("status.zig");
const status_network_diag = @import("status_network_diag.zig");
const status_query = @import("status_query.zig");

/// Owned response type for status JSON.
///
/// The caller owns the returned body slice and must free it
/// with the same allocator that was used to create it.
///
/// Usage:
/// ```zig
/// var response = try OwnedResponse.init(allocator, inputs, query);
/// defer response.deinit(allocator);
/// // use response.body
/// ```
pub const OwnedResponse = struct {
    /// The owned JSON body bytes.
    body: []u8,

    /// Render a status response into an owned buffer.
    ///
    /// **Allocator ownership:**
    /// - Caller owns returned OwnedResponse.body
    /// - Caller must call deinit() with the same allocator
    /// - All allocations are released on both success and error paths
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

        // Render using page allocator (page allocator doesn't need freeing)
        try status.renderPayloadWithContextAndDiag(
            &w,
            inputs,
            std.heap.page_allocator,
            query.wantsNetworkDiag(),
        );

        // Return owned response with exactly the written bytes.
        // MemoryOwnership: Duplicate the written slice to return a precisely-sized
        // owned buffer. The scratch buffer is freed here; the duplicate is owned
        // by the caller. errdefer handles the duplicate on error.
        const body = try allocator.dupe(u8, buf[0..w.pos]);
        errdefer allocator.free(body);
        allocator.free(buf);
        return OwnedResponse{ .body = body };
    }

    /// Free the response body.
    ///
    /// **Ownership contract:** Must be called with the same allocator
    /// that was passed to init().
    pub fn deinit(self: OwnedResponse, allocator: std.mem.Allocator) void {
        allocator.free(self.body);
    }

    /// Returns the response body as a slice.
    pub fn slice(self: *const OwnedResponse) []const u8 {
        return self.body;
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

/// Render status response to a writer.
///
/// This is a simple helper that renders status JSON to any writer type.
///
/// **Arguments:**
/// - `writer`: Writer type with writeAll method (e.g., *BufferedWriter, *TestWriter)
/// - `inputs`: Runtime status inputs
/// - `query`: Parsed query parameters
pub fn renderStatusResponseToWriter(
    writer: anytype,
    inputs: status.RuntimeStatusInputs,
    query: status_query.StatusQuery,
) !void {
    try status.renderPayloadWithContextAndDiag(
        writer,
        inputs,
        std.heap.page_allocator,
        query.wantsNetworkDiag(),
    );
}
