sync_epics() {
  echo ""
  echo "  ${MAGENTA}${ICON_SYNC}${RESET} ${BOLD}Sync Epics from Jira${RESET}"
  echo ""

  # Fetch epics
  echo -n "  ${DIM}Fetching Jira epics...${RESET}"
  if ! fetch_jira_epics; then
    printf "\r                                      \r"
    echo "  ${RED}${ICON_WARN}${RESET} Could not connect to Jira."
    echo "  ${DIM}Check VPN/Wireguard connection.${RESET}"
    echo ""
    return 1
  fi
  printf "\r                                      \r"
  echo "  ${GREEN}${ICON_OK}${RESET} ${DIM}${JIRA_EPICS_COUNT} epics fetched from Jira${RESET}"

  if [[ "$JIRA_EPICS_COUNT" -eq 0 ]]; then
    echo "  ${DIM}No epics found matching board labels.${RESET}"
    echo ""
    return 0
  fi

  # Fetch existing milestones
  echo -n "  ${DIM}Checking existing milestones...${RESET}"
  local ms_json
  ms_json=$(fetch_all_milestones)
  printf "\r                                      \r"

  # Classify epics into new vs existing (to update)
  local -a new_epics=()
  local -a new_epic_keys=()
  local -a new_epic_descs=()
  local -a update_epics=()
  local -a update_epic_keys=()
  local -a update_epic_descs=()
  local -a update_epic_ms_ids=()

  local ekey etitle edesc mdesc ms_id
  while IFS= read -r epic; do
    ekey=$(jq -r '.key' <<< "$epic")
    etitle=$(jq -r '.fields.summary' <<< "$epic")
    edesc=$(jq -r '.fields.description // ""' <<< "$epic")
    mdesc=$(jira_to_markdown "$edesc")
    # Append Jira key tag for future matching
    mdesc="${mdesc:+$mdesc

}<!-- jira:${ekey} -->"
    # Match by Jira key in description first, then fall back to title
    ms_id=$(jq -r --arg k "$ekey" '[.[] | select(.description // "" | contains("<!-- jira:" + $k + " -->"))][0] | .id // empty' <<< "$ms_json" 2>/dev/null)
    if [[ -z "$ms_id" ]]; then
      ms_id=$(jq -r --arg t "$etitle" '[.[] | select(.title == $t)][0] | .id // empty' <<< "$ms_json" 2>/dev/null)
    fi
    if [[ -n "$ms_id" ]]; then
      update_epics+=("$etitle")
      update_epic_keys+=("$ekey")
      update_epic_descs+=("$mdesc")
      update_epic_ms_ids+=("$ms_id")
    else
      new_epics+=("$etitle")
      new_epic_keys+=("$ekey")
      new_epic_descs+=("$mdesc")
    fi
  done < <(jq -c '.[]' <<< "$JIRA_EPICS_JSON")

  if [[ ${#new_epics[@]} -eq 0 && ${#update_epics[@]} -eq 0 ]]; then
    echo "  ${DIM}No epics found matching board labels.${RESET}"
    echo ""
    return 0
  fi

  # Dry-run preview
  echo ""
  hr
  echo ""
  echo "  ${BOLD}Preview${RESET}"
  echo ""
  if [[ ${#new_epics[@]} -gt 0 ]]; then
    echo "  ${GREEN}${ICON_NEW}${RESET} ${#new_epics[@]} milestones to create:"
    for (( i=1; i<=${#new_epics[@]}; i++ )); do
      echo "    ${new_epics[$i]} ${DIM}(${new_epic_keys[$i]})${RESET}"
    done
  fi
  if [[ ${#update_epics[@]} -gt 0 ]]; then
    echo "  ${ICON_SYNC} ${#update_epics[@]} milestones to update:"
    for (( i=1; i<=${#update_epics[@]}; i++ )); do
      echo "    ${update_epics[$i]} ${DIM}(${update_epic_keys[$i]})${RESET}"
    done
  fi
  echo ""
  hr
  echo ""

  echo -n "  ${BOLD}Proceed?${RESET} ${DIM}(y/n)${RESET} "
  read -r confirm
  if [[ "$confirm" != "y" ]]; then
    echo ""
    echo "  ${YELLOW}${ICON_WARN}${RESET} Aborted."
    return 0
  fi

  echo ""

  # Create new milestones
  local created=0 updated=0 failed=0 ms_title ms_desc ms_cmd
  for (( i=1; i<=${#new_epics[@]}; i++ )); do
    ms_title="${new_epics[$i]}"
    ms_desc="${new_epic_descs[$i]}"
    ms_cmd=(glab api "projects/$project_id/milestones" -X POST -f "title=$ms_title")
    [[ -n "$ms_desc" ]] && ms_cmd+=(-f "description=$ms_desc")
    if retry 3 "${ms_cmd[@]}"; then
      echo "  ${GREEN}${ICON_OK}${RESET} ${ms_title} ${DIM}(created)${RESET}"
      ((created++))
    else
      echo "  ${RED}${ICON_WARN}${RESET} ${ms_title} ${DIM}— failed${RESET}"
      ((failed++))
    fi
  done

  # Update existing milestones
  for (( i=1; i<=${#update_epics[@]}; i++ )); do
    ms_title="${update_epics[$i]}"
    ms_desc="${update_epic_descs[$i]}"
    ms_id="${update_epic_ms_ids[$i]}"
    ms_cmd=(glab api "projects/$project_id/milestones/$ms_id" -X PUT -f "title=$ms_title" -f "description=$ms_desc")
    if retry 3 "${ms_cmd[@]}"; then
      echo "  ${GREEN}${ICON_OK}${RESET} ${ms_title} ${DIM}(updated)${RESET}"
      ((updated++))
    else
      echo "  ${RED}${ICON_WARN}${RESET} ${ms_title} ${DIM}— update failed${RESET}"
      ((failed++))
    fi
  done

  echo ""
}
