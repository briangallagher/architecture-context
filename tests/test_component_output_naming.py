import json

from lib.manifest_parser import ComponentInfo
from lib.phases.architecture import (
    _remove_legacy_component_outputs,
)
from lib.phases.discover import _apply_map_overrides


def test_legacy_component_outputs_are_removed_for_prefixed_alias(tmp_path):
    platform_dir = tmp_path / "architecture" / "rhoai-3.6-ea.2"
    diagrams_dir = platform_dir / "diagrams"
    diagrams_dir.mkdir(parents=True)
    (platform_dir / "policy.md").write_text("old")
    (platform_dir / "policy" / ".analyzer").mkdir(parents=True)
    (platform_dir / "policy" / ".generation").mkdir(parents=True)
    (diagrams_dir / "policy-component.mmd").write_text("old")

    component = ComponentInfo(
        key="praxis-policy",
        repo_org="praxis-proxy",
        repo_name="policy",
        ref=None,
        source_folder=None,
    )
    _remove_legacy_component_outputs(
        tmp_path / "architecture",
        "rhoai-3.6-ea.2",
        {
            "extra_repos": [
                {
                    "org": "praxis-proxy",
                    "repo": "policy",
                    "name_prefix": "praxis-",
                },
            ],
        },
        {"praxis-policy": component},
    )

    assert not (platform_dir / "policy.md").exists()
    assert not (platform_dir / "policy").exists()
    assert not (diagrams_dir / "policy-component.mmd").exists()


def test_discovery_map_uses_alias_but_preserves_repo_identity(tmp_path):
    map_file = tmp_path / "component-map.json"
    map_file.write_text(json.dumps({
        "components": {
            "policy": {
                "key": "policy",
                "repo_org": "praxis-proxy",
                "repo_name": "policy",
            },
        },
        "excluded": {"grid": "praxis_proxy_not_in_catalog"},
        "dependency_graph": {"grid": ["policy"]},
        "metadata": {},
    }))
    config = {
        "extra_repos": [
            {
                "org": "praxis-proxy",
                "repo": "policy",
                "name_prefix": "praxis-",
            },
            {
                "org": "praxis-proxy",
                "repo": "grid",
                "name_prefix": "praxis-",
            },
        ],
        "include_components": [
            {
                "key": "praxis-grid",
                "repo_org": "praxis-proxy",
                "repo_name": "grid",
                "type": "operator",
            },
        ],
    }

    _apply_map_overrides(map_file, config)
    data = json.loads(map_file.read_text())

    assert "praxis-policy" in data["components"]
    assert data["components"]["praxis-policy"]["repo_name"] == "policy"
    assert "praxis-grid" in data["components"]
    assert data["dependency_graph"] == {"praxis-grid": ["praxis-policy"]}
