"""
Risky-shell detection and validation rules.
"""

import os
from pathlib import Path
from typing import Optional

from .model import (
    CheckResult, DISPOSITION_KEEP_WRAPPER, DISPOSITION_GRANDFATHERED,
    BOOTSTRAP_NOTES, THIN_WRAPPER_MAX_LINES
)
from .loader import get_inventory_entry_for_path, count_lines
from .model import check_annotations, parse_risk_tokens


def check_script(path: str, inventory: dict) -> CheckResult:
    """
    Check a single shell script for containment violations.
    
    Returns:
        CheckResult with passed status and list of violations
    """
    violations = []
    
    try:
        with open(path, "r", encoding="utf-8", errors="replace") as f:
            content = f.read()
    except Exception as e:
        return CheckResult(passed=False, violations=[f"Cannot read file: {e}"])
    
    # Count lines
    lines = count_lines(content)
    
    # Check risk tokens
    found_risks = parse_risk_tokens(content)
    
    # If no risk tokens, passes
    if not found_risks:
        return CheckResult(passed=True, violations=[])
    
    # Check header annotations
    annotations = check_annotations(content)
    
    # Get inventory entry
    inventory_entry = get_inventory_entry_for_path(path, inventory)
    
    # Determine disposition from inventory
    if inventory_entry:
        disposition = inventory_entry["disposition"]
        owner = inventory_entry["owner"]
    else:
        disposition = None
        owner = None
    
    # Case 1: Risky script not in inventory - MUST be listed
    if found_risks and not inventory_entry:
        violations.append("Risky shell script must be listed in docs/generated/shell_inventory.csv")
        return CheckResult(passed=False, violations=violations)
    
    # Case 2: In inventory as keep_wrapper - but has risky tokens
    if inventory_entry and disposition == DISPOSITION_KEEP_WRAPPER:
        violations.append(
            f"Inventory says keep_wrapper, but risk tokens were found: {', '.join(found_risks)}"
        )
        return CheckResult(passed=False, violations=violations)
    
    # Case 3: In inventory as grandfathered
    if disposition == DISPOSITION_GRANDFATHERED:
        is_bootstrap = inventory_entry.get("notes", "").strip() == BOOTSTRAP_NOTES
        if owner == "TBD" and not is_bootstrap:
            violations.append("Grandfathered script requires named owner (found: TBD)")
            return CheckResult(passed=False, violations=violations)
        return CheckResult(passed=True, violations=[])
    
    # Case 4: Only has role annotation (not enough for risky script)
    if annotations.has_role and not annotations.has_justification:
        violations.append("Risky script has ShellRole but missing ShellJustification")
        return CheckResult(passed=False, violations=violations)
    
    # Case 5: Not in inventory, no valid justification - FAIL
    violations.append(f"Risk tokens found: {', '.join(found_risks)}")
    if lines > THIN_WRAPPER_MAX_LINES:
        violations.append(f"Script has {lines} lines (max {THIN_WRAPPER_MAX_LINES} for thin wrapper)")
    violations.append("List in docs/generated/shell_inventory.csv")
    
    return CheckResult(passed=False, violations=violations)


def check_inventory_consistency(inventory: dict) -> CheckResult:
    """
    Check inventory consistency: all paths exist, no drift.
    
    Returns:
        CheckResult with passed status and list of issues
    """
    issues = []
    
    if not inventory:
        issues.append("Inventory is empty")
        return CheckResult(passed=False, violations=issues)
    
    for path, entry in inventory.items():
        # Check if path exists
        if not os.path.exists(path):
            issues.append(f"Inventory path does not exist: {path}")
        
        # Check owner for grandfathered entries
        owner = entry.get("owner", "")
        disposition = entry.get("disposition", "")
        notes = entry.get("notes", "")
        
        if disposition == DISPOSITION_GRANDFATHERED and owner == "TBD" and notes != BOOTSTRAP_NOTES:
            issues.append(f"Grandfathered script requires named owner: {path}")
    
    if issues:
        return CheckResult(passed=False, violations=issues)
    return CheckResult(passed=True, violations=[])


def get_shell_scripts() -> list[str]:
    """Get list of shell scripts in the repository."""
    scripts = []
    
    # Scan scripts directory
    scripts_dir = Path("scripts")
    if scripts_dir.exists():
        for f in scripts_dir.rglob("*.sh"):
            scripts.append(str(f))
    
    # Scan other directories
    for pattern in ["uvb76/scripts/*.sh", "tovarisch/scripts/*.sh"]:
        for f in Path(".").glob(pattern):
            if str(f) not in scripts:
                scripts.append(str(f))
    
    return sorted(scripts)
