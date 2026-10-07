"""Structured component data types and content-identity primitives."""

from __future__ import annotations

import copy
import hashlib
import json
import os
import re
from dataclasses import dataclass
from pathlib import Path
from types import MappingProxyType
from typing import Any, Mapping
from urllib.parse import urlparse

SEMANTIC_KEY_VERSION = "structured-component-semantic-input/v2"
DEPENDENCY_RECORD_VERSION = "structured-component-dependencies/v1"
# These analyzer fields are retained in the exact audit identity but are not
# semantic synthesis inputs. Producer/schema compatibility is checked
# separately. Recent changes and scan statistics are not supplied to reusable
# synthesis at all.
_ANALYZER_PRESENTATION_FIELDS = frozenset(
    {
        "analyzer_version",
        "commit_sha",
        "extracted_at",
        "generated_at",
        "schema_version",
    }
)
_ANALYZER_NOT_SYNTHESIZED = frozenset({"recent_changes", "scan_statistics"})
_LEGACY_SCAN_SUMMARY = re.compile(
    r"^summary:scanned \d+ (?:Python source files for authentication constructions|"
    r"runtime source/config files against \d+ platform aliases)$"
)

class ReuseConfigurationError(ValueError):
    """The explicit predecessor configuration is invalid."""

class ReuseRecordError(ValueError):
    """A prospective snapshot or dependency record is incomplete."""

def _canonical_bytes(value: Any) -> bytes:
    return json.dumps(
        value,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
        allow_nan=False,
    ).encode("utf-8")

def content_hash(value: bytes | Any) -> str:
    """Return a named SHA-256 over exact bytes or canonical JSON."""

    raw = value if isinstance(value, bytes) else _canonical_bytes(value)
    return "sha256:" + hashlib.sha256(raw).hexdigest()

def _analyzer_bundle_fingerprint(value: Mapping[str, Any]) -> str:
    """Match Go's canonical JSON HTML escaping for analyzer bundle identity."""

    canonical = _canonical_bytes(dict(value))
    for character, escape in (
        (b"&", b"\\u0026"),
        (b"<", b"\\u003c"),
        (b">", b"\\u003e"),
        ("\u2028".encode(), b"\\u2028"),
        ("\u2029".encode(), b"\\u2029"),
    ):
        canonical = canonical.replace(character, escape)
    return content_hash(canonical)

def file_hash(path: str | Path) -> str:
    digest = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return "sha256:" + digest.hexdigest()

def _frozen(value: Any) -> Any:
    if isinstance(value, Mapping):
        return MappingProxyType(
            {str(key): _frozen(item) for key, item in value.items()}
        )
    if isinstance(value, (list, tuple)):
        return tuple(_frozen(item) for item in value)
    return value

def thaw(value: Any) -> Any:
    """Return a detached mutable JSON-shaped copy of frozen snapshot data."""

    if isinstance(value, Mapping):
        return {str(key): thaw(item) for key, item in value.items()}
    if isinstance(value, tuple):
        return [thaw(item) for item in value]
    return copy.deepcopy(value)

def canonical_repository_identity(repository: str) -> str:
    """Normalize common Git remote spellings without guessing repository aliases."""

    candidate = repository.strip()
    if candidate.startswith("git@") and ":" in candidate:
        host, path = candidate[4:].split(":", 1)
        candidate = f"ssh://git@{host}/{path}"
    parsed = urlparse(candidate)
    if parsed.scheme and parsed.hostname:
        path = parsed.path
        host = parsed.hostname.casefold()
    else:
        path = candidate
        host = ""
    path = path.strip("/")
    if path.endswith(".git"):
        path = path[:-4]
    if not path or path in {".", ".."}:
        raise ReuseRecordError("repository identity is empty")
    normalized = path.casefold() if host == "github.com" else path
    return f"{host}/{normalized}" if host else normalized

@dataclass(frozen=True)
class ComponentIdentity:
    component: str
    repository: str
    aliases: tuple[str, ...] = ()

    def __post_init__(self) -> None:
        component = self.component.strip()
        if not component:
            raise ReuseRecordError("component identity is empty")
        aliases = tuple(sorted({item.strip() for item in self.aliases if item.strip()}))
        object.__setattr__(self, "component", component)
        object.__setattr__(
            self, "repository", canonical_repository_identity(self.repository)
        )
        object.__setattr__(self, "aliases", aliases)

    @property
    def names(self) -> frozenset[str]:
        return frozenset((self.component, *self.aliases))

@dataclass(frozen=True)
class ProducerCompatibility:
    analyzer_schema_version: str
    analyzer_version: str
    analyzer_build_identity: str
    normalizer_version: str
    synthesis_contract_version: str

    def complete(self) -> bool:
        return all(
            (
                self.analyzer_schema_version,
                self.analyzer_version,
                self.analyzer_build_identity,
                self.normalizer_version,
                self.synthesis_contract_version,
            )
        )

@dataclass(frozen=True)
class SourceSnapshot:
    head: str
    working_tree_identity: str
    tracked_tree_identity: str
    dirty_paths: tuple[str, ...] = ()
    untracked_paths: tuple[str, ...] = ()

@dataclass(frozen=True)
class SourceRunState:
    start: SourceSnapshot
    end: SourceSnapshot

    @property
    def unchanged(self) -> bool:
        return self.start == self.end

@dataclass(frozen=True)
class FileDependency:
    path: str
    content_hash: str
    kind: str = "checkout"

@dataclass(frozen=True)
class ReadObservation:
    path: str
    outcome: str = "observed-by-harness"
    offset: int | None = None
    limit: int | None = None

@dataclass(frozen=True)
class SearchObservation:
    root: str
    resolved_root: str
    pattern: str
    options: tuple[tuple[str, Any], ...]
    tree_identity: str
    tool: str
    justification: str
    outcome: str = "observed-by-harness"
    tracked_clean: bool = True
    replay_result_identity: str = ""
    replay_context_identity: str = ""
    observed_result_identity: str = ""
    execution_cwd: str = "."

    @property
    def reusable_shape(self) -> dict[str, Any]:
        return {
            "root": self.root,
            "pattern": self.pattern,
            "options": {key: thaw(value) for key, value in self.options},
            "tree_identity": self.tree_identity,
            "tool": self.tool,
            "justification": self.justification,
            "outcome": self.outcome,
            "tracked_clean": self.tracked_clean,
            "replay_result_identity": self.replay_result_identity,
            "replay_context_identity": self.replay_context_identity,
            "execution_cwd": self.execution_cwd,
        }

@dataclass(frozen=True)
class DependencyRecord:
    harness: str
    complete: bool
    reads: tuple[ReadObservation, ...]
    supplied_reads: tuple[str, ...]
    justifications: Mapping[str, str]
    files: tuple[FileDependency, ...]
    searches: tuple[SearchObservation, ...]
    unclassified_source_commands: tuple[str, ...] = ()
    accounted_untracked: tuple[str, ...] = ()
    generated_inputs: tuple[FileDependency, ...] = ()
    external_inputs: tuple[FileDependency, ...] = ()
    schema_version: str = DEPENDENCY_RECORD_VERSION

    def __post_init__(self) -> None:
        object.__setattr__(self, "justifications", _frozen(dict(self.justifications)))

    def reusable_payload(self) -> dict[str, Any]:
        return {
            "schema_version": self.schema_version,
            "harness": self.harness,
            "reads": [vars(item) for item in self.reads],
            "supplied_reads": list(self.supplied_reads),
            "justifications": thaw(self.justifications),
            "files": [vars(item) for item in self.files],
            "searches": [item.reusable_shape for item in self.searches],
            "accounted_untracked": list(self.accounted_untracked),
            "generated_inputs": [vars(item) for item in self.generated_inputs],
            "external_inputs": [vars(item) for item in self.external_inputs],
        }

@dataclass(frozen=True)
class InputIdentities:
    exact: str
    semantic: str
    semantic_payload: Mapping[str, Any]
    analyzer_binding: Mapping[str, Any]
    recent_changes: tuple[Any, ...]

    def __post_init__(self) -> None:
        object.__setattr__(self, "semantic_payload", _frozen(self.semantic_payload))
        object.__setattr__(self, "analyzer_binding", _frozen(self.analyzer_binding))
        object.__setattr__(self, "recent_changes", _frozen(self.recent_changes))

@dataclass(frozen=True)
class AcceptedSnapshot:
    snapshot_id: str
    platform: str
    identity: ComponentIdentity
    accepted: bool
    route: str
    inputs: InputIdentities
    compatibility: ProducerCompatibility
    dependencies: DependencyRecord
    source_state: SourceRunState
    synthesis_bytes: bytes
    synthesis_integrity: str
    response_identity: str
    document: Mapping[str, Any]
    document_integrity: str

    def __post_init__(self) -> None:
        object.__setattr__(self, "synthesis_bytes", bytes(self.synthesis_bytes))
        object.__setattr__(self, "document", _frozen(thaw(self.document)))

def _mask_checkout_paths(value: Any, checkout_root: str | Path | None) -> Any:
    if checkout_root is None:
        return value
    root = str(Path(checkout_root).resolve()).rstrip(os.sep)
    prefix = root + os.sep
    if isinstance(value, str):
        if value == root:
            return "<checkout>"
        return value.replace(prefix, "<checkout>/")
    if isinstance(value, list):
        return [_mask_checkout_paths(item, checkout_root) for item in value]
    if isinstance(value, dict):
        return {
            key: _mask_checkout_paths(item, checkout_root)
            for key, item in value.items()
        }
    return value

def _semantic_analyzer(
    analyzer_input: Mapping[str, Any],
    checkout_root: str | Path | None,
) -> dict[str, Any]:
    result = {
        key: copy.deepcopy(value)
        for key, value in analyzer_input.items()
        if key not in _ANALYZER_PRESENTATION_FIELDS | _ANALYZER_NOT_SYNTHESIZED
    }
    # Compatibility with analyzer outputs predating the dedicated top-level
    # scan_statistics field. Only the reviewed, exact scan-summary form is
    # excluded; no other coverage, evidence, gap, or cross-reference text is.
    coverage = result.get("category_coverage")
    if isinstance(coverage, dict):
        for record in coverage.values():
            if not isinstance(record, dict) or not isinstance(
                record.get("evidence"), list
            ):
                continue
            record["evidence"] = [
                item
                for item in record["evidence"]
                if not (isinstance(item, str) and _LEGACY_SCAN_SUMMARY.match(item))
            ]
    return _mask_checkout_paths(result, checkout_root)

def _predecessor_document_binding_errors(
    prior: AcceptedSnapshot,
) -> list[str]:
    document = thaw(prior.document)
    identity = document.get("identity")
    producers = document.get("producers")
    analyzer_input = document.get("analyzer_input")
    analyzer_binding = thaw(prior.inputs.analyzer_binding)
    errors: list[str] = []
    if not isinstance(identity, dict):
        return ["predecessor document identity is missing"]
    if identity.get("component") != prior.identity.component:
        errors.append("predecessor document component is not snapshot-bound")
    source_component = identity.get("source_component")
    if (
        not isinstance(source_component, str)
        or source_component not in prior.identity.names
    ):
        errors.append("predecessor source component is not snapshot-bound")
    try:
        document_repository = canonical_repository_identity(
            str(identity.get("repository") or "")
        )
    except ReuseRecordError:
        document_repository = ""
    if document_repository != prior.identity.repository:
        errors.append("predecessor document repository is not snapshot-bound")
    aliases = identity.get("aliases")
    if not isinstance(aliases, (list, tuple)) or not all(
        isinstance(alias, str) for alias in aliases
    ):
        errors.append("predecessor document aliases are malformed")
    elif tuple(sorted(aliases)) != prior.identity.aliases:
        errors.append("predecessor document aliases are not snapshot-bound")
    if identity.get("version_scope") != prior.platform:
        errors.append("predecessor document version scope is not snapshot-bound")
    if identity.get("source_revision") != prior.source_state.end.head:
        errors.append("predecessor document source revision is not snapshot-bound")
    if identity.get("source_revision") != analyzer_binding.get("commit_sha"):
        errors.append("predecessor source revision is not analyzer-input-bound")
    if not isinstance(producers, dict):
        errors.append("predecessor document producers are missing")
    else:
        if producers.get(
            "analyzer_version"
        ) != prior.compatibility.analyzer_version or producers.get(
            "analyzer_version"
        ) != analyzer_binding.get("analyzer_version"):
            errors.append("predecessor analyzer version is not snapshot-bound")
        if (
            producers.get("normalizer_version")
            != prior.compatibility.normalizer_version
        ):
            errors.append("predecessor normalizer version is not snapshot-bound")
    if not isinstance(analyzer_input, dict):
        errors.append("predecessor analyzer input is missing")
    elif analyzer_input.get(
        "schema_version"
    ) != prior.compatibility.analyzer_schema_version or analyzer_input.get(
        "schema_version"
    ) != analyzer_binding.get("schema_version"):
        errors.append("predecessor analyzer schema is not snapshot-bound")
    else:
        if analyzer_input.get("bundle_fingerprint") != analyzer_binding.get(
            "bundle_fingerprint"
        ):
            errors.append("predecessor analyzer fingerprint is not input-bound")
        if analyzer_input.get("extracted_at") != analyzer_binding.get("extracted_at"):
            errors.append("predecessor extraction metadata is not input-bound")
    predecessor_revision = prior.source_state.end.head
    for collection in (
        document.get("facts", ()),
        document.get("proposal_dispositions", ()),
    ):
        if not isinstance(collection, list):
            continue
        for item in collection:
            if not isinstance(item, dict):
                continue
            evidence = item.get("evidence", ())
            if isinstance(evidence, list) and any(
                isinstance(reference, dict)
                and reference.get("revision") != predecessor_revision
                for reference in evidence
            ):
                errors.append("predecessor evidence revision is not snapshot-bound")
                return errors
    return errors

