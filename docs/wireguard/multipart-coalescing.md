# WireGuard Generic-Netlink Multipart Limitation

## Status: Known Limitation

The GenericNetlinkBackend currently **does not implement** multipart netlink message handling (NLM_F_MULTI).

## Background

WireGuard kernel module may respond with multiple netlink messages when:
- Querying all devices (`NLM_F_DUMP`)
- Multiple peers exist on an interface
- Response exceeds the kernel's buffer size

## Current Behavior

The backend currently:
- ✅ Handles single-message responses
- ✅ Discovers WireGuard family ID dynamically
- ✅ Parses device attributes (peer_count, handshake, rx/tx bytes, listen_port)
- ❌ Does NOT handle `NLMSG_DONE` with multiple messages
- ❌ Does NOT coalesce peer data from multiple messages
- ❌ May miss peers on busy interfaces

## Code Comments

From `wg_status_boundary_netlink.zig`:

```zig
/// Query WireGuard device status.
/// ...
/// Note: This is a skeleton - multipart handling (NLM_F_MULTI) is TODO for production.
fn queryWgDevice(...) !wg.WireGuardStatus {
    // ...
}
```

## Production Switch Blocked

Switching from CLI to generic-netlink as the **production default** is blocked until:

1. Multipart recv loop is implemented
2. Peer coalescing from multiple messages is implemented
3. Response parsing is bounded by actual `recv_len`, not `MAX_RCV_SIZE`
4. End-to-end tests prove peer counting works with 10+ peers

## Implementation Notes

### Multipart Message Flow

1. Kernel sends messages with `NLM_F_MULTI` flag
2. Each message ends with `NLMSG_DONE` or `NLMSG_ERROR`
3. Need to loop on `recv()` until `NLMSG_DONE` or error
4. Aggregate peer data across all messages

### Suggested Approach

```zig
// Pseudocode for multipart handling
while (true) {
    const recv_len = try recvWithTimeout(sock, &response_buf, timeout);
    
    var offset: usize = 0;
    while (offset < recv_len) {
        const nlh = parseNlmsgHeader(response_buf[offset..]);
        
        if (nlh.nlmsg_type == NLMSG_DONE) break;
        if (nlh.nlmsg_type == NLMSG_ERROR) return error.netlink_failed;
        
        // Parse message and accumulate peer data
        parseAndAccumulatePeerData(response_buf[offset..nlh.nlmsg_len]);
        
        offset = (offset + nlh.nlmsg_len + 3) & ~@as(usize, 3);
    }
}
```

## Test Coverage

This limitation is documented in fixture tests:

- `wg_status_boundary_netlink_tests.zig` - protocol constants, u16 family ID, timespec parsing
- `wg_status_boundary_netlink_runtime_tests.zig` - runtime proof (skips if no interface)

## References

- Linux netlink(7) manual
- WireGuard UAPI: `include/uapi/linux/wireguard.h`
- Generic netlink: `include/net/genetlink.h`
