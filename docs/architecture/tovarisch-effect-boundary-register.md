# Tovarisch Effect Boundary Register

**ACT**: ACT-TOVARISCH-ZIG-HULK20  
**Purpose**: Document and verify functional core / effect boundary discipline

## Overview

Tovarisch Zig code follows a layered architecture:

```
pure domain logic
  -> stateful runtime owners
    -> effect boundary shell
```

This register classifies every production Zig module in `tovarisch/src/` by its effect capabilities.

## Classification Definitions

### PURE

A module is **PURE** when it only transforms inputs into outputs without observable side effects.

**Allowed**:
- Closed enums
- Tagged unions
- Parsing from provided slices
- Formatting into caller-provided writers/buffers
- Deterministic validation
- Caller-provided allocator only when bounded and documented
- No ambient OS/runtime dependencies

**Forbidden**:
- `std.c.*` (POSIX calls)
- `std.process.*`
- `std.fs.cwd()`, `std.fs.open`
- `openFile`, `openForRead`, `createFile`, `deleteFile`, `rename`
- `std.net.*` (sockets)
- `socket`, `connect`, `listen`, `accept`, `bind`
- `std.time.*` (wall-clock reads)
- `std.crypto.random`
- `std.heap.page_allocator`, `std.heap.c_allocator`
- `@panic` on external input
- Global mutable state

### BOUNDARY

A module is **BOUNDARY** when it is allowed to touch external systems (files, processes, sockets).

**Required**:
- Structured result union
- Bounded input/output
- No unbounded allocation from external input
- Explicit allocator source
- Tests for failure modes

### STATEFUL

A module is **STATEFUL** when it owns long-lived runtime state.

**Required**:
- Explicit owner type
- Explicit init/deinit or fixed lifetime
- No hidden global mutation
- Bounded collections or documented caps
- Tests for lifecycle/cleanup if it allocates

### TEST

A module is **TEST** when it is not production code.

**Rules**:
- May import production modules
- Must not be imported by production modules
- Naming convention: `*_tests.zig`, `test_*.zig`, `*_test.zig`

### DEFERRED

A module is **DEFERRED** when classification is unclear or direct effects remain but migration is too large for the current ACT.

**Rules**:
- Verifier reports but does not fail on DEFERRED modules
- Must have a migration plan documented

---

## Module Classifications

### PURE Modules

| Module | Reason |
|--------|--------|
| `bgp/snapshot.zig` | Closed enums, budget types, pure parsing functions |
| `bfd/snapshot.zig` | Closed enums, budget types, pure parsing functions |
| `bgp/types.zig` | Data types, closed enums |
| `bgp/message.zig` | Pure message encoding/decoding |
| `bgp/frame_decode.zig` | Pure frame parsing |
| `bgp/notification_decode.zig` | Pure notification parsing |
| `bgp/config_parse.zig` | Pure config parsing |
| `bgp/validation.zig` | Pure validation functions |
| `bgp/status.zig` | Pure derivation from runtime state |
| `bgp/session_status.zig` | Pure status aggregation |
| `bfd/packet.zig` | Pure BFD packet structures |
| `bfd/config.zig` | Pure BFD configuration types |
| `status_query.zig` | Pure query parsing |
| `status_bgp_diagnostics.zig` | Pure derivation functions |
| `net/linux_addr_parse.zig` | Pure address parsing |
| `net/private_ip.zig` | Pure IP classification |
| `net/rates.zig` | Pure rate calculations |
| `net/stat_formatter.zig` | Pure string formatting |
| `net/ss_parser.zig` | Pure SS output parsing |
| `net/wg_show_parser.zig` | Pure wg show output parsing |
| `net/interface_filter.zig` | Pure interface filtering |
| `net/network_diag_config.zig` | Pure config types |
| `metrics_dto.zig` | Pure data transfer types |
| `config_parse_helpers.zig` | Pure config helpers |

### BOUNDARY Modules

| Module | Effect Patterns | Reason |
|--------|---------------|--------|
| `tunnel_check.zig` | `std.c.access`, `std.c.mkdir`, `std.c.rmdir` | Sysfs tunnel interface detection |
| `net/linux_addr.zig` | `std.c.socket`, `std.c.poll`, `std.c.bind`, `std.c.sendto`, `std.c.recv` | Netlink address enumeration |
| `main.zig` | std.process args, stdout/stderr | CLI entrypoint |
| `cli.zig` | Thin facade to CLI commands | CLI routing |
| `http/routes.zig` | HTTP request handling | HTTP routing boundary |
| `http/response.zig` | `std.c.write` to fd | HTTP response writing |
| `http/server.zig` | `std.c` socket operations | HTTP server |
| `net/linux_read.zig` | `std.c.open`, `std.c.read` | Linux sysfs/procfs reads |
| `net/safe_command.zig` | `std.c.fork`, `std.c.execve` | Process execution |
| `net/iptables.zig` | `std.c` iptables commands | Firewall queries |
| `net/wg_status_boundary.zig` | WireGuard status reading | WireGuard boundary |
| `net/wg_dump_collector.zig` | Command execution | WG dump collection |
| `net/wg_show_collector.zig` | Command execution | WG show collection |
| `net/linux_interface_stats.zig` | File reads | Interface statistics |
| `net/linux_interfaces.zig` | File reads | Interface enumeration |
| `net/linux_stats.zig` | File reads | Network statistics |
| `net/inotify.zig` | `std.c` inotify | File watching |
| `status.zig` | Runtime state access | Status aggregation |
| `status_network_diag.zig` | Network diagnostics collection | Diagnostic effects |
| `status_network_diag_tcp.zig` | TCP socket operations | TCP diagnostics |
| `logging.zig` | File/stdout writes | Logging effects |
| `config.zig` | File reads | Config loading |
| `config_lab.zig` | File reads | Lab config loading |
| `metrics.zig` | Metrics collection | Metrics effects |
| `metrics_state.zig` | Metrics state management | Metrics state |
| `build_info.zig` | Build-time info | Build information |

### STATEFUL Modules

| Module | State Ownership | Notes |
|--------|-----------------|-------|
| `bgp/session.zig` | Owns session state (send_buf, recv_buf, FSM state) | Long-lived TCP session |
| `bgp/runtime.zig` | Owns BGP runtime bundle | Session lifecycle |
| `bgp/serve_integration.zig` | Owns serve integration state | Serve integration |
| `bgp/passive_listener.zig` | Owns listener state | Passive listener |
| `bgp/prefix_watch.zig` | Owns prefix watch state | Prefix watching |
| `bgp/prefix_file_loader.zig` | Owns prefix file state | Prefix file loading |
| `bfd/session.zig` | Owns BFD session state | Long-lived BFD session |
| `bfd/serve_integration.zig` | Owns BFD serve integration | BFD integration |
| `net/diag_event_ring.zig` | Owns event ring buffer | Ring buffer state |
| `net/interface_sampler.zig` | Owns sampler state | Interface sampling |
| `runtime/uvb76_capture.zig` | Owns capture state | UVB-76 capture |

### DEFERRED Modules

| Module | Reason | Migration Plan |
|--------|--------|----------------|
| `status_response.zig` | Uses `std.heap.page_allocator` in handleMetrics | Migrate to caller-provided allocator |
| `http/status_route_contract.zig` | Boundary between PURE and BOUNDARY | Review for pure subset |

### TEST Modules

| Module | Type |
|--------|------|
| `*_tests.zig` | All test files |
| `test_all.zig` | Test runner |
| `test_suite_*.zig` | Test suites |
| `*_fixtures.zig` | Test fixtures |
| `fixtures/` directory | Test fixtures |

---

## Effect Pattern Registry

### Forbidden Patterns in PURE Modules

The following patterns must NOT appear in PURE modules:

```
std.c
std.process
std.fs.cwd
std.fs.open
openFile
openForRead
createFile
deleteFile
rename
std.net
socket
connect
listen
accept
bind
std.time
nanoTimestamp
milliTimestamp
std.crypto.random
std.heap.page_allocator
std.heap.c_allocator
@panic
unreachable (on external input)
```

### Comments Exception

Comments may contain these patterns without triggering a violation, as they document the boundary contract.

---

## Verifier

See `scripts/verify_effect_boundaries.py` for the automated verifier.

---

## Adding New Modules

When adding a new module:

1. Classify it according to the definitions above
2. Add it to the appropriate section in this register
3. If BOUNDARY, ensure structured error handling
4. If STATEFUL, document init/deinit lifecycle
5. Run `python scripts/verify_effect_boundaries.py` to verify

---

## Violation Handling

- **PURE violations**: Must be fixed before merge
- **BOUNDARY violations**: Must be documented and approved
- **STATEFUL violations**: Must document state ownership
- **DEFERRED**: Report only; must have migration plan
- **TEST imports production**: Must be fixed before merge

---

## Revision History

- 2026-07-05: ACT-TOVARISCH-ZIG-HULK20R register/verifier consistency (moved tunnel_check.zig, net/linux_addr.zig to BOUNDARY)
- 2026-05-07: ACT-TOVARISCH-ZIG-HULK20 initial register
