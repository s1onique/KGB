"""
Scanner module for UVB-76 Artifact Secret Hygiene.

Provides file scanning and secret detection.
"""

import glob
import os
import re
import subprocess
from dataclasses import dataclass
from typing import Optional

from .inventory import ARTIFACT_INVENTORY
from .rules import UNIVERSAL_RULES, ARTIFACT_CONTEXT_RULES

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
    
    try:
        size = os.path.getsize(path)
        if size > MAX_FILE_SIZE:
            findings.append(SecretFinding(
                rule_id="UVB76-SIZE-0001",
                file_path=path,
                line_number=0,
                explanation=f"file exceeds size limit ({size} > {MAX_FILE_SIZE})",
                remediation="Split file or increase MAX_FILE_SIZE if necessary",
            ))
            return findings
    except OSError as e:
        findings.append(SecretFinding(
            rule_id="UVB76-FS-0001",
            file_path=path,
            line_number=0,
            explanation=f"cannot read file metadata: {e}",
            remediation="Check file permissions",
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
        ))
        return findings
    
    lines = content.split('\n')
    
    # Apply universal rules to ALL files (universal critical rules protect all content)
    for rule in UNIVERSAL_RULES:
        for line_num, line in enumerate(lines, 1):
            if rule["pattern"].search(line):
                findings.append(SecretFinding(
                    rule_id=rule["id"],
                    file_path=path,
                    line_number=line_num,
                    explanation=rule["explanation"],
                    remediation=rule["remediation"],
                ))
                break
    
    # Apply artifact context rules only to artifact surfaces (context-specific secrets)
    if artifact_surface:
        for rule in ARTIFACT_CONTEXT_RULES:
            for line_num, line in enumerate(lines, 1):
                if rule["pattern"].search(line):
                    findings.append(SecretFinding(
                        rule_id=rule["id"],
                        file_path=path,
                        line_number=line_num,
                        explanation=rule["explanation"],
                        remediation=rule["remediation"],
                    ))
                    break
    
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
        return f"{rel_path}:{finding.line_number}: {finding.rule_id}: {finding.explanation}"
    return f"{rel_path}: {finding.rule_id}: {finding.explanation}"
