finding() {
  id="$1"
  severity="$2"
  title="$3"
  evidence="$4"
  action="$5"
  printf '{"id":'
  json_string "$id"
  printf ',"severity":'
  json_string "$severity"
  printf ',"title":'
  json_string "$title"
  printf ',"evidence":'
  json_string "$evidence"
  printf ',"recommendedAction":'
  json_string "$action"
  printf '}'
}

info_finding() {
  finding "$1" "Info" "$2" "$3" "$4"
}

risk_finding() {
  finding "$1" "Risk" "$2" "$3" "$4"
}

blocking_finding() {
  finding "$1" "Blocking" "$2" "$3" "$4"
}

performance_finding() {
  finding "$1" "Performance" "$2" "$3" "$4"
}
