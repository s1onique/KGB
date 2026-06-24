/// Startup log emission helpers for HTTP server.
///
/// Extracts startup logging to keep server.zig under LLM-friendliness limits.
///
/// Memory ownership: No allocations. All functions use stack-allocated buffers.

const std = @import("std");
const logging = @import("../logging.zig");
const cli_args = @import("../cli/args.zig");

/// Write a log record to the output and flush.
pub fn writeLogRecord(out_writer: anytype, bytes: []const u8) !void {
    try out_writer.writeAll(bytes);

    // Flush if the writer supports it (not BufferedWriter).
    if (comptime @TypeOf(out_writer) == *logging.BufferedWriter) {
        // No-op: BufferedWriter doesn't have flush
    } else {
        out_writer.flush() catch {};
    }
}

/// Emit startup log events after successful server listen.
pub fn emitStartupLogs(
    port: u16,
    address: []const u8,
    out_writer: anytype,
) !void {
    var log_buf = logging.BufferedWriter.init();
    try logging.emit(.http_server_listening, &log_buf, &.{
        .{ .name = "bind_address", .value = logging.FieldValue{ .string = address } },
        .{ .name = "port", .value = logging.FieldValue{ .integer = port } },
    });
    try writeLogRecord(out_writer, log_buf.slice());

    log_buf.reset();
    try logging.emit(.uvb76_signal_ready, &log_buf, &.{
        .{ .name = "signal", .value = logging.FieldValue{ .string = "🚩📻" } },
        .{ .name = "message", .value = logging.FieldValue{ .string = "Listen to UVB-76 signals..." } },
    });
    try writeLogRecord(out_writer, log_buf.slice());
}

/// Emit startup logs only in normal mode (not statonly).
pub fn emitStartupLogsIfNormal(
    log_mode: cli_args.LogMode,
    port: u16,
    address: []const u8,
    out_writer: anytype,
) !void {
    if (log_mode == .normal) {
        try emitStartupLogs(port, address, out_writer);
    }
}
