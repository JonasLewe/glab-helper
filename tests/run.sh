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

  if [[ "$output" != *"Usage: glab-helper [--dev]"* ]]; then
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
    "$ROOT_DIR/src/glab-helper" <<< $'y\n' 2>&1
  )"; then
    fail "$name" "$output"
  fi

  if [[ "$output" != *"1 milestones to create"* || "$output" != *"1 milestones to update"* || "$output" != *"New Epic"* || "$output" != *"Existing Epic"* ]]; then
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
    "$ROOT_DIR/src/glab-helper" <<< $'y\n' 2>&1
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
  "label list -P 100 --output json")
    printf '%s\n' '[]'
    ;;
  "api projects/1/members/all?per_page=100")
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
  "fetch origin --quiet")
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
    "$ROOT_DIR/src/glab-helper" <<< $'Manual issue title\n\ny\ny\n\ny\n' 2>&1
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
  "issue list --output json -P 100")
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
  "fetch origin --quiet")
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

run_check "zsh-syntax" zsh -n "$ROOT_DIR/src/glab-helper"
run_check "bash-syntax" bash -n "$ROOT_DIR/install.sh"

if command -v shellcheck >/dev/null 2>&1; then
  run_check "shellcheck-install" shellcheck -f gcc "$ROOT_DIR/install.sh"
else
  printf 'SKIP shellcheck-install (shellcheck not installed)\n'
fi

run_help_check "help-smoke" "$ROOT_DIR/src/glab-helper" --help
run_help_check "dev-help-smoke" "$ROOT_DIR/src/glab-helper" --dev --help
run_jira_flow_smoke
run_sync_epics_smoke
run_sync_stories_update_smoke
run_manual_create_with_branch_smoke
run_work_issue_edit_description_smoke

printf 'All tests passed.\n'
