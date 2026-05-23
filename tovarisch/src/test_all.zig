// test_all.zig — Aggregate test root for tovarisch
//
// This file forces Zig's test discovery to find tests in all modules.
// Use this as the root_source_file for test steps to ensure every module's
// tests are included in the test binary.
//
// Pattern: import every module with tests, then refAllDecls to force linking.
const std = @import("std");

// Import all source modules to ensure they are compiled and their tests discovered
const _cli = @import("cli.zig");
const _status = @import("status.zig");
const _net_private_ip = @import("net/private_ip.zig");
const _net_linux_stats = @import("net/linux_stats.zig");
const _net_linux_stats_tests = @import("net/linux_stats_tests.zig");
const _net_linux_interfaces = @import("net/linux_interfaces.zig");
const _net_linux_interfaces_tests = @import("net/linux_interfaces_tests.zig");
const _net_linux_interface_stats = @import("net/linux_interface_stats.zig");
const _net_linux_interface_stats_tests = @import("net/linux_interface_stats_tests.zig");
const _http_response = @import("http/response.zig");
const _http_routes = @import("http/routes.zig");
const _http_server = @import("http/server.zig");
const _runtime_telemetry = @import("runtime/telemetry.zig");

// Force test discovery for all imported modules
// This ensures the test binary actually runs the tests from these modules
test {
    std.testing.refAllDecls(@import("net/private_ip.zig"));
}

test {
    std.testing.refAllDecls(@import("net/linux_stats.zig"));
}

test {
    std.testing.refAllDecls(@import("net/linux_stats_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("net/linux_interfaces.zig"));
}

test {
    std.testing.refAllDecls(@import("net/linux_interfaces_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("net/linux_interface_stats.zig"));
}

test {
    std.testing.refAllDecls(@import("net/linux_interface_stats_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("cli.zig"));
}

test {
    std.testing.refAllDecls(@import("status.zig"));
}

test {
    std.testing.refAllDecls(@import("http/response.zig"));
}

test {
    std.testing.refAllDecls(@import("http/routes.zig"));
}

test {
    std.testing.refAllDecls(@import("http/server.zig"));
}

test {
    std.testing.refAllDecls(@import("runtime/telemetry.zig"));
}
