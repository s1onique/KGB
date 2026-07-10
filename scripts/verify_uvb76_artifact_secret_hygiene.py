#!/usr/bin/env python3
"""
Verifier for UVB-76 Artifact Secret Hygiene (HULK05).

This verifier enforces the artifact-secret-hygiene contract:
- No tracked artifact may retain authentication credentials
- No session material, private key material, or sensitive configuration values
- No credential-bearing URLs and headers
- Sanitized artifacts remain structurally useful using canonical [REDACTED] marker

Supports two modes:
- Normal verification: scans tracked files for prohibited secret patterns
- Self-test mode: validates verifier behavior without committing fixtures

ACT-UVB76-HULK05-ARTIFACT-SECRET-HYGIENE

Delegates to uvb76_artifact_secret_hygiene package modules.
"""

import os
import sys

# Import from package modules
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, SCRIPT_DIR)

from uvb76_artifact_secret_hygiene.main import main

if __name__ == "__main__":
    main()
