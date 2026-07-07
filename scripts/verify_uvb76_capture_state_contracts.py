#!/usr/bin/env python3
"""
Verifier for UVB-76 HULK02 Diagnostic Capture State Machine Contracts.

This verifier validates that HULK02 capture state contracts exist and conform to expected structure:
- All HULK02 contract files exist and contain proper test patterns
- No unallowlisted t.Skip/t.Skipf in HULK02 contract tests
- Fake backend is used in capture service unit contracts
- Real tcpdump/ss/ip command execution is NOT introduced in unit contract tests
- Makefile exposes hulk-uvb76-capture-gate
- hulk-uvb76-capture-gate runs go test for diagnostics/state/server capture contracts
- Files do not exceed 450-line LLM-friendliness limit

Supports self-test mode with fixture validation.

ACT-UVB76-HULK02-DIAGNOSTIC-CAPTURE-STATE-MACHINE
ACT-UVB76-HULK02R2-CAPTURE-CONTRACT-FILE-SPLIT
ACT-UVB76-HULK02R3-CAPTURE-STATE-VERIFIER-LINE-LIMIT-SPLIT
"""

from uvb76_capture_state_contracts.runner import run

if __name__ == "__main__":
    raise SystemExit(run())
