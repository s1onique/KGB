#!/usr/bin/env python3
# lab_tovarisch_idle_memory.py — Entry point for idle staircase memory lab
#
# Thin wrapper that delegates to the lab_runner package.
# All lab logic is in scripts/lab_runner/

import sys
from lab_runner.main import main

if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
