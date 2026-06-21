// cli/net_diag_config.zig — Network diagnostics config parsing for serve command
//
// Extracted from commands.zig to satisfy LLM-friendliness line limits.

const std = @import("std");
const network_diag_config = @import("../net/network_diag_config.zig");
const wg_args = @import("wg_args.zig");

// Re-export for commands.zig convenience
pub const NetworkDiagConfig = network_diag_config.NetworkDiagConfig;
pub const parseNetworkDiagConfig = network_diag_config.parseNetworkDiagConfig;
