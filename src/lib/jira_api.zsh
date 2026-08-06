# Jira configuration and fetch helpers.

JIRA_PROJECT_PATH="${GLAB_HELPER_JIRA_PROJECT_PATH:-}"
JIRA_AVAILABLE=false

load_jira_config() {
  local var_url var_labels var_token var_target
  local variable_schema='type == "object" and (.value | type == "string") and (.value | length > 0)'

  if [[ -z "$JIRA_PROJECT_PATH" ]]; then
    JIRA_PROJECT_PATH="${repo_name//\//%2F}"
  fi

  if ! var_url=$(glab api "projects/${JIRA_PROJECT_PATH}/variables/JIRA_URL" 2>/dev/null) \
    || ! jq -e "$variable_schema" <<< "$var_url" &>/dev/null; then
    return 1
  fi

  if ! var_labels=$(glab api "projects/${JIRA_PROJECT_PATH}/variables/JIRA_BOARD_LABELS" 2>/dev/null) \
    || ! jq -e "$variable_schema" <<< "$var_labels" &>/dev/null; then
    return 1
  fi

  if ! var_token=$(glab api "projects/${JIRA_PROJECT_PATH}/variables/JIRA_TOKEN" 2>/dev/null) \
    || ! jq -e "$variable_schema" <<< "$var_token" &>/dev/null; then
    return 1
  fi

  JIRA_URL=$(jq -r '.value' <<< "$var_url")
  JIRA_BOARD_LABELS=$(jq -r '.value' <<< "$var_labels")
  JIRA_TOKEN=$(jq -r '.value' <<< "$var_token")

  if [[ -z "$JIRA_URL" || -z "$JIRA_BOARD_LABELS" || -z "$JIRA_TOKEN" ]]; then
    return 1
  fi

  if ! var_target=$(glab api "projects/${JIRA_PROJECT_PATH}/variables/JIRA_TARGET_PROJECT" 2>/dev/null) \
    || ! jq -e "$variable_schema" <<< "$var_target" &>/dev/null; then
    return 1
  fi
  JIRA_TARGET_PROJECT=$(jq -r '.value' <<< "$var_target")

  if ! $DEV_MODE && [[ "$repo_name" != "$JIRA_TARGET_PROJECT" ]]; then
    return 1
  fi

  return 0
}

fetch_jira_stories() {
  fetch_jira_stories_paginated
}

fetch_jira_search_paginated() {
  local issue_type="$1" fields="$2"
  local labels_jql start_at=0 requested_max=100
  local page_response page_issues all_issues="[]"
  local response_start response_max page_count next_start total is_last
  local has_is_last has_response_max
  local schema='
    type == "object"
    and (.issues | type == "array")
    and ((has("startAt") | not) or ((.startAt | type == "number") and .startAt >= 0 and (.startAt | floor == .)))
    and ((has("maxResults") | not) or ((.maxResults | type == "number") and .maxResults >= 0 and (.maxResults | floor == .)))
    and ((has("total") | not) or ((.total | type == "number") and .total >= 0 and (.total | floor == .)))
    and ((has("isLast") | not) or (.isLast | type == "boolean"))
  '

  if [[ "$issue_type" == "Story" ]]; then
    schema="${schema}"'
      and all(.issues[];
        (.key | type == "string") and (.key | length > 0)
        and (.fields | type == "object")
        and (.fields.summary | type == "string")
        and (.fields.status | type == "object")
        and (.fields.status.name | type == "string")
        and ((.fields.priority == null) or
          ((.fields.priority | type == "object") and
           ((.fields.priority.name == null) or (.fields.priority.name | type == "string"))))
        and ((.fields.labels == null) or
          ((.fields.labels | type == "array") and all(.fields.labels[]; type == "string")))
        and ((.fields.customfield_10000 == null) or (.fields.customfield_10000 | type == "string"))
        and ((.fields.subtasks == null) or
          ((.fields.subtasks | type == "array") and
           all(.fields.subtasks[];
             (.key | type == "string")
             and (.fields | type == "object")
             and (.fields.summary | type == "string"))))
      )
    '
  else
    schema="${schema}"'
      and all(.issues[];
        (.key | type == "string") and (.key | length > 0)
        and (.fields | type == "object")
        and (.fields.summary | type == "string")
        and ((.fields.description == null) or (.fields.description | type == "string"))
      )
    '
  fi

  labels_jql="labels in (${JIRA_BOARD_LABELS}) AND issuetype = ${issue_type}"

  while true; do
    if ! page_response=$(retry_read_json \
      "Jira ${issue_type} search at offset ${start_at}" \
      "$schema" \
      curl -fsS --retry 2 --retry-all-errors --retry-delay 0 --retry-max-time 30 \
      -H "Authorization: Bearer ${JIRA_TOKEN}" \
      "${JIRA_URL}/rest/api/2/search?jql=$(printf '%s' "$labels_jql" | jq -sRr @uri)&maxResults=${requested_max}&startAt=${start_at}&fields=${fields}"); then
      return 1
    fi

    page_issues=$(jq -c '.issues' <<< "$page_response")
    page_count=$(jq 'length' <<< "$page_issues")
    response_start=$(jq -r --argjson fallback "$start_at" '.startAt // $fallback' <<< "$page_response")
    response_max=$(jq -r --argjson fallback "$requested_max" '.maxResults // $fallback' <<< "$page_response")
    total=$(jq -r '.total // -1' <<< "$page_response")
    is_last=$(jq -r '.isLast // false' <<< "$page_response")
    has_is_last=$(jq -r 'has("isLast")' <<< "$page_response")
    has_response_max=$(jq -r 'has("maxResults")' <<< "$page_response")

    if (( response_start != start_at )); then
      api_error "Jira ${issue_type} search" "pagination" \
        "requested offset ${start_at}, response reported ${response_start}"
      return 1
    fi
    if [[ "$has_response_max" == "true" ]] && (( page_count > response_max )); then
      api_error "Jira ${issue_type} search" "pagination" \
        "response contained ${page_count} issues but maxResults was ${response_max}"
      return 1
    fi

    if (( page_count == 0 )); then
      if (( total >= 0 && response_start < total )) \
        || [[ "$has_is_last" == "true" && "$is_last" != "true" ]]; then
        api_error "Jira ${issue_type} search" "pagination" \
          "empty page at offset ${response_start} before the result set was complete"
        return 1
      fi
      break
    fi

    if ! all_issues=$(jq -e -s 'if all(.[]; type == "array") then add else error("invalid page") end' \
      <<< "${all_issues}"$'\n'"${page_issues}"); then
      api_error "Jira ${issue_type} search" "pagination" "could not combine response pages"
      return 1
    fi

    next_start=$((response_start + page_count))
    if (( next_start <= start_at )); then
      api_error "Jira ${issue_type} search" "pagination" \
        "next offset ${next_start} did not advance beyond ${start_at}"
      return 1
    fi

    if [[ "$is_last" == "true" ]]; then
      if (( total >= 0 && next_start < total )); then
        api_error "Jira ${issue_type} search" "pagination" \
          "Jira marked an incomplete page as the last page"
        return 1
      fi
      break
    fi
    if (( total >= 0 && next_start >= total )) \
      && [[ "$has_is_last" != "true" || "$is_last" == "true" ]]; then
      break
    fi

    start_at=$next_start
  done

  JIRA_SEARCH_JSON="$all_issues"
  JIRA_SEARCH_COUNT=$(jq 'length' <<< "$JIRA_SEARCH_JSON")
  return 0
}

fetch_jira_epics() {
  if ! fetch_jira_search_paginated "Epic" "key,summary,status,description"; then
    return 1
  fi
  JIRA_EPICS_JSON="$JIRA_SEARCH_JSON"
  JIRA_EPICS_COUNT="$JIRA_SEARCH_COUNT"
}

fetch_jira_stories_paginated() {
  if ! fetch_jira_search_paginated "Story" \
    "key,summary,description,status,priority,labels,issuetype,customfield_10000,subtasks"; then
    return 1
  fi
  JIRA_STORIES_JSON="$JIRA_SEARCH_JSON"
  JIRA_STORIES_COUNT="$JIRA_SEARCH_COUNT"
}
