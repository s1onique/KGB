"""Canonical artifact surface inventory and compatibility scanner views.

``surfaces.json`` is the sole editable catalog.  This module loads every
canonical field into immutable records; no hand-authored Python mirror exists.
"""

import hashlib
import json
import os
from dataclasses import dataclass, field
from enum import Enum
from typing import Optional


CATALOG_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), "surfaces.json")
CANONICAL_SURFACE_FIELDS = (
    "id", "path", "producer", "committed_allowed", "sensitivity", "sanitizer",
    "status", "persistence_policy", "binary_policy", "output_format", "owner",
    "justification", "enforcement_state", "ownership_scope", "writer_files",
    "writer_symbols", "test_files", "malformed_fixtures",
)


class ArtifactFormat(Enum):
    JSON = "json"
    TEXT = "text"
    LOG = "log"
    BINARY = "binary"
    BINARY_PROFILE = "binary_profile"
    CONFIG = "config"
    CERT = "certificate"
    CSV = "csv"
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
    REDACT_TEXT = "redact_text"
    REDACT_HEADERS = "redact_headers"
    REDACT_URL = "redact_url"
    REDACT_CONFIG = "redact_config"
    REPOSITORY_DETECTION_ONLY = "repository_detection_only"
    NONE = "none"


@dataclass(frozen=True)
class MalformedFixture:
    path: str
    fingerprint: str
    justification: str
    owner: str

    def canonical_dict(self) -> dict[str, object]:
        return {
            "path": self.path,
            "fingerprint": self.fingerprint,
            "justification": self.justification,
            "owner": self.owner,
        }


@dataclass(frozen=True)
class SurfaceRecord:
    id: str
    path: str
    producer: str
    committed_allowed: bool
    sensitivity: str
    sanitizer: str
    status: str
    persistence_policy: str
    binary_policy: str
    output_format: str
    owner: str
    justification: str
    enforcement_state: str
    ownership_scope: str = ""
    writer_files: tuple[str, ...] = field(default_factory=tuple)
    writer_symbols: tuple[str, ...] = field(default_factory=tuple)
    test_files: tuple[str, ...] = field(default_factory=tuple)
    malformed_fixtures: tuple[MalformedFixture, ...] = field(default_factory=tuple)

    @property
    def format(self) -> str:
        return self.output_format

    @property
    def purpose(self) -> str:
        return self.justification

    @property
    def rule_set(self) -> str:
        if self.enforcement_state == "not_applicable":
            return RuleSet.UNIVERSAL.value
        return RuleSet.ARTIFACT_CONTEXT.value

    def canonical_dict(self) -> dict[str, object]:
        return {
            "id": self.id,
            "path": self.path,
            "producer": self.producer,
            "committed_allowed": self.committed_allowed,
            "sensitivity": self.sensitivity,
            "sanitizer": self.sanitizer,
            "status": self.status,
            "persistence_policy": self.persistence_policy,
            "binary_policy": self.binary_policy,
            "output_format": self.output_format,
            "owner": self.owner,
            "justification": self.justification,
            "enforcement_state": self.enforcement_state,
            "ownership_scope": self.ownership_scope,
            "writer_files": list(self.writer_files),
            "writer_symbols": list(self.writer_symbols),
            "test_files": list(self.test_files),
            "malformed_fixtures": [item.canonical_dict() for item in self.malformed_fixtures],
        }


# Compatibility name retained without creating a second projection type.
ArtifactSurface = SurfaceRecord


def _load_fixture(raw: object, surface_id: str) -> MalformedFixture:
    if not isinstance(raw, dict):
        raise ValueError(f"surface {surface_id}: malformed fixture must be an object")
    return MalformedFixture(
        path=str(raw["path"]), fingerprint=str(raw["fingerprint"]),
        justification=str(raw["justification"]), owner=str(raw["owner"]),
    )


def _load_surface(raw: object) -> SurfaceRecord:
    if not isinstance(raw, dict):
        raise ValueError("surface entry must be an object")
    allowed = raw["committed_allowed"]
    if not isinstance(allowed, bool):
        raise ValueError(f"surface {raw.get('id', '<unknown>')}: committed_allowed must be bool")
    surface_id = str(raw["id"])
    return SurfaceRecord(
        id=surface_id, path=str(raw["path"]), producer=str(raw["producer"]),
        committed_allowed=allowed, sensitivity=str(raw["sensitivity"]),
        sanitizer=str(raw["sanitizer"]), status=str(raw["status"]),
        persistence_policy=str(raw["persistence_policy"]),
        binary_policy=str(raw["binary_policy"]), output_format=str(raw["output_format"]),
        owner=str(raw["owner"]), justification=str(raw["justification"]),
        enforcement_state=str(raw["enforcement_state"]),
        ownership_scope=str(raw.get("ownership_scope", "")),
        writer_files=tuple(str(value) for value in raw["writer_files"]),
        writer_symbols=tuple(str(value) for value in raw["writer_symbols"]),
        test_files=tuple(str(value) for value in raw["test_files"]),
        malformed_fixtures=tuple(
            _load_fixture(value, surface_id)
            for value in raw.get("malformed_fixtures", [])
        ),
    )


def load_canonical_catalog(path: str = CATALOG_PATH) -> list[SurfaceRecord]:
    with open(path, "r", encoding="utf-8") as catalog_file:
        data = json.load(catalog_file)
    surfaces = data.get("surfaces")
    if not isinstance(surfaces, list):
        raise ValueError("canonical catalog must contain a surfaces array")
    return [_load_surface(surface) for surface in surfaces]


def get_canonical_catalog(path: Optional[str] = None) -> list[SurfaceRecord]:
    return load_canonical_catalog(path or CATALOG_PATH)


def _normalized_raw_surface(raw: dict[str, object]) -> dict[str, object]:
    normalized = dict(raw)
    normalized.setdefault("ownership_scope", "")
    normalized.setdefault("malformed_fixtures", [])
    return {name: normalized.get(name) for name in CANONICAL_SURFACE_FIELDS}


def projection_drift_errors(
    catalog: list[SurfaceRecord], path: str = CATALOG_PATH,
) -> list[str]:
    with open(path, "r", encoding="utf-8") as catalog_file:
        raw_surfaces = json.load(catalog_file).get("surfaces", [])
    errors: list[str] = []
    raw_by_id: dict[str, dict[str, object]] = {}
    for raw in raw_surfaces:
        if not isinstance(raw, dict):
            errors.append("surface entry is not an object")
            continue
        unknown = sorted(set(raw) - set(CANONICAL_SURFACE_FIELDS))
        if unknown:
            errors.append(f"surface {raw.get('id', '<unknown>')}: unknown canonical fields {unknown}")
        surface_id = str(raw.get("id", ""))
        if surface_id in raw_by_id:
            errors.append(f"duplicate surface: {surface_id}")
        raw_by_id[surface_id] = raw
    projected = {surface.id: surface for surface in catalog}
    for surface_id in sorted(set(raw_by_id) | set(projected)):
        if surface_id not in projected:
            errors.append(f"missing projected surface: {surface_id}")
            continue
        if surface_id not in raw_by_id:
            errors.append(f"unknown projected surface: {surface_id}")
            continue
        expected = _normalized_raw_surface(raw_by_id[surface_id])
        actual = projected[surface_id].canonical_dict()
        for name in CANONICAL_SURFACE_FIELDS:
            if actual[name] != expected[name]:
                errors.append(
                    f"projection drift: {surface_id}.{name} "
                    f"projected={actual[name]!r} canonical={expected[name]!r}"
                )
    return errors


def validate_canonical_catalog(catalog: list[SurfaceRecord]) -> list[str]:
    errors: list[str] = []
    seen: set[str] = set()
    statuses = {"active", "static", "prospective", "external", "detection_only"}
    sensitivities = {"low", "medium", "high"}
    binary_policies = {
        "reject", "public_certificate_only", "public_key_only", "exact_hash_fixture",
        "archive_member_scan", "text_only_within_mixed_root", "not_applicable",
    }
    persistence_policies = {
        "atomic_redacted_json", "atomic_redacted_text", "atomic_redacted_config",
        "static_scanned", "reject_binary", "allow_public_certificate",
        "allow_public_key", "exact_hash_fixture", "external_validated",
        "prospective_no_writer",
    }
    for surface in catalog:
        if not surface.id:
            errors.append("missing surface: empty id")
        if surface.id in seen:
            errors.append(f"duplicate surface: {surface.id}")
        seen.add(surface.id)
        if not surface.path:
            errors.append(f"missing surface: empty path ({surface.id})")
        if not surface.producer:
            errors.append(f"missing surface: empty producer ({surface.id})")
        if surface.status not in statuses:
            errors.append(f"unknown surface status: {surface.id} status={surface.status!r}")
        if surface.sensitivity not in sensitivities:
            errors.append(f"sensitivity mismatch: {surface.id} sensitivity={surface.sensitivity!r}")
        if surface.binary_policy not in binary_policies:
            errors.append(f"policy mismatch: {surface.id} binary_policy={surface.binary_policy!r}")
        if surface.persistence_policy not in persistence_policies:
            errors.append(
                f"policy mismatch: {surface.id} persistence_policy={surface.persistence_policy!r}"
            )
        if surface.enforcement_state not in {"migrated", "legacy_bypass", "not_applicable"}:
            errors.append(f"enforcement state mismatch: {surface.id}")
        if surface.ownership_scope not in {"", "symbol", "dedicated_file"}:
            errors.append(f"ownership scope mismatch: {surface.id}")
        if surface.enforcement_state == "migrated":
            if surface.status != "active":
                errors.append(f"status mismatch: {surface.id} migrated surface must be active")
            if surface.ownership_scope not in {"symbol", "dedicated_file"}:
                errors.append(f"ownership scope mismatch: {surface.id} migrated surface requires scope")
        elif surface.ownership_scope:
            errors.append(f"ownership scope mismatch: {surface.id} non-migrated surface declares scope")
        if surface.status != "active" and surface.enforcement_state != "not_applicable":
            errors.append(f"enforcement state mismatch: {surface.id} non-active surface")
        if surface.status == "active" and surface.sensitivity == "high" and surface.sanitizer == "none":
            errors.append(f"sanitizer mismatch: {surface.id} ACTIVE high sensitivity")
        if surface.status == "prospective" and surface.persistence_policy != "prospective_no_writer":
            errors.append(f"policy mismatch: {surface.id} PROSPECTIVE policy")
        if surface.status == "static" and surface.writer_files:
            errors.append(f"unknown surface: {surface.id} STATIC cannot claim writer_files")
        if surface.status == "active" and surface.binary_policy != "exact_hash_fixture" and not surface.writer_files:
            errors.append(f"status mismatch: {surface.id} ACTIVE requires writer_files")
    return errors


ARTIFACT_INVENTORY: list[SurfaceRecord] = get_canonical_catalog()


def compute_file_sha256(file_path: str) -> str:
    digest = hashlib.sha256()
    try:
        with open(file_path, "rb") as fixture_file:
            for chunk in iter(lambda: fixture_file.read(65536), b""):
                digest.update(chunk)
        return digest.hexdigest()
    except (OSError, IOError):
        return ""


def validate_malformed_fixture(fixture: MalformedFixture, repo_root: str) -> tuple[bool, str]:
    if not fixture.justification or not fixture.justification.strip():
        return False, f"Malformed fixture {fixture.path}: empty justification"
    if not fixture.owner or not fixture.owner.strip():
        return False, f"Malformed fixture {fixture.path}: empty owner"
    if not fixture.fingerprint or not fixture.fingerprint.strip():
        return False, f"Malformed fixture {fixture.path}: empty fingerprint"
    if "*" in fixture.path or "?" in fixture.path:
        return False, f"Malformed fixture {fixture.path}: path cannot be glob pattern"
    abs_path = os.path.join(repo_root, fixture.path)
    if not os.path.exists(abs_path):
        return False, f"Malformed fixture {fixture.path}: file does not exist"
    if os.path.isdir(abs_path):
        return False, f"Malformed fixture {fixture.path}: path is a directory"
    actual = compute_file_sha256(abs_path)
    if not actual:
        return False, f"Malformed fixture {fixture.path}: cannot compute file fingerprint"
    if actual.lower() != fixture.fingerprint.lower():
        return False, f"Malformed fixture {fixture.path}: stale fingerprint (file changed)"
    return True, ""


def is_malformed_fixture_exempt(path: str, repo_root: str) -> tuple[bool, str]:
    normalized = os.path.normpath(path)
    for surface in ARTIFACT_INVENTORY:
        for fixture in surface.malformed_fixtures:
            if os.path.normpath(fixture.path) == normalized:
                return validate_malformed_fixture(fixture, repo_root)
    return False, ""


def get_artifact_by_id(artifact_id: str) -> Optional[SurfaceRecord]:
    return next((surface for surface in ARTIFACT_INVENTORY if surface.id == artifact_id), None)


def get_artifacts_by_producer(producer: str) -> list[SurfaceRecord]:
    return [surface for surface in ARTIFACT_INVENTORY if surface.producer == producer]


def get_artifacts_by_sensitivity(sensitivity: ArtifactSensitivity | str) -> list[SurfaceRecord]:
    value = sensitivity.value if isinstance(sensitivity, ArtifactSensitivity) else sensitivity
    return [surface for surface in ARTIFACT_INVENTORY if surface.sensitivity == value]


def validate_inventory() -> list[str]:
    errors = validate_canonical_catalog(ARTIFACT_INVENTORY)
    errors.extend(projection_drift_errors(ARTIFACT_INVENTORY))
    for surface in ARTIFACT_INVENTORY:
        for fixture in surface.malformed_fixtures:
            if not all((fixture.path, fixture.fingerprint, fixture.justification, fixture.owner)):
                errors.append(f"Artifact surface {surface.id} has incomplete malformed fixture")
    return errors


def verify_canonical_catalog_parity(repo_root: str = "") -> list[str]:
    del repo_root
    return projection_drift_errors(ARTIFACT_INVENTORY)
