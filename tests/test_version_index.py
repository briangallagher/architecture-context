"""Tests for deterministic version architecture indexes."""

from __future__ import annotations

import json
import sys
from pathlib import Path

import pytest

PROJECT_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(PROJECT_ROOT))

from lib.version_index import (  # noqa: E402
    VersionIndexError,
    build_index_model,
    generate_version_index,
    load_active_overlays,
    render_version_index,
)


def _write_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n")


def _write_overlay(
    path: Path,
    *,
    status: str = "active",
    releases: tuple[str, ...] = ("3.6",),
    affects: tuple[str, ...] = ("pending",),
    integration_status: str | None = None,
) -> None:
    status_line = (
        f"integration_status: {integration_status}\n" if integration_status else ""
    )
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        "---\n"
        f'id: "{path.name[:4]}"\n'
        f"title: Context for {path.stem}\n"
        f"status: {status}\n"
        "created: 2026-09-05\n"
        "affects:\n"
        + "".join(f"  - {value}\n" for value in affects)
        + "release:\n"
        + "".join(f'  - "{value}"\n' for value in releases)
        + status_line
        + "provenance:\n"
        + "  - https://example.test/evidence\n"
        + "author: Test\n"
        + "superseded_by: null\n"
        + "---\n\n## Fact\n\nFixture.\n"
    )


def _fixture(tmp_path: Path) -> tuple[Path, Path, Path]:
    architecture = tmp_path / "architecture"
    platform = architecture / "rhoai-3.6-ea.2"
    platform.mkdir(parents=True)
    _write_json(
        platform / "component-map.json",
        {
            "metadata": {"platform": platform.name},
            "components": {
                "praxis-policy": {
                    "key": "praxis-policy",
                    "repo_org": "praxis-proxy",
                    "repo_name": "policy",
                    "repo_url": "https://example.test/praxis-proxy/policy",
                    "type": "operator",
                    "shipped": True,
                },
                "pending": {
                    "key": "pending",
                    "repo_name": "pending",
                    "type": "service",
                },
                "service [one] #1": {
                    "key": "service [one] #1",
                    "repo_name": "service-one",
                    "type": "service|api",
                    "description": "A mapped | description.",
                    "shipped": False,
                },
                "INDEX": {"key": "INDEX", "type": "metadata"},
            },
        },
    )
    (platform / "PLATFORM.md").write_text("# Platform\n")
    (platform / "praxis-policy.md").write_text(
        "# Praxis Policy\n\n"
        "## Purpose\n\n"
        "Policy utilities from the stored document.\n\n"
        "## Security\n\nStored details.\n\n"
        "## Training Lifecycle\n\nStored details.\n"
    )
    (platform / "service [one] #1.md").write_text(
        "# Service\n\n## Purpose\n\nService purpose.\n\n"
        "## Model Serving\n\nStored details.\n\n"
        "```md\n## Authentication in example\n```\n"
    )
    _write_json(
        platform
        / "praxis-policy/.analyzer/component-architecture.json",
        {
            "component": "praxis-policy",
            "repo": "https://example.test/praxis-proxy/policy.git",
            "data_coverage": {"source": "partial"},
        },
    )
    _write_json(
        platform / "service [one] #1/.generation/SURFACE_COVERAGE.json",
        {"schema_version": "architecture-surface-coverage/v1"},
    )
    platforms = tmp_path / "platforms.yaml"
    platforms.write_text(
        "rhoai-3.6-ea.2:\n"
        "  extra_repos:\n"
        "    - org: praxis-proxy\n"
        "      repo: policy\n"
        "  integration_status:\n"
        '    "service [one] #1": current\n'
    )
    overlays = tmp_path / "overlays"
    _write_overlay(
        overlays / "0001-pending-planned.md",
        integration_status="planned",
    )
    _write_overlay(
        overlays / "0002-praxis-context.md",
        affects=("praxis-policy",),
    )
    _write_overlay(
        overlays / "0003-future.md",
        releases=("3.7",),
        affects=("praxis-policy",),
        integration_status="current",
    )
    _write_overlay(
        overlays / "0004-superseded.md",
        status="superseded",
        affects=("praxis-policy",),
        integration_status="current",
    )
    return platform, platforms, overlays


def test_model_separates_inventory_documentation_and_integration(
    tmp_path: Path,
) -> None:
    platform, platforms, overlays_dir = _fixture(tmp_path)
    overlays = load_active_overlays(overlays_dir, platform.name)
    model = build_index_model(
        platform,
        platform_config={
            "extra_repos": [{"org": "praxis-proxy", "repo": "policy"}],
            "integration_status": {"service [one] #1": "current"},
        },
        overlays=overlays,
        platforms_file=platforms,
    )
    by_alias = {item["alias"]: item for item in model["components"]}

    assert sorted(by_alias) == ["pending", "praxis-policy", "service [one] #1"]
    assert by_alias["praxis-policy"]["inventory_signal"] == "shipped=true"
    assert by_alias["praxis-policy"]["integration"] == "unknown"
    assert by_alias["praxis-policy"]["document_status"] == "available"
    assert by_alias["praxis-policy"]["description"] == (
        "Policy utilities from the stored document."
    )
    assert by_alias["pending"]["integration"] == "planned"
    assert by_alias["pending"]["integration_source"] == "active overlay"
    assert by_alias["pending"]["document_status"] == "pending"
    assert by_alias["service [one] #1"]["integration"] == "current"
    assert by_alias["service [one] #1"]["inventory_signal"] == "shipped=false"


def test_render_escapes_tables_encodes_links_and_uses_real_headings(
    tmp_path: Path,
) -> None:
    platform, platforms, overlays = _fixture(tmp_path)
    result = generate_version_index(
        platform, platforms_file=platforms, overlays_dir=overlays
    )
    text = result.path.read_text()

    assert "praxis-policy" in text
    assert "A mapped \\| description." in text
    assert "service%20%5Bone%5D%20%231.md" in text
    assert "service%20%5Bone%5D%20%231.md#model-serving" in text
    assert "praxis-policy.md#security" in text
    assert "authentication-in-example" not in text
    assert "[PLATFORM.md](PLATFORM.md)" in text
    assert "pending" in text
    assert "pending.md" not in text
    assert "coverage telemetry unavailable" in text
    assert "analyzer limitations" in text
    assert "Topic entries are navigation hints" in text


def test_generate_is_byte_stable_and_does_not_rewrite_unchanged_index(
    tmp_path: Path,
) -> None:
    platform, platforms, overlays = _fixture(tmp_path)

    first = generate_version_index(
        platform, platforms_file=platforms, overlays_dir=overlays
    )
    first_bytes = first.path.read_bytes()
    first_inode = first.path.stat().st_ino
    second = generate_version_index(
        platform, platforms_file=platforms, overlays_dir=overlays
    )

    assert first.changed is True
    assert second.changed is False
    assert second.path.read_bytes() == first_bytes
    assert second.path.stat().st_ino == first_inode


def test_malformed_required_input_preserves_existing_index(tmp_path: Path) -> None:
    platform, platforms, overlays = _fixture(tmp_path)
    output = platform / "INDEX.md"
    output.write_text("previous valid index\n")
    (platform / "component-map.json").write_text("[]\n")

    with pytest.raises(VersionIndexError, match="root must be an object"):
        generate_version_index(
            platform, platforms_file=platforms, overlays_dir=overlays
        )

    assert output.read_text() == "previous valid index\n"


def test_malformed_platform_status_preserves_existing_index(tmp_path: Path) -> None:
    platform, platforms, overlays = _fixture(tmp_path)
    output = platform / "INDEX.md"
    output.write_text("previous valid index\n")
    platforms.write_text(
        f"{platform.name}:\n"
        "  integration_status:\n"
        "    unrelated: eventually\n"
    )

    with pytest.raises(VersionIndexError, match="invalid integration_status"):
        generate_version_index(
            platform,
            platforms_file=platforms,
            overlays_dir=overlays,
        )

    assert output.read_text() == "previous valid index\n"


def test_missing_map_is_actionable_and_legacy_config_is_optional(
    tmp_path: Path,
) -> None:
    missing = tmp_path / "architecture/rhoai-2.10"
    missing.mkdir(parents=True)
    with pytest.raises(VersionIndexError, match="required component map"):
        generate_version_index(missing)

    _write_json(
        missing / "component-map.json",
        {"components": [{"key": "legacy", "type": "service"}]},
    )
    (missing / "legacy.md").write_text("# Legacy\n\n## Purpose\n\nOld.\n")
    result = generate_version_index(
        missing,
        platforms_file=tmp_path / "absent-platforms.yaml",
        overlays_dir=tmp_path / "absent-overlays",
    )
    assert result.component_count == 1
    assert "legacy.md" in result.path.read_text()


def test_analyzer_mismatch_and_invalid_sidecar_remain_visible(tmp_path: Path) -> None:
    platform, platforms, overlays = _fixture(tmp_path)
    analyzer = platform / "praxis-policy/.analyzer/component-architecture.json"
    _write_json(analyzer, {"component": "other", "repo": "https://wrong.test/repo"})
    sidecar = platform / "praxis-policy/.generation/SURFACE_COVERAGE.json"
    sidecar.parent.mkdir(parents=True)
    sidecar.write_text("not json\n")

    generate_version_index(
        platform, platforms_file=platforms, overlays_dir=overlays
    )
    text = (platform / "INDEX.md").read_text()

    assert "mismatch:component,repository" in text
    assert "coverage sidecar invalid" in text


def test_structured_overlay_status_is_release_scoped(tmp_path: Path) -> None:
    platform, _platforms, overlays_dir = _fixture(tmp_path)

    overlays = load_active_overlays(overlays_dir, platform.name)

    assert [item["path"].name for item in overlays] == [
        "0001-pending-planned.md",
        "0002-praxis-context.md",
    ]
    assert overlays[0]["integration_status"] == "planned"


def test_unsafe_alias_does_not_replace_existing_index(tmp_path: Path) -> None:
    platform = tmp_path / "architecture/rhoai-test"
    platform.mkdir(parents=True)
    output = platform / "INDEX.md"
    output.write_text("keep\n")
    _write_json(
        platform / "component-map.json",
        {"components": {"../escape": {"type": "service"}}},
    )

    with pytest.raises(VersionIndexError, match="unsafe component alias"):
        generate_version_index(
            platform,
            platforms_file=tmp_path / "missing.yaml",
            overlays_dir=tmp_path / "missing-overlays",
        )

    assert output.read_text() == "keep\n"


def test_conflicting_structured_overlays_fail_closed_to_unknown(tmp_path: Path) -> None:
    platform, platforms, overlays = _fixture(tmp_path)
    _write_overlay(
        overlays / "0005-pending-current.md",
        affects=("pending",),
        integration_status="current",
    )

    generate_version_index(
        platform, platforms_file=platforms, overlays_dir=overlays
    )
    text = (platform / "INDEX.md").read_text()

    assert "unknown (conflicting active overlays)" in text


def test_platform_overlay_applies_to_all_components(tmp_path: Path) -> None:
    platform, platforms, overlays = _fixture(tmp_path)
    _write_overlay(
        overlays / "0005-platform-planned.md",
        affects=("platform",),
        integration_status="planned",
    )

    model = build_index_model(
        platform,
        platform_config={},
        overlays=load_active_overlays(overlays, platform.name),
        platforms_file=platforms,
    )

    assert {item["integration"] for item in model["components"]} == {"planned"}


def test_component_map_cannot_make_future_integration_planned(
    tmp_path: Path,
) -> None:
    platform, platforms, overlays = _fixture(tmp_path)
    payload = json.loads((platform / "component-map.json").read_text())
    payload["components"]["praxis-policy"]["integration_status"] = "planned"
    _write_json(platform / "component-map.json", payload)

    with pytest.raises(VersionIndexError, match="must come from platforms.yaml"):
        generate_version_index(
            platform,
            platforms_file=platforms,
            overlays_dir=overlays,
        )


def test_render_function_is_stable_for_same_model(tmp_path: Path) -> None:
    platform, platforms, overlays_dir = _fixture(tmp_path)
    overlays = load_active_overlays(overlays_dir, platform.name)
    model = build_index_model(
        platform,
        platform_config={"integration_status": {"pending": "planned"}},
        overlays=overlays,
        platforms_file=platforms,
    )

    assert render_version_index(model) == render_version_index(model)
