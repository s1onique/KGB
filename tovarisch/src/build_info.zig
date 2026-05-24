// build_info.zig — Build-time injected version metadata for tovarisch
//
// Values are injected at build time via b.addOptions() in build.zig:
//   - base_version: from VERSION env var or tovarisch/VERSION file
//   - commit: from GIT_COMMIT env var or git rev-parse --short=7 HEAD
//   - dirty: from BUILD_DIRTY env var or git status --porcelain
//   - version: base_version + "+" + commit [+ ".dirty"] (e.g., "0.1.2+a1b2c3d" or "0.1.2+a1b2c3d.dirty")
//
// This module is the single source of truth for runtime version information.

pub const base_version: []const u8 = @import("build_options").base_version;
pub const commit: []const u8 = @import("build_options").commit;
pub const dirty: bool = @import("build_options").dirty;
pub const version: []const u8 = @import("build_options").version;
