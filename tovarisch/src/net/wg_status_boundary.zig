// wg_status_boundary.zig — Native-owned WireGuard status boundary for tovarisch
//
// ACT: Native-owned WireGuard status boundary for tovarisch
//
// Phase 1 of 2:
//   Phase 1: Typed owned CLI backend (wg_status_boundary_cli.zig)
//   Phase 2: Native WireGuard generic-netlink backend (future work)
//
// This module provides the single, typed boundary for all WireGuard status
// observation in tovarisch. No raw `wg show` composition is allowed outside
// this module.
//
// Anti-NIH clause:
//   WireGuard kernel remains the boundary; tovarisch owns observation semantics.
//   We do NOT implement WireGuard protocol from scratch. We observe what the
//   kernel exposes via the wg userspace tool.

const std = @import("std");

// ============================================================================
// Backend Kind
// ============================================================================

/// Identifies which backend implementation is in use.
pub const BackendKind = enum {
    /// CLI backend using `wg show` command.
    cli,
    /// Fake backend for deterministic unit tests.
    fake,
    /// Native generic-netlink backend (Phase 2, not yet implemented).
    generic_netlink,
};

/// Human-readable backend name for diagnostics.
pub fn backendKindName(kind: BackendKind) []const u8 {
    return switch (kind) {
        .cli => "cli",
        .fake => "fake",
        .generic_netlink => "generic-netlink",
    };
}

// ============================================================================
// Structured Error Types
// ============================================================================

/// WireGuard status collection errors.
/// These are structured errors that map to human-readable status check details.
pub const StatusError = error{
    /// The `wg` command is not available on this system.
    backend_missing,
    /// Permission denied when running `wg` command.
    permission_denied,
    /// The specified WireGuard interface does not exist.
    interface_missing,
    /// The `wg` command exited with a non-zero status.
    command_failed,
    /// Parser returned an error (malformed output).
    malformed_output,
    /// Command execution exceeded timeout.
    timeout,
    /// This platform does not support WireGuard CLI.
    unsupported_platform,
    /// Memory allocation failed.
    out_of_memory,
};

/// Converts StatusError to human-readable detail string for status checks.
/// These strings are static (no allocation required).
pub fn statusErrorDetail(err: StatusError) []const u8 {
    return switch (err) {
        error.backend_missing => "wg command not available",
        error.permission_denied => "wg permission denied",
        error.interface_missing => "wg interface not found",
        error.command_failed => "wg command failed",
        error.malformed_output => "wg output malformed",
        error.timeout => "wg command timeout",
        error.unsupported_platform => "wg not supported on this platform",
        error.out_of_memory => "wg check out of memory",
    };
}

/// Structured diagnostic text from stderr for debugging.
/// Not exposed in status JSON to avoid log pollution.
pub const DiagnosticText = struct {
    /// Captured stderr content (may be empty).
    stderr: []const u8,
    /// Whether the output was truncated.
    truncated: bool,

    /// Returns true if there is any diagnostic content.
    pub fn hasContent(self: DiagnosticText) bool {
        return self.stderr.len > 0;
    }

    /// Returns true if this diagnostic indicates an error condition.
    pub fn indicatesError(self: DiagnosticText) bool {
        return self.stderr.len > 0;
    }
};

// ============================================================================
// Typed Status Model
// ============================================================================

/// WireGuard interface status data.
/// This is the canonical typed representation of WireGuard status in tovarisch.
///
/// Privacy-aligned fields (exposed):
///   - interface name
///   - peer count
///   - latest handshake epoch (Unix timestamp)
///   - transfer rx/tx bytes
///
/// Explicitly excluded (not exposed):
///   - public/private keys
///   - endpoints
///   - allowed IPs
///   - preshared keys
pub const WireGuardStatus = struct {
    /// WireGuard interface name (e.g., "wg0", "wg-kgb0").
    interface: []const u8,
    /// Number of configured peers.
    peer_count: u32,
    /// Unix epoch timestamp of most recent handshake across all peers.
    /// Null if no handshakes have occurred (never-handshaked peer).
    /// Per wg(8) dump format: this is a Unix timestamp, not seconds ago.
    latest_handshake_epoch_sec: ?u64,
    /// Total bytes received via this interface.
    rx_bytes: u64,
    /// Total bytes transmitted via this interface.
    tx_bytes: u64,
    /// Listen port if available (null if not exposed by wg show).
    listen_port: ?u16,
    /// Public key (redacted, first/last 4 chars only).
    /// Empty string if not available.
    public_key_redacted: []const u8,

    /// Creates a "no interface" status indicating WireGuard is not configured.
    pub fn noInterface() WireGuardStatus {
        return WireGuardStatus{
            .interface = "",
            .peer_count = 0,
            .latest_handshake_epoch_sec = null,
            .rx_bytes = 0,
            .tx_bytes = 0,
            .listen_port = null,
            .public_key_redacted = "",
        };
    }

    /// Returns true if a WireGuard interface exists (even with no peers).
    pub fn hasInterface(self: WireGuardStatus) bool {
        return self.interface.len > 0;
    }

    /// Returns true if at least one peer is configured.
    pub fn hasPeers(self: WireGuardStatus) bool {
        return self.peer_count > 0;
    }

    /// Returns true if at least one handshake has occurred.
    /// Note: epoch_sec is null when no handshake ever occurred.
    pub fn hasHandshake(self: WireGuardStatus) bool {
        return self.latest_handshake_epoch_sec != null;
    }

    /// Derives a simple health status from this WireGuard status.
    pub fn deriveHealth(self: WireGuardStatus) WireGuardHealth {
        if (!self.hasInterface()) return .unknown;
        if (!self.hasPeers()) return .no_peers;
        if (!self.hasHandshake()) return .no_handshake;
        return .ok;
    }
};

/// Simple health status for WireGuard.
pub const WireGuardHealth = enum {
    /// All good: interface exists, has peers, has handshake.
    ok,
    /// No WireGuard interface configured.
    unknown,
    /// Interface exists but no peers configured.
    no_peers,
    /// Peers configured but no handshake yet.
    no_handshake,
};

/// Result of WireGuard status collection.
pub const WireGuardStatusResult = struct {
    /// The collected status data.
    status: WireGuardStatus,
    /// Backend that collected this status.
    backend: BackendKind,
    /// Diagnostic text captured during collection (for debugging).
    diagnostic: DiagnosticText,

    /// Creates a successful result.
    pub fn ok(s: WireGuardStatus, backend: BackendKind) WireGuardStatusResult {
        return WireGuardStatusResult{
            .status = s,
            .backend = backend,
            .diagnostic = .{ .stderr = "", .truncated = false },
        };
    }

    /// Creates a result with diagnostic text.
    pub fn withDiagnostic(s: WireGuardStatus, backend: BackendKind, stderr: []const u8) WireGuardStatusResult {
        return WireGuardStatusResult{
            .status = s,
            .backend = backend,
            .diagnostic = .{ .stderr = stderr, .truncated = stderr.len > 1024 },
        };
    }
};

// ============================================================================
// Backend Trait (interface pattern via struct with function pointers)
// ============================================================================

/// WireGuard status backend trait.
/// This is the single entry point for all WireGuard status observation.
pub const WireGuardStatusBackend = struct {
    /// Function pointer for wireguardStatus implementation.
    wireguardStatusFn: *const fn (allocator: std.mem.Allocator) StatusError!WireGuardStatusResult,
    /// Function pointer for backendKind implementation.
    backendKindFn: *const fn () BackendKind,

    /// Collect WireGuard status using this backend.
    pub fn wireguardStatus(self: WireGuardStatusBackend, allocator: std.mem.Allocator) StatusError!WireGuardStatusResult {
        return self.wireguardStatusFn(allocator);
    }

    /// Get the kind of this backend.
    pub fn backendKind(self: WireGuardStatusBackend) BackendKind {
        return self.backendKindFn();
    }
};

// ============================================================================
// Status Error Conversion Helpers
// ============================================================================

/// Converts StatusError to a CheckStatus for status.zig integration.
pub fn toCheckStatus(_err: StatusError) status.CheckStatus {
    _err catch {};
    return .warn;
}

/// Converts WireGuardStatus to a Check for status.zig integration.
pub fn toCheck(wg_status: WireGuardStatus, detail_override: ?[]const u8) status.Check {
    const health = wg_status.deriveHealth();
    const detail = detail_override orelse switch (health) {
        .ok => "wireguard peers healthy",
        .unknown => "wg0",
        .no_peers => "no peers detected",
        .no_handshake => "no handshake yet",
    };

    return status.Check{
        .name = "wg_peers",
        .status = switch (health) {
            .ok => .ok,
            else => .warn,
        },
        .detail = detail,
    };
}

/// Re-exports from status.zig for this module's use.
/// Public for status.zig integration via status_checks.zig.
pub const status = struct {
    pub const CheckStatus = enum { ok, warn, @"error", unknown };
    pub const Check = struct {
        name: []const u8,
        status: CheckStatus,
        detail: []const u8,
    };
};

// ============================================================================
// Machine-Readable Dump Parser (Phase 1 CLI format)
// ============================================================================

/// Dump format field indices for `wg show dump` output per wg(8).
/// Interface line: private_key, public_key, listen_port, fwmark
/// Peer lines: public_key, preshared_key, endpoint, allowed_ips,
///              latest_handshake, transfer_rx, transfer_tx, persistent_keepalive
const DUMP_PEER_FIELDS: usize = 8;

/// Parse `wg show dump` output into WireGuardStatus.
///
/// Machine-readable tab-separated format per wg(8):
/// Interface line fields:
///   0: private key (redacted)
///   1: public key (redacted)
///   2: listen port
///   3: fwmark
///
/// Peer lines (8 fields each):
///   0: peer public key (redacted)
///   1: preshared key (redacted)
///   2: endpoint (redacted)
///   3: allowed IPs (redacted)
///   4: latest handshake (Unix epoch timestamp, 0 = never)
///   5: transfer rx bytes
///   6: transfer tx bytes
///   7: persistent keepalive (seconds, or "off")
///
/// Note: Per wg(8) dump format, field 4 is a Unix timestamp, not seconds ago.
pub fn parseWgDumpOutput(input: []const u8) !WireGuardStatus {
    var it = std.mem.splitScalar(u8, input, '\n');

    var interface_name: []const u8 = "";
    var listen_port: ?u16 = null;
    var peer_count: u32 = 0;
    var latest_handshake_epoch: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    var found_interface_line = false;

    // First line: interface info (4 fields)
    if (it.next()) |first_line| {
        if (first_line.len == 0) {
            // Empty output = no interface
            return WireGuardStatus.noInterface();
        }

        // Per wg(8) dump format: first line is interface info, even with no peers
        found_interface_line = true;

        // Parse interface line fields
        var field_it = std.mem.splitScalar(u8, first_line, '\t');
        _ = field_it.next(); // private key (ignored for privacy)
        _ = field_it.next(); // public key (ignored for privacy)
        const port_str = field_it.next() orelse "";
        if (port_str.len > 0) {
            listen_port = std.fmt.parseInt(u16, port_str, 10) catch null;
        }
        _ = field_it.next(); // fwmark

        // Interface name is implied from the dump context
        // When using `wg show dump`, the output represents a single interface
        // We use "wg0" as the implied default since we can't determine
        // the actual interface name from the dump output format
        interface_name = "wg0";
    } else {
        return WireGuardStatus.noInterface();
    }

    // Parse peer lines (8 fields each)
    while (it.next()) |line| {
        if (line.len == 0) continue;

        // Count fields to distinguish peer lines (8) from shorter lines
        var field_count: usize = 0;
        var count_it = std.mem.splitScalar(u8, line, '\t');
        while (count_it.next()) |_| {
            field_count += 1;
        }

        if (field_count < DUMP_PEER_FIELDS) {
            continue;
        }

        // Parse actual values using correct field order per wg(8)
        var field_it = std.mem.splitScalar(u8, line, '\t');
        _ = field_it.next(); // 0: peer public key (redacted)
        _ = field_it.next(); // 1: preshared key (redacted)
        _ = field_it.next(); // 2: endpoint (redacted)
        _ = field_it.next(); // 3: allowed IPs (redacted)

        // 4: latest handshake (Unix epoch timestamp, 0 = never)
        const handshake_str = field_it.next() orelse "0";
        const handshake = std.fmt.parseInt(u64, handshake_str, 10) catch 0;

        // 5: transfer rx bytes
        const rx_str = field_it.next() orelse "0";
        const rx = std.fmt.parseInt(u64, rx_str, 10) catch 0;

        // 6: transfer tx bytes
        const tx_str = field_it.next() orelse "0";
        const tx = std.fmt.parseInt(u64, tx_str, 10) catch 0;

        _ = field_it.next(); // 7: persistent keepalive

        peer_count += 1;
        rx_bytes += rx;
        tx_bytes += tx;

        // Track maximum handshake epoch (most recent)
        // Per wg(8): field 4 is a Unix epoch timestamp, 0 means never
        if (handshake > 0) {
            if (latest_handshake_epoch == null or handshake > latest_handshake_epoch.?) {
                latest_handshake_epoch = handshake;
            }
        }
    }

    // Interface exists if we parsed the interface info line
    // (even with zero peers, the interface line is present in dump output)
    const has_interface = found_interface_line;

    return WireGuardStatus{
        .interface = if (has_interface) interface_name else "",
        .peer_count = peer_count,
        .latest_handshake_epoch_sec = latest_handshake_epoch,
        .rx_bytes = rx_bytes,
        .tx_bytes = tx_bytes,
        .listen_port = listen_port,
        .public_key_redacted = "",
    };
}
