"""
Makefile hulk gate validation for UVB-76 HULK02 Capture State Contract verification.
"""

import os
import re


def check_makefile_has_hulk_gate(repo_root: str) -> list[str]:
    """
    Check that Makefile contains hulk-uvb76-capture-gate target.

    Args:
        repo_root: Path to repository root.

    Returns:
        List of error messages (empty if no errors).
    """
    makefile_path = os.path.join(repo_root, "Makefile")
    errors = []

    if not os.path.isfile(makefile_path):
        errors.append("ERROR: Makefile does not exist")
        return errors

    with open(makefile_path, 'r') as f:
        content = f.read()

    # Check for hulk-uvb76-capture-gate target
    if not re.search(r'^hulk-uvb76-capture-gate\s*:', content, re.MULTILINE):
        errors.append("ERROR: Makefile lacks 'hulk-uvb76-capture-gate:' target")
        return errors

    # Check that hulk-uvb76-capture-gate includes go test for relevant packages
    hulk_gate_match = re.search(
        r'^hulk-uvb76-capture-gate\s*:.*?(?=\n\n|\n\.[A-Z]|\Z)',
        content,
        re.MULTILINE | re.DOTALL
    )
    if hulk_gate_match:
        gate_content = hulk_gate_match.group(0)
        if 'go test' not in gate_content:
            errors.append("ERROR: hulk-uvb76-capture-gate target lacks 'go test' command")
        if '-race' not in gate_content:
            errors.append("ERROR: hulk-uvb76-capture-gate target lacks '-race' flag for go test")
        # Check for specific packages
        if './state/' not in gate_content and './state/...' not in gate_content:
            errors.append("ERROR: hulk-uvb76-capture-gate target should include ./state/... package")
        if './server/' not in gate_content and './server/...' not in gate_content:
            errors.append("ERROR: hulk-uvb76-capture-gate target should include ./server/... package")
        if './diag/' not in gate_content and './diag/...' not in gate_content:
            errors.append("ERROR: hulk-uvb76-capture-gate target should include ./diag/... package")

    return errors
