#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/lib/json.sh"
. "$SCRIPT_DIR/lib/findings.sh"
. "$SCRIPT_DIR/lib/redact.sh"

fixture=""
server_facts=""
redacted="false"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --fixture)
      fixture="${2:-}"
      shift 2
      ;;
    --server-facts)
      server_facts="${2:-}"
      shift 2
      ;;
    --redacted)
      redacted="true"
      shift
      ;;
    *)
      printf 'Unknown argument: %s\n' "$1" >&2
      exit 64
      ;;
  esac
done

emit_finding() {
  kind="$1"
  id="$2"
  title="$3"
  evidence="$4"
  action="$5"
  if [ "$redacted" = "true" ]; then
    evidence="$(redact_evidence "$evidence")"
  fi
  case "$kind" in
    Info) info_finding "$id" "$title" "$evidence" "$action" ;;
    Risk) risk_finding "$id" "$title" "$evidence" "$action" ;;
    Blocking) blocking_finding "$id" "$title" "$evidence" "$action" ;;
    Performance) performance_finding "$id" "$title" "$evidence" "$action" ;;
  esac
}

containers_file() {
  if [ -n "$fixture" ]; then
    printf '%s/containers.tsv' "$fixture"
  else
    printf '/tmp/stash-ng-companion-containers.tsv'
  fi
}

collect_host_containers() {
  out="$(containers_file)"
  if [ -n "$fixture" ]; then
    return 0
  fi

  names_file="$out.names"
  docker ps --format '{{.Names}}' > "$names_file"
  : > "$out"

  while IFS= read -r container_name; do
    [ -n "$container_name" ] || continue
    inspect_line="$(docker inspect --format '{{.Name}}{{"\t"}}{{.Config.Image}}{{"\t"}}{{range $port, $bindings := .NetworkSettings.Ports}}{{if $bindings}}{{(index $bindings 0).HostIp}}:{{(index $bindings 0).HostPort}}->{{$port}} {{end}}{{end}}{{"\t"}}{{range .Mounts}}{{if eq .Destination "/root/.stash"}}{{.Source}}{{end}}{{end}}{{"\t"}}{{range .Mounts}}{{.Destination}}:{{.Source}}:{{.Mode}},{{end}}' "$container_name")"
    printf '%s\n' "$inspect_line" |
      awk -F '\t' 'BEGIN { OFS="\t" } { sub(/^\//, "", $1); print $1, $2, $3, $4, $5 }' >> "$out"
  done < "$names_file"
  rm -f "$names_file"
}

emit_container_findings() {
  file="$(containers_file)"
  [ -f "$file" ] || {
    emit_finding "Info" "container.none" "No Stash containers detected" "No container fixture or host container list was found." "Install or start the Stash NG container, then run audit again."
    return
  }

  candidate_count="$(awk -F '\t' 'BEGIN { c=0 } tolower($1 " " $2) ~ /stash/ { c++ } END { print c }' "$file")"
  appdata_conflicts="$(awk -F '\t' 'NF >= 4 && $4 != "" { count[$4]++ } END { c=0; for (p in count) if (count[p] > 1) c++; print c }' "$file")"
  first_candidate="$(awk -F '\t' 'tolower($1 " " $2) ~ /stash/ { print $1 " using image " $2 " on port " $3; exit }' "$file")"

  if [ "$candidate_count" -gt 0 ]; then
    emit_finding "Info" "container.candidate.detected" "Candidate container detected" "$first_candidate" "Verify this candidate is the intended Stash NG managed target."
  else
    emit_finding "Info" "container.none" "No Stash containers detected" "No container name or image contained stash." "Install or start the Stash NG container, then run audit again."
  fi

  printf ','

  if [ "$appdata_conflicts" -gt 0 ]; then
    conflict_path="$(awk -F '\t' 'NF >= 4 && $4 != "" { count[$4]++ } END { for (p in count) if (count[p] > 1) { print p; exit } }' "$file")"
    emit_finding "Blocking" "container.appdata.conflict" "Multiple containers share one appdata path" "$conflict_path is mounted by more than one container." "Stop all but the intended managed target before cutover, update, or backup actions."
  elif [ "$candidate_count" -eq 1 ]; then
    emit_finding "Info" "container.managed_target.single" "Single Stash candidate detected" "Exactly one Stash-related container was found." "Continue with template, storage, and optional API identity checks."
  else
    emit_finding "Risk" "container.identity.ambiguous" "Multiple Stash candidates detected" "$candidate_count Stash-related containers were found." "Confirm which container is the Stash NG managed target."
  fi
}

shares_file() {
  if [ -n "$fixture" ]; then
    printf '%s/shares.tsv' "$fixture"
  else
    printf '/tmp/stash-ng-companion-shares.tsv'
  fi
}

emit_template_and_storage_findings() {
  containers="$(containers_file)"
  shares="$(shares_file)"

  [ -f "$containers" ] || return 0

  if ! awk -F '\t' 'tolower($1 " " $2) ~ /stash/ && $4 != "" { found=1 } END { exit found ? 0 : 1 }' "$containers"; then
    return
  fi

  generated_mount_count="$(awk -F '\t' 'tolower($0) ~ /generated|transcode|cache/ { c++ } END { print c+0 }' "$containers")"
  if [ "$generated_mount_count" -eq 0 ]; then
    printf ','
    emit_finding "Risk" "template.generated_mount.missing" "Generated/cache mount not visible" "The candidate container fixture does not expose generated, cache, or transcode mounts." "Compare the live container to the Stash NG Docker template and add explicit generated/cache mounts before heavy generation jobs."
  fi

  [ -f "$shares" ] || return 0

  appdata_row="$(awk -F '\t' '$1 ~ /appdata\/stash-ng/ { print $0; exit }' "$shares")"
  if [ -n "$appdata_row" ]; then
    storage_class="$(printf '%s\n' "$appdata_row" | awk -F '\t' '{ print $2 }')"
    mover_sensitive="$(printf '%s\n' "$appdata_row" | awk -F '\t' '{ print $4 }')"
    if [ "$storage_class" = "array" ]; then
      printf ','
      emit_finding "Performance" "storage.appdata.array" "Appdata appears to be on array storage" "$appdata_row" "Move Stash NG appdata/database to fast cache-backed storage before relying on heavy library operations."
    fi
    if [ "$mover_sensitive" = "yes" ]; then
      printf ','
      emit_finding "Risk" "storage.appdata.mover_sensitive" "Appdata may be affected by mover" "$appdata_row" "Review share cache policy so mover does not surprise the SQLite database or active appdata."
    fi
  fi
  return 0
}

emit_server_identity_findings() {
  [ -n "$server_facts" ] || return 0
  [ -f "$server_facts" ] || {
    printf ','
    emit_finding "Risk" "server.facts.missing" "Server facts file missing" "$server_facts was requested but not found." "Run a no-credential audit or provide a valid server facts response."
    return
  }

  edition="$(awk 'match($0, /"edition"[ ]*:[ ]*"[^"]+"/) { v=substr($0, RSTART, RLENGTH); gsub(/^.*"edition"[ ]*:[ ]*"/, "", v); gsub(/"$/, "", v); print v; exit }' "$server_facts")"
  api_version="$(awk 'match($0, /"apiVersion"[ ]*:[ ]*[0-9]+/) { v=substr($0, RSTART, RLENGTH); gsub(/^.*"apiVersion"[ ]*:[ ]*/, "", v); print v; exit }' "$server_facts")"

  printf ','
  if [ "$edition" = "ng" ]; then
    emit_finding "Info" "server.identity.ng" "Stash NG server identity verified" "serverCapabilities edition=ng apiVersion=$api_version" "Continue using API-backed checks for configured paths, trash, and job state."
  else
    emit_finding "Blocking" "server.identity.mismatch" "Container identity does not match Stash NG API identity" "serverCapabilities edition=$edition apiVersion=$api_version" "Do not run guided actions until the managed target is verified as Stash NG."
  fi
  return 0
}

generated_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
collect_host_containers

printf '{'
printf '"schemaVersion":"stash-ng.unraid.audit.v1",'
printf '"generatedBy":"stash-ng-audit",'
printf '"generatedAt":'
json_string "$generated_at"
printf ','
printf '"mode":'
if [ -n "$fixture" ]; then
  json_string "fixture"
else
  json_string "host"
fi
printf ','
printf '"redactionDefault":"redacted",'
printf '"findings":['
emit_container_findings
emit_template_and_storage_findings
emit_server_identity_findings
printf ']}'
printf '\n'
