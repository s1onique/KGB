# Components

## station

Language: Go

Runs on:

- home infrastructure
- trusted VPS
- stronger machines

Responsibilities:

- HTTP API
- dashboard
- report collection
- desired-state signing
- station federation
- history storage

## tovarisch

Language: Zig

Runs on:

- tiny VPSes
- constrained machines
- useful AS placements
- family-side or relay-side nodes

Responsibilities:

- tunnel supervision
- probes
- signed reports
- desired-state pull
- local fallback control
- tiny UI/status

## kgbctl

Language: TBD

Operator CLI for:

- enrollment
- status inspection
- desired-state authoring
- diagnostics
- emergency operations
