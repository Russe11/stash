# Stash NG Server (Claude + Codex instructions)

> Auto-loaded by Claude Code (`CLAUDE.md`) and Codex (`AGENTS.md` â symlink to this file).
> This is the **server** repo â the source of truth for the NG ecosystem (one of 5 sibling repos
> under `~/projects/Stash/`). Workspace overview + cross-repo task→doc router: `../CLAUDE.md`.

## What this is

The NG **media server** â a deliberately-diverging fork of [Stash](https://github.com/stashapp/stash)
(`github.com/Russe11/stash`). Go Â· gqlgen GraphQL Â· SQLite. **Active branch: `deploy`** (`develop`
tracks upstream). Owns the data, the GraphQL contract, and **every destructive / file-relocating
mutation**. Deployed on Unraid (192.168.4.3) as the `stash-ng:latest` container.

Key dirs: `internal/api` (gqlgen resolvers), `pkg/*` (domain + `pkg/sqlite` storage),
`graphql/schema/**` (the **contract source of truth**), `internal/build/version.go` (NG capability
constants), and the Unraid Companion host plugin.

## Hard invariants (NEVER violate)

1. **NG-only.** Upstream is not a support target. Never reintroduce upstream-fallback logic.
2. **Contract source of truth lives here** (`graphql/schema/**` + `NGFeatures()` in
   `internal/build/version.go`). Any schema/capability change must keep the root `contract/` Go test
   green and be mirrored into every client copy (tvOS/StashKit, macOS, Android Apollo submodule).
   After a contract change run `make export-ng-schema` to refresh `graphql/ng-snapshot/`.
3. **Destructive ops are server-mediated.** Every delete/move goes through a server mutation so DB +
   disk stay in sync. Trash-safe deletes use `uniqueTrashDest`. Trash is OFF on the live server
   (`delete_trash_path` unset â deletes are permanent) â don't claim deletes are recoverable.
4. **No plaintext secrets; no telemetry unless opt-in + local-first.** Webhooks are HMAC-signed with
   an SSRF scheme-guard and bounded retry.

## Build / test

- Build: `make`
- Tests: `go test ./...` ; integration `make it`
- Schema snapshot (after contract changes): `make export-ng-schema`
- Unraid plugin suite: see the plugin dir's test runner.

## More

- Signature NG features (`deletedSince`, `moveFolder`, `findSimilarScenes`, `entityChanged`,
  `deviceBus`, webhooks): `../docs/api/ng-contract.md`.
- Live status / file lanes: `../docs/agent-handoff/README.md`.
- Backlog: `docs/agent-memory/{bugs,fixes,tests,optimizations,ideas,questions}.md` â remove entries
  when their work ships; `git log` is the audit trail.
