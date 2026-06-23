#!/usr/bin/env python3
"""
Helper functions for long-window leak-slope CI baseline validation.

Contains:
- manifest.yaml parsing
- artifacts.tsv parsing
- commit SHA comparison utilities
"""

import os
from typing import List, Dict, Optional


def _parse_manifest(manifest_path: str) -> tuple[List[str], Optional[Dict]]:
    """Parse manifest.yaml. Returns (errors, data)."""
    errors = []
    data = None
    
    if not os.path.exists(manifest_path):
        errors.append(f"Manifest not found: {manifest_path}")
        return errors, None
    
    try:
        import yaml
        with open(manifest_path, "r", encoding="utf-8") as f:
            data = yaml.safe_load(f)
    except ImportError:
        data = _parse_manifest_simple(manifest_path)
    except Exception as e:
        errors.append(f"Failed to parse manifest {manifest_path}: {e}")
        return errors, None
    
    if not isinstance(data, dict):
        errors.append(f"Manifest must be a YAML dict: {manifest_path}")
        return errors, None
    
    return errors, data


def _parse_manifest_simple(manifest_path: str) -> Optional[Dict]:
    """Fallback simple YAML parser for manifest.yaml."""
    data = {}
    current_section = None
    current_artifact = None
    
    with open(manifest_path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.rstrip()
            if not line or line.startswith("#"):
                continue
            
            if line.startswith("workflow_run_id:"):
                data["workflow_run_id"] = int(line.split(":", 1)[1].strip())
            elif line.startswith("source_commit_sha:"):
                data["source_commit_sha"] = line.split(":", 1)[1].strip().strip('"')
            elif line.startswith("artifacts:"):
                data["artifacts"] = {}
            elif line.startswith("workloads:"):
                data["workloads"] = {}
            elif line.strip().startswith("uvb76:") and "artifacts" in data:
                current_section = "artifacts"
                current_artifact = "uvb76"
                data["artifacts"]["uvb76"] = {}
            elif line.strip().startswith("tovarisch:") and "artifacts" in data:
                current_section = "artifacts"
                current_artifact = "tovarisch"
                data["artifacts"]["tovarisch"] = {}
            elif current_section == "artifacts" and current_artifact:
                if line.startswith("artifact_id:"):
                    data["artifacts"][current_artifact]["artifact_id"] = int(line.split(":", 1)[1].strip())
                elif line.startswith("artifact_name:"):
                    data["artifacts"][current_artifact]["artifact_name"] = line.split(":", 1)[1].strip()
                elif line.startswith("expired:"):
                    data["artifacts"][current_artifact]["expired"] = line.split(":", 1)[1].strip().lower() == "true"
            else:
                current_section = None
                current_artifact = None
    
    return data if data else None


def _parse_artifacts_tsv(tsv_path: str) -> tuple[List[str], Dict[int, Dict]]:
    """Parse artifacts.tsv. Returns (errors, {artifact_id: {name, expired, ...}})."""
    errors = []
    artifacts = {}
    
    if not os.path.exists(tsv_path):
        errors.append(f"Artifacts TSV not found: {tsv_path}")
        return errors, artifacts
    
    try:
        with open(tsv_path, "r", encoding="utf-8") as f:
            for line_num, line in enumerate(f, 1):
                line = line.rstrip()
                if not line or line.startswith("#"):
                    continue
                
                parts = line.split("\t")
                if len(parts) < 4:
                    continue
                
                try:
                    artifact_id = int(parts[0])
                except ValueError:
                    continue
                
                try:
                    artifacts[artifact_id] = {
                        "name": parts[1],
                        "size_in_bytes": int(parts[2]) if parts[2] else 0,
                        "expired": parts[3].lower() == "true" if len(parts) > 3 else False,
                    }
                except (ValueError, IndexError) as e:
                    errors.append(f"Invalid TSV line {line_num}: {line} ({e})")
    except Exception as e:
        errors.append(f"Failed to parse artifacts.tsv {tsv_path}: {e}")
    
    return errors, artifacts


def _matches_commit(artifact_commit: str, source_commit: str) -> bool:
    """
    Check if artifact commit matches source commit.
    Allows short vs full SHA comparison:
    - artifact stores short SHA (7 chars)
    - source stores full SHA (40 chars)
    Returns True if artifact_commit == source_commit[:7] or exact match.
    """
    if artifact_commit == source_commit:
        return True
    if len(source_commit) >= 7 and artifact_commit == source_commit[:7]:
        return True
    return False
