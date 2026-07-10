"""
Rules module for UVB-76 Artifact Secret Hygiene.

Defines canonical rule registry and rule sets.
"""

import re

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
# Canonical Rule Registry (authoritative mapping)
# All languages must agree on rule IDs and meanings.
# ============================================================================

RULE_REGISTRY = {
    # Universal Critical Rules (applied across all relevant tracked files)
    "UVB76-SECRET-0001": {
        "class": "private_key_pem",
        "scope": "universal",
        "severity": "critical",
        "allowlistable": False,
        "explanation": "private key PEM block detected",
        "remediation": "Remove or replace with [REDACTED]",
        "pattern": _PRIVATE_KEY,
    },
    "UVB76-SECRET-0002": {
        "class": "encrypted_private_key_pem",
        "scope": "universal",
        "severity": "critical",
        "allowlistable": False,
        "explanation": "encrypted private key PEM block detected",
        "remediation": "Remove or replace with [REDACTED]",
        "pattern": _ENCRYPTED_KEY,
    },
    "UVB76-SECRET-0003": {
        "class": "rsa_private_key_pem",
        "scope": "universal",
        "severity": "critical",
        "allowlistable": False,
        "explanation": "RSA private key PEM block detected",
        "remediation": "Remove or replace with [REDACTED]",
        "pattern": _RSA_KEY,
    },
    "UVB76-SECRET-0004": {
        "class": "ec_private_key_pem",
        "scope": "universal",
        "severity": "critical",
        "allowlistable": False,
        "explanation": "EC private key PEM block detected",
        "remediation": "Remove or replace with [REDACTED]",
        "pattern": _EC_KEY,
    },
    "UVB76-SECRET-0005": {
        "class": "openssh_private_key_pem",
        "scope": "universal",
        "severity": "critical",
        "allowlistable": False,
        "explanation": "OpenSSH private key PEM block detected",
        "remediation": "Remove or replace with [REDACTED]",
        "pattern": _OPENSSH_KEY,
    },
}

# Build universal rules from registry
UNIVERSAL_RULES = [
    {
        "id": rule_id,
        "pattern": re.compile(rules["pattern"]),
        "explanation": rules["explanation"],
        "remediation": rules["remediation"],
    }
    for rule_id, rules in RULE_REGISTRY.items()
    if rules["scope"] == "universal"
]

# ============================================================================
# Artifact Context Rules (applied to inventory artifact surfaces)
# ============================================================================

ARTIFACT_CONTEXT_RULES = [
    {
        "id": "UVB76-SECRET-0010",
        "pattern": re.compile(r'Authorization:\s*(?:Bearer|Basic|Token)\s+[A-Za-z0-9+/=_-]+', re.IGNORECASE),
        "explanation": "non-redacted Authorization header value",
        "remediation": "Replace credential value with [REDACTED]",
    },
    {
        "id": "UVB76-SECRET-0011",
        "pattern": re.compile(r'(?:Cookie|Set-Cookie):\s*[A-Za-z_][A-Za-z0-9_]*=[A-Za-z0-9+/=_-]{16,}', re.IGNORECASE),
        "explanation": "non-redacted session cookie value",
        "remediation": "Replace cookie value with [REDACTED]",
    },
    {
        "id": "UVB76-SECRET-0012",
        "pattern": re.compile(r'uvb76_session=[A-Za-z0-9+/=_-]+', re.IGNORECASE),
        "explanation": "non-redacted uvb76_session cookie",
        "remediation": "Replace cookie value with [REDACTED]",
    },
    {
        "id": "UVB76-SECRET-0013",
        "pattern": re.compile(r'X-Session-Token:\s*[A-Za-z0-9+/=_-]{20,}', re.IGNORECASE),
        "explanation": "non-redacted session token header",
        "remediation": "Replace token value with [REDACTED]",
    },
    {
        "id": "UVB76-SECRET-0020",
        "pattern": re.compile(r'"password_sha256"\s*:\s*"sha256:[a-fA-F0-9]+:[a-fA-F0-9]+"'),
        "explanation": "non-redacted password hash value",
        "remediation": "Replace with [REDACTED]",
    },
    {
        "id": "UVB76-SECRET-0021",
        "pattern": re.compile(r'"admin_password_hash"\s*:\s*"[^"]+"'),
        "explanation": "non-redacted admin password hash",
        "remediation": "Replace with [REDACTED]",
    },
    {
        "id": "UVB76-SECRET-0030",
        "pattern": re.compile(r'X-API-Key:\s*[A-Za-z0-9_-]{16,}', re.IGNORECASE),
        "explanation": "non-redacted API key header",
        "remediation": "Replace with [REDACTED]",
    },
    {
        "id": "UVB76-SECRET-0040",
        "pattern": re.compile(r'https?://[^:]+:[^@]+@[^\s"\'>]+'),
        "explanation": "credential-bearing URL with embedded userinfo",
        "remediation": "Remove userinfo or sanitize URL",
    },
    {
        "id": "UVB76-SECRET-0041",
        "pattern": re.compile(r'(?:postgres|mysql|mongodb)://[^:]+:[^@]+@'),
        "explanation": "database DSN with embedded credentials",
        "remediation": "Remove credentials from DSN",
    },
    {
        "id": "UVB76-SECRET-0050",
        "pattern": re.compile(r'eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+'),
        "explanation": "JWT-like token detected",
        "remediation": "Replace with [REDACTED]",
    },
    {
        "id": "UVB76-SECRET-0051",
        "pattern": re.compile(r'(?:Bearer|Token)\s+[A-Za-z0-9_-]{32,}'),
        "explanation": "bearer token detected",
        "remediation": "Replace with [REDACTED]",
    },
]


def build_test_private_key() -> str:
    """Build a test private key marker for self-test fixtures (not stored as literal)."""
    dashes = "-----"
    space = " "
    begin = "BEGIN"
    priv = "PRIVATE"
    key = "KEY"
    return dashes + space.join([begin, priv, key]) + dashes


def build_test_rsa_key() -> str:
    """Build a test RSA key marker for self-test fixtures (not stored as literal)."""
    dashes = "-----"
    space = " "
    begin = "BEGIN"
    rsa = "RSA"
    priv = "PRIVATE"
    key = "KEY"
    return dashes + space.join([begin, rsa, priv, key]) + dashes
