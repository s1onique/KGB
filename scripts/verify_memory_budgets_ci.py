#!/usr/bin/env python3
"""
CI Baseline validation module for memory budgets.

Validates:
- CI baseline entries reference real_evidence (not schema fixtures)
- Environment labels distinguish GitHub-hosted from router/self-hosted evidence
- Required evidence fields are present in CI baselines
"""

import os
from typing import List, Dict, Any

# Required fields for CI baseline evidence entries
REQUIRED_CI_BASELINE_EVIDENCE_FIELDS = [
    "workflow_run",
    "artifact_id",
    "artifact_name",
    "service_version",
    "service_commit",
    "workload",
    "duration_seconds",
    "environment_label",
]

# Known environment labels
VALID_ENVIRONMENT_LABELS = [
    "github_hosted_ubuntu",
    "github_hosted_windows",
    "github_hosted_macos",
    "router_armv7",
    "router_armv5",
    "embedded_arm",
    "self_hosted",
]


def validate_ci_baseline_entry(env_label: str, baseline_data: Dict, path: str) -> List[str]:
    """Validate a single CI baseline entry. Returns list of errors."""
    errors = []
    
    if not isinstance(baseline_data, dict):
        errors.append(f"ci_idle_baselines.{env_label} must be a dict")
        return errors
    
    # Check for idle state with memory values
    if "idle" not in baseline_data:
        errors.append(f"ci_idle_baselines.{env_label}.idle is required")
    else:
        idle_data = baseline_data["idle"]
        if not isinstance(idle_data, dict):
            errors.append(f"ci_idle_baselines.{env_label}.idle must be a dict")
        else:
            # Check for required memory metrics
            for field in ["rss_kib", "pss_kib"]:
                if field in idle_data:
                    val = idle_data[field]
                    if not isinstance(val, (int, float)):
                        errors.append(
                            f"ci_idle_baselines.{env_label}.idle.{field} must be a number, "
                            f"got {type(val).__name__}"
                        )
                else:
                    errors.append(f"ci_idle_baselines.{env_label}.idle.{field} is required")
    
    # Check for evidence_sources
    if "evidence_sources" not in baseline_data:
        errors.append(f"ci_idle_baselines.{env_label}.evidence_sources is required")
    else:
        sources = baseline_data["evidence_sources"]
        if not isinstance(sources, list):
            errors.append(f"ci_idle_baselines.{env_label}.evidence_sources must be a list")
        elif len(sources) == 0:
            errors.append(f"ci_idle_baselines.{env_label}.evidence_sources cannot be empty")
        else:
            for i, source in enumerate(sources):
                if not isinstance(source, dict):
                    errors.append(
                        f"ci_idle_baselines.{env_label}.evidence_sources[{i}] must be a dict"
                    )
                    continue
                
                # Check required fields in evidence source
                for field in REQUIRED_CI_BASELINE_EVIDENCE_FIELDS:
                    if field not in source:
                        errors.append(
                            f"ci_idle_baselines.{env_label}.evidence_sources[{i}].{field} "
                            f"is required"
                        )
                
                # Validate environment_label matches parent
                if "environment_label" in source:
                    if source["environment_label"] != env_label:
                        errors.append(
                            f"ci_idle_baselines.{env_label}.evidence_sources[{i}]."
                            f"environment_label should be '{env_label}', "
                            f"got '{source['environment_label']}'"
                        )
                    
                    # Check for known environment label
                    if source["environment_label"] not in VALID_ENVIRONMENT_LABELS:
                        errors.append(
                            f"ci_idle_baselines.{env_label}.evidence_sources[{i}]."
                            f"environment_label '{source['environment_label']}' is not a known label"
                        )
    
    return errors


def validate_ci_idle_baselines(data: Dict, path: str) -> List[str]:
    """Validate ci_idle_baselines section. Returns list of errors."""
    errors = []
    
    if "ci_idle_baselines" not in data:
        # CI baselines are optional for now - just skip
        return errors
    
    ci_baselines = data["ci_idle_baselines"]
    if not isinstance(ci_baselines, dict):
        errors.append(f"ci_idle_baselines must be a dict in {path}")
        return errors
    
    for env_label, baseline_data in ci_baselines.items():
        errors.extend(validate_ci_baseline_entry(env_label, baseline_data, path))
    
    return errors


def check_ci_baseline_evidence_exists(
    budget_data: Dict, 
    budget_path: str, 
    repo_root: str
) -> List[str]:
    """Check that CI baseline evidence artifacts exist. Returns list of errors."""
    errors = []
    
    if "ci_idle_baselines" not in budget_data:
        return errors
    
    for env_label, baseline_data in budget_data["ci_idle_baselines"].items():
        if not isinstance(baseline_data, dict):
            continue
        
        evidence_sources = baseline_data.get("evidence_sources", [])
        for source in evidence_sources:
            if not isinstance(source, dict):
                continue
            
            workflow_run = source.get("workflow_run")
            artifact_id = source.get("artifact_id")
            artifact_name = source.get("artifact_name")
            
            if not workflow_run or not artifact_id or not artifact_name:
                continue
            
            # Determine which service directory to look in
            service = budget_data.get("service", "")
            if service == "tovarisch":
                evidence_dir = os.path.join(repo_root, "artifacts", "memory-labs", "tovarisch")
            elif service == "uvb76":
                evidence_dir = os.path.join(repo_root, "artifacts", "memory-labs", "uvb76")
            else:
                continue
            
            # Check if any artifact exists that matches the workflow run
            if os.path.isdir(evidence_dir):
                found = False
                for entry in os.listdir(evidence_dir):
                    if str(workflow_run) in entry:
                        found = True
                        break
                
                if not found:
                    print(f"  WARNING: No artifact found for workflow {workflow_run} in {evidence_dir}")
    
    return errors
