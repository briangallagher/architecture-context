import importlib.util
import json
from pathlib import Path

import pytest

PROJECT_ROOT = Path(__file__).resolve().parent.parent


def _load_module(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


PLATFORMS = _load_module(
    "lint_platforms_for_index",
    PROJECT_ROOT / "scripts/lint_platforms.py",
)
OVERLAYS = _load_module(
    "lint_overlays_for_index",
    PROJECT_ROOT / "scripts/lint_overlays.py",
)
COMPONENT_MAP = _load_module(
    "validate_component_map_for_index",
    PROJECT_ROOT
    / ".claude/skills/discover-components/scripts/validate_component_map.py",
)


def _platform_config() -> dict:
    return {
        "integration_status": {"praxis-policy": "not-integrated"},
        "include_components": [
            {
                "key": "planned-service",
                "repo_org": "example",
                "repo_name": "service",
                "type": "service",
                "integration_status": "planned",
            }
        ],
        "component_overrides": {
            "current-service": {"integration_status": "current"}
        },
    }


def test_platform_validator_accepts_all_structured_index_status_locations():
    assert PLATFORMS.validate_platform("rhoai-test", _platform_config()) == []


def test_platform_validator_rejects_invalid_index_status():
    config = _platform_config()
    config["component_overrides"]["current-service"]["integration_status"] = (
        "eventually"
    )

    errors = PLATFORMS.validate_platform("rhoai-test", config)

    assert any(
        "component_overrides.current-service.integration_status" in error
        for error in errors
    )


def test_platform_validator_accepts_explicit_reuse_predecessor():
    config = _platform_config()
    config["reuse_from"] = "rhoai-previous"
    assert PLATFORMS.validate_platform("rhoai-current", config) == []
    assert (
        PLATFORMS.validate_reuse_graph({"rhoai-previous": {}, "rhoai-current": config})
        == []
    )


def test_platform_validator_rejects_missing_and_cyclic_reuse_predecessors():
    missing = PLATFORMS.validate_reuse_graph(
        {"rhoai-current": {"reuse_from": "rhoai-missing"}}
    )
    assert any("missing platform" in error for error in missing)

    cyclic = PLATFORMS.validate_reuse_graph(
        {
            "rhoai-previous": {"reuse_from": "rhoai-current"},
            "rhoai-current": {"reuse_from": "rhoai-previous"},
        }
    )
    assert any("cycle detected" in error for error in cyclic)

    invalid = PLATFORMS.validate_platform("rhoai-current", {"reuse_from": " "})
    assert any("non-empty" in error for error in invalid)


def _overlay(path: Path, integration_status: str) -> None:
    path.write_text(
        "---\n"
        "id: '0001'\n"
        "title: Planned service\n"
        "status: active\n"
        "created: 2026-09-05\n"
        "affects: [planned-service]\n"
        "release: ['3.6']\n"
        "provenance: [https://example.test/change]\n"
        "author: Test\n"
        f"integration_status: {integration_status}\n"
        "---\n\n"
        "## Fact\n\nFact.\n\n"
        "## Impact on Strategies\n\nImpact.\n\n"
        "## Context\n\nContext.\n"
    )


def test_overlay_validator_accepts_structured_index_status(tmp_path: Path):
    path = tmp_path / "0001-planned-service.md"
    _overlay(path, "planned")

    assert OVERLAYS.validate_overlay(path) == []


def test_overlay_validator_rejects_invalid_index_status(tmp_path: Path):
    path = tmp_path / "0001-planned-service.md"
    _overlay(path, "eventually")

    errors = OVERLAYS.validate_overlay(path)

    assert any("integration_status" in error for error in errors)


@pytest.mark.parametrize("integration_status", ["planned", "eventually", []])
def test_component_map_validator_checks_optional_index_status(
    tmp_path: Path, integration_status: object
):
    path = tmp_path / "component-map.json"
    component = {
        "key": "service",
        "repo_org": "example",
        "repo_name": "service",
        "repo_url": "https://example.test/example/service",
        "checkout_path": "checkouts/example/service",
        "has_architecture": False,
        "type": "service",
        "shipped": False,
        "architecturally_significant": True,
        "integration_status": integration_status,
    }
    path.write_text(
        json.dumps(
            {
                "metadata": {
                    "platform": "rhoai-test",
                    "discovery_method": "breadcrumb",
                    "discovered_at": "2026-09-05T00:00:00Z",
                    "total_repos_scanned": 1,
                    "components_discovered": 1,
                    "components_excluded": 0,
                },
                "components": {"service": component},
                "excluded": {},
            }
        )
    )

    errors = COMPONENT_MAP.validate(str(path))

    assert any("components.service.integration_status" in error for error in errors)
