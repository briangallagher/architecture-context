# Filter `rhoai.next` Checkouts by Organization Branch Policy

Status: complete 2026-10-07.

## Request

Add a deterministic fetch-stage eligibility policy for `rhoai.next` that
requires repositories from `red-hat-data-services` to expose at least one
remote branch matching a configured regular expression. Do not ask an agent
skill to inspect remote branches.

## Requirements

- **R1 — Scoped config:** Configure the rule only for `rhoai.next` and
  `red-hat-data-services`; leave versioned release fetch behavior unchanged.
- **R2 — Independent checkout target:** Evaluate remote branch names with a
  full-match regex without changing which branch is cloned.
- **R3 — Deterministic filtering:** Record pass/fail reasons and matching
  branches in a fetch artifact; make discovery omit failed repos before
  provenance and LLM classification.
- **R4 — Failure behavior:** A branch-list query failure must fail fetch with
  repo context rather than silently treating the repo as branchless.
- **R5 — Config validation and documentation:** Validate the rule shape and
  regex in `lint_platforms.py`, and document the behavior.

## Verification limits

Do not add or run tests unless the user asks. Use code review and non-test
checks only. No independent reviewer is assigned for this focused change.

## Implementation and verification

- Added a `rhoai.next` policy requiring a complete branch-name match for
  `rhoai-<major>.<minor>` with an optional lowercase suffix such as `-ea.2` in
  `red-hat-data-services`.
- Fetch checks remote heads after cloning without changing the checked-out
  branch. It writes `repo-branch-policy.json`; remote query errors stop fetch.
- Discovery validates the report and omits non-matching repositories before
  provenance and classification. Discovery and provenance caches are narrowed
  to the current checkout set, and an existing component map does not bypass
  an active policy.
- `python3 scripts/lint_platforms.py` passed (18 platforms).
- `ruff check lib/fetch.py lib/phases/discover.py scripts/lint_platforms.py`
  passed; `git diff --check` passed. No tests were run.
- Independent review was not performed.
