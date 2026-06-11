const std = @import("std");

/// Returns the git commit short SHA or "unknown".
/// Precedence: 1. GIT_COMMIT env var (shortened to 7 chars), 2. git rev-parse, 3. "unknown".
fn getGitCommit(b: *std.Build) []const u8 {
    // Check GIT_COMMIT env var first
    const env_map = b.graph.environ_map;
    if (env_map.get("GIT_COMMIT")) |git_commit| {
        if (git_commit.len >= 7) {
            return git_commit[0..7];
        } else if (git_commit.len > 0) {
            return git_commit;
        }
    }
    // Fall back to git rev-parse
    const argv = &.{ "git", "rev-parse", "--short=7", "HEAD" };
    var exit_code: u8 = undefined;
    const result = b.runAllowFail(argv, &exit_code, .inherit) catch return "unknown";
    const trimmed = std.mem.trim(u8, result, " \t\r\n");
    if (trimmed.len >= 7 and !std.mem.eql(u8, trimmed, "HEAD")) {
        return trimmed[0..7];
    }
    return "unknown";
}

/// Returns true if the Git working tree is dirty.
/// Precedence: 1. BUILD_DIRTY env var, 2. git status --porcelain, 3. false.
fn getGitDirty(b: *std.Build) bool {
    const env_map = b.graph.environ_map;

    // Check BUILD_DIRTY env var first
    if (env_map.get("BUILD_DIRTY")) |value| {
        const trimmed = std.mem.trim(u8, value, " \t\r\n");
        if (std.ascii.eqlIgnoreCase(trimmed, "1") or std.ascii.eqlIgnoreCase(trimmed, "true")) {
            return true;
        }
        // "0", "false", or any other value means not dirty
        return false;
    }

    // Fall back to git status --porcelain
    const argv = &.{ "git", "status", "--porcelain" };
    var exit_code: u8 = undefined;
    const result = b.runAllowFail(argv, &exit_code, .inherit) catch return false;
    const trimmed = std.mem.trim(u8, result, " \t\r\n");
    return trimmed.len > 0;
}

/// Reads VERSION file from tovarisch/ directory.
/// Precedence: 1. VERSION env var, 2. tovarisch/VERSION file, 3. "0.0.0".
fn readVersionFile(b: *std.Build) []const u8 {
    const allocator = b.allocator;

    // Check VERSION env var first
    const env_map = b.graph.environ_map;
    if (env_map.get("VERSION")) |version| {
        const trimmed = std.mem.trim(u8, version, " \t\r\n");
        if (trimmed.len > 0) {
            return allocator.dupe(u8, trimmed) catch return "0.0.0";
        }
    }

    // Fall back to VERSION file (build-root relative)
    const io = b.graph.io;
    const dir = std.Io.Dir.cwd();
    var file = dir.openFile(io, "VERSION", .{}) catch return "0.0.0";
    defer file.close(io);

    var file_reader = std.Io.File.reader(file, io, &.{});
    const content = file_reader.interface.allocRemaining(allocator, .unlimited) catch return "0.0.0";
    defer allocator.free(content);

    const trimmed = std.mem.trim(u8, content, " \t\r\n");
    if (trimmed.len > 0) {
        return allocator.dupe(u8, trimmed) catch return "0.0.0";
    }
    return "0.0.0";
}

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});
    const allocator = b.allocator;

    // Derive build metadata from VERSION file and Git state
    const base_version = readVersionFile(b);
    const commit = getGitCommit(b);
    const dirty = getGitDirty(b);

    // Build the full version string: base_version+commit or base_version+commit.dirty
    const dirty_suffix_len: usize = if (dirty) 6 else 0; // ".dirty" = 6 chars
    const version_len = base_version.len + 1 + commit.len + dirty_suffix_len;
    const version = allocator.alloc(u8, version_len) catch @panic("OOM");
    @memcpy(version[0..base_version.len], base_version);
    version[base_version.len] = '+';
    @memcpy(version[base_version.len + 1 .. base_version.len + 1 + commit.len], commit);
    if (dirty) {
        @memcpy(version[base_version.len + 1 + commit.len ..], ".dirty");
    }

    // Inject build options into the root module
    const build_options = b.addOptions();
    build_options.addOption([]const u8, "base_version", base_version);
    build_options.addOption([]const u8, "commit", commit);
    build_options.addOption([]const u8, "version", version);
    build_options.addOption(bool, "dirty", dirty);

    const exe = b.addExecutable(.{
        .name = "tovarisch",
        .root_module = b.createModule(.{
            .root_source_file = b.path("src/main.zig"),
            .target = target,
            .optimize = optimize,
            .link_libc = true,
        }),
    });
    exe.root_module.addOptions("build_options", build_options);

    b.installArtifact(exe);

    const run_cmd = b.addRunArtifact(exe);
    run_cmd.step.dependOn(b.getInstallStep());

    if (b.args) |args| {
        run_cmd.addArgs(args);
    }

    const run_step = b.step("run", "Run tovarisch");
    run_step.dependOn(&run_cmd.step);

    const unit_tests = b.addTest(.{
        .root_module = b.createModule(.{
            .root_source_file = b.path("src/test_all.zig"),
            .target = target,
            .optimize = optimize,
            .link_libc = true,
        }),
    });
    unit_tests.root_module.addOptions("build_options", build_options);

    const run_unit_tests = b.addRunArtifact(unit_tests);

    const test_step = b.step("test", "Run unit tests");
    test_step.dependOn(&run_unit_tests.step);

    const test_bin_step = b.step("test-bin", "Build test binary for kcov coverage");
    test_bin_step.dependOn(&unit_tests.step);

    const install_test = b.addInstallArtifact(unit_tests, .{
        .dest_sub_path = "tovarisch-test",
    });
    test_bin_step.dependOn(&install_test.step);

    // ============================================================================
    // Split Test Suites for CI Isolation
    //
    // Split the monolithic test binary into named suites so Linux CI can identify
    // which suite hangs. Each suite has explicit timeout in the workflow.
    // ============================================================================

    // Base tests: core modules, config, metrics, status, net utilities
    const base_tests = b.addTest(.{
        .root_module = b.createModule(.{
            .root_source_file = b.path("src/test_suite_base.zig"),
            .target = target,
            .optimize = optimize,
            .link_libc = true,
        }),
    });
    base_tests.root_module.addOptions("build_options", build_options);

    const test_base_step = b.step("test-base", "Run base tests (core, config, metrics, status, net)");
    test_base_step.dependOn(&b.addRunArtifact(base_tests).step);

    // BFD tests: BFD protocol state machine and runtime
    const bfd_tests = b.addTest(.{
        .root_module = b.createModule(.{
            .root_source_file = b.path("src/test_suite_bfd.zig"),
            .target = target,
            .optimize = optimize,
            .link_libc = true,
        }),
    });
    bfd_tests.root_module.addOptions("build_options", build_options);

    const test_bfd_step = b.step("test-bfd", "Run BFD tests (protocol, session, runtime)");
    test_bfd_step.dependOn(&b.addRunArtifact(bfd_tests).step);

    // BGP tests: BGP protocol state machine and TCP transport
    const bgp_tests = b.addTest(.{
        .root_module = b.createModule(.{
            .root_source_file = b.path("src/test_suite_bgp.zig"),
            .target = target,
            .optimize = optimize,
            .link_libc = true,
        }),
    });
    bgp_tests.root_module.addOptions("build_options", build_options);

    const test_bgp_step = b.step("test-bgp", "Run BGP tests (protocol, session, TCP transport)");
    test_bgp_step.dependOn(&b.addRunArtifact(bgp_tests).step);

    // BGP sub-suites for fine-grained CI isolation
    const bgp_protocol_tests = b.addTest(.{
        .root_module = b.createModule(.{
            .root_source_file = b.path("src/test_suite_bgp_protocol.zig"),
            .target = target,
            .optimize = optimize,
            .link_libc = true,
        }),
    });
    bgp_protocol_tests.root_module.addOptions("build_options", build_options);
    const test_bgp_protocol_step = b.step("test-bgp-protocol", "Run BGP protocol tests (types, message, validation)");
    test_bgp_protocol_step.dependOn(&b.addRunArtifact(bgp_protocol_tests).step);

    const bgp_session_tests = b.addTest(.{
        .root_module = b.createModule(.{
            .root_source_file = b.path("src/test_suite_bgp_session.zig"),
            .target = target,
            .optimize = optimize,
            .link_libc = true,
        }),
    });
    bgp_session_tests.root_module.addOptions("build_options", build_options);
    const test_bgp_session_step = b.step("test-bgp-session", "Run BGP session tests (state machine, handshake)");
    test_bgp_session_step.dependOn(&b.addRunArtifact(bgp_session_tests).step);

    const bgp_tcp_tests = b.addTest(.{
        .root_module = b.createModule(.{
            .root_source_file = b.path("src/test_suite_bgp_tcp.zig"),
            .target = target,
            .optimize = optimize,
            .link_libc = true,
        }),
    });
    bgp_tcp_tests.root_module.addOptions("build_options", build_options);
    const test_bgp_tcp_step = b.step("test-bgp-tcp", "Run BGP TCP transport tests (loopback sockets)");
    test_bgp_tcp_step.dependOn(&b.addRunArtifact(bgp_tcp_tests).step);

    const bgp_integration_tests = b.addTest(.{
        .root_module = b.createModule(.{
            .root_source_file = b.path("src/test_suite_bgp_integration.zig"),
            .target = target,
            .optimize = optimize,
            .link_libc = true,
        }),
    });
    bgp_integration_tests.root_module.addOptions("build_options", build_options);
    const test_bgp_integration_step = b.step("test-bgp-integration", "Run BGP integration tests (config, serve, status)");
    test_bgp_integration_step.dependOn(&b.addRunArtifact(bgp_integration_tests).step);

    const test_bgp_split_step = b.step("test-bgp-split", "Run all BGP sub-suite tests");
    test_bgp_split_step.dependOn(test_bgp_protocol_step);
    test_bgp_split_step.dependOn(test_bgp_session_step);
    test_bgp_split_step.dependOn(test_bgp_tcp_step);
    test_bgp_split_step.dependOn(test_bgp_integration_step);

    // HTTP tests: HTTP server, routes, serve context
    const http_tests = b.addTest(.{
        .root_module = b.createModule(.{
            .root_source_file = b.path("src/test_suite_http.zig"),
            .target = target,
            .optimize = optimize,
            .link_libc = true,
        }),
    });
    http_tests.root_module.addOptions("build_options", build_options);

    const test_http_step = b.step("test-http", "Run HTTP server tests");
    test_http_step.dependOn(&b.addRunArtifact(http_tests).step);

    // CLI tests: command parsing, config handling, serve commands
    const cli_tests = b.addTest(.{
        .root_module = b.createModule(.{
            .root_source_file = b.path("src/test_suite_cli.zig"),
            .target = target,
            .optimize = optimize,
            .link_libc = true,
        }),
    });
    cli_tests.root_module.addOptions("build_options", build_options);

    const test_cli_step = b.step("test-cli", "Run CLI tests");
    test_cli_step.dependOn(&b.addRunArtifact(cli_tests).step);

    // Combined split-test step for CI
    const test_split_step = b.step("test-split", "Run all split test suites");
    test_split_step.dependOn(test_base_step);
    test_split_step.dependOn(test_bfd_step);
    test_split_step.dependOn(test_bgp_step);
    test_split_step.dependOn(test_http_step);
    test_split_step.dependOn(test_cli_step);
}
