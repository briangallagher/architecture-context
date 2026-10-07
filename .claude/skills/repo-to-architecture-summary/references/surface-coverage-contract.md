# Architecture Surface Coverage Contract

The orchestrator may supply `--surface-inventory=PATH` and
`--surface-coverage-output=PATH` for an analyzer-backed component. The
inventory is a bounded planning input. It contains source-linked candidates and
role-based questions; candidates are not established facts.

Read the inventory after the analyzer context and baseline, before source
inspection. Plan work by surface priority. Resolving one surface does not close
its parent category or another surface in that category. Reuse analyzer facts
and source evidence already encountered. Do not repeat a source read merely to
create coverage metadata.

The sidecar uses `architecture-surface-coverage/v1` and retains these top-level
fields from the inventory:

- `component`, `inventory_id`, and `component_roles`.
- `applicability_observations`, including any uncertain surface that was not
  nominated because the analyzer supplied no concrete applicability signal.
  These records are planning context; do not turn them into documented,
  unresolved, or not-applicable surface dispositions without new evidence.
- Every seeded record in `surfaces`, with its original stable `id`, parent
  category, component role, applicability basis, question, priority, and
  candidate locations. Newly discovered important surfaces may be appended.
- `observed_reads` and `validator_findings` remain empty; the orchestrator owns
  those observations after generation.

For every surface, set:

- `evidence_status`: `available`, `unavailable`, or `unresolved`.
- `claim_support`: `supported`, `unsupported`, or `uncertain`. A documented
  disposition requires `supported`; use `unsupported` when the proposed wording
  is stronger than the evidence.
- `disposition`: `documented`, `unresolved`, or `not-applicable`.
- `evidence`: an array of objects with `kind` and `reference`. Kinds are
  `analyzer-fact`, `source-read`, `source-candidate`, or `manifest`. Source and
  manifest references require a repository-relative path with a numeric line or
  line range. Analyzer facts may use an exact JSON pointer. A
  `source-candidate` is a planning location and does not prove a documented or
  not-applicable claim. Use an exact analyzer fact, an actually observed source
  range, or an independently checked repository manifest for claim evidence.
- `document_reference`: for `documented`, either a table reference such as
  `{"kind":"table-row","category":"authentication","row_key":["Operator metrics","HTTPS"],"expected_cells":{"auth_mechanism":"controller-runtime filter"}}`
  or a section reference such as
  `{"kind":"section","section":"Security > FIPS Compliance","fact_identity":"runtime compliance is not verified"}`.
- `reason`: a concise explanation of the disposition.
- `remaining_question`: a concrete question for `unresolved`; use an empty
  string for closed surfaces.

For `not-applicable`, also change `applicability.status` to `not-applicable` and
provide evidence whose references also appear in the top-level evidence array.
The evidence must be an existing exact analyzer JSON pointer, an observed source
read, or a repository manifest reference; a source candidate or an assertion
based only on the component name or apparent role is insufficient. Keep the
seeded applicability basis unchanged. If applicability remains uncertain,
retain an unresolved record.

The orchestrator preserves the inventory identity, component roles, and seeded
applicability observations and surface fields. Do not change a seeded parent category, component role,
applicability basis, question, priority, or candidate location. Agent-written
source-read justifications describe evidence use; only harness telemetry becomes
`observed_reads`. When telemetry is unavailable, the validator reports that
inspection cannot be verified without treating the source evidence as absent.
For bounded source evidence, an observed range must fully cover the cited range;
candidate inspection needs an overlapping range. An open-ended `N-unknown`
observation covers from `N` through end of file, while a bare `unknown` range
does not verify bounded evidence or candidate inspection.

Before finishing, perform an evidence-to-output review:

1. Account for each seeded required surface and any important surface found in
   bounded inspection.
2. Map every available finding to an exact candidate document reference. If the
   evidence is available but the document does not express it, repair the
   candidate or leave an explicit synthesis omission; never label it documented.
3. Check conditional behavior, named-resource predicates, package/controller
   identity, gateway authentication modes, and FIPS limitations without
   strengthening the source evidence.
4. Use one justified targeted follow-up when a remaining required question has
   a concrete location. If the evidence is dynamic, unavailable, or still
   ambiguous, stop and record it as unresolved.

The orchestrator validates the completed sidecar against the assembled and
promoted document in warning-only mode. It separately reports structural
validity, documented/unresolved/not-applicable counts, unresolved
safety-critical surfaces, extraction gaps, inspection gaps, synthesis
omissions, merge losses, and unverifiable coverage claims. A candidate-only row
that merge policy rejects cannot remain documented.
