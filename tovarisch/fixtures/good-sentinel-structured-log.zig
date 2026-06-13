// good-sentinel-structured-log.zig
//
// GOOD SENTINEL: This file demonstrates the correct pattern.
// Uses structured JSON logging via logging.emit().
//
// This file exists to verify the gate allows proper structured logging.
const std = @import("std");
const logging = @import("logging.zig");

// GOOD: structured logging using logging.emit()
pub fn goodLogExample(writer: *logging.BufferedWriter) !void {
    try logging.emit(.http_server_listening, writer, &.{
        .{ .name = "port", .value = logging.FieldValue{ .integer = 8317 } },
        .{ .name = "addr", .value = logging.FieldValue{ .string = "127.0.0.1" } },
    });
}

// GOOD: structured error logging
pub fn goodErrorLogExample(writer: *logging.BufferedWriter) !void {
    try logging.emit(.server_error, writer, &.{
        .{ .name = "error", .value = logging.FieldValue{ .string = "BindFailed" } },
        .{ .name = "detail", .value = logging.FieldValue{ .string = "Address already in use" } },
    });
}
