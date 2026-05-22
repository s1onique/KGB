# KGB Doctrine Index

Canonical doctrine documents for KGB project hygiene and architecture.

## Core Doctrine

| Document | Purpose |
|----------|---------|
| [factory.md](./factory.md) | Factory workflow, ACTs, epics, verification |
| [kgb.md](./kgb.md) | KGB architecture, station vs tovarisch split |
| [privacy.md](./privacy.md) | Privacy principles, allowed/forbidden data |
| [tiny-leafs.md](./tiny-leafs.md) | Leaf node constraints |
| [metrics.md](./metrics.md) | Observable metrics philosophy |
| [llm-friendliness.md](./llm-friendliness.md) | Code readability for agents |

## Day-0 Practices

| Document | Purpose |
|----------|---------|
| [day-0-code-coverage.md](./day-0-code-coverage.md) | Coverage philosophy, test-as-signal, tooling rules |

## Quick Reference

- **KGB** observes infrastructure health, not people.
- **`tovarisch`** is the leaf; **`station`** is the control tower.
- Coverage is tracked from Day 0; it is a signal, not a vanity metric.
- Leafs must NOT include: Kubernetes, containers by default, embedded TSDB.
