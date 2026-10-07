# Task: Add a Selectable Codex Agent Harness

## Goal

Allow pipeline agent phases to select Codex independently from the model, using
the existing Codex login and the repository's shared skills.

## Result

Implemented the `--harness=claude|codex` CLI contract and a Codex SDK adapter.
The adapter translates slash-skill prompts to explicit shared skill inputs,
runs ephemeral workspace-write threads, normalizes results into the existing
pipeline result shape, and writes per-agent logs. Harness selection propagates
through targeted and all-phase orchestration. Environment loading now requires
the Anthropic `.env` file only when an invocation will actually launch Claude.
Codex notifications are consumed through the public turn stream and flushed to
the log as JSONL as they arrive; cancellation propagates and failures include
their exception type and representation. Direct runs reconstruct agent-message
deltas on stdout and show concise command lifecycle notices while suppressing
reasoning, token, and raw command-output noise.

The fetch plus discover path is ready for operator verification. Focused tests
cover CLI parsing, pipeline propagation, skill translation, sandbox selection,
model defaulting, incremental event logging, cancellation, result normalization,
and legacy Claude behavior.

## Status

Done on 2026-09-04.
