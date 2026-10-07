import sys
from pathlib import Path
from types import SimpleNamespace

PROJECT_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(PROJECT_ROOT))

import main  # noqa: E402


def test_codex_discovery_does_not_require_claude_environment(
    tmp_path: Path,
    monkeypatch,
):
    monkeypatch.setattr(main, "Path", lambda *_args: tmp_path / "missing.env")
    args = SimpleNamespace(
        command="pipeline",
        harness="codex",
        phase=["fetch", "discover-components"],
    )

    main._load_agent_environment(args)


def test_fetch_does_not_require_claude_environment(
    tmp_path: Path,
    monkeypatch,
):
    monkeypatch.setattr(main, "Path", lambda *_args: tmp_path / "missing.env")

    main._load_agent_environment(SimpleNamespace(command="fetch"))


def test_generate_index_does_not_require_claude_environment(
    tmp_path: Path,
    monkeypatch,
):
    monkeypatch.setattr(main, "Path", lambda *_args: tmp_path / "missing.env")

    main._load_agent_environment(SimpleNamespace(command="generate-index"))


def test_index_only_pipeline_does_not_require_claude_environment(
    tmp_path: Path,
    monkeypatch,
):
    monkeypatch.setattr(main, "Path", lambda *_args: tmp_path / "missing.env")
    args = SimpleNamespace(
        command="pipeline",
        harness="claude",
        phase=["generate-index"],
    )

    main._load_agent_environment(args)
