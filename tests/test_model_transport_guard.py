"""Check the default guard at the real SDK process-start boundaries."""

import pytest
from claude_agent_sdk import ClaudeAgentOptions
from claude_agent_sdk._internal.transport.subprocess_cli import SubprocessCLITransport
from openai_codex.client import CodexClient


def test_codex_start_is_blocked_before_process_creation():
    client = CodexClient()
    assert client._proc is None
    with pytest.raises(pytest.fail.Exception, match="Codex transport startup"):
        client.start()
    assert client._proc is None


@pytest.mark.asyncio
async def test_claude_connect_is_blocked_before_process_creation():
    transport = SubprocessCLITransport(
        "No provider call is authorized by this test",
        ClaudeAgentOptions(cli_path="/unused/test-only-claude"),
    )
    assert transport._process is None
    with pytest.raises(pytest.fail.Exception, match="Claude transport startup"):
        await transport.connect()
    assert transport._process is None
