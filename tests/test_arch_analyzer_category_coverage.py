import json
from pathlib import Path

import pytest
from jsonschema import Draft202012Validator, ValidationError

PROJECT_ROOT = Path(__file__).resolve().parent.parent
SCHEMA_PATH = (
    PROJECT_ROOT
    / "src"
    / "arch-analyzer"
    / "schema"
    / "component-architecture.schema.json"
)


def schema() -> dict[str, object]:
    return json.loads(SCHEMA_PATH.read_text())


def complete_record() -> dict[str, object]:
    return {
        "status": "complete",
        "fact_count": 0,
        "discovery_contract": "authentication/v1",
        "completed_checks": ["runtime-inventory"],
        "limitations": [],
        "evidence": ["summary:no applicable authentication surfaces"],
    }


def test_schema_remains_compatible_with_legacy_json_without_coverage():
    validator = Draft202012Validator(schema())

    validator.validate({"component": "legacy-component"})


def test_schema_accepts_typed_category_coverage():
    validator = Draft202012Validator(schema())

    validator.validate(
        {
            "component": "covered-component",
            "category_coverage": {"authentication": complete_record()},
        }
    )


def test_schema_accepts_observed_and_unresolved_behavioral_evidence():
    Draft202012Validator(schema()).validate(
        {
            "component": "operator",
            "behavioral_evidence": [
                {
                    "kind": "conditional-metrics-enforcement",
                    "status": "observed",
                    "identity": "controller-runtime metrics",
                    "serving_surface": (
                        "controller-runtime metrics serving surface"
                    ),
                    "configuration_branch": "config.MetricsSecure is true",
                    "enforcement_provider": (
                        "filters.WithAuthenticationAndAuthorization"
                    ),
                    "source": "cmd/main.go:485-500",
                },
                {
                    "kind": "named-watch-predicate",
                    "status": "unresolved",
                    "identity": "internal/controller/auth.ServiceHandler",
                    "watched_gvk": "/v1/Namespace",
                    "source": "internal/controller/auth/controller.go:63-69",
                    "limitations": ["predicate argument is dynamic"],
                },
                {
                    "kind": "named-watch-predicate",
                    "status": "observed",
                    "identity": "internal/controller/auth.ServiceHandler",
                    "watched_gvk": "/v1/Namespace",
                    "literal_values": ["models-as-a-service"],
                    "event_target": (
                        "services.platform.opendatahub.io/v1alpha1/Auth/auth"
                    ),
                    "source": "internal/controller/auth/controller.go:63-69",
                },
            ],
        }
    )


@pytest.mark.parametrize(
    "mutation",
    [
        lambda record: record.update(status="assumed"),
        lambda record: record.update(source="cmd/main.go:492"),
        lambda record: record.pop("configuration_branch"),
        lambda record: record.update(kind="method-name-guess"),
        lambda record: record.update(serving_surface=""),
        lambda record: record.update(configuration_branch=""),
        lambda record: record.update(enforcement_provider=""),
    ],
)
def test_schema_rejects_unsupported_behavioral_evidence(mutation):
    record = {
        "kind": "conditional-metrics-enforcement",
        "status": "observed",
        "identity": "controller-runtime metrics",
        "serving_surface": "controller-runtime metrics serving surface",
        "configuration_branch": "config.MetricsSecure is true",
        "enforcement_provider": "filters.WithAuthenticationAndAuthorization",
        "source": "cmd/main.go:485-500",
    }
    mutation(record)

    with pytest.raises(ValidationError):
        Draft202012Validator(schema()).validate(
            {"component": "invalid-component", "behavioral_evidence": [record]}
        )


@pytest.mark.parametrize(
    "mutation",
    [
        lambda record: record.update(identity=""),
        lambda record: record.update(watched_gvk=""),
        lambda record: record.update(literal_values=[""]),
        lambda record: record.update(event_target=""),
        lambda record: record.pop("event_target"),
    ],
)
def test_schema_rejects_empty_observed_watch_proof(mutation):
    record = {
        "kind": "named-watch-predicate",
        "status": "observed",
        "identity": "internal/controller/auth.ServiceHandler",
        "watched_gvk": "/v1/Namespace",
        "literal_values": ["models-as-a-service"],
        "event_target": "services.platform.opendatahub.io/v1alpha1/Auth/auth",
        "source": "internal/controller/auth/controller.go:63-69",
    }
    mutation(record)

    with pytest.raises(ValidationError):
        Draft202012Validator(schema()).validate(
            {"component": "invalid-component", "behavioral_evidence": [record]}
        )


@pytest.mark.parametrize(
    "mutation",
    [
        lambda record: record.pop("limitations"),
        lambda record: record.update(status="assumed"),
        lambda record: record.update(fact_count=-1),
        lambda record: record.update(evidence=[""]),
    ],
)
def test_schema_rejects_malformed_category_coverage(mutation):
    record = complete_record()
    mutation(record)
    validator = Draft202012Validator(schema())

    with pytest.raises(ValidationError):
        validator.validate(
            {
                "component": "invalid-component",
                "category_coverage": {"authentication": record},
            }
        )
