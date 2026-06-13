// bgp/session_config_builder.zig — BGP SessionConfig construction with auto-derivation
//
// Builds SessionConfig from BgpConfig with same_as auto-derivation.
// For same-AS/iBGP, AS_PATH must be empty (RFC 4271).
// Auto-derives same_as=true when local_as == peer_as.
//
// This module is extracted from serve_integration.zig to keep that file under
// the 450-line LLM-friendly limit.

const std = @import("std");
const config_parse = @import("config_parse.zig");
const session = @import("session.zig");
const types = @import("types.zig");

/// Build SessionConfig with same_as auto-derivation (RFC 4271 iBGP rule).
/// Exposed for testing to prove loadConfigAndBgp() auto-derivation.
pub fn buildSessionConfig(
    bgp_cfg: config_parse.BgpConfig,
    prefixes: []const types.Ipv4Prefix,
) !session.SessionConfig {
    const local_addr = try config_parse.parseIpv4Address(bgp_cfg.local_address);
    const router_addr = try config_parse.parseIpv4Address(bgp_cfg.router_id);
    const peer_addr = try config_parse.parseIpv4Address(bgp_cfg.peer_address);

    // Auto-derive same_as when local_as == peer_as (RFC 4271 iBGP/same-AS rule)
    const same_as = bgp_cfg.same_as or (bgp_cfg.local_as == bgp_cfg.peer_as);

    return session.SessionConfig{
        .peer_address = peer_addr,
        .peer_port = bgp_cfg.peer_port,
        .local_address = local_addr,
        .local_as = bgp_cfg.local_as,
        .peer_as = bgp_cfg.peer_as,
        .router_id = router_addr,
        .hold_time_seconds = bgp_cfg.hold_time_seconds,
        .keepalive_seconds = bgp_cfg.keepalive_seconds,
        .connect_timeout_ms = bgp_cfg.connect_timeout_ms,
        .prefixes = prefixes,
        .same_as = same_as,
    };
}
