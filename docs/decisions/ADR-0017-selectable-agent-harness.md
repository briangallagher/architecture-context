# ADR-0017: Selectable Agent Harness

## Status

Accepted

## Date

2026-09-04

## Context

Agent-backed pipeline phases were coupled directly to `claude-agent-sdk` and
accepted only Claude model aliases. This also made the top-level command require
a Claude `.env` file for phases such as fetch that launch no agent.

## Decision

Add `--harness=claude|codex` independently from the optional `--model` value.
Claude remains the default for compatibility. The Codex implementation uses
`openai-codex`, the operator's existing Codex login, an ephemeral thread, and
the workspace-write sandbox. Slash-style skill prompts are translated to
explicit Codex skill inputs that point to the same checked-in skill files used
by Claude under `.claude/skills/`.

The first supported vertical slice is fetch plus component discovery. Harness
selection is propagated through the other agent-backed phases so those phases
can be exercised and hardened incrementally. Claude-specific tool hooks and
strace transport are not emulated for Codex in this initial implementation.

Codex discovery runs in a disposable temporary workspace with a copy of the
shared skill under `.agents/skills/` and explicit checkout paths. Worker
instructions require fresh discovery and exclude pipeline development, recursive
pipeline execution, previous maps, and work-ledger maintenance. Sandbox config
removes additional writable roots for these workers. The parent validates the
candidate schema and platform before atomically replacing the destination map.
This is context isolation, not enforced read isolation; a container is deferred.

## Consequences

- Operators can compare Claude and Codex discovery without changing prompts or
  duplicating skills.
- Omitting `--model` lets Codex use its configured default; Claude continues to
  resolve an omitted model to Opus.
- Non-Claude and non-agent commands no longer require an Anthropic `.env` file.
- Codex currently relies on its workspace sandbox rather than the fine-grained
  Claude pre-tool execution guard, so later generation phases need focused
  validation before broad production use.
- `--strace` produces a clear unsupported error with the Codex harness.
