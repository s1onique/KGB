"""
Artifact Surface Inventory for UVB-76 Artifact Secret Hygiene.

This module defines the inventory of all UVB-76 artifact surfaces that must
be scanned for prohibited secret classes.

Each entry contains:
- stable artifact identifier
- path or path glob
- artifact format
- producer or owning component
- whether committed artifacts are permitted
- expected sensitivity
- required sanitizer
- scanner rule set
- retention or generation purpose
"""

from dataclasses import dataclass
from enum import Enum
from typing import Optional


class ArtifactFormat(Enum):
    JSON = "json"
    TEXT = "text"
    LOG = "log"
    BINARY = "binary"
    CONFIG = "config"
    CERT = "certificate"
    FUZZ = "fuzz"
    MIXED = "mixed"


class ArtifactSensitivity(Enum):
    HIGH = "high"
    MEDIUM = "medium"
    LOW = "low"
    NONE = "none"


class RuleSet(Enum):
    UNIVERSAL = "universal"
    ARTIFACT_CONTEXT = "artifact_context"
    NONE = "none"


class Sanitizer(Enum):
    REDACT_JSON = "redact_json"
    REDACT_HEADERS = "redact_headers"
    REDACT_URL = "redact_url"
    REDACT_CONFIG = "redact_config"
    NONE = "none"


@dataclass
class ArtifactSurface:
    id: str
    path: str
    format: ArtifactFormat
    producer: str
    committed_allowed: bool
    sensitivity: ArtifactSensitivity
    sanitizer: Sanitizer
    rule_set: RuleSet
    purpose: str
    binary_policy: Optional[str] = None


# Hygiene Infrastructure - patterns are built at runtime from fragments, 
# so these files can be scanned. They are LOW sensitivity so they don't
# require sanitizer but are covered by universal rules.
ARTIFACT_INVENTORY: list[ArtifactSurface] = [
    ArtifactSurface(
        id="hygiene-redact-go",
        path="uvb76/internal/redact/redact.go",
        format=ArtifactFormat.TEXT,
        producer="hygiene",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.LOW,
        sanitizer=Sanitizer.NONE,
        rule_set=RuleSet.UNIVERSAL,
        purpose="Hygiene infrastructure - patterns built from fragments at runtime",
    ),
    ArtifactSurface(
        id="hygiene-verifier",
        path="scripts/verify_uvb76_artifact_secret_hygiene.py",
        format=ArtifactFormat.TEXT,
        producer="hygiene",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.LOW,
        sanitizer=Sanitizer.NONE,
        rule_set=RuleSet.UNIVERSAL,
        purpose="Hygiene infrastructure - patterns built from fragments at runtime",
    ),
    ArtifactSurface(
        id="config-example",
        path="uvb76/uvb76.example.json",
        format=ArtifactFormat.JSON,
        producer="config",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.HIGH,
        sanitizer=Sanitizer.REDACT_CONFIG,
        rule_set=RuleSet.ARTIFACT_CONTEXT,
        purpose="Configuration example showing config schema",
    ),
    ArtifactSurface(
        id="capture-netns-lab-artifacts",
        path="uvb76/cmd/uvb76-capture-netns-lab/**/*.json",
        format=ArtifactFormat.JSON,
        producer="uvb76-capture-netns-lab",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.HIGH,
        sanitizer=Sanitizer.REDACT_JSON,
        rule_set=RuleSet.ARTIFACT_CONTEXT,
        purpose="Diagnostic capture netns lab evidence",
    ),
    ArtifactSurface(
        id="latency-crash-lab-artifacts",
        path="uvb76/cmd/uvb76-latency-crash-lab/**/*.json",
        format=ArtifactFormat.JSON,
        producer="uvb76-latency-crash-lab",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.HIGH,
        sanitizer=Sanitizer.REDACT_JSON,
        rule_set=RuleSet.ARTIFACT_CONTEXT,
        purpose="Latency crash lab evidence",
    ),
    ArtifactSurface(
        id="targets-crash-lab-artifacts",
        path="uvb76/cmd/uvb76-targets-crash-lab/**/*.json",
        format=ArtifactFormat.JSON,
        producer="uvb76-targets-crash-lab",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.HIGH,
        sanitizer=Sanitizer.REDACT_JSON,
        rule_set=RuleSet.ARTIFACT_CONTEXT,
        purpose="Targets crash lab evidence",
    ),
    ArtifactSurface(
        id="memory-lab-artifacts",
        path="uvb76/cmd/uvb76-memory-lab/**/*.json",
        format=ArtifactFormat.JSON,
        producer="uvb76-memory-lab",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.HIGH,
        sanitizer=Sanitizer.REDACT_JSON,
        rule_set=RuleSet.ARTIFACT_CONTEXT,
        purpose="Memory lab evidence",
    ),
    ArtifactSurface(
        id="memleak-pprof-lab-artifacts",
        path="uvb76/cmd/uvb76-memleak-pprof-lab/**/*.json",
        format=ArtifactFormat.JSON,
        producer="uvb76-memleak-pprof-lab",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.HIGH,
        sanitizer=Sanitizer.REDACT_JSON,
        rule_set=RuleSet.ARTIFACT_CONTEXT,
        purpose="Memory leak pprof lab evidence",
    ),
    ArtifactSurface(
        id="icmp-ping-soak-artifacts",
        path="uvb76/cmd/uvb76-icmp-os-ping-soak/**/*.json",
        format=ArtifactFormat.JSON,
        producer="uvb76-icmp-os-ping-soak",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.HIGH,
        sanitizer=Sanitizer.REDACT_JSON,
        rule_set=RuleSet.ARTIFACT_CONTEXT,
        purpose="ICMP ping soak lab evidence",
    ),
    ArtifactSurface(
        id="tcp-diag-telemetry-lab-artifacts",
        path="uvb76/cmd/uvb76-tcp-diag-telemetry-lab/**/*.json",
        format=ArtifactFormat.JSON,
        producer="uvb76-tcp-diag-telemetry-lab",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.HIGH,
        sanitizer=Sanitizer.REDACT_JSON,
        rule_set=RuleSet.ARTIFACT_CONTEXT,
        purpose="TCP diagnostic telemetry lab evidence",
    ),
    ArtifactSurface(
        id="fuzz-corpus",
        path="uvb76/state/testdata/fuzz/**/*",
        format=ArtifactFormat.FUZZ,
        producer="fuzzing",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.HIGH,
        sanitizer=Sanitizer.REDACT_JSON,
        rule_set=RuleSet.UNIVERSAL,
        purpose="Fuzz corpus for capture evidence projection",
    ),
    ArtifactSurface(
        id="packaging-entware-config",
        path="packaging/entware/uvb76.json.example",
        format=ArtifactFormat.CONFIG,
        producer="packaging",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.HIGH,
        sanitizer=Sanitizer.REDACT_CONFIG,
        rule_set=RuleSet.ARTIFACT_CONTEXT,
        purpose="Entware/AsusWRT-Merlin package configuration example",
    ),
    ArtifactSurface(
        id="packaging-debian-config",
        path="packaging/debian/**/*",
        format=ArtifactFormat.MIXED,
        producer="packaging",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.MEDIUM,
        sanitizer=Sanitizer.REDACT_CONFIG,
        rule_set=RuleSet.ARTIFACT_CONTEXT,
        purpose="Debian packaging files",
    ),
    ArtifactSurface(
        id="diag-capture-packets",
        path="artifacts/**/*-packet.json",
        format=ArtifactFormat.JSON,
        producer="diag/capture",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.HIGH,
        sanitizer=Sanitizer.REDACT_JSON,
        rule_set=RuleSet.UNIVERSAL,
        purpose="Captured diagnostic packets",
    ),
    ArtifactSurface(
        id="memory-lab-evidence",
        path="artifacts/memory-labs/**/*.json",
        format=ArtifactFormat.JSON,
        producer="memory-lab",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.HIGH,
        sanitizer=Sanitizer.REDACT_JSON,
        rule_set=RuleSet.ARTIFACT_CONTEXT,
        purpose="Memory lab evidence artifacts",
    ),
    ArtifactSurface(
        id="wg-netlink-lab-evidence",
        path="artifacts/wg-netlink-lab/**/*",
        format=ArtifactFormat.MIXED,
        producer="wg-netlink-lab",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.MEDIUM,
        sanitizer=Sanitizer.NONE,
        rule_set=RuleSet.UNIVERSAL,
        purpose="WireGuard netlink lab evidence",
    ),
    ArtifactSurface(
        id="memory-attribution-matrix",
        path="scripts/memory_attribution_matrix/**/*",
        format=ArtifactFormat.JSON,
        producer="memory-attribution-matrix",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.HIGH,
        sanitizer=Sanitizer.REDACT_JSON,
        rule_set=RuleSet.ARTIFACT_CONTEXT,
        purpose="Memory attribution matrix artifacts",
    ),
]


def get_artifact_by_id(artifact_id: str) -> Optional[ArtifactSurface]:
    for surface in ARTIFACT_INVENTORY:
        if surface.id == artifact_id:
            return surface
    return None


def get_artifacts_by_producer(producer: str) -> list[ArtifactSurface]:
    return [s for s in ARTIFACT_INVENTORY if s.producer == producer]


def get_artifacts_by_sensitivity(sensitivity: ArtifactSensitivity) -> list[ArtifactSurface]:
    return [s for s in ARTIFACT_INVENTORY if s.sensitivity == sensitivity]


def validate_inventory() -> list[str]:
    errors = []
    seen_ids = set()

    for surface in ARTIFACT_INVENTORY:
        if surface.id in seen_ids:
            errors.append(f"Duplicate artifact ID: {surface.id}")
        seen_ids.add(surface.id)

        if not surface.id:
            errors.append("Artifact surface has empty id")
        if not surface.path:
            errors.append(f"Artifact surface {surface.id} has empty path")
        if not surface.producer:
            errors.append(f"Artifact surface {surface.id} has empty producer")
        if not surface.purpose:
            errors.append(f"Artifact surface {surface.id} has empty purpose")

        if surface.sensitivity == ArtifactSensitivity.HIGH:
            if surface.sanitizer == Sanitizer.NONE:
                errors.append(
                    f"Artifact surface {surface.id} has HIGH sensitivity but no sanitizer"
                )

        if surface.rule_set == RuleSet.NONE and surface.sensitivity != ArtifactSensitivity.LOW:
            errors.append(f"Artifact surface {surface.id} requires a rule set")

    return errors
