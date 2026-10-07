import asyncio
import copy
import io
import json
import shlex
import subprocess
import sys
from pathlib import Path
from types import SimpleNamespace

import pytest
from openai_codex import SkillInput, TextInput
from openai_codex.client import CodexClient
from openai_codex.errors import map_jsonrpc_error
from openai_codex.generated import v2_all as codex_types
from openai_codex.generated.v2_all import ThreadStartParams, TurnStatus
from openai_codex.models import Notification, UnknownNotification

PROJECT_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(PROJECT_ROOT))

from lib import agent_runner, codex_agent  # noqa: E402


def _install_structured_transport_stub(
    monkeypatch: pytest.MonkeyPatch,
    turn: object,
    captured: dict,
    *,
    resolved_model: str = "gpt-5.6-sol",
    model_provider: str = "openai",
    reasoning_effort: str = "high",
    service_tier: str | None = None,
    rpc_error: BaseException | None = None,
    error_stage: str | None = None,
) -> None:
    """Install a protocol-level stub before AsyncCodex can construct transport."""

    class FakeThread:
        def __init__(self, _codex, thread_id):
            self.id = thread_id

        async def turn(self, _input, **kwargs):
            captured["turn"] = kwargs
            if error_stage == "turn/start":
                assert rpc_error is not None
                raise rpc_error
            return turn

    class FakeProtocol:
        async def request(self, method, params, *, response_model):
            captured["config_read"] = {
                "method": method,
                "params": params,
                "response_model": response_model,
            }
            if error_stage == "config/read":
                assert rpc_error is not None
                raise rpc_error
            return SimpleNamespace(config={
                "model": resolved_model,
                "model_provider": model_provider,
                "model_reasoning_effort": reasoning_effort,
                "service_tier": service_tier,
            })

        async def thread_start(self, params):
            captured["params"] = params
            if error_stage == "thread/start":
                assert rpc_error is not None
                raise rpc_error
            return SimpleNamespace(
                thread=SimpleNamespace(id="thread-structured-stub"),
                model=resolved_model,
                model_provider=model_provider,
                reasoning_effort=reasoning_effort,
                service_tier=service_tier,
                instruction_sources=[],
                runtime_workspace_roots=[],
                sandbox={"type": "readOnly"},
                approval_policy={"type": "never"},
            )

    class FakeCodex:
        def __init__(self, config=None):
            captured["codex_config"] = config
            self._client = FakeProtocol()

        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, traceback):
            return False

    monkeypatch.setattr(codex_agent, "AsyncCodex", FakeCodex)
    monkeypatch.setattr(codex_agent, "AsyncThread", FakeThread)
    monkeypatch.setattr(
        codex_agent,
        "resolve_codex_cli_identity",
        lambda: (
            "/transport-blocked/codex",
            {
                "implementation": "codex-cli",
                "version_output": "codex-cli 0.147.0-test",
                "binary_sha256": "sha256:" + "a" * 64,
            },
        ),
    )

    def isolated(private_home, *, cli_path):
        return (
            codex_agent.CodexConfig(
                codex_bin=cli_path,
                env={"CODEX_HOME": str(private_home)},
            ),
            {
                "protocol": codex_agent._CODEX_CONTEXT_PROTOCOL,
                "auth_route": "transport-blocked-test-auth",
                "staged_model_config": {},
                "staged_model_config_identity": codex_agent._canonical_identity({}),
                "suppressed_local_sources": [],
                "environment_access": "disabled-by-empty-thread-environments",
            },
        )

    monkeypatch.setattr(codex_agent, "_isolated_codex_config", isolated)


@pytest.mark.asyncio
async def test_codex_tool_free_uses_one_bounded_schema_turn(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch,
):
    captured = {}

    class FakeTurn:
        id = "turn-structured"

        async def stream(self):
            yield _event(
                "item/completed",
                SimpleNamespace(
                    turn_id=self.id,
                    item={
                        "type": "agentMessage",
                        "id": "answer",
                        "phase": "finalAnswer",
                        "text": '{"ok":true}',
                    },
                ),
            )
            yield _event(
                "turn/completed",
                SimpleNamespace(
                    turn=SimpleNamespace(
                        id=self.id,
                        status=TurnStatus.completed,
                        error=None,
                        duration_ms=2,
                    )
                ),
            )

    class FakeThread:
        def __init__(self, _codex, thread_id):
            self.id = thread_id

        async def turn(self, agent_input, **kwargs):
            captured["input"] = agent_input
            captured["turn"] = kwargs
            return FakeTurn()

    class FakeProtocol:
        async def thread_start(self, params):
            captured["params"] = params
            return SimpleNamespace(
                thread=SimpleNamespace(id="thread-structured"),
                model="gpt-5.6-sol",
                model_provider="openai",
                reasoning_effort="high",
                instruction_sources=[],
                sandbox={"type": "readOnly"},
                approval_policy={"type": "never"},
            )

    class FakeCodex:
        def __init__(self):
            self._client = FakeProtocol()

        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, traceback):
            return False

    _install_structured_transport_stub(
        monkeypatch,
        FakeTurn(),
        captured,
    )

    schema = {"type": "object"}
    result = await codex_agent.run_codex_agent(
        name="structured",
        cwd=str(tmp_path),
        prompt="return JSON",
        log_dir=tmp_path / "logs",
        model="gpt-5.6-sol",
        enable_skills=False,
        progress=None,
        strace_dir=None,
        tool_free=True,
        response_schema=schema,
    )

    assert result["success"] is True
    assert result["raw_response"] == '{"ok":true}'
    assert captured["params"] == {
        "approvalPolicy": "never",
        "baseInstructions": codex_agent._STRUCTURED_BASE_INSTRUCTIONS,
        "config": {"mcp_servers": {}},
        "cwd": str(tmp_path),
        "developerInstructions": codex_agent._STRUCTURED_DEVELOPER_INSTRUCTIONS,
        "dynamicTools": [],
        "environments": [],
        "ephemeral": True,
        "model": "gpt-5.6-sol",
        "sandbox": "read-only",
        "allowProviderModelFallback": False,
    }
    assert captured["turn"]["output_schema"] == schema
    assert captured["turn"]["approval_mode"] == codex_agent.ApprovalMode.deny_all
    assert captured["turn"]["sandbox"] == codex_agent.Sandbox.read_only
    assert result["telemetry"]["response_models"] == ["gpt-5.6-sol"]
    assert result["telemetry"]["parent_requested_model"] == "gpt-5.6-sol"
    assert result["telemetry"]["resolved_model_identity"] == "gpt-5.6-sol"
    assert result["telemetry"]["producing_model_identity"] == "gpt-5.6-sol"
    assert result["telemetry"]["applied_model_settings"] == {
        "model_provider": "openai",
        "reasoning_effort": "high",
        "service_tier": None,
    }
    assert result["telemetry"]["structured_execution"][
        "provider_model_fallback_allowed"
    ] is False

    # The generated 0.147.0 model silently drops this experimental field, but
    # the supported JsonObject path used by the client preserves it verbatim.
    typed = ThreadStartParams(
        model="gpt-5.6-sol",
        allowProviderModelFallback=False,
    )
    assert "allowProviderModelFallback" not in typed.model_dump(by_alias=True)
    class TransportBlockedClient(CodexClient):
        def __init__(self):
            self.config = codex_agent.CodexConfig(experimental_api=True)
            self.capture = {}

        def request(self, method, params, *, response_model):
            self.capture.update({"method": method, "params": params})
            return SimpleNamespace()

    sdk_client = TransportBlockedClient()
    assert sdk_client.config.experimental_api is True
    sdk_client.thread_start(captured["params"])
    assert sdk_client.capture == {
        "method": "thread/start",
        "params": captured["params"],
    }


@pytest.mark.asyncio
async def test_codex_missing_model_is_omitted_and_resolved_before_turn(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
):
    captured = {}

    class FakeTurn:
        id = "turn-default"

        async def stream(self):
            yield _event(
                "item/completed",
                SimpleNamespace(
                    turn_id=self.id,
                    item={
                        "type": "agentMessage",
                        "phase": "finalAnswer",
                        "text": '{"ok":true}',
                    },
                ),
            )
            yield _event(
                "turn/completed",
                SimpleNamespace(
                    turn=SimpleNamespace(
                        id=self.id,
                        status=TurnStatus.completed,
                        error=None,
                        duration_ms=1,
                    )
                ),
            )

    _install_structured_transport_stub(
        monkeypatch,
        FakeTurn(),
        captured,
        resolved_model="gpt-configured-real",
        reasoning_effort="xhigh",
    )
    result = await codex_agent.run_codex_agent(
        name="configured-default",
        cwd=str(tmp_path),
        prompt="return JSON",
        log_dir=tmp_path / "logs",
        model=None,
        enable_skills=False,
        progress=None,
        strace_dir=None,
        tool_free=True,
        response_schema={"type": "object"},
    )

    assert result["success"] is True
    assert "model" not in captured["params"]
    assert captured["turn"]["output_schema"] == {"type": "object"}
    assert result["telemetry"]["parent_requested_model"] is None
    assert result["telemetry"]["resolved_model_identity"] == (
        "gpt-configured-real"
    )
    assert result["telemetry"]["applied_model_settings"] == {
        "model_provider": "openai",
        "reasoning_effort": "xhigh",
        "service_tier": None,
    }

    class TransportBlockedClient(CodexClient):
        def __init__(self):
            self.capture = {}

        def request(self, method, params, *, response_model):
            self.capture.update({"method": method, "params": params})
            return SimpleNamespace()

    sdk_client = TransportBlockedClient()
    sdk_client.thread_start(captured["params"])
    assert sdk_client.capture["method"] == "thread/start"
    assert sdk_client.capture["params"] == captured["params"]
    assert "model" not in sdk_client.capture["params"]


@pytest.mark.asyncio
async def test_codex_explicit_model_mismatch_rejects_before_model_turn(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
):
    captured = {}

    class MustNotTurn:
        id = "must-not-turn"

        async def stream(self):
            raise AssertionError("model turn must not start")
            yield

    _install_structured_transport_stub(
        monkeypatch,
        MustNotTurn(),
        captured,
        resolved_model="gpt-5.6-mini",
    )
    result = await codex_agent.run_codex_agent(
        name="mismatch",
        cwd=str(tmp_path),
        prompt="must not run",
        log_dir=tmp_path / "logs",
        model="gpt-5.6-sol",
        enable_skills=False,
        progress=None,
        strace_dir=None,
        tool_free=True,
        response_schema={"type": "object"},
    )

    assert result["success"] is False
    assert captured["params"]["model"] == "gpt-5.6-sol"
    assert "turn" not in captured
    assert "raw_response" not in result
    assert result["provider_error"] == {
        "parent_requested_model": "gpt-5.6-sol",
        "resolved_model_identity": "gpt-5.6-mini",
        "model_provider": "openai",
        "reasoning_effort": "high",
    }


@pytest.mark.asyncio
async def test_codex_changed_preflight_context_rejects_before_model_turn(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
):
    captured = {}
    _install_structured_transport_stub(
        monkeypatch,
        object(),
        captured,
        resolved_model="gpt-5.6-sol",
    )

    result = await codex_agent.run_codex_agent(
        name="changed-context",
        cwd=str(tmp_path),
        prompt="must not run",
        log_dir=tmp_path / "logs",
        model="gpt-5.6-sol",
        enable_skills=False,
        progress=None,
        strace_dir=None,
        tool_free=True,
        response_schema={"type": "object"},
        expected_structured_context={
            "context_identity": "sha256:" + "0" * 64
        },
    )

    assert result["success"] is False
    assert "turn" not in captured
    assert result["provider_error"]["expected_context_identity"] == (
        "sha256:" + "0" * 64
    )
    assert result["provider_error"]["actual_context_identity"] != (
        "sha256:" + "0" * 64
    )


@pytest.mark.asyncio
async def test_codex_preflight_is_turn_free_and_binds_configured_selection(
    monkeypatch: pytest.MonkeyPatch,
):
    captured = {}
    _install_structured_transport_stub(
        monkeypatch,
        object(),
        captured,
        resolved_model="gpt-configured-real",
        model_provider="configured-provider",
        reasoning_effort="high",
        service_tier="priority",
    )

    context = await codex_agent.preflight_structured_codex_context(None)

    assert "turn" not in captured
    assert "model" not in captured["params"]
    assert captured["config_read"]["method"] == "config/read"
    assert context["parent_requested_model"] is None
    assert context["selection_mode"] == "configured-default"
    assert context["resolved_model_identity"] == "gpt-configured-real"
    assert context["model_provider"] == "configured-provider"
    assert context["reasoning_effort"] == "high"
    assert context["service_tier"] == "priority"
    assert context["context_identity"].startswith("sha256:")


def test_codex_isolated_context_stages_only_auth_and_model_settings(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
):
    source_home = tmp_path / "source-home"
    source_home.mkdir()
    (source_home / "auth.json").write_text('{"fake":"test-only"}\n')
    (source_home / "config.toml").write_text(
        'model = "configured-model"\n'
        'model_reasoning_effort = "high"\n'
        '[mcp_servers.unrelated]\ncommand = "must-not-copy"\n'
        '[projects."/tmp"]\ntrust_level = "trusted"\n'
    )
    for name in ("memories", "plugins", "skills", "rules"):
        directory = source_home / name
        directory.mkdir()
        (directory / "unrelated").write_text("must not copy")
    monkeypatch.setenv("CODEX_HOME", str(source_home))
    private_home = tmp_path / "private-home"

    config, staged = codex_agent._isolated_codex_config(
        private_home,
        cli_path="/transport-blocked/codex",
    )

    assert config.env == {"CODEX_HOME": str(private_home)}
    assert config.config_overrides == (
        'model="configured-model"',
        'model_reasoning_effort="high"',
    )
    assert (private_home / "auth.json").read_text() == (
        '{"fake":"test-only"}\n'
    )
    assert not (private_home / "config.toml").exists()
    assert sorted(path.name for path in private_home.iterdir()) == ["auth.json"]
    assert staged["staged_model_config"] == {
        "model": "configured-model",
        "model_reasoning_effort": "high",
    }
    assert "mcp_servers" not in staged["staged_model_config"]


@pytest.mark.asyncio
async def test_codex_structured_protocol_rejects_producing_model_reroute(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch,
):
    captured = {}

    class FakeTurn:
        id = "turn-rerouted"

        async def interrupt(self):
            captured["interrupted"] = True

        async def stream(self):
            yield _event(
                "item/completed",
                SimpleNamespace(
                    turn_id=self.id,
                    item={
                        "type": "agentMessage",
                        "id": "answer",
                        "phase": "finalAnswer",
                        "text": '{"ok":true}',
                    },
                ),
            )
            yield _event(
                "model/rerouted",
                SimpleNamespace(
                    turn_id=self.id,
                    from_model="gpt-requested-resolved",
                    to_model="gpt-producing-reroute",
                    reason="highRiskCyberActivity",
                ),
            )
            yield _event(
                "turn/completed",
                SimpleNamespace(
                    turn=SimpleNamespace(
                        id=self.id,
                        status=TurnStatus.completed,
                        error=None,
                        duration_ms=3,
                    )
                ),
            )

    class FakeThread:
        id = "thread-real-protocol"

        def __init__(self, _codex, thread_id):
            self.id = thread_id

        async def turn(self, _input, **kwargs):
            captured["turn"] = kwargs
            return FakeTurn()

    class FakeProtocol:
        async def thread_start(self, params):
            captured["params"] = params
            return SimpleNamespace(
                thread=SimpleNamespace(id="thread-real-protocol"),
                model="gpt-requested-resolved",
                model_provider="openai",
                reasoning_effort="high",
                instruction_sources=[],
                sandbox={"type": "readOnly"},
                approval_policy={"type": "never"},
            )

    class FakeCodex:
        def __init__(self):
            self._client = FakeProtocol()

        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, traceback):
            return False

    _install_structured_transport_stub(
        monkeypatch,
        FakeTurn(),
        captured,
        resolved_model="gpt-requested-resolved",
    )
    result = await codex_agent.run_codex_agent(
        name="structured",
        cwd=str(tmp_path),
        prompt="return JSON",
        log_dir=tmp_path / "logs",
        model="gpt-requested-resolved",
        enable_skills=False,
        progress=None,
        strace_dir=None,
        tool_free=True,
        response_schema={"type": "object"},
    )

    assert captured["params"]["model"] == "gpt-requested-resolved"
    assert captured["params"]["sandbox"] == "read-only"
    assert captured["params"]["allowProviderModelFallback"] is False
    assert captured["interrupted"] is True
    assert result["success"] is False
    assert "raw_response" not in result
    assert result["provider_error"]["kind"] == "codex-model-reroute"
    assert result["provider_error"]["model_reroute"][
        "rate_limit_driven"
    ] is False
    assert result["rate_limit_denied"] is False


@pytest.mark.asyncio
async def test_codex_structured_turn_interrupts_and_rejects_first_tool_event(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch,
):
    interrupted = []

    class ToolTurn:
        id = "turn-tool"

        async def interrupt(self):
            interrupted.append(self.id)

        async def stream(self):
            yield _event(
                "item/started",
                SimpleNamespace(
                    turn_id=self.id,
                    item={
                        "type": "commandExecution",
                        "id": "command",
                        "command": "pwd",
                    },
                ),
            )

    captured = {}
    _install_structured_transport_stub(monkeypatch, ToolTurn(), captured)
    result = await codex_agent.run_codex_agent(
        name="structured",
        cwd=str(tmp_path),
        prompt="return JSON",
        log_dir=tmp_path / "logs",
        model="gpt-5.6-sol",
        enable_skills=False,
        progress=None,
        strace_dir=None,
        tool_free=True,
        response_schema={"type": "object"},
    )

    assert result["success"] is False
    assert interrupted == ["turn-tool"]
    assert captured["params"]["allowProviderModelFallback"] is False
    assert result["provider_error"]["kind"] == "codex-tool-activity"
    assert result["telemetry"]["tool_activity_observations"][0]["method"] == (
        "item/started"
    )


@pytest.mark.asyncio
@pytest.mark.parametrize("method", ["item/started", "item/completed"])
@pytest.mark.parametrize(
    "item",
    [
        {"type": "commandExecution"},
        {"type": "fileChange"},
        {"type": "mcpToolCall"},
        {"type": "dynamicToolCall"},
        {"type": "collabAgentToolCall"},
        {"type": "subAgentActivity"},
        {"type": "webSearch"},
        {"type": "imageView"},
        {"type": "sleep"},
        {"type": "imageGeneration"},
        {"type": "hookPrompt"},
        {"type": "enteredReviewMode"},
        {"type": "exitedReviewMode"},
        {"type": "contextCompaction"},
        {"type": "functionCallOutput"},
        {"type": "futureToolActivity"},
        {},
        None,
    ],
    ids=lambda item: "missing" if item is None else item.get("type", "malformed"),
)
async def test_stream_guard_rejects_all_activity_and_unknown_lifecycle_items(
    method,
    item,
):
    interrupted = []

    class GuardedTurn:
        id = "turn-guarded"

        async def interrupt(self):
            interrupted.append(self.id)

        async def stream(self):
            if item == {"type": "futureToolActivity"}:
                payload = SimpleNamespace(params={
                    "turnId": self.id,
                    "item": item,
                })
            else:
                payload = SimpleNamespace(turn_id=self.id, item=item)
            yield _event(method, payload)
            raise AssertionError("guard continued after disallowed activity")

    with pytest.raises(codex_agent._CodexToolActivityRejected) as rejected:
        await codex_agent._stream_turn(
            GuardedTurn(),
            io.StringIO(),
            "guarded",
            False,
            reject_tool_activity=True,
        )

    assert interrupted == ["turn-guarded"]
    assert rejected.value.activity["method"] == method
    assert rejected.value.activity["interruption"] == {
        "attempted": True,
        "request_succeeded": True,
    }


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "item_type",
    [
        "agentMessage",
        "plan",
        "reasoning",
        "userMessage",
    ],
)
async def test_stream_guard_allows_known_passive_items(item_type):
    class PassiveTurn:
        id = "turn-passive"

        async def interrupt(self):
            raise AssertionError("passive items must not be interrupted")

        async def stream(self):
            yield _event(
                "item/completed",
                SimpleNamespace(
                    turn_id=self.id,
                    item={
                        "type": item_type,
                        "phase": "finalAnswer",
                        "text": "answer",
                    },
                ),
            )
            yield _event(
                "turn/completed",
                SimpleNamespace(
                    turn=SimpleNamespace(
                        id=self.id,
                        status=TurnStatus.completed,
                        error=None,
                        duration_ms=1,
                        items=[],
                    )
                ),
            )

    result = await codex_agent._stream_turn(
        PassiveTurn(),
        io.StringIO(),
        "passive",
        False,
        reject_tool_activity=True,
    )

    assert result.status == TurnStatus.completed


@pytest.mark.asyncio
async def test_stream_guard_rejects_terminal_tool_item_after_final_answer():
    answer = {
        "type": "agentMessage",
        "phase": "finalAnswer",
        "text": '{"ok":true}',
    }

    class RacedTurn:
        id = "turn-raced"

        async def stream(self):
            yield _event(
                "item/completed",
                SimpleNamespace(turn_id=self.id, item=answer),
            )
            yield _event(
                "turn/completed",
                SimpleNamespace(
                    turn={
                        "id": self.id,
                        "status": "completed",
                        "error": None,
                        "durationMs": 1,
                        "items": [answer, {"type": "imageGeneration"}],
                    }
                ),
            )

    with pytest.raises(codex_agent._CodexToolActivityRejected) as rejected:
        await codex_agent._stream_turn(
            RacedTurn(),
            io.StringIO(),
            "raced",
            False,
            reject_tool_activity=True,
        )

    assert rejected.value.activity == {
        "method": "turn/completed:items",
        "item": {"type": "imageGeneration"},
        "interruption": {
            "attempted": False,
            "reason": "turn-already-completed",
        },
    }


@pytest.mark.asyncio
async def test_real_unknown_lifecycle_without_turn_id_is_not_passive():
    event = Notification(
        method="item/started",
        payload=UnknownNotification(params={
            "item": {"type": "commandExecution", "id": "unknown-turn"}
        }),
    )

    class MissingIdentityTurn:
        id = "turn-current"

        def __init__(self):
            self.interrupts = 0

        async def interrupt(self):
            self.interrupts += 1

        async def stream(self):
            yield event
            raise AssertionError("guard continued after missing turn identity")

    turn = MissingIdentityTurn()
    with pytest.raises(codex_agent._CodexToolActivityRejected) as rejected:
        await codex_agent._stream_turn(
            turn,
            io.StringIO(),
            "missing-identity",
            False,
            reject_tool_activity=True,
        )

    assert turn.interrupts == 1
    assert rejected.value.activity["reason"] == "missing-or-malformed-turn-id"
    assert rejected.value.activity["interruption"] == {
        "attempted": True,
        "request_succeeded": True,
    }


@pytest.mark.asyncio
async def test_real_terminal_incomplete_items_view_rejects_collected_answer():
    answer = codex_types.ThreadItem(root=codex_types.AgentMessageThreadItem(
        id="answer",
        phase="final_answer",
        text='{"ok":true}',
        type="agentMessage",
    ))
    completed_answer = Notification(
        method="item/completed",
        payload=codex_types.ItemCompletedNotification(
            item=answer,
            thread_id="thread",
            turn_id="turn-current",
            completed_at_ms=1,
        ),
    )
    incomplete = Notification(
        method="turn/completed",
        payload=codex_types.TurnCompletedNotification(
            thread_id="thread",
            turn=codex_types.Turn(
                id="turn-current",
                items=[],
                items_view=codex_types.TurnItemsView.not_loaded,
                status=codex_types.TurnStatus.completed,
            ),
        ),
    )

    class IncompleteTerminalTurn:
        id = "turn-current"

        async def stream(self):
            yield completed_answer
            yield incomplete

    with pytest.raises(codex_agent._CodexToolActivityRejected) as rejected:
        await codex_agent._stream_turn(
            IncompleteTerminalTurn(),
            io.StringIO(),
            "incomplete-terminal",
            False,
            reject_tool_activity=True,
        )

    assert rejected.value.activity == {
        "method": "turn/completed:items",
        "item": None,
        "reason": "terminal-items-view-not-full",
        "items_view": "notLoaded",
        "interruption": {
            "attempted": False,
            "reason": "turn-already-completed",
        },
    }


@pytest.mark.asyncio
async def test_codex_tool_rejection_survives_interrupt_failure_without_output(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
):
    continued = False

    class ToolTurn:
        id = "turn-interrupt-failed"

        async def interrupt(self):
            raise RuntimeError("turn already completed")

        async def stream(self):
            nonlocal continued
            yield _event(
                "item/completed",
                SimpleNamespace(
                    turn_id=self.id,
                    item={"type": "imageView", "id": "view"},
                ),
            )
            continued = True
            yield _event(
                "turn/completed",
                SimpleNamespace(turn=SimpleNamespace(id=self.id)),
            )

    captured = {}
    _install_structured_transport_stub(monkeypatch, ToolTurn(), captured)
    result = await codex_agent.run_codex_agent(
        name="interrupt-failed",
        cwd=str(tmp_path),
        prompt="return JSON",
        log_dir=tmp_path / "logs",
        model="gpt-5.6-sol",
        enable_skills=False,
        progress=None,
        strace_dir=None,
        tool_free=True,
        response_schema={"type": "object"},
    )

    assert result["success"] is False
    assert "raw_response" not in result
    assert continued is False
    assert result["provider_error"]["kind"] == "codex-tool-activity"
    assert result["provider_error"]["tool_activity"]["interruption"] == {
        "attempted": True,
        "request_succeeded": False,
        "error_type": "RuntimeError",
        "error": "turn already completed",
    }


@pytest.mark.asyncio
async def test_stream_ignores_reroute_for_unrelated_turn():
    class UnrelatedTurn:
        id = "turn-current"

        async def interrupt(self):
            raise AssertionError("unrelated reroute must not interrupt this turn")

        async def stream(self):
            yield _event(
                "model/rerouted",
                SimpleNamespace(
                    turn_id="turn-other",
                    from_model="gpt-a",
                    to_model="gpt-b",
                    reason="highRiskCyberActivity",
                ),
            )
            yield _event(
                "item/completed",
                SimpleNamespace(
                    turn_id=self.id,
                    item={
                        "type": "agentMessage",
                        "phase": "finalAnswer",
                        "text": "answer",
                    },
                ),
            )
            yield _event(
                "turn/completed",
                SimpleNamespace(
                    turn=SimpleNamespace(
                        id=self.id,
                        status=TurnStatus.completed,
                        error=None,
                        duration_ms=1,
                        items=[],
                    )
                ),
            )

    result = await codex_agent._stream_turn(
        UnrelatedTurn(),
        io.StringIO(),
        "unrelated",
        False,
        reject_tool_activity=True,
    )

    assert result.final_response == "answer"
    assert result.model_reroutes == []


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("code", "denied"),
    [("usageLimitExceeded", True), ("internalServerError", False)],
)
async def test_codex_failed_turn_retains_structured_provider_events_and_quota_code(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    code: str,
    denied: bool,
):
    class FailedTurn:
        id = "turn-failed"

        async def stream(self):
            yield _event(
                "error",
                SimpleNamespace(
                    turn_id=self.id,
                    error={"code": code, "message": "provider failure"},
                ),
            )
            yield _event(
                "turn/completed",
                SimpleNamespace(
                    turn=SimpleNamespace(
                        id=self.id,
                        status=TurnStatus.failed,
                        error=SimpleNamespace(
                            code=code, message="provider failure"
                        ),
                        duration_ms=4,
                    )
                ),
            )

    captured = {}
    _install_structured_transport_stub(monkeypatch, FailedTurn(), captured)
    result = await codex_agent.run_codex_agent(
        name="structured",
        cwd=str(tmp_path),
        prompt="return JSON",
        log_dir=tmp_path / "logs",
        model="gpt-5.6-sol",
        enable_skills=False,
        progress=None,
        strace_dir=None,
        tool_free=True,
        response_schema={"type": "object"},
    )

    assert result["success"] is False
    assert result["rate_limit_denied"] is denied
    assert result["provider_error"]["error"]["code"] == code
    assert result["provider_error"]["provider_events"][0]["method"] == "error"


@pytest.mark.asyncio
@pytest.mark.parametrize("stage", ["config/read", "thread/start", "turn/start"])
@pytest.mark.parametrize(
    ("rpc_data", "denied"),
    [
        pytest.param(
            {
                "codexErrorInfo": "usageLimitExceeded",
                "access_token": "test-secret-must-not-persist",
            },
            True,
            id="known-codex-error-info",
        ),
        pytest.param(
            {"errorInfo": "workspaceMemberUsageLimitReached"},
            True,
            id="sdk-error-info",
        ),
        pytest.param(
            "workspace_owner_usage_limit_reached",
            True,
            id="sdk-bare-data",
        ),
        pytest.param(
            {"codexErrorInfo": {"reason": "rate_limit_error"}},
            True,
            id="sdk-nested-info-value",
        ),
        pytest.param(
            {"codex_error_info": {"reason": "sessionBudgetExceeded"}},
            True,
            id="snake-nested-info-value",
        ),
        pytest.param(
            {"codexErrorInfo": "internalServerError"},
            False,
            id="ordinary-server-error",
        ),
        pytest.param(
            {"errorInfo": "unauthorized"},
            False,
            id="ordinary-auth-error",
        ),
        pytest.param(
            {"codex_error_info": "badRequest"},
            False,
            id="ordinary-config-error",
        ),
        pytest.param(
            "usage limit exceeded",
            False,
            id="bare-prose-is-not-code",
        ),
    ],
)
async def test_codex_rpc_exceptions_preserve_safe_structure_at_each_sdk_hop(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    stage: str,
    rpc_data: object,
    denied: bool,
):
    rpc_error = map_jsonrpc_error(
        -32000,
        "usage limit words alone are not authoritative",
        rpc_data,
    )
    captured = {}
    _install_structured_transport_stub(
        monkeypatch,
        object(),
        captured,
        rpc_error=rpc_error,
        error_stage=stage,
    )

    result = await codex_agent.run_codex_agent(
        name="rpc-error",
        cwd=str(tmp_path),
        prompt="must not complete",
        log_dir=tmp_path / "logs",
        model="gpt-5.6-sol",
        enable_skills=False,
        progress=None,
        strace_dir=None,
        tool_free=True,
        response_schema={"type": "object"},
    )

    assert result["success"] is False
    assert result["rate_limit_denied"] is denied
    expected_data = copy.deepcopy(rpc_data)
    if isinstance(expected_data, dict) and "access_token" in expected_data:
        expected_data["access_token"] = "[REDACTED]"
    assert result["provider_error"] == {
        "kind": "codex-rpc-error",
        "error_type": "CodexRpcError",
        "code": -32000,
        "message": "usage limit words alone are not authoritative",
        "data": expected_data,
    }
    assert "test-secret-must-not-persist" not in Path(result["log_file"]).read_text()
    assert ("turn" in captured) is (stage == "turn/start")


@pytest.mark.asyncio
async def test_codex_http_429_exception_is_structured_without_message_inference(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
):
    class HTTP429Error(RuntimeError):
        status_code = 429

    captured = {}
    _install_structured_transport_stub(
        monkeypatch,
        object(),
        captured,
        rpc_error=HTTP429Error("ordinary transport wording"),
        error_stage="thread/start",
    )

    result = await codex_agent.run_codex_agent(
        name="http-429",
        cwd=str(tmp_path),
        prompt="must not turn",
        log_dir=tmp_path / "logs",
        model="gpt-5.6-sol",
        enable_skills=False,
        progress=None,
        strace_dir=None,
        tool_free=True,
        response_schema={"type": "object"},
    )

    assert result["rate_limit_denied"] is True
    assert result["provider_error"]["status_code"] == 429
    assert result["provider_error"]["message"] == "ordinary transport wording"


def _event(method, payload):
    return SimpleNamespace(method=method, payload=payload)


def _rg_item(checkout: Path, identifier: str, command: str) -> dict:
    completed = subprocess.run(
        shlex.split(command),
        cwd=checkout,
        check=False,
        capture_output=True,
        text=True,
    )
    assert completed.returncode in {0, 1}
    assert completed.stderr == ""
    return {
        "type": "commandExecution",
        "id": identifier,
        "cwd": str(checkout),
        "exit_code": completed.returncode,
        "command": command,
        "aggregated_output": completed.stdout,
        "command_actions": [{"type": "search", "command": command}],
    }


def test_source_read_telemetry_resolves_checkout_aliases_and_bounds(tmp_path):
    checkout = tmp_path / "repo"
    checkout.mkdir()
    alias = tmp_path / "alias"
    alias.symlink_to(checkout, target_is_directory=True)

    def command(identifier, path, text, exit_code=0):
        return {
            "type": "commandExecution", "id": identifier,
            "cwd": str(alias), "exit_code": exit_code,
            "command_actions": [{"type": "read", "path": path, "command": text}],
        }

    bounded = command("one", "main.go", "sed -n '90,210p' main.go")
    telemetry = codex_agent._source_read_telemetry([
        bounded, bounded,
        command("two", str(checkout / "main.go"), "cat main.go"),
        command("three", "../skill.md", "cat ../skill.md"),
        command("four", "failed.go", "cat failed.go", exit_code=1),
        {"type": "agentMessage", "text": "I read other.go"},
    ], checkout)
    assert telemetry["source_files_read"] == ["main.go"]
    assert telemetry["source_file_count"] == 1
    assert telemetry["source_read_operations"] == 2
    assert telemetry["source_read_ranges"] == [
        {"path": "main.go", "offset": 90, "limit": 121},
        {"path": "main.go", "offset": None, "limit": None},
    ]
    unscoped = codex_agent._source_read_telemetry([bounded], None)
    assert unscoped["source_files_read"] == []
    json.dumps(telemetry)


def test_codex_dependency_observations_classify_direct_rg_and_reject_pipeline(
    tmp_path,
):
    checkout = tmp_path / "repo"
    source = checkout / "src"
    source.mkdir(parents=True)
    (source / "main.go").write_text("// TODO: fixture\n")
    safe_prefix = (
        "rg --no-config --no-ignore-global --color never --sort path "
        "--glob '*.go'"
    )
    direct = _rg_item(checkout, "direct-rg", f"{safe_prefix} TODO src")
    opaque = {
        "type": "commandExecution",
        "id": "opaque-rg",
        "cwd": str(checkout),
        "exit_code": 0,
        "command_actions": [
            {"type": "search", "command": "rg TODO src | head"}
        ],
    }

    no_match = _rg_item(checkout, "no-match-rg", f"{safe_prefix} ABSENT src")
    observed = codex_agent._source_read_telemetry([direct, no_match], checkout)[
        "dependency_observations"
    ]
    assert observed["complete"] is True
    assert observed["searches"] == [
        {
            "tool": "rg",
            "resolved_root": str(source),
            "pattern": "TODO",
            "options": {
                "--glob": "*.go",
                "--no-config": True,
                "--no-ignore-global": True,
                "--color": "never",
                "--sort": "path",
                "argv": [
                    "--no-config", "--no-ignore-global", "--color", "never",
                    "--sort", "path", "--glob", "*.go", "TODO", "src",
                ],
            },
            "outcome": "successful-command-execution",
            "execution_cwd": str(checkout),
            "observed_result_identity": codex_agent.search_result_identity(
                0, direct["aggregated_output"]
            ),
        },
        {
            "tool": "rg",
            "resolved_root": str(source),
            "pattern": "ABSENT",
            "options": {
                "--glob": "*.go",
                "--no-config": True,
                "--no-ignore-global": True,
                "--color": "never",
                "--sort": "path",
                "argv": [
                    "--no-config", "--no-ignore-global", "--color", "never",
                    "--sort", "path", "--glob", "*.go", "ABSENT", "src",
                ],
            },
            "outcome": "successful-no-match-search",
            "execution_cwd": str(checkout),
            "observed_result_identity": codex_agent.search_result_identity(
                1, no_match["aggregated_output"]
            ),
        },
    ]

    raw_only = copy.deepcopy(direct)
    raw_only["id"] = "raw-only-rg"
    raw_only["command_actions"] = []
    raw_observed = codex_agent._source_read_telemetry([raw_only], checkout)[
        "dependency_observations"
    ]
    assert raw_observed["complete"] is True
    assert len(raw_observed["searches"]) == 1
    assert raw_observed["searches"][0]["pattern"] == "TODO"

    incomplete = codex_agent._source_read_telemetry([opaque], checkout)[
        "dependency_observations"
    ]
    assert incomplete["complete"] is False
    assert incomplete["unclassified_source_commands"] == ["rg TODO src | head"]

    ambiguous = copy.deepcopy(direct)
    ambiguous["id"] = "ambiguous-rg"
    ambiguous["command_actions"][0]["command"] = "rg --threads 4 TODO src"
    incomplete = codex_agent._source_read_telemetry([ambiguous], checkout)[
        "dependency_observations"
    ]
    assert incomplete["complete"] is False
    assert incomplete["searches"] == []


@pytest.mark.parametrize(
    ("action_type", "command"),
    [
        ("listFiles", "ls -R src"),
        ("unknown", "git show HEAD:src/a.go"),
        ("unknown", "nl -ba src/a.go"),
        ("unknown", "wc -l src/a.go"),
        ("unknown", "stat src/a.go"),
        ("unknown", "kustomize build config/default"),
    ],
)
def test_codex_unknown_commands_default_dependency_observations_to_incomplete(
    tmp_path, action_type, command
):
    checkout = tmp_path / "repo"
    (checkout / "src").mkdir(parents=True)
    item = {
        "type": "commandExecution",
        "id": command,
        "cwd": str(checkout),
        "exit_code": 0,
        "command_actions": [{"type": action_type, "command": command}],
    }

    observed = codex_agent._source_read_telemetry([item], checkout)[
        "dependency_observations"
    ]

    assert observed["complete"] is False
    assert observed["reads"] == []
    assert observed["searches"] == []
    assert observed["unclassified_source_commands"] == [command]


def test_codex_shell_wrappers_and_missing_execution_records_are_incomplete(
    tmp_path,
):
    checkout = tmp_path / "repo"
    source = checkout / "src"
    source.mkdir(parents=True)
    safe_rg = (
        "rg --no-config --no-ignore-global --color never --sort path -n TODO src"
    )
    wrapped_rg = f"/bin/bash -lc '{safe_rg}'"
    base = {
        "type": "commandExecution",
        "cwd": str(checkout),
        "exit_code": 1,
        "command": wrapped_rg,
        "aggregated_output": "",
    }
    wrapper_shapes = [
        {**base, "id": "empty-actions", "command_actions": []},
        {
            **base,
            "id": "identical-unknown-action",
            "command_actions": [{"type": "unknown", "command": wrapped_rg}],
        },
        {
            **base,
            "id": "identical-search-action",
            "command_actions": [{"type": "search", "command": wrapped_rg}],
        },
        {
            **base,
            "id": "inner-search-action",
            "command_actions": [{"type": "search", "command": safe_rg}],
        },
        {
            **base,
            "id": "recognized-read-action",
            "exit_code": 0,
            "command_actions": [
                {"type": "read", "path": "src/main.go", "command": "cat src/main.go"}
            ],
        },
        {**base, "id": "raw-only"},
        {
            **base,
            "id": "sh-wrapper",
            "command": f"sh -c '{safe_rg}'",
            "command_actions": [],
        },
    ]
    raw_read = {
        "type": "commandExecution",
        "id": "raw-read-wrapper",
        "cwd": str(checkout),
        "exit_code": 0,
        "command": "/bin/bash -lc 'cat src/a.go'",
        "command_actions": [],
    }
    missing_output = {
        "type": "commandExecution",
        "id": "missing-output",
        "cwd": str(checkout),
        "exit_code": 1,
        "command": safe_rg,
        "command_actions": [{"type": "search", "command": safe_rg}],
    }
    missing_cwd = {
        "type": "commandExecution",
        "id": "missing-cwd",
        "exit_code": 1,
        "command": safe_rg,
        "aggregated_output": "",
        "command_actions": [{"type": "search", "command": safe_rg}],
    }

    for item in (*wrapper_shapes, missing_output, missing_cwd):
        observed = codex_agent._source_read_telemetry([item], checkout)[
            "dependency_observations"
        ]
        assert observed["complete"] is False
        assert observed["searches"] == []
        assert observed["unclassified_source_commands"]

    incomplete = codex_agent._source_read_telemetry([raw_read], checkout)[
        "dependency_observations"
    ]
    assert incomplete["complete"] is False
    assert incomplete["unclassified_source_commands"] == [raw_read["command"]]


@pytest.mark.parametrize(
    "suffix",
    [" &", "\ntrue", " $HOME", " ~/source"],
)
def test_codex_rg_shell_expansion_and_composition_tokens_are_not_fake_roots(
    tmp_path, suffix
):
    checkout = tmp_path / "repo"
    checkout.mkdir()
    command = (
        "rg --no-config --no-ignore-global --color never --sort path TODO ."
        + suffix
    )
    item = {
        "type": "commandExecution",
        "id": suffix,
        "cwd": str(checkout),
        "exit_code": 0,
        "command": command,
        "aggregated_output": "",
        "command_actions": [{"type": "search", "command": command}],
    }

    observed = codex_agent._source_read_telemetry([item], checkout)[
        "dependency_observations"
    ]

    assert observed["complete"] is False
    assert observed["searches"] == []
    assert observed["unclassified_source_commands"] == [command]


@pytest.mark.parametrize(
    "command",
    [
        "rg -n TODO src 2>/dev/null",
        "rg TODO < src/a.go",
        "rg -n TODO src > matches.txt",
    ],
)
def test_codex_rg_redirections_are_incomplete_not_fake_roots(tmp_path, command):
    checkout = tmp_path / "repo"
    (checkout / "src").mkdir(parents=True)
    item = {
        "type": "commandExecution",
        "id": command,
        "cwd": str(checkout),
        "exit_code": 0,
        "command_actions": [{"type": "search", "command": command}],
    }

    observed = codex_agent._source_read_telemetry([item], checkout)[
        "dependency_observations"
    ]

    assert observed["complete"] is False
    assert observed["searches"] == []
    assert observed["unclassified_source_commands"] == [command]


def test_codex_composition_unmapped_actions_and_model_text_do_not_create_evidence(
    tmp_path,
):
    checkout = tmp_path / "repo"
    (checkout / "src").mkdir(parents=True)
    composed = {
        "type": "commandExecution",
        "id": "composed",
        "cwd": str(checkout),
        "exit_code": 0,
        "command": "rg TODO src | head",
        "command_actions": [{"type": "search", "command": "rg TODO src"}],
    }
    unmapped = {
        "type": "commandExecution",
        "id": "unmapped",
        "cwd": str(checkout),
        "exit_code": 0,
        "command_actions": [{"type": "unknown"}],
    }
    assertion = {
        "type": "agentMessage",
        "text": "I read src/claimed.go and found no matches.",
    }

    observed = codex_agent._source_read_telemetry(
        [composed, unmapped, assertion], checkout
    )["dependency_observations"]

    assert observed["complete"] is False
    assert observed["reads"] == []
    assert observed["unclassified_source_commands"] == [
        "rg TODO src",
        composed["command"],
        "unmapped-action:unknown",
    ]


@pytest.mark.asyncio
async def test_reporting_failure_is_terminal_and_retains_telemetry(
    tmp_path, monkeypatch,
):
    async def run(*args, **kwargs):
        return {"success": True, "telemetry": {"source_files_read": ["main.go"]}}

    async def report(*args):
        raise TypeError("report serialization failed")

    monkeypatch.setattr(agent_runner, "run_agent", run)
    results = await agent_runner.run_agents_concurrently(
        [{"name": "component", "cwd": str(tmp_path), "prompt": "test"}],
        tmp_path, model=None, max_concurrent=1, on_result=report,
    )
    assert results[0]["success"] is False
    assert results[0]["_postprocessed"] is True
    assert results[0]["telemetry"]["source_files_read"] == ["main.go"]
    assert "post-processing failed" in results[0]["error"]


def test_codex_input_resolves_shared_discovery_skill():
    items = codex_agent._codex_input(
        "/discover-components --platform=rhoai-3.6-ea.2 --force",
        enable_skills=True,
    )

    assert isinstance(items, list)
    assert isinstance(items[0], SkillInput)
    assert items[0].name == "discover-components"
    assert items[0].path.endswith("/.claude/skills/discover-components/SKILL.md")
    assert isinstance(items[1], TextInput)
    assert "--platform=rhoai-3.6-ea.2" in items[1].text


def test_terminal_renderer_streams_messages_and_concise_commands(capsys):
    renderer = codex_agent._TerminalEventRenderer("discovery")
    renderer.render(_event(
        "item/agentMessage/delta",
        SimpleNamespace(item_id="message-1", delta="Checking "),
    ))
    renderer.render(_event(
        "item/agentMessage/delta",
        SimpleNamespace(item_id="message-1", delta="repositories."),
    ))
    renderer.render(_event(
        "item/completed",
        SimpleNamespace(item={
            "id": "message-1",
            "type": "agentMessage",
            "text": "Checking repositories.",
        }),
    ))
    renderer.render(_event(
        "item/started",
        SimpleNamespace(item={
            "type": "commandExecution",
            "command": "rg --files checkouts",
        }),
    ))
    renderer.render(_event(
        "item/commandExecution/outputDelta",
        SimpleNamespace(delta="large command output that stays in JSONL"),
    ))
    renderer.render(_event(
        "item/completed",
        SimpleNamespace(item={
            "type": "commandExecution",
            "status": "completed",
            "exit_code": 0,
            "duration_ms": 42,
        }),
    ))

    output = capsys.readouterr().out
    assert "[discovery] Checking repositories.\n" in output
    assert "[discovery] command: rg --files checkouts\n" in output
    assert "[discovery] command completed; exit=0; 42ms\n" in output
    assert "large command output" not in output


@pytest.mark.asyncio
async def test_agent_runner_dispatches_to_codex_harness(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
):
    captured = {}
    inventory = tmp_path / "SURFACE_INVENTORY.json"
    coverage = tmp_path / "SURFACE_COVERAGE.json"
    inventory.write_text("{}\n")

    async def fake_run_codex_agent(**kwargs):
        captured.update(kwargs)
        return {"name": kwargs["name"], "success": True}

    monkeypatch.setattr(codex_agent, "run_codex_agent", fake_run_codex_agent)

    result = await agent_runner.run_agent(
        "discovery",
        str(tmp_path),
        "/discover-components --platform=rhoai-3.6-ea.2",
        tmp_path,
        model=None,
        enable_skills=True,
        harness="codex",
        checkout_path=tmp_path / "checkout",
        input_paths=(inventory,),
        output_paths=(coverage,),
    )

    assert result["success"] is True
    assert captured["model"] is None
    assert captured["enable_skills"] is True
    assert captured["checkout_path"] == tmp_path / "checkout"
    assert captured["input_paths"] == (inventory,)
    assert captured["output_paths"] == (coverage,)


@pytest.mark.asyncio
async def test_codex_agent_uses_workspace_sandbox_and_normalizes_result(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
):
    captured = {}

    class FakeTurn:
        id = "turn-1"

        async def stream(self):
            yield _event("item/completed", SimpleNamespace(
                turn_id=self.id,
                item={
                    "type": "commandExecution", "id": "read-1",
                    "cwd": str(tmp_path), "exit_code": 0,
                    "command_actions": [{
                        "type": "read", "path": "main.go",
                        "command": "sed -n '1,20p' main.go",
                    }],
                },
            ))
            yield SimpleNamespace(
                method="thread/tokenUsage/updated",
                payload=SimpleNamespace(
                    turn_id=self.id,
                    token_usage={"input_tokens": 10, "output_tokens": 20},
                ),
            )
            yield SimpleNamespace(
                method="turn/completed",
                payload=SimpleNamespace(
                    turn=SimpleNamespace(
                        id=self.id,
                        status=TurnStatus.completed,
                        error=None,
                        duration_ms=123,
                    )
                ),
            )

    class FakeThread:
        id = "thread-1"

        async def turn(self, agent_input):
            captured["input"] = agent_input
            return FakeTurn()

    class FakeCodex:
        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, traceback):
            return False

        async def thread_start(self, **kwargs):
            captured["thread_start"] = kwargs
            return FakeThread()

    monkeypatch.setattr(codex_agent, "AsyncCodex", FakeCodex)

    result = await codex_agent.run_codex_agent(
        name="discovery",
        cwd=str(tmp_path),
        prompt="/discover-components --platform=rhoai-3.6-ea.2",
        log_dir=tmp_path / "logs",
        model=None,
        enable_skills=True,
        progress=None,
        strace_dir=None,
        checkout_path=tmp_path,
    )

    assert result["success"] is True
    assert result["telemetry"]["harness"] == "codex"
    assert result["telemetry"]["turn_status"] == "completed"
    assert result["telemetry"]["source_files_read"] == ["main.go"]
    assert result["telemetry"]["source_read_observation"] == (
        "successful-sdk-read-actions"
    )
    json.dumps(result)  # Reports serialize without a permissive default=str.
    assert captured["thread_start"]["model"] is None
    assert captured["thread_start"]["ephemeral"] is True
    assert captured["thread_start"]["sandbox"].value == "workspace-write"
    assert isinstance(captured["input"][0], SkillInput)
    assert "turn-1" in (tmp_path / "logs" / "discovery.log").read_text()


@pytest.mark.asyncio
async def test_codex_restores_mutated_planning_input_and_keeps_sidecar_output(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
):
    inventory = tmp_path / "SURFACE_INVENTORY.json"
    coverage = tmp_path / "SURFACE_COVERAGE.json"
    original = '{"inventory_id":"trusted"}\n'
    inventory.write_text(original)

    class FakeTurn:
        id = "turn-integrity"

        async def stream(self):
            yield SimpleNamespace(
                method="turn/completed",
                payload=SimpleNamespace(
                    turn=SimpleNamespace(
                        id=self.id,
                        status=TurnStatus.completed,
                        error=None,
                        duration_ms=1,
                    )
                ),
            )

    class FakeThread:
        id = "thread-integrity"

        async def turn(self, _agent_input):
            inventory.write_text('{"inventory_id":"mutated"}\n')
            coverage.write_text('{"surfaces":[]}\n')
            return FakeTurn()

    class FakeCodex:
        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, traceback):
            return False

        async def thread_start(self, **_kwargs):
            return FakeThread()

    monkeypatch.setattr(codex_agent, "AsyncCodex", FakeCodex)

    result = await codex_agent.run_codex_agent(
        name="integrity",
        cwd=str(tmp_path),
        prompt="write coverage",
        log_dir=tmp_path / "logs",
        model=None,
        enable_skills=False,
        progress=None,
        strace_dir=None,
        checkout_path=tmp_path,
        input_paths=(inventory,),
        output_paths=(coverage,),
    )

    assert result["success"] is False
    assert "mutated immutable planning input" in result["error"]
    assert result["telemetry"]["immutable_input_check"] == "failed-restored"
    assert result["telemetry"]["mutated_input_paths"] == [str(inventory)]
    assert inventory.read_text() == original
    assert json.loads(coverage.read_text()) == {"surfaces": []}


@pytest.mark.asyncio
async def test_codex_agent_flushes_events_before_turn_completion(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
):
    waiting = asyncio.Event()
    release = asyncio.Event()

    class FakeTurn:
        id = "turn-stream"

        async def stream(self):
            yield SimpleNamespace(
                method="item/started",
                payload={"turnId": self.id, "item": {"type": "command"}},
            )
            waiting.set()
            await release.wait()
            yield SimpleNamespace(
                method="turn/completed",
                payload=SimpleNamespace(
                    turn=SimpleNamespace(
                        id=self.id,
                        status="completed",
                        error=None,
                        duration_ms=1,
                    )
                ),
            )

    class FakeThread:
        id = "thread-stream"

        async def turn(self, _agent_input):
            return FakeTurn()

    class FakeCodex:
        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, traceback):
            return False

        async def thread_start(self, **_kwargs):
            return FakeThread()

    monkeypatch.setattr(codex_agent, "AsyncCodex", FakeCodex)

    run = asyncio.create_task(codex_agent.run_codex_agent(
        name="streaming",
        cwd=str(tmp_path),
        prompt="plain prompt",
        log_dir=tmp_path / "logs",
        model=None,
        enable_skills=False,
        progress=None,
        strace_dir=None,
    ))
    await waiting.wait()

    in_progress_log = (tmp_path / "logs" / "streaming.log").read_text()
    assert '"method":"item/started"' in in_progress_log

    release.set()
    result = await run
    assert result["success"] is True


@pytest.mark.asyncio
async def test_codex_agent_propagates_cancellation(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
):
    class FakeTurn:
        id = "turn-cancelled"

        async def stream(self):
            raise asyncio.CancelledError
            yield  # pragma: no cover

    class FakeThread:
        id = "thread-cancelled"

        async def turn(self, _agent_input):
            return FakeTurn()

    class FakeCodex:
        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, traceback):
            return False

        async def thread_start(self, **_kwargs):
            return FakeThread()

    monkeypatch.setattr(codex_agent, "AsyncCodex", FakeCodex)

    with pytest.raises(asyncio.CancelledError):
        await codex_agent.run_codex_agent(
            name="cancelled",
            cwd=str(tmp_path),
            prompt="plain prompt",
            log_dir=tmp_path / "logs",
            model=None,
            enable_skills=False,
            progress=None,
            strace_dir=None,
        )

    log = (tmp_path / "logs" / "cancelled.log").read_text()
    assert '"error_type": "CancelledError"' in log
