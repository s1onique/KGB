// bgp/session_recv.zig — BGP receive state management
//
// Extracted from session.zig for LLM-friendliness.
// Handles recv buffer management and connection closure detection.

const transport = @import("transport.zig");
const session_recv = @This();

// ============================================================================
// Receive State Management
// ============================================================================

/// Result of a recv operation - distinguishes between no-data and connection failure.
/// This allows callers to handle TCP connection errors distinctly from empty reads.
pub const RecvResult = struct {
    /// Bytes received and copied into the buffer.
    bytes_copied: usize,
    /// Whether the transport was closed by the peer (EOF or TCP reset).
    /// When true, the session should transition to failed state and schedule reconnect.
    connection_closed: bool,
};

/// Check if transport is closed.
/// Returns true if the connection was lost (EOF or TCP reset).
/// This allows callers to distinguish between "no data yet" and "connection lost".
pub fn transportIsClosed(trans: *const transport.Transport) bool {
    return trans.isClosedFn(trans.ctx);
}
