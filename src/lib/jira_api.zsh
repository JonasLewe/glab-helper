# Jira configuration and fetch helpers.

JIRA_PROJECT_PATH="${GLAB_HELPER_JIRA_PROJECT_PATH:-ibm%2Fglab-helper}"
JIRA_AVAILABLE=false

load_jira_config() {
  local var_url var_labels var_token

  if ! var_url=$(glab api "projects/${JIRA_PROJECT_PATH}/variables/JIRA_URL" 2>/dev/null); then
    return 1
  fi

  if ! var_labels=$(glab api "projects/${JIRA_PROJECT_PATH}/variables/JIRA_BOARD_LABELS" 2>/dev/null); then
    return 1
  fi

  if ! var_token=$(glab api "projects/${JIRA_PROJECT_PATH}/variables/JIRA_TOKEN" 2>/dev/null); then
    return 1
  fi

  JIRA_URL=$(jq -r '.value' <<< "$var_url")
  JIRA_BOARD_LABELS=$(jq -r '.value' <<< "$var_labels")
  JIRA_TOKEN=$(jq -r '.value' <<< "$var_token")

  if [[ -z "$JIRA_URL" || -z "$JIRA_BOARD_LABELS" || -z "$JIRA_TOKEN" ]]; then
    return 1
  fi

  local var_target
  if ! var_target=$(glab api "projects/${JIRA_PROJECT_PATH}/variables/JIRA_TARGET_PROJECT" 2>/dev/null); then
    return 1
  fi
  JIRA_TARGET_PROJECT=$(jq -r '.value' <<< "$var_target")

  if ! $DEV_MODE && [[ "$repo_name" != "$JIRA_TARGET_PROJECT" ]]; then
    return 1
  fi

  return 0
}

fetch_jira_stories() {
  local labels_jql stories_response

  labels_jql="labels in (${JIRA_BOARD_LABELS}) AND issuetype = Story"

  if ! stories_response=$(curl -fsS \
    -H "Authorization: Bearer ${JIRA_TOKEN}" \
    "${JIRA_URL}/rest/api/2/search?jql=$(printf '%s' "$labels_jql" | jq -sRr @uri)&maxResults=100&fields=key,summary,description,status,priority,labels,issuetype,customfield_10000,subtasks"); then
    return 1
  fi

  if ! jq -e '.issues' <<< "$stories_response" &>/dev/null; then
    return 1
  fi

  JIRA_STORIES_JSON=$(jq '.issues' <<< "$stories_response")
  JIRA_STORIES_COUNT=$(jq 'length' <<< "$JIRA_STORIES_JSON")
  return 0
}

fetch_jira_epics() {
  local labels_jql start_at=0 max_results=100 total=-1
  local page_response all_issues="[]"

  labels_jql="labels in (${JIRA_BOARD_LABELS}) AND issuetype = Epic"

  while true; do
    if ! page_response=$(curl -fsS \
      -H "Authorization: Bearer ${JIRA_TOKEN}" \
      "${JIRA_URL}/rest/api/2/search?jql=$(printf '%s' "$labels_jql" | jq -sRr @uri)&maxResults=${max_results}&startAt=${start_at}&fields=key,summary,status,description"); then
      return 1
    fi

    if ! jq -e '.issues' <<< "$page_response" &>/dev/null; then
      return 1
    fi

    all_issues=$(jq -s '.[0] + .[1]' <<< "${all_issues}"$'\n'"$(jq '.issues' <<< "$page_response")")
    total=$(jq -r '.total' <<< "$page_response")
    start_at=$((start_at + max_results))

    [[ $start_at -ge $total ]] && break
  done

  JIRA_EPICS_JSON="$all_issues"
  JIRA_EPICS_COUNT=$(jq 'length' <<< "$JIRA_EPICS_JSON")
  return 0
}

fetch_jira_stories_paginated() {
  local labels_jql start_at=0 max_results=100 total=-1
  local page_response all_issues="[]"

  labels_jql="labels in (${JIRA_BOARD_LABELS}) AND issuetype = Story"

  while true; do
    if ! page_response=$(curl -fsS \
      -H "Authorization: Bearer ${JIRA_TOKEN}" \
      "${JIRA_URL}/rest/api/2/search?jql=$(printf '%s' "$labels_jql" | jq -sRr @uri)&maxResults=${max_results}&startAt=${start_at}&fields=key,summary,description,status,priority,labels,issuetype,customfield_10000,subtasks"); then
      return 1
    fi

    if ! jq -e '.issues' <<< "$page_response" &>/dev/null; then
      return 1
    fi

    all_issues=$(jq -s '.[0] + .[1]' <<< "${all_issues}"$'\n'"$(jq '.issues' <<< "$page_response")")
    total=$(jq -r '.total' <<< "$page_response")
    start_at=$((start_at + max_results))

    [[ $start_at -ge $total ]] && break
  done

  JIRA_STORIES_JSON="$all_issues"
  JIRA_STORIES_COUNT=$(jq 'length' <<< "$JIRA_STORIES_JSON")
  return 0
}
