sync_stories() {
  echo ""
  echo "  ${MAGENTA}${ICON_SYNC}${RESET} ${BOLD}Sync Stories from Jira${RESET}"
  echo ""

  # Fetch stories (paginated)
  echo -n "  ${DIM}Fetching Jira stories...${RESET}"
  if ! fetch_jira_stories_paginated; then
    printf "\r                                      \r"
    echo "  ${RED}${ICON_WARN}${RESET} Could not connect to Jira."
    echo "  ${DIM}Check VPN/Wireguard connection.${RESET}"
    echo ""
    return 1
  fi
  printf "\r                                      \r"
  echo "  ${GREEN}${ICON_OK}${RESET} ${DIM}${JIRA_STORIES_COUNT} stories fetched from Jira${RESET}"

  # Fetch existing GitLab issues (paginated) to find already-synced keys
  echo -n "  ${DIM}Checking synced issues...${RESET}"
  local gl_issues_json synced_keys
  local gl_page=1 gl_per_page=100
  local gl_page_json gl_page_count
  gl_issues_json="[]"
  while true; do
    gl_page_json=$(safe_json "$(glab api "projects/$project_id/issues?state=all&per_page=${gl_per_page}&page=${gl_page}" 2>/dev/null)" "[]")
    gl_page_count=$(jq 'length' <<< "$gl_page_json")
    gl_issues_json=$(jq -s '.[0] + .[1]' <<< "${gl_issues_json}"$'\n'"${gl_page_json}")
    [[ "$gl_page_count" -lt "$gl_per_page" ]] && break
    ((gl_page++))
  done
  synced_keys=$(jq -r '.[].title | capture("^\\[(?<key>[A-Z]+-[0-9]+)\\](\\s|$)")?.key // empty' <<< "$gl_issues_json")
  printf "\r                                      \r"

  # Split into unsynced (new) and synced (update candidates)
  local unsynced_json synced_json unsynced_count synced_count
  unsynced_json=$(jq --arg synced "$synced_keys" '
    ($synced | split("\n") | map(select(length > 0))) as $keys |
    map(select(.key as $k | ($keys | index($k)) | not))
  ' <<< "$JIRA_STORIES_JSON")
  synced_json=$(jq --arg synced "$synced_keys" '
    ($synced | split("\n") | map(select(length > 0))) as $keys |
    map(select(.key as $k | ($keys | index($k)) | not | not))
  ' <<< "$JIRA_STORIES_JSON")
  unsynced_count=$(jq 'length' <<< "$unsynced_json")
  synced_count=$(jq 'length' <<< "$synced_json")

  if [[ "$unsynced_count" -eq 0 && "$synced_count" -eq 0 ]]; then
    echo "  ${DIM}No stories found matching board labels.${RESET}"
    echo ""
    return 0
  fi

  # Fetch all epics for milestone assignment (same as sync_epics)
  echo -n "  ${DIM}Fetching epics...${RESET}"
  typeset -A epic_titles
  typeset -A epic_descs
  if ! fetch_jira_epics; then
    printf "\r                                      \r"
    echo "  ${YELLOW}${ICON_WARN}${RESET} ${DIM}Could not fetch epics — milestones will be skipped${RESET}"
  else
    printf "\r                                      \r"
    echo "  ${GREEN}${ICON_OK}${RESET} ${DIM}${JIRA_EPICS_COUNT} epics fetched from Jira${RESET}"
    local ekey etitle edesc mdesc
    while IFS= read -r epic; do
      [[ -z "$epic" ]] && continue
      ekey=$(jq -r '.key' <<< "$epic")
      etitle=$(jq -r '.fields.summary' <<< "$epic")
      edesc=$(jq -r '.fields.description // ""' <<< "$epic")
      if [[ -n "$etitle" ]]; then
        epic_titles[$ekey]="$etitle"
        mdesc=$(jira_to_markdown "$edesc")
        epic_descs[$ekey]="${mdesc:+$mdesc

}<!-- jira:${ekey} -->"
      fi
    done < <(jq -c '.[]' <<< "$JIRA_EPICS_JSON")
  fi

  # Check which milestones need creation
  local ms_json
  ms_json=$(fetch_all_milestones)

  local -a new_ms_keys=()
  local -a update_ms_keys=()
  local ms_id
  for ek in "${(@k)epic_titles}"; do
    # Match by Jira key in description first, then fall back to title
    ms_id=$(jq -r --arg k "$ek" '[.[] | select(.description // "" | contains("<!-- jira:" + $k + " -->"))][0] | .id // empty' <<< "$ms_json" 2>/dev/null)
    if [[ -z "$ms_id" ]]; then
      ms_id=$(jq -r --arg t "${epic_titles[$ek]}" '[.[] | select(.title == $t)][0] | .id // empty' <<< "$ms_json" 2>/dev/null)
    fi
    if [[ -n "$ms_id" ]]; then
      update_ms_keys+=("$ek")
    else
      new_ms_keys+=("$ek")
    fi
  done

  # Dry-run preview
  echo ""
  hr
  echo ""
  echo "  ${BOLD}Preview${RESET}"
  echo ""
  if [[ ${#new_ms_keys[@]} -gt 0 ]]; then
    echo "  ${ICON_MILE} ${#new_ms_keys[@]} milestones to create:"
    for ek in "${new_ms_keys[@]}"; do
      echo "    ${epic_titles[$ek]}"
    done
  fi
  if [[ ${#update_ms_keys[@]} -gt 0 ]]; then
    echo "  ${ICON_SYNC} ${#update_ms_keys[@]} milestones to update:"
    for ek in "${update_ms_keys[@]}"; do
      echo "    ${epic_titles[$ek]}"
    done
  fi
  local skey stitle
  if [[ $unsynced_count -gt 0 ]]; then
    echo "  ${GREEN}${ICON_NEW}${RESET} ${unsynced_count} issues to create:"
    while IFS= read -r line; do
      skey=$(jq -r '.key' <<< "$line")
      stitle=$(jq -r '.fields.summary' <<< "$line")
      echo "    [${skey}] ${stitle}"
    done < <(jq -c '.[]' <<< "$unsynced_json")
  fi
  if [[ $synced_count -gt 0 ]]; then
    echo "  ${ICON_SYNC} ${synced_count} issues to check for updates"
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

  # Create missing milestones
  local ms_created=0 ms_updated=0 ms_failed=0 ms_cmd
  if [[ ${#new_ms_keys[@]} -gt 0 || ${#update_ms_keys[@]} -gt 0 ]]; then
    echo "  ${DIM}Creating milestones...${RESET}"
    echo ""
  fi
  for ek in "${new_ms_keys[@]}"; do
    ms_cmd=(glab api "projects/$project_id/milestones" -X POST -f "title=${epic_titles[$ek]}")
    [[ -n "${epic_descs[$ek]:-}" ]] && ms_cmd+=(-f "description=${epic_descs[$ek]}")
    if retry 3 "${ms_cmd[@]}"; then
      echo "  ${GREEN}${ICON_OK}${RESET} ${epic_titles[$ek]} ${DIM}(created)${RESET}"
      ((ms_created++))
    else
      echo "  ${RED}${ICON_WARN}${RESET} ${epic_titles[$ek]} ${DIM}— failed${RESET}"
      ((ms_failed++))
    fi
  done

  # Update existing milestones (title + description)
  for ek in "${update_ms_keys[@]}"; do
    ms_id=$(jq -r --arg k "$ek" '[.[] | select(.description // "" | contains("<!-- jira:" + $k + " -->"))][0] | .id // empty' <<< "$ms_json" 2>/dev/null)
    if [[ -z "$ms_id" ]]; then
      ms_id=$(jq -r --arg t "${epic_titles[$ek]}" '[.[] | select(.title == $t)][0] | .id // empty' <<< "$ms_json" 2>/dev/null)
    fi
    ms_cmd=(glab api "projects/$project_id/milestones/$ms_id" -X PUT -f "title=${epic_titles[$ek]}" -f "description=${epic_descs[$ek]:-}")
    if retry 3 "${ms_cmd[@]}"; then
      echo "  ${GREEN}${ICON_OK}${RESET} ${epic_titles[$ek]} ${DIM}(updated)${RESET}"
      ((ms_updated++))
    else
      echo "  ${RED}${ICON_WARN}${RESET} ${epic_titles[$ek]} ${DIM}— update failed${RESET}"
      ((ms_failed++))
    fi
  done

  # Ensure all needed labels exist in GitLab
  echo -n "  ${DIM}Syncing labels...${RESET}"
  local existing_labels
  existing_labels=$(jq -r '.[].name' <<< "$(fetch_all_labels)" 2>/dev/null || echo "")
  local all_needed_labels
  all_needed_labels=$(jq -r '
    ([.[].fields.labels[]?] | unique) +
    ([.[].fields.priority.name // empty] | unique | map("prio::" + .))
    | unique | .[]
  ' <<< "$JIRA_STORIES_JSON")

  while IFS= read -r lbl; do
    [[ -z "$lbl" ]] && continue
    if ! grep -qxF "$lbl" <<< "$existing_labels"; then
      glab label create -n "$lbl" &>/dev/null && existing_labels="${existing_labels}"$'\n'"${lbl}"
    fi
  done <<< "$all_needed_labels"
  printf "\r                                      \r"

  # Create issues
  echo "  ${DIM}Creating issues...${RESET}"
  echo ""

  local created=0 failed=0
  local s_key s_summary s_desc s_priority s_labels_json s_epic_key s_subtasks_json
  local s_title s_description st_count st_key st_summary s_labels_csv s_milestone
  local cmd output attempt max_attempts success
  while IFS= read -r story; do
    s_key=$(jq -r '.key' <<< "$story")
    s_summary=$(jq -r '.fields.summary' <<< "$story")
    s_desc=$(jq -r '.fields.description // ""' <<< "$story")
    s_priority=$(jq -r '.fields.priority.name // ""' <<< "$story")
    s_labels_json=$(jq '.fields.labels // []' <<< "$story")
    s_epic_key=$(jq -r '.fields.customfield_10000 // ""' <<< "$story")
    s_subtasks_json=$(jq '.fields.subtasks // []' <<< "$story")

    # Title
    s_title="[${s_key}] ${s_summary}"

    # Description
    s_description=$(jira_to_markdown "$s_desc")

    # Subtasks as checkboxes
    st_count=$(jq 'length' <<< "$s_subtasks_json")
    if [[ "$st_count" -gt 0 ]]; then
      s_description="${s_description}"$'\n\n'"## Subtasks"
      while IFS= read -r st; do
        st_key=$(jq -r '.key' <<< "$st")
        st_summary=$(jq -r '.fields.summary' <<< "$st")
        s_description="${s_description}"$'\n'"- [ ] ${st_key}: ${st_summary}"
      done < <(jq -c '.[]' <<< "$s_subtasks_json")
    fi

    # Labels
    s_labels_csv=$(jq -r 'join(",")' <<< "$s_labels_json")
    if [[ -n "$s_priority" ]]; then
      if [[ -n "$s_labels_csv" ]]; then
        s_labels_csv="${s_labels_csv},prio::${s_priority}"
      else
        s_labels_csv="prio::${s_priority}"
      fi
    fi

    # Milestone from epic
    s_milestone=""
    if [[ -n "$s_epic_key" && "$s_epic_key" != "null" ]]; then
      s_milestone="${epic_titles[$s_epic_key]:-}"
    fi

    # Build command
    cmd=(glab issue create -t "$s_title" -d "$s_description")
    [[ -n "$s_labels_csv" ]] && cmd+=(-l "$s_labels_csv")
    [[ -n "$s_milestone" ]] && cmd+=(-m "$s_milestone")

    output="" attempt=1 max_attempts=3 success=false
    while (( attempt <= max_attempts )); do
      if output=$("${cmd[@]}" 2>&1); then
        success=true
        break
      fi
      ((attempt++))
      [[ $attempt -le $max_attempts ]] && sleep 1
    done
    if $success; then
      echo "  ${GREEN}${ICON_OK}${RESET} [${s_key}] ${s_summary}"
      ((created++))
    else
      echo "  ${RED}${ICON_WARN}${RESET} [${s_key}] ${s_summary} ${DIM}— ${output}${RESET}"
      ((failed++))
    fi
  done < <(jq -c '.[]' <<< "$unsynced_json")

  # Update existing issues (title, labels merge, milestone)
  local issue_updated=0 issue_uptodate=0
  if [[ "$synced_count" -gt 0 ]]; then
    echo ""
    echo "  ${DIM}Checking for updates...${RESET}"
    echo ""

    # Refresh milestone list (new ones may have just been created)
    ms_json=$(fetch_all_milestones)

    while IFS= read -r story; do
      s_key=$(jq -r '.key' <<< "$story")
      s_summary=$(jq -r '.fields.summary' <<< "$story")
      s_desc=$(jq -r '.fields.description // ""' <<< "$story")
      s_priority=$(jq -r '.fields.priority.name // ""' <<< "$story")
      s_labels_json=$(jq '.fields.labels // []' <<< "$story")
      s_epic_key=$(jq -r '.fields.customfield_10000 // ""' <<< "$story")
      s_subtasks_json=$(jq '.fields.subtasks // []' <<< "$story")

      # Find corresponding GitLab issue
      gl_issue=$(jq -c --arg k "$s_key" '.[] | select(.title | test("^\\[" + $k + "\\](\\s|$)"))' <<< "$gl_issues_json" | head -1)
      [[ -z "$gl_issue" ]] && continue
      gl_iid=$(jq -r '.iid' <<< "$gl_issue")
      gl_title=$(jq -r '.title' <<< "$gl_issue")
      gl_description=$(jq -r '.description // ""' <<< "$gl_issue")
      gl_labels_csv=$(jq -r '[.labels[]?] | join(",")' <<< "$gl_issue")
      gl_milestone=$(jq -r '.milestone.title // ""' <<< "$gl_issue")

      # Build expected values from Jira
      new_title="[${s_key}] ${s_summary}"
      new_description=$(jira_to_markdown "$s_desc")
      st_count=$(jq 'length' <<< "$s_subtasks_json")
      if [[ "$st_count" -gt 0 ]]; then
        new_description="${new_description}"$'\n\n'"## Subtasks"
        while IFS= read -r st; do
          st_key=$(jq -r '.key' <<< "$st")
          st_summary=$(jq -r '.fields.summary' <<< "$st")
          new_description="${new_description}"$'\n'"- [ ] ${st_key}: ${st_summary}"
        done < <(jq -c '.[]' <<< "$s_subtasks_json")
      fi

      # Merge labels: existing GitLab labels + Jira labels + priority label
      jira_labels=$(jq -r '.[]' <<< "$s_labels_json")
      [[ -n "$s_priority" ]] && jira_labels="${jira_labels:+$jira_labels
}prio::${s_priority}"
      # Union: start with GitLab labels, add missing Jira labels
      merged_labels="$gl_labels_csv"
      while IFS= read -r jl; do
        [[ -z "$jl" ]] && continue
        # Use exact match on comma-separated list to avoid substring false positives
        if ! tr ',' '\n' <<< "$merged_labels" | grep -qxF "$jl"; then
          merged_labels="${merged_labels:+$merged_labels,}${jl}"
        fi
      done <<< "$jira_labels"
      # Update old prio:: label if priority changed
      if [[ -n "$s_priority" ]]; then
        merged_labels=$(echo "$merged_labels" | tr ',' '\n' | grep -v '^prio::' | tr '\n' ',' | sed 's/,$//')
        merged_labels="${merged_labels:+$merged_labels,}prio::${s_priority}"
      fi

      # Resolve milestone from epic
      new_milestone=""
      if [[ -n "$s_epic_key" && "$s_epic_key" != "null" ]]; then
        new_milestone="${epic_titles[$s_epic_key]:-}"
      fi

      # Check if anything changed
      changed=false
      [[ "$new_title" != "$gl_title" ]] && changed=true
      [[ "$new_description" != "$gl_description" ]] && changed=true
      [[ "$merged_labels" != "$gl_labels_csv" ]] && changed=true
      [[ "$new_milestone" != "$gl_milestone" ]] && changed=true

      if ! $changed; then
        ((issue_uptodate++))
        continue
      fi

      # Build update command
      cmd=(glab api "projects/$project_id/issues/$gl_iid" -X PUT)
      cmd+=(-f "title=$new_title")
      cmd+=(-f "description=$new_description")
      [[ -n "$merged_labels" ]] && cmd+=(-f "labels=$merged_labels")
      if [[ -n "$new_milestone" ]]; then
        ms_id=$(jq -r --arg t "$new_milestone" '[.[] | select(.title == $t)][0] | .id // empty' <<< "$ms_json" 2>/dev/null)
        [[ -n "$ms_id" ]] && cmd+=(-f "milestone_id=$ms_id")
      elif [[ -n "$gl_milestone" ]]; then
        cmd+=(-f "milestone_id=0")
      fi

      if retry 3 "${cmd[@]}"; then
        echo "  ${GREEN}${ICON_OK}${RESET} [${s_key}] ${s_summary} ${DIM}(updated)${RESET}"
        ((issue_updated++))
      else
        echo "  ${RED}${ICON_WARN}${RESET} [${s_key}] ${s_summary} ${DIM}— update failed${RESET}"
        ((failed++))
      fi
    done < <(jq -c '.[]' <<< "$synced_json")
  fi

  # Summary
  echo ""
  hr
  echo ""
  echo "  ${BOLD}Sync Complete${RESET}"
  echo ""
  local issue_parts="${GREEN}${created} created${RESET}"
  [[ $issue_updated -gt 0 ]] && issue_parts="${issue_parts}, ${GREEN}${issue_updated} updated${RESET}"
  [[ $issue_uptodate -gt 0 ]] && issue_parts="${issue_parts}, ${DIM}${issue_uptodate} up-to-date${RESET}"
  [[ $failed -gt 0 ]] && issue_parts="${issue_parts}, ${RED}${failed} failed${RESET}"
  echo "  ${ICON_TITLE} Issues:     ${issue_parts}"
  local ms_parts=""
  [[ $ms_created -gt 0 ]] && ms_parts="${ms_created} created"
  [[ $ms_updated -gt 0 ]] && ms_parts="${ms_parts:+$ms_parts, }${ms_updated} updated"
  if [[ -n "$ms_parts" ]]; then
    echo "  ${ICON_MILE} Milestones: ${GREEN}${ms_parts}${RESET}"
  fi
  echo ""
}
