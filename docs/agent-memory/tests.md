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

### 2026-05-30 — DeletedSince/MoveFolder logic is covered at the helper level, not full resolver wiring
The fiddly NG logic is now unit-tested by extracting it from the resolvers: `resolver_query_deleted_test.go` covers `parseDeletedSinceAfter` (cursor parse / malformed→nil) and `buildDeletedSinceResult` (store→`DeletedRecord` mapping, cursor advance/hold-on-empty, `has_more` passthrough, `pruned` boundary); `resolver_mutation_file_test.go` covers `validateFolderPath` (out-of-library reject) and `destinationInSourceSubtree` (move-cycle). What's still NOT exercised is the **full resolver method** end-to-end — `DeletedSince`/`MoveFolder` calling the store under a real `withReadTxn`/`withTxn` (they read `manager.GetInstance().Database`, so they need a real in-memory sqlite + manager singleton, not the `mocks.Database`/`newResolver` pattern, which the `resolver_mutation_tag_test.go` TODO confirms doesn't run the txn body). Low priority — the store methods have their own `pkg/sqlite` tests and the mapping/validation logic is now covered.
