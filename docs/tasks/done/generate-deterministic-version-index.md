# Task: Generate a Deterministic Version Index

Status: complete 2026-09-05.

Implement remaining-work step 7 of the
[surface coverage plan](../../plans/architecture-surface-coverage.md).

Add a non-agent `generate-index` phase producing each version's `INDEX.md` from
existing component maps, configuration, analyzer metadata, and document headings.
Run after platform architecture and before diagrams in the default full pipeline;
support independent offline invocation and version-wide rebuilding after targeted
component generation without invoking extra agent phases.

Preserve canonical prefixes and included but not integrated repositories such as
Praxis. Separate inclusion, current integration, planned integration, and unknown
status. Reuse descriptions, use explicit topic mappings, attribute planned facts,
and distinguish available documents from pending outputs. Do not invent facts.

Acceptance: deterministic atomic rendering, valid relative links, zero LLM or
network calls, no source inspection, safe missing-input handling, and regression
tests for ordering, aliases, legacy inputs, and relationship status. Exclude the
index from all component-document enumerators and diagram jobs. This task does
not authorize live generation, source refresh, or enforcement/worker changes.

## Result

- Added the standalone and selectable `generate-index` phase, placed it after
  platform generation and before diagrams in the full pipeline, and made a
  targeted pipeline run execute it once version-wide.
- The renderer uses only local component-map, platform, overlay, analyzer,
  coverage, and promoted-document inputs. It separates inventory, document
  availability, shipped metadata, and structured integration status; future
  status can only come from platform configuration or applicable overlay
  frontmatter.
- Rendering is stable, Markdown-safe, URL-encoded, and atomic. Invalid required
  input preserves an existing index. Missing optional inputs remain explicit.
- `INDEX.md` is excluded from platform and diagram agents, component schema
  linting, snapshot and corpus comparisons, surface audits, and `arch-query`
  component loading and counts.
- Added ADR-0024 and documented the index, relationship semantics, offline CLI,
  targeted workflow, and optional overlay metadata. No real `INDEX.md` or other
  generated architecture artifact was written.

## Validation

- Repository-wide Python: 883 passed, 9 skipped.
- Required test gate: 264 passed, 4 skipped, plus all Go module tests.
- Focused index and boundary suite: 69 passed, 1 skipped.
- Ruff, Go lint/vet, lock validation, 31 overlays, 18 platform definitions, and
  901 generated component documents passed.
- `git diff --check` passed; tracked `architecture/` files are unchanged.
