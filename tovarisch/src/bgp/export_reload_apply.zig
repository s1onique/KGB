// bgp/export_reload_apply.zig — Prefix reload with live delta application
//
// ACT: Wire watched prefix reload to live BGP export delta application
//
// This module integrates:
//   - Phase 1: inotify watcher + reload detection
//   - Phase 2: export_delta.computeDelta() + session_delta.applyDelta()
//
// Design: "Daemon-state-is-source-of-truth" commit semantics
//   - Reload success: commit candidate as new current_exported_prefixes
//   - Best-effort apply to established sessions
//   - Failed sessions resync from current_exported_prefixes on re-establishment
//
// This avoids the complexity of all-or-nothing semantics when applying
// to multiple sessions (some may be down, some may fail send, etc.).
//
// Key constraints:
//   - No global mutable state
//   - Owner (BgpServeBundle) stores allocator for memory management
//   - current_exported_prefixes ownership is explicit (allocated, freed, replaced)
//   - Reload failure preserves current state, sends NO withdrawals
//   - Candidate ownership follows: adopted on success, freed on failure

const std = @import("std");
const types = @import("types.zig");
const session = @import("session.zig");
const export_delta = @import("export_delta.zig");
const session_delta = @import("session_delta.zig");
const prefix_watch = @import("prefix_watch.zig");
const prefix_watch_reload = @import("prefix_watch_reload.zig");

// ============================================================================
// Reload/Apply Result Types
// ============================================================================

/// Result of a reload + delta apply operation.
pub const ReloadApplyResult = struct {
    /// Whether reload succeeded.
    reload_success: bool,
    /// Reload error message if failed.
    reload_error: ?[]const u8,
    /// Number of prefixes in current export set after reload.
    current_prefix_count: usize,
    /// Number of prefixes added (announced).
    delta_added_count: usize,
    /// Number of prefixes removed (withdrawn).
    delta_removed_count: usize,
    /// Number of prefixes unchanged.
    delta_unchanged_count: usize,
    /// Number of withdrawal UPDATEs sent.
    withdrawals_sent: usize,
    /// Number of announcement UPDATEs sent.
    announcements_sent: usize,
    /// Apply error message if apply to sessions failed.
    apply_error: ?[]const u8,
};

/// Export state owned by the daemon.
/// This struct is embedded in BgpServeBundle to manage the current exported
/// prefix set and reload/apply diagnostics.
pub const ExportState = struct {
    const Self = @This();

    /// Currently exported prefixes (daemon-owned, allocated slice).
    /// This is the source of truth for what we're advertising.
    current_exported_prefixes: []types.Ipv4Prefix = &.{},

    /// Last reload success flag.
    last_reload_success: bool = false,
    /// Last reload error message (backed by error_buf).
    last_reload_error: ?[]const u8 = null,
    /// Error buffer for reload errors.
    reload_error_buf: [256]u8 = undefined,

    /// Last delta counts (from last successful reload).
    last_delta_added_count: usize = 0,
    last_delta_removed_count: usize = 0,
    last_delta_unchanged_count: usize = 0,

    /// Last apply error message (backed by apply_error_buf).
    last_apply_error: ?[]const u8 = null,
    /// Error buffer for apply errors.
    apply_error_buf: [256]u8 = undefined,

    /// Allocator reference (owner is responsible for setting this).
    /// Stored here so we can free/replace current_exported_prefixes.
    allocator: ?std.mem.Allocator = null,

    /// Initialize export state with an allocator.
    pub fn init(self: *Self, allocator: std.mem.Allocator) void {
        self.* = Self{
            .allocator = allocator,
            .current_exported_prefixes = &.{},
        };
    }

    /// Get current exported prefix count.
    pub fn exportedCount(self: *const Self) usize {
        return self.current_exported_prefixes.len;
    }

    /// Clear and free current_exported_prefixes.
    /// Safe to call even if current_exported_prefixes is empty.
    fn freeCurrentPrefixes(self: *Self) void {
        if (self.current_exported_prefixes.len > 0 and self.allocator != null) {
            self.allocator.?.free(self.current_exported_prefixes);
        }
        self.current_exported_prefixes = &.{};
    }

    /// Clear apply error.
    fn clearApplyError(self: *Self) void {
        self.last_apply_error = null;
    }

    /// Set apply error with buffer.
    fn setApplyError(self: *Self, message: []const u8) void {
        const copy_len = @min(message.len, self.apply_error_buf.len - 1);
        @memcpy(self.apply_error_buf[0..copy_len], message[0..copy_len]);
        self.apply_error_buf[copy_len] = 0;
        self.last_apply_error = self.apply_error_buf[0..copy_len];
    }

    /// Clear reload error.
    fn clearReloadError(self: *Self) void {
        self.last_reload_error = null;
    }

    /// Set reload error with buffer.
    fn setReloadError(self: *Self, message: []const u8) void {
        const copy_len = @min(message.len, self.reload_error_buf.len - 1);
        @memcpy(self.reload_error_buf[0..copy_len], message[0..copy_len]);
        self.reload_error_buf[copy_len] = 0;
        self.last_reload_error = self.reload_error_buf[0..copy_len];
    }

    /// Clean up export state (free owned memory).
    pub fn deinit(self: *Self) void {
        self.freeCurrentPrefixes();
    }
};

// ============================================================================
// Reload + Delta + Apply Workflow
// ============================================================================

/// Perform reload + delta + apply on a session.
///
/// Reload failure behavior:
///   - Preserves current_exported_prefixes
///   - Sends NO BGP withdrawals
///   - Records reload_error in export_state
///
/// Reload success behavior:
///   - Computes delta = computeDelta(current, candidate)
///   - Applies withdrawals for removed, announcements for added
///   - Commits candidate as new current_exported_prefixes
///   - Sends no UPDATEs for unchanged prefixes
///
/// Non-established sessions are skipped without error.
/// Failed apply to a session is recorded but doesn't block other sessions.
///
/// Candidate ownership: On success, candidate is adopted into current_exported_prefixes.
/// On failure, candidate is freed (unless it's the same as current_exported_prefixes).
///
/// Returns ReloadApplyResult with full diagnostics.
pub fn reloadAndApply(
    export_state: *ExportState,
    advertised_prefix_files_raw: []const u8,
    sess: *session.Session,
) ReloadApplyResult {
    // Initialize last_good state for reload transaction
    var last_good = prefix_watch.LastGoodState{
        .prefixes = export_state.current_exported_prefixes,
        .last_error = export_state.last_reload_error,
        .has_value = export_state.last_reload_success,
    };

    // Perform reload (reload-as-transaction)
    const reload_result = prefix_watch_reload.reloadPrefixes(
        advertised_prefix_files_raw,
        export_state.allocator.?,
        &last_good,
    );

    if (!reload_result.success) {
        // Reload failed - preserve current state, no withdrawals
        export_state.setReloadError(reload_result.error_message orelse "unknown reload error");
        export_state.last_reload_success = false;
        // last_good.prefixes still points to current_exported_prefixes (or previous last-good)
        // Don't modify current_exported_prefixes on failure

        return ReloadApplyResult{
            .reload_success = false,
            .reload_error = export_state.last_reload_error,
            .current_prefix_count = export_state.current_exported_prefixes.len,
            .delta_added_count = 0,
            .delta_removed_count = 0,
            .delta_unchanged_count = 0,
            .withdrawals_sent = 0,
            .announcements_sent = 0,
            .apply_error = null,
        };
    }

    // Reload succeeded - candidate is in last_good.prefixes
    const candidate = last_good.prefixes;
    // Track whether candidate was adopted to handle cleanup
    var candidate_adopted = false;

    // Deferred cleanup: free candidate if not adopted
    defer {
        if (!candidate_adopted and candidate.len > 0) {
            // Only free if candidate is a different allocation from current_exported_prefixes
            // (the same pointer means it points to current, which shouldn't be freed here)
            if (candidate.ptr != export_state.current_exported_prefixes.ptr) {
                export_state.allocator.?.free(candidate);
            }
        }
    }

    // Compute delta against current
    const delta = export_delta.computeDelta(export_state.allocator.?, export_state.current_exported_prefixes, candidate) catch |err| {
        // Delta computation failed - candidate will be freed by defer
        export_state.setReloadError(@errorName(err));
        export_state.last_reload_success = false;
        return ReloadApplyResult{
            .reload_success = false,
            .reload_error = export_state.last_reload_error,
            .current_prefix_count = export_state.current_exported_prefixes.len,
            .delta_added_count = 0,
            .delta_removed_count = 0,
            .delta_unchanged_count = 0,
            .withdrawals_sent = 0,
            .announcements_sent = 0,
            .apply_error = null,
        };
    };
    defer {
        export_state.allocator.?.free(delta.added);
        export_state.allocator.?.free(delta.removed);
    }

    // Store delta counts for status diagnostics
    export_state.last_delta_added_count = delta.added.len;
    export_state.last_delta_removed_count = delta.removed.len;
    export_state.last_delta_unchanged_count = delta.unchanged_count;

    // Apply delta to session (best-effort, session may not be established)
    export_state.clearApplyError();
    var apply_result: session_delta.DeltaApplyResult = undefined;

    if (delta.added.len == 0 and delta.removed.len == 0) {
        // No delta - no UPDATEs needed
        apply_result = session_delta.DeltaApplyResult{
            .withdrawals_sent = 0,
            .announcements_sent = 0,
            .withdrawn_prefixes = 0,
            .announced_prefixes = 0,
        };
    } else {
        // Apply delta to session
        apply_result = session_delta.applyDelta(sess, delta.removed, delta.added) catch |err| {
            export_state.setApplyError(@errorName(err));
            // Don't commit on apply failure - candidate will be freed by defer
            return ReloadApplyResult{
                .reload_success = true,
                .reload_error = null,
                .current_prefix_count = export_state.current_exported_prefixes.len,
                .delta_added_count = delta.added.len,
                .delta_removed_count = delta.removed.len,
                .delta_unchanged_count = delta.unchanged_count,
                .withdrawals_sent = 0,
                .announcements_sent = 0,
                .apply_error = export_state.last_apply_error,
            };
        };
    }

    // Commit candidate as new current_exported_prefixes
    // Free old prefixes first, then replace with candidate
    export_state.freeCurrentPrefixes();
    export_state.current_exported_prefixes = candidate;
    candidate_adopted = true; // Prevent defer from freeing
    export_state.clearReloadError();
    export_state.last_reload_success = true;

    return ReloadApplyResult{
        .reload_success = true,
        .reload_error = null,
        .current_prefix_count = export_state.current_exported_prefixes.len,
        .delta_added_count = delta.added.len,
        .delta_removed_count = delta.removed.len,
        .delta_unchanged_count = delta.unchanged_count,
        .withdrawals_sent = apply_result.withdrawals_sent,
        .announcements_sent = apply_result.announcements_sent,
        .apply_error = null,
    };
}

/// Initialize current_exported_prefixes from an initial prefix set.
/// Call this once at bundle initialization time.
pub fn initExportedPrefixes(
    export_state: *ExportState,
    initial_prefixes: []types.Ipv4Prefix,
) void {
    if (initial_prefixes.len > 0) {
        export_state.current_exported_prefixes = export_state.allocator.?.alloc(
            types.Ipv4Prefix,
            initial_prefixes.len,
        ) catch {
            export_state.current_exported_prefixes = &.{};
            return;
        };
        @memcpy(export_state.current_exported_prefixes, initial_prefixes);
    }
    export_state.last_reload_success = true;
}

// ============================================================================
// Watch + Apply Integration
// ============================================================================

/// Integration function that polls the watcher and applies reload if events occurred.
///
/// This is the main entry point for the runtime loop to integrate watched prefix
/// reloads with live BGP delta application.
///
/// Returns ReloadApplyResult with full diagnostics (may indicate no reload was needed).
///
/// Usage in runtime loop:
///   const result = export_reload_apply.watchAndApply(
///       &bundle.export_state,
///       bundle.bgp_config.advertised_prefix_files_raw,
///       &bundle.sess,
///       watcher,
///       debouncer,
///       now_ms,
///   );
pub fn watchAndApply(
    export_state: *ExportState,
    advertised_prefix_files_raw: []const u8,
    sess: *session.Session,
    watcher: *prefix_watch.Watcher,
    debouncer: *prefix_watch.Debouncer,
    now_ms: u64,
) ReloadApplyResult {
    // Check for watcher events
    const events = watcher.poll() catch {
        // Watcher poll failed - don't trigger reload, just return current state
        return ReloadApplyResult{
            .reload_success = export_state.last_reload_success,
            .reload_error = export_state.last_reload_error,
            .current_prefix_count = export_state.exportedCount(),
            .delta_added_count = export_state.last_delta_added_count,
            .delta_removed_count = export_state.last_delta_removed_count,
            .delta_unchanged_count = export_state.last_delta_unchanged_count,
            .withdrawals_sent = 0,
            .announcements_sent = 0,
            .apply_error = export_state.last_apply_error,
        };
    };

    // Check if any events warrant a reload.
    // Even when no new events arrive, we need to check if a pending debounce
    // window has elapsed (for the case where events occurred previously but
    // the debounce window just completed).
    if (events == null) {
        // No new events - but check if pending debounce should fire
        if (debouncer.isPending() and debouncer.shouldFire(now_ms)) {
            // Debounce window elapsed - cancel pending state and perform reload
            debouncer.cancel();
            return reloadAndApply(export_state, advertised_prefix_files_raw, sess);
        }
        // No events and no pending reload - return current state
        return ReloadApplyResult{
            .reload_success = export_state.last_reload_success,
            .reload_error = export_state.last_reload_error,
            .current_prefix_count = export_state.exportedCount(),
            .delta_added_count = export_state.last_delta_added_count,
            .delta_removed_count = export_state.last_delta_removed_count,
            .delta_unchanged_count = export_state.last_delta_unchanged_count,
            .withdrawals_sent = 0,
            .announcements_sent = 0,
            .apply_error = export_state.last_apply_error,
        };
    }

    // Schedule reload via debouncer
    _ = debouncer.schedule(now_ms);

    // Check if debounce window has elapsed
    if (!debouncer.shouldFire(now_ms)) {
        return ReloadApplyResult{
            .reload_success = export_state.last_reload_success,
            .reload_error = export_state.last_reload_error,
            .current_prefix_count = export_state.exportedCount(),
            .delta_added_count = export_state.last_delta_added_count,
            .delta_removed_count = export_state.last_delta_removed_count,
            .delta_unchanged_count = export_state.last_delta_unchanged_count,
            .withdrawals_sent = 0,
            .announcements_sent = 0,
            .apply_error = export_state.last_apply_error,
        };
    }

    // Debounce window elapsed - cancel pending state and perform reload
    debouncer.cancel();
    return reloadAndApply(export_state, advertised_prefix_files_raw, sess);
}
