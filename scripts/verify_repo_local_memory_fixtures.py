#!/usr/bin/env python3
"""
Test fixtures for verify_repo_local_memory.py self-tests.

This module contains only the test fixture data and creation logic.
It is separated to keep the main verifier under the line-count limit.
"""

import os


def create_test_fixture(tmpdir: str, variant: str) -> str:
    """Create a test fixture for self-testing."""
    
    epic_content = (
        "# Epic\n\n"
        "## ACT 1: Test Work\n\n"
        "**Status: done**\n\n"
        "| ID | Work | Status |\n"
        "|----|------|--------|\n"
        "| test-001 | Item | done |\n"
    )
    
    matrix_valid = (
        "axiom_id,axiom_name,repo_area,enforcement_level,enforcement_artifact,status,notes\n"
        "AXIOM-1,Repo-Local Project Memory,doctrine,documented,docs/doctrine/ai-native-code-discipline-axioms.md,present,Test\n"
        "AXIOM-1,Repo-Local Project Memory,epics,advisory,docs/epics/* pattern,present,Test\n"
        "AXIOM-1,Repo-Local Project Memory,gates,hard_gate,scripts/verify_repo_local_memory.py,partial,Test\n"
        "AXIOM-2,Cold-Resume Checkpoint,doctrine,documented,docs/doctrine/ai-native-code-discipline-axioms.md,present,Test\n"
        "AXIOM-3,Periodic Health Audit,doctrine,documented,docs/doctrine/ai-native-code-discipline-axioms.md,present,Test\n"
        "AXIOM-4,Production-Path Parity,doctrine,documented,docs/doctrine/ai-native-code-discipline-axioms.md,present,Test\n"
        "AXIOM-5,Managed Agent Blocks,doctrine,documented,docs/doctrine/ai-native-code-discipline-axioms.md,present,Test\n"
        "AXIOM-6,Stable Doctrine/ADR Links,doctrine,documented,docs/doctrine/ai-native-code-discipline-axioms.md,present,Test\n"
    )
    
    matrix_without_axiom1 = (
        "axiom_id,axiom_name,repo_area,enforcement_level,enforcement_artifact,status,notes\n"
        "AXIOM-2,Cold-Resume Checkpoint,doctrine,documented,docs/doctrine/test.md,present,Test\n"
        "AXIOM-3,Periodic Health Audit,doctrine,documented,docs/doctrine/test.md,present,Test\n"
        "AXIOM-4,Production-Path Parity,doctrine,documented,docs/doctrine/test.md,present,Test\n"
        "AXIOM-5,Managed Agent Blocks,doctrine,documented,docs/doctrine/test.md,present,Test\n"
        "AXIOM-6,Stable Doctrine/ADR Links,doctrine,documented,docs/doctrine/test.md,present,Test\n"
    )
    
    matrix_without_epics = (
        "axiom_id,axiom_name,repo_area,enforcement_level,enforcement_artifact,status,notes\n"
        "AXIOM-1,Repo-Local Project Memory,doctrine,documented,docs/doctrine/test.md,present,Test\n"
        "AXIOM-2,Cold-Resume Checkpoint,doctrine,documented,docs/doctrine/test.md,present,Test\n"
        "AXIOM-3,Periodic Health Audit,doctrine,documented,docs/doctrine/test.md,present,Test\n"
        "AXIOM-4,Production-Path Parity,doctrine,documented,docs/doctrine/test.md,present,Test\n"
        "AXIOM-5,Managed Agent Blocks,doctrine,documented,docs/doctrine/test.md,present,Test\n"
        "AXIOM-6,Stable Doctrine/ADR Links,doctrine,documented,docs/doctrine/test.md,present,Test\n"
    )
    
    if variant == "valid":
        os.makedirs(os.path.join(tmpdir, "docs", "doctrine"), exist_ok=True)
        os.makedirs(os.path.join(tmpdir, "docs", "epics"), exist_ok=True)
        os.makedirs(os.path.join(tmpdir, "scripts"), exist_ok=True)
        
        with open(os.path.join(tmpdir, "docs", "doctrine", "ai-native-code-discipline-axioms.md"), "w") as f:
            f.write("# AI-Native Code Discipline Axioms\n\n## Axiom 1\nContent.")
        
        with open(os.path.join(tmpdir, "docs", "doctrine", "manifesto_axiom_coverage.csv"), "w") as f:
            f.write(matrix_valid)
        
        with open(os.path.join(tmpdir, "AGENTS.md"), "w") as f:
            f.write("# AGENTS.md\n\nSee docs/doctrine/ai-native-code-discipline-axioms.md\n")
        
        os.makedirs(os.path.join(tmpdir, ".clinerules"), exist_ok=True)
        with open(os.path.join(tmpdir, ".clinerules", "00-bootstrap.md"), "w") as f:
            f.write("# Bootstrap\n\nRead AGENTS.md and docs/epics/\n")
        
        with open(os.path.join(tmpdir, "docs", "epics", "test-epic.md"), "w") as f:
            f.write(epic_content)
    
    elif variant == "missing_anchor":
        os.makedirs(os.path.join(tmpdir, "docs", "doctrine"), exist_ok=True)
        with open(os.path.join(tmpdir, "docs", "doctrine", "ai-native-code-discipline-axioms.md"), "w") as f:
            f.write("# Axioms\n")
    
    elif variant == "missing_axiom1":
        os.makedirs(os.path.join(tmpdir, "docs", "doctrine"), exist_ok=True)
        os.makedirs(os.path.join(tmpdir, "docs", "epics"), exist_ok=True)
        os.makedirs(os.path.join(tmpdir, ".clinerules"), exist_ok=True)
        
        with open(os.path.join(tmpdir, "docs", "doctrine", "ai-native-code-discipline-axioms.md"), "w") as f:
            f.write("# Axioms\n")
        
        with open(os.path.join(tmpdir, "docs", "doctrine", "manifesto_axiom_coverage.csv"), "w") as f:
            f.write(matrix_without_axiom1)
        
        with open(os.path.join(tmpdir, "AGENTS.md"), "w") as f:
            f.write("# Agents\n")
        
        with open(os.path.join(tmpdir, ".clinerules", "00-bootstrap.md"), "w") as f:
            f.write("# Bootstrap\n")
    
    elif variant == "missing_epics_wal":
        os.makedirs(os.path.join(tmpdir, "docs", "doctrine"), exist_ok=True)
        os.makedirs(os.path.join(tmpdir, ".clinerules"), exist_ok=True)
        
        with open(os.path.join(tmpdir, "docs", "doctrine", "ai-native-code-discipline-axioms.md"), "w") as f:
            f.write("# Axioms\n")
        
        with open(os.path.join(tmpdir, "docs", "doctrine", "manifesto_axiom_coverage.csv"), "w") as f:
            f.write(matrix_without_epics)
        
        with open(os.path.join(tmpdir, "AGENTS.md"), "w") as f:
            f.write("# Agents\n")
        
        with open(os.path.join(tmpdir, ".clinerules", "00-bootstrap.md"), "w") as f:
            f.write("# Bootstrap\n")
    
    elif variant == "orphan_wal":
        os.makedirs(os.path.join(tmpdir, "docs", "doctrine"), exist_ok=True)
        os.makedirs(os.path.join(tmpdir, "docs", "epics"), exist_ok=True)
        os.makedirs(os.path.join(tmpdir, ".clinerules"), exist_ok=True)
        
        with open(os.path.join(tmpdir, "docs", "doctrine", "ai-native-code-discipline-axioms.md"), "w") as f:
            f.write("# Axioms\n")
        
        with open(os.path.join(tmpdir, "docs", "doctrine", "manifesto_axiom_coverage.csv"), "w") as f:
            f.write(matrix_valid)
        
        with open(os.path.join(tmpdir, "AGENTS.md"), "w") as f:
            f.write("# Agents\n")
        
        with open(os.path.join(tmpdir, ".clinerules", "00-bootstrap.md"), "w") as f:
            f.write("# Bootstrap\n")
        
        with open(os.path.join(tmpdir, "docs", "epics", "orphan-wal.md"), "w") as f:
            f.write("# Orphan WAL\n\nThis is a WAL file without an epic.\n")
    
    elif variant == "missing_bootstrap_ref":
        os.makedirs(os.path.join(tmpdir, "docs", "doctrine"), exist_ok=True)
        os.makedirs(os.path.join(tmpdir, ".clinerules"), exist_ok=True)
        
        with open(os.path.join(tmpdir, "docs", "doctrine", "ai-native-code-discipline-axioms.md"), "w") as f:
            f.write("# Axioms\n")
        
        with open(os.path.join(tmpdir, "docs", "doctrine", "manifesto_axiom_coverage.csv"), "w") as f:
            f.write(matrix_valid)
        
        with open(os.path.join(tmpdir, "AGENTS.md"), "w") as f:
            f.write("# Agents\n\nNo doctrine reference here.\n")
        
        with open(os.path.join(tmpdir, ".clinerules", "00-bootstrap.md"), "w") as f:
            f.write("# Bootstrap\n\nNo reference to AGENTS.md.\n")
    
    return tmpdir
