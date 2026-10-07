"""Keep unit tests from starting authenticated model transports accidentally."""

import pytest
from claude_agent_sdk._internal.transport.subprocess_cli import SubprocessCLITransport
from openai_codex.client import CodexClient


@pytest.fixture(autouse=True)
def block_live_model_transports(monkeypatch):
    """Scenario stubs may replace clients; an omitted stub must fail before I/O.

    This guards SDK startup in this pytest process, including async Codex's
    underlying synchronous client. It does not intercept separate Python
    processes or unrelated network clients such as the local MLflow test server.
    """

    def reject_codex_start(*_args, **_kwargs):
        pytest.fail("Offline test attempted Codex transport startup; install a stub")

    async def reject_claude_connect(*_args, **_kwargs):
        pytest.fail("Offline test attempted Claude transport startup; install a stub")

    monkeypatch.setattr(CodexClient, "start", reject_codex_start)
    monkeypatch.setattr(SubprocessCLITransport, "connect", reject_claude_connect)
