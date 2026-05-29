---
name: tests
description: Test coverage gaps in the Stash NG server fork
updated: 2026-05-29
---

# Tests

Test coverage gaps noticed in passing: missing tests for specific branches, weak assertions, flaky tests. Be concrete (file + branch/behavior). Remove an entry when the test exists and passes.

<!--
### YYYY-MM-DD — <short title>
<one paragraph: what to verify + where the test belongs + run vs NG/upstream>
-->

### 2026-05-29 — No GraphQL resolver/contract test for any NG op
Store/mover layers ARE tested (`pkg/sqlite/deleted_record_test.go`, `pkg/sqlite/folder_count_test.go`, `pkg/file/move_test.go`, `pkg/plugin/webhook_test.go`), but **no test exercises the resolver path** the clients actually hit. Add `internal/api/resolver_query_deleted_test.go` and `resolver_mutation_file_test.go` using the existing `mocks.Database`/`newResolver` pattern (`resolver_mutation_tag_test.go:17`): assert `DeletedSince` maps store→`DeletedRecord`; assert `MoveFolder` rejects self-into-subtree (mocked `GetManyParentFolderIDs`) + out-of-library dest (`validateFolderPath`). Also add a resolver test for the new `serverCapabilities` query (edition="ng", features list). Run vs NG.

### 2026-05-29 — deletedSince same-second-collision is unproven
`TestDeletedRecordsFeed` proves the allowlist + future-watermark, not the same-second skip (see bugs.md). Add a case inserting two tombstones in one whole second, watermark on the first's `deleted_at`, assert the second isn't lost (fails today — proves the bug; pairs with the id-cursor fix). Run vs NG.

### 2026-05-29 — No original-upstream compatibility test for NG ops
Nothing verifies that an NG mutation/query degrades sanely when run against a stock upstream server (the clients depend on this). A lightweight harness that runs `deletedSince`/`moveFolder`/`serverCapabilities` against an upstream-schema executable schema and asserts the expected "cannot query field" error would document the contract. Run vs both.
