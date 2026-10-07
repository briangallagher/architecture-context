> **Agents**: If you are an AI agent using the data in this repo (rather than running the pipeline), read [AGENT_USAGE.md](AGENT_USAGE.md) for how to navigate the `architecture/` directory.

# RHOAI Architecture Diagrams

Automated pipeline that clones ODH/RHOAI component repositories, generates per-component architecture summaries using Claude or Codex agents, aggregates them into platform-level documents, and produces Mermaid/C4 diagrams. All driven by `main.py`.

## How it works

The pipeline has six numbered phases plus component discovery and static-analysis
preparation stages. Each stage is runnable independently or through `main.py all`.

### Phase 1: Fetch (`fetch`)

Uses `gh-org-clone` to clone all repositories from a GitHub org. For RHOAI, the `--branch` flag filters to repos that have that branch and creates a versioned checkout directory:

```
checkouts/red-hat-data-services.rhoai-3.4-ea.1/
  rhods-operator/
  kserve/
  odh-dashboard/
  notebooks/
  ...  (~49 repos)
```

Platforms may also define `repo_branch_policies` for an organization. Fetch
checks each cloned repo's remote branch names against the configured regex and
writes `repo-branch-policy.json` in that organization's checkout directory.
The regex must match a complete branch name. The check only determines
eligibility; it does not change the checked-out branch. Component discovery
reads the report and excludes non-matches before provenance lookup and LLM
classification. A remote-ref query failure stops the fetch phase so an
unavailable branch list cannot silently pass the policy.

### Phase 2: Parse manifests (`parse-manifests`)

Parses the operator's `get_all_manifests.sh` script to extract the `COMPONENT_MANIFESTS` bash associative arrays. This identifies ~17 components that the operator directly manages via kustomize manifests.

### Preparation: Discover components (`discover-components`)

Builds the version-scoped `component-map.json` used by subsequent stages. It
preserves canonical component aliases, repository identities, types, and
inventory metadata.

### Preparation: Static analysis (`static-analysis`)

Runs `arch-analyzer` for selected components and stores analyzer metadata below
each component's `.analyzer/` directory. Older architecture versions can lack
these project-analyzer artifacts.

### Phase 3: Generate component architecture (`generate-architecture`)

For each component without `architecture/<platform>/<component>.md`, spawns a Claude agent (via `claude-agent-sdk`) that reads analyzer artifacts from the architecture tree and targeted source files from the checkout. Agents write the structured architecture summary directly to the architecture tree and run concurrently (default 5 at a time).

**Component discovery** works in three layers:
1. **Manifest components** (~17) — parsed from `get_all_manifests.sh`
2. **Operator** (+1) — the operator itself, added explicitly
3. **Adjacent components** (+~31, RHOAI only) — all other repos in the checkout directory that aren't already covered by manifests, minus an exclusion list of utility repos

**Build metadata** from `RHOAI-Build-Config` is injected into each agent's prompt:
- Product version, supported OCP versions, CPU architectures (amd64, arm64, ppc64le, s390x)
- Operator feature flags (FIPS compliance, disconnected support, etc.)
- Container image count and the image-to-source-repo mapping from Konflux snapshot files

**Kustomize overlay context** is extracted from the operator's Go source and injected into each component agent's prompt so it knows exactly which kustomize overlay the rhods-operator applies for RHOAI:
- The `*_support.go` files in `internal/controller/components/{component}/` define per-platform overlay paths (e.g., `rhoai/onprem` for dashboard, `overlays/rhoai` for datasciencepipelines, `overlays/odh` for kserve)
- Image parameter mappings (`imageParamMap` / `imagesMap`) showing which `RELATED_IMAGE_*` env vars override params.env placeholders at deploy time
- The actual `params.env` values from `prefetched-manifests/{component}/{overlay}/params.env`, giving the agent concrete image references and configuration defaults
- This ensures agents analyze the correct overlay kustomization.yaml rather than the base, and understand what parameters the operator injects

### Phase 4: Generate platform architecture (`generate-platform-architecture`)

Component summaries are written directly into the platform architecture directory:

```
architecture/
  rhoai-3.4-ea.1/
    kserve.md
    odh-dashboard.md
    notebooks.md
    kserve/
      analyzer.json              # exact accepted analyzer input
      synthesis.json             # original synthesis response envelope
      document.json              # accepted authority and recovery provenance
      diagrams/                  # optional component derivatives + metadata
      .analyzer/
    PLATFORM.md
    INDEX.md
    diagrams/                    # platform diagrams and legacy component diagrams
      kserve-component.mmd
      kserve-component.png
      ...
  rhoai-3.4-ea.2/
    ...
```

Spawns a Claude agent that reads all component `.md` files in an architecture platform directory and produces a `PLATFORM.md` — an aggregated platform-level architecture document. Build metadata (OCP versions, shipped image topology) is included in the prompt.

### Phase 5: Generate version index (`generate-index`)

Builds a deterministic `INDEX.md` from the version's component map, structured
platform and overlay metadata, available analyzer metadata, and headings in
promoted documents. This phase reads local artifacts only and makes no agent or
network calls. It keeps inventory presence separate from current, planned,
not-integrated, and unknown integration status.

Integration status precedence is active release-applicable overlay metadata,
then `platforms.yaml`, then an explicit non-planned status in the version's
component map. Conflicting active overlays fail closed to `unknown`. Planned
integration must come from `platforms.yaml` or structured active overlay
frontmatter; free-form overlay prose is never interpreted.

For example, platform configuration can record an explicit status by canonical
alias:

```yaml
rhoai-3.6-ea.2:
  integration_status:
    praxis-policy: not-integrated
```

### Phase 6: Generate diagrams (`generate-diagrams`)

Spawns Claude agents that read each component and platform `.md` file and produce:
- Mermaid diagrams (`.mmd`): component, dataflow, dependencies, RBAC, security/network
- C4 context diagrams (`.dsl`)
- PNG renders via `scripts/generate_diagram_pngs.py`

For newly published structured components, the three nested JSON files and the
existing flat component Markdown form one validated snapshot. `document.json`
binds the analyzer and synthesis bytes; the Markdown marker binds the exact
document bytes and renderer output. Typed queries can use a valid JSON-only
authority, while repository linting and release packaging require all four
files, including Markdown. Component diagrams are written below
`{component}/diagrams/` with document/generator/status metadata. Platform and
explicitly legacy component diagrams retain the version-level `diagrams/` path.

## Project structure

```
main.py                          # CLI entry point, six phases + preparation stages
lib/
  fetch.py                       # Phase 1: gh-org-clone wrapper
  manifest_parser.py             # Phase 2: manifest parsing, adjacent discovery,
                                 #          build-config/bundle metadata extraction,
                                 #          kustomize overlay context extraction
  version_index.py               # Phase 5: deterministic version navigation
scripts/
  generate_diagram_pngs.py       # Mermaid→PNG rendering
.claude/skills/                  # Shared Claude/Codex agent skills
  repo-to-architecture-summary/  # Phase 3 skill
  aggregate-platform-architecture/# Phase 4 skill
  generate-component-diagrams/   # Phase 6 skill
architecture/                    # Output: organized architecture docs + diagrams
checkouts/                       # Cloned repositories (gitignored)
logs/                            # Agent execution logs per phase
```

## Usage

### Full pipeline

```bash
# RHOAI (specific version)
python main.py all --platform=rhoai --branch=rhoai-3.4-ea.1 --model=opus

# ODH
python main.py all --platform=odh --model=sonnet
```

Agent-backed commands default to the Claude harness. To use the existing Codex
login and the model configured for Codex, select the Codex harness without a
model override:

```bash
uv run main.py pipeline --platform=rhoai-3.6-ea.2 \
  --phase=fetch --phase=discover-components \
  --harness=codex --max-concurrent=20 --force
```

Pass `--model=<codex-model-id>` only when a specific Codex model is required.
Both harnesses use the checked-in skills under `.claude/skills/`. Codex runs in
its workspace-write sandbox; `--strace` is currently Claude-only. Codex app-server
notifications are flushed as JSONL to each agent log while a turn is running,
and direct single-agent runs stream agent messages plus concise command lifecycle
notices to the terminal.

Codex discovery uses a temporary workspace containing a runtime copy of the
shared discovery skill and an empty output directory. Checkout paths are supplied
explicitly. The parent validates the resulting map before atomically promoting
it into the architecture tree and applying sync-config/provenance processing.
Failed or invalid candidates leave the previous map intact. This keeps previous
results and pipeline development instructions out of the initial worker context;
it does not provide container-level read isolation.

### Individual phases

```bash
# Fetch repos
python main.py fetch red-hat-data-services --branch rhoai-3.4-ea.1

# Parse manifests (see what components are discovered)
python main.py parse-manifests --platform=rhoai --branch=rhoai-3.4-ea.1

# Generate architecture for a single component
python main.py generate-architecture --platform=rhoai --branch=rhoai-3.4-ea.1 \
  --component=kube-auth-proxy --model=sonnet

# Regenerate a specific component
python main.py generate-architecture --platform=rhoai --branch=rhoai-3.4-ea.1 \
  --component=kserve --force --model=opus

# Generate platform-level doc
python main.py generate-platform-architecture --platform=rhoai --version=3.4-ea.1

# Generate a version index without an agent
uv run main.py generate-index --platform=rhoai-3.4-ea.1

# Rebuild it once after selected component generation
uv run main.py pipeline --platform=rhoai-3.4-ea.1 \
  --phase=generate-architecture --phase=generate-index --component=kserve

# Generate diagrams
python main.py generate-diagrams --platform=rhoai --version=3.4-ea.1
```

### Useful flags

| Flag | Phase | Description |
|------|-------|-------------|
| `--harness` | 2b, 3, 4, 6 | Agent harness: `claude` (default) or `codex` |
| `--model` | 2b, 3, 4, 6 | Optional model understood by the selected harness |
| `--max-agent-turns` | 3 | Optional per-agent Claude SDK turn cap |
| `--max-budget-usd` | 3 | Optional per-agent Claude API-equivalent dollar cap |
| `--max-concurrent` | 3, 4, 6 | Parallel agent count (default: 5) |
| `--component` | 3 | Process a single component by key name |
| `--force` | 3 | Delete existing architecture and regenerate |
| `--force-regenerate` | 6 | Regenerate diagrams even if they exist |
| `--limit` | 3, 4, 6 | Cap number of items to process |

## Build metadata extraction

For RHOAI, the pipeline reads three files from `RHOAI-Build-Config/`:

| File | What it provides |
|------|------------------|
| `config/build-config.yaml` | Supported OCP versions (e.g. v4.19, v4.20, v4.21) |
| `bundle/csv-patch.yaml` | CPU architectures, min kube version, OLM feature flags |
| `bundle/bundle-patch.yaml` | Product version, all RELATED_IMAGE entries (84 images for 3.4-ea.1) |
| `release/*/stage/*/snapshot-components/*.yaml` | Konflux snapshot: container image → source repo + commit mapping |

This metadata is injected into agent prompts so they can factor in platform constraints (e.g., which k8s APIs are available given the OCP version range, multi-arch requirements, FIPS compliance).

## Kustomize overlay context

For each RHOAI component, the pipeline parses the operator's Go source to extract deployment context:

| Source | What it provides |
|--------|------------------|
| `internal/controller/components/{dir}/*_support.go` | Per-platform overlay paths (e.g., `rhoai/onprem`, `overlays/rhoai`) and image parameter mappings (`imageParamMap` / `imagesMap`) |
| `internal/controller/components/{dir}/*.go` | Named const source paths (e.g., `kserveManifestSourcePath = "overlays/odh"`) and computed kustomize variables (e.g., `sectionTitle`) |
| `prefetched-manifests/{key}/{overlay}/params.env` | Default image references and configuration values injected by the operator at deploy time |

The manifest key maps 1:1 to the operator component directory for most components. Special cases:
- `maas` → `modelsasservice/` directory
- `workbenches/*` (sub-keys like `workbenches/kf-notebook-controller`) → `workbenches/` directory
- `operator` → skipped (no kustomize overlay context for the operator itself)

This context is injected into Phase 3 agent prompts so they start analysis from the correct overlay `kustomization.yaml` rather than the base, and understand which image parameters and config values the operator substitutes.

## Requirements

- Python 3.13+
- `gh-org-clone` CLI tool (auto-installed to `./bin` if not found in PATH)
- Go (required only if `gh-org-clone` needs to be built)
- `claude-agent-sdk` (installed via `uv sync`)
- `pyyaml`
- `ANTHROPIC_API_KEY` or Vertex AI credentials (see `.env.example`)
- `GITHUB_TOKEN` (optional, recommended to avoid API rate limits — see `.env.example`)

## Setup

```bash
uv sync
cp .env.example .env
# Edit .env with API credentials and optionally add GITHUB_TOKEN
```

**GitHub Token**: To avoid GitHub API rate limits when cloning many repositories, add a GitHub Personal Access Token to your `.env` file:
```bash
GITHUB_TOKEN=ghp_your_token_here
```
Create a token at https://github.com/settings/tokens with `repo` scope (for private repos) or `public_repo` scope (for public repos only).
