# KGB Doctrine

KGB is a lightweight anti-censorship control plane.

It exists to keep reliable connectivity available for trusted people across hostile or degraded networks.

## Non-goals

- KGB is not a VPN protocol.
- KGB is not a Kubernetes platform.
- KGB is not an enterprise audit system.
- KGB is not a user-activity monitoring system.
- KGB is not a generic observability stack.

## Design center

KGB answers:

- Which escape routes are alive?
- Which tunnel backend currently works?
- Which fallback is ready?
- Which config is active?
- Which UVB-76s are reachable?
- Can trusted users still reach the outside?

## Core split

UVB-76s may be civilized.

Leafs must survive barbarism.

## Transport doctrine

Transport backends are replaceable.

Initial candidates:

- WireGuard
- AmneziaWG
- Shadowsocks/Outline-compatible fallback
- Xray/REALITY emergency fallback

KGB manages and verifies transports. It does not initially implement transport cryptography itself.
