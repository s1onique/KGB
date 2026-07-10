"""
Structured artifact scanner for UVB-76 Artifact Secret Hygiene.

Handles structured JSON artifacts using field_name and structured_json detectors
from the canonical registry.
"""

import json
import re
import urllib.parse
from dataclasses import dataclass, field
from typing import Any

from .registry_loader import get_registry, get_artifact_context_rules


# ============================================================================
# Field name patterns derived from registry
# ============================================================================

# Build field name sets from registry
_registry = get_registry()

# Sensitive field names by rule class
FIELD_RULES: dict[str, tuple[set[str], str, str]] = {}
for rule in get_artifact_context_rules(_registry):
    if rule.get("detector_kind") == "field_name":
        field_names = set(f.lower() for f in rule.get("field_names", []))
        FIELD_RULES[rule["rule_id"]] = (
            field_names,
            rule["safe_explanation"],
            rule["safe_remediation"],
        )

# Query parameter rules derived from registry
QUERY_PARAM_RULES: dict[str, tuple[set[str], str, str]] = {}
for rule in get_artifact_context_rules(_registry):
    if rule.get("detector_kind") == "url_component":
        query_params = rule.get("query_params", [])
        if query_params:
            QUERY_PARAM_RULES[rule["rule_id"]] = (
                {p.lower() for p in query_params},
                rule["safe_explanation"],
                rule["safe_remediation"],
            )

# Password hash specific check (sha256: prefix)
PASSWORD_HASH_PREFIX = "sha256:"

# Query bounds - fail closed on overflow
MAX_QUERY_PARAMS = 100
MAX_QUERY_LENGTH = 10000


def is_safe_value(value: Any) -> bool:
    """Check if a value is safe (should not be flagged)."""
    if value is None:
        return True
    if isinstance(value, str):
        # [REDACTED] is safe
        if value == "[REDACTED]":
            return True
        # null string is safe
        if value.lower() == "null":
            return True
        # Empty strings are safe
        if value == "":
            return True
        # Placeholders
        if value.startswith("${") and value.endswith("}"):
            return True
        if value.startswith("<") and value.endswith(">"):
            return True
    return False


class StructuredFinding:
    """Represents a finding from structured artifact scanning."""
    def __init__(
        self,
        rule_id: str,
        field_path: str,
        explanation: str,
        remediation: str,
    ):
        self.rule_id = rule_id
        self.field_path = field_path
        self.explanation = explanation
        self.remediation = remediation


# Malformed artifact rule ID
STRUCTURE_ERROR_RULE_ID = "UVB76-STRUCTURE-0001"


@dataclass
class MalformedArtifactFinding:
    """Represents a malformed structured artifact finding."""
    rule_id: str
    file_path: str
    explanation: str
    remediation: str


def _build_field_path(parts: list) -> str:
    """Build a JSON path string from path components."""
    result = []
    for i, part in enumerate(parts):
        if isinstance(part, int):
            result.append(f"[{part}]")
        else:
            if i > 0:
                result.append(".")
            result.append(part)
    return "".join(result)


def scan_json_value(
    value: Any,
    field_path: list,
    findings: list[StructuredFinding],
    depth: int = 0,
) -> None:
    """Recursively scan a parsed JSON value for secrets."""
    # Prevent runaway recursion
    if depth > 50:
        return

    if isinstance(value, dict):
        for key, v in value.items():
            new_path = field_path + [key]
            _scan_field_value(key, v, new_path, findings, depth)

    elif isinstance(value, list):
        for i, item in enumerate(value):
            new_path = field_path + [i]
            scan_json_value(item, new_path, findings, depth + 1)


def _scan_field_value(
    field_name: str,
    value: Any,
    field_path: list,
    findings: list[StructuredFinding],
    depth: int,
) -> None:
    """Scan a single field value for secrets."""
    field_lower = field_name.lower()

    # Skip safe field names (but still recurse into nested structures)
    safe_fields = {"id", "name", "type", "version", "timestamp", "created", "modified"}
    if field_lower in safe_fields:
        scan_json_value(value, field_path, findings, depth + 1)
        return

    # Check against field name rules
    for rule_id, (field_names, explanation, remediation) in FIELD_RULES.items():
        if field_lower in field_names:
            # Found a sensitive field - check if value is safe
            if is_safe_value(value):
                # Safe value, don't report
                return

            # Special case: password_hash fields with sha256: prefix
            if "hash" in field_lower and isinstance(value, str):
                if value.startswith(PASSWORD_HASH_PREFIX):
                    findings.append(StructuredFinding(
                        rule_id=rule_id,
                        field_path=_build_field_path(field_path),
                        explanation=explanation,
                        remediation=remediation,
                    ))
                    return

            # Password fields - report any non-empty value
            if "password" in field_lower:
                findings.append(StructuredFinding(
                    rule_id=rule_id,
                    field_path=_build_field_path(field_path),
                    explanation=explanation,
                    remediation=remediation,
                ))
                return

            # Generic token fields - report if value is a non-empty string
            if isinstance(value, str) and len(value) >= 4:
                # Report the finding
                findings.append(StructuredFinding(
                    rule_id=rule_id,
                    field_path=_build_field_path(field_path),
                    explanation=explanation,
                    remediation=remediation,
                ))
                return
            return

    # Not a sensitive field - recurse into nested structures
    scan_json_value(value, field_path, findings, depth + 1)


def _scan_url_for_sensitive_params(content: str, field_path: str) -> list[StructuredFinding]:
    """
    Scan text content for URLs with sensitive query parameters.

    This handles the sensitive_url_query_parameter rule (UVB76-SECRET-0072).

    Supports:
    - Absolute URLs: https://host/path?token=value
    - HTTP URLs: http://host/path?token=value
    - Protocol-relative: //host/path?token=value
    - Relative paths: /auth/callback?token=value
    - Query-only: ?token=value

    Query bounds: fails closed on overflow (over-limit or malformed query
    produces a safe contract finding rather than silently skipping fields).
    """
    findings = []

    # Pattern 1: Full URLs with scheme (http://, https://)
    absolute_url_pattern = re.compile(
        r'https?://[^\s"\'<>]+',
        re.IGNORECASE
    )

    # Pattern 2: Protocol-relative URLs (//host/path?query)
    protocol_relative_pattern = re.compile(
        r'//[^\s"\'<>]+',
        re.IGNORECASE
    )

    # Pattern 3: Relative URLs starting with /
    relative_path_pattern = re.compile(
        r'/[^\s"\'<>]*\?[^"\'<>]*',
        re.IGNORECASE
    )

    # Pattern 4: Query-only strings (?param=value)
    query_only_pattern = re.compile(
        r'\?[^\s"\'<>]+',
        re.IGNORECASE
    )

    def scan_url(url: str, base_field_path: str) -> None:
        """Scan a single URL for sensitive query parameters."""
        if not url or '?' not in url:
            return

        # Split URL from query string
        if '?' in url:
            path_part, query_part = url.split('?', 1)
        else:
            return

        if not query_part:
            return

        # Parse query parameters using bounded parsing
        # Fails closed: over-limit or malformed query produces safe contract finding
        try:
            # Limit query string length to prevent DoS
            if len(query_part) > MAX_QUERY_LENGTH:
                # Query too long - fail closed
                findings.append(StructuredFinding(
                    rule_id="UVB76-SECRET-0072",
                    field_path=f"{base_field_path}.query",
                    explanation="query parameter count or length exceeds safety bounds",
                    remediation="Reduce query parameters or truncate values",
                ))
                return

            # Use bounded query parsing - strict_parsing=True fails on malformed input
            # max_num_fields=MAX_QUERY_PARAMS enforces bounds
            params = urllib.parse.parse_qsl(
                query_part,
                keep_blank_values=True,
                strict_parsing=True,
                max_num_fields=MAX_QUERY_PARAMS,
            )

            # If we got here, parsing succeeded within bounds
            for param_name, param_value in params:
                param_lower = param_name.lower()

                # Check against sensitive query parameter rules
                for rule_id, (sensitive_params, explanation, remediation) in QUERY_PARAM_RULES.items():
                    if param_lower in sensitive_params:
                        # Check if value is safe
                        if is_safe_value(param_value):
                            continue

                        # Build a field path for this URL location
                        # Format: <field_path>.query.<param_name>
                        url_field_path = f"{base_field_path}.query.{param_name}"

                        findings.append(StructuredFinding(
                            rule_id=rule_id,
                            field_path=url_field_path,
                            explanation=explanation,
                            remediation=remediation,
                        ))
                        break  # Only report once per param

        except ValueError:
            # Malformed query (strict_parsing=True) - fail closed
            findings.append(StructuredFinding(
                rule_id="UVB76-SECRET-0072",
                field_path=f"{base_field_path}.query",
                explanation="malformed query string",
                remediation="Fix URL encoding or remove malformed parameters",
            ))
        except Exception:
            # Any other error - fail closed
            findings.append(StructuredFinding(
                rule_id="UVB76-SECRET-0072",
                field_path=f"{base_field_path}.query",
                explanation="query parsing error",
                remediation="Fix query string format",
            ))

    # Scan all URL patterns
    for pattern in [absolute_url_pattern, protocol_relative_pattern, relative_path_pattern, query_only_pattern]:
        for match in pattern.finditer(content):
            url = match.group(0)
            scan_url(url, field_path)

    return findings


def check_malformed_json(content: str) -> MalformedArtifactFinding | None:
    """
    Check if JSON content is malformed.
    Returns MalformedArtifactFinding if malformed, None otherwise.
    """
    try:
        json.loads(content)
        return None
    except (json.JSONDecodeError, ValueError) as e:
        return MalformedArtifactFinding(
            rule_id=STRUCTURE_ERROR_RULE_ID,
            file_path="",
            explanation=f"malformed JSON artifact: {type(e).__name__}",
            remediation="Fix JSON syntax before scanning for secrets",
        )


def scan_structured_json(content: str, field_path: str = "root") -> tuple[list[StructuredFinding], MalformedArtifactFinding | None]:
    """
    Scan JSON content for structured secret patterns.
    Returns tuple of (findings, malformed_finding).
    Malformed JSON returns malformed_finding with UVB76-STRUCTURE-0001.
    """
    findings: list[StructuredFinding] = []

    try:
        data = json.loads(content)
    except (json.JSONDecodeError, ValueError):
        # Malformed JSON - fail closed for artifact scanning
        malformed = MalformedArtifactFinding(
            rule_id=STRUCTURE_ERROR_RULE_ID,
            file_path="",
            explanation="malformed JSON artifact prevents secret scanning",
            remediation="Fix JSON syntax before scanning for secrets",
        )
        return findings, malformed

    # Scan JSON structure for field-based secrets
    scan_json_value(data, [], findings)

    # Also scan string values for URL-based secrets (query parameters)
    # This handles cases where URLs appear in JSON string values
    # Skip strings that look like regex patterns (contain (?: which is a non-capturing group)
    for string_value in _extract_string_values(data):
        if isinstance(string_value, str) and '?' in string_value:
            # Skip regex patterns - they use (?: for non-capturing groups
            if '(?:' in string_value or '(?' in string_value:
                continue
            url_findings = _scan_url_for_sensitive_params(string_value, field_path)
            findings.extend(url_findings)

    return findings, None


def _extract_string_values(data: Any) -> list[str]:
    """Recursively extract all string values from parsed JSON."""
    strings = []
    if isinstance(data, str):
        strings.append(data)
    elif isinstance(data, dict):
        for v in data.values():
            strings.extend(_extract_string_values(v))
    elif isinstance(data, list):
        for item in data:
            strings.extend(_extract_string_values(item))
    return strings


def scan_json_file(path: str) -> tuple[list[StructuredFinding], MalformedArtifactFinding | None]:
    """Scan a JSON file for structured secret patterns."""
    try:
        with open(path, 'r', encoding='utf-8') as f:
            content = f.read()
    except (OSError, UnicodeDecodeError):
        return [], None
