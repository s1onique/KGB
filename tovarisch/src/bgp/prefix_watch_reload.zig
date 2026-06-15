// bgp/prefix_watch_reload.zig — Prefix reload logic
//
// Implements the "reload-as-transaction" pattern:
// 1. Read all configured prefix files
// 2. Parse into candidate prefix set
// 3. Validate
// 4. Commit on success, preserve last-good on failure
//
// Key constraint: Do NOT mutate live export set until candidate parse succeeds.

const std = @import("std");
const prefix_file_loader = @import("prefix_file_loader.zig");
const prefix_file = @import("prefix_file.zig");
const config_parse = @import("config_parse.zig");
const prefix_watch = @import("prefix_watch.zig");
const types = @import("types.zig");

/// Error buffer size for last-good state.
const ERROR_BUF_SIZE: usize = 256;

/// Reload prefixes from all configured files.
/// This is the "reload-as-transaction" function.
///
/// Returns ReloadResult with:
/// - success=true: new prefix set is valid and committed
/// - success=false: last-good state is preserved, error_message is set
pub fn reloadPrefixes(
    advertised_prefix_files_raw: []const u8,
    allocator: std.mem.Allocator,
    last_good: *prefix_watch.LastGoodState,
) prefix_watch.ReloadResult {
    // Parse file paths from config
    const file_paths = config_parse.parsePrefixList(advertised_prefix_files_raw, allocator) catch |e| {
        last_good.last_error = copyError(last_good, @errorName(e));
        return prefix_watch.ReloadResult{
            .success = false,
            .prefix_count = last_good.prefixes.len,
            .error_message = last_good.last_error,
            .error_paths = &.{},
        };
    };
    defer allocator.free(file_paths);

    // Empty input string means no files configured - this is valid
    if (advertised_prefix_files_raw.len == 0) {
        const empty: []types.Ipv4Prefix = &.{};
        last_good.prefixes = empty;
        last_good.last_error = null;
        last_good.has_value = true;
        return prefix_watch.ReloadResult{
            .success = true,
            .prefix_count = 0,
            .error_message = null,
            .error_paths = &.{},
        };
    }

    // Non-empty input but zero files returned - this is a failure (invalid path)
    if (file_paths.len == 0) {
        last_good.last_error = copyError(last_good, "invalid file path");
        return prefix_watch.ReloadResult{
            .success = false,
            .prefix_count = last_good.prefixes.len,
            .error_message = last_good.last_error,
            .error_paths = &.{},
        };
    }

    // Load and parse all files
    var prefixes = std.ArrayList(types.Ipv4Prefix).empty;
    errdefer prefixes.deinit(allocator);

    var error_paths = std.ArrayList([]const u8).empty;
    errdefer {
        for (error_paths.items) |p| allocator.free(p);
        error_paths.deinit(allocator);
    }

    var any_file_failed = false;

    for (file_paths) |file_path| {
        // Read file content
        const file_content = prefix_file_loader.loadPrefixFile(file_path, allocator) catch |e| {
            last_good.last_error = copyError(last_good, @errorName(e));
            any_file_failed = true;
            continue;
        };
        defer allocator.free(file_content);

        // Parse prefixes
        const parse_result = prefix_file.parse(file_content, allocator) catch |e| {
            last_good.last_error = copyError(last_good, @errorName(e));
            any_file_failed = true;
            continue;
        };
        defer allocator.free(parse_result.prefixes);

        // Append prefixes from this file
        for (parse_result.prefixes) |prefix| {
            prefixes.append(allocator, prefix) catch {
                last_good.last_error = copyError(last_good, "out of memory");
                return prefix_watch.ReloadResult{
                    .success = false,
                    .prefix_count = last_good.prefixes.len,
                    .error_message = last_good.last_error,
                    .error_paths = error_paths.items,
                };
            };
        }
    }

    // If any files had errors, report failure but preserve last-good
    if (any_file_failed) {
        return prefix_watch.ReloadResult{
            .success = false,
            .prefix_count = last_good.prefixes.len,
            .error_message = last_good.last_error,
            .error_paths = error_paths.items,
        };
    }

    // Success - commit new prefix set
    last_good.prefixes = prefixes.toOwnedSlice(allocator) catch {
        last_good.last_error = copyError(last_good, "out of memory");
        return prefix_watch.ReloadResult{
            .success = false,
            .prefix_count = last_good.prefixes.len,
            .error_message = last_good.last_error,
            .error_paths = &.{},
        };
    };
    last_good.last_error = null;
    last_good.has_value = true;

    return prefix_watch.ReloadResult{
        .success = true,
        .prefix_count = last_good.prefixes.len,
        .error_message = null,
        .error_paths = &.{},
    };
}

/// Copy an error message into the last-good state buffer.
fn copyError(last_good: *prefix_watch.LastGoodState, message: []const u8) []const u8 {
    const copy_len = @min(message.len, last_good.error_buf.len - 1);
    // MemoryCopySafety: last_good.error_buf is a fixed [256]u8 buffer. message is a
    // caller-provided slice. They are distinct memory regions; no aliasing possible.
    @memcpy(last_good.error_buf[0..copy_len], message[0..copy_len]);
    last_good.error_buf[copy_len] = 0;
    last_good.last_error = last_good.error_buf[0..copy_len];
    return last_good.last_error.?;
}
