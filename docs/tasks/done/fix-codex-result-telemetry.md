# Fix Codex result telemetry

Completed 2026-09-04 at the user's request.

- Serialize SDK enums by their wire values before object introspection.
- Pass checkout identity into the Codex adapter; report observed read actions
  with resolved repo-relative paths and conservative line bounds.
- Preserve telemetry and terminal failure status when postprocessing fails.
- Validate with SDK-enum, adapter-dispatch, read-observation, and callback-error
  regression tests, plus replay of the rhods-operator 20260905T030959Z log.

All 43 focused tests and scoped Ruff checks pass. No live agent run or generated
architecture edits were necessary. The handbook's linked
`docs/notes/agentic_work_ledger.md` is absent in this checkout.
