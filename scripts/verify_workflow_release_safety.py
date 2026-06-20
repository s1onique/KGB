#!/usr/bin/env python3
"""Verifies GitHub Actions workflows follow release barrier doctrine."""

import argparse, re, sys, tempfile
from pathlib import Path
from typing import Optional

# Violation IDs
VID_BUILD_RELEASE_CREATE = "BUILD-RELEASE-CREATE"
VID_BUILD_RELEASE_UPLOAD = "BUILD-RELEASE-UPLOAD"
VID_BUILD_RELEASE_EDIT = "BUILD-RELEASE-EDIT"
VID_PUBLISH_NO_NEEDS = "PUBLISH-NO-NEEDS"
VID_WORKFLOW_LEVEL_WRITE = "WORKFLOW-LEVEL-WRITE"
VID_LEGACY_TAG_PATTERN = "LEGACY-TAG-PATTERN"
VID_MULTIPLE_PUBLISH_JOBS = "MULTIPLE-PUBLISH-JOBS"
VID_MISSING_TAG_PATTERN = "MISSING-TAG-PATTERN"

LEGACY_TAG_PATTERNS = [r'^v\*$', r'^v\d+\.\d+\.\d+.*-opkg-.*$', r'^v\d+\.\d+\.\d+.*-deb-.*$']
APPROVED_TAG_WORKFLOWS = {"release-opkg.yml", "tovarisch-deb-release.yml"}
RELEASE_WORKFLOWS = {"release-opkg.yml", "tovarisch-deb-release.yml", "tovarisch-release.yml",
                     "test-release-opkg.yml", "test-tovarisch-deb-release.yml"}


class Violation:
    def __init__(self, vid: str, msg: str, wf: str, job: Optional[str] = None):
        self.violation_id, self.message, self.workflow, self.job = vid, msg, wf, job

    def __str__(self) -> str:
        j = f" job '{self.job}'" if self.job else ""
        return f"VIOLATION [{self.violation_id}]{j} in {self.workflow}: {self.message}"


class Job:
    def __init__(self, name: str, start_line: int):
        self.name, self.start_line, self.permissions = name, start_line, None
        self.needs: list[str] = []
        self.has_gh_release_create = self.has_gh_release_upload = self.has_gh_release_edit = False


class WorkflowParser:
    def __init__(self, workflow_path: Path):
        self.path = workflow_path
        self.content = workflow_path.read_text()
        self.lines = self.content.split('\n')
        self.name = self._extract_workflow_name()
        self.workflow_permissions = self._extract_workflow_permissions()
        self.tag_patterns = self._extract_tag_patterns()
        self.jobs = self._extract_jobs()

    def _get_indent(self, line: str) -> int:
        return len(line) - len(line.lstrip())

    def _extract_workflow_name(self) -> Optional[str]:
        for line in self.lines:
            stripped = line.strip()
            if stripped.startswith('name:'):
                val = stripped[5:].strip().strip('"\'')
                if val:
                    return val
        return None

    def _extract_workflow_permissions(self) -> Optional[str]:
        for i, line in enumerate(self.lines):
            if re.match(r'^jobs:\s*$', line):
                break
            if re.search(r'permissions:\s*\{[^}]*contents:\s*write', line):
                return 'write'
            if line.strip() == 'permissions:':
                for j in range(i + 1, min(i + 5, len(self.lines))):
                    next_line = self.lines[j].strip()
                    if next_line == 'contents: write':
                        return 'write'
                    if next_line == 'contents: read':
                        return 'read'
                    if next_line and not next_line.startswith('#') and re.match(r'^[a-z-]+:', next_line):
                        break
        return None

    def _extract_tag_patterns(self) -> list:
        patterns, in_on, in_push, in_tags = [], False, False, False
        for line in self.lines:
            stripped, indent = line.strip(), self._get_indent(line)
            if not in_on and stripped.startswith('on:'):
                in_on, in_push, in_tags = True, False, False
                continue
            if in_on and indent == 0 and re.match(r'^[a-z-]+:', stripped) and not stripped.startswith('on:'):
                in_on, in_push, in_tags = False, False, False
            if in_on and stripped == 'push:':
                in_push, in_tags = True, False
                continue
            if in_push and stripped == 'tags:':
                in_tags = True
                continue
            if in_tags:
                if m := re.match(r'^\s*-\s+"([^"]+)"', stripped):
                    patterns.append(m.group(1))
                    continue
                if stripped and not stripped.startswith('#') and not re.match(r'^\s*-\s+', stripped):
                    in_tags = False
        return patterns

    def _extract_jobs(self) -> dict:
        jobs, current_job, in_jobs, in_permissions, in_needs = {}, None, False, False, False
        for line_num, line in enumerate(self.lines, 1):
            stripped, stripped_no_ws = line.rstrip(), line.strip()
            indent = self._get_indent(line)  # Must use original line for indent
            if re.match(r'^jobs:\s*$', stripped):
                in_jobs = True
                continue
            if not in_jobs:
                continue
            if indent == 2 and (m := re.match(r'^\s*([a-zA-Z][-a-zA-Z0-9_]*):\s*$', stripped)):
                if current_job:
                    jobs[current_job.name] = current_job
                current_job = Job(m.group(1), line_num)
                in_permissions = in_needs = False
                continue
            if current_job:
                if stripped == 'permissions:' or re.match(r'^\s+permissions:\s*$', stripped):
                    in_permissions, in_needs = True, False
                    continue
                if re.match(r'^\s*permissions:\s*\{', stripped):
                    if 'contents: write' in stripped:
                        current_job.permissions = 'write'
                    elif 'contents: read' in stripped:
                        current_job.permissions = 'read'
                    continue
                if stripped == 'needs:' or re.match(r'^\s+needs:\s*$', stripped):
                    in_needs, in_permissions = True, False
                    continue
                if re.match(r'^\s*needs:\s*\[', stripped) or re.match(r'^\s*needs:\s+[a-zA-Z]', stripped):
                    if m := re.search(r'\[([^\]]+)\]', stripped):
                        current_job.needs.extend(re.findall(r'([a-zA-Z][-a-zA-Z0-9_]+)', m.group(1)))
                    elif m := re.match(r'^\s*needs:\s+([a-zA-Z][-a-zA-Z0-9_]+)', stripped):
                        current_job.needs.append(m.group(1))
                    in_needs, in_permissions = True, False
                    continue
                # Fixed: use stripped_no_ws for key detection
                if indent == 4 and stripped_no_ws and not stripped_no_ws.startswith('#'):
                    if re.match(r'^[a-z-]+:', stripped_no_ws):
                        in_permissions = in_needs = False
                if in_needs:
                    if m := re.match(r'^\s+-\s+([a-zA-Z][-a-zA-Z0-9_]*)', stripped):
                        current_job.needs.append(m.group(1))
                    continue
                if in_permissions:
                    if re.search(r'\bcontents:\s*write\b', stripped):
                        current_job.permissions = 'write'
                    elif re.search(r'\bcontents:\s*read\b', stripped):
                        current_job.permissions = 'read'
                    continue
                if 'gh release create' in stripped:
                    current_job.has_gh_release_create = True
                if 'gh release upload' in stripped:
                    current_job.has_gh_release_upload = True
                if 'gh release edit' in stripped:
                    current_job.has_gh_release_edit = True
        if current_job:
            jobs[current_job.name] = current_job
        return jobs

    def is_release_workflow(self) -> bool:
        return self.path.name in RELEASE_WORKFLOWS

    def requires_tag_pattern(self) -> bool:
        return self.path.name in APPROVED_TAG_WORKFLOWS


def check_workflow(workflow_path: Path, skip_sentinel: bool = True,
                   self_test_mode: bool = False) -> list[Violation]:
    violations, workflow_name = [], workflow_path.name
    if skip_sentinel and not self_test_mode and ('sentinel' in workflow_name or 'bad-workflow' in workflow_name):
        return violations
    try:
        workflow = WorkflowParser(workflow_path)
    except Exception as e:
        violations.append(Violation("PARSE-ERROR", f"Failed to parse: {e}", workflow_name))
        return violations

    publish_jobs = {jn for jn, j in workflow.jobs.items() if j.permissions == 'write'}

    # Multiple publish jobs violation
    if len(publish_jobs) > 1:
        violations.append(Violation(VID_MULTIPLE_PUBLISH_JOBS,
            f"Multiple publish jobs: {sorted(publish_jobs)}", workflow_name))

    # Workflow-level write: release workflows always fail, others only if no publish jobs
    if workflow.workflow_permissions == 'write':
        if workflow.is_release_workflow():
            violations.append(Violation(VID_WORKFLOW_LEVEL_WRITE,
                "Release workflow has workflow-level write; use job-level only", workflow_name))
        elif not publish_jobs:
            violations.append(Violation(VID_WORKFLOW_LEVEL_WRITE,
                "Workflow-level write but no publish jobs", workflow_name))

    for job_name, job in workflow.jobs.items():
        is_publish = job_name in publish_jobs
        if not is_publish:
            if job.has_gh_release_create:
                violations.append(Violation(VID_BUILD_RELEASE_CREATE,
                    "Build job has 'gh release create'", workflow_name, job_name))
            if job.has_gh_release_upload:
                violations.append(Violation(VID_BUILD_RELEASE_UPLOAD,
                    "Build job has 'gh release upload'", workflow_name, job_name))
            if job.has_gh_release_edit:
                violations.append(Violation(VID_BUILD_RELEASE_EDIT,
                    "Build job has 'gh release edit'", workflow_name, job_name))
        if is_publish and not job.needs:
            violations.append(Violation(VID_PUBLISH_NO_NEEDS,
                "Publish job has no 'needs' dependency", workflow_name, job_name))

    if workflow.is_release_workflow():
        # Missing tag pattern violation
        if workflow.requires_tag_pattern() and not workflow.tag_patterns:
            violations.append(Violation(VID_MISSING_TAG_PATTERN,
                "Release workflow requires push tag pattern", workflow_name))
        for pattern in workflow.tag_patterns:
            for lp in LEGACY_TAG_PATTERNS:
                if re.match(lp, pattern):
                    violations.append(Violation(VID_LEGACY_TAG_PATTERN,
                        f"Legacy tag pattern '{pattern}'", workflow_name))
                    break
            if workflow.requires_tag_pattern():
                approved = r"^uvb76-opkg-v\*$" if "opkg" in workflow_name else r"^tovarisch-deb-v\*$"
                if not re.match(approved, pattern):
                    violations.append(Violation(VID_LEGACY_TAG_PATTERN,
                        f"Non-approved tag pattern '{pattern}'", workflow_name))
    return violations


def run_self_test() -> bool:
    from verify_workflow_release_safety_fixtures import FIXTURES
    print("[verify-workflow-release-safety] SELF-TEST MODE\n")
    with tempfile.TemporaryDirectory() as tmpdir:
        workflows_dir = Path(tmpdir) / ".github" / "workflows"
        workflows_dir.mkdir(parents=True)
        expected_counts = {}
        for filename, content, violations_list in FIXTURES:
            (workflows_dir / filename).write_text(content)
            for vid in violations_list:
                expected_counts[vid] = expected_counts.get(vid, 0) + 1
        print("Running verifier on self-test fixtures...\n")
        all_violations = {}
        for wf in sorted(workflows_dir.glob("*.yml")) + sorted(workflows_dir.glob("*.yaml")):
            for v in check_workflow(wf, skip_sentinel=False, self_test_mode=True):
                all_violations[v.violation_id] = all_violations.get(v.violation_id, 0) + 1
                print(f"  {v}")
        print("\n=== Self-test results ===")
        test_passed = True
        for vid, expected in sorted(expected_counts.items()):
            actual = all_violations.get(vid, 0)
            if actual != expected:
                print(f"FAIL: Expected {expected}x '{vid}', found {actual}")
                test_passed = False
            else:
                print(f"OK: Found {actual}x '{vid}'")
        for vid, actual in sorted(all_violations.items()):
            if expected_counts.get(vid, 0) < actual:
                print(f"FAIL: Unexpected '{vid}' ({actual}x, expected {expected_counts.get(vid, 0)}x)")
                test_passed = False
        print()
        print(f"SELF-TEST {'PASS' if test_passed else 'FAIL'}")
        return test_passed


def main():
    parser = argparse.ArgumentParser(description="Verify GitHub Actions workflow release safety")
    parser.add_argument("--self-test", action="store_true", help="Run self-test")
    parser.add_argument("--workflows-dir", default=None, help="Path to workflows directory")
    parser.add_argument("--include-sentinel", action="store_true", help="Include sentinel files")
    args = parser.parse_args()
    if args.self_test:
        sys.exit(0 if run_self_test() else 2)
    script_dir = Path(__file__).parent
    workflows_dir = Path(args.workflows_dir) if args.workflows_dir else script_dir.parent / ".github" / "workflows"
    if not workflows_dir.exists():
        print(f"[verify-workflow-release-safety] ERROR: workflows dir not found: {workflows_dir}", file=sys.stderr)
        sys.exit(2)
    skip_sentinel = not args.include_sentinel
    print("[verify-workflow-release-safety] starting workflow release safety verification\n")
    all_violations = []
    for wf in sorted(workflows_dir.glob("*.yml")) + sorted(workflows_dir.glob("*.yaml")):
        wf_name = wf.name
        if skip_sentinel and ('sentinel' in wf_name or 'bad-workflow' in wf_name):
            print(f"  SKIP: {wf_name} (sentinel)")
            continue
        print(f"  checking: {wf_name}")
        for v in check_workflow(wf, skip_sentinel=skip_sentinel):
            all_violations.append(v)
            print(f"    {v}", file=sys.stderr)
    print()
    if all_violations:
        unique = len(set(v.violation_id for v in all_violations))
        print(f"[verify-workflow-release-safety] FAIL: {len(all_violations)} violation(s) ({unique} types)", file=sys.stderr)
        sys.exit(1)
    print("[verify-workflow-release-safety] PASS: No release-in-build violations detected")
    sys.exit(0)


if __name__ == "__main__":
    main()
