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

### 2026-06-01 — Trick-play (HLS I-frame scrub previews): logic green, real-device playback UNVERIFIED
The trick-play feature (ADR-0003) is built + unit-tested + ffmpeg-validated locally, but the one thing only hardware can confirm is still open: **does AVPlayerViewController on a real Apple TV actually render scrub thumbnails from the generated master playlist?** Verified so far: `rewriteTrickplayManifest` cumulative-offset fix (`pkg/scene/generate/trickplay_test.go`, fed real buggy ffmpeg `@0` output); `buildHLSMasterPlaylist`/`trickplayDimensions` (`internal/api/routes_scene_trickplay_test.go`); and an end-to-end ffmpeg run proving the exact `trickplayArgs` recipe yields a `.ts` whose corrected byte-ranges each decode to one H.264 frame. **Still to do (needs the user's Unraid box + Apple TV):** (1) deploy the server build and run a generate pass so scenes get `<hash>_trickplay.{m3u8,ts}`; (2) confirm thumbnails appear while scrubbing on tvOS; (3) confirm **macOS AVPlayer + the web player** tolerate the master playlist on generated scenes (Android is out — its player is being rebuilt to match tvOS). Lower-priority polish: add a `CODECS=` attribute to the master-playlist variants (`buildHLSMasterPlaylist`) — AVPlayer plays without it, but Apple's `mediastreamvalidator` flags its absence.

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
