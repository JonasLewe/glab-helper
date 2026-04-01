#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

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

  if "$@" >/tmp/glab-helper-test.out 2>&1; then
    pass "$name"
    return 0
  fi

  fail "$name" "$(cat /tmp/glab-helper-test.out)"
}

run_help_check() {
  local name="$1"
  shift
  local output

  if ! output="$("$@" 2>&1)"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"Usage: glab-helper [--dev] [--dry-run]"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_preview_menu_hidden_by_default_smoke() {
  local name="preview-menu-hidden-by-default-smoke"
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
    "$ROOT_DIR/src/glab-helper" 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ ! -f "$menu_capture" || "$(cat "$menu_capture")" == *"Preview story sync from Jira"* || "$(cat "$menu_capture")" != *"Sync stories from Jira"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_preview_menu_visible_with_dry_run_smoke() {
  local name="preview-menu-visible-with-dry-run-smoke"
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
    "$ROOT_DIR/src/glab-helper" --dev --dry-run 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ ! -f "$menu_capture" || "$(cat "$menu_capture")" != *"Preview story sync from Jira"* || "$(cat "$menu_capture")" != *"Sync stories from Jira"* ]]; then
    fail "$name" "$output"
  fi

  pass "$name"
}

run_jira_flow_smoke() {
  local name="jira-create-flow-smoke"
  local tmpdir stubdir responses_file output

  tmpdir="$(mktemp -d)"
  stubdir="$tmpdir/bin"
  responses_file="$tmpdir/fzf-responses"
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
    FZF_RESPONSES_FILE="$responses_file" \
    "$ROOT_DIR/src/glab-helper" <<< $'\nn\n' 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" == *"local: can only be used in a function"* ]]; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"Summary"* || "$output" != *"Epic Alpha"* || "$output" != *"Aborted."* ]]; then
    fail "$name" "$output"
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
    "$ROOT_DIR/src/glab-helper" --dev --dry-run 2>&1
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
    "$ROOT_DIR/src/glab-helper" 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"1 milestones to update"* || "$output" != *"Existing Epic"* || "$output" != *"(description)"* || "$output" != *"description: changed"* || "$output" != *"1 issues to create:"* || "$output" != *"[PROJ-2] Brand new story"* || "$output" != *"1 issue updates planned:"* || "$output" != *"(title, description, labels, milestone)"* || "$output" != *"1 to create"* || "$output" != *"1 to update"* ]]; then
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
~ Sync stories from Jira
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
~ Sync stories from Jira
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
~ Sync stories from Jira
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

run_check "zsh-syntax" zsh -n "$ROOT_DIR/src/glab-helper"
run_check "bash-syntax" bash -n "$ROOT_DIR/install.sh"

if command -v shellcheck >/dev/null 2>&1; then
  run_check "shellcheck-install" shellcheck -f gcc "$ROOT_DIR/install.sh"
else
  printf 'SKIP shellcheck-install (shellcheck not installed)\n'
fi

run_help_check "help-smoke" "$ROOT_DIR/src/glab-helper" --help
run_help_check "dev-help-smoke" "$ROOT_DIR/src/glab-helper" --dev --help
run_help_check "dev-dry-run-help-smoke" "$ROOT_DIR/src/glab-helper" --dev --dry-run --help
run_preview_menu_hidden_by_default_smoke
run_preview_menu_visible_with_dry_run_smoke
run_jira_flow_smoke
run_snapshot_export_smoke
run_sync_stories_dry_run_smoke
run_sync_stories_dry_run_noop_smoke
run_sync_stories_dry_run_mixed_plan_smoke
run_sync_stories_update_milestone_description_only_smoke
run_sync_stories_update_smoke
run_sync_stories_update_title_only_smoke

printf 'All tests passed.\n'
