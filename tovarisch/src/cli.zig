/// Thin public facade for the tovarisch CLI.
/// All actual implementation is in the cli/ subdirectory.
const commands = @import("cli/commands.zig");

pub const ExitCode = commands.ExitCode;
pub const run = commands.run;

// Re-export for callers that need these types directly
pub const CliError = @import("cli/args.zig").CliError;
pub const ServeParseResult = @import("cli/args.zig").ServeParseResult;
pub const parseServeArgs = @import("cli/args.zig").parseServeArgs;
