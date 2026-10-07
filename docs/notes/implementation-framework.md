# Implementation and Independent Review Framework

## Scope and authority

Use this framework for future repository implementation work. It complements
[AGENTS.md](../../AGENTS.md), [PLAN.md](../../PLAN.md), and the task ledger;
it does not authorize implementation, live runs, spending, commits, or rollout.
User instructions and task-specific constraints take precedence. Record any
task-specific departures and their reasons in the task before relying on them.

The referenced `docs/notes/agentic_work_ledger.md` is currently absent. This
document does not replace that specification; follow the available handbook's
task, bug, ADR, and session-log conventions until it is restored.

## Roles and current defaults

Separate responsibilities before choosing models. The following assignments
record the user's 2026-09-06 discussion, not a benchmark ranking or a guarantee
of availability. Future tasks may select other models without changing the
role and evidence requirements.

| Role | Responsibility | Current default |
|---|---|---|
| Lead/coordinator | Own scope, requirement coverage, contracts, delegation, integration, and gate scheduling | GPT Astra |
| Implementer | Deliver bounded changes and reproducible verification | GPT Sol |
| Independent gate reviewer | Inspect contracts, changes, and evidence; independently test consequential claims | Claude Fable 5.1 |
| Fallback reviewer | Perform the same independent review for an agreed non-rate-limit availability or budget substitution; never bypass a rate limit | Claude Opus 5 |

At kickoff, record the actual harness, requested model identifier, and resolved
model identity when exposed. These human-readable defaults are not CLI aliases.
Verify access rather than assuming it; do not silently substitute a model or
claim delegation that did not occur. Obtain direction if substitution changes
an explicit user choice, authorization, or agreed spending limit.

Prefer different model families for implementation and review when practical,
but model diversity is not proof of independence or correctness. A fresh session
of the same model can review if the limitation is disclosed and the task permits
it. The coordinator may integrate code but cannot independently approve its own
substantive contributions. A contract author cannot be its sole independent
approver, even if another agent implements it. A reviewer who supplies a
substantive replacement design or patch becomes an author of that contribution;
assign a separate reviewer to it.

## Rate limits: pause, never degrade

Forced or automatic model degradation when rate limits are exceeded is prohibited.
This applies to coordinators, implementers, reviewers, delegated workers, and
harness/provider fallback behavior. Do not switch models, lower reasoning effort,
reduce required review, or reassign the rate-limited role to another agent to
keep the work moving. The fallback reviewer and budget provisions above do not
authorize rate-limit-driven substitution.

When a rate limit is reached:

1. Checkpoint progress and pause the affected work and any dependent phase gates.
   Notify the user and record the selected model, limit event, pending action,
   and provider-reported retry/reset time when available.
2. Wait for the rate-limit window or quota to replenish using the available
   wait/resume mechanism. Honor retry/reset guidance; if none is supplied, use
   bounded backoff checks rather than inventing a reset time or busy retrying.
3. Resume the pending work with the same selected model and reasoning settings
   once capacity is available. Record the actual resumption and verify model
   identity when exposed. Never treat the pause as successful completion or a
   waived review gate.

Disable automatic model fallback where configurable. If the harness cannot
prevent substitution or reliably preserve the requested model, stop that launch
and report the limitation rather than accepting a downgraded run. If waiting
cannot persist in the current session, leave a durable paused checkpoint for
resumption with the same model; do not substitute one to finish the turn.
Only a new explicit user instruction can change this policy for a task; an
agent-authored plan exception, deadline, or cost preference cannot override it.

## Scale the process to the work

Small, low-risk edits need a concise task record and proportionate checks, not a
multi-model team. State whether independent review occurred; never describe
self-checks as independent acceptance. Cross-cutting changes to contracts,
persistence, caching, security, compatibility, or multiple consumers require
explicit phase gates and independent review. Existing plan-specific gates still
apply regardless of apparent task size.

Use one coordinator and only as many workers as the dependency structure needs.
Reserve expensive review for consequential gates. Agree on a review/repair budget
and escalation point at kickoff; reaching it leaves the gate unaccepted and
requires direction, not automatic approval or an indefinite review loop.

## Communication and CLI handoffs

Default to coordinator-mediated shell/subprocess invocation and shared task
records. Direct worker-to-worker messaging, a messaging service, and an SDK
integration are not prerequisites. The coordinator sends bounded assignments,
receives results, routes questions and repairs, and records gate decisions.
The durable task record is authoritative; stdout is a notification/evidence
channel, not a substitute for that record.

The basic non-interactive invocation shapes, checked against the installed CLIs
on 2026-09-06, are:

```bash
claude -p --model MODEL "Your assignment"
codex exec --model MODEL "Your assignment"
```

`MODEL` is a placeholder for the exact agreed model identifier. Claude's `-p`
means `--print`; Codex's `-p` means `--profile`, not prompt/print. Codex uses
`exec` for non-interactive work. Verify installed `--help` before constructing
launch, output, effort, permission, or resume arguments; flags are not portable
between harnesses. These examples show syntax only, not a complete launch policy.

A direct launch or thin wrapper must:

- Persist a bounded assignment with task/phase/requirement IDs, owned paths,
  input identity, acceptance checks, output location, and stop conditions. For
  long prompts, use a prompt file and the harness's supported stdin mechanism
  or pass its contents as a single subprocess argument; do not interpolate
  untrusted prompt content into an evaluated shell command.
- Set the working directory, explicit model, supported reasoning settings, and
  authorized permissions. Preserve the agreed login/authentication route; do
  not assume shell orchestration requires new API credentials or permission
  bypasses. Avoid logging credentials or raw environment dumps.
- Capture stdout/events, stderr, exit status, final response, and session ID
  when exposed in a unique attempt location linked from the task. Record actual
  model identity when available. Inspect the result and required evidence: a
  zero exit status alone does not establish completion or gate acceptance.
- Route worker check-ins at ambiguity, blocker, scope/interface change, and
  completion boundaries. A non-interactive worker without live messaging must
  return or checkpoint a question for the coordinator; it must not assume an
  answer. Resume through an explicit session ID or a new bounded handoff.
- Start independent reviewers in fresh sessions with the requirements, reviewed
  input identity, diff, and evidence packet—not a continuation of the author's
  conversation. Freeze reviewed inputs or record their exact snapshot so that
  concurrent changes cannot silently invalidate acceptance. Serialize shared
  edits and ledger updates; only parallelize disjoint ownership.
- Apply the rate-limit policy above: checkpoint and pause affected work, wait,
  then resume with the same model/settings. Do not pass automatic fallback
  options. If a session cannot be resumed, reconcile existing edits before a
  new same-model handoff to avoid blindly replaying partially completed work.

Keep prompts and detailed invocation logs in task-specific run storage, with
durable summaries and evidence links in the ledger. Do not forward full
transcripts or poll continuously. Record that a worker ran only after observing
the actual invocation; writing an assignment file is not agent communication.
If launching another harness is unavailable or unauthorized, report the missing
capability and leave an explicit manual handoff rather than claiming execution.
This protocol can be followed with shell tools; a reusable wrapper is optional
and must not become an unrelated infrastructure prerequisite.

For Codex implementation assignments and Claude independent reviews, the
repository provides
[`scripts/run_implementation_worker.py`](../../scripts/run_implementation_worker.py).
Pass a saved prompt, a new attempt directory, and explicit model/effort settings;
select `--harness claude` for the review harness (default: Codex).
See [its usage example](../../scripts/README.md#run_implementation_workerpy).
It retains invocation evidence without launcher retries or model fallback.
Review the resulting artifacts and errors under the same gates described here.

## Execution loop

1. **Scope and baseline.** Read the handbook and active plan. Move an authorized
   pending task to current, or create a scoped task. Record the requested outcome,
   non-goals, authority limits, existing dirty files, and relevant baseline checks.
   Preserve unrelated work and identify overlapping ownership before edits.
2. **Requirements and phases.** Assign stable requirement IDs, each with an
   acceptance check. Include affected producers and consumers, compatibility,
   preservation, failure behavior, and migration/rollback where relevant. Give
   each phase a bounded deliverable and entry/exit criteria. Do not silently
   remove requirements; record superseded/deferred requirements and rationale.
3. **Delegate bounded work.** Give each worker owned paths, interfaces, requirement
   IDs, fixtures, checks, dependencies, and stop conditions. Parallelize disjoint
   work; serialize shared contracts and integration. Workers must report actual
   changes, checks, and uncertainty, not merely a completion assertion.
4. **Implement and verify.** Build the smallest coherent phase. Add regressions
   for discovered defects. Record commands, results, limitations, and exact input
   identity. Distinguish new failures from evidenced baseline failures. Passing
   schemas or tests does not by itself prove factual correctness.
5. **Review the gate.** Supply the packet below to a fresh reviewer. The reviewer
   inspects actual artifacts and source, not only the implementer's summary, and
   independently runs risk-relevant checks. Return PASS, REQUEST_CHANGES, or
   BLOCKED with requirement IDs and concrete evidence. Unverified requirements
   remain open; lack of review access is not a pass.
6. **Repair and re-review.** Address findings and retain their history. Recheck
   affected requirements and integration boundaries. Any change after approval
   requires impact assessment and renewed review of affected guarantees.
7. **Close and hand off.** Mark implementation done only when required deliverables
   and gates are satisfied. Report remaining rollout decisions separately.
   Record architectural decisions as ADRs, bugs in the bug ledger, and checkpoints
   in the session log. Do not close a bug merely because a plan or diagnostic exists.

## Reusable task and gate packet

Keep this record in the existing task or link to durable evidence; do not invent
a parallel tracking system or copy entire transcripts into each review.

```text
Task / plan / phase:
Scope and non-goals:
Requirements: ID -> acceptance check -> evidence -> status
Roles: agent/session, harness, requested/resolved model, owned contributions
Authority and cost limits:
Rate-limit pause: selected model/settings, checkpoint, retry/reset time, resumption
Baseline: commit plus dirty diff/input hashes where relevant
Changed paths and contract/consumer impact:
Verification: exact commands, outcomes, fixtures/input identities
Preservation, negative-case, compatibility and recovery evidence:
Open findings, baseline failures, deferred work and reasons:
Reviewer: identity/session, independence limits, checks actually performed
Verdict: PASS | REQUEST_CHANGES | BLOCKED, with findings and evidence
Next action / owner:
Implementation status / live-evaluation status / rollout status:
```

Commit IDs alone do not identify a dirty worktree. Record enough diff, build,
fixture, and source provenance to reproduce material claims without committing
unrelated user changes. Retain failed evidence; a successful replay does not
retroactively make an earlier rejected run pass.

## Cost and operational boundaries

Reuse bounded evidence packets and durable checkpoints instead of repeatedly
reconstructing the entire task. Re-review changed surfaces and their dependencies;
retain broader final integration checks where the plan requires them. Record
calls, tokens, elapsed time, and monetary cost when available, keeping measured
values separate from estimates. Do not promise task savings from token prices.

Independent implementation-review agents are distinct from workers launched by
the product being changed. This framework does not enable pipeline workers,
change enforcement policy, or relax any product rollout gate. Local validation,
live evaluation, default adoption, and deployment are separate outcomes with
their own authorization. If required capability or authority is absent, record
the limitation and request direction rather than fabricating completion.

## Origin and maintenance

Extracted from the [structured-component assembly plan](../plans/structured-component-assembly-consolidated.md)
and the user's model-role discussion on 2026-09-06. The
[original proposal and reviews](../plans/structured-component-assembly.md) remain
historical provenance. New plans should link here and state only their overrides,
actual assignments, and task-specific gates. Update dated model defaults
explicitly; they must not rewrite the model identities in historical task records.
