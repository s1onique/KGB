"""
Rules module for UVB-76 Artifact Secret Hygiene.

Derives rule definitions from the canonical registry.
Registry path: registry.json
"""

import re

from .registry_loader import get_registry, get_universal_rules, get_artifact_context_rules


# ============================================================================
# Fragment helper - breaks strings to avoid containing complete secret markers
# ============================================================================

def _FRAG(s: str) -> str:
    """Create a non-secret-shaped fragment for bootstrap safety."""
    if len(s) > 4:
        mid = len(s) // 2
        return s[:mid] + s[mid+1:]
    return s


# Build PEM markers at runtime from fragments to avoid self-rejection
def _build_pem_marker(parts: list) -> str:
    """Build a PEM marker from parts at runtime."""
    dashes = "-----"
    space = " "
    return dashes + space.join(parts) + dashes


# Fragmented parts for bootstrap safety
_BEGIN = "BEGIN"
_PRIVATE = "PRIVATE"
_ENCRYPTED = "ENCRYPTED"
_RSA = "RSA"
_EC = "EC"
_OPENSSH = "OPENSSH"
_KEY = "KEY"

# Build patterns at module load time
_PRIVATE_KEY = _build_pem_marker([_BEGIN, _PRIVATE, _KEY])
_ENCRYPTED_KEY = _build_pem_marker([_BEGIN, _ENCRYPTED, _PRIVATE, _KEY])
_RSA_KEY = _build_pem_marker([_BEGIN, _RSA, _PRIVATE, _KEY])
_EC_KEY = _build_pem_marker([_BEGIN, _EC, _PRIVATE, _KEY])
_OPENSSH_KEY = _build_pem_marker([_BEGIN, _OPENSSH, _PRIVATE, _KEY])


# ============================================================================
# Registry-derived Universal Rules
# Loaded once at module import for performance.
# ============================================================================

_registry = get_registry()

# Universal rules - applied to ALL relevant tracked files
UNIVERSAL_RULES: list[dict] = []
for rule in get_universal_rules(_registry):
    # Skip rules without patterns (shouldn't happen for universal rules)
    if "pattern" not in rule:
        continue
    UNIVERSAL_RULES.append({
        "id": rule["rule_id"],
        "class": rule["class"],
        "pattern": re.compile(rule["pattern"]),
        "explanation": rule["safe_explanation"],
        "remediation": rule["safe_remediation"],
    })

# Artifact context rules - applied based on artifact type/sensitivity
ARTIFACT_CONTEXT_RULES: list[dict] = []
for rule in get_artifact_context_rules(_registry):
    rule_id = rule["rule_id"]
    detector_kind = rule.get("detector_kind", "")

    if detector_kind == "pattern":
        # Line-oriented pattern detection
        pattern = rule.get("pattern", "")
        if pattern:
            ARTIFACT_CONTEXT_RULES.append({
                "id": rule_id,
                "class": rule["class"],
                "pattern": re.compile(pattern),
                "explanation": rule["safe_explanation"],
                "remediation": rule["safe_remediation"],
                "detector_kind": detector_kind,
            })

    elif detector_kind == "header_name":
        # Header-based pattern detection
        header_pattern = rule.get("header_pattern", "")
        if header_pattern:
            ARTIFACT_CONTEXT_RULES.append({
                "id": rule_id,
                "class": rule["class"],
                "pattern": re.compile(header_pattern, re.IGNORECASE),
                "explanation": rule["safe_explanation"],
                "remediation": rule["safe_remediation"],
                "detector_kind": detector_kind,
            })

    elif detector_kind == "url_component":
        # URL component detection
        pattern = rule.get("pattern", "")
        if pattern:
            ARTIFACT_CONTEXT_RULES.append({
                "id": rule_id,
                "class": rule["class"],
                "pattern": re.compile(pattern),
                "explanation": rule["safe_explanation"],
                "remediation": rule["safe_remediation"],
                "detector_kind": detector_kind,
            })

    # field_name and structured_json detectors are handled by structured scanning

# Query parameter rules loaded at module init (derived from registry)
# These are used by structured_scanner for URL-based scanning
QUERY_PARAM_RULES: dict[str, tuple[set[str], str, str]] = {}

# Load query parameter rules from registry
for rule in get_artifact_context_rules(_registry):
    if rule.get("detector_kind") == "url_component":
        query_params = rule.get("query_params", [])
        if query_params:
            QUERY_PARAM_RULES[rule["rule_id"]] = (
                {p.lower() for p in query_params},
                rule["safe_explanation"],
                rule["safe_remediation"],
            )


# ============================================================================
# Test helpers (for self-test fixtures without storing literals)
# ============================================================================

def build_test_private_key() -> str:
    """Build a test private key marker for self-test fixtures (not stored as literal)."""
    return _PRIVATE_KEY


def build_test_rsa_key() -> str:
    """Build a test RSA key marker for self-test fixtures (not stored as literal)."""
    return _RSA_KEY


def build_test_encrypted_key() -> str:
    """Build a test encrypted private key marker for self-test fixtures."""
    return _ENCRYPTED_KEY


def build_test_ec_key() -> str:
    """Build a test EC key marker for self-test fixtures."""
    return _EC_KEY


def build_test_openssh_key() -> str:
    """Build a test OpenSSH key marker for self-test fixtures."""
    return _OPENSSH_KEY


def build_test_certificate() -> str:
    """Build a test public certificate marker (NOT a private key)."""
    return _build_pem_marker([_BEGIN, "CERTIFICATE"])
