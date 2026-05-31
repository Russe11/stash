#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/lib/json.sh"

appdata=""
destination=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --appdata)
      appdata="${2:-}"
      shift 2
      ;;
    --destination)
      destination="${2:-}"
      shift 2
      ;;
    *)
      printf 'Unknown argument: %s\n' "$1" >&2
      exit 64
      ;;
  esac
done

[ -n "$appdata" ] || { printf 'Missing --appdata\n' >&2; exit 64; }
[ -n "$destination" ] || { printf 'Missing --destination\n' >&2; exit 64; }
[ -d "$appdata" ] || { printf 'Appdata directory not found: %s\n' "$appdata" >&2; exit 66; }

timestamp="$(date -u '+%Y%m%dT%H%M%SZ')"
backup_dir="$destination/stash-ng-critical-appdata-$timestamp"
mkdir -p "$backup_dir"

copy_if_exists() {
  src="$1"
  dst="$2"
  if [ -e "$src" ]; then
    if [ -d "$src" ]; then
      mkdir -p "$dst"
      (cd "$src" && tar cf - .) | (cd "$dst" && tar xf -)
    else
      mkdir -p "$(dirname "$dst")"
      cp -p "$src" "$dst"
    fi
  fi
}

copy_if_exists "$appdata/config.yml" "$backup_dir/config.yml"
copy_if_exists "$appdata/stash-go.sqlite" "$backup_dir/stash-go.sqlite"
copy_if_exists "$appdata/stash-go.sqlite-wal" "$backup_dir/stash-go.sqlite-wal"
copy_if_exists "$appdata/stash-go.sqlite-shm" "$backup_dir/stash-go.sqlite-shm"
copy_if_exists "$appdata/plugins" "$backup_dir/plugins"
copy_if_exists "$appdata/scrapers" "$backup_dir/scrapers"

printf '{'
printf '"schemaVersion":"stash-ng.unraid.backup.v1",'
printf '"status":"created",'
printf '"path":'
json_string "$backup_dir"
printf ','
printf '"excluded":["generated","cache","transcodes","blobs","media"],'
printf '"warning":'
json_string "If the Stash NG container was running, verify SQLite/WAL consistency before relying on this backup for rollback."
printf '}'
printf '\n'
