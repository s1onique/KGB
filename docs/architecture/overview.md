# Architecture Overview

KGB consists of control towers and leaf nodes.

## Station

A station is a control tower.

Stations run on trusted or home infrastructure and may use Go, a database, and a denser web UI.

Responsibilities:

- receive signed reports
- sign desired state
- manage node inventory
- show operational dashboard
- synchronize with peer stations
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

Stations do not require inbound access to leafs.

Desired state is signed.

Reports are signed.

Last-known-good config must survive bad desired state.
