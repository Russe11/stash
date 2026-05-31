redact_evidence() {
  value="$1"
  value="$(printf '%s' "$value" | sed -e 's#/mnt/cache/#/mnt/<redacted>/#g' -e 's#/mnt/user/#/mnt/<redacted>/#g' -e 's#/mnt/disk[0-9][0-9]*/#/mnt/<redacted>/#g')"
  printf '%s' "$value"
}
