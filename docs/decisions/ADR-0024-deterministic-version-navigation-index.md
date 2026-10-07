# ADR-0024: Generate a Deterministic Version Navigation Index

## Status

Accepted

## Date

2026-09-05

## Context

Architecture consumers must currently inspect a component map, many component
documents, `PLATFORM.md`, analyzer metadata, coverage sidecars, and overlays to
locate relevant evidence. Generated summaries also mix inventory membership and
platform relationships in ways that can lead a consumer to treat an included
repository as an integrated component.

This repository needs a low-cost navigation interface that can be rebuilt after
targeted generation and on legacy inputs without another agent run. It must not
infer relationships or capabilities from repository names, free-form prose, or
the mere presence of a component in an inventory.

## Decision

Add a non-agent `generate-index` phase after platform architecture generation
and before diagram generation. It atomically renders
`architecture/<version>/INDEX.md` from the version's `component-map.json`,
structured `platforms.yaml` data, active release-applicable overlay metadata,
available analyzer and coverage metadata, and headings and Purpose text already
present in promoted architecture documents.

The index keeps these concepts separate:

- Inventory inclusion and the component map's `shipped` value are displayed as
  inventory signals.
- Integration is `unknown` unless a structured version-scoped value establishes
  `current`, `planned`, or `not-integrated` status.
- Active release-applicable overlays take precedence, followed by platform
  configuration and then an explicit non-planned component-map value.
- Conflicting active overlay statuses resolve to `unknown`.
- `planned` status is accepted only from platform configuration or structured
  active overlay frontmatter. Overlay prose is not parsed for integration facts.

Topic links come from actual document headings and are labeled as navigation
hints. Pending component documents have no links. Missing optional analyzer or
coverage artifacts remain explicitly unavailable. Malformed required inputs
fail before replacing an existing valid index.

Treat `INDEX.md` as reserved version metadata. Component enumeration, platform
aggregation, architecture-document schema linting, diagram jobs, snapshot and
corpus comparisons, rollout audits, and `arch-query` component loading exclude
it. A selected targeted pipeline runs the version-wide index phase once, even
when component phases run once per selected component.

## Consequences

- Consumers gain a byte-stable inventory and topic directory without network,
  source-checkout, model, or agent calls.
- The index reduces navigation work but does not replace component evidence,
  `PLATFORM.md`, diagrams, or `arch-query`.
- Human-authored structured metadata is required to describe planned
  integration, and conflicting metadata remains visible as uncertainty.
- Existing generated architecture directories are unchanged until an operator
  explicitly runs the new phase.

## Related Records

- [Architecture surface coverage plan](../plans/architecture-surface-coverage.md)
- [Deterministic version index task](../tasks/done/generate-deterministic-version-index.md)
- [Architecture context overlays](ADR-0005-architecture-context-overlays.md)
- [Component map intermediate artifact](ADR-0007-component-map-json.md)
