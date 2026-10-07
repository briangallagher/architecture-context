"""Codex SDK implementation of the pipeline agent contract."""

from __future__ import annotations

import asyncio
import dataclasses
import hashlib
import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import tempfile
import time
import tomllib
from dataclasses import dataclass
from enum import Enum
from pathlib import Path
from typing import TYPE_CHECKING, Any

from openai_codex import (
    ApprovalMode,
    AsyncCodex,
    CodexConfig,
    Sandbox,
    SkillInput,
    TextInput,
)
from openai_codex.api import AsyncThread
from openai_codex.errors import JsonRpcError
from openai_codex.generated.v2_all import (
    AskForApproval,
    AskForApprovalValue,
    ConfigReadResponse,
    SandboxMode,
    ThreadStartParams,
)

from lib.context_telemetry import (
    dependency_observations,
    parse_bounded_rg_invocation,
    search_result_identity,
)
from lib.skill_paths import resolve_skill_file
from lib.telemetry_redact import redact_dict, redact_value

if TYPE_CHECKING:
    from lib.progress import AgentProgress

_SKILL_PROMPT = re.compile(r"^/([A-Za-z0-9][A-Za-z0-9._-]*)(?:\s+(.*))?$", re.DOTALL)
_PROVABLY_NON_READING_COMMANDS = frozenset({":", "false", "pwd", "true"})
_STRUCTURED_BASE_INSTRUCTIONS = (
    "Return one final answer matching the supplied JSON schema. Do not invoke "
    "tools, request approvals, read files, use network access, or delegate."
)
_STRUCTURED_DEVELOPER_INSTRUCTIONS = (
    "The parent supplied every allowed semantic input in the user message. "
    "A tool lifecycle event causes rejection and a best-effort interruption request."
)
_STRUCTURED_PASSIVE_ITEM_TYPES = frozenset({
    "agentMessage",
    "plan",
    "reasoning",
    "userMessage",
})
_CODEX_STAGED_MODEL_CONFIG_KEYS = (
    "model",
    "model_auto_compact_token_limit",
    "model_auto_compact_token_limit_scope",
    "model_context_window",
    "model_provider",
    "model_reasoning_effort",
    "model_reasoning_summary",
    "model_verbosity",
    "service_tier",
)
_CODEX_CONTEXT_PROTOCOL = "structured-codex-local-context/v1"


@dataclass(slots=True)
class _StreamedTurnResult:
    """Subset of the SDK turn result consumed by the pipeline."""

    id: str
    status: object
    error: object | None
    duration_ms: int | None
    final_response: str | None
    items: list[object]
    usage: object | None
    provider_events: list[dict[str, Any]]
    model_reroutes: list[dict[str, Any]]


class _CodexTurnFailed(RuntimeError):
    def __init__(self, message: str, details: dict[str, Any]) -> None:
        super().__init__(message)
        self.details = details


class _CodexToolActivityRejected(RuntimeError):
    def __init__(self, activity: dict[str, Any]) -> None:
        super().__init__(
            "bounded structured Codex route rejected observed tool activity"
        )
        self.activity = activity


class _CodexModelRerouteRejected(RuntimeError):
    def __init__(self, reroute: dict[str, Any]) -> None:
        super().__init__(
            "bounded structured Codex route rejected a producing-model reroute"
        )
        self.reroute = reroute


class _CodexModelSelectionRejected(RuntimeError):
    def __init__(self, details: dict[str, Any]) -> None:
        requested = details.get("parent_requested_model")
        resolved = details.get("resolved_model_identity")
        super().__init__(
            "bounded structured Codex route rejected requested/resolved model "
            f"mismatch: requested {requested!r}, resolved {resolved!r}"
        )
        self.details = details


class _CodexContextRejected(RuntimeError):
    def __init__(self, message: str, details: dict[str, Any]) -> None:
        super().__init__(message)
        self.details = details


class _TerminalEventRenderer:
    """Render useful Codex events without exposing raw protocol noise."""

    def __init__(self, name: str):
        self.name = name
        self._message_ids: set[str] = set()
        self._open_message_id: str | None = None

    def render(self, event) -> None:
        payload = event.payload
        if event.method == "item/agentMessage/delta":
            self._render_message_delta(payload)
            return
        if event.method not in {"item/started", "item/completed"}:
            return

        item = _jsonable(getattr(payload, "item", None))
        if not isinstance(item, dict):
            return
        item_type = item.get("type")
        if item_type == "agentMessage" and event.method == "item/completed":
            self._finish_message(item)
        elif item_type == "commandExecution":
            self._render_command(event.method, item)

    def _render_message_delta(self, payload) -> None:
        item_id = str(getattr(payload, "item_id", ""))
        delta = getattr(payload, "delta", "")
        if not isinstance(delta, str) or not delta:
            return
        if self._open_message_id != item_id:
            self._ensure_newline()
            sys.stdout.write(f"[{self.name}] ")
            self._open_message_id = item_id
        self._message_ids.add(item_id)
        sys.stdout.write(delta)
        sys.stdout.flush()

    def _finish_message(self, item: dict) -> None:
        item_id = str(item.get("id", ""))
        if item_id not in self._message_ids:
            text = item.get("text")
            if isinstance(text, str) and text:
                self._ensure_newline()
                sys.stdout.write(f"[{self.name}] {text}")
        if self._open_message_id == item_id or item_id not in self._message_ids:
            self._ensure_newline()

    def _render_command(self, method: str, item: dict) -> None:
        self._ensure_newline()
        if method == "item/started":
            command = " ".join(str(item.get("command", "")).split())
            if len(command) > 180:
                command = f"{command[:177]}..."
            print(f"[{self.name}] command: {command}", flush=True)
            return
        status = item.get("status", "completed")
        exit_code = item.get("exit_code")
        duration_ms = item.get("duration_ms")
        details = [str(status)]
        if exit_code is not None:
            details.append(f"exit={exit_code}")
        if duration_ms is not None:
            details.append(f"{duration_ms}ms")
        print(f"[{self.name}] command {'; '.join(details)}", flush=True)

    def _ensure_newline(self) -> None:
        if self._open_message_id is not None:
            sys.stdout.write("\n")
            sys.stdout.flush()
            self._open_message_id = None


def _codex_input(
    prompt: str, enable_skills: bool, cwd: str | None = None,
) -> str | list[object]:
    """Translate the pipeline's slash-skill convention to Codex SDK input."""
    if not enable_skills:
        return prompt
    match = _SKILL_PROMPT.match(prompt.strip())
    if not match:
        raise ValueError(
            "Skill-enabled Codex prompts must start with /<skill-name>"
        )
    name, arguments = match.groups()
    staged_skill = (
        Path(cwd) / ".agents" / "skills" / name / "SKILL.md" if cwd else None
    )
    skill_file = (
        staged_skill if staged_skill and staged_skill.is_file()
        else resolve_skill_file(name)
    ).resolve()
    items: list[object] = [
        SkillInput(name=name, path=str(skill_file))
    ]
    items.append(TextInput(text=(
        f"Execute the attached {name} skill now. "
        f"First read the complete skill file at {skill_file}.\n"
        f"Its supporting scripts and references are under {skill_file.parent}. "
        "Resolve relative skill references against that directory, and use it "
        "where the skill refers to CLAUDE_SKILL_DIR.\n"
        "Perform its workflow directly. You are a pipeline worker. "
        "The following are skill arguments and task context:\n"
        f"{arguments or ''}"
    )))
    return items


def _jsonable(value: Any) -> Any:
    if isinstance(value, Enum):
        return _jsonable(value.value)
    if dataclasses.is_dataclass(value):
        return _jsonable(dataclasses.asdict(value))
    if hasattr(value, "model_dump"):
        return value.model_dump(mode="json")
    if isinstance(value, dict):
        return {str(key): _jsonable(item) for key, item in value.items()}
    if isinstance(value, (list, tuple)):
        return [_jsonable(item) for item in value]
    if hasattr(value, "__dict__"):
        return _jsonable(vars(value))
    if hasattr(value, "value"):
        return value.value
    return value


def _canonical_identity(value: object) -> str:
    payload = json.dumps(
        value,
        sort_keys=True,
        separators=(",", ":"),
        ensure_ascii=True,
    ).encode()
    return f"sha256:{hashlib.sha256(payload).hexdigest()}"


def _file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return f"sha256:{digest.hexdigest()}"


def resolve_codex_cli_identity() -> tuple[str, dict[str, str]]:
    """Resolve and pin the SDK-bundled CLI without starting an app server."""

    from codex_cli_bin import bundled_codex_path

    path = bundled_codex_path().resolve()
    if not path.is_file():
        raise FileNotFoundError(f"SDK-bundled Codex CLI is missing: {path}")
    with tempfile.TemporaryDirectory(prefix="structured-codex-version-") as raw:
        environment = os.environ.copy()
        environment["CODEX_HOME"] = raw
        completed = subprocess.run(
            [str(path), "--version"],
            check=False,
            capture_output=True,
            text=True,
            timeout=10,
            env=environment,
        )
    version_output = (completed.stdout or completed.stderr).strip()
    if completed.returncode != 0 or not version_output:
        raise RuntimeError(
            "cannot identify SDK-bundled Codex CLI: "
            f"exit {completed.returncode}"
        )
    return str(path), {
        "implementation": "codex-cli",
        "version_output": version_output,
        "binary_sha256": _file_sha256(path),
    }


def _toml_scalar(value: object, key: str) -> str:
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, int) and not isinstance(value, bool):
        return str(value)
    if isinstance(value, str):
        return json.dumps(value, ensure_ascii=True)
    raise _CodexContextRejected(
        f"unsupported Codex model-setting type for {key}",
        {"key": key, "value_type": type(value).__name__},
    )


def _source_codex_model_config() -> tuple[Path, dict[str, object]]:
    configured_home = os.environ.get("CODEX_HOME")
    source_home = (
        Path(configured_home).expanduser()
        if configured_home
        else Path.home() / ".codex"
    ).resolve()
    config_path = source_home / "config.toml"
    if not config_path.is_file():
        return source_home, {}
    try:
        source = tomllib.loads(config_path.read_text())
    except (OSError, tomllib.TOMLDecodeError) as exc:
        raise _CodexContextRejected(
            "cannot read the existing Codex model configuration",
            {"error_type": type(exc).__name__},
        ) from exc
    selected = {
        key: source[key]
        for key in _CODEX_STAGED_MODEL_CONFIG_KEYS
        if key in source and source[key] is not None
    }
    for key, value in selected.items():
        _toml_scalar(value, key)
    provider = selected.get("model_provider")
    providers = source.get("model_providers")
    if (
        isinstance(provider, str)
        and isinstance(providers, dict)
        and provider in providers
    ):
        raise _CodexContextRejected(
            "custom Codex provider configuration cannot be staged without "
            "copying provider internals",
            {"model_provider": provider},
        )
    return source_home, selected


def _isolated_codex_config(
    private_home: Path,
    *,
    cli_path: str,
) -> tuple[CodexConfig, dict[str, Any]]:
    """Stage only existing authentication and selected model settings."""

    source_home, selected = _source_codex_model_config()
    auth_source = source_home / "auth.json"
    if not auth_source.is_file():
        raise _CodexContextRejected(
            "existing Codex auth.json is unavailable for isolated staging",
            {"auth_route": "existing-auth-json-required"},
        )
    private_home.mkdir(mode=0o700, parents=True, exist_ok=True)
    auth_target = private_home / "auth.json"
    shutil.copyfile(auth_source, auth_target)
    auth_target.chmod(0o600)
    overrides = tuple(
        f"{key}={_toml_scalar(value, key)}"
        for key, value in sorted(selected.items())
    )
    environment = {"CODEX_HOME": str(private_home)}
    config = CodexConfig(
        codex_bin=cli_path,
        config_overrides=overrides,
        env=environment,
    )
    staged = {
        "protocol": _CODEX_CONTEXT_PROTOCOL,
        "auth_route": "private-copy-of-existing-auth-json",
        "staged_model_config": selected,
        "staged_model_config_identity": _canonical_identity(selected),
        "suppressed_local_sources": [
            "config-keys-outside-staged-model-settings",
            "memories",
            "plugins",
            "skills",
            "rules",
            "project-dot-codex",
        ],
        "environment_access": "disabled-by-empty-thread-environments",
    }
    return config, staged


async def _effective_codex_config(codex: object, cwd: str) -> dict[str, Any]:
    client = getattr(codex, "_client", None)
    if client is None or not hasattr(client, "request"):
        raise _CodexContextRejected(
            "bounded structured Codex route requires config/read support",
            {"required_method": "config/read"},
        )
    response = await client.request(
        "config/read",
        {"cwd": str(Path(cwd).resolve()), "includeLayers": True},
        response_model=ConfigReadResponse,
    )
    config = _jsonable(getattr(response, "config", None))
    if not isinstance(config, dict):
        raise _CodexContextRejected(
            "Codex config/read did not return an effective configuration",
            {"required_method": "config/read"},
        )
    return {
        "effective_config_identity": _canonical_identity(config),
        "effective_model_config": {
            key: config.get(key) for key in _CODEX_STAGED_MODEL_CONFIG_KEYS
        },
    }


def _source_read_telemetry(
    items: list[object], checkout_path: str | Path | None,
) -> dict:
    """Observe SDK-classified reads; do not infer evidence from agent claims.

    Unknown commands and uncertain line bounds are deliberately not guessed.
    This is telemetry, not enforcement of Claude's read-budget hooks.
    """
    checkout = Path(checkout_path).resolve() if checkout_path else None
    files: list[str] = []
    ranges: list[dict] = []
    commands = 0
    seen: set[str] = set()
    observed_reads: list[dict[str, object]] = []
    searches: list[dict[str, object]] = []
    unclassified: list[str] = []
    complete = checkout is not None
    for raw in items:
        item = _jsonable(raw)
        if not isinstance(item, dict) or item.get("type") != "commandExecution":
            continue
        item_id = item.get("id")
        if item_id and item_id in seen:
            continue
        if item_id:
            seen.add(item_id)
        commands += 1
        if checkout is None:
            continue
        exit_code = item.get("exit_code")
        actions = item.get("command_actions", ())
        if not isinstance(actions, (list, tuple)):
            complete = False
            unclassified.append("malformed-command-actions")
            actions = ()
        raw_command = item.get("command")
        raw_command = raw_command.strip() if isinstance(raw_command, str) else ""
        for action in actions:
            if not isinstance(action, dict):
                complete = False
                unclassified.append("malformed-command-action")
                continue
            command = str(action.get("command") or "").strip()
            classified = False
            if (
                exit_code == 0
                and action.get("type") == "read"
                and action.get("path")
                and not _has_unsupported_shell_syntax(command)
            ):
                path = Path(action["path"])
                if not path.is_absolute():
                    if not item.get("cwd"):
                        complete = False
                        unclassified.append(command or "relative-read-without-cwd")
                        continue
                    path = Path(item["cwd"]) / path
                try:
                    relative = path.resolve().relative_to(checkout).as_posix()
                except ValueError:
                    complete = False
                    unclassified.append(command or f"read-outside-checkout:{path}")
                    continue
                if relative not in files:
                    files.append(relative)
                match = re.fullmatch(
                    r"sed -n ['\"]?(\d+),(\d+)p['\"]? [^|;&\n]+", command
                )
                offset, limit = None, None
                if match:
                    first, last = map(int, match.groups())
                    if 0 < first <= last:
                        offset, limit = first, last - first + 1
                record = {"path": relative, "offset": offset, "limit": limit}
                ranges.append(record)
                observed_reads.append(
                    {**record, "outcome": "successful-sdk-read-action"}
                )
                classified = True
            rg_searches = (
                _codex_rg_searches(
                    command, cwd=item.get("cwd"), checkout=checkout
                )
                if not classified and action.get("type") == "search"
                else None
            )
            search_context_complete = (
                raw_command == command
                and isinstance(item.get("cwd"), str)
                and isinstance(item.get("aggregated_output"), str)
            )
            if (
                rg_searches is not None
                and exit_code in {0, 1}
                and search_context_complete
            ):
                outcome = (
                    "successful-command-execution"
                    if exit_code == 0
                    else "successful-no-match-search"
                )
                for search in rg_searches:
                    search["outcome"] = outcome
                    search["execution_cwd"] = str(Path(str(item["cwd"])).resolve())
                    search["observed_result_identity"] = search_result_identity(
                        exit_code, item["aggregated_output"]
                    )
                searches.extend(rg_searches)
                classified = True
            if (
                not classified
                and exit_code == 0
                and _is_provably_non_reading_command(command)
            ):
                classified = True
            if not classified:
                complete = False
                action_type = str(action.get("type") or "unknown")
                unclassified.append(command or f"unmapped-action:{action_type}")
        if actions and raw_command and _has_unsupported_shell_syntax(raw_command):
            # Command actions are not guaranteed to describe every branch of a
            # composed shell command. Never let a partial action mapping make
            # the enclosing execution look complete.
            complete = False
            unclassified.append(raw_command)
        if not actions:
            parsed = _codex_rg_searches(
                raw_command, cwd=item.get("cwd"), checkout=checkout
            )
            search_context_complete = (
                isinstance(item.get("cwd"), str)
                and isinstance(item.get("aggregated_output"), str)
            )
            if (
                parsed is not None
                and exit_code in {0, 1}
                and search_context_complete
            ):
                outcome = (
                    "successful-command-execution"
                    if exit_code == 0
                    else "successful-no-match-search"
                )
                for search in parsed:
                    search["outcome"] = outcome
                    search["execution_cwd"] = str(Path(str(item["cwd"])).resolve())
                    search["observed_result_identity"] = search_result_identity(
                        exit_code, item["aggregated_output"]
                    )
                searches.extend(parsed)
            elif not (
                exit_code == 0
                and _is_provably_non_reading_command(raw_command)
            ):
                complete = False
                unclassified.append(
                    raw_command or "command-execution-without-classifiable-command"
                )
    return {
        "source_read_observation": "successful-sdk-read-actions",
        "tool_calls": commands,
        "tool_calls_by_name": {"commandExecution": commands},
        "tool_calls_by_activity": {"targeted_source_read": len(ranges)},
        "source_files_read": files,
        "source_file_count": len(files),
        "source_read_operations": len(ranges),
        "source_read_ranges": ranges,
        "dependency_observations": dependency_observations(
            harness="codex",
            reads=observed_reads,
            searches=searches,
            complete=complete,
            unclassified_source_commands=list(dict.fromkeys(unclassified)),
        ),
    }


def _has_unsupported_shell_syntax(command: str) -> bool:
    """Return whether exact command provenance is obscured by shell syntax.

    This intentionally checks the source string rather than attempting shell
    evaluation. It therefore retains the known conservative false miss for a
    quoted ``rg`` alternation containing ``|``.
    """

    if not command:
        return False
    try:
        words = shlex.split(command)
    except ValueError:
        return True
    shell_wrapper = (
        len(words) >= 2
        and Path(words[0]).name in {"bash", "sh"}
        and words[1] in {"-c", "-lc"}
    )
    return shell_wrapper or any(
        token in command
        for token in ("|", ";", "&", "`", "$", ">", "<", "\n", "\r")
    )


def _is_provably_non_reading_command(command: str) -> bool:
    if not command or _has_unsupported_shell_syntax(command):
        return False
    try:
        words = shlex.split(command)
    except ValueError:
        return False
    return len(words) == 1 and words[0] in _PROVABLY_NON_READING_COMMANDS


def _codex_rg_searches(
    command: str,
    *,
    cwd: object,
    checkout: Path,
) -> list[dict[str, object]] | None:
    """Classify a direct, unpiped rg command or return ``None`` conservatively."""

    if not command or _has_unsupported_shell_syntax(command):
        return None
    try:
        words = shlex.split(command)
    except ValueError:
        return None
    parsed = parse_bounded_rg_invocation(
        words,
        execution_cwd=str(cwd) if cwd else str(checkout),
        checkout_root=checkout,
    )
    if parsed is None:
        return None
    result: list[dict[str, object]] = []
    for resolved in parsed["resolved_roots"]:
        result.append(
            {
                "tool": "rg",
                "resolved_root": resolved,
                "pattern": parsed["pattern"],
                "options": parsed["options"],
                "outcome": "successful-command-execution",
            }
        )
    return result


def _snapshot_immutable_inputs(
    paths: tuple[str | Path, ...],
) -> dict[Path, bytes]:
    snapshots: dict[Path, bytes] = {}
    for raw_path in paths:
        path = Path(raw_path).resolve()
        if not path.is_file():
            raise FileNotFoundError(f"immutable planning input is missing: {path}")
        snapshots[path] = path.read_bytes()
    return snapshots


def _restore_mutated_inputs(snapshots: dict[Path, bytes]) -> list[str]:
    mutated: list[str] = []
    for path, original in snapshots.items():
        try:
            current = path.read_bytes()
        except OSError:
            current = None
        if current == original:
            continue
        mutated.append(str(path))
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(original)
    return mutated


def _value(value: object) -> object:
    """Return an enum's wire value while preserving plain values."""
    return getattr(value, "value", value)


def _final_response(items: list[object]) -> str | None:
    """Extract the last agent response without depending on generated types."""
    fallback = None
    for item in reversed(items):
        item_data = _jsonable(item)
        if not isinstance(item_data, dict):
            continue
        item_type = item_data.get("type")
        text = item_data.get("text")
        phase = item_data.get("phase")
        if item_type != "agentMessage" or not isinstance(text, str):
            continue
        if phase in {"finalAnswer", "final_answer"}:
            return text
        if phase is None and fallback is None:
            fallback = text
    return fallback


def _payload_field(payload: object, name: str, default: object = None) -> object:
    if isinstance(payload, dict):
        if name in payload:
            return payload[name]
        words = name.split("_")
        alias = words[0] + "".join(word.title() for word in words[1:])
        return payload.get(alias, default)
    value = getattr(payload, name, default)
    if value is not default:
        return value
    params = getattr(payload, "params", None)
    if isinstance(params, dict):
        return _payload_field(params, name, default)
    return default


def _is_tool_item(item: object) -> bool:
    """Reject every lifecycle item not known to be passive model output."""

    value = _jsonable(item)
    if not isinstance(value, dict):
        return True
    item_type = value.get("type")
    return not (
        isinstance(item_type, str)
        and item_type in _STRUCTURED_PASSIVE_ITEM_TYPES
    )


async def _request_interrupt(turn: object, activity: dict[str, Any]) -> None:
    """Request best-effort interruption without weakening the rejection."""

    interruption = {
        "attempted": True,
        "request_succeeded": False,
    }
    activity["interruption"] = interruption
    try:
        await turn.interrupt()
    except Exception as exc:
        interruption.update({
            "error_type": type(exc).__name__,
            "error": str(exc) or repr(exc),
        })
    else:
        interruption["request_succeeded"] = True


_CODEX_RATE_LIMIT_CODES = frozenset({
    "ratelimit",
    "ratelimiterror",
    "ratelimitreached",
    "sessionbudgetexceeded",
    "usagelimit",
    "usagelimitexceeded",
    "workspacememberusagelimitreached",
    "workspaceownerusagelimitreached",
})
_CODEX_RATE_LIMIT_CODE_FIELDS = frozenset({
    "code",
    "codexerrorinfo",
    "errorinfo",
    "errorcode",
    "type",
})
_CODEX_ERROR_INFO_FIELDS = frozenset({"codexerrorinfo", "errorinfo"})
_CODEX_HTTP_STATUS_FIELDS = frozenset({"httpstatuscode", "statuscode"})
_CODEX_ERROR_CODE_TOKEN = re.compile(r"[a-z][a-z0-9_-]*", re.IGNORECASE)


def _normalized_error_code(value: object) -> str:
    return re.sub(r"[^a-z0-9]", "", str(value).casefold())


def _known_rate_limit_code(value: object) -> bool:
    return (
        isinstance(value, str)
        and _CODEX_ERROR_CODE_TOKEN.fullmatch(value) is not None
        and _normalized_error_code(value) in _CODEX_RATE_LIMIT_CODES
    )


def _codex_rate_limit_denied(value: object) -> bool:
    """Recognize structured quota refusal codes or an actual HTTP 429.

    Message fields are deliberately not classified. A provider exception must
    expose a known code in structured metadata or an HTTP status of 429.
    """

    def contains_structured_denial(
        data: object,
        *,
        allow_code_value: bool,
    ) -> bool:
        if isinstance(data, dict):
            for key, item in data.items():
                normalized_key = _normalized_error_code(key)
                if normalized_key in _CODEX_HTTP_STATUS_FIELDS and item == 429:
                    return True
                if (
                    normalized_key in _CODEX_RATE_LIMIT_CODE_FIELDS
                    and _known_rate_limit_code(item)
                ):
                    return True
                if normalized_key == "data" and _known_rate_limit_code(item):
                    return True
                child_allows_code = (
                    allow_code_value
                    or normalized_key in _CODEX_ERROR_INFO_FIELDS
                )
                if isinstance(item, (dict, list)) and contains_structured_denial(
                    item,
                    allow_code_value=child_allows_code,
                ):
                    return True
                if child_allows_code and _known_rate_limit_code(item):
                    return True
            return False
        if isinstance(data, list):
            return any(
                contains_structured_denial(
                    item,
                    allow_code_value=allow_code_value,
                )
                for item in data
            )
        return allow_code_value and _known_rate_limit_code(data)

    data = _jsonable(value)
    return contains_structured_denial(
        data,
        allow_code_value=not isinstance(data, (dict, list)),
    )


def _sanitized_provider_value(value: object) -> object:
    """Make provider metadata JSON-safe while redacting credential material."""

    data = _jsonable(value)
    if isinstance(data, dict):
        return redact_dict(data)
    if isinstance(data, list):
        return redact_dict({"value": data})["value"]
    return redact_value("value", data)


def codex_exception_details(error: BaseException) -> dict[str, Any]:
    """Retain safe structured SDK failure evidence for durable audit."""

    error_text = str(error) or repr(error)
    details: dict[str, Any] = {
        "kind": "codex-exception",
        "error_type": type(error).__name__,
        "message": redact_value("message", error_text),
    }
    response = getattr(error, "response", None)
    status_code = getattr(error, "status_code", None)
    if status_code is None and response is not None:
        status_code = getattr(response, "status_code", None)
    if isinstance(status_code, int):
        details["status_code"] = status_code
    if isinstance(error, JsonRpcError):
        details.update({
            "kind": "codex-rpc-error",
            "code": error.code,
            "message": redact_value("message", error.message),
            "data": _sanitized_provider_value(error.data),
        })
    return details


async def _stream_turn(
    turn,
    log,
    name: str,
    show_events: bool,
    *,
    reject_tool_activity: bool = False,
):
    """Log turn notifications as JSONL and collect the terminal result."""
    completed_turn = None
    items = []
    usage = None
    provider_events: list[dict[str, Any]] = []
    model_reroutes: list[dict[str, Any]] = []
    renderer = _TerminalEventRenderer(name) if show_events else None

    async for event in turn.stream():
        event_data = _jsonable(event)
        log.write(json.dumps(event_data, separators=(",", ":"), default=str))
        log.write("\n")
        log.flush()
        if renderer is not None:
            renderer.render(event)

        payload = event.payload
        event_turn_id = _payload_field(payload, "turn_id")
        item = _payload_field(payload, "item")
        if (
            reject_tool_activity
            and event.method in {"item/started", "item/completed"}
            and (
                not isinstance(event_turn_id, str)
                or not event_turn_id
            )
        ):
            activity = {
                "method": event.method,
                "item": _jsonable(item),
                "reason": "missing-or-malformed-turn-id",
            }
            await _request_interrupt(turn, activity)
            raise _CodexToolActivityRejected(activity)
        if (
            reject_tool_activity
            and event_turn_id == turn.id
            and event.method in {"item/started", "item/completed"}
            and _is_tool_item(item)
        ):
            activity = {
                "method": event.method,
                "item": _jsonable(item),
            }
            await _request_interrupt(turn, activity)
            raise _CodexToolActivityRejected(activity)
        if event.method == "error" and event_turn_id == turn.id:
            provider_events.append(event_data)
        elif event.method == "model/rerouted" and event_turn_id == turn.id:
            model_reroutes.append(event_data)
            if reject_tool_activity:
                reroute = {
                    "event": event_data,
                    "rate_limit_driven": _codex_rate_limit_denied(event_data),
                }
                await _request_interrupt(turn, reroute)
                raise _CodexModelRerouteRejected(reroute)
        if event.method == "item/completed" and event_turn_id == turn.id:
            items.append(item)
        elif (
            event.method == "thread/tokenUsage/updated"
            and event_turn_id == turn.id
        ):
            usage = _payload_field(payload, "token_usage")
        elif event.method == "turn/completed":
            candidate = _payload_field(payload, "turn")
            if (
                candidate is not None
                and _payload_field(candidate, "id") == turn.id
            ):
                if reject_tool_activity:
                    items_view = _value(
                        _payload_field(candidate, "items_view", "full")
                    )
                    if items_view != "full":
                        raise _CodexToolActivityRejected({
                            "method": "turn/completed:items",
                            "item": None,
                            "reason": "terminal-items-view-not-full",
                            "items_view": items_view,
                            "interruption": {
                                "attempted": False,
                                "reason": "turn-already-completed",
                            },
                        })
                    candidate_items = _payload_field(candidate, "items", [])
                    if not isinstance(candidate_items, list):
                        raise _CodexToolActivityRejected({
                            "method": "turn/completed:items",
                            "item": _jsonable(candidate_items),
                            "reason": "terminal-items-malformed",
                            "interruption": {
                                "attempted": False,
                                "reason": "turn-already-completed",
                            },
                        })
                    for candidate_item in candidate_items:
                        if _is_tool_item(candidate_item):
                            raise _CodexToolActivityRejected({
                                "method": "turn/completed:items",
                                "item": _jsonable(candidate_item),
                                "interruption": {
                                    "attempted": False,
                                    "reason": "turn-already-completed",
                                },
                            })
                completed_turn = candidate

    if completed_turn is None:
        raise RuntimeError("turn completed event not received")

    status = _value(_payload_field(completed_turn, "status"))
    if status == "failed":
        error = _payload_field(completed_turn, "error")
        message = _payload_field(error, "message")
        details = {
            "kind": "codex-turn",
            "status": status,
            "error": _jsonable(error),
            "provider_events": provider_events,
            "model_reroutes": model_reroutes,
        }
        raise _CodexTurnFailed(
            message or f"turn failed with status {status}", details
        )

    return _StreamedTurnResult(
        id=str(_payload_field(completed_turn, "id")),
        status=_payload_field(completed_turn, "status"),
        error=_payload_field(completed_turn, "error"),
        duration_ms=_payload_field(completed_turn, "duration_ms"),
        final_response=_final_response(items),
        items=items,
        usage=usage,
        provider_events=provider_events,
        model_reroutes=model_reroutes,
    )


async def _start_codex_thread(
    codex: object,
    *,
    cwd: str,
    model: str | None,
    structured: bool,
) -> tuple[object, dict[str, Any]]:
    """Start through the typed local protocol and retain resolved settings."""

    if structured:
        if not hasattr(codex, "_client"):
            raise RuntimeError(
                "bounded structured Codex route requires the typed protocol client"
            )
        params = ThreadStartParams(
            approval_policy=AskForApproval(root=AskForApprovalValue.never),
            base_instructions=_STRUCTURED_BASE_INSTRUCTIONS,
            config={"mcp_servers": {}},
            cwd=str(Path(cwd).resolve()),
            developer_instructions=_STRUCTURED_DEVELOPER_INSTRUCTIONS,
            ephemeral=True,
            model=model,
            sandbox=SandboxMode.read_only,
        )
        wire_params = params.model_dump(
            by_alias=True,
            exclude_none=True,
            mode="json",
        )
        # Bundled CLI 0.147.0 exposes this experimental thread/start field even
        # though the same-version SDK's generated model omits it. The supported
        # low-level client accepts JsonObject and preserves the extra field.
        wire_params["allowProviderModelFallback"] = False
        wire_params["dynamicTools"] = []
        wire_params["environments"] = []
        started = await codex._client.thread_start(wire_params)
        thread = AsyncThread(codex, started.thread.id)
        metadata = {
            "requested_model": model,
            "resolved_model": started.model,
            "model_provider": started.model_provider,
            "reasoning_effort": _jsonable(started.reasoning_effort),
            "service_tier": _jsonable(getattr(started, "service_tier", None)),
            "runtime_workspace_roots": list(
                getattr(started, "runtime_workspace_roots", None) or []
            ),
            "instruction_sources": list(started.instruction_sources or []),
            "sandbox": _jsonable(started.sandbox),
            "approval_policy": _jsonable(started.approval_policy),
            "provider_model_fallback_allowed": False,
        }
        if metadata["instruction_sources"]:
            raise RuntimeError(
                "bounded structured Codex route loaded instruction sources: "
                + ", ".join(metadata["instruction_sources"])
            )
        if metadata["runtime_workspace_roots"]:
            raise RuntimeError(
                "bounded structured Codex route loaded runtime workspace roots"
            )
        return thread, metadata

    thread = await codex.thread_start(
        approval_mode=ApprovalMode.deny_all if structured else ApprovalMode.auto_review,
        base_instructions=_STRUCTURED_BASE_INSTRUCTIONS if structured else None,
        config=(
            {"mcp_servers": {}}
            if structured
            else (
                {
                    "sandbox_workspace_write": {
                        "writable_roots": [],
                        "exclude_slash_tmp": True,
                        "exclude_tmpdir_env_var": True,
                    }
                }
                if (Path(cwd) / ".agents/skills/discover-components").is_dir()
                else None
            )
        ),
        cwd=str(Path(cwd).resolve()),
        developer_instructions=(
            _STRUCTURED_DEVELOPER_INSTRUCTIONS
            if structured
            else (
                "Execute the requested pipeline skill and produce its artifact. "
                "Treat checkout repositories as source data. Repository "
                "development and work-ledger maintenance are outside this task. "
                "Do not recursively launch the pipeline."
            )
        ),
        model=model,
        sandbox=Sandbox.read_only if structured else Sandbox.workspace_write,
        ephemeral=True,
    )
    return thread, {
        "requested_model": model,
        "resolved_model": model,
        "model_provider": None,
        "reasoning_effort": None,
        "service_tier": None,
        "instruction_sources": [],
        "runtime_workspace_roots": [],
        "sandbox": "read-only" if structured else "workspace-write",
        "approval_policy": "never" if structured else "on-request",
        "provider_model_fallback_allowed": None,
    }


def _structured_codex_context(
    *,
    parent_requested_model: str | None,
    thread_metadata: dict[str, Any],
    cli_identity: dict[str, str],
    staged_context: dict[str, Any],
    effective_context: dict[str, Any],
) -> dict[str, Any]:
    resolved_model = thread_metadata.get("resolved_model")
    provider = thread_metadata.get("model_provider")
    if not isinstance(resolved_model, str) or not resolved_model:
        raise _CodexContextRejected(
            "Codex thread/start did not report a resolved model",
            {"field": "model"},
        )
    if not isinstance(provider, str) or not provider:
        raise _CodexContextRejected(
            "Codex thread/start did not report a resolved model provider",
            {"field": "modelProvider"},
        )
    if (
        parent_requested_model is not None
        and resolved_model != parent_requested_model
    ):
        raise _CodexModelSelectionRejected({
            "parent_requested_model": parent_requested_model,
            "resolved_model_identity": resolved_model,
            "model_provider": provider,
            "reasoning_effort": thread_metadata.get("reasoning_effort"),
        })
    payload = {
        "protocol": _CODEX_CONTEXT_PROTOCOL,
        "parent_requested_model": parent_requested_model,
        "selection_mode": (
            "explicit" if parent_requested_model is not None else "configured-default"
        ),
        "resolved_model_identity": resolved_model,
        "model_provider": provider,
        "reasoning_effort": thread_metadata.get("reasoning_effort"),
        "service_tier": thread_metadata.get("service_tier"),
        "sandbox": thread_metadata.get("sandbox"),
        "approval_policy": thread_metadata.get("approval_policy"),
        "provider_model_fallback_allowed": thread_metadata.get(
            "provider_model_fallback_allowed"
        ),
        "cli_implementation": cli_identity,
        "isolated_local_context": {
            **staged_context,
            **effective_context,
        },
    }
    return {**payload, "context_identity": _canonical_identity(payload)}


def _resolved_codex_settings(context: dict[str, Any]) -> dict[str, Any]:
    return {
        "model_provider": context["model_provider"],
        "reasoning_effort": context["reasoning_effort"],
        "service_tier": context["service_tier"],
    }


async def preflight_structured_codex_context(
    parent_requested_model: str | None,
) -> dict[str, Any]:
    """Resolve the isolated structured context without starting a model turn."""

    cli_path, cli_identity = resolve_codex_cli_identity()
    with tempfile.TemporaryDirectory(prefix="structured-codex-preflight-") as raw:
        root = Path(raw)
        cwd = root / "work"
        cwd.mkdir()
        config, staged = _isolated_codex_config(
            root / "codex-home",
            cli_path=cli_path,
        )
        config.cwd = str(cwd)
        async with AsyncCodex(config=config) as codex:
            effective = await _effective_codex_config(codex, str(cwd))
            _thread, metadata = await _start_codex_thread(
                codex,
                cwd=str(cwd),
                model=parent_requested_model,
                structured=True,
            )
    return _structured_codex_context(
        parent_requested_model=parent_requested_model,
        thread_metadata=metadata,
        cli_identity=cli_identity,
        staged_context=staged,
        effective_context=effective,
    )


async def run_codex_agent(
    *,
    name: str,
    cwd: str,
    prompt: str,
    log_dir: Path,
    model: str | None,
    enable_skills: bool,
    progress: AgentProgress | None,
    strace_dir: Path | None,
    checkout_path: str | Path | None = None,
    input_paths: tuple[str | Path, ...] = (),
    output_paths: tuple[str | Path, ...] = (),
    tool_free: bool = False,
    response_schema: dict | None = None,
    expected_structured_context: dict[str, Any] | None = None,
) -> dict:
    """Run one Codex turn and normalize its result for pipeline callers."""
    if strace_dir is not None:
        raise ValueError("--strace is not supported by the codex harness")
    if response_schema is not None and not tool_free:
        raise ValueError("Codex response_schema requires the bounded structured route")

    log_file = log_dir / f"{name.replace('/', '_')}.log"
    log_dir.mkdir(parents=True, exist_ok=True)
    agent_input = _codex_input(prompt, enable_skills, cwd=cwd)
    model_label = model or "configured default"
    emit = progress.log if progress else print

    emit(f"\n{'=' * 60}")
    emit(f"Starting agent: {name}")
    emit("Harness: codex")
    emit(f"Model: {model_label}")
    emit(f"Working directory: {cwd}")
    emit(f"Log file: {log_file}")
    emit(f"{'=' * 60}")

    log_file.write_text(
        f"Agent: {name}\nHarness: codex\nModel: {model_label}\n"
        f"Working directory: {cwd}\n{'=' * 60}\n\nPROMPT:\n{prompt}\n\n"
        f"{'=' * 60}\nAGENT OUTPUT:\n\n"
    )
    started = time.monotonic()
    heartbeat_task = None
    input_snapshots = _snapshot_immutable_inputs(input_paths)
    integrity_mutations: list[str] = []
    codex_config_dir: tempfile.TemporaryDirectory[str] | None = None
    runtime_context: dict[str, Any] | None = None
    codex_config: CodexConfig | None = None
    cli_identity: dict[str, str] | None = None
    staged_context: dict[str, Any] | None = None
    if progress:
        progress.agent_started(name)
    else:
        async def _heartbeat():
            while True:
                await asyncio.sleep(30)
                elapsed = int(time.monotonic() - started)
                emit(f"[{name}] ... still running ({elapsed}s elapsed)")

        heartbeat_task = asyncio.create_task(_heartbeat())

    try:
        if tool_free:
            cli_path, cli_identity = resolve_codex_cli_identity()
            codex_config_dir = tempfile.TemporaryDirectory(
                prefix="structured-codex-context-"
            )
            codex_config, staged_context = _isolated_codex_config(
                Path(codex_config_dir.name),
                cli_path=cli_path,
            )
            codex_config.cwd = str(Path(cwd).resolve())
        with log_file.open("a") as log:
            codex_instance = (
                AsyncCodex(config=codex_config) if tool_free else AsyncCodex()
            )
            async with codex_instance as codex:
                effective_context = (
                    await _effective_codex_config(codex, cwd)
                    if tool_free
                    else {}
                )
                thread, thread_metadata = await _start_codex_thread(
                    codex,
                    cwd=cwd,
                    model=model,
                    structured=tool_free,
                )
                if tool_free:
                    assert cli_identity is not None
                    assert staged_context is not None
                    runtime_context = _structured_codex_context(
                        parent_requested_model=model,
                        thread_metadata=thread_metadata,
                        cli_identity=cli_identity,
                        staged_context=staged_context,
                        effective_context=effective_context,
                    )
                    if (
                        expected_structured_context is not None
                        and runtime_context.get("context_identity")
                        != expected_structured_context.get("context_identity")
                    ):
                        raise _CodexContextRejected(
                            "Codex structured context changed after preflight",
                            {
                                "expected_context_identity": (
                                    expected_structured_context.get(
                                        "context_identity"
                                    )
                                ),
                                "actual_context_identity": runtime_context.get(
                                    "context_identity"
                                ),
                            },
                        )
                    turn = await thread.turn(
                        agent_input,
                        output_schema=response_schema,
                        sandbox=Sandbox.read_only,
                        approval_mode=ApprovalMode.deny_all,
                    )
                else:
                    turn = await thread.turn(agent_input)
                result = await _stream_turn(
                    turn,
                    log,
                    name,
                    show_events=progress is None,
                    reject_tool_activity=tool_free,
                )

        integrity_mutations = _restore_mutated_inputs(input_snapshots)
        if integrity_mutations:
            raise RuntimeError(
                "Codex mutated immutable planning input(s): "
                + ", ".join(integrity_mutations)
            )

        elapsed = time.monotonic() - started
        with log_file.open("a") as log:
            summary = {"type": "codex_turn_result", "result": _jsonable(result)}
            log.write(json.dumps(summary, separators=(",", ":"), default=str))
            log.write("\n")
            log.flush()

        if progress:
            progress.agent_completed(name, success=True)
        emit(f"Completed: {name} ({int(elapsed)}s)")
        reroutes = [
            item.get("payload", item)
            for item in result.model_reroutes
            if isinstance(item, dict)
        ]
        producing_model = thread_metadata["resolved_model"]
        if reroutes:
            last = reroutes[-1]
            producing_model = (
                last.get("to_model")
                or last.get("toModel")
                or producing_model
            )
        return {
            "name": name,
            "success": True,
            "log_file": str(log_file),
            "duration_seconds": elapsed,
            "raw_response": result.final_response,
            "telemetry": {
                **_source_read_telemetry(result.items, checkout_path),
                "harness": "codex",
                "guard_enforcement": (
                    "read-only-deny-all-no-provider-fallback-reject-activity"
                    if tool_free
                    else "workspace-write-sandbox"
                ),
                "immutable_input_check": "passed",
                "immutable_input_paths": sorted(
                    str(path) for path in input_snapshots
                ),
                "allowed_output_paths": sorted(
                    str(Path(path).resolve()) for path in output_paths
                ),
                "thread_id": thread.id,
                "turn_id": result.id,
                "turn_status": _jsonable(result.status),
                "duration_api_ms": result.duration_ms,
                "usage": _jsonable(result.usage),
                "requested_model_identity": thread_metadata["resolved_model"],
                "parent_requested_model": model,
                "resolved_model_identity": thread_metadata["resolved_model"],
                "producing_model_identity": producing_model,
                "response_models": [producing_model] if producing_model else [],
                "model_reroutes": result.model_reroutes,
                "provider_errors": result.provider_events,
                "applied_model_settings": (
                    _resolved_codex_settings(runtime_context)
                    if runtime_context is not None
                    else {}
                ),
                "codex_cli_identity": cli_identity,
                "codex_structured_context": runtime_context,
                "structured_execution": (
                    {
                        "outer_turns": 1,
                        "response_schema": response_schema is not None,
                        "sandbox": "read-only",
                        "network": "disabled-by-sandbox",
                        "approval_policy": "never",
                        "tool_activity_policy": "interrupt-and-reject-first-event",
                        "provider_model_fallback_allowed": thread_metadata[
                            "provider_model_fallback_allowed"
                        ],
                        "instruction_sources": thread_metadata[
                            "instruction_sources"
                        ],
                        "runtime_workspace_roots": thread_metadata[
                            "runtime_workspace_roots"
                        ],
                        "parent_requested_model": model,
                        "resolved_model_identity": thread_metadata[
                            "resolved_model"
                        ],
                        "model_provider": thread_metadata["model_provider"],
                        "reasoning_effort": thread_metadata["reasoning_effort"],
                        "service_tier": thread_metadata["service_tier"],
                        "base_instructions": _STRUCTURED_BASE_INSTRUCTIONS,
                        "developer_instructions": (
                            _STRUCTURED_DEVELOPER_INSTRUCTIONS
                        ),
                    }
                    if tool_free
                    else {}
                ),
            },
        }
    except asyncio.CancelledError as exc:
        if progress:
            progress.agent_completed(name, success=False)
        with log_file.open("a") as log:
            log.write(
                json.dumps({
                    "type": "codex_turn_cancelled",
                    "error_type": type(exc).__name__,
                    "error": repr(exc),
                })
            )
            log.write("\n")
            log.flush()
        raise
    except Exception as exc:
        elapsed = time.monotonic() - started
        integrity_mutations.extend(
            path
            for path in _restore_mutated_inputs(input_snapshots)
            if path not in integrity_mutations
        )
        if progress:
            progress.agent_completed(name, success=False)
        error_text = str(exc) or repr(exc)
        safe_error_text = str(redact_value("message", error_text))
        emit(
            f"Failed: {name} ({int(elapsed)}s) — "
            f"{type(exc).__name__}: {safe_error_text}"
        )
        with log_file.open("a") as log:
            log.write(
                json.dumps({
                    "type": "codex_turn_error",
                    "error_type": type(exc).__name__,
                    "error": safe_error_text,
                })
            )
            log.write("\n")
            log.flush()
        provider_error = (
            exc.details
            if isinstance(
                exc,
                (
                    _CodexTurnFailed,
                    _CodexModelSelectionRejected,
                    _CodexContextRejected,
                ),
            )
            else {
                **codex_exception_details(exc),
                **(
                    {"kind": "codex-tool-activity", "tool_activity": exc.activity}
                    if isinstance(exc, _CodexToolActivityRejected)
                    else {}
                ),
                **(
                    {"kind": "codex-model-reroute", "model_reroute": exc.reroute}
                    if isinstance(exc, _CodexModelRerouteRejected)
                    else {}
                ),
            }
        )
        return {
            "name": name,
            "success": False,
            "error": safe_error_text,
            "provider_error": provider_error,
            "rate_limit_denied": _codex_rate_limit_denied(provider_error),
            "log_file": str(log_file),
            "duration_seconds": elapsed,
            "telemetry": {
                "harness": "codex",
                "guard_enforcement": (
                    "read-only-deny-all-no-provider-fallback-reject-activity"
                    if tool_free
                    else "workspace-write-sandbox"
                ),
                "immutable_input_check": (
                    "failed-restored" if integrity_mutations else "passed"
                ),
                "mutated_input_paths": integrity_mutations,
                "allowed_output_paths": sorted(
                    str(Path(path).resolve()) for path in output_paths
                ),
                "tool_activity_observations": (
                    [exc.activity]
                    if isinstance(exc, _CodexToolActivityRejected)
                    else []
                ),
                "model_reroutes": (
                    [exc.reroute]
                    if isinstance(exc, _CodexModelRerouteRejected)
                    else []
                ),
                "codex_structured_context": runtime_context,
            },
        }
    finally:
        _restore_mutated_inputs(input_snapshots)
        if heartbeat_task:
            heartbeat_task.cancel()
            try:
                await heartbeat_task
            except asyncio.CancelledError:
                pass
        if codex_config_dir is not None:
            codex_config_dir.cleanup()
