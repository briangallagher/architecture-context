# Project Plan

## Current Milestone

Continue the analyzer-assisted track: valid analyzer artifacts default to the
bounded partial (extend-and-improve) route for all readiness classifications
(sufficient, partial, insufficient, unknown); synthesis is not selected for
normal generation; the synthesis migration allowlist is retained for audit
only; legacy is reserved for missing/invalid artifacts or explicit operator
override. External rollout gates remain separate. The next implementation
milestone is the [arch-analyzer optimization follow-up](docs/plans/architecture-context-static-migration.md),
driven by the completed 97-component run.
The analyzer gap evidence and read justification plan is complete; its replay
measurements are recorded in
[docs/notes/architecture-context-static-migration.md](docs/notes/architecture-context-static-migration.md).
The next focused milestone is the
[arch-analyzer evidence quality follow-up](docs/plans/architecture-context-static-migration.md).

## Active Tasks

- [Prepare Structured Offline Checks](docs/tasks/done/prepare-structured-offline-checks.md) — cleanup and checks independently accepted in the completed structured implementation.

- [Implement Structured Component Assembly](docs/tasks/done/implement-structured-component-assembly.md) — implementation and offline gates complete; final independent PASS. Live evaluation and default adoption remain HOLD.
- [Evaluate the Structured Route Live](docs/tasks/pending/evaluate-structured-component-live-canary.md) — prepared bounded trial; requires separate execution, model and spending authorization.
- [Adopt the Structured Component Layout](docs/tasks/pending/make-structured-publication-the-default.md) — **active**; ADR-0027 keeps the existing agent generation route and changes only the producer's output contract and what is published. ADR-0026 was drafted and rejected the same day.
- [Repair Live-Canary Promotion Defects](docs/tasks/done/repair-live-canary-promotion-defects.md) — complete and independently accepted; repaired promotion preserves non-resource RBAC facts and rejects misplaced configured synthesis subsections with durable diagnostics.
- [Generate a Deterministic Version Index](docs/tasks/done/generate-deterministic-version-index.md) — complete zero-agent navigation phase after platform architecture and before diagrams; live generation remains operator-controlled.
- [Checkpoint Surface Work and Restore Validation](docs/tasks/done/checkpoint-surface-work-and-restore-validation.md) — complete; checkpoint committed and full validation baseline restored.
- [Fix Surface FIPS Applicability](docs/tasks/done/fix-surface-fips-applicability.md) — complete after iterative independent review; source provenance and FIPS-specific negative boundaries are regressed.
- [Refresh Surface Analyzer Evidence](docs/tasks/done/refresh-surface-analyzer-evidence.md) — complete; nine exact source checkouts were refreshed in isolation and independently reviewed.
- [Run Repeated Surface Coverage Live Canary](docs/tasks/done/run-surface-coverage-live-canary.md) — complete; independent review accepted the corrected report, which rejects the canary because all four promotions lost analyzer-rendered RBAC rows.
- [Decide Surface Coverage Rollout](docs/tasks/done/decide-surface-coverage-rollout.md) — complete; ADR-0023 keeps coverage warning-only and subsection workers disabled.
- [Audit Architecture Surface Rollout](docs/tasks/done/audit-architecture-surface-rollout.md) — completed read-only audit; stored artifacts lack the evidence needed for an enforcement decision.
- [Improve Architecture Surface Coverage](docs/tasks/done/improve-architecture-surface-coverage.md) — implemented and independently reviewed; the offline-canary selection remains provisional pending any separate rollout decision.
- [Complete the Architecture Context Static Migration](docs/tasks/done/complete-architecture-context-static-migration.md) — consolidated implementation and iteration history.
- [Replace Markdown Change Records with a JSON Patch Contract](docs/tasks/done/replace-markdown-change-record-with-json-patch.md) — complete; new generation uses a validated JSON patch and historical Markdown replay remains available.
- [Resolve External Analyzer-Assisted Rollout Gates](docs/tasks/blocked/resolve-external-analyzer-assisted-rollout-gates.md) — blocked on external and human inputs; not a local implementation blocker.

## Structured component review gates

These gates are complete under the parent task's phase budgets. The final
rate-limit interruption was resumed with the same model and effort.

1. [Finish P3 synthesis review](docs/tasks/done/review-structured-synthesis-final.md) — P3 independently accepted; all quota findings fixed.
2. [Review P4 publishing and consumers](docs/tasks/done/review-structured-publication-consumers.md) — P4 independently accepted after focused repairs.
3. [Review P5 final checks and adoption hold](docs/tasks/done/review-structured-final-readiness.md) — final PASS in cycle 2; live evaluation/adoption remain HOLD.

## Open Bugs

- [Partial Route Component Runtime Remains High](docs/bugs/open/partial-route-component-runtime-remains-high.md)
- [Surface Coverage Document References Produce False Positives](docs/bugs/open/surface-coverage-document-reference-false-positives.md)
- [Codex Discovery Telemetry Does Not Classify rg Searches](docs/bugs/open/codex-discovery-telemetry-misses-rg.md)

## Plans

- [Structured Component Assembly — Consolidated Plan](docs/plans/structured-component-assembly-consolidated.md)
- [Structured Component Assembly — Original Proposal and Reviews](docs/plans/structured-component-assembly.md)
- [Architecture Surface Coverage](docs/plans/architecture-surface-coverage.md)
- [Architecture Context Static Migration](docs/plans/architecture-context-static-migration.md)
- [Architecture Diagram Implementation](docs/plans/000-architecture-diagram-implementation.md)

## Decisions

- [ADR-0001: Architecture Diagram Proposal](docs/decisions/ADR-0001-architecture-diagram-proposal.md)
- [ADR-0002: Skills-First MVP](docs/decisions/ADR-0002-skills-first-mvp.md)
- [ADR-0003: Python Orchestrator Pipeline](docs/decisions/ADR-0003-python-orchestrator.md)
- [ADR-0004: Kustomize Overlay Context Injection](docs/decisions/ADR-0004-kustomize-overlay-context.md)
- [ADR-0005: Architecture Context Overlays](docs/decisions/ADR-0005-architecture-context-overlays.md)
- [ADR-0006: platforms.yaml Configuration](docs/decisions/ADR-0006-platforms-yaml.md)
- [ADR-0007: component-map.json Intermediate Artifact](docs/decisions/ADR-0007-component-map-json.md)
- [ADR-0008: Pure Skill Invocation](docs/decisions/ADR-0008-pure-skill-invocation.md)
- [ADR-0009: Sub-Agent Dispatch](docs/decisions/ADR-0009-sub-agent-dispatch.md)
- [ADR-0010: arch-query Go CLI](docs/decisions/ADR-0010-arch-query-go-cli.md)
- [ADR-0011: rhoai.next Rolling Target](docs/decisions/ADR-0011-rhoai-next-rolling-target.md)
- [ADR-0012: Linting and CI](docs/decisions/ADR-0012-linting-and-ci.md)
- [ADR-0013: Webhook Inventory Phase](docs/decisions/ADR-0013-webhook-inventory-phase.md)
- [ADR-0014: Declarative exclude_files](docs/decisions/ADR-0014-exclude-files.md)
- [ADR-0015: Build Metadata Extraction](docs/decisions/ADR-0015-build-metadata-extraction.md)
- [ADR-0016: Image and Repo Provenance](docs/decisions/ADR-0016-image-and-repo-provenance.md)
- [ADR-0017: Selectable Agent Harness](docs/decisions/ADR-0017-selectable-agent-harness.md)
- [ADR-0018: Retire the Deleted Legacy Benchmark Harness](docs/decisions/ADR-0018-retire-legacy-benchmark-harness.md)
- [ADR-0019: Use Versioned JSON Architecture Table Patches](docs/decisions/ADR-0019-versioned-json-architecture-patches.md)
- [ADR-0023: Keep Surface Coverage Warning-Only and Subsection Workers Disabled](docs/decisions/ADR-0023-keep-surface-coverage-warning-only.md)
- [ADR-0024: Generate a Deterministic Version Navigation Index](docs/decisions/ADR-0024-deterministic-version-navigation-index.md)

## Notes

- [Implementation and Independent Review Framework](docs/notes/implementation-framework.md) — reusable execution roles, requirements, phase gates, and evidence records.
- [FIPS Applicability Review Packet](docs/notes/fips-applicability-review-packet.md)
- [Architecture Surface Coverage Completion Audit](docs/notes/architecture-surface-coverage-completion-audit.md)
- [Architecture Context Static Migration](docs/notes/architecture-context-static-migration.md)
- [Architecture Diagram Requirements](docs/notes/architecture-diagram-requirements.md)
- [Webhooks feature reference](docs/notes/webhooks.md)
