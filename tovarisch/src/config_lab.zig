// config_lab.zig — Lab configuration for tovarisch
//
// Extracted from config.zig to keep file sizes under LLM-friendly limits.
// Contains LabConfig and its parsing logic only.

const std = @import("std");
const config_parse_helpers = @import("config_parse_helpers.zig");

/// Re-export errors and helpers from config_parse_helpers for use in this module.
pub const ConfigError = config_parse_helpers.ConfigError;
pub const requireNonEmpty = config_parse_helpers.requireNonEmpty;
pub const getString = config_parse_helpers.getString;
pub const parseBool = config_parse_helpers.parseBool;

/// LabConfig represents the [lab] section parsed from tovarisch.conf.
/// This enables the /lab/probe endpoint for KGB netns lab testing.
/// When lab_mode is false/absent, /lab/probe returns 404 (not a production control surface).
pub const LabConfig = struct {
    /// Whether lab mode is enabled. When false, /lab/probe returns 404.
    lab_mode: bool = false,
    /// Path to the failure file. When this file exists, /lab/probe returns 503.
    /// Required when lab_mode is true.
    lab_probe_failure_file: []const u8 = "",
    /// Enable native event emission (for idle staircase lab).
    native_events_enabled: bool = false,
    /// Path for native event timeline TSV output.
    native_events_path: []const u8 = "",
    /// Disable heartbeat thread when true.
    disable_heartbeat: bool = false,
    /// Disable WG periodic checks when true.
    disable_wg_checks: bool = false,
    /// Disable BGP maintenance when true.
    disable_bgp: bool = false,
    /// Disable BFD tick loop when true.
    disable_bfd: bool = false,
};

/// Parse the [lab] section from raw config into LabConfig.
/// This is called by config.parseLabConfig() after extracting the lab section.
pub fn parseLabConfigSection(section: anytype) ConfigError!LabConfig {
    var cfg = LabConfig{};
    if (getString(section, "lab_mode")) |value| {
        cfg.lab_mode = try parseBool(value);
    }

    // If lab_mode is enabled, failure_file is required
    if (cfg.lab_mode) {
        if (getString(section, "lab_probe_failure_file")) |value| {
            try requireNonEmpty(value);
            cfg.lab_probe_failure_file = value;
        } else return ConfigError.MissingKey;
    }

    // Native events toggle
    if (getString(section, "native_events_enabled")) |value| {
        cfg.native_events_enabled = try parseBool(value);
    }
    if (getString(section, "native_events_path")) |value| {
        cfg.native_events_path = value;
    }

    // Runtime subsystem toggles for idle staircase lab
    if (getString(section, "disable_heartbeat")) |value| {
        cfg.disable_heartbeat = try parseBool(value);
    }
    if (getString(section, "disable_wg_checks")) |value| {
        cfg.disable_wg_checks = try parseBool(value);
    }
    if (getString(section, "disable_bgp")) |value| {
        cfg.disable_bgp = try parseBool(value);
    }
    if (getString(section, "disable_bfd")) |value| {
        cfg.disable_bfd = try parseBool(value);
    }

    return cfg;
}

// --- LabConfig tests ---

test "parseLabConfig absent section returns defaults" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    const section = raw.get("lab") orelse return;
    const cfg = try parseLabConfigSection(section);
    try std.testing.expect(!cfg.lab_mode);
    try std.testing.expect(cfg.lab_probe_failure_file.len == 0);
}

test "parseLabConfig lab_mode=false" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var lab_section = std.StringArrayHashMapUnmanaged([]const u8){};
    try lab_section.put(std.heap.page_allocator, "lab_mode", "false");
    try raw.put(std.heap.page_allocator, "lab", lab_section);
    const cfg = try parseLabConfigSection(raw.get("lab").?);
    try std.testing.expect(!cfg.lab_mode);
}

test "parseLabConfig lab_mode=true requires lab_probe_failure_file" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var lab_section = std.StringArrayHashMapUnmanaged([]const u8){};
    try lab_section.put(std.heap.page_allocator, "lab_mode", "true");
    try raw.put(std.heap.page_allocator, "lab", lab_section);
    try std.testing.expectError(ConfigError.MissingKey, parseLabConfigSection(raw.get("lab").?));
}

test "parseLabConfig lab_mode=true with file path" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var lab_section = std.StringArrayHashMapUnmanaged([]const u8){};
    try lab_section.put(std.heap.page_allocator, "lab_mode", "true");
    try lab_section.put(std.heap.page_allocator, "lab_probe_failure_file", "/tmp/probe-failing");
    try raw.put(std.heap.page_allocator, "lab", lab_section);
    const cfg = try parseLabConfigSection(raw.get("lab").?);
    try std.testing.expect(cfg.lab_mode);
    try std.testing.expectEqualStrings("/tmp/probe-failing", cfg.lab_probe_failure_file);
}

test "parseLabConfig lab_mode=true rejects empty failure file" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var lab_section = std.StringArrayHashMapUnmanaged([]const u8){};
    try lab_section.put(std.heap.page_allocator, "lab_mode", "true");
    try lab_section.put(std.heap.page_allocator, "lab_probe_failure_file", "   ");
    try raw.put(std.heap.page_allocator, "lab", lab_section);
    try std.testing.expectError(ConfigError.EmptyValue, parseLabConfigSection(raw.get("lab").?));
}
