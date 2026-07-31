# Pure-ish transformation helpers.

slugify() {
  local result
  result=$(echo "$1" \
    | tr '[:upper:]' '[:lower:]' \
    | sed 's/[^a-z0-9]/-/g' \
    | sed 's/--*/-/g' \
    | sed 's/^-//;s/-$//')
  if [[ -z "$result" ]]; then
    result="issue"
  fi
  echo "$result"
}

jira_to_markdown() {
  local text="$1"
  text=$(tr -d '\r' <<< "$text")
  text=$(sed -E 's/^\*\*\*[[:space:]]+/      - /;s/^\*\*[[:space:]]+/    - /;s/^\*[[:space:]]+/- /' <<< "$text")
  text=$(sed -E 's/^###[[:space:]]+/      1. /;s/^##[[:space:]]+/    1. /;s/^#[[:space:]]+/1. /' <<< "$text")
  text=$(sed -E 's/^h1\.[[:space:]]*/# /;s/^h2\.[[:space:]]*/## /;s/^h3\.[[:space:]]*/### /;s/^h4\.[[:space:]]*/#### /' <<< "$text")
  text=$(sed -E 's/\*([^*]+)\*/**\1**/g' <<< "$text")
  text=$(sed -E 's/(^|[[:space:]])_([^_]+)_([[:space:],.:;!?)]|$)/\1*\2*\3/g' <<< "$text")
  text=$(sed -E 's/(^|[[:space:]])-([^- ][^-]*)-([[:space:]]|[,.:;!?)]|$)/\1~~\2~~\3/g' <<< "$text")
  text=$(sed -E 's/\[([^|]*)\|([^]]*)\]/[\1](\2)/g' <<< "$text")
  text=$(sed -E 's/\{\{([^}]*)\}\}/`\1`/g' <<< "$text")
  text=$(sed -E 's/\{code(:[^}]*)?\}/```/g' <<< "$text")
  text=$(sed 's/{noformat}/```/g' <<< "$text")
  text=$(sed -E 's/\{panel(:[^}]*)?\}//g;s/\{color(:[^}]*)?\}//g;s/\{quote\}//g' <<< "$text")
  text=$(awk '!code && NR>1 && /^./ && prev ~ /^./ {print ""}
             /^```/{code=!code}
             {print; prev=$0}' <<< "$text")
  printf '%s\n' "$text"
}
