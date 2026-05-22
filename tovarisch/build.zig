const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    const exe = b.addExecutable(.{
        .name = "tovarisch",
        .root_module = b.createModule(.{
            .root_source_file = b.path("src/main.zig"),
            .target = target,
            .optimize = optimize,
            // Explicitly require libc linking for HTTP server socket support.
            //
            // The HTTP server (src/http/server.zig) uses std.c.socket, std.c.bind,
            // std.c.listen, std.c.accept, and std.c.write for cross-platform networking.
            //
            // Decision rationale:
            // - Zig 0.16's std.posix namespace does NOT expose socket/bind/listen/accept
            //   functions directly; they remain in std.c or OS-specific modules.
            // - Using std.c.* with cross-platform targets (e.g., arm-linux-musleabihf)
            //   requires explicit libc linking to satisfy the compiler.
            // - The leaf-service doctrine prefers avoiding libc where practical, but
            //   HTTP socket functionality is a documented exception for now.
            // - Alternative: Use OS-specific modules (std.os.linux.socket, etc.) which
            //   use direct syscalls and don't require libc. This would require significant
            //   refactoring of the HTTP server abstraction layer.
            // - TODO: Investigate std.os.linux.socket-based alternative for pure syscall approach
            .link_libc = true,
        }),
    });

    b.installArtifact(exe);

    const run_cmd = b.addRunArtifact(exe);
    run_cmd.step.dependOn(b.getInstallStep());

    if (b.args) |args| {
        run_cmd.addArgs(args);
    }

    const run_step = b.step("run", "Run tovarisch");
    run_step.dependOn(&run_cmd.step);

    // Test artifact for coverage — test-bin step produces:
    // zig-out/bin/tovarisch-test (not zig-out/test/)
    //
    // Uses test_all.zig as root to ensure all module tests are discovered.
    // test_all.zig imports cli.zig and status.zig and calls refAllDecls
    // to force Zig to link and run their tests.
    const unit_tests = b.addTest(.{
        .root_module = b.createModule(.{
            .root_source_file = b.path("src/test_all.zig"),
            .target = target,
            .optimize = optimize,
        }),
    });

    const run_unit_tests = b.addRunArtifact(unit_tests);

    const test_step = b.step("test", "Run unit tests");
    test_step.dependOn(&run_unit_tests.step);

    // test-bin step: builds test artifact and prepares it for coverage
    // The test executable is compiled and available via unit_tests
    const test_bin_step = b.step("test-bin", "Build test binary for kcov coverage");
    test_bin_step.dependOn(&unit_tests.step);

    // Install test binary to zig-out/bin/tovarisch-test
    const install_test = b.addInstallArtifact(unit_tests, .{
        .dest_sub_path = "tovarisch-test",
    });
    test_bin_step.dependOn(&install_test.step);
}
