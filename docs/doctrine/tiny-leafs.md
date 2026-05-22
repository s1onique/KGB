# Tiny Leafs Doctrine

Leaf nodes are expected to run on the cheapest useful machines in strategically useful ASes.

## Leaf service

The leaf service is called `tovarisch`.

`tovarisch` is a constrained Zig daemon responsible for:

- tunnel supervision
- health probes
- signed status reports
- desired-state pull
- last-known-good config
- fallback readiness
- tiny local diagnostics UI

## Hard rules

- No Kubernetes on leafs.
- No containers by default.
- No embedded TSDB.
- No full observability stack.
- No modern web UI requirement.
- No unbounded memory growth.
- No unbounded disk growth.
- No destination logging.
- No framework gravity.

## Local interfaces

Acceptable:

- `tovarisch status`
- `tovarisch doctor`
- localhost-only tiny web UI
- emergency TUI
- local override draft editor

Leaf-local overrides are temporary, explicit, visible, and reported.
