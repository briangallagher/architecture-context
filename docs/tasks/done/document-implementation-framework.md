# Task: Document a Reusable Implementation Framework

Status: done. Requested and completed 2026-09-06.

Scope: extract the agreed coordination, implementation, and independent-review
workflow into reusable repository guidance; link the handbook and active plan.
No runtime changes, live runs, model launches, or commits are in scope.

Acceptance:

- IF-01: Persist role-based guidance with dated model defaults and substitution rules.
- IF-02: Define independent review, requirement evidence, proportional gates,
  bounded delegation/cost, and separate implementation/rollout authorization.
- IF-03: Link the handbook, PLAN.md, and structured-assembly plan/task without
  modifying original review provenance or generated architecture.
- IF-04: Check links and whitespace; record validation and review limitations.

Result: IF-01 through IF-04 satisfied by
[the framework](../../notes/implementation-framework.md), handbook and index
links, and the structured-assembly execution/task updates. Referenced target
files exist; `git diff --check` passed. Original proposal SHA-256 remains
`65a62e75850cadf9ffefa9c6b6cd6b55b2b1d3e8058142a479d11fe0d1157217`.
Documentation-only self-check; no independent reviewer was launched or claimed.
No runtime tests, generated architecture edits, or implementation runs performed.

Follow-up 2026-09-06: Added the user's explicit rate-limit requirement: no forced
or automatic degradation or fallback, pause affected work until limits replenish,
and resume with the same model/settings. Clarified precedence over reviewer
fallbacks and added checkpoint/retry/resume evidence. Documentation-only
consistency review and `git diff --check`; no runtime enforcement implemented.

Follow-up 2026-09-06: Documented coordinator-mediated CLI handoffs in both the
framework and consolidated plan, including the distinct `claude -p` and
`codex exec` syntax, bounded prompts, captured results/session IDs, fresh
reviewer sessions, durable check-ins, and rate-limit-safe resumption. No direct
messaging service or wrapper implementation is required. Documentation-only
self-check: link targets/heading, original proposal hash, and diff whitespace;
no agents launched or independent review claimed.
