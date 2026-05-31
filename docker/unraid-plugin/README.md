# Stash NG Unraid Companion Plugin

Host-resident Unraid companion for auditing a Stash NG Docker deployment.

V1 is audit-first:

- No daemon.
- No sidecar container.
- No new network listener.
- No external calls by default.
- No Docker socket proxy.
- No arbitrary command runner.
- No automatic Docker template rewrites.
- No automatic container stop/start/restart.
- No database migration or restore.
- No media movement or deletion.

The companion has two fixed helpers:

- `helpers/stash-ng-audit.sh` emits a JSON audit report.
- `helpers/stash-ng-backup.sh` creates a user-triggered critical appdata backup.

Run fixture tests from the repository root:

```bash
sh Stash/docker/unraid-plugin/tests/run.sh
```

The audit helper must never read `stash-go.sqlite`; library truth belongs to the Stash NG Server API.

## Files

- `helpers/stash-ng-audit.sh` emits the V1 audit report.
- `helpers/stash-ng-backup.sh` creates a critical appdata backup.
- `pages/StashNGCompanion.php` is a thin Unraid UI page that invokes fixed helpers only.
- `tests/run.sh` runs fixture tests without a live Unraid host.

## Security Notes

The PHP page invokes fixed helper paths only. It does not accept arbitrary commands, command fragments, or user-provided executable paths.

Redacted exports are the default. Exact local evidence belongs inside the authenticated Unraid UI.
