"""
Scanner module for UVB-76 Artifact Secret Hygiene.

Provides file scanning and secret detection.
"""

import glob
import json
import os
import re
import subprocess
from dataclasses import dataclass
from typing import Optional, Any

from .inventory import ARTIFACT_INVENTORY, RuleSet
from .rules import UNIVERSAL_RULES, ARTIFACT_CONTEXT_RULES
from .structured_scanner import scan_structured_json, StructuredFinding

# Canonical path to the registry file (exact match required)
CANONICAL_REGISTRY_PATH = os.path.join(
    os.path.dirname(__file__), "registry.json"
)

# Fields in registry.json that contain detector definitions (not secrets)
# These fields' values should be scanned as patterns, not as potential secrets
REGISTRY_DETECTOR_METADATA_FIELDS = {"pattern", "header_pattern", "field_names", "query_params"}

# Bounds
MAX_FILE_SIZE = 1024 * 1024  # 1MB per file
MAX_FILES_SCANNED = 10000
MAX_DIAGNOSTICS = 100
MAX_BYTES_PER_FILE = 10 * 1024 * 1024  # 10MB max retained in memory

# Text extensions
TEXT_EXTENSIONS = {".go", ".py", ".json", ".yaml", ".yml", ".sh", ".md", ".txt", ".conf", ".cfg"}
BINARY_EXTENSIONS = {".png", ".jpg", ".jpeg", ".gif", ".ico", ".pdf", ".zip", ".gz", ".tar", ".tgz", ".wasm"}


@dataclass
class SecretFinding:
    """Represents a detected secret without exposing the value."""
    rule_id: str
    file_path: str
    line_number: int
    explanation: str
    remediation: str
    field_path: Optional[str] = None  # Structured field path for JSON artifacts


def is_text_file(path: str) -> bool:
    """Determine if a file is likely text."""
    ext = os.path.splitext(path)[1].lower()
    if ext in TEXT_EXTENSIONS:
        return True
    if ext in BINARY_EXTENSIONS:
        return False
    try:
        with open(path, 'rb') as f:
            chunk = f.read(512)
            if b'\x00' in chunk:
                return False
    except:
        return False
    return True


def scan_file_for_secrets(path: str, artifact_surface: bool = False) -> list[SecretFinding]:
    """Scan a single file for secret patterns."""
    findings = []

    # Fail closed when candidate files disappear between enumeration and scan
    if not os.path.exists(path):
        findings.append(SecretFinding(
            rule_id="UVB76-FS-0003",
            file_path=path,
            line_number=0,
            explanation="candidate file disappeared before scan",
            remediation="Re-enumerate candidates and re-scan",
            field_path=None,
        ))
        return findings

    try:
        size = os.path.getsize(path)
        if size > MAX_FILE_SIZE:
            findings.append(SecretFinding(
                rule_id="UVB76-SIZE-0001",
                file_path=path,
                line_number=0,
                explanation=f"file exceeds size limit ({size} > {MAX_FILE_SIZE})",
                remediation="Split file or increase MAX_FILE_SIZE if necessary",
                field_path=None,
            ))
            return findings
    except OSError as e:
        findings.append(SecretFinding(
            rule_id="UVB76-FS-0001",
            file_path=path,
            line_number=0,
            explanation=f"cannot read file metadata: {e}",
            remediation="Check file permissions",
            field_path=None,
        ))
        return findings

    if not is_text_file(path):
        if artifact_surface:
            findings.append(SecretFinding(
                rule_id="UVB76-BINARY-0001",
                file_path=path,
                line_number=0,
                explanation="binary artifact requires explicit classification",
                remediation="Add binary_policy to inventory entry or convert to text",
                field_path=None,
            ))
        return findings

    try:
        with open(path, 'r', encoding='utf-8', errors='replace') as f:
            content = f.read(MAX_BYTES_PER_FILE)
    except OSError as e:
        findings.append(SecretFinding(
            rule_id="UVB76-FS-0002",
            file_path=path,
            line_number=0,
            explanation=f"cannot read file: {e}",
            remediation="Check file permissions",
            field_path=None,
        ))
        return findings

    lines = content.split('\n')

    # Special handling ONLY for the canonical registry.json path
    # Use exact path matching, not basename
    is_canonical_registry = os.path.abspath(path) == os.path.abspath(CANONICAL_REGISTRY_PATH)

    if is_canonical_registry:
        # Parse registry and identify lines containing detector metadata
        # These lines should be skipped to avoid false positives on pattern definitions
        registry_metadata_lines = _get_registry_metadata_lines(content)
    else:
        registry_metadata_lines = set()

    # Apply universal rules to ALL files (universal critical rules protect all content)
    for rule in UNIVERSAL_RULES:
        for line_num, line in enumerate(lines, 1):
            # Skip lines that are detector metadata in canonical registry.json
            if line_num in registry_metadata_lines:
                continue
            if rule["pattern"].search(line):
                findings.append(SecretFinding(
                    rule_id=rule["id"],
                    file_path=path,
                    line_number=line_num,
                    explanation=rule["explanation"],
                    remediation=rule["remediation"],
                    field_path=None,
                ))
                break

    # Apply artifact context rules only to artifact surfaces with context rules
    # Hygiene infrastructure files (redact.go, registry.json) are universal-only
    # and should not receive context rules to avoid false positives on
    # implementation code containing patterns.
    if artifact_surface:
        from .inventory import RuleSet
        # Determine relative path for matching and repo root
        abs_path = os.path.abspath(path)
        repo_root = None
        scan_dir = os.path.dirname(path)
        while scan_dir and scan_dir != '/':
            if os.path.exists(os.path.join(scan_dir, '.git')) or os.path.exists(os.path.join(scan_dir, 'AGENTS.md')):
                repo_root = scan_dir
                try:
                    rel_path = os.path.relpath(path, repo_root)
                except ValueError:
                    rel_path = path
                break
            scan_dir = os.path.dirname(scan_dir)
        else:
            rel_path = path
            repo_root = os.path.dirname(path)  # Fallback

        matched_surface = None
        for surf in ARTIFACT_INVENTORY:
            if surf.path == rel_path or glob.fnmatch.fnmatch(rel_path, surf.path.replace('**/*', '*')):
                matched_surface = surf
                break

        # Skip artifact context rules for universal-only surfaces
        if matched_surface and matched_surface.rule_set == RuleSet.UNIVERSAL.value:
            pass  # Don't apply context rules to universal-only surfaces
        else:
            for rule in ARTIFACT_CONTEXT_RULES:
                for line_num, line in enumerate(lines, 1):
                    if rule["pattern"].search(line):
                        findings.append(SecretFinding(
                            rule_id=rule["id"],
                            file_path=path,
                            line_number=line_num,
                            explanation=rule["explanation"],
                            remediation=rule["remediation"],
                            field_path=None,
                        ))
                        break

        # For JSON artifact surfaces, run structured scanning
        if os.path.splitext(path)[1].lower() == '.json':
            sf_findings, malformed = scan_structured_json(content, field_path="root")
            # Add structured findings with field path preserved
            for sf in sf_findings:
                findings.append(SecretFinding(
                    rule_id=sf.rule_id,
                    file_path=path,
                    line_number=0,
                    explanation=sf.explanation,
                    remediation=sf.remediation,
                    field_path=sf.field_path,  # PRESERVE field path
                ))
            # Fail closed for malformed JSON unless exact fixture exemption exists
            if malformed:
                from .inventory import is_malformed_fixture_exempt
                is_exempt, error = is_malformed_fixture_exempt(rel_path, repo_root)
                if not is_exempt:
                    findings.append(SecretFinding(
                        rule_id=malformed.rule_id,
                        file_path=path,
                        line_number=0,
                        explanation=malformed.explanation + (f" ({error})" if error else ""),
                        remediation=malformed.remediation,
                        field_path=None,
                    ))

    return findings


def get_candidate_files(repo_root: str) -> list[str]:
    """Get list of all candidate files (tracked + untracked non-ignored)."""
    try:
        result_tracked = subprocess.run(
            ["git", "ls-files", "-z", "--cached"],
            cwd=repo_root,
            capture_output=True,
            timeout=60,
        )
        if result_tracked.returncode != 0:
            return []

        result_others = subprocess.run(
            ["git", "ls-files", "-z", "--others", "--exclude-standard"],
            cwd=repo_root,
            capture_output=True,
            timeout=60,
        )
        if result_others.returncode != 0:
            return []

        tracked = set(f.strip() for f in result_tracked.stdout.decode('utf-8', errors='replace').split('\0') if f.strip())
        others = set(f.strip() for f in result_others.stdout.decode('utf-8', errors='replace').split('\0') if f.strip())
        all_files = tracked | others

        if not all_files:
            return []

        return [os.path.join(repo_root, f) for f in all_files]

    except (subprocess.TimeoutExpired, Exception):
        return []


def format_finding(finding: SecretFinding, repo_root: str) -> str:
    """Format a finding without exposing the secret value."""
    rel_path = os.path.relpath(finding.file_path, repo_root)
    if finding.line_number > 0:
        location = f"{rel_path}:{finding.line_number}"
    elif finding.field_path:
        location = f"{rel_path}: {finding.field_path}"
    else:
        location = rel_path

    return f"{location}: {finding.rule_id}: {finding.explanation}"


def _get_registry_metadata_lines(content: str) -> set[int]:
    """
    Parse registry.json content and identify line numbers containing detector metadata.

    Returns set of 1-based line numbers that contain detector metadata fields
    (pattern, header_pattern, field_names, query_params). These lines should be
    skipped during scanning to avoid false positives on pattern definitions.

    Only applies to the canonical registry.json file.
    """
    metadata_lines = set()
    lines = content.split('\n')

    try:
        # Parse the registry to understand its structure
        data = json.loads(content)
    except (json.JSONDecodeError, ValueError):
        return metadata_lines

    # Build a set of metadata field values to find in the original text
    # We need to match the exact values to find their line numbers
    metadata_values = set()

    for rule in data.get("rules", []):
        for field in REGISTRY_DETECTOR_METADATA_FIELDS:
            if field in rule:
                value = rule[field]
                if isinstance(value, str):
                    metadata_values.add(value)
                elif isinstance(value, list):
                    for item in value:
                        if isinstance(item, str):
                            metadata_values.add(item)

    # Find lines containing these metadata values
    for line_num, line in enumerate(lines, 1):
        for value in metadata_values:
            if value in line:
                metadata_lines.add(line_num)
                break

    return metadata_lines
