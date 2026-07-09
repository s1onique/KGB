// cli/daemon_command.zig — Daemon/serve command implementation
//
// Extracted from commands.zig to satisfy LLM-friendliness line limits.

const std = @import("std");
const cli_args = @import("args.zig");
const wg_args = @import("wg_args.zig");
const http = @import("../http/server.zig");
const logging = @import("../logging.zig");
const bfd_serve = @import("bfd_serve.zig");
const bfd_status = @import("../bfd/status.zig");
const bgp_serve = @import("bgp_serve.zig");
const config = @import("../config.zig");
const status = @import("../status.zig");
const net_diag_config = @import("net_diag_config.zig");
const command_model = @import("command_model.zig");
const startup_trace = @import("../startup_trace.zig");

/// Execute the `serve` command (daemon mode).
/// Returns ExitCode.ok on clean shutdown, .serve_error on errors.
///
/// NOTE: Systemd's "Started" line in journald indicates process launch, NOT
/// application readiness. For Type=simple services (default), systemd considers
/// startup complete when the process forks/execs. The canonical application
/// readiness event is `startup_ready`, emitted after the HTTP accept loop starts.
pub fn serveCommand(serve_args: []const []const u8, stdout: anytype, stderr: anytype) command_model.ExitCode {
    // Initialize startup tracer at process entry.
    // This timestamps all subsequent phases up to startup_ready.
    var tracer = startup_trace.StartupTracer.init();

    const parsed = cli_args.parseServeArgs(serve_args, stderr);

    switch (parsed) {
        .usage => return .usage,
        .ok => |serve_config| {
            // Copy http_config so we can modify it with config file values.
            // CLI-parsed values take precedence unless --listen was not explicit.
            var http_config = serve_config.http_config;

            // Phase: config_load — config file parsing and CLI override resolution
            {
                var guard = tracer.begin(.config_load);
                guard.emitStarted(stderr) catch {};
                defer { guard.finish(stderr) catch {}; }

                // Apply [server].listen from config file if present and no explicit --listen was given.
                // This wires the bug fix: TOML [server].listen -> Config.server.listen -> HTTP bind.
                if (!serve_config.explicit_listen and serve_config.config_path != null) {
                    read_config: {
                        var raw = wg_args.readConfig(serve_config.config_path.?, std.heap.page_allocator) catch break :read_config;
                        defer raw.deinit(std.heap.page_allocator);
                        const server_cfg = config.parseServerConfig(&raw);
                        if (server_cfg.listen) |listen_addr| {
                            if (command_model.parseListenAddr(listen_addr)) |listen_parsed| {
                                // Store owned copy of host string to avoid dangling pointer.
                                const host_copy = std.heap.page_allocator.dupe(u8, listen_parsed.host) catch break :read_config;
                                http_config.address = host_copy;
                                http_config.port = listen_parsed.port;
                            }
                        }
                    }
                }
            }

            // Phase: bfd_start — BFD configuration loading and loop startup
            var bfd_bundle: ?*bfd_serve.BfdServeBundle = null;
            var bfd_rt: ?*const bfd_status.BfdRuntime = null;
            {
                var guard = tracer.begin(.bfd_start);
                guard.emitStarted(stderr) catch {};
                defer { guard.finish(stderr) catch {}; }

                // Load BFD configuration
                const bfd_result = bfd_serve.loadConfigAndBfd(serve_config.config_path, stderr);

                // Only fail on actual errors, not on "no config" or "disabled"
                switch (bfd_result) {
                    .failed => return .serve_error,
                    else => {},
                }

                // Get BFD bundle pointer
                bfd_bundle = switch (bfd_result) {
                    .configured => |bundle| bundle,
                    else => null,
                };

                // Startup assertion: verify BFD bundle is properly initialized before serve
                if (bfd_bundle) |bundle| {
                    if (bundle.loop_state == null) {
                        stderr.writeAll("FATAL: BFD bundle initialized but loop_state is null\n") catch {};
                        return .serve_error;
                    }
                    if (bundle.transmit_loop_state == null) {
                        stderr.writeAll("FATAL: BFD bundle initialized but transmit_loop_state is null\n") catch {};
                        return .serve_error;
                    }
                    if (bundle.receive_thread == null) {
                        stderr.writeAll("FATAL: BFD bundle initialized but receive_thread is null\n") catch {};
                        return .serve_error;
                    }
                    if (bundle.transmit_thread == null) {
                        stderr.writeAll("FATAL: BFD bundle initialized but transmit_thread is null\n") catch {};
                        return .serve_error;
                    }
                    if (!bundle.bfd_active) {
                        stderr.writeAll("FATAL: BFD bundle initialized but bfd_active is false\n") catch {};
                        return .serve_error;
                    }
                    if (bundle.stop_signal.load()) {
                        stderr.writeAll("FATAL: BFD bundle initialized but stop_signal is already set\n") catch {};
                        return .serve_error;
                    }

                    // Extract BFD runtime pointer for HTTP server status integration
                    bfd_rt = &bundle.runtime;
                }
            }

            // Phase: bgp_load — BGP config loading
            var bgp_bundle: ?*bgp_serve.BgpServeBundle = null;
            var bgp_result: bgp_serve.BgpLoadResult = .no_config;
            {
                var guard = tracer.begin(.bgp_load);
                guard.emitStarted(stderr) catch {};
                defer { guard.finish(stderr) catch {}; }

                var bgp_log_buf = logging.BufferedWriter.init();
                logging.logBgpLoadStarted(&bgp_log_buf) catch {};
                stderr.writeAll(bgp_log_buf.slice()) catch {};
                bgp_result = bgp_serve.loadConfigAndBgp(serve_config.config_path, stderr);
                bgp_log_buf.reset();
                const result_tag: []const u8 = switch (bgp_result) {
                    .configured => "configured", .disabled => "disabled",
                    .no_config => "no_config", .not_configured => "not_configured",
                    .failed => |load_err| load_err.message,
                };
                logging.logBgpLoadResult(&bgp_log_buf, result_tag, "") catch {};
                stderr.writeAll(bgp_log_buf.slice()) catch {};

                bgp_bundle = switch (bgp_result) {
                    .configured => |bundle| bundle,
                    else => null,
                };
            }

            // Clean up BGP bundle ONLY when configured (defer after extracting bundle)
            defer if (bgp_bundle) |bundle| bgp_serve.cleanupBgpBundle(bundle);

            // Phase: bgp_runtime_start — BGP FSM thread startup
            if (bgp_bundle) |bundle| {
                var guard = tracer.begin(.bgp_runtime_start);
                guard.emitStarted(stderr) catch {};
                _ = bgp_serve.startBgpRuntimeThread(bundle, stderr);
                guard.finish(stderr) catch {};
            }

            // Derive config check state from config_path.
            const config_check: status.ConfigCheckState = if (serve_config.config_path) |path|
                .{ .loaded = .{ .path = path } }
            else
                .no_config;

            // Phase: lab_and_net_diag_config_parse — Lab and network diag config parsing
            // We parse both from the same config file to avoid reading it twice.
            var lab_cfg: config.LabConfig = .{};
            var net_diag_cfg: net_diag_config.NetworkDiagConfig = .{};
            if (serve_config.config_path) |path| {
                var guard = tracer.begin(.lab_and_net_diag_config_parse);
                guard.emitStarted(stderr) catch {};
                defer { guard.finish(stderr) catch {}; }

                var raw = wg_args.readConfig(path, std.heap.page_allocator) catch |e| {
                    stderr.print("error: failed to read config: {s}\n", .{@errorName(e)}) catch {};
                    return .serve_error;
                };
                defer raw.deinit(std.heap.page_allocator);

                // Parse lab config
                lab_cfg = config.parseLabConfig(&raw) catch |e| {
                    stderr.print("error: failed to parse [lab]: {s}\n", .{@errorName(e)}) catch {};
                    return .serve_error;
                };

                // Parse network diagnostics config
                net_diag_cfg = net_diag_config.parseNetworkDiagConfig(&raw);
            }

            // Clean up BFD bundle on any exit
            defer if (bfd_bundle) |bundle| bfd_serve.cleanupBfdBundle(bundle);

            // Call HTTP serve with tracer for startup_ready emission.
            // The serve function will emit startup_ready after http_accept_loop_started.
            // Pass the real bgp_result so HTTP status reflects actual BGP state.
            http.serveForeverWithContextAndLabAndTracer(http_config, .{
                .bfd_runtime = bfd_rt,
                .config_check = config_check,
                .bgp_result = bgp_result,
            }, lab_cfg, net_diag_cfg, stdout, &tracer) catch |err| {
                var log_buf = logging.BufferedWriter.init();
                logging.emit(.server_error, &log_buf, &.{
                    .{ .name = "error", .value = logging.FieldValue{ .string = @errorName(err) } },
                }) catch {};
                stderr.writeAll(log_buf.slice()) catch {};
                return .serve_error;
            };

            return .ok;
        },
    }
}
