# ACT 5: Add private interface traffic collector from Linux sysfs

**Status: open** (slices ACT 5a ✅, ACT 5b ✅, ACT 5c ✅, ACT 5d ✅, ACT 5e ✅)

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
- [x] Live sysfs collection (ACT 5b) is deferred (now done).

## ACT 5b: Explicit-interface sysfs stat file reader (slice)

**Status: ✅ done**

This slice adds filesystem reading for one explicit interface name.
It does NOT enumerate interfaces, does NOT filter private interfaces,
and does NOT wire `/metrics.json` yet.

### Board

| ID | Work Item | Status |
|---|---|---|
| webservice-032 | Add `ReadError` error set for filesystem errors | ✅ done |
| webservice-033 | Add `readCounterFile()` helper | ✅ done |
| webservice-034 | Add `readInterfaceStats()` function | ✅ done |
| webservice-035 | Add fixture-based tests | ✅ done |
| webservice-036 | Run `make gate` and verify | ✅ done |

### Acceptance

- [x] `readInterfaceStats()` exists and reads four sysfs stat files.
- [x] Uses `parseCounter()` / `statsFromCounters()` — no duplicated parsing.
- [x] `sysfs_root` is injectable for test fixtures.
- [x] Tests use temporary fixture directories/files.
- [x] Tests do not read real `/sys/class/net`.
- [x] Missing files/directories produce errors.
- [x] Invalid/overflow counter contents produce errors.
- [x] `make tovarisch-test` passes.
- [x] `make gate` passes.
- [x] No live interface enumeration was added.
- [x] No `/metrics.json` wiring was added.

### Implementation

The filesystem reader provides:

- `ReadError` error set supplementing `ParseError`:
  - `InterfaceNotFound` — interface directory missing
  - `StatisticsDirMissing` — statistics/ subdirectory missing
  - `StatFileMissing` — required stat file missing
  - `StatFileUnreadable` — failed to read/open a stat file
  - `InvalidStatContents` — parsing failed for file contents
  (merged with `ParseError` variants like `InvalidCounter`, `CounterOverflow`)
- `readInterfaceStats(allocator, sysfs_root, iface) !InterfaceStats`
- `readCounterFile(allocator, path) !u64` — internal helper

### Future: ACT 5c

ACT 5c will handle:
- Interface enumeration from `/sys/class/net`
- Private interface filtering
- Or metrics wiring, depending on the next slice chosen

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

## ACT 5c: Sysfs interface enumeration without filtering (slice)

**Status: ✅ done**

This slice adds interface name enumeration from an injectable sysfs-style root.
It does NOT filter private interfaces, does NOT read statistics,
and does NOT wire `/metrics.json` yet.

### Board

| ID | Work Item | Status |
|---|---|---|
| webservice-037 | Create `net/linux_interfaces.zig` | ✅ done |
| webservice-038 | Add `listInterfaces()` function | ✅ done |
| webservice-039 | Add `freeInterfaceList()` helper | ✅ done |
| webservice-040 | Add fixture-based tests | ✅ done |
| webservice-041 | Add Linux-only smoke test | ✅ done |
| webservice-042 | Update test_all.zig | ✅ done |
| webservice-043 | Run `make gate` and verify | ✅ done |

### Acceptance

- [x] `listInterfaces()` exists and enumerates names from injectable sysfs root.
- [x] `freeInterfaceList()` properly frees allocator-owned copies.
- [x] `sysfs_root` is injectable for test fixtures.
- [x] Tests use temporary fixture directories.
- [x] Tests do not read real `/sys/class/net` (except Linux smoke test).
- [x] Missing root directory produces `RootDirMissing` error.
- [x] Empty fixture root returns empty list.
- [x] Interface names with safe characters (eth0, wg0, br-lan, veth1234) are handled.
- [x] No private interface filtering was added.
- [x] No statistics reading was added inside enumeration.
- [x] No `/metrics.json` wiring was added.
- [x] `make tovarisch-test` passes (142 tests, 2 skipped).
- [x] `make gate` passes.
- [x] ACT 5 remains open.

### Implementation

The interface enumerator provides:

- `ListError` error set:
  - `RootDirMissing` — sysfs root directory does not exist
  - `RootDirUnreadable` — sysfs root directory cannot be opened
  - `OutOfMemory` — allocation failure
- `listInterfaces(allocator, sysfs_root) ![][]const u8`
  - Opens sysfs root directory
  - Iterates directory entries using `opendir()`/`readdir()`/`closedir()`
  - Skips "." and ".."
  - Returns allocator-owned copies of interface names
- `freeInterfaceList(allocator, names) void`

### Files Changed

- `tovarisch/src/net/linux_interfaces.zig` — listInterfaces, freeInterfaceList
- `tovarisch/src/net/linux_interfaces_tests.zig` — fixture tests, Linux smoke test
- `tovarisch/src/test_all.zig` — Added refAllDecls for linux_interfaces modules

### Next: ACT 5d

ACT 5d should combine enumeration + `readInterfaceStats()` into a list of interface stats,
still without private filtering or metrics wiring.

## ACT 5d: Combine interface enumeration with stats reading (slice)

**Status: ✅ done**

This slice adds a composition layer that combines `listInterfaces()` with
`readInterfaceStats()` to return a list of interface names with their traffic
counters. It does NOT filter private interfaces, does NOT wire `/metrics.json`.

### Board

| ID | Work Item | Status |
|---|---|---|
| webservice-044 | Create `net/linux_interface_stats.zig` | ✅ done |
| webservice-045 | Define `InterfaceStatsSnapshot` struct | ✅ done |
| webservice-046 | Add `collectInterfaceStats()` function | ✅ done |
| webservice-047 | Add `freeInterfaceStatsSnapshots()` function | ✅ done |
| webservice-048 | Add fixture-based tests | ✅ done |
| webservice-049 | Add Linux-only smoke test | ✅ done |
| webservice-050 | Update test_all.zig | ✅ done |
| webservice-051 | Run `make gate` and verify | ✅ done |

### Acceptance

- [x] `collectInterfaceStats()` exists and composes enumeration + stats reading.
- [x] `InterfaceStatsSnapshot` struct holds name and stats.
- [x] `freeInterfaceStatsSnapshots()` properly frees allocator-owned copies.
- [x] `sysfs_root` is injectable for test fixtures.
- [x] Tests use temporary fixture directories.
- [x] Missing/unreadable stats skip that interface, not fail the collection.
- [x] Empty fixture root returns empty snapshot list.
- [x] Missing root returns error from enumeration.
- [x] No private interface filtering was added.
- [x] No `/metrics.json` wiring was added.
- [x] `make tovarisch-test` passes.
- [x] `make gate` passes.
- [x] ACT 5 remains open.

### Implementation

The composition layer provides:

- `InterfaceStatsSnapshot` struct with `name: []const u8` and `stats: InterfaceStats`
- `collectInterfaceStats(allocator, sysfs_root) ![]InterfaceStatsSnapshot`
  - Calls `listInterfaces()` to enumerate interface names
  - Calls `readInterfaceStats()` for each interface
  - Skips interfaces with missing, unreadable, or malformed stats
  - Returns owned snapshots with allocator-owned name copies
- `freeInterfaceStatsSnapshots(allocator, snapshots) void`

### Files Changed

- `tovarisch/src/net/linux_interface_stats.zig` — collectInterfaceStats, freeInterfaceStatsSnapshots
- `tovarisch/src/net/linux_interface_stats_tests.zig` — fixture tests, Linux smoke test
- `tovarisch/src/test_all.zig` — Added refAllDecls for linux_interface_stats modules

### Next: ACT 5e

ACT 5e should add private-interface filtering based on existing `private_ip.zig` logic.

## ACT 5e: Private-interface filtering using fixture-backed address source

**Status: ✅ done**

This slice adds a filtering layer that takes collected interface stats snapshots
and keeps only interfaces that have at least one private IP address.
It does NOT wire `/metrics.json`, does NOT change HTTP output, does NOT change
listen behavior.

### Board

| ID | Work Item | Status |
|---|---|---|
| webservice-052 | Create `net/interface_filter.zig` | ✅ done |
| webservice-053 | Define `InterfaceAddress` struct | ✅ done |
| webservice-054 | Add `interfaceHasPrivateAddress()` predicate | ✅ done |
| webservice-055 | Add `filterPrivateInterfaceStats()` function | ✅ done |
| webservice-056 | Add fixture-based tests | ✅ done |
| webservice-057 | Update test_all.zig | ✅ done |
| webservice-058 | Run `make gate` and verify | ✅ done |

### Acceptance

- [x] `interface_filter.zig` exists with address-source abstraction.
- [x] `InterfaceAddress` struct holds iface and address fields.
- [x] `interfaceHasPrivateAddress()` predicate returns true if any address is private.
- [x] Reuses existing `private_ip.zig` classification logic.
- [x] `filterPrivateInterfaceStats()` returns owned snapshot copies.
- [x] Input snapshots are not freed or modified.
- [x] Result can be freed via `freeInterfaceStatsSnapshots()`.
- [x] Tests cover all required cases (RFC1918, public, multiple, no addresses, etc.).
- [x] Loopback and link-local correctly excluded (not private per private_ip).
- [x] Malformed addresses ignored and do not include interface.
- [x] IPv6 addresses handled per current private_ip semantics (IPv4-only).
- [x] No live address discovery added.
- [x] No rtnetlink implemented.
- [x] No `/proc/net/fib_trie` parsing.
- [x] No shell command parsing (ip addr).
- [x] No `/metrics.json` wiring.
- [x] `make tovarisch-test` passes.
- [x] `make gate` passes.
- [x] ACT 5 remains open.

### Implementation

The filtering module provides:

- `InterfaceAddress` struct with `iface: []const u8` and `address: []const u8`
- `interfaceHasPrivateAddress(iface, addresses) bool`
  - Pure predicate with no allocator requirements
  - Checks all addresses for matching interface name
  - Returns true if any address is classified as `.private` by private_ip.zig
  - Returns false for no addresses, no private, or malformed
- `filterPrivateInterfaceStats(allocator, snapshots, addresses) ![]InterfaceStatsSnapshot`
  - Includes only snapshots with at least one private address
  - Returns owned copies with allocator-owned name copies
  - Uses errdefer for partial cleanup on allocation failure

### Files Changed

- `tovarisch/src/net/interface_filter.zig` — InterfaceAddress, interfaceHasPrivateAddress, filterPrivateInterfaceStats
- `tovarisch/src/net/interface_filter_tests.zig` — comprehensive fixture tests
- `tovarisch/src/test_all.zig` — Added refAllDecls for interface_filter modules

### Next: ACT 5f

ACT 5f should add live Linux address discovery (likely via rtnetlink).
The `/metrics.json` endpoint remains unwired until live address discovery exists.

Note: Linux rtnetlink is the proper kernel interface for reading network addresses.
`rtnetlink(7)` documents that NETLINK_ROUTE sockets control network routes, IP addresses,
link parameters, neighbors, and related state. This is deferred until ACT 5f.

## Future: ACT 5b (live sysfs collection)

ACT 5b will add:
- Reading `/sys/class/net/<iface>/statistics/*` files
- Enumerating network interfaces
- Filtering to private interfaces
- Wiring stats into the metrics model

See main epic `tovarisch-webservice-day0.md` for full board.
