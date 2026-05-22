# KGB Doctrine Rules

## What KGB Is

- KGB is a lightweight anti-censorship control plane.
- KGB is NOT a VPN protocol.
- KGB is NOT a generic observability stack.
- KGB observes infrastructure health, not people.

## Architecture Split

- **Station**: comfortable Go control tower, may use database, web UI.
- **Tovarisch**: constrained Zig leaf daemon, tiny footprint, brutal constraints.

## Leaf Constraints

Leafs must NOT include:
- Kubernetes
- Containers by default
- Embedded TSDB
- Full observability stack
- Modern web UI requirement
- Unbounded memory/disk growth

## Naming Rules

- `tovarisch` is the leaf daemon; do NOT rename to "agent".
- KGB is the whole system; station/tovarisch are components.

For detailed doctrine, see `docs/doctrine/kgb.md`, `docs/doctrine/privacy.md`, `docs/doctrine/tiny-leafs.md`.
