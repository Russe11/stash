#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
AUDIT="$ROOT/helpers/stash-ng-audit.sh"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

assert_contains() {
  haystack="$1"
  needle="$2"
  printf '%s' "$haystack" | grep -F "$needle" >/dev/null || fail "expected output to contain: $needle"
}

assert_not_contains() {
  haystack="$1"
  needle="$2"
  if printf '%s' "$haystack" | grep -F "$needle" >/dev/null; then
    fail "expected output not to contain: $needle"
  fi
}

happy="$("$AUDIT" --fixture "$ROOT/tests/fixtures/happy")"

assert_contains "$happy" '"schemaVersion":"stash-ng.unraid.audit.v1"'
assert_contains "$happy" '"generatedBy":"stash-ng-audit"'
assert_contains "$happy" '"findings":['
assert_contains "$happy" '"redactionDefault":"redacted"'
assert_contains "$happy" '"id":"container.candidate.detected"'
assert_contains "$happy" '"id":"container.managed_target.single"'
assert_not_contains "$happy" '"id":"container.appdata.conflict"'

conflict="$("$AUDIT" --fixture "$ROOT/tests/fixtures/conflict")"
assert_contains "$conflict" '"id":"container.appdata.conflict"'
assert_contains "$conflict" '"severity":"Blocking"'

drift="$("$AUDIT" --fixture "$ROOT/tests/fixtures/template-drift")"
assert_contains "$drift" '"id":"template.generated_mount.missing"'
assert_contains "$drift" '"severity":"Risk"'
assert_contains "$drift" '"id":"storage.appdata.array"'
assert_contains "$drift" '"severity":"Performance"'

server_aware="$("$AUDIT" --fixture "$ROOT/tests/fixtures/server-aware" --server-facts "$ROOT/tests/fixtures/server-aware/server-capabilities.json")"
assert_contains "$server_aware" '"id":"server.identity.ng"'
assert_contains "$server_aware" '"severity":"Info"'

server_mismatch="$("$AUDIT" --fixture "$ROOT/tests/fixtures/server-mismatch" --server-facts "$ROOT/tests/fixtures/server-mismatch/server-capabilities.json")"
assert_contains "$server_mismatch" '"id":"server.identity.mismatch"'
assert_contains "$server_mismatch" '"severity":"Blocking"'

redacted="$("$AUDIT" --fixture "$ROOT/tests/fixtures/conflict" --redacted)"
assert_contains "$redacted" '/mnt/<redacted>/appdata/stash-ng'
assert_not_contains "$redacted" '/mnt/cache/appdata/stash-ng'

fake_docker_dir="$ROOT/tests/tmp-fake-docker"
mkdir -p "$fake_docker_dir"
cat > "$fake_docker_dir/docker" <<'EOF'
#!/bin/sh
set -eu

case "$1" in
  ps)
    printf 'stash-ng\n'
    printf 'stash-old\n'
    ;;
  inspect)
    shift
    for name in "$@"; do
      case "$name" in
        stash-ng)
          printf '%s\t%s\t%s\t%s\t%s\n' \
            'stash-ng' \
            'stash-ng:latest' \
            '0.0.0.0:9998->9999/tcp' \
            '/mnt/cache/appdata/stash-ng' \
            '/root/.stash:/mnt/cache/appdata/stash-ng:rw,/generated:/mnt/cache/appdata/stash-ng/generated:rw,/cache:/mnt/cache/appdata/stash-ng/cache:rw'
          ;;
        stash-old)
          printf '%s\t%s\t%s\t%s\t%s\n' \
            'stash-old' \
            'stashapp/stash:latest' \
            '0.0.0.0:9999->9999/tcp' \
            '/mnt/cache/appdata/stash-ng' \
            '/root/.stash:/mnt/cache/appdata/stash-ng:rw'
          ;;
      esac
    done
    ;;
  *)
    printf 'unexpected docker command: %s\n' "$1" >&2
    exit 64
    ;;
esac
EOF
chmod +x "$fake_docker_dir/docker"
host_report="$(PATH="$fake_docker_dir:$PATH" "$AUDIT")"
assert_contains "$host_report" '"mode":"host"'
assert_contains "$host_report" '"id":"container.appdata.conflict"'
assert_contains "$host_report" '"severity":"Blocking"'
assert_not_contains "$host_report" '"id":"template.generated_mount.missing"'
rm -rf "$fake_docker_dir"

backup_root="$ROOT/tests/tmp-backups"
rm -rf "$backup_root"
backup_report="$("$ROOT/helpers/stash-ng-backup.sh" --appdata "$ROOT/tests/fixtures/backup/appdata" --destination "$backup_root")"
assert_contains "$backup_report" '"schemaVersion":"stash-ng.unraid.backup.v1"'
assert_contains "$backup_report" '"status":"created"'
backup_path="$(printf '%s' "$backup_report" | sed -n 's/^.*"path":"\([^"]*\)".*$/\1/p')"
[ -n "$backup_path" ] || fail "backup path missing"
[ -f "$backup_path/config.yml" ] || fail "backup missing config.yml"
[ -f "$backup_path/stash-go.sqlite" ] || fail "backup missing stash-go.sqlite"
[ -f "$backup_path/plugins/sample.yml" ] || fail "backup missing plugin state"
[ -f "$backup_path/scrapers/sample.yml" ] || fail "backup missing scraper state"
[ ! -e "$backup_path/generated/derived.txt" ] || fail "backup included generated artifact"
rm -rf "$backup_root"

[ -f "$ROOT/pages/StashNGCompanion.page" ] || fail "missing Unraid page metadata"
[ -f "$ROOT/pages/StashNGCompanion.php" ] || fail "missing Unraid PHP page"
assert_contains "$(cat "$ROOT/pages/StashNGCompanion.php")" 'stash-ng-audit.sh'
assert_contains "$(cat "$ROOT/pages/StashNGCompanion.php")" 'escapeshellarg'
assert_not_contains "$(cat "$ROOT/pages/StashNGCompanion.php")" 'shell_exec($_'
assert_not_contains "$(cat "$ROOT/pages/StashNGCompanion.php")" 'passthru($_'

printf 'PASS: audit report contract\n'
printf 'PASS: candidate container findings\n'
printf 'PASS: appdata conflict findings\n'
printf 'PASS: template drift findings\n'
printf 'PASS: share policy findings\n'
printf 'PASS: server-aware identity findings\n'
printf 'PASS: redacted audit export\n'
printf 'PASS: host docker inspect collection\n'
printf 'PASS: critical appdata backup\n'
printf 'PASS: unraid page fixed-helper wiring\n'
