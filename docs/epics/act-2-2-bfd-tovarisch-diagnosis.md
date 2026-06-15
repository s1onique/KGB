# [Open] ACT 2.2: Diagnose tovarisch BFD Receive/Respond Path

## Parent Epic

[bgp-bfd-netns-lab.md](./bgp-bfd-netns-lab.md) — BGP/BFD Netns Lab

## Goal

Diagnose why BIRD sends BFD packets but tovarisch does not respond, and prove the receive/respond path.

## Context

ACT 2.1 fixed BIRD multihop BFD configuration, which successfully created a BFD session object and sent packets. However, the session remains `Down` because:

- tcpdump in BIRD namespace shows packets sent to `10.77.0.2:4784`
- tcpdump shows `Your Discriminator: 0x00000000` (BIRD waiting for tovarisch to respond)
- No response packets visible
- tovarisch shows `0/1 bfd sessions up`

## Key Questions

| Question | Diagnostic | Evidence Needed |
|----------|------------|-----------------|
| Is tovarisch listening on UDP/4784? | `ss -lunp` in namespace | Socket bound to port 4784 |
| Do BIRD packets arrive at veth-tovarisch? | tcpdump on `veth-tovarisch` | Incoming BFD packets captured |
| Does tovarisch log BFD receive? | tovarisch log grep | BFD receive/transmit events |
| Does tovarisch send any BFD packets back? | tcpdump on `veth-tovarisch` | Outgoing BFD control packets |

## Implementation

### New Files

| File | Purpose |
|------|---------|
| `scripts/lab_bgp_bfd_netns_tovarisch_diag.sh` | tovarisch-side BFD diagnostics |

### New Functions

| Function | Purpose |
|----------|---------|
| `collect_tovarisch_socket_state()` | `ss -lunp` to prove UDP/4784 listener |
| `start_tcpdump_tovarisch_bfd()` | Capture BFD packets on `veth-tovarisch` |
| `collect_tovarisch_bfd_logs()` | Grep tovarisch logs for BFD events |
| `collect_tovarisch_bfd_tx_stats()` | HTTP status BFD TX/rx counters |
| `collect_tovarisch_bfd_diagnostics()` | Comprehensive pre-BFD diagnostics |

### Enhanced Functions

| Function | Enhancement |
|----------|-------------|
| `assert_bfd_up()` | Pre-BFD diagnostics, tcpdump in both namespaces, comprehensive failure output |
| `print_diagnostics()` | ACT 2.2 artifact display |

### New Artifacts

| Artifact | Purpose |
|----------|---------|
| `tovarisch-socket-state.txt` | `ss -lunp` output proving UDP/4784 listener |
| `tcpdump-bfd-tovarisch.txt` | BFD packets captured on `veth-tovarisch` |
| `tovarisch-bfd-logs.txt` | tovarisch log grep for BFD keywords |
| `veth-stats.txt` | veth interface RX/TX statistics |
| `tovarisch-bfd-tx-stats.txt` | BFD TX statistics from HTTP status |

## ACT 2.2 Board

| ID | Work Item | Status |
|----|-----------|--------|
| netns-024 | Create tovarisch-side diagnostic script | **done** |
| netns-025 | Add tcpdump in tovarisch namespace | **done** |
| netns-026 | Add `ss -lunp` socket collection | **done** |
| netns-027 | Add veth RX/TX statistics | **done** |
| netns-028 | Add tovarisch BFD log grep | **done** |
| netns-029 | Enhance assert_bfd_up failure output | **done** |
| netns-030 | Split epic to pass LLM-friendliness gate | **done** |
| netns-031 | Run syntax checks | **done** |
| netns-032 | Run `make gate` | **done** |
| netns-033 | Manual CI run for diagnostics evidence | pending |

## Acceptance Criteria

### Implemented

- [x] `tcpdump` capture in `kgb-lab-tovarisch` namespace (on `veth-tovarisch`)
- [x] `ss -lunp` collection proving UDP/4784 listener existence
- [x] `veth` interface statistics (RX/TX packet counts, errors)
- [x] tovarisch log BFD keyword grep
- [x] BFD TX statistics from HTTP status
- [x] Enhanced failure diagnostics with actionable output
- [x] Pre-wait and post-wait diagnostic collection
- [x] `make gate` passes
- [x] Syntax checks pass

### Manual CI Close Condition (Diagnostic Success)

**ACT 2.2 is a diagnostic ACT.** A failing workflow with complete artifacts is a successful diagnostic run. The close condition is:

Manual diagnostic run complete means:
- workflow reaches ACT 2.2 diagnostics
- artifacts upload succeeds even if BFD remains Down
- `tovarisch-socket-state.txt` exists
- `tcpdump-bfd-tovarisch.txt` exists
- `tcpdump-bfd-bird.txt` exists
- `tovarisch-bfd-logs.txt` exists
- `veth-stats.txt` exists
- `status-http-bfd.json` exists

### Finding Classification

After inspecting artifacts, classify the finding:

| Finding | Evidence | Root Cause |
|---------|----------|------------|
| **A** | No UDP/4784 socket in `tovarisch-socket-state.txt` | BFD runtime not started or socket not bound |
| **B** | Socket exists, `tcpdump-bfd-tovarisch.txt` empty | Packet delivery failure (veth, routing) |
| **C** | tcpdump shows incoming packets, no logs | Packet received but not handled |
| **D** | Logs show transmit, tcpdump no outgoing | tovarisch sends but return path fails |

**Do not require BFD Up for ACT 2.2 close.** The diagnostic artifacts are the deliverable.

## Expected Findings

Based on ACT 2.1 evidence, one of these will be true:

### Finding A: tovarisch not listening on UDP/4784

```
tovarisch-socket-state.txt shows no UDP 4784 socket
→ Root cause: BFD runtime not started, socket not bound
→ Fix: Check tovarisch BFD initialization logs
```

### Finding B: Packets not arriving at veth-tovarisch

```
tovarisch-socket-state.txt shows UDP 4784 socket EXISTS
tcpdump-bfd-tovarisch.txt shows NO packets
→ Root cause: Packet delivery issue (veth, routing, firewall)
→ Fix: Check veth-stats.txt for RX drops
```

### Finding C: Packets arriving but tovarisch not responding

```
tcpdump-bfd-tovarisch.txt shows INCOMING BFD packets
tovarisch-bfd-logs.txt shows NO receive events
→ Root cause: BFD packet not delivered to socket (namespace isolation issue)
→ Fix: Check socket binding vs interface binding
```

### Finding D: tovarisch sending but packets not reaching BIRD

```
tovarisch-bfd-logs.txt shows TRANSMIT events
tcpdump-bfd-tovarisch.txt shows OUTGOING packets
tcpdump-bfd-bird.txt shows NO incoming packets from tovarisch
→ Root cause: Return path issue
→ Fix: Check veth-stats.txt TX side
```

## Next Step

ACT 3: Assert BGP session establishes after BFD is confirmed Up.
