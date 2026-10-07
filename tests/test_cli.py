import sys
from pathlib import Path

PROJECT_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(PROJECT_ROOT))

from lib.cli import parse_args, resolve_script_path  # noqa: E402
from lib.manifest_parser import (  # noqa: E402
    parse_manifests_config,
    process_manifest_script,
)


def test_generate_architecture_defaults_to_evidence_gated_merge(monkeypatch):
    monkeypatch.setattr(
        sys,
        "argv",
        ["main.py", "generate-architecture", "--platform", "rhoai.next"],
    )

    args = parse_args()

    assert args.evidence_gated_merge is True
    assert args.structured_synthesis is False
    assert args.structured_inputs is None
    assert args.structured_total_calls == 3
    assert args.structured_evidence_followups == 1
    assert args.structured_repairs == 1
    assert args.structured_refresh is False


def test_generate_architecture_accepts_bounded_structured_opt_in(monkeypatch):
    monkeypatch.setattr(
        sys,
        "argv",
        [
            "main.py",
            "generate-architecture",
            "--platform",
            "rhoai.next",
            "--structured-synthesis",
            "--structured-inputs",
            "parent.json",
            "--structured-total-calls",
            "5",
            "--structured-evidence-followups",
            "2",
            "--structured-repairs",
            "2",
            "--structured-refresh",
        ],
    )

    args = parse_args()

    assert args.structured_synthesis is True
    assert args.structured_inputs == "parent.json"
    assert args.structured_total_calls == 5
    assert args.structured_evidence_followups == 2
    assert args.structured_repairs == 2
    assert args.structured_refresh is True


def test_generate_architecture_allows_legacy_merge_opt_out(monkeypatch):
    monkeypatch.setattr(
        sys,
        "argv",
        [
            "main.py",
            "generate-architecture",
            "--platform",
            "rhoai.next",
            "--no-evidence-gated-merge",
        ],
    )

    args = parse_args()

    assert args.evidence_gated_merge is False


def test_all_defaults_to_evidence_gated_merge(monkeypatch):
    monkeypatch.setattr(
        sys,
        "argv",
        ["main.py", "all", "--platform", "rhoai.next"],
    )

    args = parse_args()

    assert args.evidence_gated_merge is True


def test_all_allows_legacy_merge_opt_out(monkeypatch):
    monkeypatch.setattr(
        sys,
        "argv",
        [
            "main.py",
            "all",
            "--platform",
            "rhoai.next",
            "--no-evidence-gated-merge",
        ],
    )

    args = parse_args()

    assert args.evidence_gated_merge is False


def test_pipeline_accepts_repeated_phases_components_and_repos(monkeypatch):
    monkeypatch.setattr(
        sys,
        "argv",
        [
            "main.py",
            "pipeline",
            "--platform",
            "rhoai.next",
            "--phase",
            "static-analysis",
            "--phase",
            "generate-architecture",
            "--component",
            "models-as-a-service",
            "--component",
            "eval-hub",
            "--repo",
            "red-hat-data-services/llm-d-inference-scheduler",
            "--max-concurrent",
            "2",
            "--force",
        ],
    )

    args = parse_args()

    assert args.command == "pipeline"
    assert args.phase == ["static-analysis", "generate-architecture"]
    assert args.component == ["models-as-a-service", "eval-hub"]
    assert args.repo == ["red-hat-data-services/llm-d-inference-scheduler"]
    assert args.max_concurrent == 2
    assert args.force is True
    assert args.evidence_gated_merge is True


def test_generate_index_has_offline_local_defaults(monkeypatch):
    monkeypatch.setattr(
        sys,
        "argv",
        ["main.py", "generate-index", "--platform", "rhoai.next"],
    )

    args = parse_args()

    assert args.command == "generate-index"
    assert args.architecture_dir == "architecture"
    assert args.platforms_file == "platforms.yaml"
    assert args.overlays_dir == "overlays"
    assert not hasattr(args, "harness")


def test_pipeline_accepts_generate_index_as_explicit_phase(monkeypatch):
    monkeypatch.setattr(
        sys,
        "argv",
        [
            "main.py",
            "pipeline",
            "--platform",
            "rhoai.next",
            "--phase",
            "generate-architecture",
            "--phase",
            "generate-index",
            "--component",
            "example",
        ],
    )

    args = parse_args()

    assert args.phase == ["generate-architecture", "generate-index"]


def test_pipeline_accepts_codex_harness_and_codex_model(monkeypatch):
    monkeypatch.setattr(
        sys,
        "argv",
        [
            "main.py",
            "pipeline",
            "--platform",
            "rhoai-3.6-ea.2",
            "--phase",
            "fetch",
            "--phase",
            "discover-components",
            "--harness",
            "codex",
            "--model",
            "gpt-5.3-codex",
        ],
    )

    args = parse_args()

    assert args.harness == "codex"
    assert args.model == "gpt-5.3-codex"


def test_codex_harness_uses_configured_model_by_default(monkeypatch):
    monkeypatch.setattr(
        sys,
        "argv",
        [
            "main.py",
            "discover-components",
            "--platform",
            "rhoai-3.6-ea.2",
            "--harness",
            "codex",
        ],
    )

    args = parse_args()

    assert args.harness == "codex"
    assert args.model is None


def test_generate_architecture_accepts_claude_run_limits(monkeypatch):
    monkeypatch.setattr(
        sys,
        "argv",
        [
            "main.py",
            "generate-architecture",
            "--platform",
            "rhoai-3.6-ea.2",
            "--harness",
            "claude",
            "--model",
            "claude-opus-4-6",
            "--max-agent-turns",
            "50",
            "--max-budget-usd",
            "20",
        ],
    )

    args = parse_args()

    assert args.model == "claude-opus-4-6"
    assert args.max_agent_turns == 50
    assert args.max_budget_usd == 20.0


def test_pipeline_allows_evidence_gated_merge_opt_out(monkeypatch):
    monkeypatch.setattr(
        sys,
        "argv",
        [
            "main.py",
            "pipeline",
            "--platform",
            "rhoai.next",
            "--phase",
            "generate-architecture",
            "--no-evidence-gated-merge",
        ],
    )

    args = parse_args()

    assert args.evidence_gated_merge is False


def test_parse_manifests_config_extracts_components(tmp_path: Path):
    config = tmp_path / "manifests-config.yaml"
    config.write_text("""\
components:
  kserve:
    odh:
      repo: opendatahub-io/kserve
      ref: release-v0.17@abc123
      sourcePath: config
    rhoai:
      repo: red-hat-data-services/kserve
      ref: rhoai-3.5@def456
      sourcePath: kserve-module/config
ccmCharts:
  cert-manager-operator:
    rhoai:
      repo: red-hat-data-services/odh-gitops
      ref: rhoai-3.5@aaa111
      sourcePath: charts/dependencies/cert-manager-operator
""")
    components = parse_manifests_config(config, "rhoai")
    assert "kserve" in components
    assert components["kserve"].repo_org == "red-hat-data-services"
    assert components["kserve"].repo_name == "kserve"
    assert components["kserve"].ref == "rhoai-3.5@def456"
    assert components["kserve"].source_folder == "kserve-module/config"
    assert "cert-manager-operator" in components
    assert len(components) == 2


def test_parse_manifests_config_filters_by_platform(tmp_path: Path):
    config = tmp_path / "manifests-config.yaml"
    config.write_text("""\
components:
  kserve:
    odh:
      repo: opendatahub-io/kserve
      ref: main@abc123
      sourcePath: config
""")
    components = parse_manifests_config(config, "rhoai")
    assert len(components) == 0


def test_process_manifest_script_dispatches_to_yaml(tmp_path: Path):
    config = tmp_path / "manifests-config.yaml"
    checkout = tmp_path / "kserve"
    checkout.mkdir()
    config.write_text("""\
components:
  kserve:
    rhoai:
      repo: test-org/kserve
      ref: main@abc123
      sourcePath: config
""")
    components = process_manifest_script(
        str(config),
        platform="rhoai",
        checkouts_dir=str(tmp_path),
    )
    assert "kserve" in components
    assert components["kserve"].checkout_path == checkout


def test_process_manifest_script_dispatches_to_shell(tmp_path: Path):
    script = tmp_path / "get_all_manifests.sh"
    checkout = tmp_path / "kserve"
    checkout.mkdir()
    script.write_text("""\
declare -A RHOAI_COMPONENT_MANIFESTS=(
    ["kserve"]="test-org:kserve:main@abc123:config"
)
""")
    components = process_manifest_script(
        str(script),
        platform="rhoai",
        checkouts_dir=str(tmp_path),
    )
    assert "kserve" in components


def test_resolve_script_path_prefers_shell_script(tmp_path: Path):
    operator_dir = tmp_path / "checkouts" / "org.platform" / "rhods-operator"
    operator_dir.mkdir(parents=True)
    (operator_dir / "get_all_manifests.sh").write_text("#!/bin/bash\n")
    (operator_dir / "manifests-config.yaml").write_text("components: {}\n")
    result = resolve_script_path(
        platform="rhoai",
        org="org",
        suffix="platform",
        checkouts_dir=str(tmp_path / "checkouts"),
    )
    assert result.endswith("get_all_manifests.sh")


def test_resolve_script_path_falls_back_to_yaml(tmp_path: Path):
    operator_dir = tmp_path / "checkouts" / "org.platform" / "rhods-operator"
    operator_dir.mkdir(parents=True)
    (operator_dir / "manifests-config.yaml").write_text("components: {}\n")
    result = resolve_script_path(
        platform="rhoai",
        org="org",
        suffix="platform",
        checkouts_dir=str(tmp_path / "checkouts"),
    )
    assert result.endswith("manifests-config.yaml")


def test_parse_manifests_config_normalizes_versioned_platform(tmp_path: Path):
    config = tmp_path / "manifests-config.yaml"
    config.write_text("""\
components:
  kserve:
    rhoai:
      repo: red-hat-data-services/kserve
      ref: rhoai-3.6@abc123
      sourcePath: config
""")
    components = parse_manifests_config(config, "rhoai-3.6-ea.1")
    assert "kserve" in components
    assert components["kserve"].repo_org == "red-hat-data-services"


def test_find_component_checkouts_rejects_path_traversal(tmp_path: Path):
    from lib.manifest_parser import ComponentInfo, find_component_checkouts

    outside = tmp_path / "outside"
    outside.mkdir()
    checkouts = tmp_path / "checkouts"
    checkouts.mkdir()

    components = {
        "evil": ComponentInfo(
            key="evil",
            repo_org="org",
            repo_name="../../outside",
            ref="main",
            source_folder="config",
        ),
        "good": ComponentInfo(
            key="good",
            repo_org="org",
            repo_name="kserve",
            ref="main",
            source_folder="config",
        ),
    }
    (checkouts / "kserve").mkdir()
    result = find_component_checkouts(components, checkouts)
    assert "evil" not in result
    assert "good" in result
