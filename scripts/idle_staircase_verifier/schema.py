# schema.py — Constants and types for idle staircase artifact verification
"""Schema constants for verifier artifact validation."""

from typing import Final

# Required files in artifact directory
REQUIRED_FILES: Final[list[str]] = [
    "manifest.yaml",
    "memory_samples.tsv",
    "event_timeline.tsv",
    "verdict.txt",
]

# Required files for native event artifacts (when native_events_enabled=true)
NATIVE_ARTIFACT_FILES: Final[list[str]] = [
    "native_event_timeline.tsv",
]

# Valid verdict values
VALID_VERDICTS: Final[list[str]] = [
    "confirmed_leak",
    "bounded_warmup_or_allocator_highwater",
    "inconclusive",
]

# Required columns for event_timeline.tsv
REQUIRED_EVENT_COLS: Final[list[str]] = ["timestamp", "elapsed_sec", "event", "subsystem"]

# Terminal events that end the lab
TERMINAL_EVENTS: Final[list[str]] = ["idle_complete", "shutdown", "lab_failed"]

# Event prefixes by subsystem
SYNTHETIC_EVENT_PREFIXES: Final[list[str]] = ["heartbeat_", "wg_", "bgp_", "bfd_"]

# Memory sample step threshold (KiB)
STEP_THRESHOLD_KIB: Final[int] = 50

# Confirmed leak thresholds
CONFIRMED_LEAK_MIN_STEPS: Final[int] = 3
CONFIRMED_LEAK_MIN_GROWTH_KIB: Final[int] = 500

# Bounded verdict rejection thresholds
BOUNDED_MAX_GROWTH_KIB: Final[int] = 2000
BOUNDED_MAX_STEPS: Final[int] = 10

# Required verdict fields
REQUIRED_VERDICT_FIELDS: Final[list[str]] = [
    "verdict:",
    "steps_detected:",
    "total_growth_kib:",
    "growth_rate_kib_per_min:",
]

# Owner prefix to event prefix mapping
OWNER_TO_PREFIXES: Final[dict[str, list[str]]] = {
    "heartbeat": ["heartbeat_", "heartbeat"],
    "wireguard": ["wg_", "wg"],
    "bgp": ["bgp_", "bgp"],
    "bfd": ["bfd_", "bfd"],
}

# Subsystem counts keys
SUBSYSTEM_KEYS: Final[list[str]] = [
    "heartbeat",
    "wireguard",
    "bgp",
    "bfd",
    "health",
    "status",
]
