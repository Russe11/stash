json_escape() {
  awk 'BEGIN {
    s = ARGV[1]
    ARGV[1] = ""
    gsub(/\\/, "\\\\", s)
    gsub(/"/, "\\\"", s)
    gsub(/\t/, "\\t", s)
    gsub(/\r/, "\\r", s)
    gsub(/\n/, "\\n", s)
    printf "%s", s
  }' "$1"
}

json_string() {
  printf '"%s"' "$(json_escape "$1")"
}

json_bool() {
  case "$1" in
    true|false) printf '%s' "$1" ;;
    *) printf 'false' ;;
  esac
}
