"""Pipeline boundary tests for the deterministic index phase."""

from __future__ import annotations

import asyncio
import json
import sys
from pathlib import Path
from types import SimpleNamespace

PROJECT_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(PROJECT_ROOT))

from lib.phases import diagrams, index, platform  # noqa: E402


def test_index_phase_runs_without_agent_dispatch(tmp_path: Path, monkeypatch) -> None:
    architecture = tmp_path / "architecture"
    version = architecture / "rhoai-test"
    version.mkdir(parents=True)
    (version / "component-map.json").write_text(
        json.dumps({"components": {"example": {"type": "service"}}})
    )
    (version / "example.md").write_text(
        "# Example\n\n## Purpose\n\nExample service.\n"
    )

    def forbidden(*_args, **_kwargs):
        raise AssertionError("agent dispatch is forbidden for generate-index")

    monkeypatch.setattr(diagrams, "run_agents_concurrently", forbidden)
    monkeypatch.setattr(platform, "run_agents_concurrently", forbidden)
    args = SimpleNamespace(
        architecture_dir=str(architecture),
        platform="rhoai-test",
        platforms_file=str(tmp_path / "missing-platforms.yaml"),
        overlays_dir=str(tmp_path / "missing-overlays"),
    )

    asyncio.run(index.run_generate_index_phase(args))

    assert (version / "INDEX.md").is_file()


def test_diagram_discovery_excludes_version_index(tmp_path: Path, monkeypatch) -> None:
    architecture = tmp_path / "architecture"
    version = architecture / "rhoai-test"
    version.mkdir(parents=True)
    (version / "INDEX.md").write_text("# Index\n")

    async def forbidden(*_args, **_kwargs):
        raise AssertionError("INDEX.md must not create a diagram job")

    monkeypatch.setattr(diagrams, "run_agents_concurrently", forbidden)
    args = SimpleNamespace(
        architecture_dir=str(architecture),
        platform="rhoai-test",
        version=None,
        component=None,
        force_regenerate=False,
        limit=None,
        export_png=False,
        max_concurrent=1,
        model=None,
        harness="claude",
        strace=False,
    )

    asyncio.run(diagrams.run_generate_diagrams_phase(args))


def test_platform_aggregation_excludes_version_index(
    tmp_path: Path, monkeypatch
) -> None:
    architecture = tmp_path / "architecture"
    version = architecture / "rhoai-test"
    version.mkdir(parents=True)
    (version / "INDEX.md").write_text("# Index\n")

    async def forbidden(*_args, **_kwargs):
        raise AssertionError("INDEX.md must not create a platform aggregation job")

    monkeypatch.setattr(platform, "_ensure_arch_query", forbidden)
    monkeypatch.setattr(platform, "run_agents_concurrently", forbidden)
    args = SimpleNamespace(
        architecture_dir=str(architecture),
        platform="rhoai-test",
        version=None,
        force=False,
        limit=None,
        max_concurrent=1,
        model=None,
        harness="claude",
        strace=False,
    )

    asyncio.run(platform.run_generate_platform_architecture_phase(args))


def test_diagram_jobs_keep_platform_output_and_nest_accepted_components(
    tmp_path: Path, monkeypatch
) -> None:
    architecture = tmp_path / "architecture"
    version = architecture / "rhoai-test"
    component_dir = version / "example"
    component_dir.mkdir(parents=True)
    (component_dir / "document.json").write_text("fixture marker")
    (version / "example.md").write_text("# Component: example\n")
    (version / "PLATFORM.md").write_text("# Platform\n")
    captured = []

    async def fake_ensure():
        return "/offline/arch-analyzer"

    def fake_publications(*_args, **_kwargs):
        return {"example": SimpleNamespace(document=b"accepted-document")}

    async def fake_run(jobs, *_args, **_kwargs):
        captured.extend(jobs)
        for job in jobs:
            job["diagrams_dir"].mkdir(parents=True, exist_ok=True)
            (job["diagrams_dir"] / f"{job['component_name']}-context.mmd").write_text(
                "graph TD\n"
            )
        return [{"success": True, "name": job["name"]} for job in jobs]

    monkeypatch.setattr(diagrams, "_ensure_arch_analyzer", fake_ensure)
    monkeypatch.setattr(diagrams, "accepted_publications", fake_publications)
    monkeypatch.setattr(diagrams, "run_agents_concurrently", fake_run)
    args = SimpleNamespace(
        architecture_dir=str(architecture),
        platform="rhoai-test",
        version=None,
        component=None,
        force_regenerate=False,
        limit=None,
        export_png=False,
        max_concurrent=1,
        model="offline-model",
        harness="claude",
        strace=False,
    )
    asyncio.run(diagrams.run_generate_diagrams_phase(args))

    jobs = {job["component_name"]: job for job in captured}
    assert jobs["platform"]["diagrams_dir"] == version / "diagrams"
    assert jobs["example"]["diagrams_dir"] == component_dir / "diagrams"
    metadata = json.loads(
        (component_dir / "diagrams" / "metadata.json").read_text()
    )
    assert metadata["state"] == "available"
    assert metadata["document_hash"].startswith("sha256:")

    captured.clear()
    asyncio.run(diagrams.run_generate_diagrams_phase(args))
    assert captured == []

    metadata["document_hash"] = "sha256:" + "0" * 64
    (component_dir / "diagrams" / "metadata.json").write_text(
        json.dumps(metadata)
    )
    asyncio.run(diagrams.run_generate_diagrams_phase(args))
    assert [job["component_name"] for job in captured] == ["example"]

    accepted_before = (version / "example.md").read_bytes()

    async def fake_failure(jobs, *_args, **_kwargs):
        return [
            {"success": False, "name": job["name"], "error": "offline failure"}
            for job in jobs
        ]

    metadata = json.loads(
        (component_dir / "diagrams" / "metadata.json").read_text()
    )
    metadata["generator_identity"] = "stale-generator"
    (component_dir / "diagrams" / "metadata.json").write_text(
        json.dumps(metadata)
    )
    monkeypatch.setattr(diagrams, "run_agents_concurrently", fake_failure)
    asyncio.run(diagrams.run_generate_diagrams_phase(args))
    failed = json.loads(
        (component_dir / "diagrams" / "metadata.json").read_text()
    )
    assert failed["state"] == "failed"
    assert (version / "example.md").read_bytes() == accepted_before
