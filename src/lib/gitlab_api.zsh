# GitLab data access helpers.

get_default_branch() {
  local branch
  branch=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@')
  if [[ -z "$branch" ]]; then
    branch=$(glab api "projects/$project_id" 2>/dev/null | jq -r '.default_branch // empty')
  fi
  echo "$branch"
}

fetch_all_milestones() {
  local state="active" page=1 per_page=100 all="[]" page_json count
  if [[ $# -gt 0 ]]; then
    state="$1"
  fi
  local state_filter=""
  [[ -n "$state" ]] && state_filter="state=${state}&"
  while true; do
    page_json=$(safe_json "$(glab api "projects/$project_id/milestones?${state_filter}per_page=${per_page}&page=${page}" 2>/dev/null)" "[]")
    count=$(jq 'length' <<< "$page_json")
    all=$(jq -s '.[0] + .[1]' <<< "${all}"$'\n'"${page_json}")
    [[ "$count" -lt "$per_page" ]] && break
    ((page++))
  done
  printf '%s\n' "$all"
}

fetch_all_issues() {
  local state="${1:-}" page=1 per_page=100 all="[]" page_json count
  local state_filter=""
  [[ -n "$state" ]] && state_filter="state=${state}&"
  while true; do
    page_json=$(safe_json "$(glab api "projects/$project_id/issues?${state_filter}per_page=${per_page}&page=${page}" 2>/dev/null)" "[]")
    count=$(jq 'length' <<< "$page_json")
    all=$(jq -s '.[0] + .[1]' <<< "${all}"$'\n'"${page_json}")
    [[ "$count" -lt "$per_page" ]] && break
    ((page++))
  done
  printf '%s\n' "$all"
}

fetch_all_labels() {
  local page=1 per_page=100 page_json count all="[]"

  while true; do
    page_json=$(safe_json "$(glab api "projects/$project_id/labels?per_page=${per_page}&page=${page}" 2>/dev/null)" "[]")
    count=$(jq 'if type == "array" then length else 0 end' <<< "$page_json")
    all=$(jq -s '.[0] + .[1]' <<< "${all}"$'\n'"${page_json}")
    [[ "$count" -lt "$per_page" ]] && break
    ((page++))
  done

  printf '%s\n' "$all"
}

fetch_all_members() {
  local page=1 per_page=100 page_json count all="[]"

  while true; do
    page_json=$(safe_json "$(glab api "projects/$project_id/members/all?per_page=${per_page}&page=${page}" 2>/dev/null)" "[]")
    count=$(jq 'if type == "array" then length else 0 end' <<< "$page_json")
    all=$(jq -s '.[0] + .[1]' <<< "${all}"$'\n'"${page_json}")
    [[ "$count" -lt "$per_page" ]] && break
    ((page++))
  done

  printf '%s\n' "$all"
}

export_gitlab_snapshot() {
  local label="${1:-manual}"
  local snapshot_root="$PWD/.glab-helper-snapshots"
  local snapshot_dir snapshot_slug timestamp created_at
  local issues_json milestones_json labels_json

  timestamp=$(date +%Y%m%d-%H%M%S)
  created_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  snapshot_slug=$(slugify "$label")
  snapshot_dir="${snapshot_root}/${timestamp}-${snapshot_slug}"

  echo -n "  ${DIM}Exporting GitLab snapshot...${RESET}"

  if ! mkdir -p "$snapshot_dir"; then
    printf "\r                                      \r"
    echo "  ${RED}${ICON_WARN}${RESET} Could not create snapshot directory."
    echo ""
    return 1
  fi

  issues_json=$(fetch_all_issues)
  milestones_json=$(fetch_all_milestones "")
  labels_json=$(fetch_all_labels)

  if ! printf '%s\n' "$issues_json" > "$snapshot_dir/issues.json" \
    || ! printf '%s\n' "$milestones_json" > "$snapshot_dir/milestones.json" \
    || ! printf '%s\n' "$labels_json" > "$snapshot_dir/labels.json" \
    || ! jq -n \
      --arg created_at "$created_at" \
      --arg repo_name "$repo_name" \
      --arg project_id "$project_id" \
      --arg label "$label" \
      '{created_at:$created_at, repo_name:$repo_name, project_id:$project_id, label:$label}' \
      > "$snapshot_dir/metadata.json"; then
    printf "\r                                      \r"
    echo "  ${RED}${ICON_WARN}${RESET} Failed to write snapshot files."
    echo ""
    return 1
  fi

  printf "\r                                      \r"
  echo "  ${GREEN}${ICON_OK}${RESET} Snapshot exported to ${DIM}${snapshot_dir}${RESET}"
  echo ""
  return 0
}
