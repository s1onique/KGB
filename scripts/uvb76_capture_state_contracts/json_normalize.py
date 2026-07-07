"""
JSON normalization helpers for UVB-76 HULK02 Capture State verification.

Provides pure string/content normalization utilities without knowledge of
repo paths or line limits. Safe to use in unit tests.
"""

import re


def strip_json_strings(content: str) -> str:
    """
    Remove JSON string contents to avoid false positives from test data.

    Strips double-quoted strings (handles escaped quotes) and single-quoted strings.
    This prevents test data like "command_tool": "ss" from being flagged as
    forbidden command execution.

    Args:
        content: Raw file content to strip.

    Returns:
        Content with string literals replaced by empty placeholders.
    """
    # Remove double-quoted strings (handles escaped quotes)
    content = re.sub(r'"(?:[^"\\]|\\.)*"', '""', content)
    # Remove single-quoted strings
    content = re.sub(r"'(?:[^'\\]|\\.)*'", "''", content)
    return content


def strip_comments(content: str) -> str:
    """
    Remove Go-style comments from content to avoid false positives.

    Strips single-line comments (// ...) and multi-line comments (/* ... */).
    This allows doctrine comments in code to be ignored during pattern matching.

    Args:
        content: Raw file content.

    Returns:
        Content with comments removed.
    """
    # Remove single-line comments
    content = re.sub(r'//.*$', '', content, flags=re.MULTILINE)
    # Remove multi-line comments
    content = re.sub(r'/\*.*?\*/', '', content, flags=re.DOTALL)
    return content


def normalize_reason(value: object) -> str:
    """
    Normalize a TCP absence reason value to string.

    Handles both string values and structured dict values with a 'reason' key.
    Used when parsing structured diagnostic capture absence payloads.

    Args:
        value: Raw value from JSON (str or dict).

    Returns:
        Normalized string reason, or empty string if unparseable.
    """
    if isinstance(value, str):
        return value.strip()
    if isinstance(value, dict):
        reason = value.get("reason", "")
        if isinstance(reason, str):
            return reason.strip()
    return ""
