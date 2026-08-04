#!/usr/bin/env bash

# Focused failure-path checks for the P0 audit. This file is sourced by run.sh.

run_offline_cli_validation_smoke() {
  local name="offline-cli-validation-smoke"
  local tmpdir stubdir command_log output rc

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  command_log="$tmpdir/commands"
  mkdir -p "$stubdir"
  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"${COMMAND_LOG:?}"
exit 99
EOF
  chmod +x "$stubdir/glab"

  set +e
  output="$(
    PATH="$stubdir:$PATH" \
    COMMAND_LOG="$command_log" \
    "$ROOT_DIR/src/glab-helper" --dry-rnu 2>&1
  )"
  rc=$?
  set -e

  if [[ $rc -eq 0 || "$output" != *"Unknown argument: --dry-rnu"* || "$output" != *"Usage:"* ]]; then
    fail "$name" "$output"
  fi

  if ! output="$(
    PATH="$stubdir:$PATH" \
    COMMAND_LOG="$command_log" \
    "$ROOT_DIR/src/glab-helper" --version 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != "glab-helper 0.1.0" || -e "$command_log" ]]; then
    fail "$name" "Offline flags contacted GitLab or returned unexpected output: $output"
  fi

  pass "$name"
}

run_jira_pagination_contract_smoke() {
  local name="jira-pagination-contract-smoke"
  local tmpdir stubdir request_log output rc offsets

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  request_log="$tmpdir/requests"
  mkdir -p "$stubdir"
  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$stubdir/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

url="${@: -1}"
printf '%s\n' "$url" >>"${REQUEST_LOG:?}"
start="${url##*startAt=}"
start="${start%%&*}"

case "${PAGINATION_SCENARIO:?}:$start" in
  capped:0|capped:50|capped:100)
    jq -cn --argjson start "$start" '
      {startAt:$start,maxResults:50,total:150,
       issues:[range($start; $start + 50) |
         {key:("PROJ-" + ((. + 1) | tostring)),
          fields:{summary:"Story",status:{name:"To Do"},priority:null,labels:[],customfield_10000:null,subtasks:[]}}]}'
    ;;
  no-total:0)
    jq -cn '{startAt:0,maxResults:2,issues:[
      {key:"PROJ-1",fields:{summary:"Story",status:{name:"To Do"},priority:null,labels:[],customfield_10000:null,subtasks:[]}},
      {key:"PROJ-2",fields:{summary:"Story",status:{name:"To Do"},priority:null,labels:[],customfield_10000:null,subtasks:[]}}]}'
    ;;
  no-total:2)
    jq -cn '{startAt:2,maxResults:2,issues:[
      {key:"PROJ-3",fields:{summary:"Story",status:{name:"To Do"},priority:null,labels:[],customfield_10000:null,subtasks:[]}}]}'
    ;;
  no-total:3)
    jq -cn '{startAt:3,maxResults:2,issues:[]}'
    ;;
  invalid:0)
    jq -cn '{startAt:0,maxResults:50,total:100,issues:[range(0;50) |
      {key:("PROJ-" + ((. + 1) | tostring)),
       fields:{summary:"Story",status:{name:"To Do"},priority:null,labels:[],customfield_10000:null,subtasks:[]}}]}'
    ;;
  invalid:50)
    printf '<html>proxy error</html>\n'
    ;;
  empty-known:0)
    jq -cn '{startAt:0,maxResults:50,total:150,issues:[range(0;50) |
      {key:("PROJ-" + ((. + 1) | tostring)),
       fields:{summary:"Story",status:{name:"To Do"},priority:null,labels:[],customfield_10000:null,subtasks:[]}}]}'
    ;;
  empty-known:50)
    jq -cn '{startAt:50,maxResults:50,total:150,issues:[]}'
    ;;
  changing-total:0)
    jq -cn '{startAt:0,maxResults:50,total:200,issues:[range(0;50) |
      {key:("PROJ-" + ((. + 1) | tostring)),
       fields:{summary:"Story",status:{name:"To Do"},priority:null,labels:[],customfield_10000:null,subtasks:[]}}]}'
    ;;
  changing-total:50)
    jq -cn '{startAt:50,maxResults:50,total:75,issues:[range(50;75) |
      {key:("PROJ-" + ((. + 1) | tostring)),
       fields:{summary:"Story",status:{name:"To Do"},priority:null,labels:[],customfield_10000:null,subtasks:[]}}]}'
    ;;
  *)
    printf 'unexpected pagination request: %s\n' "$url" >&2
    exit 1
    ;;
esac
EOF
  chmod +x "$stubdir/curl"

  run_pagination_case() {
    local scenario="$1"
    : >"$request_log"
    PATH="$stubdir:$PATH" \
    REQUEST_LOG="$request_log" \
    PAGINATION_SCENARIO="$scenario" \
    GLAB_HELPER_TEST_ROOT="$ROOT_DIR" \
    zsh -c '
      emulate -R zsh
      source "$GLAB_HELPER_TEST_ROOT/src/lib/common.zsh"
      source "$GLAB_HELPER_TEST_ROOT/src/lib/jira_api.zsh"
      JIRA_URL="https://jira.example.com"
      JIRA_TOKEN="secret"
      JIRA_BOARD_LABELS="team-a"
      fetch_jira_stories_paginated || exit 1
      printf "%s\n" "$JIRA_STORIES_COUNT"
    '
  }

  if ! output="$(run_pagination_case capped 2>&1)"; then
    fail "$name" "$output"
  fi
  offsets="$(sed -E 's/.*startAt=([0-9]+).*/\1/' "$request_log" | paste -sd, -)"
  if [[ "$output" != "150" || "$offsets" != "0,50,100" ]]; then
    fail "$name" "Capped pagination returned count=$output offsets=$offsets"
  fi

  if ! output="$(run_pagination_case no-total 2>&1)"; then
    fail "$name" "$output"
  fi
  offsets="$(sed -E 's/.*startAt=([0-9]+).*/\1/' "$request_log" | paste -sd, -)"
  if [[ "$output" != "3" || "$offsets" != "0,2,3" ]]; then
    fail "$name" "Pagination without total returned count=$output offsets=$offsets"
  fi

  if ! output="$(run_pagination_case changing-total 2>&1)"; then
    fail "$name" "$output"
  fi
  offsets="$(sed -E 's/.*startAt=([0-9]+).*/\1/' "$request_log" | paste -sd, -)"
  if [[ "$output" != "75" || "$offsets" != "0,50" ]]; then
    fail "$name" "Changing total returned count=$output offsets=$offsets"
  fi

  for scenario in invalid empty-known; do
    set +e
    output="$(run_pagination_case "$scenario" 2>&1)"
    rc=$?
    set -e
    if [[ $rc -eq 0 || "$output" != *"Jira Story search"* ]]; then
      fail "$name" "Scenario $scenario did not fail closed: $output"
    fi
  done

  pass "$name"
}

run_gitlab_read_failure_smoke() {
  local name="gitlab-read-failure-smoke"
  local tmpdir stubdir responses_file command_log output rc scenario

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  command_log="$tmpdir/commands"
  mkdir -p "$stubdir"
  trap 'rm -rf "$tmpdir"' RETURN

  write_clear_stub "$stubdir"
  write_fzf_stub "$stubdir"

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${COMMAND_LOG:?}"

case "$*" in
  "repo view --output json")
    printf '%s\n' '{"path_with_namespace":"group/project","id":1}'
    ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_URL")
    printf '%s\n' '{"value":"https://jira.example.com"}'
    ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_BOARD_LABELS")
    printf '%s\n' '{"value":"team-a"}'
    ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_TOKEN")
    printf '%s\n' '{"value":"token"}'
    ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_TARGET_PROJECT")
    printf '%s\n' '{"value":"group/project"}'
    ;;
  "api projects/1/issues?state=all&per_page=100&page=1")
    if [[ "${READ_SCENARIO:?}" == "invalid" ]]; then
      printf '<html>not JSON</html>\n'
    else
      jq -cn '[range(0;100) | {iid:(. + 1),title:("Issue " + ((. + 1) | tostring)),description:"",labels:[],milestone:null}]'
    fi
    ;;
  "api projects/1/issues?state=all&per_page=100&page=2")
    printf 'HTTP 500 while reading page 2\n' >&2
    exit 1
    ;;
  *)
    printf 'unexpected glab invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  cat >"$stubdir/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' '{"startAt":0,"maxResults":100,"total":1,"issues":[{"key":"PROJ-1","fields":{"summary":"Story","description":"","status":{"name":"To Do"},"priority":null,"labels":[],"customfield_10000":"","subtasks":[]}}]}'
EOF
  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  for scenario in late invalid; do
    printf '%s\n' '~ Sync stories from Jira' >"$responses_file"
    : >"$command_log"
    set +e
    output="$(
      PATH="$stubdir:$PATH" \
      TERM=xterm \
      COMMAND_LOG="$command_log" \
      READ_SCENARIO="$scenario" \
      FZF_RESPONSES_FILE="$responses_file" \
      "$ROOT_DIR/src/glab-helper" 2>&1
    )"
    rc=$?
    set -e

    if [[ $rc -eq 0 || "$output" != *"could not be read completely"* ]]; then
      fail "$name" "Scenario $scenario did not fail closed: $output"
    fi
    if rg -q -- '(^| )(issue (create|update|close)|label create|api .* -X (POST|PUT|PATCH|DELETE))' "$command_log"; then
      fail "$name" "Scenario $scenario mutated GitLab: $(cat "$command_log")"
    fi
  done

  pass "$name"
}

run_gitlab_http_status_smoke() {
  local name="gitlab-http-status-smoke"
  local tmpdir stubdir output rc status

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  mkdir -p "$stubdir"
  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
printf 'HTTP %s from GitLab\n' "${HTTP_STATUS:?}" >&2
exit 1
EOF
  chmod +x "$stubdir/glab"

  for status in 401 403 429 500; do
    set +e
    output="$(
      PATH="$stubdir:$PATH" \
      HTTP_STATUS="$status" \
      GLAB_HELPER_TEST_ROOT="$ROOT_DIR" \
      zsh -c '
        emulate -R zsh
        source "$GLAB_HELPER_TEST_ROOT/src/lib/common.zsh"
        source "$GLAB_HELPER_TEST_ROOT/src/lib/gitlab_api.zsh"
        project_id=1
        fetch_all_labels
      ' 2>&1
    )"
    rc=$?
    set -e
    if [[ $rc -eq 0 || "$output" != *"HTTP ${status}"* || "$output" != *"GitLab labels page 1"* ]]; then
      fail "$name" "HTTP $status was not reported clearly: $output"
    fi
  done

  pass "$name"
}

run_gitlab_native_pagination_smoke() {
  local name="gitlab-native-pagination-smoke"
  local tmpdir stubdir output rc

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  mkdir -p "$stubdir"
  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$*" != "api --paginate projects/1/labels?per_page=100" ]]; then
  printf 'unexpected glab invocation: %s\n' "$*" >&2
  exit 1
fi
printf '%s\n' '[{"name":"first"}]'
if [[ "${PAGINATE_SCENARIO:-success}" == "failure" ]]; then
  printf 'HTTP 500 on a later page\n' >&2
  exit 1
fi
printf '%s\n' '[{"name":"second"}]'
EOF
  chmod +x "$stubdir/glab"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    GLAB_HELPER_PAGINATION_MODE=paginate \
    GLAB_HELPER_TEST_ROOT="$ROOT_DIR" \
    zsh -c '
      emulate -R zsh
      source "$GLAB_HELPER_TEST_ROOT/src/lib/common.zsh"
      source "$GLAB_HELPER_TEST_ROOT/src/lib/gitlab_api.zsh"
      project_id=1
      fetch_all_labels | jq -r "map(.name) | join(\",\")"
    ' 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != "first,second" ]]; then
    fail "$name" "$output"
  fi

  set +e
  output="$(
    PATH="$stubdir:$PATH" \
    PAGINATE_SCENARIO=failure \
    GLAB_HELPER_PAGINATION_MODE=paginate \
    GLAB_HELPER_TEST_ROOT="$ROOT_DIR" \
    zsh -c '
      emulate -R zsh
      source "$GLAB_HELPER_TEST_ROOT/src/lib/common.zsh"
      source "$GLAB_HELPER_TEST_ROOT/src/lib/gitlab_api.zsh"
      project_id=1
      fetch_all_labels
    ' 2>&1
  )"
  rc=$?
  set -e
  if [[ $rc -eq 0 || "$output" != *"HTTP 500 on a later page"* ]]; then
    fail "$name" "Native pagination did not reject a partial response: $output"
  fi

  pass "$name"
}

run_epic_failure_blocks_story_sync_smoke() {
  local name="epic-failure-blocks-story-sync-smoke"
  local tmpdir stubdir responses_file command_log output rc

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  command_log="$tmpdir/commands"
  mkdir -p "$stubdir"
  trap 'rm -rf "$tmpdir"' RETURN

  printf '%s\n' '~ Sync stories from Jira' >"$responses_file"
  write_clear_stub "$stubdir"
  write_fzf_stub "$stubdir"

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${COMMAND_LOG:?}"
case "$*" in
  "repo view --output json") printf '%s\n' '{"path_with_namespace":"group/project","id":1}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_URL") printf '%s\n' '{"value":"https://jira.example.com"}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_BOARD_LABELS") printf '%s\n' '{"value":"team-a"}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_TOKEN") printf '%s\n' '{"value":"token"}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_TARGET_PROJECT") printf '%s\n' '{"value":"group/project"}' ;;
  "api projects/1/issues?state=all&per_page=100&page=1")
    printf '%s\n' '[{"iid":9,"title":"[PROJ-1] Story","description":"","labels":[],"milestone":{"title":"Keep Me"},"state":"opened"}]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[{"id":5,"title":"Keep Me","description":""}]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[]'
    ;;
  *) printf 'unexpected glab invocation: %s\n' "$*" >&2; exit 1 ;;
esac
EOF

  cat >"$stubdir/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${@: -1}" in
  *"issuetype%20%3D%20Story"*)
    printf '%s\n' '{"startAt":0,"maxResults":100,"total":1,"issues":[{"key":"PROJ-1","fields":{"summary":"Story","description":"","status":{"name":"To Do"},"priority":null,"labels":[],"customfield_10000":"EPIC-1","subtasks":[]}}]}'
    ;;
  *"issuetype%20%3D%20Epic"*)
    if [[ "${EPIC_SCENARIO:-failure}" == "missing" ]]; then
      printf '%s\n' '{"startAt":0,"maxResults":100,"total":0,"issues":[]}'
    else
      printf 'HTTP 500 from Jira epic search\n' >&2
      exit 1
    fi
    ;;
  *) exit 1 ;;
esac
EOF
  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  set +e
  output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    COMMAND_LOG="$command_log" \
    EPIC_SCENARIO=failure \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" 2>&1
  )"
  rc=$?
  set -e

  if [[ $rc -eq 0 || "$output" != *"epic data is incomplete"* || "$output" != *"No milestone or issue changes"* ]]; then
    fail "$name" "$output"
  fi
  if rg -q -- 'milestone_id=0|(^| )(issue (create|update|close)|label create|api .* -X (POST|PUT|PATCH|DELETE))' "$command_log"; then
    fail "$name" "Epic failure caused a mutation: $(cat "$command_log")"
  fi

  printf '%s\n' '~ Preview story sync from Jira' >"$responses_file"
  : >"$command_log"
  set +e
  output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    COMMAND_LOG="$command_log" \
    EPIC_SCENARIO=missing \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" --dry-run 2>&1
  )"
  rc=$?
  set -e
  if [[ $rc -eq 0 || "$output" != *"Milestone changes are suspended"* \
    || "$output" == *"milestone: Keep Me -> none"* ]]; then
    fail "$name" "An unresolved epic did not preserve its milestone: $output"
  fi

  pass "$name"
}

run_dry_run_write_barrier_smoke() {
  local name="dry-run-write-barrier-smoke"
  local tmpdir stubdir responses_file command_log output rc mode

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  command_log="$tmpdir/commands"
  mkdir -p "$stubdir"
  trap 'rm -rf "$tmpdir"' RETURN

  write_clear_stub "$stubdir"
  write_fzf_stub "$stubdir"

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${COMMAND_LOG:?}"
case "$*" in
  "repo view --output json") printf '%s\n' '{"path_with_namespace":"group/project","id":1}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_URL") printf '%s\n' '{"value":"https://jira.example.com"}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_BOARD_LABELS") printf '%s\n' '{"value":"team-a"}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_TOKEN") printf '%s\n' '{"value":"token"}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_TARGET_PROJECT") printf '%s\n' '{"value":"group/project"}' ;;
  "api projects/1/issues?state=all&per_page=100&page=1") printf '%s\n' '[]' ;;
  "api projects/1/milestones?state=active&per_page=100&page=1") printf '%s\n' '[]' ;;
  "api projects/1/labels?per_page=100&page=1") printf '%s\n' '[]' ;;
  *) printf 'unexpected glab invocation: %s\n' "$*" >&2; exit 1 ;;
esac
EOF

  cat >"$stubdir/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${@: -1}" in
  *"issuetype%20%3D%20Story"*)
    printf '%s\n' '{"startAt":0,"maxResults":100,"total":1,"issues":[{"key":"PROJ-1","fields":{"summary":"Preview only","description":"","status":{"name":"To Do"},"priority":null,"labels":[],"customfield_10000":"","subtasks":[]}}]}'
    ;;
  *"issuetype%20%3D%20Epic"*)
    printf '%s\n' '{"startAt":0,"maxResults":100,"total":0,"issues":[]}'
    ;;
  *) exit 1 ;;
esac
EOF
  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  for mode in plain dev; do
    printf '%s\n' '~ Preview story sync from Jira' >"$responses_file"
    : >"$command_log"
    if [[ "$mode" == "dev" ]]; then
      output="$(
        PATH="$stubdir:$PATH" TERM=xterm COMMAND_LOG="$command_log" \
        FZF_RESPONSES_FILE="$responses_file" \
        "$ROOT_DIR/src/glab-helper" --dev --dry-run 2>&1
      )"
      rc=$?
    else
      output="$(
        PATH="$stubdir:$PATH" TERM=xterm COMMAND_LOG="$command_log" \
        FZF_RESPONSES_FILE="$responses_file" \
        "$ROOT_DIR/src/glab-helper" --dry-run 2>&1
      )"
      rc=$?
    fi

    if [[ $rc -ne 0 || "$output" != *"No GitLab changes were applied."* ]]; then
      fail "$name" "$output"
    fi
    if rg -q -- '(^| )(issue (create|update|close)|label create|api .* -X (POST|PUT|PATCH|DELETE))' "$command_log"; then
      fail "$name" "Mode $mode executed a write: $(cat "$command_log")"
    fi
  done

  pass "$name"
}

run_partial_failure_exit_code_smoke() {
  local name="partial-failure-exit-code-smoke"
  local tmpdir stubdir responses_file command_log output rc post_count

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  command_log="$tmpdir/commands"
  mkdir -p "$stubdir"
  trap 'rm -rf "$tmpdir"' RETURN

  printf '%s\n' '~ Sync stories from Jira' >"$responses_file"
  write_clear_stub "$stubdir"
  write_fzf_stub "$stubdir"

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${COMMAND_LOG:?}"
case "$*" in
  "repo view --output json") printf '%s\n' '{"path_with_namespace":"group/project","id":1}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_URL") printf '%s\n' '{"value":"https://jira.example.com"}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_BOARD_LABELS") printf '%s\n' '{"value":"team-a"}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_TOKEN") printf '%s\n' '{"value":"token"}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_TARGET_PROJECT") printf '%s\n' '{"value":"group/project"}' ;;
  "api projects/1/issues?state=all&per_page=100&page=1") printf '%s\n' '[]' ;;
  "api projects/1/milestones?state=active&per_page=100&page=1") printf '%s\n' '[]' ;;
  "api projects/1/labels?per_page=100&page=1") printf '%s\n' '[]' ;;
  "api projects/1/issues -X POST -f title=[PROJ-1] Failing story -f description=")
    printf 'connection lost after request\n' >&2
    exit 1
    ;;
  *) printf 'unexpected glab invocation: %s\n' "$*" >&2; exit 1 ;;
esac
EOF

  cat >"$stubdir/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${@: -1}" in
  *"issuetype%20%3D%20Story"*)
    printf '%s\n' '{"startAt":0,"maxResults":100,"total":1,"issues":[{"key":"PROJ-1","fields":{"summary":"Failing story","description":"","status":{"name":"To Do"},"priority":null,"labels":[],"customfield_10000":"","subtasks":[]}}]}'
    ;;
  *"issuetype%20%3D%20Epic"*)
    printf '%s\n' '{"startAt":0,"maxResults":100,"total":0,"issues":[]}'
    ;;
  *) exit 1 ;;
esac
EOF
  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  set +e
  output="$(
    PATH="$stubdir:$PATH" TERM=xterm COMMAND_LOG="$command_log" \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" <<< $'y\n' 2>&1
  )"
  rc=$?
  set -e

  post_count="$(rg -c -F 'api projects/1/issues -X POST' "$command_log" || true)"
  if [[ $rc -eq 0 || "$output" != *"1 failed"* || "$post_count" != "1" ]]; then
    fail "$name" "rc=$rc post_count=$post_count output=$output"
  fi

  pass "$name"
}

run_snapshot_read_failure_smoke() {
  local name="snapshot-read-failure-smoke"
  local tmpdir workdir stubdir responses_file output rc

  tmpdir="$(mktemp -d)"
  workdir="$tmpdir/work"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir" "$workdir"
  trap 'rm -rf "$tmpdir"' RETURN

  printf '%s\n' 'Export GitLab snapshot' >"$responses_file"
  write_clear_stub "$stubdir"
  write_fzf_stub "$stubdir"

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  "repo view --output json") printf '%s\n' '{"path_with_namespace":"group/project","id":1}' ;;
  api\ projects/ibm%2Fglab-helper/variables/*) exit 1 ;;
  "api projects/1/issues?per_page=100&page=1") printf '%s\n' '[]' ;;
  "api projects/1/milestones?per_page=100&page=1") printf 'HTTP 500\n' >&2; exit 1 ;;
  *) printf 'unexpected glab invocation: %s\n' "$*" >&2; exit 1 ;;
esac
EOF
  cat >"$stubdir/curl" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  set +e
  output="$(
    cd "$workdir"
    PATH="$stubdir:$PATH" TERM=xterm FZF_RESPONSES_FILE="$responses_file" \
      "$ROOT_DIR/src/glab-helper" 2>&1
  )"
  rc=$?
  set -e

  if [[ $rc -eq 0 || "$output" == *"Snapshot exported"* || -e "$workdir/.glab-helper-snapshots" ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}
