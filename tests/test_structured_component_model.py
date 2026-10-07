import json
import os
import subprocess
from pathlib import Path

from jsonschema import Draft202012Validator
from referencing import Registry, Resource

ROOT = Path(__file__).resolve().parents[1]
SCHEMA_DIR = ROOT / "schemas"
FIXTURE_DIR = ROOT / "tests" / "fixtures" / "structured_component"


def _schemas():
    paths = sorted(SCHEMA_DIR.glob("structured-*.schema.json"))
    schemas = {path.name: json.loads(path.read_text()) for path in paths}
    registry = Registry().with_resources(
        [
            (schema["$id"], Resource.from_contents(schema))
            for schema in schemas.values()
        ]
    )
    return schemas, registry


def test_structured_schemas_and_cli_document_are_valid(tmp_path):
    schemas, registry = _schemas()
    for schema in schemas.values():
        Draft202012Validator.check_schema(schema)

    output = tmp_path / "document.json"
    env = os.environ.copy()
    env.setdefault("GOCACHE", "/tmp/structured-component-go-cache")
    subprocess.run(
        [
            "go",
            "run",
            ".",
            "normalize",
            "--input",
            str(FIXTURE_DIR / "analyzer-rbac-praxis.json"),
            "--component-map",
            str(FIXTURE_DIR / "component-map-praxis.json"),
            "--component-map-id",
            "rhoai-component-map",
            "--sections",
            str(FIXTURE_DIR / "sections-all-conditional.json"),
            "--section-origin-kind",
            "model",
            "--section-origin-id",
            "fixture-response",
            "--section-claim-class",
            "implementation",
            "--version-scope",
            "rhoai-3.6-ea.2",
            "--output",
            str(output),
        ],
        cwd=ROOT / "src" / "arch-analyzer",
        env=env,
        check=True,
        capture_output=True,
        text=True,
    )
    document = json.loads(output.read_text())
    validator = Draft202012Validator(
        schemas["structured-component-document-v1.schema.json"],
        registry=registry,
    )
    validator.validate(document)
    assert document["assembly_inputs"]["component_map"]["origin"] == {
        "kind": "orchestrator",
        "id": "rhoai-component-map",
    }
    assert all(
        section["authority"]
        == {
            "kind": "model",
            "origin": "fixture-response",
            "claim_class": "implementation",
        }
        for section in document["sections"]
    )
    table_ids = {
        block["table_id"]
        for section in document["sections"]
        for block in section["blocks"]
        if block["type"] == "table"
    }
    assert {
        "security.fips-build-time",
        "security.build-hermeticity",
        "multi-tenancy.tenant-model",
    }.issubset(table_ids)

    invalid = json.loads(json.dumps(document))
    fips = next(
        section
        for section in invalid["sections"]
        if section["id"] == "security.fips-compliance"
    )
    build_table = next(
        block
        for block in fips["blocks"]
        if block.get("table_id") == "security.fips-build-time"
    )
    build_table["rows"][0]["cells"].pop()
    assert list(validator.iter_errors(invalid))

    empty_table = json.loads(json.dumps(document))
    fips = next(
        section
        for section in empty_table["sections"]
        if section["id"] == "security.fips-compliance"
    )
    build_table = next(
        block
        for block in fips["blocks"]
        if block.get("table_id") == "security.fips-build-time"
    )
    build_table["rows"] = []
    assert list(validator.iter_errors(empty_table))

    def inject_evidence(candidate, target, field):
        if target == "section":
            reference = candidate["sections"][0]["evidence"][0]
        elif target == "table":
            reference = next(
                block
                for section in candidate["sections"]
                for block in section["blocks"]
                if block["type"] == "table"
            )["rows"][0]["evidence"][0]
        else:
            reference = next(
                fact for fact in candidate["facts"] if fact["evidence"]
            )["evidence"][0]
        reference[field] = (
            "source.go\n```" if field == "path" else "revision\n```"
        )

    for target in ("section", "table", "fact"):
        for field in ("path", "revision"):
            injected = json.loads(json.dumps(document))
            inject_evidence(injected, target, field)
            assert list(validator.iter_errors(injected)), (target, field)

    inconsistent = json.loads(json.dumps(document))
    inconsistent["identity"]["integration_status"] = "current"
    assert list(validator.iter_errors(inconsistent))

    empty_provenance_map = tmp_path / "component-map-empty-provenance.json"
    component_map = json.loads(
        (FIXTURE_DIR / "component-map-praxis.json").read_text()
    )
    component_map["provenance"] = {}
    empty_provenance_map.write_text(json.dumps(component_map))
    projection_output = tmp_path / "empty-provenance-document.json"
    subprocess.run(
        [
            "go",
            "run",
            ".",
            "normalize",
            "--input",
            str(FIXTURE_DIR / "analyzer-rbac-praxis.json"),
            "--component-map",
            str(empty_provenance_map),
            "--output",
            str(projection_output),
        ],
        cwd=ROOT / "src" / "arch-analyzer",
        env=env,
        check=True,
        capture_output=True,
        text=True,
    )
    projection = json.loads(projection_output.read_text())
    validator.validate(projection)
    assert projection["assembly_inputs"]["component_map"]["value"][
        "provenance"
    ] == {"repos": {}}


def test_untrusted_patch_schema_cannot_carry_authority_or_decisions():
    schemas, registry = _schemas()
    validator = Draft202012Validator(
        schemas["structured-component-patch-v1.schema.json"],
        registry=registry,
    )
    proposal = {
        "schema_version": "1.0.0",
        "patch_id": "model-proposal",
        "bundle_fingerprint": "sha256:" + "a" * 64,
        "operations": [],
    }
    validator.validate(proposal)

    spoofed = {
        **proposal,
        "origin": {
            "kind": "model",
            "id": "self",
            "claim_class": "implementation",
        },
        "authority": {"actor": "model", "allowed_fact_types": ["service"]},
    }
    errors = list(validator.iter_errors(spoofed))
    assert errors
    assert "Additional properties" in errors[0].message

    proposal["operations"] = [
        {
            "operation_id": "add-service",
            "action": "add",
            "fact_type": "service",
            "value": {
                "name": "api",
                "source": "config/service.yaml",
                "type": "ClusterIP",
                "ports": [],
                "target_deployment": "api",
            },
            "evidence": [
                {"path": "config/service.yaml", "revision": "abcdef"}
            ],
            "reason": "Evidence-backed proposal.",
        }
    ]
    validator.validate(proposal)
    for field, injected in (
        ("path", "config/service.yaml\n```"),
        ("revision", "abcdef\n```"),
    ):
        invalid_evidence = json.loads(json.dumps(proposal))
        invalid_evidence["operations"][0]["evidence"][0][field] = injected
        assert list(validator.iter_errors(invalid_evidence)), field


def test_trusted_policy_schema_requires_explicit_human_decision_provenance():
    schemas, registry = _schemas()
    validator = Draft202012Validator(
        schemas["structured-component-assembly-policy-v1.schema.json"],
        registry=registry,
    )
    policy = {
        "schema_version": "1.0.0",
        "policy_id": "review-policy",
        "patch_id": "model-proposal",
        "proposal_fingerprint": "sha256:" + "b" * 64,
        "bundle_fingerprint": "sha256:" + "a" * 64,
        "version_scope": "rhoai-3.6-ea.2",
        "origin": {
            "kind": "model",
            "id": "response-01",
            "claim_class": "correction",
        },
        "authority": {
            "actor": "orchestrator",
            "allowed_fact_types": ["service"],
        },
        "decisions": [
            {
                "decision_id": "human-decision-01",
                "operation_id": "update-service",
                "decision": "accept",
                "decided_by": "reviewer@example",
                "reason": "Explicit evidence-backed approval.",
            }
        ],
    }
    validator.validate(policy)
    del policy["decisions"][0]["decided_by"]
    assert list(validator.iter_errors(policy))
