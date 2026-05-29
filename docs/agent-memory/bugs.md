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

### 2026-05-29 — deletedSince feed is not safely resumable (1-second resolution + strict `>`)
`deleted_at` is stored as `time.RFC3339` (no fractional seconds, `pkg/sqlite/timestamp.go:8`) and `GetDeletedSince` filters `WHERE deleted_at > ?` (`pkg/sqlite/deleted_record.go:77`). A sync client that advances its watermark to the last record's `deleted_at` will **skip any tombstone sharing that same whole second** inserted afterward. The table has a monotonic autoincrement `id` (the correct cursor) but the API exposes/filters only `deleted_at`. macOS masks this with a `-2000ms` overlap + full-id reconcile; the server contract is still wrong. Fix: expose `id` as an opaque cursor with `after`/`limit`, or store `RFC3339Nano`. The feed is also unbounded (no `LIMIT`, no tombstone retention) so `since:0` grows forever. Repro test belongs in `deleted_record_test.go` (insert two tombstones in one second; assert the second isn't lost — fails today).
