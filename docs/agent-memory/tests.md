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

### 2026-05-30 — MoveFiles resolver has no end-to-end test (real-file staging needed)
`DeletedSince` and `MoveFolder` are now decoupled from the manager singleton and covered end-to-end. `DeletedSince` reads `r.deletedRecordReader` (a `DeletedRecordReader` seam) and `MoveFolder`/`MoveFiles` read `r.stashPaths` (a `StashPathsReader` seam) instead of `manager.GetInstance()`; both seams are wired from `mgr.Database`/`mgr.Config` in `internal/api/server.go Initialize()` (production behaviour byte-identical). `internal/api/resolver_query_deleted_integration_test.go` (`//go:build integration`) drives `r.Query().DeletedSince` (field mapping, cursor advance, `has_more`, `pruned`) and `r.Mutation().MoveFolder` against a real sqlite DB + real temp library (by-id move re-parents the row and renames the dir on disk; self-move and self-into-subtree and out-of-library destinations are rejected). What's still uncovered is **`MoveFiles` end-to-end**: its stash-paths read is decoupled, but the test would have to stage actual *files* on a temp library root (primary-file / zip-hierarchy / extension-validation branches) and seed matching `files` + `folders` rows, which is heavier than the folder-only MoveFolder harness — not done. The helper-level logic IS unit-tested (`resolver_mutation_file_test.go`: `validateFolderPath` out-of-library reject, `destinationInSourceSubtree` move-cycle). To close: extend the integration harness to stage a primary + secondary file under a folder row and drive `r.Mutation().MoveFiles` for the by-id and by-path branches, asserting the file row's `parent_folder_id`/path and the on-disk rename. Low priority — the validation/cycle logic and the folder path are covered; this is the remaining file-move filesystem path.
