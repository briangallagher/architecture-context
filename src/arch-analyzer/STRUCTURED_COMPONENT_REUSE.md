# Structured component assembly: phase 2 reuse contract

This document describes the phase-two whole-component reuse gate. It does not
wire a new pipeline route, publish snapshots, invoke a live model, or change the
phase-one accepted document, patch, policy, or renderer APIs. Phase 3 can call
the bounded interface in `lib/structured_component_reuse.py` after it supplies
the current accepted-document revalidation operation and the normal synthesis
operation.

## Decision flow and phase boundary

Reuse is opt-in. A target platform must name `reuse_from`; the resolver rejects
missing, empty, self-referential, and cyclic edges. A predecessor snapshot is
selected by explicit platform plus intersecting canonical component name or
declared alias. The canonical repository identity must also match, and an
ambiguous alias is an error. Directory names are never repository identity.

Snapshots are immutable records. A reuse decision requires an accepted
`structured-component/v1` predecessor, a different target platform, matching
component/repository identity, exact producer compatibility, an unchanged source
run, complete independently recorded dependencies, an equal semantic input
identity, and intact accepted-document and original-synthesis hashes. Rejected
and legacy snapshots are not upgraded or backfilled. Explicit refresh always
misses.

Every prospective target carries a frozen current normalization and its integrity
hash. That document must come from the actual target analyzer input and bind its
component/repository identity, target version scope, source commit, extraction
timestamp, analyzer bundle fingerprint, producer versions, rendered revision,
and deterministic recent-change facts. Missing, malformed, stale, or mismatched
target normalization is an explicit miss; phase 2 never fills these values from
the predecessor or guesses them from the target platform name.

On an otherwise eligible hit, the caller must revalidate all four current gates:
accepted-document schema, typed references, patch policy, and current acceptance
rules. Those results must come from the trusted current validator, not model
assertions. The returned document must equal the frozen target normalization,
not merely satisfy callback booleans. An invariant comparison permits only the
reviewed cross-version refresh fields: target version/source/integration identity,
analyzer fingerprint and extraction time, evidence revisions, deterministic
recent-change facts/accounting/rendering, and fingerprints rebound by current
typed patches and trusted policies. Patch operation IDs, dispositions, authority,
decisions, accepted facts, sections, and all other content remain invariant.
Arbitrary fact loss, semantic changes, unrelated output, or an input-unbound
predecessor converts the decision to an auditable miss. A missing, false, or
exceptional validator check likewise converts the decision to a miss.
The hit returns a detached copy of that freshly validated document while
retaining the predecessor's exact synthesis bytes and response identity. It also
returns prior/target platforms, components, repositories, exact and semantic
comparison hashes, source identities, snapshot identity, synthesis integrity,
revalidation results, returned/expected document hashes, and consistency errors.
It makes zero synthesis calls. Every miss calls the single supplied normal
synthesis callback exactly once.

The returned provenance is deliberately a bounded in-memory value rather than a
new persistence envelope. Phase 3 must place it into the accepted shared
snapshot contract it owns; phase 2 does not preempt that shape.

## Exact identity, semantic identity, and compatibility

The exact identity is an audit hash of the complete analyzer value, complete
component configuration, overlays, contracts, settings, and dependency record.
It therefore changes for timestamps, commit and release labels, schema labels,
scan statistics, and platform integration metadata even where those values are
not synthesis semantics. The synthesis response is never part of either input
identity and is integrity-hashed separately.

The versioned semantic identity includes every analyzer field supplied to
reusable synthesis except this reviewed list:

- analyzer extraction/generation timestamps, commit labels, analyzer/schema
  labels, and checkout-root prefixes, because they identify production or
  location rather than component meaning;
- top-level `scan_statistics`, because scan effort is operational metadata and
  is now separated from accepted facts;
- the two legacy `summary:scanned <count> ...` coverage evidence strings, solely
  for analyzer documents produced before that separation;
- `recent_changes`, because reusable synthesis must not consume it and the
  target document refreshes it deterministically during current revalidation;
- target release and platform-integration labels in the per-component
  configuration, because platform assembly owns them.

No general evidence exclusion exists. Summary, dependencies and versions,
permissions, conditions, ordering, RBAC, coverage findings, limitations,
synthesis and cross-cutting evidence, gap questions, cross-references, actual
component configuration, overlays, contracts, settings, and internal component
dependencies remain significant. If synthesis later consumes `recent_changes`,
scan statistics, or another presently excluded value, the semantic key version
and payload must change first. Renderer identity never enters synthesis
eligibility, so renderer-only rerendering does not invoke synthesis.

Semantic key version `structured-component-semantic-input/v2` additionally
binds the complete trusted `observations`
object and every model-visible source selection: path, start/end line, selected
text, whole-file hash and line count, origin, and justification. A whole-file
hash alone is not a selection identity. Only the source selection's `revision`
label is excluded, for the same documented reason as analyzer `commit_sha`;
the target still independently binds the current commit, file bytes, and Go
normalization. Changing observations or an excerpt range is therefore a miss,
while a new commit/extraction time/release label with unchanged semantic bytes
can remain a hit.

The model context also distinguishes the parent's requested Claude model value
from its locally resolved identifier. The alias resolution and the exact Claude
CLI implementation selected by the SDK (version output plus executable SHA-256)
are semantic inputs. Changing either therefore misses prior reuse even when the
SDK package version and component evidence are unchanged. The response-producing
`AssistantMessage.model` and auxiliary usage remain separate audit fields.

Compatibility is checked separately and initially admits exact equality only:
analyzer schema version, analyzer version, actual analyzer build identity,
normalizer version, and synthesis contract version. The build helper hashes HEAD
plus tracked and untracked producer source content, so a dirty extractor cannot
hide behind an unchanged version string. Compatible version ranges are not
inferred.

## Independent file and search dependencies

The Claude and Codex harness telemetry retain an additive versioned observation
record without changing their existing fields. It contains independently
observed read paths and ranges, resolved search roots, patterns, all represented
options (including ordered Codex `rg` argv), outcomes, and any source-reading
command that could not be classified. Model assertions are not observations.

Before a record becomes reuse-eligible, the parent must reconcile every observed
or explicitly supplied read with exactly one non-empty justification. It hashes
the entire supporting file, even when only a range was read. Every observed
search likewise requires a separate justification keyed by its resolved
repository-relative root, pattern, options, and tool. The record stores that
relative root and the tracked directory-tree content identity so a negative
finding changes when the searched tree changes. A dirty searched scope is an
explicit miss. For the supported Codex `rg` subset, it also stores an
independently replayed result identity and replay execution-context identity.
Consequently an ignore rule outside the recorded subtree cannot silently change
a positive or negative result. Absolute checkout roots are retained only in the
observation and are excluded from the reusable payload.

Generated and external input files carry their own content hashes. Every
untracked checkout path present at the source snapshot must be explicitly
accounted and hashed or reuse is unavailable. Both start and end snapshots bind
HEAD, the tracked tree, the actual working tree, dirty paths, and untracked
paths; any midrun change is a miss. Equal HEAD alone is never sufficient.
Porcelain rename/copy records retain and check both source and destination paths.
Missing observation arrays or versions, missing or extra read/search
justifications, missing whole-file hashes, unaccounted untracked files, and any
unclassified source-reading command are conservative misses or record
construction errors.

Codex classifies SDK-confirmed successful reads and a small direct,
shell-uncomposed `rg` subset with relative in-checkout roots. A reusable search
must have the completed SDK output, explicit cwd, and either a sole raw command
or identical raw/action command,
normal exit 0 or no-match exit 1, and the flags `--no-config`,
`--no-ignore-global`, `--color never`, and `--sort path`. Those flags make the
replay deterministic and disable the installed ripgrep's ambient
`RIPGREP_CONFIG_PATH` and Git global-ignore inputs. The similarly named
`RG_CONFIG_PATH` is not recognized by installed ripgrep 15.2.0, but replay also
removes it rather than assigning it unverified meaning. Both initial verification
and target replay reparse the actual persisted argv with the same direct-command
parser used for telemetry. Before any subprocess can execute those inputs, the
parser requires argv to represent the recorded pattern, root, complete options,
and cwd exactly. It rejects preprocessors, external pattern files, absolute or
escaping roots, unknown or scope-expanding flags, missing isolation, and mismatched
representations. The parent then resolves and
hashes the actual `rg` executable/version, replays the exact argv under recorded
cwd with `LC_ALL=C` and `NO_COLOR=1`, and requires the replay result to equal the
SDK-observed result before the predecessor record is complete. Target records
use the explicit target-replay mode to run the predecessor argv again.

The SDK-observed result identity is retained separately for audit and is not a
reusable dependency claim. Only the independently computed replay result and
context identities enter the semantic payload; model prose and parent-supplied
configuration hashes do not. If completed output, cwd, executable, or replay is
unavailable, reuse is unavailable. Shell wrappers, including `sh -c` and
`bash -lc`, are rejected because arbitrary startup behavior is not described by
the parent process environment. Exit 0 and exit 1 retain distinct outcomes,
complete options, ordered argv, actual pattern, and roots. Supported `-g` and
`--glob` rules are replayed, including inclusion globs that override ignore
rules; an explicitly named file root is likewise replayed under ripgrep's
documented ignore override.

Every command/action not positively classified defaults to unclassified; the
only shell commands treated as provably non-reading are exact single-word `:`,
`true`, `false`, and `pwd`. Pipes, substitutions, composition, redirections,
shell wrappers in raw or inner action shapes, variable/home expansion tokens,
unmapped actions, malformed commands, absolute/outside-checkout roots, failed
reads, unknown options, and missing replay records make the observation
incomplete. Modes that can expand `rg` beyond the prior tracked-tree boundary
(`--no-ignore`, `--no-ignore-vcs`, `--hidden`, `--follow`, and `-L`) remain
rejected. Local and ancestor `.gitignore`, `.ignore`, `.rgignore`, and repository
exclude behavior is supported only through the verified replay result, so
creating, deleting, or changing those inputs either changes the replay identity
or leaves the exact search semantics unchanged.

Claude records authorized reads and resolved Glob/Grep scopes at its existing
pre-tool hook boundary; this records invocation scope rather than a
post-execution result and is preserved as such in `outcome`. Because the hook
cannot bind actual result bytes and a subtree hash does not cover ancestor/local
ignore behavior, every current Claude Glob/Grep observation explicitly makes the
dependency record incomplete. Its actual resolved path, pattern, and options
remain available for audit and a future post-result verifier; verified direct and
supplied file reads remain intact. On unrestricted
routes, any tool not positively handled as Read, Glob/Grep, or a non-reading
Write/Edit is unclassified, including NotebookRead, Task, LS, and WebFetch.
Denied restricted-route calls are not observations because the tools did not
execute. Neither harness turns model prose or claimed reads into observations.

For a target record, the caller must replay the predecessor's recorded read/search
dependencies and independently rehash them against the target checkout. They
must never be reconstructed from model guesses or from claims in the accepted
document. A missing replay record is incomplete and cannot produce a hit.

## Analyzer scan-statistics separation

`model.Input.scan_statistics` is an operational input family, not an accepted
fact family. Authentication file counts and internal-dependency file/alias scan
counts are emitted there rather than as category evidence. The structured
registry explicitly marks the field as metadata and assembly tests ensure it
does not become a fact. Exact audit identity still retains it; semantic reuse
does not.

## Current limitations

- Reuse is whole-component only. There is no section-level reuse or partial
  historical reconstruction.
- A predecessor that required an evidence follow-up retains its completed bundle,
  including the added source selection. A fresh target starts from parent-nominated
  evidence and therefore conservatively misses instead of inventing the prior
  follow-up request or a broader reuse algorithm.
- No pipeline route or live platform configuration selects this gate yet.
- No legacy snapshot becomes eligible, and historical observations are never
  inferred from prose, model claims, or prior documents.
- Search identities retain tracked repository trees plus verified result replay
  for the bounded Codex subset. A dirty searched scope is rejected rather than
  attempting a mixed tracked/untracked negative-evidence identity. Codex shell
  wrappers and ambient-config searches are conservative misses. Claude Glob/Grep
  observations are audit-only and conservatively miss until post-execution result
  verification exists.
- Codex command parsing deliberately rejects an `rg` pattern containing `|`
  even when shell quoting would make it literal alternation. This retained false
  miss is safe but can lower reuse eligibility. The original discovery-metrics
  `rg` bug remains open; additive reuse observations do not close or replace it.
- File dependencies are whole-file hashes; citation relocation and semantic
  source diffing are not attempted.
- An explicitly named ignored, untracked file search root can be invisible to the
  tracked tree and working-tree snapshots. Its current binding is the verified
  whole-query replay result only; a non-matching edit can therefore leave the
  identity unchanged. This query-result-only boundary is disclosed and is not a
  claim of file-content completeness.
- Phase 3 still owns durable snapshot/envelope publication and integration with
  its bounded synthesis/evidence workflow. It must run current Go normalization
  for the exact target analyzer input and supply that frozen result to
  `ReuseTarget`; typed proposals and reviewed policies must be valid for the
  target bundle/version binding. It must never copy predecessor analyzer metadata
  under a target identity. If it cannot construct that trustworthy target-bound
  normalization while retaining the original synthesis response bytes, reuse is
  unavailable and the one bounded synthesis miss path is required.

## Phase 3 private CLI connection

The opt-in `--structured-synthesis` route now accepts an exact per-component
`reuse_from` object in the trusted parent input: `version_scope` and `component`
are both required. The selected version must also be the current platform's
explicit `reuse_from` edge in the configured platform graph. Missing, self,
cyclic, mismatched, ambiguous-alias, and wrong-repository selections do not fall
back to a guessed version or component. The loader derives exactly one private
location:

```text
architecture/<version>/<component>/.generation/structured/run-record.json
```

That versioned internal record is written last and only after completed Go
assembly. It serializes the reviewed Phase 2 input, compatibility, dependency,
and source-state types and hashes fixed-name staged artifacts. It does not embed
a second model response. The loader rejects missing, failed, malformed,
hash-inconsistent, response-inconsistent, schema-invalid, or renderer-inconsistent
records. Save performs the same analyzer, semantic-input, component-map, response,
and document binding checks before it writes a completed record; an inconsistent
current run is not merely left for a later load to reject. The seam captures the
analyzer, component map, configured component/overlays, initial evidence bundle,
and nominated source bytes once. Identity construction, target Go normalization,
and the model call consume those bound values, and a boundary mutation is restored
and rejected. On a valid candidate the loader parses the retained raw response
again, checks raw and parsed identities, copies typed proposals, and rebinds
their target analyzer
fingerprint/evidence revision and the separately trusted policies, runs current
Go normalization, and then calls the reviewed Phase 2 decision and current Go
validation. The retained `synthesis.json` bytes are never rewritten on a hit.
`--structured-refresh` forces the normal bounded miss path.

The private record is not a fifth published core artifact, a publication
transaction, or evidence that a run was independently accepted. No accepted
document field was added by this connection: the existing Phase 3 `reuse` field
continues to hold the target decision record. Phase 4 must replace the private
loader binding with the hash-linked published four-core snapshot and retain
essential metadata inside those core artifacts before publication.

A syntactically successful response whose producing identity is unknown or
ineligible remains in `synthesis.json` with its raw response and explicit
`reuse_eligibility`; save rejects it and does not write a completed private run
record. A save failure never turns that response into an eligible predecessor or
deletes its audit payload.

### Observed and unobservable context

The parent can directly observe and bind the analyzer JSON, exact component-map
entry, active overlay frontmatter, trusted synthesis input and policies, supplied
source ranges and whole-file hashes, checkout HEAD/tracked/working-tree state,
and the local Go analyzer/normalizer source identity. The structured harness
receives only the parent-built evidence bundle. Project settings and skills are
disabled for this route; Claude is limited to one turn with an empty allowed-tool
set and explicit disallowed tools. These local controls mean tool reads are not
inferred from model prose or accepted through caller booleans.

The Claude adapter asks for plain response text in one SDK turn with tools empty
and explicitly disallowed and project settings disabled. It does not pass the
SDK `output_format` option, so the SDK serializer emits no `--json-schema` flag
and the CLI does not inject its `StructuredOutput` synthetic tool or internal
schema-repair loop. The response schema remains in the parent-built evidence
bundle; the parent retains the raw text first, then performs schema validation
and applies only its configured evidence-follow-up and repair ceilings. The
adapter resolves the CLI through the SDK's own resolver before the call, records
its version output and executable SHA-256 in the model context, and pins the SDK
call to that inspected path. The SDK-bundled and separately installed system CLI
can differ; only the implementation actually selected by the adapter is bound.

The Codex adapter starts one ephemeral read-only thread with deny-all approvals,
empty MCP and dynamic-tool configuration, disabled thread environments, explicit
base/developer instructions, and a response schema. Before reuse is considered,
the production adapter performs a local, turn-free preflight through that same
route: `config/read` binds the isolated effective configuration and `thread/start`
reports the selected model, provider, reasoning effort, service tier, instruction
sources, sandbox, and approval policy. An omitted parent model remains `None` and
is absent from the `thread/start` wire object; there is no `configured-default`
model alias. The resolved identity is used in the evidence bundle and reuse key.
An explicit parent model that resolves differently is rejected before
`turn/start`. Each producing call repeats the local resolution and must match the
preflight before its model turn begins.

The Python SDK 0.147.0 launches its bundled Codex CLI 0.147.0 by default; the
separately installed system CLI 0.153.4 is not this adapter's transport. The
selected bundled executable's version output and SHA-256 are part of the
preflight identity. The bundled CLI's experimental `thread/start` schema supports
`allowProviderModelFallback`, while the same-version generated Python model omits
it. The SDK enables the experimental protocol and its supported low-level client
accepts a JSON object, so the adapter serializes the typed parameters, adds
`allowProviderModelFallback: false`, and sends that exact request. This disables
the documented authoritative-static-catalog replacement of an unavailable
requested model. It is not a claim that every provider-side reroute mechanism is
disabled: a producing-turn `model/rerouted` event is interrupted and rejected,
while an event belonging to another explicit turn is ignored.

This SDK still has no global tool-disable switch. The parent treats only the
installed union's user, assistant, plan, and reasoning text items as passive.
Every other started or completed lifecycle item is rejected, including hook,
command, patch, MCP/dynamic/collaboration, subagent, web, image-view, sleep,
image-generation, mode/compaction, malformed, missing-type, and unknown future
items. The SDK's raw unknown-notification wrapper is handled explicitly. On the
single ephemeral thread, a lifecycle record with a missing or malformed turn
identity is rejected, while a different explicit turn identity is provably
unrelated and ignored. Terminal items are checked before any final answer is
accepted; an `itemsView` other than `full`, or a malformed item collection, is
rejected instead of being treated as an empty passive view. On an active
lifecycle event the parent makes one best-effort interrupt request and
rejects immediately even if that request fails; observing a start or completion
means activity may already have begun or finished, so interruption does not
prove execution was prevented. There is no retry or open-ended tool loop. Local
instruction sources are checked at thread start and must be empty. These controls
and their SDK version are part of the context identity.

Each SDK/app-server launch receives a private temporary `CODEX_HOME` containing
only a private copy of the existing `auth.json`; the user's credential file is
not modified and credential contents are not stored in artifacts. The parent
stages only supported scalar model-selection settings from the existing local
config. Memories, plugins, skills, rules, project configuration, MCP
configuration, and other local settings are absent or explicitly overridden.
The effective isolated configuration is read through `config/read` and hashed
into the context identity. A custom provider definition that cannot be staged
without copying opaque provider internals is an explicit capability failure.
Authentication data and opaque server-side provider implementation remain the
same non-source transport boundary accepted for Claude; they are not fabricated
as source reads or silently described as complete local prompt input.

Reuse also requires a producing envelope whose accepted response reports exactly
the locally resolved requested model and applied resolved provider, effort, and
service-tier settings. The parent selection (including omission), local
resolution, actual response-producing model, and auxiliary
usage are retained separately. Unknown identity,
a rerouted/different producing model, unknown settings, or a tampered eligibility
flag is a miss and cannot be saved as a completed private record. Auxiliary model
usage is retained separately and is not confused with the response-producing
model. Every raw answer is persisted before parsing. Standard UTF-8 text is stored
directly; text containing an unpaired surrogate is stored reversibly as
UTF-8-surrogatepass base64, and its hash covers that representation. Non-standard
nonfinite JSON, numeric overflow, unpaired surrogates, and nesting beyond the
fixed parser bound produce explicit diagnostics and consume only the configured
repair ceiling.

No new credential or authentication path is introduced. The production transport
contract is covered with offline SDK/protocol stubs; no live-provider capability
claim is made by those tests.

A syntactically valid but producer-ineligible answer stays in `synthesis.json`
with its original raw response, producer telemetry, and a `producer-ineligible`
diagnostic. That component is marked failed and no completed private run record
is written, but unrelated components continue. A structured rate-limit refusal
remains different: it is durably recorded and immediately re-raised so no later
component or model call starts.
