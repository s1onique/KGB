"""
Shell containment verifier package.

Provides verification that shell scripts follow containment doctrine.
"""

from .model import (
    RISK_PATTERNS,
    THIN_WRAPPER_MAX_LINES,
    INVENTORY_CSV,
    CheckResult,
    InventoryEntry,
    AnnotationCheck,
)
from .loader import load_inventory, count_lines
from .rules import check_script, check_inventory_consistency, get_shell_scripts
from .selftest import run_tests
from .cli import main

__all__ = [
    "RISK_PATTERNS",
    "THIN_WRAPPER_MAX_LINES",
    "INVENTORY_CSV",
    "CheckResult",
    "InventoryEntry",
    "AnnotationCheck",
    "load_inventory",
    "count_lines",
    "check_script",
    "check_inventory_consistency",
    "get_shell_scripts",
    "run_tests",
    "main",
]
