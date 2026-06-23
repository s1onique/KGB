# Memory Lab Artifact Schema — KGB Doctrine

**URI**: `kgb://doctrine/memory-lab-artifact-schema`

Every memory lab MUST produce artifacts that conform to this schema. The schema ensures consistent memory evidence collection across tovarisch and uvb76 labs.

## Version

Current schema version: `1.0`

## Artifact Files

Each memory lab produces a directory of artifacts:

```
lab-result.json           # Required: Final lab summary with pass/fail
memory-snapshots.json     # Optional: Time-series RSS/PSS snapshots
runtime-stats.json        # Optional: Runtime-specific stats (Go: goroutines, GC, etc.)
workload-log.json         # Optional: Detailed operation log
```

## lab-result.json Schema

```json
{
  "schema_version": "1.0",
  "service": {
    "name": "string (required) - e.g., 'tovarisch' or 'uvb76'",
    "version": "string (required) - e.g., '1.2.3+abc1234'",
    "commit": "string (required) - git commit hash or 'unknown'"
  },
  "environment": {
    "arch": "string (required) - e.g., 'linux/arm64', 'darwin/amd64'",
    "kernel": "string (optional) - e.g., '5.15.0-generic'",
    "os": "string (optional) - e.g., 'Linux', 'Darwin'"
  },
  "workload": {
    "type": "string (required) - e.g., 'status-json-warmup', 'icmp-probe-loop'",
    "operations": "integer (required) - total operations performed",
    "errors": "integer (required) - total errors encountered",
    "duration_ms": "integer (required) - total wall-clock time in ms",
    "interval_ms": "integer (optional) - operation interval in ms"
  },
  "memory": {
    "first": {
      "rss_kib": "integer (required) - RSS at start in KiB",
      "pss_kib": "integer (optional) - PSS at start in KiB"
    },
    "max": {
      "rss_kib": "integer (required) - RSS peak in KiB",
      "pss_kib": "integer (optional) - PSS peak in KiB"
    },
    "last": {
      "rss_kib": "integer (required) - RSS at end in KiB",
      "pss_kib": "integer (optional) - PSS at end in KiB"
    },
    "growth": {
      "rss_kib": "integer (required) - last RSS minus first RSS",
      "pss_kib": "integer (optional) - last PSS minus first PSS",
      "rss_percent": "float (required) - growth percentage"
    }
  },
  "runtime": {
    "goroutines": "integer (optional, Go only) - peak goroutine count",
    "gc_count": "integer (optional, Go only) - GC cycles observed",
    "heap_alloc_bytes": "integer (optional) - peak heap alloc",
    "gc_pause_ns": "integer (optional, Go only) - peak GC pause in ns",
    "zig_allocator": "string (optional, Zig only) - allocator type used"
  },
  "decision": {
    "pass": "boolean (required) - true if within budget",
    "reason": "string (required) - explanation of pass/fail",
    "budget_checked": "string (optional) - which budget was checked",
    "budget_value": "string (optional) - measured vs budget"
  }
}
```

## memory-snapshots.json Schema (Optional)

```json
{
  "snapshots": [
    {
      "timestamp_ms": "integer - relative time in ms from lab start",
      "rss_kib": "integer - RSS at this point",
      "pss_kib": "integer (optional) - PSS at this point",
      "goroutines": "integer (optional, Go only)",
      "heap_alloc_bytes": "integer (optional)"
    }
  ]
}
```

## Example Artifact

```json
{
  "schema_version": "1.0",
  "service": {
    "name": "tovarisch",
    "version": "0.1.0+abc1234",
    "commit": "abc1234"
  },
  "environment": {
    "arch": "linux/arm64",
    "kernel": "5.15.0-generic",
    "os": "Linux"
  },
  "workload": {
    "type": "status-json-warmup",
    "operations": 100,
    "errors": 0,
    "duration_ms": 10000,
    "interval_ms": 100
  },
  "memory": {
    "first": {
      "rss_kib": 2048,
      "pss_kib": 1900
    },
    "max": {
      "rss_kib": 2100,
      "pss_kib": 1950
    },
    "last": {
      "rss_kib": 2060,
      "pss_kib": 1910
    },
    "growth": {
      "rss_kib": 12,
      "pss_kib": 10,
      "rss_percent": 0.59
    }
  },
  "runtime": {
    "zig_allocator": "ArenaAllocator"
  },
  "decision": {
    "pass": true,
    "reason": "RSS growth 0.59% < 5% threshold",
    "budget_checked": "status_json_warmup",
    "budget_value": "12 KiB < 100 KiB budget"
  }
}
```

## Verifier

Use `scripts/verify_memory_lab_artifact.py` to validate artifacts against this schema.

## Evidence Classes

Memory lab artifacts are classified by signal quality:

### Short-Window Evidence (warmup_sensitive)

- **Window**: 10 seconds
- **signal_quality**: `warmup_sensitive`
- **Use**: Quick CI feedback, not definitive leak proof
- **Limitations**: Warmup effects dominate the signal

### Long-Window Evidence (long_window)

- **Window**: >= 900 seconds (15+ minutes)
- **signal_quality**: `long_window`
- **Use**: Durable leak-slope evidence with warmup settled
- **Requirements**:
  - operations >= 9000
  - duration_seconds >= 900
  - request_errors == 0

### Evidence Preserved Location

Long-window evidence from workflow run 28024460972 is preserved at:
```
docs/evidence/memory-lab/run-28024460972/
```

## See Also

- `kgb://doctrine/embedded-memory-frugality` — Memory doctrine
- `docs/memory/budgets/` — Memory budget YAML files
