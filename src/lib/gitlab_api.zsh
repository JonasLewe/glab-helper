# GitLab data access helpers.

get_default_branch() {
  local branch project_json
  branch=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@')
  if [[ -z "$branch" ]]; then
    if ! project_json=$(retry_read_json \
      "GitLab project $project_id" \
      'type == "object" and (.default_branch == null or (.default_branch | type == "string"))' \
      glab api "projects/$project_id"); then
      return 1
    fi
    branch=$(jq -r '.default_branch // empty' <<< "$project_json")
  fi
  printf '%s\n' "$branch"
}

fetch_gitlab_array_pages() {
  local resource="$1" endpoint="$2"
  local page=1 per_page=100 all="[]" page_json count separator pages_json
  separator="?"
  [[ "$endpoint" == *"?"* ]] && separator="&"

  if [[ "${GLAB_HELPER_PAGINATION_MODE:-paginate}" != "manual" ]]; then
    if ! pages_json=$(retry_read_json \
      "$resource pagination" \
      'if type == "array" then true else error("expected an array page") end' \
      glab api --paginate "${endpoint}${separator}per_page=${per_page}"); then
      return 1
    fi
    if ! all=$(jq -e -s \
      'if length > 0 and all(.[]; type == "array") then add else error("invalid paginated response") end' \
      <<< "$pages_json"); then
      api_error "$resource" "pagination" "could not combine response pages"
      return 1
    fi
    printf '%s\n' "$all"
    return 0
  fi

  while true; do
    if ! page_json=$(retry_read_json \
      "$resource page $page" \
      'type == "array"' \
      glab api "${endpoint}${separator}per_page=${per_page}&page=${page}"); then
      return 1
    fi
    count=$(jq 'length' <<< "$page_json")
    if ! all=$(jq -e -s 'if all(.[]; type == "array") then add else error("non-array page") end' \
      <<< "${all}"$'\n'"${page_json}"); then
      api_error "$resource" "pagination" "could not combine response pages"
      return 1
    fi
    [[ "$count" -lt "$per_page" ]] && break
    ((page++))
  done

  printf '%s\n' "$all"
}

fetch_all_milestones() {
  local state="active"
  [[ $# -gt 0 ]] && state="$1"
  local endpoint="projects/$project_id/milestones"
  [[ -n "$state" ]] && endpoint="${endpoint}?state=${state}"
  fetch_gitlab_array_pages "GitLab milestones" "$endpoint"
}

fetch_all_issues() {
  local state="${1:-}"
  local endpoint="projects/$project_id/issues"
  [[ -n "$state" ]] && endpoint="${endpoint}?state=${state}"
  fetch_gitlab_array_pages "GitLab issues" "$endpoint"
}

fetch_all_labels() {
  fetch_gitlab_array_pages "GitLab labels" "projects/$project_id/labels"
}

fetch_all_members() {
  fetch_gitlab_array_pages "GitLab project members" "projects/$project_id/members/all"
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

  echo -n "  ${DIM}Reading GitLab snapshot data...${RESET}"

  if ! issues_json=$(fetch_all_issues) \
    || ! milestones_json=$(fetch_all_milestones "") \
    || ! labels_json=$(fetch_all_labels); then
    printf "\r                                      \r"
    echo "  ${RED}${ICON_WARN}${RESET} Snapshot export aborted because GitLab data is incomplete."
    echo ""
    return 1
  fi

  if ! require_writes_allowed "snapshot export"; then
    return 1
  fi

  if ! mkdir -p "$snapshot_dir"; then
    printf "\r                                      \r"
    echo "  ${RED}${ICON_WARN}${RESET} Could not create snapshot directory."
    echo ""
    return 1
  fi

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
