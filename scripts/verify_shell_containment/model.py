"""
Data models and constants for shell containment verifier.
"""

import re
from dataclasses import dataclass
from typing import Optional


# Risk token patterns (regex)
# Note: patterns must be specific enough to avoid false positives
RISK_PATTERNS = {
    "jq": r"\bjq\s",
    "curl_parse": r"curl.*\$\|curl.*parse\|curl\s+\$\(",
    "polling_loop": r"while\s+true|until\s+true|while\s+\[\[|while\s+\(\(|until\s+\[\[|until\s+\(\(",
    "retry": r"\bretry\b|\bcooldown\b",
    "gh_release": r"\bgh\s+release\b",
    "trap_cleanup": r"trap.*cleanup|trap.*exit",
    "json_write": r"\$\(.*json|JSON\s*=\s*\$\(",
}

# Thin wrapper max lines
THIN_WRAPPER_MAX_LINES = 50

# Default inventory path
INVENTORY_CSV = "docs/generated/shell_inventory.csv"

# Required CSV columns
REQUIRED_COLUMNS = {"path", "disposition", "risk_flags", "owner", "notes"}

# Disposition values
DISPOSITION_KEEP_WRAPPER = "keep_wrapper"
DISPOSITION_GRANDFATHERED = "grandfathered_needs_owner"

# Bootstrap notes marker
BOOTSTRAP_NOTES = "Bootstrap inventory"


@dataclass
class InventoryEntry:
    """Represents a single inventory entry for a shell script."""
    path: str
    disposition: str
    risk_flags: str
    owner: str
    notes: str


@dataclass
class CheckResult:
    """Result of checking a single script."""
    passed: bool
    violations: list[str]


@dataclass
class AnnotationCheck:
    """Result of checking shell containment annotations."""
    has_justification: bool
    has_role: bool
    has_migration_plan: bool


def is_thin_wrapper(lines: int) -> bool:
    """Check if script qualifies as thin wrapper based on line count."""
    return lines <= THIN_WRAPPER_MAX_LINES


def parse_risk_tokens(content: str) -> list[str]:
    """Extract risk tokens found in script content."""
    found = []
    for risk_name, pattern in RISK_PATTERNS.items():
        if re.search(pattern, content, re.IGNORECASE):
            found.append(risk_name)
    return found


def check_annotations(content: str) -> AnnotationCheck:
    """
    Check if script has explicit shell containment justification headers.
    """
    return AnnotationCheck(
        has_justification=bool(re.search(r"#\s*ShellJustification:", content, re.IGNORECASE)),
        has_role=bool(re.search(r"#\s*ShellRole:", content, re.IGNORECASE)),
        has_migration_plan=bool(re.search(r"#\s*MigrationPlan:", content, re.IGNORECASE)),
    )
