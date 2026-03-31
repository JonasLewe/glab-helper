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
  local state="${1:-active}" page=1 per_page=100 all="[]" page_json count
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
