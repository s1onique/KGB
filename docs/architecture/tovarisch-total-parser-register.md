# Tovarisch Total Parser Register

Canonical register of external-input parsers and boundary adapters in the `tovarisch` leaf daemon.

**Doctrine**: External input must be parsed by total functions: `input bytes/slices -> structured success or structured failure`.

## Classification Categories

### TOTAL
Parser modules where ALL public functions are total. No `@panic`, `unreachable`, `.?` unwrap, or `@enumFromInt` without bounds validation allowed in public API surface.

### BOUNDARY_TOTAL
Boundary adapters that consume external data but delegate to TOTAL parsers internally. May use optional unwrap in internal computation, but external API is total.

### STATEFUL_ADAPTER
Stateful session/protocol adapters with external input handling. Error recovery and FSM state transitions must not panic.

### DEFERRED
Parser modules with known partial behavior that cannot be fixed in current ACT.

**Current DEFERRED count: 0**

All external-input parser modules in tovarisch are now total.

---

## TOTAL Modules

### status_query.zig
- **Source**: HTTP query string parameters (`/status.json?include=network_diag`)
- **External input**: Raw query bytes
- **Structured failure**: `StatusInclude.unsupported` + `has_unknown=true` for malformed inputs
- **Forbidden patterns**: None
- **Note**: Query parsing is closed enum-based; malformed values classified as `.unsupported`

### config_parse_helpers.zig
- **Source**: Configuration file values (booleans, ports, CIDR notation)
- **External input**: String values from INI config sections
- **Structured failure**: `ConfigError` variants (InvalidValue, InvalidPort, InvalidCidr, EmptyValue)
- **Forbidden patterns**: None
- **Note**: All parsing functions return errors; no panics

### bgp/config_parse.zig
- **Source**: INI configuration file `[bgp]` section
- **External input**: Config file string values
- **Structured failure**: `ConfigError` variants
- **Forbidden patterns**: None
- **Note**: Total parser via ConfigError returns; parseIpv4Address validates octets

### bgp/frame_decode.zig
- **Source**: BGP wire protocol frames
- **External input**: Raw packet bytes
- **Structured failure**: `DecodeError` variants (BadMarker, LengthTooShort, LengthTooLong, IncompleteFrame, UnknownMessageType)
- **Forbidden patterns**: None
- **Note**: Uses switch on message type with explicit else returning error

### bgp/notification_decode.zig
- **Source**: BGP NOTIFICATION error codes
- **External input**: Wire format error code/subcode bytes
- **Structured failure**: Returns `"Unknown Error"` / `"Unknown Subcode"` for unrecognized codes
- **Forbidden patterns**: None
- **Note**: Fallthrough to else case is explicit and safe

### bfd/packet.zig
- **Source**: BFD wire protocol packets (RFC 5880)
- **External input**: Raw UDP packet bytes
- **Structured failure**: `error.InvalidPacket` for truncated/malformed packets
- **Accepted patterns**: `@enumFromInt` after bit masking (RFC 5880 guarantees valid range)
- **Note**: State and Diagnostic enums decoded via `@truncate` + mask + `@enumFromInt`

### net/ss_parser.zig
- **Source**: `ss -tin` command output
- **External input**: Shell command stdout
- **Structured failure**: `ParseError` variants (NoData, MalformedOutput, InvalidNumber)
- **Accepted patterns**: `.?` after null check via `if (x) |val|` pattern
- **Note**: All optional unwraps are preceded by explicit null checks

### net/wg_show_parser.zig
- **Source**: `wg show` command output
- **External input**: Shell command stdout
- **Structured failure**: `ParseError` variants (NoInterface, InvalidNumber, MalformedOutput)
- **Forbidden patterns**: None
- **Note**: Line-by-line parsing with explicit null returns for unrecognized format

### net/linux_addr_parse.zig
- **Source**: rtnetlink message attributes
- **External input**: Netlink buffer bytes
- **Structured failure**: `parseLabel` returns null for invalid labels
- **Forbidden patterns**: None
- **Note**: Helper functions only; bounds-checked slicing

### net/private_ip.zig
- **Source**: IPv4 text addresses
- **External input**: String IP addresses
- **Structured failure**: Returns `.invalid` for malformed input
- **Forbidden patterns**: None
- **Note**: Total parser - all malformed inputs return `.invalid`

### net/interface_filter.zig
- **Source**: Interface address data
- **External input**: InterfaceAddress list
- **Structured failure**: Uses private_ip.zig for address classification
- **Forbidden patterns**: None
- **Note**: Predicates are total; filtering skips malformed addresses

---

## BOUNDARY_TOTAL Modules

These modules have total external APIs but may use medium-risk patterns internally (like `@intCast` with bounds checks).

### config.zig
- **Source**: INI configuration file parsing
- **External input**: Config file content
- **Structured failure**: Delegates to `config_parse_helpers`
- **Forbidden patterns**: None
- **Note**: Public API is total via ConfigError

### config_lab.zig
- **Source**: `[lab]` section from INI config
- **External input**: Lab configuration values
- **Structured failure**: `ConfigError` variants
- **Forbidden patterns**: None
- **Note**: Delegates to config_parse_helpers

### net/linux_read.zig
- **Source**: Linux sysfs/procfs files
- **External input**: Kernel-provided file content
- **Structured failure**: `LinuxReadResult` union variants (missing, permission_denied, unsupported_platform, too_large, malformed, io_error)
- **Forbidden patterns**: None
- **Note**: `trimAndClone()` returns `TrimCloneError![]u8`; caller handles OutOfMemory

### net/linux_stats.zig
- **Source**: sysfs statistics files
- **External input**: Counter file content
- **Structured failure**: `ParseError` variants via `parseCounter()`
- **Forbidden patterns**: None
- **Note**: Uses linux_read.zig boundary; parseCounter is total

### net/linux_interface_stats.zig
- **Source**: sysfs interface enumeration + statistics
- **External input**: Interface list, stat files
- **Structured failure**: Catches and skips errors from linux_stats.zig
- **Forbidden patterns**: None
- **Note**: Composition boundary, error propagation is total

### net/linux_interfaces.zig
- **Source**: `/sys/class/net` directory enumeration
- **External input**: Kernel-provided directory entries
- **Structured failure**: `ListError` variants (RootDirMissing, RootDirUnreadable)
- **Forbidden patterns**: None
- **Note**: Uses C readdir with explicit null-skip; path validation via access()

### net/safe_command.zig
- **Source**: Shell command output
- **External input**: Command stdout
- **Structured failure**: Error variants for execution failures
- **Forbidden patterns**: None

### net/wg_dump_collector.zig
- **Source**: `wg show dump` command output
- **External input**: WireGuard dump data
- **Structured failure**: Via wg_show_parser.zig
- **Forbidden patterns**: None

### net/wg_show_collector.zig
- **Source**: `wg show` command output
- **External input**: WireGuard show data
- **Structured failure**: Via wg_show_parser.zig
- **Forbidden patterns**: None

### cli.zig
- **Source**: CLI arguments
- **External input**: Command-line arguments
- **Structured failure**: Error variants for parse failures
- **Forbidden patterns**: None

### http/routes.zig
- **Source**: HTTP request parameters
- **External input**: Request query strings, path parameters
- **Structured failure**: Error variants for malformed requests
- **Forbidden patterns**: None

---

## STATEFUL_ADAPTER Modules

### bgp/session.zig
- **Source**: BGP wire protocol messages
- **External input**: TCP stream frames
- **Structured failure**: FSM state transitions handle errors gracefully
- **Note**: Uses frame_decode.zig for parsing; session state machine is robust

### bfd/session.zig
- **Source**: BFD wire protocol packets
- **External input**: UDP packets
- **Structured failure**: Session state machine handles errors
- **Note**: Uses bfd/packet.zig for parsing

### bfd/status.zig
- **Source**: BFD session status
- **External input**: Session state data
- **Structured failure**: Status parsing handles null states gracefully
- **Note**: Uses `.?` after null checks for internal status computation

---

## Accepted Patterns

| Pattern | Location | Rationale |
|---------|----------|-----------|
| `@enumFromInt(diag_val\|state_val)` | bfd/packet.zig:168,172 | Bit-masked `@truncate` output; RFC 5880 guarantees 2-bit state and 5-bit diag |
| `@enumFromInt(field.value)` | net/ss_parser.zig:279 | Uses enum field value; safe by construction |
| `retransmits.?`, `unacked.?`, `rto_ms.?`, `colon_idx.?`, etc. | net/ss_parser.zig | Specific nullable variables with null guards in same function |
| `latest_handshake.?` | net/wg_show_parser.zig:116 | Null-or guard ensures non-null in branch |
| `.?` after null check | bfd/status.zig | Internal status computation; null check precedes unwrap |
| `else => error` | bgp/frame_decode.zig | Explicit unknown handling in switch |
| `@intCast` with bounds | net/*.zig, bfd/*.zig | Prior range validation documented in function comments |

---

## Module Counts

| Category | Count |
|----------|-------|
| TOTAL | 11 |
| BOUNDARY_TOTAL | 11 |
| STATEFUL_ADAPTER | 3 |
| DEFERRED | 0 |
| **Total** | **25** |

---

## Verification Commands

```bash
python3 scripts/verify_total_parsers.py
python3 scripts/verify_total_parsers.py --self-test
```

---

## Maintenance

When adding new external-input parser modules:

1. Classify according to categories above
2. Ensure all public API functions are total
3. Update this register
4. Add to `scripts/total_parser_verifier/classifications.py`
5. Document any accepted patterns in the Accepted Patterns table
6. Verify no forbidden patterns via `verify_total_parsers.py`
