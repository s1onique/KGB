#!/usr/bin/env python3
"""
Shell Containment Verifier

Thin wrapper that imports and runs the verifier package.
See verify_shell_containment/ for implementation details.
"""

from verify_shell_containment.cli import main

if __name__ == "__main__":
    raise SystemExit(main())
