// bgp/serve_bundle_constructor.zig — canonical `BgpServeBundle`
// constructor.
//
// ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01-FA-3 production
// wiring: this is the single source of truth for the initial TCP
// connect, state install, session, passive listener, prefix
// watcher, and the initial `recordSocketOpen` (FA-3 socket
// accounting). `loadConfigAndBgp` in `serve_integration.zig` is a
// thin file-system wrapper that delegates to this helper after
// parsing the file inputs, so the daemon's real constructor and
// the lifecycle regression tests exercise the same code path.
//
// The split keeps `serve_integration.zig` focused on the daemon's
// configuration wrapper, status accessors, and lifecycle
// re-exports; the constructor body lives in this small module so
// each file stays under the LLM-friendliness hard limit.
//
// Single-owner contract (FA-3 re-verdict P0):
//   * the constructor CONSUMES `raw`, `tcp`, and `prefixes` from
//     entry on every path (success or failure),
//   * the constructor releases every transferred resource on the
//     failure path,
//   * the caller MUST NOT touch any of these resources after
//     `initBgpServeBundle` returns.
//   * the failure-path helpers call `bundle.tcp.close()` exactly
//     once (the bundle-owned copy), NEVER the caller's input
//     pointer, so the kernel fd is closed exactly once.

const std = @import("std");
const config = @import("../config.zig");
const config_parse = @import("config_parse.zig");
const session = @import("session.zig");
const types = @import("types.zig");
const tcp_transport = @import("tcp_transport.zig");
const passive_listener_integration = @import("passive_listener_integration.zig");
const allocation_tracker = @import("../runtime/allocation_tracker.zig");
const export_reload_apply = @import("export_reload_apply.zig");
const serve_export_integration = @import("serve_export_integration.zig");
const serve_integration = @import("serve_integration.zig");

const BgpServeBundle = serve_integration.BgpServeBundle;
const BgpLoadResult = serve_integration.BgpLoadResult;

/// FA-3 re-verdict P0: single-owner release helper. Closes the
/// bundle-owned TCP descriptor (NOT the caller's input pointer),
/// deinits `export_state` (frees the `current_exported_prefixes`
/// copy), frees the input `prefixes` slice via the passed
/// allocator, deinits `raw`, and destroys the bundle struct.
/// After this runs the caller MUST NOT touch any of the
/// previously-transferred resources.
fn releaseBundleOnFailure(bundle: *BgpServeBundle, allocator: std.mem.Allocator) void {
    if (bundle.socket_owned) {
        bundle.tcp.close();
        bundle.socket_owned = false;
    }
    bundle.export_state.deinit();
    if (bundle.prefixes.len > 0) {
        allocator.free(bundle.prefixes);
    }
    bundle.raw.deinit(std.heap.page_allocator);
    std.heap.page_allocator.destroy(bundle);
}

/// Canonical `BgpServeBundle` constructor: takes already-parsed
/// pieces and returns either a fully-wired bundle or a `LoadFailure`.
///
/// `loadConfigAndBgp` is the file-system wrapper that parses the raw
/// inputs and then calls this helper. Hermetic tests call this
/// helper directly with pre-built pieces so they don't depend on
/// the file system.
///
/// Single-owner contract (FA-3 re-verdict P0):
///   * the constructor CONSUMES `raw`, `tcp`, and `prefixes` from
///     entry on every path (success or failure),
///   * the constructor releases every transferred resource on the
///     failure path,
///   * the caller MUST NOT touch any of these resources after
///     `initBgpServeBundle` returns.
///
/// FA-3 session-init fix: the failure path closes the PHYSICAL
/// descriptor BEFORE recording the accounting transition. The
/// previous order cleared `socket_owned` before the helper could
/// look at it, so the kernel fd leaked while the oracle reported
/// a clean socket count.
pub fn initBgpServeBundle(
    raw_input: config.RawConfig,
    bgp_cfg: config_parse.BgpConfig,
    session_config: session.SessionConfig,
    tcp: *tcp_transport.TcpTransport,
    prefixes: []types.Ipv4Prefix,
    stderr: anytype,
    allocator: std.mem.Allocator,
) BgpLoadResult {
    // The constructor consumes `raw` from entry on every path,
    // success or failure. We move the by-value parameter into a
    // local mutable so the pre-bundle failure branch can call
    // `deinit` directly without re-parsing the input.
    var owned_raw = raw_input;

    // Pre-bundle allocation failure: caller no longer owns
    // anything (constructor consumes from entry). Clean up every
    // input:
    //   * prefixes slice (the input owns the backing memory)
    //   * tcp (the bundle never came up so the kernel fd must be
    //     closed by US, not by `releaseBundleOnFailure`)
    //   * raw (the by-value copy holds the heap backing storage)
    const bundle = std.heap.page_allocator.create(BgpServeBundle) catch {
        if (prefixes.len > 0) allocator.free(prefixes);
        tcp.close();
        owned_raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "out of memory creating BGP bundle" } };
    };

    bundle.* = BgpServeBundle{
        .raw = owned_raw,
        .bgp_config = bgp_cfg,
        .session_config = session_config,
        .state = .not_configured,
        .last_error = null,
        .prefixes = prefixes,
        .tcp = undefined,
        .trans = undefined,
        .sess = undefined,
        .export_state = .{},
        .reconnect_faults = null,
    };
    bundle.tcp = tcp.*;
    bundle.trans = bundle.tcp.toTransport();
    bundle.socket_owned = true;
    bundle.allocator = allocator;
    bundle.export_state.init(allocator);
    export_reload_apply.initExportedPrefixes(&bundle.export_state, prefixes);

    bundle.reconnect_connector.ctx = @ptrCast(&bundle.production_connector_ctx);

    bundle.reconnect_memory_state = allocation_tracker.init(allocator) catch |e| {
        releaseBundleOnFailure(bundle, allocator);
        return .{ .failed = .{ .message = @errorName(e) } };
    };

    if (bundle.socket_owned) {
        allocation_tracker.recordSocketOpen(
            bundle.reconnect_memory_state.?,
        );
    }

    const sess = session.init(session_config, &bundle.trans) catch |e| {
        // FA-3 re-verdict P0: close PHYSICAL before accounting.
        if (bundle.reconnect_memory_state) |state| {
            if (bundle.socket_owned) {
                bundle.tcp.close();
                allocation_tracker.recordSocketClose(state);
                bundle.socket_owned = false;
            }
            allocation_tracker.validateForDestroy(state) catch |err| {
                std.debug.panic(
                    "constructor rollback left dirty state: {s}",
                    .{@errorName(err)},
                );
            };
            allocation_tracker.deinit(state, allocator);
            bundle.reconnect_memory_state = null;
        }
        releaseBundleOnFailure(bundle, allocator);
        return .{ .failed = .{ .message = @errorName(e) } };
    };
    bundle.sess = sess;
    bundle.state = .configured;

    passive_listener_integration.createPassiveListener(bundle, stderr);
    _ = serve_export_integration.initPrefixWatcher(bundle, stderr, allocator);

    return .{ .configured = bundle };
}
