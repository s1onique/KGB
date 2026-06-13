/// BGP UPDATE diagnostics for session.zig.
///
/// Extracted from session.zig to stay under the 450-line LLM-friendliness limit.
/// Provides UpdateInfo and UpdateDiagnostic for structured UPDATE frame logging.

const frame_decode = @import("frame_decode.zig");

/// Information about an UPDATE that was sent during runOnce.
pub const UpdateInfo = struct {
    len: u16,
    withdrawn_len: u16,
    attrs_len: u16,
    nlri_prefixes: usize,
    nlri_bytes: usize,
    batch_end: usize,
    configured: usize,
};

/// Result of UPDATE diagnostic capture before flush.
pub const UpdateDiagnostic = union(enum) {
    none,
    sent: UpdateInfo,
    decode_failed,
    parse_failed,
};

/// Capture UPDATE diagnostic before flushSend.
///
/// Decodes the encoded UPDATE frame to capture diagnostic information.
/// Sets decode_failed if decode fails, parse_failed if parse fails,
/// or sent:UpdateInfo if both succeed.
pub fn captureUpdateDiagnostic(
    sess: anytype,
    send_buf: []u8,
    send_pos: usize,
    batch_end: usize,
    configured: usize,
) void {
    const decoded = frame_decode.decodeFrame(send_buf[0..send_pos]) catch {
        sess.last_update_diagnostic = .decode_failed;
        return;
    };
    if (frame_decode.isUpdate(decoded)) {
        if (frame_decode.parseUpdateBody(decoded)) |ub| {
            sess.last_update_diagnostic = .{ .sent = UpdateInfo{
                .len = ub.total_length,
                .withdrawn_len = ub.withdrawn_routes_length,
                .attrs_len = ub.path_attributes_length,
                .nlri_prefixes = ub.nlri_prefix_count,
                .nlri_bytes = ub.nlri_byte_count,
                .batch_end = batch_end,
                .configured = configured,
            } };
        } else {
            sess.last_update_diagnostic = .parse_failed;
        }
    }
}
