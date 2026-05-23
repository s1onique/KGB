# Architecture Overview

KGB consists of control towers and leaf nodes.

## UVB-76

UVB-76 is the control tower.

UVB-76 runs on trusted or home infrastructure and may use Go, a database, and a denser web UI.

Responsibilities:

- receive signed reports
- sign desired state
- manage node inventory
- show operational dashboard
- synchronize with peer UVB-76s
- manage transport configuration

## Tovarisch

`tovarisch` is the leaf daemon.

It runs on tiny remote machines and should be implemented in Zig.

Responsibilities:

- pull signed desired state
- supervise transport backends
- run health probes
- keep last-known-good config
- report signed status
- expose minimal local diagnostics

## Control model

Leafs pull.

UVB-76s do not require inbound access to leafs.

Desired state is signed.

Reports are signed.

Last-known-good config must survive bad desired state.
