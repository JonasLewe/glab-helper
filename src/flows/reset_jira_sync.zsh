# Maintenance-only cleanup for all GitLab issues and milestones.

reset_project_planning_data() {
  local issues_json milestones_json reset_issues reset_milestones
  local current_issues_json current_milestones_json current_reset_issues current_reset_milestones
  local planned_issue_fingerprint current_issue_fingerprint
  local planned_milestone_fingerprint current_milestone_fingerprint
  local issue_count milestone_count expected_confirmation confirmation
  local iid milestone_id
  local delete_error
  local deleted_issues=0 deleted_milestones=0 failed_issues=0 failed_milestones=0
  local issue_schema='type == "array" and all(.[];
    (.iid | type == "number")
    and (.title | type == "string")
    and ((.description == null) or (.description | type == "string")))'
  local milestone_schema='type == "array" and all(.[];
    (.id | type == "number")
    and (.title | type == "string")
    and ((.description == null) or (.description | type == "string")))'

  echo ""
  echo "  ${RED}${ICON_WARN}${RESET} ${BOLD}Reset all issues and milestones${RESET}"
  echo ""

  if [[ "${MAINTENANCE_MODE:-false}" != "true" ]]; then
    echo "  ${RED}${ICON_WARN}${RESET} This action requires --maintenance."
    echo ""
    return 1
  fi

  if [[ "${JIRA_AVAILABLE:-false}" != "true" \
    || -z "${JIRA_TARGET_PROJECT:-}" \
    || "$repo_name" != "$JIRA_TARGET_PROJECT" ]]; then
    echo "  ${RED}${ICON_WARN}${RESET} Reset blocked: this repository is not the configured Jira target project."
    echo ""
    return 1
  fi

  echo -n "  ${DIM}Building deletion plan...${RESET}"
  if ! issues_json=$(fetch_all_issues "all") \
    || ! milestones_json=$(fetch_all_milestones ""); then
    printf "\r                                      \r"
    echo "  ${RED}${ICON_WARN}${RESET} Reset aborted because GitLab data could not be read completely."
    echo ""
    return 1
  fi

  if ! jq -e "$issue_schema" <<< "$issues_json" &>/dev/null \
    || ! jq -e "$milestone_schema" <<< "$milestones_json" &>/dev/null; then
    printf "\r                                      \r"
    echo "  ${RED}${ICON_WARN}${RESET} Reset aborted because GitLab returned an unexpected data shape."
    echo ""
    return 1
  fi

  reset_issues="$issues_json"
  reset_milestones="$milestones_json"
  issue_count=$(jq 'length' <<< "$reset_issues")
  milestone_count=$(jq 'length' <<< "$reset_milestones")
  printf "\r                                      \r"

  echo "  ${DIM}Project:${RESET} ${BOLD}${repo_name}${RESET}"
  echo ""

  if [[ "$issue_count" -eq 0 && "$milestone_count" -eq 0 ]]; then
    echo "  ${DIM}No issues or milestones found.${RESET}"
    echo ""
    return 0
  fi

  echo "  ${RED}${issue_count}${RESET} issues will be permanently deleted:"
  if [[ "$issue_count" -gt 0 ]]; then
    jq -r '.[] | "    #\(.iid) [\(.state // "unknown")] \(.title)"' <<< "$reset_issues"
  fi
  echo ""
  echo "  ${RED}${milestone_count}${RESET} milestones will be permanently deleted:"
  if [[ "$milestone_count" -gt 0 ]]; then
    jq -r '.[] | "    ID \(.id) \(.title)"' <<< "$reset_milestones"
  fi
  echo ""
  echo "  ${DIM}Branches, labels, and merge requests are preserved.${RESET}"
  echo ""

  if [[ "${DRY_RUN_MODE:-false}" == "true" ]]; then
    echo "  ${GREEN}${ICON_OK}${RESET} ${BOLD}Dry-run complete${RESET}"
    echo "  ${DIM}No GitLab changes or local snapshots were written.${RESET}"
    echo ""
    return 0
  fi

  expected_confirmation="RESET ALL ${repo_name}"
  echo "  ${RED}${BOLD}This permanently deletes GitLab issues and their discussions.${RESET}"
  echo -n "  ${BOLD}Type '${expected_confirmation}' to continue:${RESET} "
  read -r confirmation
  if [[ "$confirmation" != "$expected_confirmation" ]]; then
    echo ""
    echo "  ${YELLOW}${ICON_WARN}${RESET} Aborted. Nothing was deleted."
    echo ""
    return 0
  fi

  echo ""
  echo "  ${DIM}Creating mandatory pre-reset snapshot...${RESET}"
  if ! export_gitlab_snapshot "pre-reset-all-issues-milestones"; then
    echo "  ${RED}${ICON_WARN}${RESET} Reset aborted because the mandatory snapshot failed."
    echo ""
    return 1
  fi

  echo "  ${DIM}Revalidating deletion plan...${RESET}"
  if ! current_issues_json=$(fetch_all_issues "all") \
    || ! current_milestones_json=$(fetch_all_milestones "") \
    || ! jq -e "$issue_schema" <<< "$current_issues_json" &>/dev/null \
    || ! jq -e "$milestone_schema" <<< "$current_milestones_json" &>/dev/null; then
    echo "  ${RED}${ICON_WARN}${RESET} Reset aborted because the deletion plan could not be revalidated."
    echo ""
    return 1
  fi

  current_reset_issues="$current_issues_json"
  current_reset_milestones="$current_milestones_json"
  planned_issue_fingerprint=$(jq -S -c 'map({iid, title, description}) | sort_by(.iid)' <<< "$reset_issues")
  current_issue_fingerprint=$(jq -S -c 'map({iid, title, description}) | sort_by(.iid)' <<< "$current_reset_issues")
  planned_milestone_fingerprint=$(jq -S -c 'map({id, title, description}) | sort_by(.id)' <<< "$reset_milestones")
  current_milestone_fingerprint=$(jq -S -c 'map({id, title, description}) | sort_by(.id)' <<< "$current_reset_milestones")

  if [[ "$planned_issue_fingerprint" != "$current_issue_fingerprint" \
    || "$planned_milestone_fingerprint" != "$current_milestone_fingerprint" ]]; then
    echo "  ${RED}${ICON_WARN}${RESET} Reset aborted because the deletion plan changed after confirmation."
    echo "  ${DIM}Review a fresh preview before trying again.${RESET}"
    echo ""
    return 1
  fi

  echo "  ${DIM}Deleting all issues...${RESET}"
  while IFS= read -r iid; do
    [[ -z "$iid" ]] && continue
    delete_error=""
    if require_writes_allowed "delete GitLab issue" \
      && delete_error=$(glab api "projects/$project_id/issues/$iid" -X DELETE 2>&1); then
      ((deleted_issues++))
    else
      ((failed_issues++))
      echo "  ${RED}${ICON_WARN}${RESET} Failed to delete issue #${iid}: $(summarize_error "$delete_error")"
    fi
  done < <(jq -r '.[].iid' <<< "$reset_issues")

  if [[ "$failed_issues" -gt 0 ]]; then
    echo "  ${RED}${ICON_WARN}${RESET} Milestone deletion skipped because ${failed_issues} issue deletion(s) failed."
    echo "  ${DIM}${deleted_issues} issues deleted; rerun after resolving permissions or API errors.${RESET}"
    echo ""
    return 1
  fi

  echo "  ${DIM}Deleting all milestones...${RESET}"
  while IFS= read -r milestone_id; do
    [[ -z "$milestone_id" ]] && continue
    delete_error=""
    if require_writes_allowed "delete GitLab milestone" \
      && delete_error=$(glab api "projects/$project_id/milestones/$milestone_id" -X DELETE 2>&1); then
      ((deleted_milestones++))
    else
      ((failed_milestones++))
      echo "  ${RED}${ICON_WARN}${RESET} Failed to delete milestone ID ${milestone_id}: $(summarize_error "$delete_error")"
    fi
  done < <(jq -r '.[].id' <<< "$reset_milestones")

  echo ""
  if [[ "$failed_milestones" -gt 0 ]]; then
    echo "  ${RED}${ICON_WARN}${RESET} Reset incomplete: ${deleted_issues} issues and ${deleted_milestones} milestones deleted; ${failed_milestones} milestone deletion(s) failed."
    echo ""
    return 1
  fi

  echo "  ${GREEN}${ICON_OK}${RESET} Reset complete: ${deleted_issues} issues and ${deleted_milestones} milestones deleted."
  echo "  ${DIM}Branches and labels were not changed.${RESET}"
  echo ""
  return 0
}
