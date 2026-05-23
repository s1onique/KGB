# ACT 5: Add private interface traffic collector from Linux sysfs

**Status: open** (slice ACT 5a ✅)

Linux exposes per-interface statistics under:
`/sys/class/net/<iface>/statistics/`

The kernel ABI documents counters such as `rx_bytes`, `tx_bytes`, `rx_packets`, and `tx_packets`.

## ACT 5a: Pure sysfs interface statistics parser (slice)

**Status: ✅ done**

This slice introduces only the data model and pure parsing helpers.
It does NOT enumerate live interfaces or read `/sys/class/net` yet.

### Board

| ID | Work Item | Status |
|---|---|---|
| webservice-026 | Create `net/linux_stats.zig` - pure stats parser | ✅ done |
| webservice-026a | Define `InterfaceStats` struct | ✅ done |
| webservice-026b | Add `parseCounter()` helper | ✅ done |
| webservice-026c | Add `statsFromCounters()` test helper | ✅ done |
| webservice-027 | Implement `getInterfaceStats()` function | open |
| webservice-028 | Filter to private interfaces only | open |
| webservice-029 | Add interface stats to metrics model | open |
| webservice-030 | Add tests for Linux stats collection | open |
| webservice-031 | Run `make gate`, `make tovarisch-build`, `make tovarisch-test` | open |

### Acceptance

- [x] `linux_stats.zig` exists with `InterfaceStats` struct.
- [x] Pure parsing helpers (`parseCounter`, `statsFromCounters`) work correctly.
- [x] Parser rejects empty, negative, non-numeric, and overflow input.
- [x] Tests are wired via `test_all.zig`.
- [x] `make tovarisch-test` passes (140 tests).
- [x] `make gate` passes.
- [ ] Live sysfs collection (ACT 5b) is deferred.
- [ ] Interface stats are available for private interfaces.
- [ ] Non-private interfaces are filtered out by default.

### Files Changed

- `tovarisch/src/net/linux_stats.zig` — InterfaceStats struct, parseCounter, statsFromCounters
- `tovarisch/src/test_all.zig` — Added refAllDecls for linux_stats.zig

### Implementation Details

The pure parser (`linux_stats.zig`) provides:

- `ParseError` error set with `EmptyCounter`, `InvalidCounter`, `NegativeCounter`, `CounterOverflow`
- `InterfaceStats` struct with `rx_bytes`, `tx_bytes`, `rx_packets`, `tx_packets` (all u64)
- `parseCounter(bytes: []const u8) !u64` — parses sysfs stat file contents
- `statsFromCounters(...) !InterfaceStats` — builds stats from four counter strings

Parser behavior:
- Trims ASCII whitespace (` \t\r\n`) before parsing
- Rejects empty input
- Rejects negative numbers
- Rejects non-digit characters
- Propagates u64 overflow as `CounterOverflow`

Tests cover all edge cases including u64 max (18446744073709551615) and overflow values.

## Future: ACT 5b (live sysfs collection)

ACT 5b will add:
- Reading `/sys/class/net/<iface>/statistics/*` files
- Enumerating network interfaces
- Filtering to private interfaces
- Wiring stats into the metrics model

See main epic `tovarisch-webservice-day0.md` for full board.
