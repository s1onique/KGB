// wg/config.zig — WireGuard configuration module
//
// This module provides WireGuard-specific configuration types and helpers.
// It re-exports WgConfig from the parent config.zig for convenience.

const std = @import("std");
const config = @import("../config.zig");

/// Re-export WgConfig for convenience in wg submodules.
pub const WgConfig = config.WgConfig;

/// Re-export ConfigError for convenience.
pub const ConfigError = config.ConfigError;

/// Re-export parseWgConfig for convenience.
pub const parseWgConfig = config.parseWgConfig;
