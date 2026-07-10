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

import hashlib
import os
from dataclasses import dataclass, field
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
    """Represents a tracked artifact surface with its scanning configuration."""
    id: str
    path: str
    format: ArtifactFormat
    producer: str
    committed_allowed: bool
    sensitivity: ArtifactSensitivity
    sanitizer: Sanitizer
    rule_set: RuleSet
    purpose: str
    malformed_fixtures: list[MalformedFixture] = field(default_factory=list)


@dataclass
class MalformedFixture:
    """Represents an intentionally malformed test fixture.

    Uses exact paths and SHA-256 fingerprints for validation.
    """
    path: str  # Exact repository-relative path
    fingerprint: str  # SHA-256 of current file bytes (hex-encoded)
    justification: str  # Why this fixture is intentionally malformed
    owner: str  # Team or person responsible


def compute_file_sha256(file_path: str) -> str:
    """Compute SHA-256 hash of a file's contents."""
    sha256 = hashlib.sha256()
    try:
        with open(file_path, 'rb') as f:
            for chunk in iter(lambda: f.read(65536), b''):
                sha256.update(chunk)
        return sha256.hexdigest()
    except (OSError, IOError):
        return ""


def validate_malformed_fixture(
    fixture: MalformedFixture,
    repo_root: str,
) -> tuple[bool, str]:
    """
    Validate a malformed fixture against actual file state.

    Returns (is_valid, error_message).

    Validation checks:
    - Exact normalized repository-relative path
    - SHA-256 of current file bytes matches declared fingerprint
    - Non-empty justification
    - Non-empty owner
    """
    # Check non-empty justification
    if not fixture.justification or not fixture.justification.strip():
        return False, f"Malformed fixture {fixture.path}: empty justification"

    # Check non-empty owner
    if not fixture.owner or not fixture.owner.strip():
        return False, f"Malformed fixture {fixture.path}: empty owner"

    # Check non-empty fingerprint
    if not fixture.fingerprint or not fixture.fingerprint.strip():
        return False, f"Malformed fixture {fixture.path}: empty fingerprint"

    # Check exact path is not a directory or glob
    if '*' in fixture.path or '?' in fixture.path:
        return False, f"Malformed fixture {fixture.path}: path cannot be glob pattern"

    # Compute actual file fingerprint
    abs_path = os.path.join(repo_root, fixture.path)

    if not os.path.exists(abs_path):
        return False, f"Malformed fixture {fixture.path}: file does not exist"

    if os.path.isdir(abs_path):
        return False, f"Malformed fixture {fixture.path}: path is a directory"

    actual_fingerprint = compute_file_sha256(abs_path)

    if not actual_fingerprint:
        return False, f"Malformed fixture {fixture.path}: cannot compute file fingerprint"

    # Compare fingerprints (case-insensitive hex comparison)
    if actual_fingerprint.lower() != fixture.fingerprint.lower():
        return False, f"Malformed fixture {fixture.path}: stale fingerprint (file changed)"

    return True, ""


def is_malformed_fixture_exempt(
    path: str,
    repo_root: str,
) -> tuple[bool, str]:
    """
    Check if a path is exempt as a known malformed fixture.

    Returns (is_exempt, error_message).

    Exemption is only granted if:
    - Path exactly matches a declared malformed fixture
    - File SHA-256 matches declared fingerprint
    - Justification and owner are non-empty
    """
    for surface in ARTIFACT_INVENTORY:
        for fixture in surface.malformed_fixtures:
            # Normalize path comparison
            normalized_fixture = os.path.normpath(fixture.path)
            normalized_path = os.path.normpath(path)

            if normalized_fixture == normalized_path:
                # Found matching fixture - validate fingerprint
                is_valid, error = validate_malformed_fixture(fixture, repo_root)
                if is_valid:
                    return True, ""
                else:
                    return False, error

    return False, ""


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
        id="hygiene-registry",
        path="scripts/uvb76_artifact_secret_hygiene/registry.json",
        format=ArtifactFormat.JSON,
        producer="hygiene",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.LOW,
        sanitizer=Sanitizer.NONE,
        rule_set=RuleSet.UNIVERSAL,
        purpose="Hygiene infrastructure - registry contains pattern literals as data, not secrets",
    ),
    ArtifactSurface(
        id="hygiene-rules",
        path="scripts/uvb76_artifact_secret_hygiene/rules.py",
        format=ArtifactFormat.TEXT,
        producer="hygiene",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.LOW,
        sanitizer=Sanitizer.NONE,
        rule_set=RuleSet.UNIVERSAL,
        purpose="Hygiene infrastructure - patterns derived from registry at runtime",
    ),
    ArtifactSurface(
        id="hygiene-registry-loader",
        path="scripts/uvb76_artifact_secret_hygiene/registry_loader.py",
        format=ArtifactFormat.TEXT,
        producer="hygiene",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.LOW,
        sanitizer=Sanitizer.NONE,
        rule_set=RuleSet.UNIVERSAL,
        purpose="Hygiene infrastructure - registry loading and validation",
    ),
    ArtifactSurface(
        id="hygiene-structured-scanner",
        path="scripts/uvb76_artifact_secret_hygiene/structured_scanner.py",
        format=ArtifactFormat.TEXT,
        producer="hygiene",
        committed_allowed=True,
        sensitivity=ArtifactSensitivity.LOW,
        sanitizer=Sanitizer.NONE,
        rule_set=RuleSet.UNIVERSAL,
        purpose="Hygiene infrastructure - structured JSON scanning",
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
        # Exact malformed fixture - only this specific file is exempt
        malformed_fixtures=[
            MalformedFixture(
                path="uvb76/cmd/uvb76-tcp-diag-telemetry-lab/internal/verifier/testdata/fail_malformed_json/captured-diagnostic-packet.json",
                fingerprint="638d7f2fba1b155f9715957a70fba13f4025da6bf886f72e18d37d209fe8e2e2",
                justification="Intentional malformed packet for testing parser resilience",
                owner="uvb76-team",
            ),
        ],
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

        # Validate malformed fixtures
        for fixture in surface.malformed_fixtures:
            if not fixture.path:
                errors.append(f"Artifact surface {surface.id} has malformed fixture with empty path")
            if not fixture.fingerprint:
                errors.append(f"Artifact surface {surface.id} fixture {fixture.path} missing fingerprint")
            if not fixture.justification:
                errors.append(f"Artifact surface {surface.id} fixture {fixture.path} missing justification")
            if not fixture.owner:
                errors.append(f"Artifact surface {surface.id} fixture {fixture.path} missing owner")

    return errors
