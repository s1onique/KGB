"""
Canonical ownership mapping for UVB-76 Artifact Secret Hygiene.

This module provides executable proof that every registry rule has an owner.
The ownership mapping is derived from and must agree with the canonical registry.

Each rule ID maps to one or more ownership kinds from the closed vocabulary.

Key invariants:
- OWNERSHIP_ENTRIES is validated as an ordered sequence first.
- RULE_OWNERSHIP is constructed only from an already-valid sequence.
- validate_ownership() iterates entries directly, not a pre-built dict.
"""

from dataclasses import dataclass
from typing import Any

# Closed vocabulary of supported ownership kinds
VALID_OWNERSHIP_KINDS = frozenset({
    "private_key_marker_redactor",
    "typed_header_redactor",
    "request_cookie_redactor",
    "set_cookie_redactor",
    "config_redactor",
    "structured_json_redactor",
    "url_redactor",
    "repository_detection_only",
})


@dataclass(frozen=True)
class OwnershipEntry:
    """Immutable ownership entry for a single rule."""
    rule_id: str
    kinds: tuple[str, ...]


# Canonical ownership entries - defined as tuple of OwnershipEntry for duplicate detection
# Each rule ID maps to its owner kind(s)
OWNERSHIP_ENTRIES: tuple[OwnershipEntry, ...] = (
    # Private key PEM markers (universal scope)
    OwnershipEntry("UVB76-SECRET-0001", ("private_key_marker_redactor",)),
    OwnershipEntry("UVB76-SECRET-0002", ("private_key_marker_redactor",)),
    OwnershipEntry("UVB76-SECRET-0003", ("private_key_marker_redactor",)),
    OwnershipEntry("UVB76-SECRET-0004", ("private_key_marker_redactor",)),
    OwnershipEntry("UVB76-SECRET-0005", ("private_key_marker_redactor",)),

    # Authorization headers (typed_header_redactor)
    OwnershipEntry("UVB76-SECRET-0010", ("typed_header_redactor",)),
    OwnershipEntry("UVB76-SECRET-0011", ("typed_header_redactor",)),
    OwnershipEntry("UVB76-SECRET-0012", ("typed_header_redactor",)),
    OwnershipEntry("UVB76-SECRET-0013", ("typed_header_redactor",)),

    # Session headers
    OwnershipEntry("UVB76-SECRET-0020", ("typed_header_redactor",)),

    # Cookie rules
    OwnershipEntry("UVB76-SECRET-0030", ("request_cookie_redactor",)),
    OwnershipEntry("UVB76-SECRET-0031", ("set_cookie_redactor",)),
    OwnershipEntry("UVB76-SECRET-0032", ("request_cookie_redactor",)),

    # Structured JSON / config field rules
    OwnershipEntry("UVB76-SECRET-0040", ("structured_json_redactor", "config_redactor")),
    OwnershipEntry("UVB76-SECRET-0041", ("structured_json_redactor", "config_redactor")),
    OwnershipEntry("UVB76-SECRET-0050", ("structured_json_redactor", "config_redactor")),
    OwnershipEntry("UVB76-SECRET-0060", ("structured_json_redactor",)),
    OwnershipEntry("UVB76-SECRET-0061", ("structured_json_redactor",)),

    # URL rules
    OwnershipEntry("UVB76-SECRET-0070", ("url_redactor",)),
    OwnershipEntry("UVB76-SECRET-0071", ("url_redactor",)),
    OwnershipEntry("UVB76-SECRET-0072", ("url_redactor",)),

    # Detection-only rules (no active redaction)
    OwnershipEntry("UVB76-SECRET-0080", ("repository_detection_only",)),
    OwnershipEntry("UVB76-SECRET-0081", ("repository_detection_only",)),
)


class OwnershipValidationError(Exception):
    """Raised when ownership validation fails."""
    pass


def validate_ownership(
    entries: tuple[OwnershipEntry, ...] = OWNERSHIP_ENTRIES,
    registry: dict[str, Any] | None = None,
) -> list[str]:
    """
    Validate ownership entries against a registry.

    Args:
        entries: Tuple of OwnershipEntry objects to validate.
        registry: Optional registry dict. If omitted, loads the canonical registry.

    Returns:
        List of error messages (empty if valid).

    Validation checks performed on the sequence before projection:
    - duplicate rule ID in entries
    - empty rule ID
    - empty ownership tuple
    - duplicate ownership kind within one entry
    - unsupported ownership kind
    - unknown registry rule ID
    - missing registry rule ID
    - every registry rule has ownership
    """
    if registry is None:
        from .registry_loader import get_registry
        registry = get_registry()

    errors: list[str] = []
    registry_rule_ids = {r["rule_id"] for r in registry.get("rules", [])}
    ownership_rule_ids: set[str] = set()
    seen_ownership: set[tuple[str, str]] = set()

    # Iterate entries directly - NOT RULE_OWNERSHIP
    for entry in entries:
        rule_id = entry.rule_id
        kinds = entry.kinds

        # Check for empty rule ID
        if not rule_id:
            errors.append("Empty rule ID in ownership entries")
            continue

        # Check for duplicate rule IDs
        if rule_id in ownership_rule_ids:
            errors.append(f"Duplicate ownership entry for rule ID: {rule_id}")
        ownership_rule_ids.add(rule_id)

        # Check for empty ownership tuple
        if not kinds:
            errors.append(f"Empty ownership for rule ID: {rule_id}")
            continue

        # Check each ownership kind
        for kind in kinds:
            # Track (rule_id, kind) pairs for duplicate check
            key = (rule_id, kind)
            if key in seen_ownership:
                errors.append(f"Duplicate ownership kind '{kind}' for rule ID: {rule_id}")
            seen_ownership.add(key)

            # Check ownership kind is supported
            if kind not in VALID_OWNERSHIP_KINDS:
                errors.append(f"Unknown ownership kind '{kind}' for rule ID: {rule_id}")

    # Check that every registry rule has ownership
    missing_ownership = registry_rule_ids - ownership_rule_ids
    if missing_ownership:
        errors.append(f"Missing ownership for registry rules: {sorted(missing_ownership)}")

    # Check for unknown rule IDs in ownership (not in registry)
    unknown_rules = ownership_rule_ids - registry_rule_ids
    if unknown_rules:
        errors.append(f"Unknown rule IDs in ownership: {sorted(unknown_rules)}")

    return errors


def build_rule_ownership(
    entries: tuple[OwnershipEntry, ...],
) -> dict[str, tuple[str, ...]]:
    """
    Build a rule ownership dict from ownership entries with validation.

    Args:
        entries: Tuple of OwnershipEntry objects to build the mapping from.

    Returns:
        Dict mapping rule IDs to their ownership kinds.

    Raises:
        OwnershipValidationError: If validation fails.

    Contract:
        Validation occurs BEFORE dictionary projection.
        This function will never silently overwrite entries.
    """
    errors = validate_ownership(entries)
    if errors:
        raise OwnershipValidationError(errors)

    result: dict[str, tuple[str, ...]] = {}
    for entry in entries:
        result[entry.rule_id] = entry.kinds
    return result


# Canonical ownership mapping (dict form for backwards compatibility)
# Built using the checked helper - validation occurs before projection
RULE_OWNERSHIP: dict[str, tuple[str, ...]] = build_rule_ownership(OWNERSHIP_ENTRIES)


def get_ownership_for_rule(rule_id: str) -> list[str]:
    """Get the ownership kinds for a rule ID."""
    return RULE_OWNERSHIP.get(rule_id, [])


def get_all_ownership_rule_ids() -> set[str]:
    """Get all rule IDs that have ownership."""
    return set(RULE_OWNERSHIP.keys())


def count_ownership_entries() -> int:
    """Count total ownership entries (sum of all ownership kinds)."""
    return sum(len(kinds) for kinds in RULE_OWNERSHIP.values())


def count_ownership_assignments() -> int:
    """
    Count total ownership assignments (sum of all ownership kinds).

    This is the canonical count function.
    count_ownership_entries() is kept as a compatibility alias.
    """
    return count_ownership_entries()


def count_unique_rules() -> int:
    """Count unique rule IDs with ownership."""
    return len(RULE_OWNERSHIP)
