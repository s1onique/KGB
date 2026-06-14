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

### BGP/BFD Netns Lab (manual only)

```bash
make lab-bgp-bfd
```

Manual CI lab for tovarisch BFD/BGP behavior using Linux network namespaces.

**Important:** This is NOT part of `make gate`. Primary execution is GitHub Actions manual workflow (`workflow_dispatch`). Local Linux execution is optional for debugging only.

To run the lab in GitHub Actions:
1. Go to the Actions tab
2. Select "BGP/BFD Netns Lab"
3. Click "Run workflow"

The lab creates isolated network namespaces with:
- `kgb-lab-tovarisch` namespace (tovarisch daemon)
- `kgb-lab-bird` namespace (BIRD router)
- veth pair connection (10.77.0.1/30 ↔ 10.77.0.2/30)

Artifacts (logs, configs, status) are uploaded automatically on completion or failure.

**Status:** Manual execution only. Will be promoted to scheduled advisory workflow after 3-5 clean manual runs.
