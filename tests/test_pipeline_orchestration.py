import asyncio
import sys
from pathlib import Path
from types import SimpleNamespace

PROJECT_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(PROJECT_ROOT))

from lib.phases import orchestration  # noqa: E402


def _component(key, repo_org, repo_name, repo_url=None):
    return SimpleNamespace(
        key=key,
        repo_org=repo_org,
        repo_name=repo_name,
        repo_url=repo_url or f"https://github.com/{repo_org}/{repo_name}",
    )


def test_resolve_pipeline_components_accepts_repo_selectors(monkeypatch):
    monkeypatch.setattr(
        orchestration,
        "read_component_map",
        lambda *args, **kwargs: {
            "models-as-a-service": _component(
                "models-as-a-service",
                "red-hat-data-services",
                "models-as-a-service",
            ),
            "llm-d-inference-scheduler": _component(
                "llm-d-inference-scheduler",
                "llm-d",
                "llm-d-inference-scheduler",
            ),
        },
    )
    args = SimpleNamespace(
        platform="rhoai.next",
        architecture_dir="architecture",
        component=["models-as-a-service"],
        repo=[
            "llm-d/llm-d-inference-scheduler",
            "models-as-a-service",
        ],
    )

    assert orchestration._resolve_pipeline_components(args) == [
        "models-as-a-service",
        "llm-d-inference-scheduler",
    ]


def test_pipeline_dispatches_component_scoped_phases_in_order(monkeypatch):
    calls = []

    async def fake_run_pipeline_phase(phase, phase_args):
        calls.append((phase, getattr(phase_args, "component", None)))

    monkeypatch.setattr(orchestration, "_run_pipeline_phase", fake_run_pipeline_phase)
    args = SimpleNamespace(
        platform="rhoai.next",
        phase=["static-analysis", "generate-architecture"],
        component=["models-as-a-service", "eval-hub"],
        repo=[],
        architecture_dir="architecture",
        checkouts_dir="checkouts",
        max_concurrent=1,
        force=True,
        skip_schemas=False,
        model="opus",
        log_dir="logs/pipeline/test/generate-architecture",
        version=None,
        evidence_gated_merge=True,
        tier="all",
        strace=False,
    )

    asyncio.run(orchestration.run_pipeline_phases(args))

    assert calls == [
        ("static-analysis", "models-as-a-service"),
        ("static-analysis", "eval-hub"),
        ("generate-architecture", "models-as-a-service"),
        ("generate-architecture", "eval-hub"),
    ]


def test_pipeline_lets_discovery_resolve_platform_checkout_dirs():
    args = SimpleNamespace(
        platform="rhoai-3.6-ea.2",
        architecture_dir="architecture",
        checkouts_dir="checkouts",
        max_concurrent=1,
        force=True,
        model="opus",
        harness="codex",
        strace=False,
    )

    phase_args = orchestration._pipeline_phase_args(
        args, "discover-components", None,
    )

    assert phase_args.checkouts_dir is None
    assert phase_args.harness == "codex"


def test_targeted_pipeline_runs_version_index_once_and_only_when_selected(
    monkeypatch,
):
    calls = []

    async def fake_run_pipeline_phase(phase, phase_args):
        calls.append((phase, getattr(phase_args, "component", None)))

    monkeypatch.setattr(orchestration, "_run_pipeline_phase", fake_run_pipeline_phase)
    args = SimpleNamespace(
        platform="rhoai.next",
        phase=["generate-architecture", "generate-index"],
        component=["one", "two"],
        repo=[],
        architecture_dir="architecture",
        checkouts_dir="checkouts",
        max_concurrent=1,
        force=False,
        model=None,
        harness="claude",
        log_dir="logs/test",
        version=None,
        evidence_gated_merge=True,
        tier="all",
        strace=False,
        platforms_file="platforms.yaml",
        overlays_dir="overlays",
    )

    asyncio.run(orchestration.run_pipeline_phases(args))

    assert calls == [
        ("generate-architecture", "one"),
        ("generate-architecture", "two"),
        ("generate-index", None),
    ]


def test_all_places_index_after_platform_and_before_diagrams(monkeypatch):
    calls = []

    def fake(name):
        async def run(_args):
            calls.append(name)

        return run

    monkeypatch.setattr(orchestration, "load_platform_config", lambda *_: {"orgs": []})
    monkeypatch.setattr(orchestration, "run_fetch_phase", fake("fetch"))
    monkeypatch.setattr(orchestration, "run_manifest_phase", fake("manifests"))
    monkeypatch.setattr(
        orchestration, "run_discover_components_phase", fake("discover")
    )
    monkeypatch.setattr(
        orchestration, "run_static_analysis_phase", fake("static-analysis")
    )
    monkeypatch.setattr(
        orchestration, "run_generate_architecture_phase", fake("architecture")
    )
    monkeypatch.setattr(
        orchestration,
        "run_generate_platform_architecture_phase",
        fake("platform"),
    )
    monkeypatch.setattr(orchestration, "run_generate_index_phase", fake("index"))
    monkeypatch.setattr(
        orchestration, "run_generate_diagrams_phase", fake("diagrams")
    )
    args = SimpleNamespace(
        platform="rhoai.next",
        org=None,
        suffix=None,
        branch=None,
        version=None,
        clean=False,
        force=False,
        harness="codex",
        model=None,
        pull=False,
        strace=False,
        max_concurrent=1,
        evidence_gated_merge=True,
        tier="all",
        no_diagrams=False,
        export_png=False,
    )

    asyncio.run(orchestration.run_all_phases(args))

    assert calls == [
        "fetch",
        "manifests",
        "discover",
        "static-analysis",
        "architecture",
        "platform",
        "index",
        "diagrams",
    ]
