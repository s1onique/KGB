/// BGP UPDATE diagnostics for structured logging.
///
/// This module provides UPDATE frame logging for the BGP runtime.
/// Logs are emitted as structured JSON records to stdout.
/// 
/// Uses session.last_update_diagnostic captured BEFORE flush for accurate diagnostics.

const std = @import("std");
const logging = @import("../logging.zig");
const session = @import("session.zig");

/// Count of UPDATE frames to log before suppressing.
const UPDATE_LOG_LIMIT: usize = 5;

/// Track how many UPDATE frames have been logged.
var update_log_count: usize = 0;

/// Log an outgoing BGP UPDATE with structured fields.
fn logOutgoingUpdate(info: session.UpdateInfo) void {
    if (update_log_count >= UPDATE_LOG_LIMIT) return;
    update_log_count += 1;

    var log_buf = logging.BufferedWriter.init();
    logging.emit(.bgp_outgoing_update, &log_buf, &.{
        .{ .name = "len", .value = logging.FieldValue{ .integer = info.len } },
        .{ .name = "withdrawn_len", .value = logging.FieldValue{ .integer = info.withdrawn_len } },
        .{ .name = "attrs_len", .value = logging.FieldValue{ .integer = info.attrs_len } },
        .{ .name = "nlri_prefixes", .value = logging.FieldValue{ .integer = @as(i64, @intCast(info.nlri_prefixes)) } },
        .{ .name = "nlri_bytes", .value = logging.FieldValue{ .integer = @as(i64, @intCast(info.nlri_bytes)) } },
        .{ .name = "batch_end", .value = logging.FieldValue{ .integer = @as(i64, @intCast(info.batch_end)) } },
        .{ .name = "configured", .value = logging.FieldValue{ .integer = @as(i64, @intCast(info.configured)) } },
    }) catch return;
    _ = std.c.write(1, log_buf.slice().ptr, log_buf.slice().len);
}

/// Log UPDATE parse failure event.
fn logOutgoingUpdateParseFailed() void {
    var log_buf = logging.BufferedWriter.init();
    logging.emit(.bgp_outgoing_update_parse_failed, &log_buf, &.{
        .{ .name = "detail", .value = logging.FieldValue{ .string = "failed to parse encoded UPDATE before flush" } },
    }) catch return;
    _ = std.c.write(1, log_buf.slice().ptr, log_buf.slice().len);
}

/// Log UPDATE decode failure event.
fn logOutgoingUpdateDecodeFailed() void {
    var log_buf = logging.BufferedWriter.init();
    logging.emit(.bgp_outgoing_update_decode_failed, &log_buf, &.{
        .{ .name = "detail", .value = logging.FieldValue{ .string = "failed to decode frame" } },
    }) catch return;
    _ = std.c.write(1, log_buf.slice().ptr, log_buf.slice().len);
}

/// Log UPDATE from session after runOnce.
/// Uses session.last_update_diagnostic captured before flush.
/// Emits success, decode failure, or parse failure events.
pub fn logUpdateFromSession(sess: *session.Session) void {
    switch (sess.last_update_diagnostic) {
        .none => {},
        .sent => |info| logOutgoingUpdate(info),
        .decode_failed => logOutgoingUpdateDecodeFailed(),
        .parse_failed => logOutgoingUpdateParseFailed(),
    }
}
