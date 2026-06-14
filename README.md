# KGB

KGB is a lightweight anti-censorship control plane for keeping escape routes alive across hostile, degraded, or unreliable networks.

KGB is not a VPN protocol. It is a control plane and supervision layer for transport backends.

## Core doctrine

- UVB-76s may be comfortable. Leafs must be brutal.
- `tovarisch` is the constrained leaf service.
- KGB observes infrastructure health, not people.
- Transport backends are swappable.
- Nodes pull desired state; UVB-76s do not require inbound reachability to leafs.
- Last-known-good config must survive bad control-plane decisions.
- Generic observability is insufficient; KGB tracks escape-route vital signs.

## Components

- `UVB-76`: Go-based control tower running on trusted/home infrastructure.
- `tovarisch`: Zig-based leaf daemon for tiny remote machines.
- `kgbctl`: operator CLI.

## CI / Automation

### Fast Gate (blocking)

```bash
make gate
```

Runs quality checks that must pass before any merge.

### Scheduled Health Audit (advisory)

```bash
make health-audit
```

Advisory health checks run weekly via GitHub Actions (Monday 06:17 UTC) or manually via `workflow_dispatch`.

**Note:** Advisory audit findings do not replace `make gate`. The health audit is for visibility and trend monitoring only.

Checks include:
- Required documentation presence (including AI-native axioms doc)
- Agent configuration (.clinerules) presence
- Core source structure
- Zig package validity
- Naming compliance (forbidden generic naming)
- Documentation freshness (git-history based)
- Stable-reference hygiene (forbidden chat-memory patterns)
- LLM-friendliness gate reuse
- Memory ownership hygiene gate reuse
- Git state
