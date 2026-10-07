"""Bounded, parent-controlled structured component synthesis.

The module is opt-in and writes only private run artifacts. It never parses
Markdown candidates, never falls back to the legacy route, and never publishes
an accepted component. Model calls are injected behind a one-response adapter;
the parent owns evidence, limits, validation, assembly policy, and persistence.
"""

from __future__ import annotations

import base64
import copy
import json
import math
import os
import subprocess
import tempfile
from pathlib import Path
from typing import Any, Mapping, Sequence

from jsonschema import Draft202012Validator
from referencing import Registry, Resource

from lib.structured_component_reuse import (
    SEMANTIC_KEY_VERSION,
    AcceptedSnapshot,
    ComponentIdentity,
    DependencyRecord,
    FileDependency,
    InputIdentities,
    ProducerCompatibility,
    ReuseConfigurationError,
    ReuseRecordError,
    SearchObservation,
    SourceRunState,
    SourceSnapshot,
    _analyzer_bundle_fingerprint,
    _predecessor_document_binding_errors,
    content_hash,
    file_hash,
    thaw,
)
from lib.telemetry_redact import redact_dict

SYNTHESIS_ROUTE = "structured-component/v1"
RESPONSE_SCHEMA = "structured-component-synthesis-response-v1.schema.json"
BUNDLE_SCHEMA = "structured-component-evidence-bundle-v1.schema.json"
ENVELOPE_SCHEMA = "structured-component-synthesis-envelope-v1.schema.json"
RUN_RECORD_SCHEMA = "structured-component-private-run-record-v1.schema.json"
DOCUMENT_SCHEMA = "structured-component-document-v1.schema.json"
SCHEMA_VERSION = "1.0.0"
PRIVATE_RUN_RECORD = "structured-component-private-run/v1"
MAX_RESPONSE_JSON_DEPTH = 64
RUN_RECORD_FILENAME = "run-record.json"
SYNTHESIS_BUNDLE_FILENAME = "synthesis-evidence-bundle.json"
_SCHEMA_DIR = Path(__file__).resolve().parent.parent / "schemas"

class StructuredSynthesisError(RuntimeError):
    """The bounded structured route could not produce an accepted result."""

    def __init__(
        self,
        message: str,
        *,
        provider_error: Mapping[str, Any] | None = None,
    ) -> None:
        super().__init__(message)
        self.provider_error = redact_dict(copy.deepcopy(dict(provider_error or {})))

class EvidenceScopeError(StructuredSynthesisError):
    """A requested or cited source falls outside the parent-owned boundary."""

def _canonical(value: Any) -> bytes:
    return json.dumps(
        value,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
        allow_nan=False,
    ).encode()

def _raw_response_text(value: Any) -> str:
    if isinstance(value, str):
        return value
    if not isinstance(value, Mapping) or value.get("encoding") != (
        "utf-8-surrogatepass-base64"
    ):
        raise ReuseRecordError("predecessor raw response representation is invalid")
    try:
        raw = base64.b64decode(str(value["data"]), validate=True).decode(
            "utf-8", errors="surrogatepass"
        )
    except (KeyError, ValueError, UnicodeDecodeError) as error:
        raise ReuseRecordError(
            "predecessor raw response representation is invalid"
        ) from error
    if len(raw) != value.get("character_count"):
        raise ReuseRecordError(
            "predecessor raw response representation length is invalid"
        )
    return raw

def _raw_response_hash(value: str | Mapping[str, Any]) -> str:
    if isinstance(value, str):
        return content_hash(value.encode("utf-8"))
    return content_hash(dict(value))

def _reject_excessive_json_nesting(raw: str) -> None:
    depth = 0
    quoted = False
    escaped = False
    for character in raw:
        if quoted:
            if escaped:
                escaped = False
            elif character == "\\":
                escaped = True
            elif character == '"':
                quoted = False
            continue
        if character == '"':
            quoted = True
        elif character in "[{":
            depth += 1
            if depth > MAX_RESPONSE_JSON_DEPTH:
                raise StructuredSynthesisError(
                    "response JSON nesting exceeds configured maximum "
                    f"of {MAX_RESPONSE_JSON_DEPTH}"
                )
        elif character in "]}":
            depth -= 1

def _validate_json_text(value: Any, path: str = "$") -> None:
    if isinstance(value, str):
        try:
            value.encode("utf-8")
        except UnicodeEncodeError as error:
            raise StructuredSynthesisError(
                f"response contains unpaired surrogate text at {path}"
            ) from error
        return
    if isinstance(value, Mapping):
        for key, item in value.items():
            _validate_json_text(str(key), f"{path}.<key>")
            _validate_json_text(item, f"{path}.{key}")
    elif isinstance(value, (list, tuple)):
        for index, item in enumerate(value):
            _validate_json_text(item, f"{path}[{index}]")

def _schemas() -> tuple[dict[str, dict[str, Any]], Registry]:
    schemas = {
        path.name: json.loads(path.read_text())
        for path in sorted(_SCHEMA_DIR.glob("structured-*.schema.json"))
    }
    registry = Registry().with_resources(
        [(schema["$id"], Resource.from_contents(schema)) for schema in schemas.values()]
    )
    return schemas, registry

def _validate_schema(name: str, value: Any) -> None:
    schemas, registry = _schemas()
    validator = Draft202012Validator(schemas[name], registry=registry)
    errors = sorted(validator.iter_errors(value), key=lambda item: list(item.path))
    if errors:
        error = errors[0]
        location = "/" + "/".join(str(item) for item in error.absolute_path)
        raise StructuredSynthesisError(
            f"{name} validation failed at {location}: {error.message}"
        )

def _atomic_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    payload = json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n"
    temporary = path.with_name(f".{path.name}.tmp")
    temporary.write_text(payload)
    temporary.replace(path)

def _bundle_without_identity(bundle: Mapping[str, Any]) -> dict[str, Any]:
    result = copy.deepcopy(dict(bundle))
    result.pop("bundle_identity", None)
    return result

def _response_diagnostics(
    payload: Mapping[str, Any], bundle: Mapping[str, Any]
) -> list[str]:
    errors: list[str] = []
    if payload.get("input_bundle_identity") != bundle["bundle_identity"]:
        errors.append("response input_bundle_identity does not match current bundle")
    status = payload.get("completion_status")
    requests = payload.get("evidence_requests")
    if status == "complete" and requests:
        errors.append("complete response must not contain evidence_requests")
    if status == "needs-evidence" and not requests:
        errors.append("needs-evidence response requires evidence_requests")
    if status == "unresolved" and not payload.get("limitations"):
        errors.append("unresolved response requires explicit limitations")
    if status == "unresolved" and requests:
        errors.append("unresolved response must not contain evidence_requests")
    analyzer_fingerprint = _analyzer_bundle_fingerprint(bundle["analyzer"]["payload"])
    patch_ids: set[str] = set()
    for patch in payload.get("typed_patches", ()):
        patch_id = patch.get("patch_id")
        if patch_id in patch_ids:
            errors.append(f"duplicate patch_id: {patch_id}")
        patch_ids.add(patch_id)
        if patch.get("bundle_fingerprint") != analyzer_fingerprint:
            errors.append(f"patch {patch_id} is not bound to the analyzer bundle")
    section_ids = [item.get("id") for item in payload.get("sections", ())]
    if len(section_ids) != len(set(section_ids)):
        errors.append("duplicate section id")
    return errors

def _parse_response(raw: str, bundle: Mapping[str, Any]) -> dict[str, Any]:
    _reject_excessive_json_nesting(raw)

    def reject_constant(value: str) -> None:
        raise ValueError(f"non-standard nonfinite number {value} is not allowed")

    def finite_float(value: str) -> float:
        result = float(value)
        if not math.isfinite(result):
            raise ValueError(f"nonfinite number {value} is not allowed")
        return result

    try:
        payload = json.loads(
            raw,
            parse_constant=reject_constant,
            parse_float=finite_float,
        )
    except (json.JSONDecodeError, RecursionError, ValueError) as error:
        raise StructuredSynthesisError(
            f"response is not one JSON value: {error}"
        ) from error
    if not isinstance(payload, dict):
        raise StructuredSynthesisError("response JSON root must be an object")
    _validate_json_text(payload)
    try:
        _validate_schema(RESPONSE_SCHEMA, payload)
    except RecursionError as error:
        raise StructuredSynthesisError(
            "response validation exceeded the configured structural bound"
        ) from error
    errors = _response_diagnostics(payload, bundle)
    if errors:
        raise StructuredSynthesisError("; ".join(errors))
    return payload

def _evidence_covers(
    reference: Mapping[str, Any], evidence: Sequence[Mapping[str, Any]]
) -> bool:
    start = reference.get("start_line")
    end = reference.get("end_line")
    for supplied in evidence:
        if reference.get("path") != supplied.get("path") or reference.get(
            "revision"
        ) != supplied.get("revision"):
            continue
        if start is None and end is None:
            return supplied.get("start_line") == 1 and supplied.get(
                "end_line"
            ) == supplied.get("whole_file_line_count")
        if (
            isinstance(start, int)
            and isinstance(end, int)
            and supplied["start_line"] <= start <= end <= supplied["end_line"]
        ):
            return True
    return False

def _validate_model_evidence(
    payload: Mapping[str, Any], bundle: Mapping[str, Any]
) -> None:
    references: list[Mapping[str, Any]] = []
    for section in payload["sections"]:
        references.extend(section["evidence"])
        for block in section["blocks"]:
            for row in block.get("rows", ()):
                references.extend(row["evidence"])
    for patch in payload["typed_patches"]:
        for operation in patch["operations"]:
            references.extend(operation["evidence"])
    missing = [
        f"{item.get('path')}:{item.get('start_line', '?')}-{item.get('end_line', '?')}"
        for item in references
        if not _evidence_covers(item, bundle["source_evidence"])
    ]
    if missing:
        raise EvidenceScopeError(
            "model cited evidence not supplied by parent: " + ", ".join(missing)
        )

class GoStructuredAssembler:
    """Invoke the current Go normalizer/validator and sole Markdown renderer."""

    def __init__(self, command: Sequence[str], cwd: Path) -> None:
        self.command = tuple(command)
        self.cwd = cwd

    def _run(self, arguments: Sequence[str]) -> None:
        completed = subprocess.run(
            [*self.command, *arguments],
            cwd=self.cwd,
            env={**os.environ, "GOCACHE": "/tmp/structured-component-go-cache"},
            check=False,
            capture_output=True,
            text=True,
        )
        if completed.returncode != 0:
            raise StructuredSynthesisError(
                "Go structured assembly failed: "
                + (completed.stderr or completed.stdout).strip()
            )

    def validate_and_render(self, document: Path, markdown: Path) -> None:
        _validate_schema(DOCUMENT_SCHEMA, json.loads(document.read_text()))
        self._run(
            ["render-document", "--input", str(document), "--output", str(markdown)]
        )

def special_synthesis_envelope(
    state: str,
    *,
    input_bundle_identity: str,
    reason: str,
) -> dict[str, Any]:
    """Represent deterministic-only and missing historical response states."""

    if state not in {"deterministic-only", "historical-response-missing"}:
        raise ValueError("unsupported special synthesis state")
    envelope = {
        "schema_version": SCHEMA_VERSION,
        "state": state,
        "route": SYNTHESIS_ROUTE,
        "input_bundle_identity": input_bundle_identity,
        "context_identity": content_hash({"state": state}),
        "reuse_eligibility": {
            "available": False,
            "reason": reason,
            "unobserved_context": [],
        },
        "requested": {"harness": "none", "model": "none", "settings": {}},
        "reported": {"models": [], "settings": [], "auxiliary_models": []},
        "calls": {
            "total": 0,
            "initial": 0,
            "evidence_followup": 0,
            "repair": 0,
            "limits": {"total": 0, "evidence_followup": 0, "repair": 0},
        },
        "responses": [],
        "resolution_provenance": [],
        "observations": {},
        "justifications": [],
        "accepted_response_identity": None,
        "diagnostics": [{"code": state, "detail": reason}],
    }
    _validate_schema(ENVELOPE_SCHEMA, envelope)
    return envelope

def _source_snapshot_value(snapshot: SourceSnapshot) -> dict[str, Any]:
    return {
        "head": snapshot.head,
        "working_tree_identity": snapshot.working_tree_identity,
        "tracked_tree_identity": snapshot.tracked_tree_identity,
        "dirty_paths": list(snapshot.dirty_paths),
        "untracked_paths": list(snapshot.untracked_paths),
    }

def _source_snapshot_from_value(value: Mapping[str, Any]) -> SourceSnapshot:
    return SourceSnapshot(
        head=str(value["head"]),
        working_tree_identity=str(value["working_tree_identity"]),
        tracked_tree_identity=str(value["tracked_tree_identity"]),
        dirty_paths=tuple(value["dirty_paths"]),
        untracked_paths=tuple(value["untracked_paths"]),
    )

def _dependency_value(record: DependencyRecord) -> dict[str, Any]:
    return {
        **record.reusable_payload(),
        "complete": record.complete,
        "unclassified_source_commands": list(record.unclassified_source_commands),
        "searches": [
            {
                "root": item.root,
                "resolved_root": item.resolved_root,
                "pattern": item.pattern,
                "options": {key: thaw(value) for key, value in item.options},
                "tree_identity": item.tree_identity,
                "tool": item.tool,
                "justification": item.justification,
                "outcome": item.outcome,
                "tracked_clean": item.tracked_clean,
                "replay_result_identity": item.replay_result_identity,
                "replay_context_identity": item.replay_context_identity,
                "observed_result_identity": item.observed_result_identity,
                "execution_cwd": item.execution_cwd,
            }
            for item in record.searches
        ],
    }

def _dependency_from_value(value: Mapping[str, Any]) -> DependencyRecord:
    from lib.structured_component_reuse import ReadObservation

    return DependencyRecord(
        schema_version=str(value["schema_version"]),
        harness=str(value["harness"]),
        complete=value["complete"] is True,
        reads=tuple(ReadObservation(**item) for item in value["reads"]),
        supplied_reads=tuple(value["supplied_reads"]),
        justifications=dict(value["justifications"]),
        files=tuple(FileDependency(**item) for item in value["files"]),
        searches=tuple(
            SearchObservation(
                root=item["root"],
                resolved_root=item.get("resolved_root", item["root"]),
                pattern=item["pattern"],
                options=tuple(sorted(item["options"].items())),
                tree_identity=item["tree_identity"],
                tool=item["tool"],
                justification=item["justification"],
                outcome=item.get("outcome", "observed-by-harness"),
                tracked_clean=item.get("tracked_clean", True),
                replay_result_identity=item.get("replay_result_identity", ""),
                replay_context_identity=item.get("replay_context_identity", ""),
                observed_result_identity=item.get("observed_result_identity", ""),
                execution_cwd=item.get("execution_cwd", "."),
            )
            for item in value["searches"]
        ),
        unclassified_source_commands=tuple(value["unclassified_source_commands"]),
        accounted_untracked=tuple(value["accounted_untracked"]),
        generated_inputs=tuple(
            FileDependency(**item) for item in value["generated_inputs"]
        ),
        external_inputs=tuple(
            FileDependency(**item) for item in value["external_inputs"]
        ),
    )

def _input_value(inputs: InputIdentities) -> dict[str, Any]:
    return {
        "exact": inputs.exact,
        "semantic": inputs.semantic,
        "semantic_payload": thaw(inputs.semantic_payload),
        "analyzer_binding": thaw(inputs.analyzer_binding),
        "recent_changes": thaw(inputs.recent_changes),
    }

def _input_from_value(value: Mapping[str, Any]) -> InputIdentities:
    return InputIdentities(
        exact=str(value["exact"]),
        semantic=str(value["semantic"]),
        semantic_payload=dict(value["semantic_payload"]),
        analyzer_binding=dict(value["analyzer_binding"]),
        recent_changes=tuple(value["recent_changes"]),
    )

def _compatibility_value(value: ProducerCompatibility) -> dict[str, str]:
    return {
        "analyzer_schema_version": value.analyzer_schema_version,
        "analyzer_version": value.analyzer_version,
        "analyzer_build_identity": value.analyzer_build_identity,
        "normalizer_version": value.normalizer_version,
        "synthesis_contract_version": value.synthesis_contract_version,
    }

def _record_without_identity(record: Mapping[str, Any]) -> dict[str, Any]:
    value = copy.deepcopy(dict(record))
    value.pop("record_identity", None)
    return value

def _successful_response(
    synthesis_bytes: bytes,
    bundle: Mapping[str, Any],
) -> tuple[dict[str, Any], str]:
    try:
        envelope = json.loads(synthesis_bytes)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise ReuseRecordError(f"predecessor synthesis is not JSON: {error}") from error
    try:
        _validate_schema(ENVELOPE_SCHEMA, envelope)
        _validate_schema(BUNDLE_SCHEMA, bundle)
    except StructuredSynthesisError as error:
        raise ReuseRecordError(str(error)) from error
    if envelope.get("state") != "synthesized":
        raise ReuseRecordError("predecessor synthesis did not complete successfully")
    eligibility = envelope.get("reuse_eligibility")
    if not isinstance(eligibility, Mapping) or eligibility.get("available") is not True:
        raise ReuseRecordError("predecessor producing envelope is reuse-ineligible")
    if envelope.get("input_bundle_identity") != bundle.get("bundle_identity"):
        raise ReuseRecordError("predecessor synthesis/evidence bundle mismatch")
    if content_hash(_bundle_without_identity(bundle)) != bundle.get("bundle_identity"):
        raise ReuseRecordError("predecessor evidence bundle identity is invalid")
    accepted_identity = envelope.get("accepted_response_identity")
    responses = envelope.get("responses", ())
    accepted_indexes = [
        index
        for index, item in enumerate(responses)
        if item.get("response_identity") == accepted_identity
    ]
    if len(accepted_indexes) != 1:
        raise ReuseRecordError("predecessor accepted response is missing or ambiguous")
    accepted_index = accepted_indexes[0]
    response = responses[accepted_index]
    requested = envelope.get("requested")
    if not isinstance(requested, Mapping):
        raise ReuseRecordError("predecessor requested producer identity is missing")
    model_context = bundle.get("synthesis_configuration", {}).get("model_context", {})
    if (
        not isinstance(model_context, Mapping)
        or requested.get("harness") != model_context.get("adapter")
        or requested.get("model") != model_context.get("requested_model")
        or requested.get("settings") != model_context.get("requested_settings")
    ):
        raise ReuseRecordError(
            "predecessor requested producer is not bound to the evidence bundle"
        )
    if (
        not isinstance(response.get("requested_model_identity"), str)
        or response.get("reported_model") != response.get("requested_model_identity")
        or response.get("reported_settings") != requested.get("settings")
    ):
        raise ReuseRecordError(
            "predecessor requested/reported producer settings are incompatible"
        )
    if requested.get("harness") in {"claude", "codex"}:
        resolution = model_context.get("implicit_context", {}).get(
            "requested_model_resolution", {}
        )
        resolved_model_identity = resolution.get("resolved_model_identity")
        if (
            not isinstance(resolved_model_identity, str)
            or response.get("requested_model_identity") != resolved_model_identity
        ):
            raise ReuseRecordError(
                "predecessor model resolution is not bound to the request"
            )
        parent_requested_model = resolution.get("parent_requested_model")
        if (
            requested.get("harness") == "codex"
            and parent_requested_model is not None
            and parent_requested_model != resolved_model_identity
        ):
            raise ReuseRecordError(
                "predecessor Codex explicit model request was substituted"
            )
    reported = envelope.get("reported")
    models = reported.get("models") if isinstance(reported, Mapping) else None
    settings = reported.get("settings") if isinstance(reported, Mapping) else None
    auxiliary = (
        reported.get("auxiliary_models") if isinstance(reported, Mapping) else None
    )
    if (
        not isinstance(models, list)
        or not isinstance(settings, list)
        or not isinstance(auxiliary, list)
        or len(models) != len(responses)
        or len(settings) != len(responses)
        or models[accepted_index] != response.get("reported_model")
        or settings[accepted_index] != response.get("reported_settings")
        or not all(item in auxiliary for item in response.get("auxiliary_models") or ())
    ):
        raise ReuseRecordError("predecessor producing model audit is inconsistent")
    raw_representation = response.get("raw_response")
    parsed = response.get("parsed_payload")
    if not isinstance(parsed, dict):
        raise ReuseRecordError("predecessor accepted response bytes are missing")
    try:
        raw = _raw_response_text(raw_representation)
        expected_raw_hash = _raw_response_hash(raw_representation)
    except (ReuseRecordError, UnicodeEncodeError, TypeError, ValueError) as error:
        raise ReuseRecordError(
            "predecessor raw response representation is invalid"
        ) from error
    if response.get("raw_response_hash") != expected_raw_hash:
        raise ReuseRecordError("predecessor raw response hash is invalid")
    if response.get("parsed_payload_hash") != content_hash(parsed):
        raise ReuseRecordError("predecessor parsed response hash is invalid")
    try:
        reparsed = _parse_response(raw, bundle)
        _validate_model_evidence(reparsed, bundle)
    except (StructuredSynthesisError, EvidenceScopeError) as error:
        raise ReuseRecordError(f"predecessor response is invalid: {error}") from error
    if reparsed != parsed or response.get("validation", {}).get("valid") is not True:
        raise ReuseRecordError("predecessor raw and parsed responses are inconsistent")
    if not isinstance(accepted_identity, str) or not accepted_identity:
        raise ReuseRecordError("predecessor response identity is missing")
    return parsed, accepted_identity

class PrivateRunRecordStore:
    """Strict loader/writer for the phase-three private staged run record."""

    _ARTIFACTS = {
        "evidence_bundle": "evidence-bundle.json",
        "synthesis": "synthesis.json",
        "document": "document.json",
        "markdown": "candidate.md",
    }

    def __init__(
        self, architecture_dir: Path, assembler: GoStructuredAssembler
    ) -> None:
        self.architecture_dir = architecture_dir.resolve()
        self.assembler = assembler

    def directory(self, version_scope: str, component: str) -> Path:
        for label, value in (("version", version_scope), ("component", component)):
            if (
                not value
                or Path(value).name != value
                or value in {".", ".."}
                or any(ord(character) < 32 for character in value)
            ):
                raise ReuseConfigurationError(f"invalid predecessor {label}: {value!r}")
        directory = (
            self.architecture_dir
            / version_scope
            / component
            / ".generation"
            / "structured"
        ).resolve()
        try:
            directory.relative_to(self.architecture_dir)
        except ValueError as error:
            raise ReuseConfigurationError(
                "predecessor location escapes architecture"
            ) from error
        return directory

    def invalidate(self, version_scope: str, component: str) -> None:
        (self.directory(version_scope, component) / RUN_RECORD_FILENAME).unlink(
            missing_ok=True
        )

    def load(self, version_scope: str, component: str) -> AcceptedSnapshot:
        directory = self.directory(version_scope, component)
        record_path = directory / RUN_RECORD_FILENAME
        try:
            record = json.loads(record_path.read_text())
        except (OSError, json.JSONDecodeError) as error:
            raise ReuseRecordError(
                f"invalid predecessor run record {record_path}: {error}"
            ) from error
        try:
            _validate_schema(RUN_RECORD_SCHEMA, record)
        except StructuredSynthesisError as error:
            raise ReuseRecordError(str(error)) from error
        if record["record_identity"] != content_hash(_record_without_identity(record)):
            raise ReuseRecordError("predecessor run record identity is invalid")
        if record["version_scope"] != version_scope or record["component"] != component:
            raise ReuseRecordError("predecessor run record selection is inconsistent")
        artifacts: dict[str, bytes] = {}
        for name, filename in {
            **self._ARTIFACTS,
            "synthesis_bundle": record["artifacts"]["synthesis_bundle"]["path"],
        }.items():
            reference = record["artifacts"][name]
            allowed_paths = (
                {"evidence-bundle.json", SYNTHESIS_BUNDLE_FILENAME}
                if name == "synthesis_bundle"
                else {filename}
            )
            if reference["path"] not in allowed_paths:
                raise ReuseRecordError(f"predecessor {name} path is not canonical")
            path = directory / reference["path"]
            try:
                raw = path.read_bytes()
            except OSError as error:
                raise ReuseRecordError(
                    f"predecessor {name} is missing: {error}"
                ) from error
            if content_hash(raw) != reference["content_hash"]:
                raise ReuseRecordError(f"predecessor {name} hash is invalid")
            artifacts[name] = raw
        try:
            bundle = json.loads(artifacts["evidence_bundle"])
            document = json.loads(artifacts["document"])
        except (UnicodeDecodeError, json.JSONDecodeError) as error:
            raise ReuseRecordError(
                f"predecessor artifact is not JSON: {error}"
            ) from error
        try:
            _validate_schema(BUNDLE_SCHEMA, bundle)
        except StructuredSynthesisError as error:
            raise ReuseRecordError(str(error)) from error
        if content_hash(_bundle_without_identity(bundle)) != bundle.get(
            "bundle_identity"
        ):
            raise ReuseRecordError(
                "predecessor current evidence bundle identity is invalid"
            )
        synthesis_bundle = json.loads(artifacts["synthesis_bundle"])
        _, response_identity = _successful_response(
            artifacts["synthesis"], synthesis_bundle
        )
        if record["response_identity"] != response_identity:
            raise ReuseRecordError("predecessor response identity is inconsistent")
        try:
            with tempfile.TemporaryDirectory(prefix="structured-predecessor-") as raw:
                rendered = Path(raw) / "candidate.md"
                document_path = Path(raw) / "document.json"
                document_path.write_bytes(artifacts["document"])
                self.assembler.validate_and_render(document_path, rendered)
                if rendered.read_bytes() != artifacts["markdown"]:
                    raise ReuseRecordError(
                        "predecessor Markdown does not match the current renderer"
                    )
        except StructuredSynthesisError as error:
            raise ReuseRecordError(
                f"predecessor document fails current Go validation: {error}"
            ) from error
        identity = ComponentIdentity(**record["identity"])
        compatibility = ProducerCompatibility(**record["compatibility"])
        dependencies = _dependency_from_value(record["dependencies"])
        source_state = SourceRunState(
            _source_snapshot_from_value(record["source_state"]["start"]),
            _source_snapshot_from_value(record["source_state"]["end"]),
        )
        inputs = _input_from_value(record["inputs"])
        snapshot = AcceptedSnapshot(
            snapshot_id=record["snapshot_id"],
            platform=record["version_scope"],
            identity=identity,
            accepted=True,
            route=record["route"],
            inputs=inputs,
            compatibility=compatibility,
            dependencies=dependencies,
            source_state=source_state,
            synthesis_bytes=artifacts["synthesis"],
            synthesis_integrity=record["artifacts"]["synthesis"]["content_hash"],
            response_identity=response_identity,
            document=document,
            document_integrity=record["document_integrity"],
        )
        expected_snapshot_id = content_hash(
            {
                "version_scope": version_scope,
                "component": component,
                "document": record["artifacts"]["document"]["content_hash"],
                "synthesis": record["artifacts"]["synthesis"]["content_hash"],
            }
        )
        if snapshot.snapshot_id != expected_snapshot_id:
            raise ReuseRecordError("predecessor snapshot identity is invalid")
        if content_hash(document) != snapshot.document_integrity:
            raise ReuseRecordError("predecessor document integrity is invalid")
        if (
            content_hash(thaw(snapshot.inputs.semantic_payload))
            != snapshot.inputs.semantic
        ):
            raise ReuseRecordError("predecessor semantic input identity is invalid")
        if (
            thaw(snapshot.inputs.semantic_payload).get("dependencies")
            != snapshot.dependencies.reusable_payload()
        ):
            raise ReuseRecordError("predecessor dependency/input binding is invalid")
        analyzer = bundle["analyzer"]["payload"]
        expected_binding = {
            "component": analyzer.get("component"),
            "repository": analyzer.get("repo"),
            "commit_sha": analyzer.get("commit_sha"),
            "extracted_at": analyzer.get("extracted_at"),
            "analyzer_version": analyzer.get("analyzer_version"),
            "schema_version": analyzer.get("schema_version"),
            "bundle_fingerprint": _analyzer_bundle_fingerprint(analyzer),
        }
        if thaw(snapshot.inputs.analyzer_binding) != expected_binding:
            raise ReuseRecordError("predecessor analyzer/input binding is invalid")
        supplied = {
            item["path"]: (item["whole_file_hash"], item["justification"])
            for item in bundle["source_evidence"]
        }
        dependency_files = {
            item.path: item.content_hash for item in snapshot.dependencies.files
        }
        if (
            set(supplied) != set(snapshot.dependencies.supplied_reads)
            or any(
                dependency_files.get(path) != value[0]
                for path, value in supplied.items()
            )
            or any(
                snapshot.dependencies.justifications.get(path) != value[1]
                for path, value in supplied.items()
            )
        ):
            raise ReuseRecordError("predecessor evidence/dependency binding is invalid")
        binding_errors = _predecessor_document_binding_errors(snapshot)
        if binding_errors:
            raise ReuseRecordError(
                "predecessor snapshot/document binding is invalid: "
                + "; ".join(binding_errors)
            )
        return snapshot

    def save(
        self,
        *,
        version_scope: str,
        component: str,
        identity: ComponentIdentity,
        inputs: InputIdentities,
        compatibility: ProducerCompatibility,
        dependencies: DependencyRecord,
        source_state: SourceRunState,
        response_identity: str,
        rebindings: Sequence[Mapping[str, Any]],
    ) -> Path:
        directory = self.directory(version_scope, component)
        artifact_values: dict[str, dict[str, str]] = {}
        for name, filename in self._ARTIFACTS.items():
            path = directory / filename
            if not path.is_file():
                raise ReuseRecordError(f"completed run lacks {filename}")
            artifact_values[name] = {
                "path": filename,
                "content_hash": file_hash(path),
            }
        synthesis_bytes = (directory / "synthesis.json").read_bytes()
        bundle = json.loads((directory / "evidence-bundle.json").read_text())
        try:
            _, actual_response_identity = _successful_response(synthesis_bytes, bundle)
        except ReuseRecordError:
            synthesis_bundle_path = directory / SYNTHESIS_BUNDLE_FILENAME
            if not synthesis_bundle_path.is_file():
                raise
            synthesis_bundle = json.loads(synthesis_bundle_path.read_text())
            _, actual_response_identity = _successful_response(
                synthesis_bytes, synthesis_bundle
            )
        else:
            synthesis_bundle_path = directory / "evidence-bundle.json"
        artifact_values["synthesis_bundle"] = {
            "path": synthesis_bundle_path.name,
            "content_hash": file_hash(synthesis_bundle_path),
        }
        if response_identity != actual_response_identity:
            raise ReuseRecordError("completed run response identity is inconsistent")
        document = json.loads((directory / "document.json").read_text())
        _validate_schema(DOCUMENT_SCHEMA, document)
        semantic_payload = thaw(inputs.semantic_payload)
        if content_hash(semantic_payload) != inputs.semantic:
            raise ReuseRecordError("completed run semantic identity is inconsistent")
        if semantic_payload.get("key_version") != SEMANTIC_KEY_VERSION:
            raise ReuseRecordError("completed run semantic key version is unsupported")
        if semantic_payload.get("dependencies") != dependencies.reusable_payload():
            raise ReuseRecordError("completed run dependency identity is inconsistent")
        analyzer = bundle["analyzer"]["payload"]
        expected_binding = {
            "component": analyzer.get("component"),
            "repository": analyzer.get("repo"),
            "commit_sha": analyzer.get("commit_sha"),
            "extracted_at": analyzer.get("extracted_at"),
            "analyzer_version": analyzer.get("analyzer_version"),
            "schema_version": analyzer.get("schema_version"),
            "bundle_fingerprint": _analyzer_bundle_fingerprint(analyzer),
        }
        if thaw(inputs.analyzer_binding) != expected_binding:
            raise ReuseRecordError("completed run analyzer identity is inconsistent")
        component_map = (
            document.get("assembly_inputs", {})
            .get("component_map", {})
            .get("value", {})
        )
        map_components = (
            component_map.get("components", {})
            if isinstance(component_map, Mapping)
            else {}
        )
        document_component = (
            map_components.get(identity.component)
            if isinstance(map_components, Mapping)
            else None
        )
        if not isinstance(document_component, Mapping) or (
            _go_component_configuration(document_component)
            != _go_component_configuration(
                semantic_payload.get("component_configuration") or {}
            )
        ):
            raise ReuseRecordError(
                "completed run component configuration is inconsistent"
            )
        probe = AcceptedSnapshot(
            snapshot_id="pending",
            platform=version_scope,
            identity=identity,
            accepted=True,
            route=SYNTHESIS_ROUTE,
            inputs=inputs,
            compatibility=compatibility,
            dependencies=dependencies,
            source_state=source_state,
            synthesis_bytes=synthesis_bytes,
            synthesis_integrity=content_hash(synthesis_bytes),
            response_identity=response_identity,
            document=document,
            document_integrity=content_hash(document),
        )
        binding_errors = _predecessor_document_binding_errors(probe)
        if binding_errors:
            raise ReuseRecordError(
                "completed run document/input binding is inconsistent: "
                + "; ".join(binding_errors)
            )
        snapshot_id = content_hash(
            {
                "version_scope": version_scope,
                "component": component,
                "document": artifact_values["document"]["content_hash"],
                "synthesis": artifact_values["synthesis"]["content_hash"],
            }
        )
        record = {
            "schema_version": SCHEMA_VERSION,
            "record_kind": PRIVATE_RUN_RECORD,
            "record_identity": "",
            "state": "completed",
            "acceptance": "validated-private-assembly",
            "route": SYNTHESIS_ROUTE,
            "snapshot_id": snapshot_id,
            "version_scope": version_scope,
            "component": component,
            "identity": {
                "component": identity.component,
                "repository": identity.repository,
                "aliases": list(identity.aliases),
            },
            "inputs": _input_value(inputs),
            "compatibility": _compatibility_value(compatibility),
            "dependencies": _dependency_value(dependencies),
            "source_state": {
                "start": _source_snapshot_value(source_state.start),
                "end": _source_snapshot_value(source_state.end),
            },
            "response_identity": response_identity,
            "document_integrity": content_hash(document),
            "artifacts": artifact_values,
            "target_rebindings": copy.deepcopy(list(rebindings)),
            "migration": {
                "phase": "P4",
                "obligation": (
                    "Bind to the published four-core snapshot and retain essential "
                    "metadata in core artifacts before publication."
                ),
            },
        }
        record["record_identity"] = content_hash(_record_without_identity(record))
        _validate_schema(RUN_RECORD_SCHEMA, record)
        path = directory / RUN_RECORD_FILENAME
        _atomic_json(path, record)
        return path

def _go_component_configuration(component: Mapping[str, Any]) -> dict[str, Any]:
    """Project the fields the Go component-map decoder actually consumes."""

    return {
        key: copy.deepcopy(component.get(key, ""))
        for key in ("repo_org", "repo_name", "repo_url")
    }

def _identity_from_document(document: Mapping[str, Any]) -> ComponentIdentity:
    identity = document.get("identity")
    if not isinstance(identity, Mapping):
        raise ReuseRecordError("normalized target identity is missing")
    return ComponentIdentity(
        component=str(identity.get("component") or ""),
        repository=str(identity.get("repository") or ""),
        aliases=tuple(identity.get("aliases") or ()),
    )
