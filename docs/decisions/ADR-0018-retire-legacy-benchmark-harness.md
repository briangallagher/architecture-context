# ADR-0018: Retire the Deleted Legacy Benchmark Harness

## Status

Accepted

## Date

2026-09-05

## Context

Commit `f8a6f6ff` deliberately deleted `benchmark/analyzer-assisted-v1` and
`benchmark/consumer-v1` with the message "get rid of benchmarks for now." It
left benchmark-only tests, launchers, container build inputs, and two active
library schema references in place. The result was 12 pytest collection errors,
additional runtime failures in benchmark-only tests, and a Claude task-runner
image that could not build from the current tree.

Restoring the deleted benchmark would reverse the recorded project decision and
revive a large evaluation corpus with no current task or acceptance review.
Skipping its tests would keep dead dependencies hidden.

## Decision

Treat the two deleted benchmark versions as retired. Remove test modules and
launchers whose only subject is the deleted evaluator, corpus, planner, scorer,
or report generator. Do not skip those tests or restore the benchmark payload.

Preserve contracts still used by active code independently of that harness:

- Move the context-metrics contract to
  `schemas/context-metrics-v1.schema.json` and keep telemetry tests bound to it.
- Move the failure-proposal contract to
  `schemas/failure-proposal-v1.schema.json` and update the proposal CLI and
  tests to use it.
- Keep the generic Claude task launcher, remove deleted benchmark inputs from
  its image, and make `claude` the image entry point.

Update surviving tests that still describe superseded repository contracts:
valid analyzer artifacts use the bounded partial route for every readiness
classification, source budgets are soft telemetry, the aggregate-platform
template lives under `templates/`, and `.env` is passed to Podman without shell
execution.

## Consequences

- Repository-wide pytest collection and execution no longer depend on files
  intentionally removed from the project.
- Active telemetry and failure-proposal outputs retain versioned JSON schemas.
- The Claude task-runner image no longer requires the retired consumer
  benchmark to build or start.
- Historical benchmark references in plans and notes remain historical record.
- Reintroducing either benchmark requires a new scoped task with its own inputs,
  tests, and review rather than silently restoring the deleted tree.
