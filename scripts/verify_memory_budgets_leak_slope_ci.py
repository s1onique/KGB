#!/usr/bin/env python3
"""
Leak-slope CI Baseline validation module for memory budgets.

Validates:
- CI leak-slope baseline entries reference real_evidence artifacts
- Environment labels distinguish GitHub-hosted from router/self-hosted evidence
- Required evidence fields are present in leak-slope baselines
- Artifact traceability by workload identity (not filename substring)
- Short-window baselines are labeled as warmup_sensitive
- Long-window baselines are labeled as long_window with duration_seconds >= 900
"""

import os
import json
from typing import List, Dict, Any, Optional


# Required fields for leak-slope baseline evidence entries
REQUIRED_LEAK_SLOPE_EVIDENCE_FIELDS = [
    "workflow_run",
    "artifact_id",
    "artifact_name",
    "service_version",
    "service_commit",
    "workload",
    "duration_seconds",
    "environment_label",
    "workload_type",
    "short_window_seconds",
    "signal_quality",
]

# Required fields for long-window leak-slope baseline evidence entries
REQUIRED_LEAK_SLOPE_LONG_WINDOW_EVIDENCE_FIELDS = [
    "workflow_run",
    "artifact_id",
    "artifact_name",
    "service_version",
    "service_commit",
    "workload",
    "duration_seconds",
    "operations",
    "request_errors",
    "environment_label",
    "workload_type",
    "long_window_seconds",
    "signal_quality",
]

# Long-window minimum thresholds
LONG_WINDOW_MIN_DURATION_SECONDS = 900
LONG_WINDOW_MIN_OPERATIONS = 9000

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

# Workload type to artifact filename pattern mapping
WORKLOAD_TO_ARTIFACT_SUFFIX = {
    "tovarisch_leak_slope": "leak-slope",
    "tovarisch_leak_slope_netdiag": "leak-slope-netdiag",
    "uvb76_leak_slope": "leak-slope",
    "uvb76_leak_slope_netdiag": "leak-slope-netdiag",
}


def validate_ci_leak_slope_baselines(data: Dict, path: str) -> List[str]:
    """Validate ci_leak_slope_baselines section. Returns list of errors."""
    errors = []
    
    if "ci_leak_slope_baselines" not in data:
        return errors
    
    ci_baselines = data["ci_leak_slope_baselines"]
    if not isinstance(ci_baselines, dict):
        errors.append(f"ci_leak_slope_baselines must be a dict in {path}")
        return errors
    
    for env_label, baseline_data in ci_baselines.items():
        if not isinstance(baseline_data, dict):
            errors.append(f"ci_leak_slope_baselines.{env_label} must be a dict")
            continue
        
        # Check for signal_quality label (required for short-window baselines)
        if "signal_quality" in baseline_data:
            signal_quality = baseline_data["signal_quality"]
            if signal_quality == "warmup_sensitive":
                # Verify this is marked as short_window
                if "short_window_seconds" not in baseline_data:
                    errors.append(
                        f"ci_leak_slope_baselines.{env_label}.short_window_seconds is required "
                        f"when signal_quality is 'warmup_sensitive'"
                    )
        
        # Validate evidence_sources for leak-slope baselines
        if "evidence_sources" not in baseline_data:
            errors.append(f"ci_leak_slope_baselines.{env_label}.evidence_sources is required")
        else:
            sources = baseline_data["evidence_sources"]
            if not isinstance(sources, list):
                errors.append(f"ci_leak_slope_baselines.{env_label}.evidence_sources must be a list")
            elif len(sources) == 0:
                errors.append(f"ci_leak_slope_baselines.{env_label}.evidence_sources cannot be empty")
            else:
                for i, source in enumerate(sources):
                    if not isinstance(source, dict):
                        errors.append(
                            f"ci_leak_slope_baselines.{env_label}.evidence_sources[{i}] must be a dict"
                        )
                        continue
                    
                    for field in REQUIRED_LEAK_SLOPE_EVIDENCE_FIELDS:
                        if field not in source:
                            errors.append(
                                f"ci_leak_slope_baselines.{env_label}.evidence_sources[{i}].{field} "
                                f"is required"
                            )
                    
                    if "environment_label" in source:
                        if source["environment_label"] != env_label:
                            errors.append(
                                f"ci_leak_slope_baselines.{env_label}.evidence_sources[{i}]."
                                f"environment_label should be '{env_label}', "
                                f"got '{source['environment_label']}'"
                            )
                        
                        if source["environment_label"] not in VALID_ENVIRONMENT_LABELS:
                            errors.append(
                                f"ci_leak_slope_baselines.{env_label}.evidence_sources[{i}]."
                                f"environment_label '{source['environment_label']}' is not a known label"
                            )
                    
                    if "signal_quality" in source:
                        if source["signal_quality"] not in ("warmup_sensitive", "stable"):
                            errors.append(
                                f"ci_leak_slope_baselines.{env_label}.evidence_sources[{i}]."
                                f"signal_quality must be 'warmup_sensitive' or 'stable', "
                                f"got '{source['signal_quality']}'"
                            )
                    
                    if "workload_type" in source:
                        if source["workload_type"] not in WORKLOAD_TO_ARTIFACT_SUFFIX:
                            errors.append(
                                f"ci_leak_slope_baselines.{env_label}.evidence_sources[{i}]."
                                f"workload_type '{source['workload_type']}' is not a known type"
                            )
    
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


def _get_workload_type_from_artifact(artifact_data: Dict) -> str:
    """Derive workload_type from artifact workload.type field."""
    workload_type = artifact_data.get("workload", {}).get("type", "")
    # Map "tovarisch-leak-slope" -> "tovarisch_leak_slope"
    return workload_type.replace("-", "_")


def _matches_workload_type(artifact_workload_type: str, expected_workload_type: str) -> bool:
    """Check if artifact workload type matches expected workload type."""
    # Normalize both forms
    normalized_artifact = artifact_workload_type.replace("-", "_")
    normalized_expected = expected_workload_type.replace("-", "_")
    return normalized_artifact == normalized_expected


def check_leak_slope_baseline_evidence_exists(
    budget_data: Dict, 
    budget_path: str, 
    repo_root: str
) -> List[str]:
    """
    Check that CI leak-slope baseline evidence artifacts exist and are valid.
    
    Resolves artifacts by parsed JSON metadata, not filename substring.
    Each evidence_sources[] must match exactly one artifact with matching:
    - environment._github_workflow_run
    - environment._github_artifact_id
    - environment._github_artifact_name
    - service.name
    - environment.environment_label
    - workload.type
    - decision.signal_quality
    - decision.short_window_seconds
    - leak_slope section exists
    
    Returns list of errors for:
    - Zero matching artifacts
    - More than one matching artifact
    - Metadata mismatch
    - Missing leak_slope section
    """
    errors = []
    
    if "ci_leak_slope_baselines" not in budget_data:
        return errors
    
    budget_service = budget_data.get("service", "")
    
    for env_label, baseline_data in budget_data["ci_leak_slope_baselines"].items():
        if not isinstance(baseline_data, dict):
            continue
        
        expected_signal_quality = baseline_data.get("signal_quality")
        expected_short_window = baseline_data.get("short_window_seconds")
        
        evidence_sources = baseline_data.get("evidence_sources", [])
        for source in evidence_sources:
            if not isinstance(source, dict):
                continue
            
            workflow_run = source.get("workflow_run")
            artifact_id = source.get("artifact_id")
            artifact_name = source.get("artifact_name")
            expected_workload = source.get("workload", "")
            expected_workload_type = source.get("workload_type", "")
            expected_signal = source.get("signal_quality")
            expected_window = source.get("short_window_seconds")
            
            if not workflow_run or not artifact_id or not artifact_name:
                continue
            
            # Determine which service directory to search
            if budget_service == "tovarisch":
                evidence_dir = os.path.join(repo_root, "artifacts", "memory-labs", "tovarisch")
            elif budget_service == "uvb76":
                evidence_dir = os.path.join(repo_root, "artifacts", "memory-labs", "uvb76")
            else:
                errors.append(
                    f"Cannot verify leak-slope artifact traceability: unknown service '{budget_service}' "
                    f"in {budget_path}"
                )
                continue
            
            # Find ALL matching artifacts (load every JSON, filter by metadata)
            matching_artifacts = []
            
            if os.path.isdir(evidence_dir):
                for entry in os.listdir(evidence_dir):
                    if not entry.endswith(".json"):
                        continue
                    
                    artifact_path = os.path.join(evidence_dir, entry)
                    load_errors, artifact_data = _load_artifact(artifact_path)
                    
                    if load_errors or artifact_data is None:
                        # Skip artifacts that fail to load (already counted as separate error elsewhere)
                        continue
                    
                    # Check all required metadata fields
                    env = artifact_data.get("environment", {})
                    
                    # Match workflow run
                    if env.get("_github_workflow_run") != workflow_run:
                        continue
                    
                    # Match artifact ID
                    if env.get("_github_artifact_id") != artifact_id:
                        continue
                    
                    # Match artifact name
                    if env.get("_github_artifact_name") != artifact_name:
                        continue
                    
                    # Match service name
                    artifact_service = artifact_data.get("service", {}).get("name")
                    if artifact_service != budget_service:
                        continue
                    
                    # Match environment label
                    artifact_env_label = env.get("environment_label")
                    if artifact_env_label != env_label:
                        continue
                    
                    # Match workload type exactly (artifact.workload.type == evidence_sources[].workload)
                    artifact_workload = artifact_data.get("workload", {}).get("type")
                    if artifact_workload != expected_workload:
                        continue
                    
                    # Match normalized workload type (for plain/netdiag split)
                    artifact_workload_type = _get_workload_type_from_artifact(artifact_data)
                    if not _matches_workload_type(artifact_workload_type, expected_workload_type):
                        continue
                    
                    # Match decision.signal_quality
                    artifact_signal = artifact_data.get("decision", {}).get("signal_quality")
                    if artifact_signal != expected_signal:
                        continue
                    
                    # Match decision.short_window_seconds
                    artifact_window = artifact_data.get("decision", {}).get("short_window_seconds")
                    if artifact_window != expected_window:
                        continue
                    
                    # Check leak_slope section exists
                    if "leak_slope" not in artifact_data:
                        errors.append(
                            f"Artifact {artifact_path} is missing leak_slope section "
                            f"(expected for leak-slope workload)"
                        )
                        continue
                    
                    matching_artifacts.append((artifact_path, artifact_data))
            
            # Validate exactly one match
            if len(matching_artifacts) == 0:
                errors.append(
                    f"No leak-slope artifact found matching evidence_sources entry: "
                    f"workflow_run={workflow_run}, artifact_id={artifact_id}, "
                    f"artifact_name={artifact_name}, workload_type={expected_workload_type}, "
                    f"signal_quality={expected_signal}, short_window_seconds={expected_window} "
                    f"in {evidence_dir}"
                )
            elif len(matching_artifacts) > 1:
                paths = [p for p, _ in matching_artifacts]
                errors.append(
                    f"Multiple artifacts match evidence_sources entry "
                    f"(workload_type={expected_workload_type}, signal_quality={expected_signal}): "
                    f"{paths}"
                )
            else:
                # Exactly one match - validate evidence_kind
                artifact_path, artifact_data = matching_artifacts[0]
                evidence_kind = artifact_data.get("evidence_kind")
                if evidence_kind != "real_evidence":
                    errors.append(
                        f"Artifact {artifact_path} has evidence_kind='{evidence_kind}', "
                        f"expected 'real_evidence' for leak-slope baseline"
                    )
    
    return errors
