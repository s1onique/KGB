# Privacy Doctrine

KGB observes infrastructure health, not people.

## Allowed

KGB may record:

- node identity
- transport state
- last handshake age
- UVB-76 reachability
- probe success/failure
- fallback readiness
- config version
- clock skew
- route correctness
- bounded diagnostic events

## Forbidden

KGB must not record:

- browsing history
- visited domains
- destination IP flow logs
- message contents
- per-user behavioral timelines
- unnecessary personal data

## Principle

Connectivity facts are allowed.

Human behavior surveillance is forbidden.
