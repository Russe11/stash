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

### 2026-05-29 — No resolver test for the deleted/move NG ops (serverCapabilities now covered)
The `serverCapabilities` resolver is now tested (`internal/api/resolver_server_capabilities_test.go` — edition/apiVersion/features/retention). Store/mover layers are also tested (`pkg/sqlite/deleted_record_test.go`, `pkg/sqlite/folder_count_test.go`, `pkg/file/move_test.go`, `pkg/plugin/webhook_test.go`). Still missing: a resolver-path test for the deletion feed and file move. Add `internal/api/resolver_query_deleted_test.go` and `resolver_mutation_file_test.go` using the `mocks.Database`/`newResolver` pattern (`resolver_mutation_tag_test.go:17`): assert `DeletedSince` maps store→`DeletedRecord` (incl. cursor/has_more); assert `MoveFolder` rejects self-into-subtree (mocked `GetManyParentFolderIDs`) + out-of-library dest (`validateFolderPath`). Run vs NG.
