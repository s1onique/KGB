// http/serve_context.zig — Serve context for HTTP route handlers.
//
// Context passed to HTTP route handlers containing runtime state derived at serve startup.
// This is the canonical state container for /status endpoint rendering.

const std = @import("std");
const metrics_state = @import("../metrics_state.zig");
const bfd_status = @import("../bfd/status.zig");
const bgp_status = @import("../bgp/status.zig");
const status = @import("../status.zig");

/// Context passed to HTTP route handlers.
///
/// This struct owns the optional BFD runtime pointer for the duration
/// of a serve session. Daemon owns the runtime; context just passes it
/// explicitly to handlers.
///
/// Ownership model:
/// - ServeContext does NOT own the BFD runtime (daemon owns it)
/// - ServeContext does NOT own the BGP bundle (daemon owns it)
/// - ServeContext does NOT own the config_check (immutable state derived at startup)
/// - ServeContext owns the metrics state for rate calculations
/// - MetricsState owns InterfaceSampler (freed in deinit)
pub const ServeContext = struct {
    const Self = @This();

    /// Metrics state for rate calculations.
    metrics: metrics_state.MetricsState,

    /// Optional BFD runtime owned by the daemon.
    /// Null when BFD is not configured.
    bfd_runtime: ?*const bfd_status.BfdRuntime,

    /// Config check state derived at serve startup from loaded config.
    /// This is immutable for the daemon lifetime.
    config_check: status.ConfigCheckState,

    /// BGP status state derived at serve startup from loaded config.
    /// This is immutable for the daemon lifetime.
    bgp_state: bgp_status.BgpStatusState,

    /// Initialize serve context with allocator.
    /// Uses default no_config state since no config path is available.
    pub fn init(allocator: std.mem.Allocator) Self {
        return .{
            .metrics = metrics_state.MetricsState.init(allocator),
            .bfd_runtime = null,
            .config_check = .no_config,
            .bgp_state = .no_config,
        };
    }

    /// Initialize serve context with allocator and pre-configured BFD runtime.
    /// Uses default no_config state since no config path is available.
    pub fn initWithBfd(allocator: std.mem.Allocator, bfd_runtime: *const bfd_status.BfdRuntime) Self {
        return .{
            .metrics = metrics_state.MetricsState.init(allocator),
            .bfd_runtime = bfd_runtime,
            .config_check = .no_config,
            .bgp_state = .no_config,
        };
    }

    /// Initialize serve context with full runtime inputs (BFD + config check + BGP).
    /// This is the primary init path for production serve with config.
    pub fn initWithContext(
        allocator: std.mem.Allocator,
        bfd_runtime: ?*const bfd_status.BfdRuntime,
        config_check: status.ConfigCheckState,
        bgp_state: bgp_status.BgpStatusState,
    ) Self {
        return .{
            .metrics = metrics_state.MetricsState.init(allocator),
            .bfd_runtime = bfd_runtime,
            .config_check = config_check,
            .bgp_state = bgp_state,
        };
    }

    /// Free all context-owned memory.
    pub fn deinit(self: *Self) void {
        self.metrics.deinit();
    }
};
