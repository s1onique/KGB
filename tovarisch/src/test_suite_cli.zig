// test_suite_cli.zig — CLI test suite
//
// Tests: CLI argument parsing, config loading, serve commands.
// No real sockets - tests use RawConfig with in-memory data.

const std = @import("std");

// CLI modules
const _cli = @import("cli.zig");
const _cli_serve_config_tests = @import("cli_serve_config_tests.zig");
const _bfd_serve_config_tests = @import("bfd_serve_config_tests.zig");
const _status_bgp_tests = @import("status_bgp_tests.zig");

// WireGuard config modules
const _wg_generate = @import("wg/generate.zig");
const _cli_wg_args = @import("cli/wg_args.zig");
const _cli_bfd_serve = @import("cli/bfd_serve.zig");
const _cli_bgp_serve = @import("cli/bgp_serve.zig");

// Force test discovery
test { std.testing.refAllDecls(@import("cli.zig")); }
test { std.testing.refAllDecls(@import("cli_serve_config_tests.zig")); }
test { std.testing.refAllDecls(@import("bfd_serve_config_tests.zig")); }
test { std.testing.refAllDecls(@import("status_bgp_tests.zig")); }
test { std.testing.refAllDecls(@import("wg/generate.zig")); }
test { std.testing.refAllDecls(@import("cli/wg_args.zig")); }
test { std.testing.refAllDecls(@import("cli/bfd_serve.zig")); }
test { std.testing.refAllDecls(@import("cli/bgp_serve.zig")); }
