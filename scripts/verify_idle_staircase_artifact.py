#!/usr/bin/env python3
# verify_idle_staircase_artifact.py — Thin CLI wrapper for idle staircase artifact verifier
#
# Usage:
#   python scripts/verify_idle_staircase_artifact.py <artifact_dir>
#   python scripts/verify_idle_staircase_artifact.py --self-test
#
# Exit codes:
#   0 - Artifact is valid
#   1 - Artifact validation failed (with reason)

import argparse
import sys
from pathlib import Path

from idle_staircase_verifier import verify_artifact, run_self_tests


def main():
    parser = argparse.ArgumentParser(
        description="Verify idle staircase memory lab artifacts"
    )
    parser.add_argument(
        "artifact_dir",
        nargs="?",
        help="Path to artifact directory to verify"
    )
    parser.add_argument(
        "--self-test",
        action="store_true",
        help="Run self-tests on the verifier"
    )
    
    args = parser.parse_args()
    
    if args.self_test:
        success = run_self_tests()
        sys.exit(0 if success else 1)
    
    if not args.artifact_dir:
        parser.print_help()
        sys.exit(1)
    
    artifact_path = Path(args.artifact_dir)
    valid, error = verify_artifact(artifact_path)
    
    if valid:
        print(f"OK: Artifact is valid: {artifact_path}")
        sys.exit(0)
    else:
        print(f"ERROR: {error}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
