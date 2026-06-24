# idle_staircase_verifier/__init__.py — Verifier package for idle staircase artifacts
"""
Idle Staircase Artifact Verifier Package

Verifies memory lab artifacts from the tovarisch idle staircase lab.
Rejects artifacts with shell-side synthetic events claiming confirmed_leak.

Usage:
    from idle_staircase_verifier import verify_artifact, run_self_tests
"""

from .artifact_checks import verify_artifact
from .self_tests import run_self_tests

__all__ = ["verify_artifact", "run_self_tests"]
