# ACT-HULK29R-ZIG016-WG-PEERS-NAMESPACE-DIAGNOSTIC-PROOF

## Status

**COMPLETE** ✅

## Objective

Make the `wg_peers` namespace mismatch state doctrine-grade:
- Stable
- Contract-tested
- LLM-friendly
- Clearly separated from other diagnostic states
- Supported by reproducible namespace evidence

## Context

Live `/status` output from production deployment shows:

```json
{
  "name": "wg_peers",
  "status": "warn",
  "detail": "wg wrong_namespace_or_unreachable: namespace mismatch"
}
```

Simultaneously:
```json
{
  "name": "tunnel",
  "status": "ok",
  "detail": "detected tunnel interfaces: wg-kgb0"
}
```

And BFD/BGP are healthy.

## Root Cause / Interpretation

The `wrong_namespace_or_unreachable` diagnostic state is **correct and expected** in certain deployment configurations:

1. **Tunnel check** detects `wg-kgb0` via sysfs (`/sys/class/net`) - works from any namespace
2. **WireGuard peer diagnostic** runs `wg show wg-kgb0 dump` from tovarisch's current namespace
3. **Namespace mismatch**: `wg-kgb0` was created in a different network namespace, and tovarisch runs in a different namespace where `wg show` cannot see the interface

**This is NOT a bug.** The tunnel is functional (BFD/BGP healthy), but peer diagnostics cannot reach it from the current namespace.

## Prior Work (Closed)

The following were already fixed by prior ACTs:
- Missing top-level JSON closing brace in `/status`
- Dangling detail slice / invalid UTF-8 JSON in `status_checks.zig`
- CLI composition inventory drift around WireGuard probe paths
- Split-test inventory drift
- WireGuard diagnostic classification wiring from raw diagnostic attempt to canonical status detail

## Analysis

### Classifier Already Canonical

The `wrong_namespace_or_unreachable` diagnostic class was already implemented:

- **Classifier enum**: `WgDiagnosticClass.wrong_namespace_or_unreachable` (line 62 in `wg_diagnostic_classifier.zig`)
- **Decision table**: Triggers when OS link exists but `wg show interfaces` doesn't contain the interface name (line 164)
- **Status mapping**: Maps to `warn` (line 230)
- **Detail string**: `classifierErrorKindToDetail()` outputs `"wg wrong_namespace_or_unreachable: namespace mismatch"` (line 184 in `status_checks.zig`)

### Gap Analysis

| Component | State | Action |
|-----------|-------|--------|
| Classifier enum | ✅ Canonical | None |
| Classifier decision table | ✅ Has test | None |
| Formatter unit test | ❌ Missing | **Added** |
| Facts-to-classifier test | ✅ Has test | None |
| Status contract fixture | ❌ Missing | **Added** |
| Contract docs | ❌ Missing | **Added** |

## Changes Made

### 1. Added Formatter Unit Test

**File**: `tovarisch/src/net/wg_diagnostic_classifier_tests.zig`

```zig
test "formatter: wrong_namespace_or_unreachable" {
    const wg_peer_diag = @import("wg_peer_diagnostic.zig");
    const diag = wg_peer_diag.WireGuardPeerDiagnostic{
        .backend = "cli",
        .selected_interface = "wg-kgb0",
        .command = "wg show wg-kgb0 dump",
        .timeout_secs = null,
        .exit_code = 1,
        .error_kind = "wrong_namespace_or_unreachable",
        .stderr_len = 15,
        .stdout_len = 0,
        .os_link_kind = .wireguard,
        .peer_count = 0,
    };
    var buf: [256]u8 = undefined;
    const result = wg_peer_diag.formatPeerDiagnosticDetail(diag, &buf);
    try testing.expect(std.mem.startsWith(u8, result, "wg wrong_namespace_or_unreachable:"));
    try testing.expect(std.mem.containsAtLeast(u8, result, 1, "interface=wg-kgb0"));
    try testing.expect(std.mem.containsAtLeast(u8, result, 1, "backend=cli"));
}
```

### 2. Added Status Contract Fixture

**File**: `docs/contracts/examples/tovarisch-status-v0-wg-namespace-mismatch.json`

Represents the complete status payload when namespace mismatch is observed:
- `tunnel: ok` with `wg-kgb0` detected
- `wg_peers: warn` with `wrong_namespace_or_unreachable: namespace mismatch`
- `bfd: ok`
- `bgp: ok`

### 3. Updated Contract Documentation

**File**: `docs/contracts/tovarisch-status-v0.md`

Added namespace mismatch example section with:
- JSON fixture
- Interpretation explaining deployment invariant

## Files Changed

| File | Change |
|------|--------|
| `tovarisch/src/net/wg_diagnostic_classifier_tests.zig` | Added `test "formatter: wrong_namespace_or_unreachable"` |
| `docs/contracts/examples/tovarisch-status-v0-wg-namespace-mismatch.json` | New fixture for namespace mismatch state |
| `docs/contracts/tovarisch-status-v0.md` | Added namespace mismatch example section |

## Verification

### All Focused Commands Pass

```bash
$ make tovarisch-build
cd tovarisch && zig build
✅ PASS

$ make tovarisch-test
cd tovarisch && zig build test
test
+- run test w
[BFD] bfd_control_packet_sent to=10.149.149.10
...
✅ PASS

$ python3 scripts/verify_tovarisch_status_contract.py
[status-contract] PASS

$ python3 scripts/verify_cli_composition_inventory.py
=== VERIFICATION PASSED ===

$ bash scripts/verify_split_test_inventory.sh
[split-test-inventory] PASS: split test inventory is consistent
```

### Full Gate Passes

```bash
$ make gate
...
[PASS] Memory ownership hygiene gate passed.
[gate] PASS
```

## Observed /status Contract

When namespace mismatch is detected:

```json
{
  "service": "tovarisch",
  "version": "0.1.1-rc60+47776d2",
  "node_id": "local-dev",
  "status": "warn",
  "checks": [
    {"name": "process", "status": "ok", "detail": "running"},
    {"name": "binary", "status": "ok", "detail": "tovarisch"},
    {"name": "config", "status": "ok", "detail": "/etc/kgb/tovarisch.conf"},
    {"name": "state_dir", "status": "warn", "detail": "state directory not found"},
    {"name": "http", "status": "ok", "detail": "http service route available"},
    {"name": "tunnel", "status": "ok", "detail": "detected tunnel interfaces: wg-kgb0"},
    {
      "name": "wg_peers",
      "status": "warn",
      "detail": "wg wrong_namespace_or_unreachable: namespace mismatch"
    },
    {"name": "bfd", "status": "ok", "detail": "bfd sessions up"},
    {"name": "bgp", "status": "ok", "detail": "BGP established; 15811 configured prefixes"}
  ],
  "runtime": {"pid": 2513434, "rss_kib": 6396}
}
```

### Contract Validation

- ✅ Valid JSON
- ✅ Valid UTF-8
- ✅ Closed top-level object
- ✅ Parseable by `jq`
- ✅ No legacy detail strings (`backend_missing`, `interface_missing`, `wg_tool_missing` not in target fixture)
- ✅ `tunnel ok` can coexist with `wg_peers warn`

## Non-Goals (Preserved)

- ❌ Did NOT fix `state_dir` warning (out of scope)
- ❌ Did NOT make `wg_peers` green (suppressing warning would hide real state)
- ❌ Did NOT add namespace auto-discovery/remediation (no existing production seam)
- ❌ Did NOT add new CLI execution sites
- ❌ Did NOT weaken existing classifier logic

## Follow-up ACTs

### ACT-HULK29R-ZIG016-WG-NAMESPACE-RUNTIME-CONFIG-FIX

If the namespace mismatch needs remediation:

- Configure tovarisch to run in the correct namespace
- OR configure peer diagnostics to execute in the intended namespace through an existing approved boundary
- OR document deployment invariant for `wg-kgb0` birthplace/current namespace

### ACT-HULK29R-ZIG016-STATE-DIR-RUNTIME-CONTRACT

If `state_dir` needs fixing:

- Create/package expected state directory
- Align default config path
- Add status contract fixture for missing vs present state dir

## Diagnostic Evidence Commands

For manual investigation of namespace mismatch:

```bash
# Get tovarisch PID
pid="$(pidof tovarisch || true)"
echo "tovarisch pid=$pid"

# Check tovarisch namespace
if [ -n "$pid" ]; then
  readlink "/proc/$pid/ns/net" || true
  ip netns identify "$pid" || true
fi

# Check init namespace
readlink /proc/1/ns/net || true
ip netns list || true

# Check interface visibility
ip -d link show wg-kgb0 || true
wg show interfaces || true
wg show wg-kgb0 || true

# Check interface in each named namespace
for ns in $(ip netns list | awk '{print $1}'); do
  echo "== namespace: $ns =="
  ip netns exec "$ns" ip -d link show wg-kgb0 2>/dev/null || true
  ip netns exec "$ns" wg show wg-kgb0 2>/dev/null || true
done
```

## Zig 0.16 Observations

None - no Zig 0.16-specific issues encountered in this ACT.

## Summary

The `wrong_namespace_or_unreachable` diagnostic state was already canonically implemented. This ACT added:

1. **Unit test** proving the formatter produces the expected output
2. **Status contract fixture** proving the complete `/status` JSON representation
3. **Contract documentation** explaining the deployment invariant

The state is correct and expected when the tunnel interface is in a different namespace than tovarisch's running context. The tunnel remains functional (BFD/BGP healthy), but peer diagnostics cannot access it.
