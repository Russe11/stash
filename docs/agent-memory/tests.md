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

### 2026-05-30 — MoveFiles/MoveFolder still reach the manager singleton + an inline real-filesystem mover (no end-to-end resolver test)
`DeletedSince` is now decoupled and covered end-to-end (the resolver reads `r.deletedRecordReader` — a `DeletedRecordReader` seam wired from `mgr.Database` in `internal/api/server.go Initialize()`, production behaviour byte-identical; `internal/api/resolver_query_deleted_integration_test.go`, `//go:build integration`, drives `r.Query().DeletedSince` against a real sqlite DB for field mapping, cursor advance across pages, `has_more`, and the `pruned` boundary). What remains uncovered is the **MoveFiles/MoveFolder resolver methods end-to-end**, and they were deliberately left coupled. Both reach `manager.GetInstance().Config.GetStashPaths()` AND construct a `file.NewMover(...)` inline (`resolver_mutation_file.go`) that performs real `os.Rename`/`os.Mkdir`/`mover.CreateFolderHierarchy`. Decoupling just the stash-paths read is easy (same seam shape as the DeletedSince fix), but the mover is the hard part: it is built inline rather than injected, and a real-move test must stage actual files/folders on a temp library root AND seed `folders` rows whose `Path` matches that root — disproportionate/risky on the deployed branch, so it was not done. The helper-level logic IS unit-tested (`resolver_mutation_file_test.go`: `validateFolderPath` out-of-library reject, `destinationInSourceSubtree` move-cycle). To close: give `Resolver` a stash-paths provider + a mover factory (or inject `file.Mover`) so the methods are drivable with the cheap real-DB `db.Repository()` harness over a temp filesystem; then add the self-into-subtree reject / out-of-library reject / successful by-id move cases. Low priority — validation/cycle logic is covered; the gap is the filesystem txn-wiring layer behind a moderate testability refactor.
