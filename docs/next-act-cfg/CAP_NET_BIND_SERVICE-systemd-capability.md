# Next ACT: Grant CAP_NET_BIND_SERVICE in packaged systemd unit

You are working in the KGB/tovarisch repository.

## ACT: Grant CAP_NET_BIND_SERVICE in packaged systemd unit

### Goal

Allow the non-root `tovarisch` systemd service user to bind privileged BGP TCP/179 once the always-on passive BGP listener is implemented.

### Context

- `tovarisch` currently runs as a dedicated non-root service user, not root.
- Live process example:
  `tovarisch ... /usr/bin/tovarisch serve --config /etc/kgb/tovarisch.conf`
- We are going to make BGP always do both:
  - active outbound connect to configured peer
  - passive listen on configured `local_address:179`
- Binding TCP ports below 1024 as non-root requires `CAP_NET_BIND_SERVICE`.
- **Do NOT use `setcap` on `/usr/bin/tovarisch`**; package upgrades can lose file capabilities and it grants the cap to anyone executing the binary.
- Use the packaged systemd unit instead.

### Requirements

1. Find the packaged systemd unit source at `packaging/systemd/tovarisch.service`.
2. Keep the daemon running as the dedicated `tovarisch` user/group (already present).
3. Add the minimal capability grant:
   - `AmbientCapabilities=CAP_NET_BIND_SERVICE`
   - `CapabilityBoundingSet=CAP_NET_BIND_SERVICE`
   - `NoNewPrivileges=true` is already present — keep it.
4. Do not grant broad capabilities — only `CAP_NET_BIND_SERVICE`.
5. Do not run the service as root.
6. Do not add active/passive BGP config modes in this ACT.
7. Update `scripts/verify_deb_systemd_package.sh` so the packaged unit is checked for the capability lines:
   - Change from `check_required 'CapabilityBoundingSet=' ...` to check for `CapabilityBoundingSet=CAP_NET_BIND_SERVICE`
   - Change from `check_required 'AmbientCapabilities=' ...` to check for `AmbientCapabilities=CAP_NET_BIND_SERVICE`
8. Update `docs/packaging/debian-systemd.md`:
   - In the "Hardening Features" table, document `CAP_NET_BIND_SERVICE` with explanation that it allows binding BGP TCP/179 as non-root.
   - Add a "Why systemd capabilities not file setcap?" subsection explaining the rationale.
9. Keep the diff surgical and LLM-friendly.

### Current systemd unit excerpt (lines 26-27)

```
CapabilityBoundingSet=
AmbientCapabilities=
```

### Acceptance Criteria

- Packaged systemd unit contains:
  - `User=tovarisch`
  - `Group=tovarisch`
  - `AmbientCapabilities=CAP_NET_BIND_SERVICE`
  - `CapabilityBoundingSet=CAP_NET_BIND_SERVICE`
  - `NoNewPrivileges=true`
- Existing `ExecStart=/usr/bin/tovarisch serve --config /etc/kgb/tovarisch.conf` behavior is preserved.
- `scripts/verify_deb_systemd_package.sh` fails if the capability lines are missing.
- `docs/packaging/debian-systemd.md` mentions privileged BGP TCP/179 binding and explains why systemd capabilities are used instead of file `setcap`.
- `make tovarisch-test` passes.
- `make gate` passes.

### Important

- This ACT only grants the service capability. It does NOT implement the passive BGP listener itself.
- After this ACT, the next ACT will implement always-on passive BGP listener alongside active connect.
- Keep the change small. No unrelated formatting, cleanup, or refactoring.

### Files to Change

1. `packaging/systemd/tovarisch.service` — add capability lines
2. `scripts/verify_deb_systemd_package.sh` — update capability checks
3. `docs/packaging/debian-systemd.md` — document the capability and rationale

### Before Closing

- Show changed files.
- Show verification output (`make gate`, `make tovarisch-test`).
- Report whether any Zig 0.16 lessons were discovered; if none, say none.

---

## Next ACT (for after this one completes)

**Prompt for passive BGP listener ACT:**

You are working in the KGB/tovarisch repository.

## ACT: Implement always-on passive BGP TCP/179 listener alongside active connect

### Goal

Add a passive TCP/179 listener alongside the existing active outbound BGP connection. The `tovarisch` daemon will accept incoming BGP peer connections on a configured `local_address:179` in addition to initiating outbound connections.

### Context

- The previous ACT granted `CAP_NET_BIND_SERVICE` via systemd so the service can bind port 179 as non-root `tovarisch` user.
- The current `bgp` module has active connect logic.
- The passive listener should be always-on once configured.
- Both active and passive connections share the same BGP session state machine.

### Requirements

1. Verify existing `[bgp].local_address` parsing and runtime propagation.
   - If already supported, do not rewrite config parsing.
   - If not fully propagated into the BGP runtime, make the smallest change needed.
   - The passive listener binds to the configured `local_address` on TCP/179.
2. Extend the BGP runtime to:
   - Start a passive TCP server listening on `local_address:179`.
   - Accept incoming peer connections and hand them to the existing session management.
   - The passive listener is always-on when `local_address` is configured.
3. Keep the existing active connect behavior — both can be active simultaneously.
4. Update status reporting to include passive listener state.
5. Add tests for passive listener acceptance and integration.
6. Update `docs/configuration/tovarisch-config.md` to document `local_address` in the `[bgp]` section.
7. Do not change systemd unit further — capability was already granted in the previous ACT.

### Acceptance Criteria

- When `local_address` is configured in `[bgp]`, `tovarisch` listens on that address port 179.
- Incoming BGP connections are accepted and processed.
- Existing active connect behavior is preserved.
- Tests pass.
- Documentation is updated.
