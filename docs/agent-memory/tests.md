---
name: tests
description: Test coverage gaps in the Stash NG server fork
updated: 2026-05-30
---

# Tests

Test coverage gaps noticed in passing: missing tests for specific branches, weak assertions, flaky tests. Be concrete (file + branch/behavior). Remove an entry when the test exists and passes.

<!--
### YYYY-MM-DD — <short title>
<one paragraph: what to verify + where the test belongs + run vs NG/upstream>
-->

### 2026-05-30 — DeletedSince/MoveFolder full-resolver wiring blocked on the manager singleton (helper-level coverage only; refactor needed)
The fiddly NG logic is unit-tested by extracting it from the resolvers: `resolver_query_deleted_test.go` covers `parseDeletedSinceAfter` (cursor parse / malformed→nil) and `buildDeletedSinceResult` (store→`DeletedRecord` mapping, cursor advance/hold-on-empty, `has_more` passthrough, `pruned` boundary); `resolver_mutation_file_test.go` covers `validateFolderPath` (out-of-library reject) and `destinationInSourceSubtree` (move-cycle). What's still NOT exercised is the **full resolver method** end-to-end. A 2026-05-30 feasibility audit confirmed this is **blocked**, not merely low-priority: an `internal/api` test can trivially stand up a real DB (`sqlite.NewDatabase()` + `db.Open(tmpfile)` + `db.Repository()`, exactly as `pkg/sqlite/setup_test.go` does) and plant it as `Resolver.repository` so `withTxn`/`withReadTxn` run against real sqlite — but both resolvers reach past `r.repository` into the **manager singleton**, which has *no exported setter* (`manager.instance` is package-private; only `manager.Initialize(cfg,l)` sets it, and that's a full app bootstrap that opens its *own* internal DB at `cfg.GetDatabasePath()` and starts job/plugin/scraper/DLNA/session/FFmpeg — so it can't be pointed at a pre-seeded test DB). Specifically: (1) `DeletedSince` does `db := manager.GetInstance().Database` and calls `db.GetDeletedSincePage`/`db.MinDeletedRecordCursor`, which are **`*sqlite.Database`-only methods absent from `models.Repository`** (verified — `pkg/sqlite/deleted_record.go`), so a planted `r.repository` can't reach them at all; (2) `MoveFolder` needs `manager.GetInstance().Config.GetStashPaths()` plus a real-filesystem mover (`os.Rename`/`os.Mkdir`). Proper fix (then write `resolver_query_deleted_integration_test.go` gated `//go:build integration`): give `Resolver` a deleted-record store field + stash-paths source (or add a `DeletedRecord` store to `models.Repository`) and have the resolvers read those instead of `manager.GetInstance()`, making them drivable with the cheap real-DB `db.Repository()` harness and no manager bootstrap. Low priority — the store methods have their own `pkg/sqlite` tests and the mapping/validation logic is covered; the gap is just the thin txn-wiring layer, gated behind a small testability refactor.
