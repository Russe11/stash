# Running the forked Stash server on Unraid

This directory packages the fork as a self-hosted container that runs **separately
from**, or as a **one-way cutover from**, your existing `stashapp/stash` install.

Files:

| File | What it is |
|------|------------|
| `Dockerfile` | The project's proven multi-stage build, with default build-args so a plain `docker build` works. Produces a schema **v86** server. |
| `docker-compose.yml` | Builds + runs the fork on host port **9998**, with the exact volume mounts to optionally reuse your appdata. |
| `stash-ng.xml` | Unraid Community-Applications template (local image, port 9998). |

---

## 1. Choose what to deploy

`docker build` compiles **whatever branch is checked out** in the build context.
The fork's features live on separate branches off `develop`:

- `develop` — merged server features: `moveFolder`, the deletion feed, recursive `Folder.scene_count` (this is what makes it schema **v86**).
- `roadmap/webhooks` — outbound webhooks (`webhook_urls`).
- `roadmap/folder-stats` — recursive `Folder.image_count` + `total_size`.

If you want **everything** in one image, make a deploy branch first:

```bash
git checkout -b deploy develop
git merge --no-ff roadmap/webhooks roadmap/folder-stats
```

Otherwise just `git checkout develop` (or whichever single branch you want) before building.

## 2. Build the image

Build context must be the **repo root** (the Dockerfile copies `./ui`, `./pkg`, …):

```bash
docker build -f docker/unraid/Dockerfile -t stash-ng:latest .
# or, equivalently:
docker compose -f docker/unraid/docker-compose.yml build
```

On Unraid, clone the repo somewhere persistent (e.g. under `/boot` or an appdata
share) and run the build there, so the `stash-ng:latest` image is local to the host.
(First build is slow — it compiles the full UI and the Go backend.)

## 3a. Run it standalone (safe, no risk to your current server)

Give it its **own** appdata and leave the defaults in `docker-compose.yml`
(`/mnt/user/appdata/stash-ng`). It starts empty; point it at your media and let it
scan. Your existing server is untouched. Good for evaluating the fork.

```bash
docker compose -f docker/unraid/docker-compose.yml up -d
# open http://YOUR-UNRAID-IP:9998
```

## 3b. Cut over to the same appdata (one-way — read this first)

The fork is schema **v86**; your stock server is **v85**. Migration is forward-only:
once the fork opens your library it upgrades `stash-go.sqlite` to v86, and the stock
server can **never reopen it** (`MismatchedSchemaVersionError`). Two servers must
**never** touch the same database at once. So this is a deliberate cutover, not a share.

It is safe and reversible **as long as you keep a v85 backup**:

1. **Stop** your current Stash container (a clean stop flushes the SQLite WAL).
2. **Back up the database yourself** — independent of Stash's own backup:
   ```bash
   cp stash-go.sqlite       stash-go.sqlite.v85.bak
   cp stash-go.sqlite-wal   stash-go.sqlite-wal.bak 2>/dev/null || true
   cp stash-go.sqlite-shm   stash-go.sqlite-shm.bak 2>/dev/null || true
   ```
   (Or just copy the whole appdata dir.)
3. In `docker-compose.yml` (or the Unraid template), set every **left-side host
   path to the same paths your current container uses**, so the fork reads the
   identical config + DB + generated + blobs. Keep the host port at **9998**.
   For extra safety on the first boot, append **`:ro`** to the media mount.
4. `docker compose -f docker/unraid/docker-compose.yml up -d`, then open
   `http://YOUR-UNRAID-IP:9998`. The fork detects the older DB and shows a
   **Migrate** prompt — it will not serve the library until you run it.
5. Click **Migrate** with **"Backup Database before migrating" left ON**. Stash
   always backs up before migrating, but with the box on it *keeps* the backup as
   `stash-go.sqlite.85.<timestamp>` (the number is the pre-migration version — exactly
   what your stock server can reopen). With the box off, that backup is deleted.
6. Once it's serving correctly, drop the `:ro` from the media mount if you added it.

**Never start both servers against this appdata simultaneously** — even briefly.
SQLite is single-writer (the fork opens it with a 50 ms busy timeout); two live
servers will throw `database is locked` and risk corruption.

### Rolling back to the stock server

```bash
docker compose -f docker/unraid/docker-compose.yml down       # stop the fork
cp stash-go.sqlite.v85.bak stash-go.sqlite                    # restore v85 DB
# (or use the kept stash-go.sqlite.85.<timestamp> backup)
# then start your stock stashapp/stash container again
```

`generated/`, `blobs/`, `cache/`, scrapers and plugins are schema-independent on
disk and shared fine one-server-at-a-time. `config.yml` is forward/backward
tolerant — the fork may add keys (e.g. `webhook_urls`); the stock server ignores
unknown keys. The database is the only one-way door.
