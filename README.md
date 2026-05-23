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
