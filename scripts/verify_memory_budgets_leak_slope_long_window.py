#!/usr/bin/env python3
"""
Long-window leak-slope CI Baseline validation module for memory budgets.

Validates long-window evidence with:
- signal_quality: long_window
- duration_seconds >= 900
- operations >= 9000
- request_errors == 0
- Full artifact traceability from manifest/artifacts.tsv
- Exact workload equality validation
"""

import os
import json
from typing import List, Dict

# Import shared constants and utilities from the main leak-slope CI module
import sys
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, SCRIPT_DIR)

from verify_memory_budgets_leak_slope_ci import (
    VALID_ENVIRONMENT_LABELS,
    WORKLOAD_TO_ARTIFACT_SUFFIX,
    _load_artifact,
    _get_workload_type_from_artifact,
    _matches_workload_type,
)

# Import helpers from separate module
from verify_memory_budgets_leak_slope_helpers import (
    _parse_manifest,
    _parse_artifacts_tsv,
    _matches_commit,
)

# Required fields for long-window leak-slope baseline evidence entries
REQUIRED_LEAK_SLOPE_LONG_WINDOW_EVIDENCE_FIELDS = [
    "workflow_run", "artifact_id", "artifact_name", "service_version",
    "service_commit", "workload", "duration_seconds", "operations",
    "request_errors", "environment_label", "workload_type",
    "long_window_seconds", "signal_quality",
]

# Long-window minimum thresholds
LONG_WINDOW_MIN_DURATION_SECONDS = 900
LONG_WINDOW_MIN_OPERATIONS = 9000
DURATION_TOLERANCE_SECONDS = 0.001


def validate_ci_leak_slope_long_window(data: Dict, path: str) -> List[str]:
    """Validate ci_leak_slope_long_window section. Returns list of errors."""
    errors = []
    
    if "ci_leak_slope_long_window" not in data:
        return errors
    
    ci_long_window = data["ci_leak_slope_long_window"]
    if not isinstance(ci_long_window, dict):
        errors.append(f"ci_leak_slope_long_window must be a dict in {path}")
        return errors
    
    for env_label, baseline_data in ci_long_window.items():
        if not isinstance(baseline_data, dict):
            errors.append(f"ci_leak_slope_long_window.{env_label} must be a dict")
            continue
        
        # Validate evidence_status is present
        if "evidence_status" not in baseline_data:
            errors.append(f"ci_leak_slope_long_window.{env_label}.evidence_status is required")
        
        # Validate signal_quality is long_window
        if "signal_quality" in baseline_data:
            sq = baseline_data["signal_quality"]
            if sq != "long_window":
                errors.append(f"ci_leak_slope_long_window.{env_label}.signal_quality must be 'long_window', got '{sq}'")
        
        # Validate long_window_seconds >= 900
        if "long_window_seconds" in baseline_data:
            lw = baseline_data["long_window_seconds"]
            if not isinstance(lw, (int, float)):
                errors.append(f"ci_leak_slope_long_window.{env_label}.long_window_seconds must be numeric")
            elif lw < LONG_WINDOW_MIN_DURATION_SECONDS:
                errors.append(f"ci_leak_slope_long_window.{env_label}.long_window_seconds ({lw}) must be >= {LONG_WINDOW_MIN_DURATION_SECONDS}")
        
        # Validate evidence_sources
        if "evidence_sources" not in baseline_data:
            errors.append(f"ci_leak_slope_long_window.{env_label}.evidence_sources is required")
        else:
            sources = baseline_data["evidence_sources"]
            if not isinstance(sources, list):
                errors.append(f"ci_leak_slope_long_window.{env_label}.evidence_sources must be a list")
            elif len(sources) == 0:
                errors.append(f"ci_leak_slope_long_window.{env_label}.evidence_sources cannot be empty")
            else:
                for i, source in enumerate(sources):
                    if not isinstance(source, dict):
                        errors.append(f"ci_leak_slope_long_window.{env_label}.evidence_sources[{i}] must be a dict")
                        continue
                    
                    for field in REQUIRED_LEAK_SLOPE_LONG_WINDOW_EVIDENCE_FIELDS:
                        if field not in source:
                            errors.append(f"ci_leak_slope_long_window.{env_label}.evidence_sources[{i}].{field} is required")
                    
                    _validate_source_field(errors, env_label, i, source, "long_window_seconds", int, float)
                    _validate_source_field(errors, env_label, i, source, "operations", int)
                    ops = source.get("operations", 0)
                    if isinstance(ops, int) and ops < LONG_WINDOW_MIN_OPERATIONS:
                        errors.append(f"ci_leak_slope_long_window.{env_label}.evidence_sources[{i}].operations ({ops}) must be >= {LONG_WINDOW_MIN_OPERATIONS}")
                    _validate_source_field(errors, env_label, i, source, "request_errors", int)
                    _validate_source_field(errors, env_label, i, source, "duration_seconds", int, float)
                    
                    if source.get("request_errors", 0) > 0:
                        errors.append(f"ci_leak_slope_long_window.{env_label}.evidence_sources[{i}].request_errors must be 0")
                    
                    if source.get("signal_quality") != "long_window":
                        errors.append(f"ci_leak_slope_long_window.{env_label}.evidence_sources[{i}].signal_quality must be 'long_window'")
                    
                    env = source.get("environment_label")
                    if env and env != env_label:
                        errors.append(f"ci_leak_slope_long_window.{env_label}.evidence_sources[{i}].environment_label should be '{env_label}'")
                    if env and env not in VALID_ENVIRONMENT_LABELS:
                        errors.append(f"ci_leak_slope_long_window.{env_label}.evidence_sources[{i}].environment_label '{env}' is not known")
                    
                    wtype = source.get("workload_type")
                    if wtype and wtype not in WORKLOAD_TO_ARTIFACT_SUFFIX:
                        errors.append(f"ci_leak_slope_long_window.{env_label}.evidence_sources[{i}].workload_type '{wtype}' is not known")
    
    return errors


def _validate_source_field(errors: List[str], env_label: str, idx: int, source: Dict, field: str, *expected_types):
    """Helper to validate a field has expected type."""
    if field in source:
        val = source[field]
        if not isinstance(val, expected_types):
            errors.append(f"ci_leak_slope_long_window.{env_label}.evidence_sources[{idx}].{field} must be {expected_types[0].__name__}, got {type(val).__name__}")


def check_long_window_evidence_traceability(budget_data: Dict, budget_path: str, repo_root: str) -> List[str]:
    """
    Check that CI long-window leak-slope baseline evidence artifacts exist and are valid.
    
    Validates:
    1. manifest.yaml exists and has workflow_run_id, source_commit_sha
    2. artifacts.tsv exists and maps artifact_id -> artifact_name
    3. source.artifact_id exists in artifacts.tsv
    4. source.artifact_name matches that artifact ID
    5. Manifest artifact entry maps to correct service
    6. Manifest artifact has expired: false
    7. Exact workload equality (operations, duration, request_errors)
    8. Commit SHA matches (short vs full comparison)
    """
    errors = []
    
    if "ci_leak_slope_long_window" not in budget_data:
        return errors
    
    budget_service = budget_data.get("service", "")
    
    for env_label, baseline_data in budget_data["ci_leak_slope_long_window"].items():
        if not isinstance(baseline_data, dict):
            continue
        
        for source in baseline_data.get("evidence_sources", []):
            if not isinstance(source, dict):
                continue
            
            workflow_run = source.get("workflow_run")
            artifact_id = source.get("artifact_id")
            artifact_name = source.get("artifact_name")
            expected_workload = source.get("workload", "")
            expected_workload_type = source.get("workload_type", "")
            expected_ops = source.get("operations", 0)
            expected_dur = source.get("duration_seconds", 0)
            expected_errs = source.get("request_errors", 0)
            expected_commit = source.get("service_commit", "")
            
            if not workflow_run:
                continue
            
            evidence_dir = os.path.join(repo_root, "docs", "evidence", "memory-lab", f"run-{workflow_run}")
            
            # Parse manifest and artifacts.tsv
            manifest_path = os.path.join(evidence_dir, "manifest.yaml")
            manifest_errors, manifest_data = _parse_manifest(manifest_path)
            errors.extend(manifest_errors)
            
            tsv_path = os.path.join(evidence_dir, "artifacts.tsv")
            tsv_errors, tsv_artifacts = _parse_artifacts_tsv(tsv_path)
            errors.extend(tsv_errors)
            
            # Validate manifest against source
            if manifest_data:
                mrun_id = manifest_data.get("workflow_run_id")
                if mrun_id and mrun_id != workflow_run:
                    errors.append(f"Manifest workflow_run_id ({mrun_id}) != source.workflow_run ({workflow_run})")
                
                msha = manifest_data.get("source_commit_sha", "")
                if msha and expected_commit and not _matches_commit(msha, expected_commit):
                    errors.append(f"Manifest source_commit_sha ({msha[:7]}...) != source.service_commit ({expected_commit[:7]}...)")
                
                m_artifacts = manifest_data.get("artifacts", {})
                if budget_service in m_artifacts:
                    ma = m_artifacts[budget_service]
                    if artifact_id and ma.get("artifact_id") != artifact_id:
                        errors.append(f"source.artifact_id ({artifact_id}) != manifest.artifacts.{budget_service}.artifact_id ({ma.get('artifact_id')})")
                    if artifact_name and ma.get("artifact_name") != artifact_name:
                        errors.append(f"source.artifact_name ({artifact_name}) != manifest.artifacts.{budget_service}.artifact_name ({ma.get('artifact_name')})")
                    if ma.get("expired"):
                        errors.append(f"Manifest artifact for {budget_service} has expired: true")
                else:
                    errors.append(f"Manifest missing artifact entry for '{budget_service}'")
            
            # Validate artifacts.tsv
            if tsv_artifacts and artifact_id:
                if artifact_id not in tsv_artifacts:
                    errors.append(f"source.artifact_id ({artifact_id}) not found in artifacts.tsv")
                else:
                    ta = tsv_artifacts[artifact_id]
                    if artifact_name and ta.get("name") != artifact_name:
                        errors.append(f"source.artifact_name ({artifact_name}) != artifacts.tsv name ({ta.get('name')}) for artifact_id {artifact_id}")
                    if ta.get("expired"):
                        errors.append(f"Artifact {artifact_id} has expired in artifacts.tsv")
            
            if manifest_errors or tsv_errors:
                continue
            
            if not os.path.isdir(evidence_dir):
                continue
            
            # Find and validate artifact JSON
            matching = []
            for fname in os.listdir(evidence_dir):
                if not fname.endswith(".json"):
                    continue
                fpath = os.path.join(evidence_dir, fname)
                load_errs, adata = _load_artifact(fpath)
                if load_errs or adata is None:
                    errors.extend(load_errs)
                    continue
                awt = _get_workload_type_from_artifact(adata)
                if not _matches_workload_type(awt, expected_workload_type):
                    continue
                if adata.get("service", {}).get("name") != budget_service:
                    continue
                matching.append((fpath, adata))
            
            if not matching:
                errors.append(f"No long-window artifact found for workflow_run={workflow_run}, artifact_id={artifact_id}")
                continue
            
            for apath, adata in matching:
                ls = adata.get("leak_slope", {})
                wl = adata.get("workload", {})
                svc = adata.get("service", {})
                
                # Workload type match
                if wl.get("type") != expected_workload:
                    errors.append(f"Artifact {apath} workload.type ({wl.get('type')}) != source.workload ({expected_workload})")
                
                # Operations match
                aops = wl.get("operations", 0)
                if not isinstance(aops, int):
                    errors.append(f"Artifact {apath} workload.operations must be int")
                elif aops != expected_ops:
                    errors.append(f"Artifact {apath} workload.operations ({aops}) != source.operations ({expected_ops})")
                
                # Request count match
                acount = ls.get("request_count", 0)
                if not isinstance(acount, int):
                    errors.append(f"Artifact {apath} leak_slope.request_count must be int")
                elif acount != expected_ops:
                    errors.append(f"Artifact {apath} leak_slope.request_count ({acount}) != source.operations ({expected_ops})")
                
                # Request errors match
                aerrs = ls.get("request_errors", -1)
                if not isinstance(aerrs, int):
                    errors.append(f"Artifact {apath} leak_slope.request_errors must be int")
                elif aerrs != expected_errs:
                    errors.append(f"Artifact {apath} leak_slope.request_errors ({aerrs}) != source.request_errors ({expected_errs})")
                
                # Duration match
                adur = ls.get("duration_seconds", 0)
                if isinstance(adur, (int, float)) and isinstance(expected_dur, (int, float)):
                    if abs(adur - expected_dur) > DURATION_TOLERANCE_SECONDS:
                        errors.append(f"Artifact {apath} duration_seconds ({adur}) differs from source ({expected_dur})")
                
                # Duration_ms match
                adurms = wl.get("duration_ms", 0)
                if isinstance(adurms, (int, float)) and isinstance(expected_dur, (int, float)):
                    secs = adurms / 1000.0
                    if abs(secs - expected_dur) > DURATION_TOLERANCE_SECONDS:
                        errors.append(f"Artifact {apath} duration_ms/1000 ({secs}) differs from source ({expected_dur})")
                
                # Commit SHA match
                acommit = svc.get("commit", "")
                if expected_commit and acommit and not _matches_commit(acommit, expected_commit):
                    errors.append(f"Artifact {apath} service.commit ({acommit}) != source.service_commit ({expected_commit[:7]}...)")
                
                # Threshold checks
                if isinstance(aops, int) and aops < LONG_WINDOW_MIN_OPERATIONS:
                    errors.append(f"Artifact {apath} operations ({aops}) < {LONG_WINDOW_MIN_OPERATIONS}")
                if isinstance(adur, (int, float)) and adur < LONG_WINDOW_MIN_DURATION_SECONDS:
                    errors.append(f"Artifact {apath} duration_seconds ({adur}) < {LONG_WINDOW_MIN_DURATION_SECONDS}")
                if isinstance(aerrs, int) and aerrs > 0:
                    errors.append(f"Artifact {apath} has request_errors={aerrs} > 0")
    
    return errors
