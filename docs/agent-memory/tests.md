---
name: tests
description: Test coverage gaps in the Stash NG server fork
updated: 2026-05-31
---

# Tests

Test coverage gaps noticed in passing: missing tests for specific branches, weak assertions, flaky tests. Be concrete (file + branch/behavior). Remove an entry when the test exists and passes.

<!--
### YYYY-MM-DD — <short title>
<one paragraph: what to verify + where the test belongs + run vs NG/upstream>
-->

### 2026-05-30 — MoveFiles DestinationBasename (rename-on-move) branch still uncovered (manager-coupled)
The MoveFiles file-move filesystem path is now covered end-to-end: `internal/api/resolver_mutation_movefiles_integration_test.go`
(`//go:build integration`) drives `r.Mutation().MoveFiles` against a real sqlite DB + real temp library via the `stashPaths`
seam, reusing the MoveFolder harness (`newMoveFolderTestResolver`, `makeFolderOnDisk`, `reloadFolder`) plus new
`makeFileOnDisk`/`reloadFile` helpers. Covered: by-id (`DestinationFolderID`) move re-parents the file row and renames it on
disk while a sibling stays put; by-path (`DestinationFolder`) move that get-or-creates the destination folder row + directory;
and out-of-library path rejection (no DB/disk mutation). **Still uncovered:** the `DestinationBasename` rename-on-move branch
and the zip-hierarchy branch. Rename can't be tested in the seam-only harness because supplying a basename calls
`validateFileExtension`, which reads `manager.GetInstance().Config` (not behind a seam) and panics "manager not initialized";
covering it needs either a manager bootstrap in the test or a new extension-list seam (`r.fileExtensions` mirroring
`r.stashPaths`, wired from `mgr.Config` in `server.go Initialize()`, production behaviour byte-identical). The zip branch needs
zip-file rows seeded. Both are lower value than the primary file-move path now covered; do the extension seam first since it
also unblocks unit-testing `validateFileExtension` directly.
