---
name: bugs
description: Reproducible defects in the Stash NG server fork
updated: 2026-05-29
---

# Bugs

Reproducible defects (something is broken). Be concrete (file + behavior). Remove an entry once fixed; `git log` is the audit trail.

<!--
### YYYY-MM-DD — <short title>
<one paragraph: repro + expected vs actual + file:line>
-->

### 2026-05-29 — `TestStudioQueryFast` fails locally: `no such function: mod`
`go test -tags integration ./pkg/sqlite/` fails only in `TestStudioQueryFast` (`studio_test.go:1691`) with `no such function: mod`: the studio random-sort `ORDER BY mod(...)` SQL relies on a `mod` SQLite function that isn't registered in this local CGo/SQLite build. Confirmed pre-existing on a clean `deploy` checkout (independent of the deletedSince cursor work). Either register a `mod` user function on the sqlite connection (alongside the other custom functions) or change the random-sort expression to use the `%` operator. All other `pkg/sqlite` integration tests pass.
