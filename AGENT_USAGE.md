# Agent Usage Guide

This document explains how to navigate and use the pre-generated architecture data in this repository. If you cloned this repo to answer questions about RHOAI or ODH component architecture, security, RBAC, networking, or dependencies, start here.

## Quick Start

All generated architecture data lives under `./architecture/`. The `checkouts/` directory is gitignored and only exists locally when someone runs the orchestrator pipeline — you will not find it in a clone from GitHub.

## Directory Layout

```
overlays/                             # Architecture updates between regeneration cycles
  README.md
  NNNN-short-description.md
architecture/
  rhoai-{version}/                  # One directory per analyzed version
    INDEX.md                        # Deterministic inventory and topic navigation
    {component}.md                  # Component architecture document (structured markdown)
    {component}/
      analyzer.json                 # accepted deterministic input
      synthesis.json                # original model-response envelope
      document.json                 # authoritative accepted facts and provenance
      diagrams/                     # optional structured-component derivatives
        metadata.json               # document/generator identity and availability
    PLATFORM.md                     # Aggregated platform-level architecture document
    diagrams/
      {component}-component.mmd    # Mermaid component diagram
      {component}-component.png    # Rendered PNG
      {component}-dataflow.mmd     # Mermaid data flow diagram
      {component}-dataflow.png
      {component}-dependencies.mmd # Mermaid dependency diagram
      {component}-dependencies.png
      {component}-rbac.mmd         # Mermaid RBAC diagram
      {component}-rbac.png
      {component}-security-network.mmd  # Mermaid security/network diagram
      {component}-security-network.png
      {component}-security-network.txt  # ASCII art security/network diagram
      {component}-c4-context.dsl        # Structurizr C4 context diagram
    prompts/                        # (some versions) The prompts used to generate component docs
```

## Available Versions

Versions follow the pattern `rhoai-{major}.{minor}` or `rhoai-{major}.{minor}-ea.{n}` for early access. List them with:

```
ls architecture/ | sort -V
```

As of March 2026, versions range from `rhoai-2.6` through `rhoai-3.4`. The latest GA release is typically the highest version number without an `-ea` suffix.

## Component Architecture Files (`{component}.md`)

Versions can mix two explicit formats. A legacy component has only the flat
Markdown and optional legacy analyzer JSON. A published structured component
also has `{component}/{analyzer,synthesis,document}.json`; its `document.json` is
the authority and the flat Markdown is a validated derivative. If those nested
files exist but are inconsistent, do not fall back to legacy Markdown. Missing
or stale Markdown can be rerendered locally from the accepted document with the
arch-analyzer renderer and no model call. `arch-query` can answer typed queries
from a valid JSON-only authority, but repository linting and release packaging
require the complete four-file snapshot, including Markdown. The Python producer
and reuse path recomputes model-response identity and eligibility; Go query and
packaging consumers verify the published cross-artifact bindings but do not
independently recompute those producer properties.

Each component `.md` file follows a standardized structure with these sections. Every section uses markdown tables for machine-readable structured data:

| Section | Contents |
|---------|----------|
| **Metadata** | Repository URL, version, git commit, distribution (RHOAI/ODH), languages, deployment type, supported OCP versions, CPU architectures, image count, operator feature flags, generation date |
| **Purpose** | Short (1 sentence) and detailed (1-2 paragraphs) description |
| **Architecture Components** | Table of sub-components (name, type, purpose) |
| **APIs Exposed** | CRD table (group, version, kind, scope, purpose) and HTTP endpoints table (path, method, port, protocol, encryption, auth) |
| **Dependencies** | External dependencies table (component, version, required, purpose) and internal RHOAI dependencies table (component, interaction type, purpose) |
| **Network Architecture** | Services table (name, type, port, target port, protocol, encryption, auth, exposure), Ingress table, Egress table |
| **Security** | RBAC cluster roles table (role, API group, resources, verbs), Secrets table (name, type, purpose, provisioner, auto-rotate), Auth & authz table |
| **Data Flows** | Numbered step-by-step tables per flow (step, source, destination, port, protocol, encryption, auth) |
| **Integration Points** | Table of inter-component interactions (component, type, port, protocol, encryption, purpose) |
| **Recent Changes** | Changelog table (version, date, changes) |
| **Source References** | Table of analyzed files (file, lines, sections informed) |

### How to use component files

- **To find what CRDs a component owns**: read the `APIs Exposed > Custom Resource Definitions` table.
- **To find what ports/protocols a component uses**: read the `Network Architecture > Services` table.
- **To find what RBAC permissions a component needs**: read the `Security > RBAC - Cluster Roles` table.
- **To trace a request through the system**: read the `Data Flows` tables for step-by-step source→destination chains with ports and auth.
- **To find what a component depends on**: read the `Dependencies` tables (both external and internal RHOAI).
- **To find secrets a component uses or creates**: read the `Security > Secrets` table.

## Version Navigation Index (`INDEX.md`)

Newly indexed version directories contain an `INDEX.md` generated from local,
structured artifacts. Start there to find canonical component aliases, available
and pending component documents, topic headings, analyzer and coverage metadata,
and release-applicable overlays.

The index is a navigation document. Topic links point to headings that exist,
but do not prove that a component has a capability or relationship. Inventory
presence and `shipped` metadata are shown separately from integration status.
Current, planned, and not-integrated labels require structured version-scoped
metadata; absent evidence remains `unknown`. Planned status comes only from
platform configuration or structured active overlay frontmatter. Follow the
linked component document, `PLATFORM.md`, analyzer metadata, or overlay before
using a substantive claim.

Legacy version directories may not contain an index. In that case, use the
component map and the individual Markdown files directly.

## Platform Architecture File (`PLATFORM.md`)

Each version directory contains a `PLATFORM.md` that aggregates all component data into a platform-level view. It includes:

- Complete component inventory table with types and versions
- Component relationship/dependency graph
- Cross-component data flow analysis
- Platform-wide security posture (RBAC aggregation, network policies, TLS coverage)
- Shared infrastructure (Gateway API, service mesh, monitoring)

Use `PLATFORM.md` when you need a holistic view or need to understand how components interact. Use individual component `.md` files when you need deep detail on a specific component.

## Diagrams

New structured components place diagram files in
`{component}/diagrams/`; `metadata.json` binds them to the accepted document and
records generator identity and success/failure. Platform diagrams and legacy
component diagrams remain under the version-level `diagrams/` directory.

| Diagram | Format | Purpose |
|---------|--------|---------|
| `{component}-component.mmd` | Mermaid | Internal architecture: sub-components and their relationships |
| `{component}-dataflow.mmd` | Mermaid | Request/data flows through the component |
| `{component}-dependencies.mmd` | Mermaid | What this component depends on (internal and external) |
| `{component}-rbac.mmd` | Mermaid | RBAC roles, bindings, and permission scopes |
| `{component}-security-network.mmd` | Mermaid | Network topology with encryption and auth at each hop |
| `{component}-security-network.txt` | ASCII art | Same as above but in text-art form (useful for terminals/plain text) |
| `{component}-c4-context.dsl` | Structurizr DSL | C4 model context diagram showing external actors and systems |

PNG renders (`*.png`) are pre-generated from the Mermaid sources.

## Overlays (`overlays/`)

Overlays are architecture updates that correct or extend the generated docs between regeneration cycles. When a version bump, maturity change, or dependency shift happens and the architecture docs haven't been regenerated yet, an overlay captures the fact so consumers use current information.

```
overlays/
  README.md                              # Format docs, naming convention, lifecycle
  0001-kfp-sdk-2.16-in-rhoai-3.4.md     # Example overlay
```

Each overlay is a Markdown file with YAML frontmatter (`status`, `affects`, `release`, `provenance`, `author`) and three body sections (`## Fact`, `## Impact on Strategies`, `## Context`).

### How to use overlays

- **Read active overlays before trusting generated docs**: check `overlays/*.md` for any with `status: active` that affect the components you're looking at. Active overlays take precedence over the generated architecture docs when they conflict.
- **Filter by component**: the `affects` field lists which components an overlay applies to. Match against the component you're researching.
- **Filter by release**: the `release` field scopes overlays to specific RHOAI versions. Use `"all"` for timeless facts.
- **Ignore superseded overlays**: overlays with `status: superseded` have been absorbed into the regenerated docs and can be skipped.

See `overlays/README.md` for the full format specification.

## The `checkouts/` Directory (Local Only)

This directory is **gitignored** and will not exist in a GitHub clone. It only appears when someone runs the orchestrator pipeline locally.

When present, it contains the actual cloned source repositories:

```
checkouts/
  red-hat-data-services.rhoai-{version}/
    kserve/               # Full git checkout of the kserve repo at that branch
    odh-dashboard/        # Full git checkout of odh-dashboard
    rhods-operator/       # Full git checkout of rhods-operator
    notebooks/            # ...
    ...                   # ~49 repos per version
```

Each subdirectory is a complete git repository checked out at the version-specific branch (e.g., `rhoai-3.4`). The component architecture `.md` files were generated by analyzing these source checkouts. If `checkouts/` exists, you can cross-reference the architecture docs against the actual source code.

## Component Name Mapping

Component names in filenames match the repository names from the `red-hat-data-services` GitHub org. Examples:

| Filename | Repository | What it is |
|----------|-----------|------------|
| `kserve.md` | red-hat-data-services/kserve | Model serving controller |
| `rhods-operator.md` | red-hat-data-services/rhods-operator | Central platform operator |
| `odh-dashboard.md` | red-hat-data-services/odh-dashboard | Web UI console |
| `notebooks.md` | red-hat-data-services/notebooks | Jupyter workbench images |
| `kubeflow.md` | red-hat-data-services/kubeflow | Training operator |
| `data-science-pipelines-operator.md` | red-hat-data-services/data-science-pipelines-operator | Pipeline infrastructure operator |
| `model-registry-operator.md` | red-hat-data-services/model-registry-operator | Model registry lifecycle |

The number of components grows across versions (e.g., `rhoai-2.6` has ~10 components, `rhoai-3.4` has ~48).

## Common Agent Tasks

### "What components exist in version X?"

```
cat architecture/rhoai-X/INDEX.md
```

If that version predates index generation, inspect `component-map.json`; avoid
counting `INDEX.md`, `PLATFORM.md`, or `README.md` as components.

### "What are the network ports for component Y in version X?"

Read `architecture/rhoai-X/Y.md` and look for the `## Network Architecture` section, specifically the `### Services` and `### Ingress` tables.

### "What changed between version A and version B?"

Compare the component `.md` files across version directories. The `## Recent Changes` section in each file documents version-specific changes. For a structural comparison, diff the `PLATFORM.md` files.

### "What RBAC does component Y need?"

Read `architecture/rhoai-X/Y.md` and look for the `## Security > ### RBAC` tables. For a newly structured component, use `architecture/rhoai-X/Y/diagrams/`; legacy diagrams remain in `architecture/rhoai-X/diagrams/`.

### "Show me how requests flow through the system"

Read the `## Data Flows` section in either a component `.md` (for component-specific flows) or `PLATFORM.md` (for cross-component flows). Use `{component}/diagrams/` for newly structured component derivatives and `diagrams/` for platform or legacy derivatives.

### "What are all the CRDs in the platform?"

Read `PLATFORM.md` for the aggregated view across all components, or check individual component `.md` files for per-component CRD tables under `## APIs Exposed`.
