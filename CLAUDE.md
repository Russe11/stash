<!-- GENERATED FILE — do not edit directly.
     Source of truth: stash-ng-workspace/tools/gen-brains/
       (invariants.md + repos/server.md). Regenerate from the workspace root:
         python3 tools/gen-brains/gen_brains.py
     The drift guard (.github/workflows/brains.yml) fails if this is stale. -->

# Stash NG Server (Claude + Codex instructions)

> Auto-loaded by Claude Code (`CLAUDE.md`) and Codex (`AGENTS.md` → symlink to this file).
> This is the **server** repo — the source of truth for the NG ecosystem (one of 5 sibling repos
> under `~/projects/Stash/`). Cross-cutting brains (invariants, contract, glossary, cross-repo ADRs)
> live in the workspace root — `../CLAUDE.md` in the workspace, or `Russe11/stash-ng-workspace`
> standalone. Cross-repo task→doc router: root `CLAUDE.md`.

## What this is

The NG **media server** — a deliberately-diverging fork of [Stash](https://github.com/stashapp/stash)
(`github.com/Russe11/stash`). Go · gqlgen GraphQL · SQLite. **Active branch: `deploy`** (`develop`
tracks upstream). Owns the data, the GraphQL contract, and **every destructive / file-relocating
mutation**. Deployed on Unraid (192.168.4.3) as the `stash-ng:latest` container.

Key dirs: `internal/api` (gqlgen resolvers), `pkg/*` (domain + `pkg/sqlite` storage),
`graphql/schema/**` (the **contract source of truth**), `internal/build/version.go` (NG capability
constants), and the Unraid Companion host plugin.

### This repo's role in the invariants

Contract **source of truth lives here** (`graphql/schema/**` + `NGFeatures()` in
`internal/build/version.go`); after a contract change run `make export-ng-schema` to refresh
`graphql/ng-snapshot/`. Every destructive/file-relocating op is **server-mediated** so DB + disk stay
in sync; trash-safe deletes use `uniqueTrashDest`. Webhooks are HMAC-signed with an SSRF scheme-guard
and bounded retry.

## Hard invariants (NEVER violate)

> These five are **product-wide** and identical across every repo. They are the canonical source;
> do not paraphrase them per-repo. Each repo states *its role* in them under "This repo's role" above.

1. **NG-only.** Upstream `stashapp/stash` is **not** a support target. Never add upstream-fallback
   logic or "missing capability = upstream" assumptions. "Compatibility" means NG-client ↔ NG-server
   version skew and contract drift *between clients* — never upstream compatibility.
2. **One contract, multiple copies.** The schema has one source (`Stash/graphql/schema/**`) and three
   derived copies (tvOS via StashKit; macOS via its own forked strings; Android via Apollo codegen from
   the `stash-server` submodule). A schema change must land in the server **and** every client copy.
   The root `contract/` Go test goes red on drift — keep it green.
3. **Destructive ops are macOS-only**, routed through server mutations so DB + disk stay in sync. Never
   add disk-delete to a viewer (Android's couch-remote `delete_file` is a bug to gate, not a feature).
4. **Secrets in the platform secure store** (Keychain / EncryptedSharedPreferences) — **never
   plaintext.** No telemetry unless explicitly opt-in + local-first.
5. **Trash is OFF on the live server** (`delete_trash_path` unset → deletes are permanent). Don't claim
   deletes are recoverable until that's set.

## Build / test

- Build: `make`
- Tests: `go test ./...` ; integration `make it`
- Schema snapshot (after contract changes): `make export-ng-schema`
- Unraid plugin suite: see the plugin dir's test runner.

## More

- Signature NG features (`deletedSince`, `moveFolder`, `findSimilarScenes`, `entityChanged`,
  `deviceBus`, webhooks): `../docs/api/ng-contract.md`.
- Live status / file lanes: `../docs/agent-handoff/README.md`.
- Backlog: `docs/agent-memory/{bugs,fixes,tests,optimizations,ideas,questions}.md` — remove entries
  when their work ships; `git log` is the audit trail.
