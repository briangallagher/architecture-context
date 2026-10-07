# Structured component assembly: phase 1 contract

This document inventories the deterministic phase-one slice. It does not
describe or implement cross-version reuse, synthesis calls, publication,
recovery, or downstream consumer selection.

## Flow and authority

The new offline flow is:

```text
analyzer JSON -> arch-analyzer normalize -> accepted document JSON
accepted document JSON -> arch-analyzer render-document -> component Markdown
```

`render-document` passes the accepted document's compatibility projection and
typed sections to the existing `internal/renderer.Markdown` implementation. It
does not parse candidate Markdown, read a source checkout, invoke an agent, or
publish a component snapshot. The older `render` command remains available for
compatibility and byte-parity comparison.

The accepted document schema is
`schemas/structured-component-document-v1.schema.json`. Its fact payloads are
validated against the analyzer's Go input types, while all persisted field names
are explicit snake_case JSON tags; the schema does not depend on Go's default
field-name casing. Typed changes use an untrusted proposal contract in
`schemas/structured-component-patch-v1.schema.json`. Attribution, claim class,
allowed fact types, applicability, and accept/reject decisions exist only in the
separate trusted
`schemas/structured-component-assembly-policy-v1.schema.json`. A policy binds to
the canonical proposal fingerprint, canonical analyzer-input fingerprint, exact
patch ID, and a concrete version scope.

Object key order and insignificant JSON whitespace do not change either
fingerprint; meaningful array order remains significant. A proposal with no
operations is valid without a policy. Every operation otherwise requires one
explicit trusted decision. Model payload fields cannot self-declare origin,
claim class, authority, or acceptance because the strict proposal decoder and
schema reject those properties.

## Analyzer fact inventory and accounting

`internal/structured/registry.go` is the executable inventory. It covers every
non-identity JSON field of `model.Input`, including all nested RBAC and dependency
families. A reflection regression test fails when `model.Input`, `model.RBAC`, or
`model.Dependencies` gains an unregistered field.

The 51 registered types are:

- purpose and component identity: `summary`, `source_component`, `entrypoint`,
  `deployment`, `dockerfile`, `managed_component_contract`,
  `runtime_managed_component`, `infrastructure_resource`;
- API and serving surfaces: `crd`, `serving_runtime_definition`,
  `api_reference_contract`, `field_projection`, `http_endpoint`, `grpc_service`,
  `runtime_server`;
- dependencies and relationships: `dependency_go_version`,
  `dependency_go_module`, `dependency_package`, `dependency_internal`,
  `runtime_module_use`, `runtime_client`, `component_ref`, `integration_point`,
  `controller_watch`, `external_webhook`, `cross_reference`;
- network and deployment: `service`, `ingress_route`, `external_connection`,
  `source_default`;
- security and admission: `rbac_cluster_role`, `rbac_role`,
  `rbac_cluster_role_binding`, `rbac_role_binding`, `secret_reference`,
  `authentication`, `security_evidence`, `behavioral_evidence`, `access_policy`,
  `runtime_security_control`, `runtime_proxy_control`, `runtime_webhook_server`,
  `webhook`;
- coverage and context: `data_coverage`, `category_coverage`,
  `coverage_finding`, `synthesis_evidence`, `cross_cutting_evidence`,
  `gap_evidence_candidate`, `context_contract`, `recent_change`.

Every input element or keyed map record gets a deterministic typed fact ID,
input JSON pointer, source revision/evidence when available, uncertainty, and
authority. Its accounting record says either `rendered` or `retained` and names
the renderer-owned section IDs. The latter state is important: analyzer families
not projected by the current Markdown tables remain accepted facts instead of
being silently discarded. Unknown top-level/nested fields, unsupported schema
versions, unregistered fact types, duplicate IDs, incomplete accounting, bad
source ranges, and invalid repository-relative evidence paths fail explicitly.

RBAC roles and bindings are separate fact families. Resource permissions,
non-resource URL permissions, mixed rules, and legacy rules with no URL data are
preserved as distinct values. Missing legacy URLs remain empty; normalization
does not invent them.

## Identity, provenance, uncertainty, and corrections

Accepted identity keeps the canonical component key, analyzer-emitted source
component, repository identity, source revision, release scope, aliases, and an
explicit integration state (`current`, `planned`, `not-integrated`, or
`unknown`). This allows `praxis-policy` to remain the canonical prefixed key
while retaining repository identity `praxis-proxy/policy`; lack of current
integration does not remove the component.

The complete component-map `components`/`provenance` projection used to derive
canonical component identity and repo lineage is retained as a separately
attributed parent assembly input with a verified canonical content fingerprint.
Unconsumed map metadata and dependency-graph fields are not rendering inputs. The
retained projection is not counted as an analyzer fact.
Accepted-document validation rebuilds the compatibility projection with this
stored value and rejects changed map content, identity, or repo-lineage rows;
`rendering_view.repo_lineage` is not trusted as an independent content source.

Typed proposal operations are `add`, `update`, or `delete`, with a fact type,
stable target ID where applicable, structured evidence, and proposal reason.
The separate policy supplies trusted origin and authority plus a human-attributed
decision and decision reason. Assembly rejects stale proposal or bundle
fingerprints, unknown/mismatched scopes, missing or duplicate decisions, missing
accepted evidence, missing targets, type mismatches, unauthorized categories,
conflicting accepted operations, duplicate additions, and ambiguous numeric
legacy patch-v1 envelopes. Updated facts use a `/patches/...` input pointer rather
than retaining a misleading analyzer pointer. Proposal dispositions retain the
proposal/bundle identities, action and fact type, evidence, target/result IDs,
origin and claim class, policy authority, decision identity, actor, and reasons.
Accepted non-analyzer facts fail validation without a matching disposition.
The document also retains one canonical patch-input record per proposal with
the expected operation IDs. Validation requires exactly one disposition for
each listed operation, so accepted deletes and rejected proposals cannot lose
their disposition merely because they leave no resulting fact.
Facts attributed as `support` or `planned` are retained but excluded from the
implementation rendering view, so roadmap/support statements cannot silently
become observed implementation. Approved corrections remain attributed and may
affect the view.

## Section registry

The renderer owns titles, levels, parentage, and placement for these conditional
sections:

| Section ID | Rendered heading | Parent |
|---|---|---|
| `aipcc-ecosystems-use` | `## AIPCC Ecosystems Use` | document |
| `sub-component-details` | `## Sub-Component Details` | document |
| `deployment-manifests` | `## Deployment Manifests` | document |
| `security.fips-compliance` | `### FIPS Compliance` | `## Security` |
| `security.build-hermeticity` | `### Build Hermeticity` | `## Security` |
| `multi-tenancy` | `## Multi-Tenancy` | document |

Each section has attributed origin and claim class, and every evidence reference
includes its source revision. `support` and `planned` sections are retained in
JSON but excluded from implementation Markdown. Content supports single-line
paragraphs and lists plus named typed tables. Table IDs select renderer-owned
subheadings, headers, column counts, and placement; rows carry safe cells and
revision-bound evidence. The current table registry covers the baseline AIPCC,
sub-component, deployment, FIPS build/runtime, build-hermeticity, and
multi-tenancy tables.

Evidence revisions identify the source that supports each individual claim and
are not required to equal `identity.source_revision`. In particular, an
attributed overlay may cite its own revision while the accepted identity keeps
the analyzed component repository revision. Paths and revisions are constrained
to single-line, control-free citation atoms and are escaped again when rendered.
Unresolved implementation sections always render their explicit status and
uncertainty before any paragraph, list, or table content. Typed tables require at
least one row; empty tables are rejected during initial normalization rather than
being serialized into a document that cannot be decoded consistently.

The CLI section file is an untrusted content-only array: it cannot carry
authority fields. The trusted invocation supplies section origin kind, origin
ID, and claim class through separate flags, and normalization writes that
attribution into every accepted section. Internal callers likewise pass
already-attributed sections only across the trusted `Options` boundary.

User-supplied headings, fences, thematic breaks, indented code, nested ordered or
unordered lists, raw tables, raw Markdown links/autolinks, block quotes, and HTML
are rejected. Rendering trims inline content and escapes backslashes, angle
brackets, brackets, and pipes as a second defense, while preserving supported
emphasis and inline-code delimiters. FIPS cannot be represented as a top-level
section because its title and level come only from the registry.

## Consumer inventory and phase boundary

The producer is `internal/extractor`/`internal/model`. Phase 1 changes only the
analyzer normalizer/renderer boundary and adds its CLI entry points. The current
consumers identified for later phases are:

- pipeline routing, static analysis, generation, merge, orchestration, and
  collection code under `lib/`;
- the legacy Markdown section assembler in `src/arch-doc`;
- the analyzer JSON mapper, Markdown parser, loader, commands, and embedded data
  path in `src/arch-query`;
- component-summary, platform aggregation, component-map discovery, diagram,
  surface-coverage, validation, lint, audit, index, and packaging tools/skills;
- human and RFE/STRAT consumers of flat component Markdown, `PLATFORM.md`, and
  `INDEX.md`.

Those consumers are deliberately unchanged here. In particular, this phase does
not select `document.json` in the pipeline or arch-query, change publication
layout, create synthesis envelopes, collect verified read/search dependencies,
or implement reuse. The accepted model and stable snake_case rendering view are
designed so the planned loader adapter can map them without parsing component
Markdown.

## Rendering parity and intentional differences

With no typed conditional sections, accepted-document rendering is byte-identical
to the current analyzer normalization/renderer output. Supplying a registered
conditional section intentionally adds only its renderer-owned heading and typed
content and tables at the registered location. Section and table evidence renders
with its revision. `render-document` also intentionally does
not emit the compatibility-only `analyzer_synthesis_context.md` sidecar created by
the old `render --output` path.
