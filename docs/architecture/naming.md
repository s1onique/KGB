# Naming

## Project

`KGB` is the whole system.

## Components

- `UVB-76`: control tower (the future station/control endpoint)
- `tovarisch`: constrained leaf service
- `kgbctl`: operator CLI

## Naming Doctrine

**UVB-76** is the memorable, distinctive name for the control tower/control endpoint component.

- Use `UVB-76` in prose and user-facing docs
- Use `uvb76` in code identifiers, config keys, filenames if needed
- Do NOT use generic terms like "station" or "control-plane" in new documentation

**tovarisch** is the leaf daemon/service name. It stays as-is.

- `tovarisch` is not a generic agent
- It is the leaf comrade that keeps escape routes alive
- Do not call the leaf service `agent` in code or docs except when explaining its role generically

### Why UVB-76?

The name is weird and memorable — perfect for a mysterious control tower that keeps escape routes alive. It evokes the Cold War listener stations of folklore while staying operator-log friendly. Runtime messages may use 🚩📻 as the UVB-76 signal marker.
