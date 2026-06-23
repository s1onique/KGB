#!/usr/bin/env python3
"""
CI Baseline validation module for memory budgets.

Validates:
- CI baseline entries reference real_evidence (not schema fixtures)
- Environment labels distinguish GitHub-hosted from router/self-hosted evidence
- Required evidence fields are present in CI baselines
- Artifact traceability: workflow run, artifact ID, artifact name, service, environment label
"""

import os
import json
from typing import List, Dict, Any, Optional


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
                
                for field in REQUIRED_CI_BASELINE_EVIDENCE_FIELDS:
                    if field not in source:
                        errors.append(
                            f"ci_idle_baselines.{env_label}.evidence_sources[{i}].{field} "
                            f"is required"
                        )
                
                if "environment_label" in source:
                    if source["environment_label"] != env_label:
                        errors.append(
                            f"ci_idle_baselines.{env_label}.evidence_sources[{i}]."
                            f"environment_label should be '{env_label}', "
                            f"got '{source['environment_label']}'"
                        )
                    
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
        return errors
    
    ci_baselines = data["ci_idle_baselines"]
    if not isinstance(ci_baselines, dict):
        errors.append(f"ci_idle_baselines must be a dict in {path}")
        return errors
    
    for env_label, baseline_data in ci_baselines.items():
        errors.extend(validate_ci_baseline_entry(env_label, baseline_data, path))
    
    return errors


def _load_artifact(artifact_path: str) -> tuple[List[str], Optional[Dict]]:
    """Load and parse a JSON artifact file. Returns (errors, data)."""
    errors = []
    data = None
    
    if not os.path.exists(artifact_path):
        errors.append(f"Artifact file does not exist: {artifact_path}")
        return errors, None
    
    try:
        with open(artifact_path, "r", encoding="utf-8") as f:
            data = json.load(f)
    except json.JSONDecodeError as e:
        errors.append(f"Artifact is not valid JSON: {artifact_path}: {e}")
        return errors, None
    
    if not isinstance(data, dict):
        errors.append(f"Artifact must be a JSON object: {artifact_path}")
        return errors, None
    
    return errors, data


def _validate_artifact_evidence_kind(artifact_path: str, artifact_data: Dict) -> List[str]:
    """Validate artifact evidence_kind field. Returns list of errors."""
    errors = []
    evidence_kind = artifact_data.get("evidence_kind")
    
    if evidence_kind is None:
        errors.append(f"Artifact missing evidence_kind field: {artifact_path}")
    elif evidence_kind != "real_evidence":
        errors.append(
            f"Artifact evidence_kind must be 'real_evidence', got '{evidence_kind}' "
            f"in {artifact_path}"
        )
    
    return errors


def _validate_artifact_service_name(
    artifact_path: str, 
    artifact_data: Dict, 
    expected_service: str
) -> List[str]:
    """Validate artifact service.name matches expected service. Returns list of errors."""
    errors = []
    
    if "service" not in artifact_data:
        errors.append(f"Artifact missing service field: {artifact_path}")
        return errors
    
    service_data = artifact_data["service"]
    if not isinstance(service_data, dict):
        errors.append(f"Artifact service must be a dict: {artifact_path}")
        return errors
    
    artifact_service_name = service_data.get("name")
    if artifact_service_name is None:
        errors.append(f"Artifact missing service.name field: {artifact_path}")
    elif artifact_service_name != expected_service:
        errors.append(
            f"Artifact service.name '{artifact_service_name}' does not match "
            f"budget service '{expected_service}' in {artifact_path}"
        )
    
    return errors


def _validate_artifact_environment_label(
    artifact_path: str, 
    artifact_data: Dict, 
    expected_label: str
) -> List[str]:
    """Validate artifact environment.environment_label matches expected label."""
    errors = []
    
    if "environment" not in artifact_data:
        errors.append(f"Artifact missing environment field: {artifact_path}")
        return errors
    
    env_data = artifact_data["environment"]
    if not isinstance(env_data, dict):
        errors.append(f"Artifact environment must be a dict: {artifact_path}")
        return errors
    
    artifact_label = env_data.get("environment_label")
    if artifact_label is None:
        errors.append(f"Artifact missing environment.environment_label field: {artifact_path}")
    elif artifact_label != expected_label:
        errors.append(
            f"Artifact environment.environment_label '{artifact_label}' does not match "
            f"baseline label '{expected_label}' in {artifact_path}"
        )
    
    return errors


def _validate_artifact_workflow_metadata(
    artifact_path: str, 
    artifact_data: Dict, 
    expected_workflow_run: int,
    expected_artifact_id: int,
    expected_artifact_name: str
) -> List[str]:
    """Validate artifact workflow/artifact metadata. Returns list of errors."""
    errors = []
    
    if "environment" not in artifact_data:
        errors.append(f"Artifact missing environment field: {artifact_path}")
        return errors
    
    env_data = artifact_data["environment"]
    
    artifact_workflow_run = env_data.get("_github_workflow_run")
    if artifact_workflow_run is None:
        errors.append(
            f"Artifact missing environment._github_workflow_run field: {artifact_path}"
        )
    elif artifact_workflow_run != expected_workflow_run:
        errors.append(
            f"Artifact _github_workflow_run {artifact_workflow_run} does not match "
            f"evidence_sources workflow_run {expected_workflow_run} in {artifact_path}"
        )
    
    artifact_id = env_data.get("_github_artifact_id")
    if artifact_id is None:
        errors.append(
            f"Artifact missing environment._github_artifact_id field: {artifact_path}"
        )
    elif artifact_id != expected_artifact_id:
        errors.append(
            f"Artifact _github_artifact_id {artifact_id} does not match "
            f"evidence_sources artifact_id {expected_artifact_id} in {artifact_path}"
        )
    
    artifact_name = env_data.get("_github_artifact_name")
    if artifact_name is None:
        errors.append(
            f"Artifact missing environment._github_artifact_name field: {artifact_path}"
        )
    elif artifact_name != expected_artifact_name:
        errors.append(
            f"Artifact _github_artifact_name '{artifact_name}' does not match "
            f"evidence_sources artifact_name '{expected_artifact_name}' in {artifact_path}"
        )
    
    return errors


def check_ci_baseline_evidence_exists(
    budget_data: Dict, 
    budget_path: str, 
    repo_root: str
) -> List[str]:
    """
    Check that CI baseline evidence artifacts exist and are valid.
    
    Returns list of hard errors for:
    - No artifact exists for referenced workflow run
    - Artifact is not valid JSON
    - evidence_kind != real_evidence
    - Artifact service.name doesn't match budget file service
    - Artifact environment.environment_label doesn't match baseline label
    - Artifact workflow/artifact metadata doesn't match evidence_sources entry
    """
    errors = []
    
    if "ci_idle_baselines" not in budget_data:
        return errors
    
    budget_service = budget_data.get("service", "")
    
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
            
            if budget_service == "tovarisch":
                evidence_dir = os.path.join(repo_root, "artifacts", "memory-labs", "tovarisch")
            elif budget_service == "uvb76":
                evidence_dir = os.path.join(repo_root, "artifacts", "memory-labs", "uvb76")
            else:
                errors.append(
                    f"Cannot verify artifact traceability: unknown service '{budget_service}' "
                    f"in {budget_path}"
                )
                continue
            
            artifact_path = None
            if os.path.isdir(evidence_dir):
                for entry in os.listdir(evidence_dir):
                    if str(workflow_run) in entry and entry.endswith(".json"):
                        artifact_path = os.path.join(evidence_dir, entry)
                        break
            
            if artifact_path is None:
                errors.append(
                    f"No artifact found for workflow run {workflow_run} in {evidence_dir} "
                    f"(expected artifact name containing '{artifact_name}')"
                )
                continue
            
            load_errors, artifact_data = _load_artifact(artifact_path)
            errors.extend(load_errors)
            
            if artifact_data is None:
                continue
            
            errors.extend(_validate_artifact_evidence_kind(artifact_path, artifact_data))
            errors.extend(_validate_artifact_service_name(
                artifact_path, artifact_data, budget_service
            ))
            errors.extend(_validate_artifact_environment_label(
                artifact_path, artifact_data, env_label
            ))
            errors.extend(_validate_artifact_workflow_metadata(
                artifact_path, 
                artifact_data, 
                workflow_run,
                artifact_id,
                artifact_name
            ))
    
    return errors
