# Isolate Codex discovery execution

Implemented disposable worker workspaces with copied shared skills and explicit
checkout arguments. The worker writes a candidate map; the parent validates its
schema/platform and atomically promotes it. Failed candidates preserve existing
output. Tests exercise staging and all promotion outcomes without model calls.

Scope: Codex component discovery. Other generation phases retain their existing
working directories. The workspace prevents accidental context contamination,
but does not enforce read isolation. Live discovery quality remains to be checked.
