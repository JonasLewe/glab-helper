#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export GLAB_HELPER_RETRY_BASE_DELAY=0
export GLAB_HELPER_PAGINATION_MODE=manual
TEST_TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TEST_TMP_ROOT"' EXIT

pass() {
  printf 'PASS %s\n' "$1"
}

fail() {
  printf 'FAIL %s\n' "$1" >&2
  if [[ $# -gt 1 ]]; then
    printf '%s\n' "$2" >&2
  fi
  exit 1
}

run_check() {
  local name="$1"
  shift

  if "$@" >"$TEST_TMP_ROOT/check.out" 2>&1; then
    pass "$name"
    return 0
  fi

  fail "$name" "$(cat "$TEST_TMP_ROOT/check.out")"
}

run_help_check() {
  local name="$1"
  shift
  local output

  if ! output="$("$@" 2>&1)"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"Usage: glab-helper [--dev] [--dry-run] [--version]"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

write_clear_stub() {
  local stubdir="$1"

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
}

write_fzf_stub() {
  local stubdir="$1"

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF
}

write_nvim_replace_stub() {
  local stubdir="$1"

  cat >"$stubdir/nvim" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
file="${@: -1}"
printf '%s\n' "${STUB_EDITOR_CONTENT:?}" >"$file"
EOF
}

run_main_and_dev_menu_smoke() {
  local name="main-and-dev-menu-smoke"
  local tmpdir stubdir menu_capture output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  menu_capture="$tmpdir/menu"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
cat >"${FZF_CAPTURE_FILE:?}"
if [[ "${FZF_SELECT_EXIT:-}" == "1" ]]; then
  printf '%s\n' '× Exit'
  exit 0
fi
exit 1
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
  *)
    printf 'unexpected glab invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_CAPTURE_FILE="$menu_capture" \
    "$ROOT_DIR/src/glab-helper" 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ ! -f "$menu_capture" \
    || "$(cat "$menu_capture")" != $'~ Sync Jira\n▸ Work on existing issue\n× Exit' ]]; then
    fail "$name" "$output"
  fi

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_CAPTURE_FILE="$menu_capture" \
    "$ROOT_DIR/src/glab-helper" --dev 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$(cat "$menu_capture")" != *"Sync epics from Jira"* \
    || "$(cat "$menu_capture")" != *"Sync stories from Jira"* \
    || "$(cat "$menu_capture")" != *"Create issue"* \
    || "$(cat "$menu_capture")" != *"Work on existing issue"* \
    || "$(cat "$menu_capture")" != *"Export GitLab snapshot"* \
    || "$(cat "$menu_capture")" == *$'\n'"~ Sync Jira"* \
    || "$(tail -n 1 "$menu_capture")" != "× Exit" ]]; then
    fail "$name" "$output"
  fi

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_CAPTURE_FILE="$menu_capture" \
    FZF_SELECT_EXIT=1 \
    "$ROOT_DIR/src/glab-helper" 2>&1
  )" || [[ "$output" != *"Done."* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_dry_run_menu_read_only_smoke() {
  local name="dry-run-menu-read-only-smoke"
  local tmpdir stubdir menu_capture output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  menu_capture="$tmpdir/menu"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

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
  *)
    printf 'unexpected glab invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_CAPTURE_FILE="$menu_capture" \
    "$ROOT_DIR/src/glab-helper" --dry-run 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ ! -f "$menu_capture" || "$(cat "$menu_capture")" != $'~ Preview Jira\n× Exit' ]]; then
    fail "$name" "$output"
  fi

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_CAPTURE_FILE="$menu_capture" \
    "$ROOT_DIR/src/glab-helper" --dev --dry-run 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$(cat "$menu_capture")" != $'~ Preview epic sync from Jira\n~ Preview story sync from Jira\n× Exit' \
    || "$(cat "$menu_capture")" == *"Create issue"* \
    || "$(cat "$menu_capture")" == *"Work on existing issue"* \
    || "$(cat "$menu_capture")" == *"Export GitLab snapshot"* \
    || "$(cat "$menu_capture")" == *"Reset:"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_jira_flow_smoke() {
  local name="jira-create-flow-smoke"
  local tmpdir stubdir responses_file command_log output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  command_log="$tmpdir/commands"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
+ Create issue
▸ From Jira
PROJ-1
EOF

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF

  cat >"$stubdir/nvim" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
  "api projects/1/issues?state=opened&per_page=100&page=1")
    printf '%s\n' '[]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[{"name":"team-a"},{"name":"prio::high"}]'
    ;;
  "api projects/1/members/all?per_page=100&page=1")
    printf '%s\n' '[]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[]'
    ;;
  "api projects/1/milestones -X POST -f title=Epic Alpha -f description=## Scope"$'\n\n'"<!-- jira:EPIC-1 -->")
    printf '%s\n' '{}'
    ;;
  *)
    printf 'unexpected glab invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
printf '%s\n' "$*" >>"${COMMAND_LOG:?}"
EOF

  cat >"$stubdir/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

case "$*" in
  *"/rest/api/2/search?"*"issuetype%20%3D%20Story"* )
    printf '%s\n' '{"issues":[{"key":"PROJ-1","fields":{"summary":"Regression coverage","description":"h2. Summary","status":{"name":"To Do"},"priority":{"name":"high"},"labels":["team-a"],"customfield_10000":"EPIC-1","subtasks":[]}}],"total":1}'
    ;;
  *"/rest/api/2/issue/EPIC-1?fields=summary,description"* )
    printf '%s\n' '{"fields":{"summary":"Epic Alpha","description":"h2. Scope"}}'
    ;;
  *)
    printf 'unexpected curl invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/nvim" "$stubdir/glab" "$stubdir/curl"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    COMMAND_LOG="$command_log" \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" --dev <<< $'\nn\n' 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" == *"local: can only be used in a function"* ]]; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"Summary"* || "$output" != *"Epic Alpha"* || "$output" != *"Aborted."* ]]; then
    fail "$name" "$output"
  fi
  if rg -q -- '(^| )(issue create|label create|api .* -X (POST|PUT|PATCH|DELETE))' "$command_log"; then
    fail "$name" "A mutation ran before final confirmation: $(cat "$command_log")"
  fi

  pass "$name"
}

run_snapshot_export_smoke() {
  local name="snapshot-export-smoke"
  local tmpdir workdir stubdir responses_file output snapshot_root snapshot_dir

  tmpdir="$(mktemp -d)"
  workdir="$tmpdir/work"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$workdir" "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
▸ Export GitLab snapshot
EOF

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
  "api projects/1/issues?per_page=100&page=1")
    printf '%s\n' '[{"iid":11,"title":"Snapshot issue"}]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[{"name":"manual"}]'
    ;;
  "api projects/1/milestones?per_page=100&page=1")
    printf '%s\n' '[{"id":9,"title":"Snapshot milestone"}]'
    ;;
  *)
    printf 'unexpected glab invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab"

  if ! output="$(
    cd "$workdir"
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" --dev 2>&1
  )"; then
    fail "$name" "$output"
  fi

  snapshot_root="$workdir/.glab-helper-snapshots"
  snapshot_dir="$(find "$snapshot_root" -mindepth 1 -maxdepth 1 -type d | head -n 1)"

  if [[ -z "$snapshot_dir" || ! -f "$snapshot_dir/issues.json" || ! -f "$snapshot_dir/milestones.json" || ! -f "$snapshot_dir/labels.json" || ! -f "$snapshot_dir/metadata.json" ]]; then
    fail "$name" "$output"
  fi

  if ! grep -q 'Snapshot issue' "$snapshot_dir/issues.json" || ! grep -q 'Snapshot milestone' "$snapshot_dir/milestones.json" || ! grep -q 'manual' "$snapshot_dir/labels.json"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"Snapshot exported to"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_sync_stories_dry_run_smoke() {
  local name="sync-stories-dry-run-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
~ Preview story sync from Jira
EOF

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
    printf '%s\n' '[{"iid":42,"title":"[PROJ-1] Old summary","description":"Old description","labels":["manual","prio::low"],"milestone":{"title":"Legacy Epic"}}]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[{"name":"manual"},{"name":"team-a"},{"name":"prio::high"}]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[]'
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

case "$*" in
  *"/rest/api/2/search?"*"issuetype%20%3D%20Story"* )
    printf '%s\n' '{"issues":[{"key":"PROJ-1","fields":{"summary":"Regression coverage","description":"h2. Summary","status":{"name":"To Do"},"priority":{"name":"high"},"labels":["team-a"],"customfield_10000":"","subtasks":[]}}],"total":1}'
    ;;
  *"/rest/api/2/search?"*"issuetype%20%3D%20Epic"* )
    printf '%s\n' '{"issues":[],"total":0}'
    ;;
  *)
    printf 'unexpected curl invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" --dev --dry-run 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"Dry-run complete"* || "$output" != *"[PROJ-1] Regression coverage"* || "$output" != *"(title, description, labels, milestone)"* || "$output" != *"title: [PROJ-1] Old summary -> [PROJ-1] Regression coverage"* || "$output" != *"description: changed"* || "$output" != *"priority: prio::low -> prio::high"* || "$output" != *"add: team-a"* || "$output" != *"milestone: Legacy Epic -> none"* || "$output" != *"No GitLab changes were applied."* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_sync_stories_dry_run_noop_smoke() {
  local name="sync-stories-dry-run-noop-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
~ Preview story sync from Jira
EOF

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
    printf '%s\n' '[{"iid":42,"title":"[PROJ-1] Regression coverage","description":"## Summary","labels":["manual","team-a","prio::high"],"milestone":null}]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[{"name":"manual"},{"name":"team-a"},{"name":"prio::high"}]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[]'
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

case "$*" in
  *"/rest/api/2/search?"*"issuetype%20%3D%20Story"* )
    printf '%s\n' '{"issues":[{"key":"PROJ-1","fields":{"summary":"Regression coverage","description":"h2. Summary","status":{"name":"To Do"},"priority":{"name":"high"},"labels":["team-a"],"customfield_10000":"","subtasks":[]}}],"total":1}'
    ;;
  *"/rest/api/2/search?"*"issuetype%20%3D%20Epic"* )
    printf '%s\n' '{"issues":[],"total":0}'
    ;;
  *)
    printf 'unexpected curl invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" --dev --dry-run 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"No issue updates required."* || "$output" != *"1 synced issues already up-to-date"* || "$output" != *"0 to create"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_sync_stories_dry_run_mixed_plan_smoke() {
  local name="sync-stories-dry-run-mixed-plan-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
~ Preview story sync from Jira
EOF

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
    printf '%s\n' '[{"iid":42,"title":"[PROJ-1] Old summary","description":"Old description","labels":["manual","prio::low"],"milestone":{"title":"Legacy Epic"}}]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[{"name":"manual"},{"name":"team-a"},{"name":"prio::high"},{"name":"team-b"},{"name":"prio::low"}]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[{"id":55,"title":"Existing Epic","description":"<!-- jira:EPIC-2 -->"}]'
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

case "$*" in
  *"/rest/api/2/search?"*"issuetype%20%3D%20Story"* )
    printf '%s\n' '{"issues":[{"key":"PROJ-1","fields":{"summary":"Regression coverage","description":"h2. Summary","status":{"name":"To Do"},"priority":{"name":"high"},"labels":["team-a"],"customfield_10000":"EPIC-2","subtasks":[]}},{"key":"PROJ-2","fields":{"summary":"Brand new story","description":"h2. Fresh","status":{"name":"To Do"},"priority":{"name":"low"},"labels":["team-b"],"customfield_10000":"","subtasks":[]}}],"total":2}'
    ;;
  *"/rest/api/2/search?"*"issuetype%20%3D%20Epic"* )
    printf '%s\n' '{"issues":[{"key":"EPIC-2","fields":{"summary":"Existing Epic","description":"h2. Updated"}}],"total":1}'
    ;;
  *)
    printf 'unexpected curl invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" --dev --dry-run 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"1 milestones to update"* || "$output" != *"Existing Epic"* || "$output" != *"(description)"* || "$output" != *"description: changed"* || "$output" != *"1 issues to create:"* || "$output" != *"[PROJ-2] Brand new story"* || "$output" != *"1 issue updates planned:"* || "$output" != *"(title, description, labels, milestone)"* || "$output" != *"1 to create"* || "$output" != *"1 to update"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_sync_stories_dry_run_ignored_suspicious_epic_smoke() {
  local name="sync-stories-dry-run-ignored-suspicious-epic-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
~ Preview story sync from Jira
EOF

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
    printf '%s\n' '[{"iid":42,"title":"[PROJ-1] Regression coverage","description":"## Summary","labels":["team-a","prio::high"],"milestone":{"title":"Tests in der CI/CD Pipeline"}}]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[{"name":"team-a"},{"name":"prio::high"}]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[{"id":55,"title":"Tests in der CI/CD Pipeline","description":"<!-- jira:EPIC-OLD -->"},{"id":56,"title":"Code Quality und Security Checks","description":"## Scope\n\n<!-- jira:EPIC-NEW -->"}]'
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

case "$*" in
  *"/rest/api/2/search?"*"issuetype%20%3D%20Story"* )
    printf '%s\n' '{"issues":[{"key":"PROJ-1","fields":{"summary":"Regression coverage","description":"h2. Summary","status":{"name":"To Do"},"priority":{"name":"high"},"labels":["team-a"],"customfield_10000":"EPIC-NEW","subtasks":[]}}],"total":1}'
    ;;
  *"/rest/api/2/search?"*"issuetype%20%3D%20Epic"* )
    printf '%s\n' '{"issues":[{"key":"EPIC-OLD","fields":{"summary":".","description":"h2. Retired"}},{"key":"EPIC-NEW","fields":{"summary":"Code Quality und Security Checks","description":"h2. Scope"}}],"total":2}'
    ;;
  *)
    printf 'unexpected curl invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" --dev --dry-run 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"1 suspicious Jira epics ignored:"* || "$output" != *"[EPIC-OLD] ."* || "$output" != *"milestone: Tests in der CI/CD Pipeline -> Code Quality und Security Checks"* || "$output" == *". (title"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_sync_stories_dry_run_trimmed_titles_noop_smoke() {
  local name="sync-stories-dry-run-trimmed-titles-noop-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
~ Preview story sync from Jira
EOF

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
    printf '%s\n' '[{"iid":42,"title":"[PROJ-1] Regression coverage","description":"## Summary","labels":["team-a","prio::high"],"milestone":{"title":"Existing Epic"}}]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[{"name":"team-a"},{"name":"prio::high"}]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[{"id":55,"title":"Existing Epic","description":"## Scope\n\n<!-- jira:EPIC-2 -->"}]'
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

case "$*" in
  *"/rest/api/2/search?"*"issuetype%20%3D%20Story"* )
    printf '%s\n' '{"issues":[{"key":"PROJ-1","fields":{"summary":"Regression coverage   ","description":"h2. Summary","status":{"name":"To Do"},"priority":{"name":"high"},"labels":["team-a"],"customfield_10000":"EPIC-2","subtasks":[]}}],"total":1}'
    ;;
  *"/rest/api/2/search?"*"issuetype%20%3D%20Epic"* )
    printf '%s\n' '{"issues":[{"key":"EPIC-2","fields":{"summary":"Existing Epic   ","description":"h2. Scope"}}],"total":1}'
    ;;
  *)
    printf 'unexpected curl invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" --dev --dry-run 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"No issue updates required."* || "$output" != *"1 synced issues already up-to-date"* || "$output" == *"(title)"* || "$output" == *"milestones to update"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_sync_stories_dry_run_status_forward_smoke() {
  local name="sync-stories-dry-run-status-forward-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
~ Preview story sync from Jira
EOF

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
    printf '%s\n' '[{"iid":42,"state":"opened","title":"[PROJ-1] Regression coverage","description":"## Summary","labels":["manual","team-a","prio::high"],"milestone":null}]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[{"name":"manual"},{"name":"team-a"},{"name":"prio::high"},{"name":"status::in-progress"}]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[]'
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

case "$*" in
  *"/rest/api/2/search?"*"issuetype%20%3D%20Story"* )
    printf '%s\n' '{"issues":[{"key":"PROJ-1","fields":{"summary":"Regression coverage","description":"h2. Summary","status":{"name":"In Progress","statusCategory":{"key":"indeterminate"}},"priority":{"name":"high"},"labels":["team-a"],"customfield_10000":"","subtasks":[]}}],"total":1}'
    ;;
  *"/rest/api/2/search?"*"issuetype%20%3D%20Epic"* )
    printf '%s\n' '{"issues":[],"total":0}'
    ;;
  *)
    printf 'unexpected curl invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" --dev --dry-run 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"(status)"* || "$output" != *"status: open -> in-progress"* || "$output" == *"labels:"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_sync_stories_dry_run_status_no_regression_smoke() {
  local name="sync-stories-dry-run-status-no-regression-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
~ Preview story sync from Jira
EOF

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
    printf '%s\n' '[{"iid":42,"state":"opened","title":"[PROJ-1] Regression coverage","description":"## Summary","labels":["manual","team-a","prio::high","status::review"],"milestone":null}]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[{"name":"manual"},{"name":"team-a"},{"name":"prio::high"},{"name":"status::review"}]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[]'
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

case "$*" in
  *"/rest/api/2/search?"*"issuetype%20%3D%20Story"* )
    printf '%s\n' '{"issues":[{"key":"PROJ-1","fields":{"summary":"Regression coverage","description":"h2. Summary","status":{"name":"In Progress","statusCategory":{"key":"indeterminate"}},"priority":{"name":"high"},"labels":["team-a"],"customfield_10000":"","subtasks":[]}}],"total":1}'
    ;;
  *"/rest/api/2/search?"*"issuetype%20%3D%20Epic"* )
    printf '%s\n' '{"issues":[],"total":0}'
    ;;
  *)
    printf 'unexpected curl invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" --dev --dry-run 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"No issue updates required."* || "$output" != *"1 synced issues already up-to-date"* || "$output" == *"status:"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_sync_stories_create_done_smoke() {
  local name="sync-stories-create-done-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
~ Sync Jira
EOF

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
    printf '%s\n' '[]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[{"name":"team-a"},{"name":"prio::high"}]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[]'
    ;;
  "api projects/1/issues -X POST -f title=[PROJ-1] Done story -f description=## Summary -f labels=team-a,prio::high")
    printf '%s\n' '{"iid":42}'
    ;;
  "api projects/1/issues/42 -X PUT -f state_event=close")
    printf '%s\n' '{}'
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

case "$*" in
  *"/rest/api/2/search?"*"issuetype%20%3D%20Story"* )
    printf '%s\n' '{"issues":[{"key":"PROJ-1","fields":{"summary":"Done story","description":"h2. Summary","status":{"name":"Done","statusCategory":{"key":"done"}},"priority":{"name":"high"},"labels":["team-a"],"customfield_10000":"","subtasks":[]}}],"total":1}'
    ;;
  *"/rest/api/2/search?"*"issuetype%20%3D%20Epic"* )
    printf '%s\n' '{"issues":[],"total":0}'
    ;;
  *)
    printf 'unexpected curl invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" <<< $'y\nn\n' 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"(status: done)"* || "$output" != *"[PROJ-1] Done story"* || "$output" != *"(closed)"* || "$output" != *"1 created"* || "$output" != *"Skipping snapshot prompt: no existing issues or milestones to back up"* || "$output" == *"Create local snapshot first?"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_sync_stories_update_status_forward_smoke() {
  local name="sync-stories-update-status-forward-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
~ Sync Jira
EOF

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
    printf '%s\n' '[{"iid":42,"state":"opened","title":"[PROJ-1] Regression coverage","description":"## Summary","labels":["manual","team-a","prio::high"],"milestone":null}]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[{"name":"manual"},{"name":"team-a"},{"name":"prio::high"},{"name":"status::in-progress"}]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[]'
    ;;
  api\ projects/1/issues/42\ -X\ PUT*)
    case "$*" in
      *"labels=manual,team-a,prio::high,status::in-progress"*)
        if [[ "$*" == *"title="* || "$*" == *"description="* || "$*" == *"milestone_id="* || "$*" == *"state_event="* ]]; then
          printf 'unexpected extra fields in status-forward payload: %s\n' "$*" >&2
          exit 1
        fi
        printf '%s\n' '{}'
        ;;
      *)
        printf 'unexpected update payload: %s\n' "$*" >&2
        exit 1
        ;;
    esac
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

case "$*" in
  *"/rest/api/2/search?"*"issuetype%20%3D%20Story"* )
    printf '%s\n' '{"issues":[{"key":"PROJ-1","fields":{"summary":"Regression coverage","description":"h2. Summary","status":{"name":"In Progress","statusCategory":{"key":"indeterminate"}},"priority":{"name":"high"},"labels":["team-a"],"customfield_10000":"","subtasks":[]}}],"total":1}'
    ;;
  *"/rest/api/2/search?"*"issuetype%20%3D%20Epic"* )
    printf '%s\n' '{"issues":[],"total":0}'
    ;;
  *)
    printf 'unexpected curl invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" <<< $'y\nn\n' 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"(status)"* || "$output" != *"status: open -> in-progress"* || "$output" != *"1 updated"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_sync_stories_update_status_done_smoke() {
  local name="sync-stories-update-status-done-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
~ Sync Jira
EOF

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
    printf '%s\n' '[{"iid":42,"state":"opened","title":"[PROJ-1] Regression coverage","description":"## Summary","labels":["manual","team-a","prio::high","status::review"],"milestone":null}]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[{"name":"manual"},{"name":"team-a"},{"name":"prio::high"},{"name":"status::review"}]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[]'
    ;;
  api\ projects/1/issues/42\ -X\ PUT*)
    case "$*" in
      *"labels=manual,team-a,prio::high"*\
*"state_event=close"*)
        if [[ "$*" == *"title="* || "$*" == *"description="* || "$*" == *"milestone_id="* ]]; then
          printf 'unexpected extra fields in status-done payload: %s\n' "$*" >&2
          exit 1
        fi
        printf '%s\n' '{}'
        ;;
      *)
        printf 'unexpected update payload: %s\n' "$*" >&2
        exit 1
        ;;
    esac
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

case "$*" in
  *"/rest/api/2/search?"*"issuetype%20%3D%20Story"* )
    printf '%s\n' '{"issues":[{"key":"PROJ-1","fields":{"summary":"Regression coverage","description":"h2. Summary","status":{"name":"Done","statusCategory":{"key":"done"}},"priority":{"name":"high"},"labels":["team-a"],"customfield_10000":"","subtasks":[]}}],"total":1}'
    ;;
  *"/rest/api/2/search?"*"issuetype%20%3D%20Epic"* )
    printf '%s\n' '{"issues":[],"total":0}'
    ;;
  *)
    printf 'unexpected curl invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" <<< $'y\nn\n' 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"(status)"* || "$output" != *"status: review -> done"* || "$output" != *"1 updated"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_sync_stories_update_milestone_description_only_smoke() {
  local name="sync-stories-update-milestone-description-only-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
~ Sync Jira
EOF

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
    printf '%s\n' '[{"iid":42,"title":"[PROJ-1] Regression coverage","description":"## Summary","labels":["team-a","prio::high"],"milestone":{"title":"Existing Epic"}}]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[{"name":"team-a"},{"name":"prio::high"}]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[{"id":55,"title":"Existing Epic","description":"<!-- jira:EPIC-2 -->"}]'
    ;;
  api\ projects/1/milestones/55\ -X\ PUT*)
    case "$*" in
      *"description=## Updated"$'\n\n'"<!-- jira:EPIC-2 -->"*)
        if [[ "$*" == *"title="* ]]; then
          printf 'unexpected title field in milestone update payload: %s\n' "$*" >&2
          exit 1
        fi
        printf '%s\n' '{}'
        ;;
      *)
        printf 'unexpected milestone update payload: %s\n' "$*" >&2
        exit 1
        ;;
    esac
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

case "$*" in
  *"/rest/api/2/search?"*"issuetype%20%3D%20Story"* )
    printf '%s\n' '{"issues":[{"key":"PROJ-1","fields":{"summary":"Regression coverage","description":"h2. Summary","status":{"name":"To Do"},"priority":{"name":"high"},"labels":["team-a"],"customfield_10000":"EPIC-2","subtasks":[]}}],"total":1}'
    ;;
  *"/rest/api/2/search?"*"issuetype%20%3D%20Epic"* )
    printf '%s\n' '{"issues":[{"key":"EPIC-2","fields":{"summary":"Existing Epic","description":"h2. Updated"}}],"total":1}'
    ;;
  *)
    printf 'unexpected curl invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" <<< $'y\nn\n' 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"Existing Epic"* || "$output" != *"(description)"* || "$output" != *"description: changed"* || "$output" != *"1 synced issues already up-to-date"* || "$output" != *"Milestones:"* || "$output" != *"1 updated"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_sync_stories_update_title_only_smoke() {
  local name="sync-stories-update-title-only-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
~ Sync Jira
EOF

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
    printf '%s\n' '[{"iid":42,"title":"[PROJ-1] Old summary","description":"## Summary","labels":["manual","team-a","prio::high"],"milestone":null}]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[{"name":"manual"},{"name":"team-a"},{"name":"prio::high"}]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[]'
    ;;
  api\ projects/1/issues/42\ -X\ PUT*)
    case "$*" in
      *"title=[PROJ-1] Regression coverage"*)
        if [[ "$*" == *"description="* || "$*" == *"labels="* || "$*" == *"milestone_id="* ]]; then
          printf 'unexpected extra fields in update payload: %s\n' "$*" >&2
          exit 1
        fi
        printf '%s\n' '{}'
        ;;
      *)
        printf 'unexpected update payload: %s\n' "$*" >&2
        exit 1
        ;;
    esac
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

case "$*" in
  *"/rest/api/2/search?"*"issuetype%20%3D%20Story"* )
    printf '%s\n' '{"issues":[{"key":"PROJ-1","fields":{"summary":"Regression coverage","description":"h2. Summary","status":{"name":"To Do"},"priority":{"name":"high"},"labels":["team-a"],"customfield_10000":"","subtasks":[]}}],"total":1}'
    ;;
  *"/rest/api/2/search?"*"issuetype%20%3D%20Epic"* )
    printf '%s\n' '{"issues":[],"total":0}'
    ;;
  *)
    printf 'unexpected curl invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" <<< $'y\nn\n' 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"(title)"* || "$output" != *"title: [PROJ-1] Old summary -> [PROJ-1] Regression coverage"* || "$output" != *"1 updated"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_sync_stories_update_smoke() {
  local name="sync-stories-update-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
~ Sync Jira
EOF

  cat >"$stubdir/clear" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

  cat >"$stubdir/fzf" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
responses_file="${FZF_RESPONSES_FILE:?}"
if [[ ! -s "$responses_file" ]]; then
  exit 1
fi
response="$(head -n 1 "$responses_file")"
tail -n +2 "$responses_file" >"${responses_file}.tmp"
mv "${responses_file}.tmp" "$responses_file"
printf '%s\n' "$response"
EOF

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
    printf '%s\n' '[{"iid":42,"title":"[PROJ-1] Old summary","description":"Old description","labels":["manual","prio::low"],"milestone":{"title":"Legacy Epic"}}]'
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[{"name":"manual"},{"name":"team-a"},{"name":"prio::high"}]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[]'
    ;;
  api\ projects/1/issues/42\ -X\ PUT*)
    case "$*" in
      *"title=[PROJ-1] Regression coverage"*\
*"description=## Summary"*\
*"labels=manual,team-a,prio::high"*\
*"milestone_id=0"*)
        printf '%s\n' '{}'
        ;;
      *)
        printf 'unexpected update payload: %s\n' "$*" >&2
        exit 1
        ;;
    esac
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

case "$*" in
  *"/rest/api/2/search?"*"issuetype%20%3D%20Story"* )
    printf '%s\n' '{"issues":[{"key":"PROJ-1","fields":{"summary":"Regression coverage","description":"h2. Summary","status":{"name":"To Do"},"priority":{"name":"high"},"labels":["team-a"],"customfield_10000":"","subtasks":[]}}],"total":1}'
    ;;
  *"/rest/api/2/search?"*"issuetype%20%3D%20Epic"* )
    printf '%s\n' '{"issues":[],"total":0}'
    ;;
  *)
    printf 'unexpected curl invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" <<< $'y\nn\n' 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" == *"s_key="* || "$output" == *"cmd=("*
  ]]; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"Checking for updates..."* || "$output" != *"[PROJ-1] Regression coverage"* || "$output" != *"1 updated"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_sync_epics_smoke() {
  local name="sync-epics-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
Sync epics from Jira
EOF

  write_clear_stub "$stubdir"
  write_fzf_stub "$stubdir"

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

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
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[{"id":55,"title":"Existing Epic","description":"<!-- jira:EPIC-2 -->"}]'
    ;;
  "api projects/1/milestones -X POST -f title=New Epic -f description=## Scope"$'\n\n'"<!-- jira:EPIC-1 -->")
    printf '%s\n' '{}'
    ;;
  "api projects/1/milestones/55 -X PUT -f title=Existing Epic -f description=## Updated"$'\n\n'"<!-- jira:EPIC-2 -->")
    printf '%s\n' '{}'
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

case "$*" in
  *"/rest/api/2/search?"*"issuetype%20%3D%20Epic"* )
    printf '%s\n' '{"issues":[{"key":"EPIC-1","fields":{"summary":"New Epic","description":"h2. Scope"}},{"key":"EPIC-2","fields":{"summary":"Existing Epic","description":"h2. Updated"}}],"total":2}'
    ;;
  *)
    printf 'unexpected curl invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/glab" "$stubdir/curl"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" --dev <<< $'y\n' 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"1 milestones to create"* || "$output" != *"1 milestones to update"* || "$output" != *"New Epic"* || "$output" != *"Existing Epic"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_manual_create_with_branch_smoke() {
  local name="manual-create-with-branch-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
Create issue
Skip (no labels)
Skip (no milestone)
main (default)
EOF

  write_clear_stub "$stubdir"
  write_fzf_stub "$stubdir"
  write_nvim_replace_stub "$stubdir"

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

case "$*" in
  "repo view --output json")
    printf '%s\n' '{"path_with_namespace":"group/project","id":1}'
    ;;
  api\ projects/ibm%2Fglab-helper/variables/*)
    exit 1
    ;;
  "api projects/1/labels?per_page=100&page=1")
    printf '%s\n' '[]'
    ;;
  "api projects/1/members/all?per_page=100&page=1")
    printf '%s\n' '[]'
    ;;
  "api projects/1/milestones?state=active&per_page=100&page=1")
    printf '%s\n' '[]'
    ;;
  issue\ create\ -t\ Manual\ issue\ title\ -d*)
    printf '%s\n' 'https://gitlab.example.com/group/project/-/issues/42'
    ;;
  *)
    printf 'unexpected glab invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  cat >"$stubdir/git" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

case "$*" in
  "show-ref --verify --quiet refs/heads/42-manual-issue-title")
    exit 1
    ;;
  "fetch --prune origin --quiet")
    exit 0
    ;;
  "symbolic-ref refs/remotes/origin/HEAD")
    printf '%s\n' 'refs/remotes/origin/main'
    ;;
  for-each-ref\ --sort=-committerdate\ --format=%\(refname:short\)\ refs/remotes/origin/)
    printf '%s\n' 'origin/main'
    printf '%s\n' 'origin/dev'
    ;;
  "show-ref --verify --quiet refs/remotes/origin/main")
    exit 0
    ;;
  "branch 42-manual-issue-title origin/main")
    exit 0
    ;;
  "checkout 42-manual-issue-title")
    exit 0
    ;;
  *)
    printf 'unexpected git invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/nvim" "$stubdir/glab" "$stubdir/git"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    STUB_EDITOR_CONTENT=$'## Ready for implementation\n\n- [ ] first check' \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" --dev <<< $'Manual issue title\n\ny\ny\n\ny\n' 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"Issue created successfully"* || "$output" != *"42-manual-issue-title"* || "$output" != *"created from"* || "$output" != *"Switched to"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_work_issue_edit_description_smoke() {
  local name="work-issue-edit-description-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
  mkdir -p "$stubdir"

  trap 'rm -rf "$tmpdir"' RETURN

  cat >"$responses_file" <<'EOF'
Work on existing issue
#7
Edit description
EOF

  write_clear_stub "$stubdir"
  write_fzf_stub "$stubdir"
  write_nvim_replace_stub "$stubdir"

  cat >"$stubdir/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

case "$*" in
  "repo view --output json")
    printf '%s\n' '{"path_with_namespace":"group/project","id":1}'
    ;;
  api\ projects/ibm%2Fglab-helper/variables/*)
    exit 1
    ;;
  "api projects/1/issues?state=opened&per_page=100&page=1")
    printf '%s\n' '[{"iid":7,"title":"Existing issue","description":"Old description","labels":[],"assignees":[],"milestone":null}]'
    ;;
  "issue view 7 --output json")
    printf '%s\n' '{"description":"Old description"}'
    ;;
  issue\ update\ 7\ -d*)
    printf '%s\n' '{}'
    ;;
  *)
    printf 'unexpected glab invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  cat >"$stubdir/git" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

case "$*" in
  "fetch --prune origin --quiet")
    exit 0
    ;;
  "branch -r")
    exit 0
    ;;
  "branch")
    exit 0
    ;;
  *)
    printf 'unexpected git invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "$stubdir/clear" "$stubdir/fzf" "$stubdir/nvim" "$stubdir/glab" "$stubdir/git"

  if ! output="$(
    PATH="$stubdir:$PATH" \
    TERM=xterm \
    STUB_EDITOR_CONTENT=$'## Updated description' \
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"Description updated"* || "$output" != *"Done."* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

source "$ROOT_DIR/tests/p0_safety.sh"

run_check "zsh-syntax" zsh -n "$ROOT_DIR/src/glab-helper" "$ROOT_DIR"/src/lib/*.zsh "$ROOT_DIR"/src/flows/*.zsh
run_check "bash-syntax" bash -n "$ROOT_DIR/install.sh"

if command -v shellcheck >/dev/null 2>&1; then
  run_check "shellcheck-install" shellcheck -f gcc "$ROOT_DIR/install.sh"
else
  printf 'SKIP shellcheck-install (shellcheck not installed)\n'
fi

run_help_check "help-smoke" "$ROOT_DIR/src/glab-helper" --help
run_help_check "dev-help-smoke" "$ROOT_DIR/src/glab-helper" --dev --help
run_help_check "dev-dry-run-help-smoke" "$ROOT_DIR/src/glab-helper" --dev --dry-run --help
run_main_and_dev_menu_smoke
run_dry_run_menu_read_only_smoke
run_jira_flow_smoke
run_sync_epics_smoke
run_snapshot_export_smoke
run_sync_stories_dry_run_smoke
run_sync_stories_dry_run_noop_smoke
run_sync_stories_dry_run_mixed_plan_smoke
run_sync_stories_dry_run_ignored_suspicious_epic_smoke
run_sync_stories_dry_run_trimmed_titles_noop_smoke
run_sync_stories_dry_run_status_forward_smoke
run_sync_stories_dry_run_status_no_regression_smoke
run_sync_stories_create_done_smoke
run_sync_stories_update_status_forward_smoke
run_sync_stories_update_status_done_smoke
run_sync_stories_update_milestone_description_only_smoke
run_sync_stories_update_smoke
run_sync_stories_update_title_only_smoke
run_manual_create_with_branch_smoke
run_work_issue_edit_description_smoke
run_offline_cli_validation_smoke
run_jira_pagination_contract_smoke
run_gitlab_read_failure_smoke
run_gitlab_http_status_smoke
run_gitlab_native_pagination_smoke
run_epic_failure_blocks_story_sync_smoke
run_dry_run_write_barrier_smoke
run_partial_failure_exit_code_smoke
run_snapshot_read_failure_smoke

printf 'All tests passed.\n'
