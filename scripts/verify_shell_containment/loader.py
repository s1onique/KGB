"""
CSV loader and validation for shell inventory.
"""

import csv
import os
from pathlib import Path
from typing import Optional

from .model import InventoryEntry, REQUIRED_COLUMNS, DISPOSITION_GRANDFATHERED, BOOTSTRAP_NOTES


def load_inventory(csv_path: str) -> dict:
    """
    Load inventory from CSV file, skipping comment lines.
    
    Returns:
        dict mapping path -> inventory entry dict
    """
    inventory = {}
    
    if not os.path.exists(csv_path):
        return inventory

    with open(csv_path, "r", encoding="utf-8") as f:
        # Filter out comment lines and empty lines
        lines = [
            line for line in f
            if line.strip() and not line.strip().startswith("#")
        ]

    if not lines:
        raise ValueError(f"Shell inventory is empty: {csv_path}")

    reader = csv.DictReader(lines)
    
    if set(reader.fieldnames or []) != REQUIRED_COLUMNS:
        raise ValueError(f"Invalid shell inventory header: {reader.fieldnames}")

    for row in reader:
        path = row["path"].strip()
        if path:
            inventory[path] = {
                "disposition": row["disposition"].strip(),
                "risk_flags": row["risk_flags"].strip(),
                "owner": row["owner"].strip(),
                "notes": row["notes"].strip(),
            }

    if not inventory:
        raise ValueError(f"Shell inventory loaded zero entries from: {csv_path}")

    return inventory


def validate_inventory_entry(path: str, entry: dict) -> list[str]:
    """
    Validate a single inventory entry.
    
    Returns:
        List of validation issues (empty if valid)
    """
    issues = []
    owner = entry.get("owner", "")
    disposition = entry.get("disposition", "")
    notes = entry.get("notes", "")
    
    # Check owner for grandfathered entries
    if disposition == DISPOSITION_GRANDFATHERED:
        is_bootstrap = notes.strip() == BOOTSTRAP_NOTES
        if owner == "TBD" and not is_bootstrap:
            issues.append(f"Grandfathered script requires named owner: {path}")
    
    return issues


def get_inventory_entry_for_path(path: str, inventory: dict) -> Optional[dict]:
    """
    Get inventory entry for a path, trying full path then basename.
    """
    rel_path = str(Path(path).as_posix())
    basename = Path(path).name
    return inventory.get(rel_path) or inventory.get(basename)


def count_lines(content: str) -> int:
    """Count lines in content string."""
    return content.count("\n") + (1 if content and not content.endswith("\n") else 0)
