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

    // Derive build metadata from VERSION file
    const base_version = readVersionFile(b);
    const commit = getGitCommit(b);

    // Build the full version string: base_version+commit
    const version_len = base_version.len + 1 + commit.len;
    const version = allocator.alloc(u8, version_len) catch @panic("OOM");
    @memcpy(version[0..base_version.len], base_version);
    version[base_version.len] = '+';
    @memcpy(version[base_version.len + 1 ..], commit);

    // Inject build options into the root module
    const build_options = b.addOptions();
    build_options.addOption([]const u8, "base_version", base_version);
    build_options.addOption([]const u8, "commit", commit);
    build_options.addOption([]const u8, "version", version);

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
}
