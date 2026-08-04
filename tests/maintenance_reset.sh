run_maintenance_menu_smoke() {
  local name="maintenance-menu-smoke"
  local tmpdir stubdir menu_capture output rc

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  menu_capture="$tmpdir/menu"
  mkdir -p "$stubdir"
  trap 'rm -rf "$tmpdir"' RETURN

  write_clear_stub "$stubdir"

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
cat >"${FZF_CAPTURE_FILE:?}"
exit 1
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  "repo view --output json") printf '%s\n' '{"path_with_namespace":"group/project","id":1}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_URL") printf '%s\n' '{"value":"https://jira.example.com"}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_BOARD_LABELS") printf '%s\n' '{"value":"team-a"}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_TOKEN") printf '%s\n' '{"value":"token"}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_TARGET_PROJECT")
    printf '{"value":"%s"}\n' "${TARGET_PROJECT:-group/project}"
    ;;
  *) printf 'unexpected glab invocation: %s\n' "$*" >&2; exit 1 ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab"

  if ! output="$(
    PATH="$stubdir:$PATH" TERM=xterm FZF_CAPTURE_FILE="$menu_capture" \
      "$ROOT_DIR/src/glab-helper" --maintenance 2>&1
  )"; then
    fail "$name" "$output"
  fi
  if [[ "$(cat "$menu_capture")" != $'! Reset all issues & milestones\n× Exit' ]]; then
    fail "$name" "$output"
  fi

  if ! output="$(
    PATH="$stubdir:$PATH" TERM=xterm FZF_CAPTURE_FILE="$menu_capture" \
      "$ROOT_DIR/src/glab-helper" --maintenance --dry-run 2>&1
  )"; then
    fail "$name" "$output"
  fi
  if [[ "$(cat "$menu_capture")" != $'! Preview full project reset\n× Exit' ]]; then
    fail "$name" "$output"
  fi

  set +e
  output="$(
    PATH="$stubdir:$PATH" TERM=xterm TARGET_PROJECT=other/project \
      FZF_CAPTURE_FILE="$menu_capture" \
      "$ROOT_DIR/src/glab-helper" --maintenance 2>&1
  )"
  rc=$?
  set -e
  if [[ $rc -eq 0 || "$output" != *"requires the configured Jira target project"* ]]; then
    fail "$name" "Target-project mismatch was not blocked: $output"
  fi

  pass "$name"
}

write_maintenance_reset_stubs() {
  local stubdir="$1"

  write_clear_stub "$stubdir"
  write_fzf_stub "$stubdir"

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${COMMAND_LOG:?}"

issues='[
  {"iid":11,"state":"opened","title":"[PROJ-1] Synced legacy story","description":""},
  {"iid":12,"state":"closed","title":"Renamed synced story","description":"<!-- glab-helper:jira-story:PROJ-2 -->"},
  {"iid":13,"state":"opened","title":"Manual issue","description":"Keep me"}
]'
issues_changed='[
  {"iid":11,"state":"opened","title":"[PROJ-1] Changed after preview","description":""},
  {"iid":12,"state":"closed","title":"Renamed synced story","description":"<!-- glab-helper:jira-story:PROJ-2 -->"},
  {"iid":13,"state":"opened","title":"Manual issue","description":"Keep me"}
]'
milestones='[
  {"id":21,"title":"Synced epic","description":"<!-- jira:EPIC-1 -->"},
  {"id":22,"title":"Manual milestone","description":"Keep me"}
]'

case "$*" in
  "repo view --output json") printf '%s\n' '{"path_with_namespace":"group/project","id":1}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_URL") printf '%s\n' '{"value":"https://jira.example.com"}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_BOARD_LABELS") printf '%s\n' '{"value":"team-a"}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_TOKEN") printf '%s\n' '{"value":"token"}' ;;
  "api projects/ibm%2Fglab-helper/variables/JIRA_TARGET_PROJECT") printf '%s\n' '{"value":"group/project"}' ;;
  "api projects/1/issues?state=all&per_page=100&page=1")
    if [[ "${RESET_SCENARIO:-success}" == "plan-change" \
      && "$(grep -cF 'api projects/1/issues?state=all&per_page=100&page=1' "${COMMAND_LOG:?}")" -gt 1 ]]; then
      printf '%s\n' "$issues_changed"
    else
      printf '%s\n' "$issues"
    fi
    ;;
  "api projects/1/issues?per_page=100&page=1")
    if [[ "${RESET_SCENARIO:-success}" == "snapshot-failure" ]]; then
      printf 'HTTP 500 during snapshot\n' >&2
      exit 1
    fi
    printf '%s\n' "$issues"
    ;;
  "api projects/1/milestones?per_page=100&page=1")
    printf '%s\n' "$milestones"
    ;;
  "api projects/1/labels?per_page=100&page=1") printf '%s\n' '[{"name":"team-a"}]' ;;
  "api projects/1/issues/11 -X DELETE")
    if [[ "${RESET_SCENARIO:-success}" == "issue-failure" ]]; then
      printf 'HTTP 403\n' >&2
      exit 1
    fi
    ;;
  "api projects/1/issues/12 -X DELETE") ;;
  "api projects/1/issues/13 -X DELETE") ;;
  "api projects/1/milestones/21 -X DELETE") ;;
  "api projects/1/milestones/22 -X DELETE") ;;
  *) printf 'unexpected glab invocation: %s\n' "$*" >&2; exit 1 ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab"
}

run_maintenance_reset_dry_run_smoke() {
  local name="maintenance-reset-dry-run-smoke"
  local tmpdir workdir stubdir responses_file command_log output

  tmpdir="$(mktemp -d)"
  workdir="$tmpdir/work"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  command_log="$tmpdir/commands"
  mkdir -p "$workdir" "$stubdir"
  trap 'rm -rf "$tmpdir"' RETURN

  printf '%s\n' '! Preview full project reset' >"$responses_file"
  : >"$command_log"
  write_maintenance_reset_stubs "$stubdir"

  if ! output="$(
    cd "$workdir"
    PATH="$stubdir:$PATH" TERM=xterm COMMAND_LOG="$command_log" \
      FZF_RESPONSES_FILE="$responses_file" \
      "$ROOT_DIR/src/glab-helper" --maintenance --dry-run 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"issues will be permanently deleted"* \
    || "$output" != *"milestones will be permanently deleted"* \
    || "$output" != *"#11"* \
    || "$output" != *"#12"* \
    || "$output" != *"#13"* \
    || "$output" != *"Manual milestone"* \
    || "$output" != *"No GitLab changes or local snapshots were written"* ]]; then
    fail "$name" "$output"
  fi
  if rg -q -- '-X DELETE' "$command_log" || [[ -e "$workdir/.glab-helper-snapshots" ]]; then
    fail "$name" "Dry-run wrote data: $(cat "$command_log")"
  fi

  printf '%s\n' '! Reset all issues & milestones' >"$responses_file"
  : >"$command_log"
  if ! output="$(
    cd "$workdir"
    PATH="$stubdir:$PATH" TERM=xterm COMMAND_LOG="$command_log" \
      FZF_RESPONSES_FILE="$responses_file" \
      "$ROOT_DIR/src/glab-helper" --maintenance <<< 'wrong project' 2>&1
  )"; then
    fail "$name" "$output"
  fi
  if [[ "$output" != *"Aborted. Nothing was deleted"* \
    || -e "$workdir/.glab-helper-snapshots" ]] \
    || rg -q -- '-X DELETE' "$command_log"; then
    fail "$name" "Invalid confirmation was not safe: $output"
  fi

  pass "$name"
}

run_maintenance_reset_apply_smoke() {
  local name="maintenance-reset-apply-smoke"
  local tmpdir workdir stubdir responses_file command_log output snapshot_dir

  tmpdir="$(mktemp -d)"
  workdir="$tmpdir/work"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  command_log="$tmpdir/commands"
  mkdir -p "$workdir" "$stubdir"
  trap 'rm -rf "$tmpdir"' RETURN

  printf '%s\n' '! Reset all issues & milestones' >"$responses_file"
  : >"$command_log"
  write_maintenance_reset_stubs "$stubdir"

  if ! output="$(
    cd "$workdir"
    PATH="$stubdir:$PATH" TERM=xterm COMMAND_LOG="$command_log" \
      FZF_RESPONSES_FILE="$responses_file" \
      "$ROOT_DIR/src/glab-helper" --maintenance <<< 'RESET ALL group/project' 2>&1
  )"; then
    fail "$name" "$output"
  fi

  snapshot_dir="$(find "$workdir/.glab-helper-snapshots" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
  if [[ "$output" != *"Reset complete: 3 issues and 2 milestones deleted"* \
    || -z "$snapshot_dir" \
    || ! -f "$snapshot_dir/issues.json" \
    || ! -f "$snapshot_dir/milestones.json" \
    || ! -f "$snapshot_dir/labels.json" ]]; then
    fail "$name" "$output"
  fi
  if ! rg -q -F 'api projects/1/issues/11 -X DELETE' "$command_log" \
    || ! rg -q -F 'api projects/1/issues/12 -X DELETE' "$command_log" \
    || ! rg -q -F 'api projects/1/issues/13 -X DELETE' "$command_log" \
    || ! rg -q -F 'api projects/1/milestones/21 -X DELETE' "$command_log" \
    || ! rg -q -F 'api projects/1/milestones/22 -X DELETE' "$command_log"; then
    fail "$name" "Deletion scope was incorrect: $(cat "$command_log")"
  fi

  pass "$name"
}

run_maintenance_reset_failure_smoke() {
  local name="maintenance-reset-failure-smoke"
  local tmpdir workdir stubdir responses_file command_log output rc

  tmpdir="$(mktemp -d)"
  workdir="$tmpdir/work"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  command_log="$tmpdir/commands"
  mkdir -p "$workdir" "$stubdir"
  trap 'rm -rf "$tmpdir"' RETURN

  printf '%s\n' '! Reset all issues & milestones' >"$responses_file"
  : >"$command_log"
  write_maintenance_reset_stubs "$stubdir"

  set +e
  output="$(
    cd "$workdir"
    PATH="$stubdir:$PATH" TERM=xterm COMMAND_LOG="$command_log" \
      RESET_SCENARIO=issue-failure FZF_RESPONSES_FILE="$responses_file" \
      "$ROOT_DIR/src/glab-helper" --maintenance <<< 'RESET ALL group/project' 2>&1
  )"
  rc=$?
  set -e

  if [[ $rc -eq 0 \
    || "$output" != *"Milestone deletion skipped"* \
    || "$output" != *"HTTP 403"* \
    || ! -d "$workdir/.glab-helper-snapshots" ]] \
    || rg -q -- 'milestones/[0-9]+ -X DELETE' "$command_log"; then
    fail "$name" "$output"
  fi

  printf '%s\n' '! Reset all issues & milestones' >"$responses_file"
  : >"$command_log"
  set +e
  output="$(
    cd "$workdir"
    PATH="$stubdir:$PATH" TERM=xterm COMMAND_LOG="$command_log" \
      RESET_SCENARIO=plan-change FZF_RESPONSES_FILE="$responses_file" \
      "$ROOT_DIR/src/glab-helper" --maintenance <<< 'RESET ALL group/project' 2>&1
  )"
  rc=$?
  set -e

  if [[ $rc -eq 0 \
    || "$output" != *"deletion plan changed after confirmation"* ]] \
    || rg -q -- '-X DELETE' "$command_log"; then
    fail "$name" "Changed plan was not blocked: $output"
  fi

  printf '%s\n' '! Reset all issues & milestones' >"$responses_file"
  : >"$command_log"
  set +e
  output="$(
    cd "$workdir"
    PATH="$stubdir:$PATH" TERM=xterm COMMAND_LOG="$command_log" \
      RESET_SCENARIO=snapshot-failure FZF_RESPONSES_FILE="$responses_file" \
      "$ROOT_DIR/src/glab-helper" --maintenance <<< 'RESET ALL group/project' 2>&1
  )"
  rc=$?
  set -e

  if [[ $rc -eq 0 \
    || "$output" != *"mandatory snapshot failed"* ]] \
    || rg -q -- '-X DELETE' "$command_log"; then
    fail "$name" "Snapshot failure did not block deletion: $output"
  fi

  pass "$name"
}
