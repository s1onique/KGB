#!/usr/bin/env python3
"""
Test fixtures for verify_cold_resume_checkpoints.py self-tests.

This module contains only the test fixture data for self-testing.
Separated to keep the main verifier under the line-count limit.
"""


# Test fixtures for self-tests
# Each fixture is a tuple of (name, epic_content, allowlist_content, should_pass)

TEST_FIXTURES = [
    {
        "name": "valid closed checkpoint passes",
        "epic_content": """# [Closed] ACT 1: Test Work

## Status

**Status: Complete**

### Summary

Test work completed.

### Files Changed

- test.zig

### Verification

- make gate passes

### Caveats

None.

### Next Step

ACT 2: Next work.
""",
        "allowlist_content": "",
        "should_pass": True,
    },
    {
        "name": "valid open checkpoint passes",
        "epic_content": """# [Open] ACT 2: More Work

## Status

**Status: Open**

### Next Step

Complete remaining items.
""",
        "allowlist_content": "",
        "should_pass": True,
    },
    {
        "name": "closed checkpoint missing files_changed fails",
        "epic_content": """# [Closed] ACT 3: Incomplete Work

## Status

**Status: Complete**

### Summary

Work done.

### Verification

- make gate passes

### Caveats

None.

### Next Step

Done.
""",
        "allowlist_content": "",
        "should_pass": False,
    },
    {
        "name": "closed checkpoint missing verification fails",
        "epic_content": """# [Closed] ACT 4: No Verification

## Status

**Status: Complete**

### Summary

Work done.

### Files Changed

- test.zig

### Caveats

None.

### Next Step

Done.
""",
        "allowlist_content": "",
        "should_pass": False,
    },
    {
        "name": "closed checkpoint missing caveats fails",
        "epic_content": """# [Closed] ACT 5: No Caveats

## Status

**Status: Complete**

### Summary

Work done.

### Files Changed

- test.zig

### Verification

- make gate passes

### Next Step

Done.
""",
        "allowlist_content": "",
        "should_pass": False,
    },
    {
        "name": "closed checkpoint missing next_step fails",
        "epic_content": """# [Closed] ACT 6: No Next Step

## Status

**Status: Complete**

### Summary

Work done.

### Files Changed

- test.zig

### Verification

- make gate passes

### Caveats

None.
""",
        "allowlist_content": "",
        "should_pass": False,
    },
    {
        "name": "legacy allowlisted incomplete checkpoint passes with debt reported",
        "epic_content": """# Epic: Old Work

## ACT Old

**Status: Complete**

Work was done but no close report.
""",
        "allowlist_content": (
            "path,reason,planned_resolution,allowed_missing_markers\n"
            'test-epic.md,Historical,Not applicable,"summary,files_changed,verification,caveats,next_step"\n'
        ),
        "should_pass": True,
    },
    {
        "name": "multiple checkpoint blocks in one file pass",
        "epic_content": """# Epic: Multi-ACT

## ACT 1: First Work

**Status: Complete**

### Summary

First done.

### Files Changed

- a.zig

### Verification

- make gate

### Caveats

None.

### Next Step

ACT 2.

## ACT 2: Second Work

**Status: Complete**

### Summary

Second done.

### Files Changed

- b.zig

### Verification

- make gate

### Caveats

None.

### Next Step

Done.
""",
        "allowlist_content": "",
        "should_pass": True,
    },
    {
        "name": "no checkpoint block detected passes",
        "epic_content": """# Epic: Historical

## ACT Old

**Status: Complete**

This was done before close reports were required.
""",
        "allowlist_content": "",
        "should_pass": True,
    },
]
